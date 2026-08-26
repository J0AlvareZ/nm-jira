package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
	jira "github.com/andygrunwald/go-jira"
)

var workloadCmd = &cobra.Command{
	Use:   "workload <issue> <duration>",
	Short: "Add a worklog entry to an issue",
	Args:  cobra.ExactArgs(2),
	RunE:  runWorkload,
}

func init() {
	workloadCmd.Flags().String("comment", "", "optional worklog comment")
}

func runWorkload(cmd *cobra.Command, args []string) error {
	issueKey := args[0]
	rawDuration := args[1]
	comment, _ := cmd.Flags().GetString("comment")

	normalized, err := jiraclient.NormalizeDuration(rawDuration)
	if err != nil {
		return fmt.Errorf("invalid duration %q: use formats like '2h', '30m', '2h30m', or '2h 30m': %w", rawDuration, err)
	}

	record := &jira.WorklogRecord{TimeSpent: normalized}
	if comment != "" {
		record.Comment = comment
	}

	if _, _, err := client.Issue.AddWorklogRecord(issueKey, record); err != nil {
		return err
	}
	fmt.Printf("Logged %s on %s\n", normalized, issueKey)
	return nil
}
