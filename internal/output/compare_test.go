package output

import (
	"strings"
	"testing"
	"time"

	"github.com/agnivo988/Repo-lyzer/internal/github"
)

func TestBuildCompareReport(t *testing.T) {
	report := BuildCompareReport(
		CompareInput{
			Repo: &github.Repo{
				FullName:      "owner/alpha",
				Description:   "Alpha repo",
				Stars:         120,
				Forks:         18,
				OpenIssues:    7,
				Language:      "Go",
				DefaultBranch: "main",
				PushedAt:      time.Now().Add(-24 * time.Hour),
			},
			Commits: []github.Commit{{}, {}, {}},
			Contributors: []github.Contributor{{}, {}},
			Languages: map[string]int{"Go": 1000, "Shell": 100},
		},
		CompareInput{
			Repo: &github.Repo{
				FullName:      "owner/beta",
				Description:   "Beta repo",
				Stars:         80,
				Forks:         12,
				OpenIssues:    10,
				Language:      "TypeScript",
				DefaultBranch: "main",
				PushedAt:      time.Now().Add(-48 * time.Hour),
			},
			Commits: []github.Commit{{}, {}},
			Contributors: []github.Contributor{{}},
			Languages: map[string]int{"TypeScript": 900, "CSS": 50},
		},
	)

	if report.Repo1.FullName != "owner/alpha" || report.Repo2.FullName != "owner/beta" {
		t.Fatalf("unexpected repo names: %#v", report)
	}

	if report.Verdict == "" {
		t.Fatal("expected a non-empty verdict")
	}

	if report.Repo1.HealthScore == 0 || report.Repo2.HealthScore == 0 {
		t.Fatal("expected health scores to be computed")
	}
}

func TestRenderCompareOutputs(t *testing.T) {
	report := BuildCompareReport(
		CompareInput{
			Repo: &github.Repo{FullName: "owner/alpha", Stars: 10, Forks: 4, OpenIssues: 1, Language: "Go", DefaultBranch: "main"},
			Commits: []github.Commit{{}},
			Contributors: []github.Contributor{{}},
			Languages: map[string]int{"Go": 1},
		},
		CompareInput{
			Repo: &github.Repo{FullName: "owner/beta", Stars: 20, Forks: 6, OpenIssues: 2, Language: "Rust", DefaultBranch: "main"},
			Commits: []github.Commit{{}, {}},
			Contributors: []github.Contributor{{}, {}},
			Languages: map[string]int{"Rust": 2},
		},
	)

	terminal := RenderCompareTerminal(report)
	if !strings.Contains(terminal, "Repository Comparison") {
		t.Fatalf("terminal output missing header: %s", terminal)
	}

	jsonData, err := RenderCompareJSON(report)
	if err != nil {
		t.Fatalf("RenderCompareJSON failed: %v", err)
	}
	if !strings.Contains(string(jsonData), "owner/alpha") {
		t.Fatalf("JSON output missing repo name: %s", string(jsonData))
	}

	markdown, err := RenderCompareMarkdown(report)
	if err != nil {
		t.Fatalf("RenderCompareMarkdown failed: %v", err)
	}
	if !strings.Contains(string(markdown), "# Repository Comparison") {
		t.Fatalf("markdown output missing title: %s", string(markdown))
	}

	htmlData, err := RenderCompareHTML(report)
	if err != nil {
		t.Fatalf("RenderCompareHTML failed: %v", err)
	}
	if !strings.Contains(string(htmlData), "Repo-lyzer comparison report") {
		t.Fatalf("html output missing title: %s", string(htmlData))
	}
}