package btelegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchMembers_happyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/members" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("chat"); got != "@sniffo" {
			t.Fatalf("expected chat=@sniffo, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
			"chat": "@sniffo",
			"cached_seconds_ttl": 100,
			"members": [
				{"id": 1, "username": "alice", "is_admin": true},
				{"id": 2, "first_name": "Bob"},
				{"id": 3, "is_deleted": true}
			]
		}`))
	}))
	defer srv.Close()

	c := newSidecarClient(srv.URL)
	ms, err := c.fetchMembers(context.Background(), "@sniffo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ms) != 3 {
		t.Fatalf("expected 3 members, got %d", len(ms))
	}
	if ms[0].Username != "alice" || !ms[0].IsAdmin {
		t.Errorf("alice not parsed correctly: %+v", ms[0])
	}
}

func TestFetchMembers_non2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer srv.Close()

	c := newSidecarClient(srv.URL)
	_, err := c.fetchMembers(context.Background(), "@x")
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("expected HTTP 502 error, got %v", err)
	}
}

func TestFetchMembers_timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := newSidecarClient(srv.URL)
	c.http.Timeout = 20 * time.Millisecond
	_, err := c.fetchMembers(context.Background(), "@x")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
}

func TestFormatMembersOneLine_empty(t *testing.T) {
	if got := formatMembersOneLine(nil); got != "no members" {
		t.Errorf("expected 'no members', got %q", got)
	}
}

func TestFormatMembersOneLine_skipsDeleted(t *testing.T) {
	ms := []SidecarMember{
		{ID: 1, Username: "alice"},
		{ID: 2, IsDeleted: true},
		{ID: 3, FirstName: "Bob"},
	}
	got := formatMembersOneLine(ms)
	if !strings.Contains(got, "alice") || !strings.Contains(got, "Bob") {
		t.Errorf("missing expected handles in %q", got)
	}
	if strings.Contains(got, "@alice") {
		t.Errorf("@-prefix should be stripped to avoid TG mention: %q", got)
	}
	if !strings.Contains(got, "(2, 0 admins)") {
		t.Errorf("expected '(2, 0 admins)' summary in %q", got)
	}
}

func TestFormatMembersOneLine_adminMark(t *testing.T) {
	ms := []SidecarMember{
		{ID: 1, Username: "alice", IsAdmin: true},
		{ID: 2, Username: "bob", IsCreator: true},
		{ID: 3, Username: "carol"},
	}
	got := formatMembersOneLine(ms)
	if !strings.Contains(got, "alice*") || !strings.Contains(got, "bob*") {
		t.Errorf("expected admin/creator marker '*' in %q", got)
	}
	if strings.Contains(got, "carol*") {
		t.Errorf("non-admin should not have '*' in %q", got)
	}
	if !strings.Contains(got, "2 admins") {
		t.Errorf("expected '2 admins' summary in %q", got)
	}
}

func TestFormatMembersOneLine_fallbackLabels(t *testing.T) {
	ms := []SidecarMember{
		{ID: 42},                  // no username, no name → user_42
		{ID: 7, FirstName: "Eve"}, // first name only → Eve
	}
	got := formatMembersOneLine(ms)
	if !strings.Contains(got, "user_42") || !strings.Contains(got, "Eve") {
		t.Errorf("expected fallback labels in %q", got)
	}
}
