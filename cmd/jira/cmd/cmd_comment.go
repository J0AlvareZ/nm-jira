package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	jira "github.com/andygrunwald/go-jira"
)

var commentCmd = &cobra.Command{
	Use:   "comment <issue> [text]",
	Short: "Add a comment to an issue (opens $EDITOR if text is omitted)",
	Args:  cobra.RangeArgs(1, 2),
	RunE:  runComment,
}

func runComment(cmd *cobra.Command, args []string) error {
	issueKey, err := resolveIssueKey(args[0], cfg.DefaultProject)
	if err != nil {
		return err
	}

	var body string
	if len(args) > 1 {
		body = args[1]
	} else {
		var err error
		body, err = openInEditor(cfg.Editor, "")
		if err != nil {
			return err
		}
	}

	body = strings.TrimSpace(body)
	if body == "" {
		return fmt.Errorf("comment cannot be empty")
	}

	if _, _, err := client.Issue.AddComment(issueKey, &jira.Comment{Body: body}); err != nil {
		return err
	}
	fmt.Printf("Commented on %s\n", issueKey)
	return nil
}
