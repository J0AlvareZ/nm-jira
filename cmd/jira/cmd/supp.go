package cmd

import "github.com/spf13/cobra"

var suppCmd = &cobra.Command{
	Use:   "supp",
	Short: "List approved Support issues with your worklog",
	RunE:  runSupp,
}

func runSupp(cmd *cobra.Command, args []string) error {
	jql := "worklogAuthor = currentUser() AND component = Support AND status = Approved"
	return printPlainIssues(jql)
}
