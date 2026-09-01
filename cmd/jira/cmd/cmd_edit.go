package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	issueKey, err := resolveIssueKey(args[0], cfg.DefaultProject)
	if err != nil {
		return err
	}

	issue, _, err := client.Issue.Get(issueKey, nil)
	if err != nil {
		return fmt.Errorf("getting issue %s: %w", issueKey, err)
	}

	var storyPointEstimateID, storyPointsDevelopmentID string
	if issue.Fields.Project.Key == "MRI" {
		storyPointEstimateID, err = resolveCustomFieldID("story-point-estimate")
		if err != nil {
			return fmt.Errorf("resolving story-point-estimate custom field: %w", err)
		}

		storyPointsDevelopmentID, err = resolveCustomFieldID("story-points-desarrollo")
		if err != nil {
			return fmt.Errorf("resolving story-points-desarrollo custom field: %w", err)
		}
	}

	// edited, err := openInEditor(cfg.Editor, issueToEditor(issue))
	edited, err := openInEditor(
		cfg.Editor,
		issueToEditor(issue, storyPointEstimateID, storyPointsDevelopmentID),
	)
	if err != nil {
		return err
	}

	fields, err := parseEditor(
		edited,
		issue.Fields.Project.Key,
	)
	if err != nil {
		return err
	}

	if issue.Fields.Assignee != nil {
		fields.Assignee = issue.Fields.Assignee
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

func issueToEditor(
	issue *jira.Issue,
	storyPointEstimateID, storyPointsDevelopmentID string,
) string {
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
		switch {
		case f.Assignee.DisplayName != "":
			assignee = f.Assignee.DisplayName
		case f.Assignee.AccountID != "":
			assignee = f.Assignee.AccountID
		case f.Assignee.Name != "":
			assignee = f.Assignee.Name

		}
	}
	fmt.Fprintf(&b, "assignee: %s\n", assignee)

	fmt.Fprintf(&b, "labels: %s\n", strings.Join(f.Labels, ", "))

	if f.Project.Key == "MRI" {
		fmt.Fprintf(
			&b,
			"story-point-estimate: %s\n",
			customFieldValueForEditor(issue, storyPointEstimateID),
		)
		fmt.Fprintf(
			&b,
			"story-points-desarrollo: %s\n",
			customFieldValueForEditor(issue, storyPointsDevelopmentID),
		)
	}

	b.WriteString(descriptionMarker + "\n")
	b.WriteString(f.Description)
	b.WriteString("\n")
	return b.String()
}

func parseEditor(text, project string) (*jira.IssueFields, error) {
	lines := strings.Split(text, "\n")

	storyPointEstimate := ""
	storyPointEstimateSet := false

	storyPointsDevelopment := ""
	storyPointsDevelopmentSet := false

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
		case "story-point-estimate":
			storyPointEstimate = strings.TrimSpace(val)
			storyPointEstimateSet = true
		case "story-points-desarrollo":
			storyPointsDevelopment = strings.TrimSpace(val)
			storyPointsDevelopmentSet = true
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

	if storyPointEstimateSet || storyPointsDevelopmentSet {
		if project != "MRI" {
			return nil, fmt.Errorf(
				"story points are only supported for project MRI",
			)
		}

		unknowns := make(map[string]interface{})

		if storyPointEstimate != "" {
			value, err := parseStoryPoints(
				"story-point-estimate",
				storyPointEstimate,
			)
			if err != nil {
				return nil, err
			}

			fieldID, err := resolveCustomFieldID("story-point-estimate")
			if err != nil {
				return nil, err
			}

			unknowns[fieldID] = value
		}

		if storyPointsDevelopment != "" {
			value, err := parseStoryPoints(
				"story-points-desarrollo",
				storyPointsDevelopment,
			)
			if err != nil {
				return nil, err
			}

			fieldID, err := resolveCustomFieldID("story-points-desarrollo")
			if err != nil {
				return nil, err
			}

			unknowns[fieldID] = value
		}

		if len(unknowns) > 0 {
			fields.Unknowns = unknowns
		}
	}

	return fields, nil
}

func customFieldValueForEditor(issue *jira.Issue, fieldID string) string {
	if issue == nil || fieldID == "" {
		return ""
	}

	value, ok := issue.Fields.Unknowns[fieldID]
	if !ok || value == nil {
		return ""
	}

	var raw string
	switch value := value.(type) {
	case float64:
		raw = strconv.FormatFloat(value, 'f', -1, 64)
	case int:
		raw = strconv.Itoa(value)
	case int64:
		raw = strconv.FormatInt(value, 10)
	case json.Number:
		raw = value.String()
	case string:
		raw = value
	default:
		return ""
	}

	storyPoints, err := parseStoryPoints(fieldID, raw)
	if err != nil {
		return ""
	}

	return strconv.Itoa(storyPoints)
}
