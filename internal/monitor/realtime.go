package monitor

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/agnivo988/Repo-lyzer/internal/github"
)

// RealtimeEvent represents a categorized live activity event
type RealtimeEvent struct {
	Category    string // "COMMIT", "PR", "ISSUE"
	Description string
	Actor       string
	Time        string
}

// ParseRealtimeEvents converts raw GitHub events into the structured activity feed
func ParseRealtimeEvents(rawEvents []github.Event) []RealtimeEvent {
	var feed []RealtimeEvent

	for _, e := range rawEvents {
		event := RealtimeEvent{
			Actor: e.Actor.Login,
			Time:  e.CreatedAt.Format("15:04:05"),
		}

		switch e.Type {
		case "PushEvent":
			event.Category = "COMMIT"

			// Extract commit count if possible
			var payload struct {
				Size    int `json:"size"`
				Commits []struct {
					Message string `json:"message"`
				} `json:"commits"`
			}
			json.Unmarshal(e.Payload, &payload)

			msg := "Pushed code"
			if payload.Size > 0 && len(payload.Commits) > 0 {
				firstLine := strings.Split(payload.Commits[0].Message, "\n")[0]
				msg = fmt.Sprintf("Pushed %d commits (e.g. %q)", payload.Size, firstLine)
			}
			event.Description = msg

		case "PullRequestEvent":
			event.Category = "PR"
			var payload struct {
				Action      string `json:"action"`
				PullRequest struct {
					Number int    `json:"number"`
					Title  string `json:"title"`
				} `json:"pull_request"`
			}
			json.Unmarshal(e.Payload, &payload)
			event.Description = fmt.Sprintf("%s PR #%d %q", strings.Title(payload.Action), payload.PullRequest.Number, payload.PullRequest.Title)

		case "IssuesEvent":
			event.Category = "ISSUE"
			var payload struct {
				Action string `json:"action"`
				Issue  struct {
					Number int    `json:"number"`
					Title  string `json:"title"`
				} `json:"issue"`
			}
			json.Unmarshal(e.Payload, &payload)
			event.Description = fmt.Sprintf("%s issue #%d %q", strings.Title(payload.Action), payload.Issue.Number, payload.Issue.Title)

		case "IssueCommentEvent":
			event.Category = "ISSUE"
			var payload struct {
				Issue struct {
					Number int `json:"number"`
				} `json:"issue"`
			}
			json.Unmarshal(e.Payload, &payload)
			event.Description = fmt.Sprintf("Commented on issue #%d", payload.Issue.Number)

		case "PullRequestReviewEvent":
			event.Category = "PR"
			var payload struct {
				Review struct {
					State string `json:"state"`
				} `json:"review"`
				PullRequest struct {
					Number int `json:"number"`
				} `json:"pull_request"`
			}
			json.Unmarshal(e.Payload, &payload)
			event.Description = fmt.Sprintf("Submitted %s review on PR #%d", payload.Review.State, payload.PullRequest.Number)

		default:
			// Ignore other noise for the feed
			continue
		}

		feed = append(feed, event)
	}

	return feed
}
