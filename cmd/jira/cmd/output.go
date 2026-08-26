package cmd

import (
	"fmt"

	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
	jira "github.com/andygrunwald/go-jira"
)

// printPlainIssues prints a flat "KEY\tSUMMARY" list, mirroring the zsh
// go-jira CLI's --plain output.
func printPlainIssues(jql string) error {
	issues, err := jiraclient.SearchJQL(jql, 100)
	if err != nil {
		return err
	}
	for _, issue := range issues {
		fmt.Printf("%s\t%s\n", issue.Key, summaryOf(issue))
	}
	return nil
}

func summaryOf(issue jira.Issue) string {
	if issue.Fields == nil {
		return ""
	}
	return issue.Fields.Summary
}

func statusOf(issue jira.Issue) string {
	if issue.Fields == nil || issue.Fields.Status == nil {
		return ""
	}
	return issue.Fields.Status.Name
}

func priorityOf(issue jira.Issue) string {
	if issue.Fields == nil || issue.Fields.Priority == nil {
		return ""
	}
	return issue.Fields.Priority.Name
}
