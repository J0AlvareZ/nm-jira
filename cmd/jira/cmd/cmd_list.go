package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
)

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List issues assigned to the current user",
	RunE:  runList,
}

var listSprintCmd = &cobra.Command{
	Use:   "sprint",
	Short: "List assigned issues in open sprints",
	Args:  cobra.NoArgs,
	RunE:  runListSprint,
}

func init() {
	listCmd.Flags().StringP("status", "s", "", "filter by status")
	listCmd.Flags().StringP("label", "l", "", "filter by label")
	listCmd.Flags().StringP("epic", "e", "", "filter by epic key")
	listCmd.AddCommand(listSprintCmd)
	listSprintCmd.Flags().String(
		"name",
		"",
		"Sprint name",
	)
	listCmd.PersistentFlags().StringP(
		"project",
		"p",
		"",
		"Jira project key",
	)
	listCmd.PersistentFlags().StringP(
		"assignee",
		"a",
		"",
		"Jira assignee",
	)
}

func runList(cmd *cobra.Command, args []string) error {
	status, _ := cmd.Flags().GetString("status")
	label, _ := cmd.Flags().GetString("label")
	epic, _ := cmd.Flags().GetString("epic")

	jql := "assignee = currentUser()"
	if status != "" {
		jql += fmt.Sprintf(" AND status = %q", status)
	}
	if label != "" {
		jql += fmt.Sprintf(" AND labels = %q", label)
	}
	if epic != "" {
		epic, err := resolveIssueKey(epic, cfg.DefaultProject)
		if err != nil {
			return err
		}
		jql += fmt.Sprintf(" AND parent = %q", epic)
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

func runListSprint(cmd *cobra.Command, args []string) error {
	project, err := cmd.Flags().GetString("project")
	if err != nil {
		return err
	}

	assignee, err := cmd.Flags().GetString("assignee")
	if err != nil {
		return err
	}

	sprint, err := cmd.Flags().GetString("name")
	if err != nil {
		return err
	}

	project = strings.TrimSpace(project)
	if project == "" {
		project = strings.TrimSpace(cfg.DefaultProject)
	}
	if project == "" {
		return fmt.Errorf(
			"project is required: use --project or configure DEFAULT_PROJECT",
		)
	}

	assignee = strings.TrimSpace(assignee)
	if assignee == "" {
		assignee = strings.TrimSpace(cfg.DefaultUser)
	}
	if assignee == "" {
		return fmt.Errorf(
			"assignee is required: use --assignee or configure DEFAULT_USER",
		)
	}

	user, err := jiraclient.ResolveAssignee(assignee)
	if err != nil {
		return fmt.Errorf("resolving assignee %q: %w", assignee, err)
	}

	if user == nil || user.AccountID == "" {
		return fmt.Errorf(
			"could not resolve assignee %q to an account ID",
			assignee,
		)
	}

	sprint = strings.TrimSpace(sprint)

	sprintClause := "Sprint in openSprints()"
	if sprint != "" {
		sprintClause = fmt.Sprintf(
			"Sprint = %s",
			quoteJQLString(sprint),
		)
	}

	jql := fmt.Sprintf(
		`project = %q AND assignee = %q AND %s`,
		project,
		user.AccountID,
		sprintClause,
	)

	return printPlainIssues(jql)
}

func quoteJQLString(value string) string {
	return `"` + strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
	).Replace(value) + `"`
}
