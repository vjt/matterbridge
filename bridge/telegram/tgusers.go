package btelegram

import (
	"context"
	"strings"
	"time"

	"github.com/42wim/matterbridge/bridge/config"
)

// triggerTGUsers is the IRC-side command we intercept on the TG bridge.
const triggerTGUsers = "!tg_users"

// tgUsersFetchTimeout caps how long the sidecar HTTP call may take before we
// post a failure reply.
const tgUsersFetchTimeout = 8 * time.Second

// tgUsersDeps captures everything the !tg_users handler needs from the bridge
// runtime. Pulled into a struct so tests can drive it without constructing a
// full Btelegram (which depends on logrus, the matterbridge config layer, and
// a real bot client).
type tgUsersDeps struct {
	sidecarURL string
	chatRef    string
	channel    string
	account    string
	pushBack   func(config.Message)
	logErr     func(format string, args ...any)
}

// runTGUsersFetch performs the sidecar HTTP fetch and pushes the reply (success
// or failure) via deps.pushBack. Intended to run on a goroutine — synchronous
// callers must respect the timeout via the supplied ctx.
func runTGUsersFetch(ctx context.Context, deps tgUsersDeps) {
	client := newSidecarClient(deps.sidecarURL)
	members, err := client.fetchMembers(ctx, deps.chatRef)
	if err != nil {
		if deps.logErr != nil {
			deps.logErr("tg_users sidecar fetch: %v", err)
		}
		deps.pushBack(config.Message{
			Username: "system",
			Text:     "!tg_users: failed to fetch (" + err.Error() + ")",
			Channel:  deps.channel,
			Account:  deps.account,
		})
		return
	}
	deps.pushBack(config.Message{
		Username: "system",
		Text:     formatMembersOneLine(members),
		Channel:  deps.channel,
		Account:  deps.account,
	})
}

// chatRefFor computes the sidecar's `chat` query parameter from a matterbridge
// channel string and an optional explicit override.
//
//   - When `override` is non-empty, it wins (lets the operator point the
//     sidecar at @username while matterbridge bridges by numeric chat_id, or
//     the inverse).
//   - Otherwise, strip the optional `/<topicid>` suffix used by matterbridge's
//     forum-topic bridges (sidecar wants only the chat id, topics are not a
//     member-list dimension).
func chatRefFor(channel, override string) string {
	if override != "" {
		return override
	}
	if i := strings.Index(channel, "/"); i > 0 {
		channel = channel[:i]
	}
	return channel
}

// maybeHandleTGUsers checks whether the inbound message is `!tg_users` and, if
// so, kicks off the sidecar fetch on a goroutine and tells the caller to
// suppress the original message (return value `true`). Returns `false` for
// any other text — caller handles normally.
func (b *Btelegram) maybeHandleTGUsers(msg *config.Message) bool {
	if strings.TrimSpace(msg.Text) != triggerTGUsers {
		return false
	}

	url := b.GetString("MembersSidecarURL")
	if url == "" {
		b.Remote <- config.Message{
			Username: "system",
			Text:     "!tg_users: MembersSidecarURL not configured on this bridge",
			Channel:  msg.Channel,
			Account:  b.Account,
		}
		return true
	}

	deps := tgUsersDeps{
		sidecarURL: url,
		chatRef:    chatRefFor(msg.Channel, b.GetString("MembersSidecarChat")),
		channel:    msg.Channel,
		account:    b.Account,
		pushBack:   func(m config.Message) { b.Remote <- m },
		logErr:     b.Log.Errorf,
	}
	go func(d tgUsersDeps) {
		ctx, cancel := context.WithTimeout(context.Background(), tgUsersFetchTimeout)
		defer cancel()
		runTGUsersFetch(ctx, d)
	}(deps)
	return true
}
