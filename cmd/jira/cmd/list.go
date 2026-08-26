package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues assigned to the current user",
	RunE:  runList,
}

func init() {
	listCmd.Flags().StringP("status", "s", "", "filter by status")
	listCmd.Flags().StringP("label", "l", "", "filter by label")
}

func runList(cmd *cobra.Command, args []string) error {
	status, _ := cmd.Flags().GetString("status")
	label, _ := cmd.Flags().GetString("label")

	jql := "assignee = currentUser()"
	if status != "" {
		jql += fmt.Sprintf(" AND status = %q", status)
	}
	if label != "" {
		jql += fmt.Sprintf(" AND labels = %q", label)
	}

	issues, err := jiraclient.SearchJQL(jql, 100)
	if err != nil {
		return err
	}

	fmt.Printf("%-14s  %-14s  %-20s  %s\n", "KEY", "STATUS", "PRIORITY", "SUMMARY")
	for _, issue := range issues {
		fmt.Printf("%-14s  %-14s  %-20s  %s\n",
			issue.Key, statusOf(issue), priorityOf(issue), summaryOf(issue))
	}
	fmt.Printf("\n%d issue(s)\n", len(issues))
	return nil
}
