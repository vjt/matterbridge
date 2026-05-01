package btelegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
)

// captureDeps returns a tgUsersDeps wired to receive replies on the returned
// channel. Test helper.
func captureDeps(sidecarURL, chatRef, channel string) (tgUsersDeps, chan config.Message) {
	got := make(chan config.Message, 1)
	deps := tgUsersDeps{
		sidecarURL: sidecarURL,
		chatRef:    chatRef,
		channel:    channel,
		account:    "telegram.test",
		pushBack:   func(m config.Message) { got <- m },
	}
	return deps, got
}

func TestRunTGUsersFetch_happyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("chat"); got != "@sniffo" {
			http.Error(w, "wrong chat", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chat":"@sniffo","cached_seconds_ttl":120,"members":[{"id":1,"username":"alice"},{"id":2,"username":"bob","is_admin":true}]}`))
	}))
	defer srv.Close()

	deps, got := captureDeps(srv.URL, "@sniffo", "@sniffo")
	runTGUsersFetch(context.Background(), deps)

	select {
	case m := <-got:
		if !strings.Contains(m.Text, "alice") || !strings.Contains(m.Text, "bob*") {
			t.Errorf("expected handles in reply, got %q", m.Text)
		}
		if strings.Contains(m.Text, "@alice") {
			t.Errorf("@-prefix should be stripped: %q", m.Text)
		}
		if m.Channel != "@sniffo" {
			t.Errorf("expected channel propagated, got %q", m.Channel)
		}
		if m.Account != "telegram.test" {
			t.Errorf("expected account propagated, got %q", m.Account)
		}
		if m.Username != "system" {
			t.Errorf("expected system username, got %q", m.Username)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a reply on the pushBack channel")
	}
}

func TestRunTGUsersFetch_upstreamError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	logged := 0
	deps, got := captureDeps(srv.URL, "@x", "@x")
	deps.logErr = func(string, ...any) { logged++ }
	runTGUsersFetch(context.Background(), deps)

	select {
	case m := <-got:
		if !strings.Contains(m.Text, "failed to fetch") {
			t.Errorf("expected failure reply, got %q", m.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected an error reply on the pushBack channel")
	}
	if logged != 1 {
		t.Errorf("expected logErr to be called once, got %d", logged)
	}
}

func TestRunTGUsersFetch_timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	deps, got := captureDeps(srv.URL, "@x", "@x")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	runTGUsersFetch(ctx, deps)

	select {
	case m := <-got:
		if !strings.Contains(m.Text, "failed to fetch") {
			t.Errorf("expected timeout failure reply, got %q", m.Text)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("expected a timeout reply on the pushBack channel")
	}
}

func TestChatRefFor(t *testing.T) {
	cases := []struct {
		name, channel, override, want string
	}{
		{"override wins", "12345", "@sniffo", "@sniffo"},
		{"strips topic suffix", "12345/67", "", "12345"},
		{"plain numeric passthrough", "-1001234567", "", "-1001234567"},
		{"plain @username passthrough", "@public", "", "@public"},
		{"empty channel passthrough", "", "", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := chatRefFor(c.channel, c.override); got != c.want {
				t.Errorf("chatRefFor(%q, %q) = %q, want %q", c.channel, c.override, got, c.want)
			}
		})
	}
}
