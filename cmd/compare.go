// Package cmd provides command-line interface commands for the Repo-lyzer application.
// It includes commands for analyzing repositories, comparing repositories, and running the interactive menu.
package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/agnivo988/Repo-lyzer/internal/github"
	"github.com/agnivo988/Repo-lyzer/internal/output"
	"github.com/agnivo988/Repo-lyzer/internal/progress"
	"github.com/spf13/cobra"
)

// RunCompare executes the compare command for two GitHub repositories.
// It takes two repository identifiers in owner/repo format, analyzes both repositories,
// and displays a comparison table with metrics like stars, forks, commits, contributors,
// bus factor, and maturity scores.
// Parameters:
//   - r1: First repository in owner/repo format
//   - r2: Second repository in owner/repo format
//
// Returns an error if the comparison fails.
func RunCompare(r1, r2 string) error {
	compareCmd.SetArgs([]string{r1, r2})
	return compareCmd.Execute()
}

var compareCmd = &cobra.Command{
	Use:   "compare owner1/repo1 owner2/repo2",
	Short: "Compare two GitHub repositories side-by-side",
	Long: `Compare two GitHub repositories and display a side-by-side comparison
of their key metrics and health indicators.

Comparison includes:
	• Stars, Forks, and Open Issues
	• Commit activity (past year)
	• Contributor count and engagement
	• Bus Factor and risk assessment  
	• Repository maturity scores
	• Verdict on which repository is more mature/stable

Examples:
	# Compare popular frameworks
	repo-lyzer compare facebook/react vuejs/vue

	# Compare similar tools
	repo-lyzer compare golang/go rust-lang/rust

	# Compare forks
	repo-lyzer compare original/repo fork/repo

	# Export as HTML
	repo-lyzer compare golang/go microsoft/vscode --format html --save report.html

	# Export as JSON
	repo-lyzer compare golang/go facebook/react --format json --save compare.json

	# Export as Markdown to stdout
	repo-lyzer compare golang/go rust-lang/rust --format markdown`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		format, _ := cmd.Flags().GetString("format")
		savePath, _ := cmd.Flags().GetString("save")
		format = strings.ToLower(strings.TrimSpace(format))
		if format == "" {
			format = "terminal"
		}

		// Parse repo names
		r1 := strings.Split(args[0], "/")
		r2 := strings.Split(args[1], "/")

		if len(r1) != 2 || len(r2) != 2 {
			return fmt.Errorf("repositories must be in owner/repo format")
		}

		client := github.NewClient()
		spinner := progress.NewSpinner()

		spinner.Start(fmt.Sprintf("🔍 Analyzing %s/%s...", r1[0], r1[1]))
		side1, err := fetchCompareInput(client, r1[0], r1[1])
		if err != nil {
			spinner.Stop()
			return err
		}
		spinner.StopWithMessage(fmt.Sprintf("Analyzed %s/%s", r1[0], r1[1]))

		spinner.Start(fmt.Sprintf("🔍 Analyzing %s/%s...", r2[0], r2[1]))
		side2, err := fetchCompareInput(client, r2[0], r2[1])
		if err != nil {
			spinner.Stop()
			return err
		}
		spinner.StopWithMessage(fmt.Sprintf("Analyzed %s/%s", r2[0], r2[1]))

		report := output.BuildCompareReport(side1, side2)

		var rendered []byte
		switch format {
		case "terminal":
			terminalOutput := output.RenderCompareTerminal(report)
			fmt.Print(terminalOutput)
			if savePath != "" {
				return saveCompareOutput(savePath, []byte(terminalOutput))
			}
			return nil
		case "json":
			rendered, err = output.RenderCompareJSON(report)
		case "markdown":
			rendered, err = output.RenderCompareMarkdown(report)
		case "html":
			rendered, err = output.RenderCompareHTML(report)
		default:
			return fmt.Errorf("unsupported compare format: %s", format)
		}
		if err != nil {
			return err
		}

		if savePath != "" {
			if err := saveCompareOutput(savePath, rendered); err != nil {
				return err
			}
			fmt.Printf("Saved comparison report to %s\n", savePath)
			return nil
		}

		fmt.Print(string(rendered))
		return nil
	},
}

func fetchCompareInput(client *github.Client, owner, repo string) (output.CompareInput, error) {
	repoInfo, err := client.GetRepo(owner, repo)
	if err != nil {
		return output.CompareInput{}, err
	}

	languages, _ := client.GetLanguages(owner, repo)
	commits, _ := client.GetCommits(owner, repo, 365)
	contributors, err := client.GetContributorsWithAvatars(owner, repo, 15)
	if err != nil {
		return output.CompareInput{}, fmt.Errorf("error fetching contributors for %s/%s: %w", owner, repo, err)
	}
	_, _ = client.GetFileTree(owner, repo, repoInfo.DefaultBranch)

	return output.CompareInput{
		Repo:         repoInfo,
		Commits:      commits,
		Contributors: contributors,
		Languages:    languages,
	}, nil
}

func saveCompareOutput(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, data, 0644)
}

func init() {
	rootCmd.AddCommand(compareCmd)
	compareCmd.Flags().String("format", "terminal", "Output format: terminal, html, json, markdown")
	compareCmd.Flags().String("save", "", "Write the comparison output to a file")
}
