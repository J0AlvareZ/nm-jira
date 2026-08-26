package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	jira "github.com/andygrunwald/go-jira"
)

const descriptionMarker = "--DESCRIPTION--"

var editCmd = &cobra.Command{
	Use:   "edit <issue>",
	Short: "Open an issue in $EDITOR and update the edited fields",
	Args:  cobra.ExactArgs(1),
	RunE:  runEdit,
}

func runEdit(cmd *cobra.Command, args []string) error {
	issueKey := args[0]

	issue, _, err := client.Issue.Get(issueKey, nil)
	if err != nil {
		return fmt.Errorf("getting issue %s: %w", issueKey, err)
	}

	edited, err := openInEditor(issueToEditor(issue))
	if err != nil {
		return err
	}

	fields, err := parseEditor(edited)
	if err != nil {
		return err
	}

	update := &jira.Issue{
		Key:    issue.Key,
		ID:     issue.ID,
		Fields: fields,
	}

	if _, _, err := client.Issue.Update(update); err != nil {
		return err
	}
	fmt.Printf("Updated %s\n", issueKey)
	return nil
}

func issueToEditor(issue *jira.Issue) string {
	f := issue.Fields
	var b strings.Builder

	fmt.Fprintf(&b, "# Editing %s\n", issue.Key)
	b.WriteString("# Lines starting with '#' are ignored.\n")
	b.WriteString("# Edit the value after the colon on the single-line fields below.\n")

	fmt.Fprintf(&b, "summary: %s\n", f.Summary)

	priority := ""
	if f.Priority != nil {
		priority = f.Priority.Name
	}
	fmt.Fprintf(&b, "priority: %s\n", priority)

	assignee := ""
	if f.Assignee != nil {
		assignee = f.Assignee.DisplayName
	}
	fmt.Fprintf(&b, "assignee: %s\n", assignee)

	fmt.Fprintf(&b, "labels: %s\n", strings.Join(f.Labels, ", "))

	b.WriteString(descriptionMarker + "\n")
	b.WriteString(f.Description)
	b.WriteString("\n")
	return b.String()
}

// parseEditor parses the key/value header and the raw description body after
// the --DESCRIPTION-- marker.
func parseEditor(text string) (*jira.IssueFields, error) {
	lines := strings.Split(text, "\n")

	var summary, priority, assignee, labelsStr string
	descStart := -1

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == descriptionMarker {
			descStart = i
			break
		}
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, val, ok := strings.Cut(line, ":")
		if !ok {
			return nil, fmt.Errorf("invalid editor line: %q", line)
		}
		switch strings.TrimSpace(key) {
		case "summary":
			summary = strings.TrimSpace(val)
		case "priority":
			priority = strings.TrimSpace(val)
		case "assignee":
			assignee = strings.TrimSpace(val)
		case "labels":
			labelsStr = strings.TrimSpace(val)
		}
	}

	if descStart < 0 {
		return nil, fmt.Errorf("missing %s marker", descriptionMarker)
	}
	description := strings.TrimRight(strings.Join(lines[descStart+1:], "\n"), "\n")

	fields := &jira.IssueFields{Description: description}

	if summary != "" {
		fields.Summary = summary
	}
	if priority != "" {
		fields.Priority = &jira.Priority{Name: priority}
	}
	if assignee != "" {
		fields.Assignee = toUser(assignee)
	}
	if labelsStr != "" {
		var labels []string
		for _, l := range strings.Split(labelsStr, ",") {
			l = strings.TrimSpace(l)
			if l != "" {
				labels = append(labels, l)
			}
		}
		fields.Labels = labels
	}

	return fields, nil
}
