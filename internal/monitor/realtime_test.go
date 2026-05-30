package monitor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/agnivo988/Repo-lyzer/internal/github"
)

func TestParseRealtimeEvents(t *testing.T) {
	now := time.Now()

	events := []github.Event{
		{
			Type:      "PushEvent",
			Actor:     github.User{Login: "agnivo988"},
			CreatedAt: now,
			Payload: json.RawMessage(`{
				"size": 2,
				"commits": [
					{"message": "feat: add polling support"}
				]
			}`),
		},
		{
			Type:      "IssueCommentEvent",
			Actor:     github.User{Login: "contributor1"},
			CreatedAt: now,
			Payload: json.RawMessage(`{
				"issue": {"number": 123}
			}`),
		},
		{
			Type:      "RandomUnknownEvent",
			Actor:     github.User{Login: "bot"},
			CreatedAt: now,
		},
	}

	feeds := ParseRealtimeEvents(events)

	if len(feeds) != 2 {
		t.Fatalf("expected 2 parsed events, got %d", len(feeds))
	}

	if feeds[0].Category != "COMMIT" {
		t.Errorf("expected COMMIT category, got %s", feeds[0].Category)
	}

	if feeds[1].Category != "ISSUE" {
		t.Errorf("expected ISSUE category, got %s", feeds[1].Category)
	}
}
