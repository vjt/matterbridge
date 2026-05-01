package birc

import (
	"sort"
	"testing"

	"github.com/lrstanley/girc"
)

func TestFormatJoinLeaveText(t *testing.T) {
	src := func(name, ident, host string) *girc.Source {
		return &girc.Source{Name: name, Ident: ident, Host: host}
	}

	cases := []struct {
		name    string
		event   girc.Event
		verbose bool
		want    string
	}{
		{
			name:  "join",
			event: girc.Event{Source: src("alice", "alice", "host"), Command: "JOIN", Params: []string{"#chan"}},
			want:  "alice joins",
		},
		{
			name:  "part without reason",
			event: girc.Event{Source: src("alice", "alice", "host"), Command: "PART", Params: []string{"#chan"}},
			want:  "alice parts",
		},
		{
			name:  "part with reason",
			event: girc.Event{Source: src("alice", "alice", "host"), Command: "PART", Params: []string{"#chan", "see ya"}},
			want:  "alice parts (see ya)",
		},
		{
			name:  "quit with reason",
			event: girc.Event{Source: src("alice", "alice", "host"), Command: "QUIT", Params: []string{"Bye"}},
			want:  "alice quits (Bye)",
		},
		{
			name:  "quit without reason",
			event: girc.Event{Source: src("alice", "alice", "host"), Command: "QUIT", Params: []string{}},
			want:  "alice quits",
		},
		{
			name:  "kick with reason",
			event: girc.Event{Source: src("vjt", "vjt", "host"), Command: "KICK", Params: []string{"#chan", "alice", "spam"}},
			want:  "vjt kicked alice (spam)",
		},
		{
			name:  "kick without reason",
			event: girc.Event{Source: src("vjt", "vjt", "host"), Command: "KICK", Params: []string{"#chan", "alice"}},
			want:  "vjt kicked alice",
		},
		{
			name:    "verbose join",
			event:   girc.Event{Source: src("alice", "alice", "host.example"), Command: "JOIN", Params: []string{"#chan"}},
			verbose: true,
			want:    "alice (alice@host.example) joins",
		},
		{
			name:    "verbose kick",
			event:   girc.Event{Source: src("vjt", "vjt", "host.example"), Command: "KICK", Params: []string{"#chan", "alice", "bye"}},
			verbose: true,
			want:    "vjt (vjt@host.example) kicked alice (bye)",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := formatJoinLeaveText(c.event, c.verbose)
			if got != c.want {
				t.Errorf("formatJoinLeaveText() = %q, want %q", got, c.want)
			}
		})
	}
}

func newTrackingBirc() *Birc {
	return &Birc{userChans: make(map[string]map[string]struct{})}
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

func TestUserChansTracking(t *testing.T) {
	b := newTrackingBirc()

	b.trackJoin("Alice", "#one")
	b.trackJoin("alice", "#two")
	b.trackJoin("BOB", "#one")

	got := sortedCopy(b.trackQuit("ALICE"))
	want := []string{"#one", "#two"}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("trackQuit(alice) = %v, want %v", got, want)
	}
	// Quitter is removed from tracking.
	if got2 := b.trackQuit("alice"); got2 != nil {
		t.Errorf("trackQuit(alice) after quit = %v, want nil", got2)
	}

	// Bob still tracked. PART removes only that channel.
	b.trackPart("bob", "#one")
	if got := b.trackQuit("bob"); got != nil {
		t.Errorf("trackQuit(bob) after final part = %v, want nil (purged)", got)
	}

	// Rename carries channel set across.
	b.trackJoin("carol", "#one")
	b.trackRename("CAROL", "Caroline")
	if got := b.trackQuit("carol"); got != nil {
		t.Errorf("trackQuit(carol) after rename = %v, want nil", got)
	}
	if got := b.trackQuit("caroline"); len(got) != 1 || got[0] != "#one" {
		t.Errorf("trackQuit(caroline) = %v, want [#one]", got)
	}
}

func TestHandleNamesReplySeed(t *testing.T) {
	b := newTrackingBirc()
	event := girc.Event{
		Command: girc.RPL_NAMREPLY,
		Params:  []string{"Bot", "=", "#chan", "@op +voiced regular ~founder"},
	}
	b.handleNamesReply(nil, event)

	for _, nick := range []string{"op", "voiced", "regular", "founder"} {
		got := b.trackQuit(nick)
		if len(got) != 1 || got[0] != "#chan" {
			t.Errorf("trackQuit(%s) = %v, want [#chan]", nick, got)
		}
	}
}
