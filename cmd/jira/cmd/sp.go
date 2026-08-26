package cmd

import "github.com/spf13/cobra"

var spCmd = &cobra.Command{
	Use:   "sp",
	Short: "List issues in open sprints with your worklog",
	RunE:  runSp,
}

func runSp(cmd *cobra.Command, args []string) error {
	jql := "worklogAuthor = currentUser() AND Sprint in openSprints()"
	return printPlainIssues(jql)
}
