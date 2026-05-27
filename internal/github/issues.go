package github

import "time"

type IssueLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type Issue struct {
	Number      int          `json:"number"`
	Title       string       `json:"title"`
	State       string       `json:"state"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
	Comments    int          `json:"comments"`
	PullRequest *struct{}    `json:"pull_request,omitempty"`
	Labels      []IssueLabel `json:"labels"`
	User        User         `json:"user"`
}

func (c *Client) GetIssues(owner, repo string, state string) ([]Issue, error) {
	var issues []Issue
	url := "https://api.github.com/repos/" + owner + "/" + repo + "/issues?state=" + state + "&per_page=100"
	err := c.get(url, &issues)
	return issues, err
}
