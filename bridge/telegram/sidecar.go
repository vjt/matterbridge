package btelegram

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SidecarMember mirrors the JSON shape returned by tg-members-sidecar.
// Field tags must stay in sync with internal/tgclient/client.go in the
// sidecar repo.
type SidecarMember struct {
	ID        int64  `json:"id"`
	Username  string `json:"username,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
	IsBot     bool   `json:"is_bot,omitempty"`
	IsAdmin   bool   `json:"is_admin,omitempty"`
	IsCreator bool   `json:"is_creator,omitempty"`
	IsPremium bool   `json:"is_premium,omitempty"`
	IsDeleted bool   `json:"is_deleted,omitempty"`
}

type sidecarResponse struct {
	Chat             string          `json:"chat"`
	CachedSecondsTTL int             `json:"cached_seconds_ttl"`
	Members          []SidecarMember `json:"members"`
}

type sidecarClient struct {
	baseURL string
	http    *http.Client
}

func newSidecarClient(baseURL string) *sidecarClient {
	return &sidecarClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		http:    &http.Client{Timeout: 5 * time.Second},
	}
}

// fetchMembers calls GET <baseURL>/members?chat=<chatRef> and decodes the JSON
// response. Returns an error on any non-2xx, network failure, or malformed body.
func (s *sidecarClient) fetchMembers(ctx context.Context, chatRef string) ([]SidecarMember, error) {
	q := url.Values{}
	q.Set("chat", chatRef)
	full := s.baseURL + "/members?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return nil, fmt.Errorf("sidecar: build request: %w", err)
	}
	resp, err := s.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sidecar: GET %s: %w", full, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("sidecar: GET %s: HTTP %d", full, resp.StatusCode)
	}

	var out sidecarResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("sidecar: decode body: %w", err)
	}
	return out.Members, nil
}

// formatMembersOneLine produces a single-line, comma-separated list of TG handles
// suitable for posting on IRC and bridging back to TG without triggering TG mention
// notifications. The `@` prefix is stripped. Members without a username fall back
// to first_name (or "user_<id>" if even that is absent). Deleted/empty entries are
// skipped. Admins/creator are marked with a trailing `*`.
func formatMembersOneLine(ms []SidecarMember) string {
	if len(ms) == 0 {
		return "no members"
	}
	parts := make([]string, 0, len(ms))
	admins := 0
	for _, m := range ms {
		if m.IsDeleted {
			continue
		}
		var label string
		switch {
		case m.Username != "":
			label = m.Username // no @ prefix on purpose — avoids TG @-mention
		case m.FirstName != "":
			label = m.FirstName
		default:
			label = fmt.Sprintf("user_%d", m.ID)
		}
		if m.IsAdmin || m.IsCreator {
			label += "*"
			admins++
		}
		parts = append(parts, label)
	}
	if len(parts) == 0 {
		return "no members"
	}
	return fmt.Sprintf("tg_users (%d, %d admins): %s",
		len(parts), admins, strings.Join(parts, ", "))
}
