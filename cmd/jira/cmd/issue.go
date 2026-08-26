package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
	jira "github.com/andygrunwald/go-jira"
)

const (
	defaultLabel    = "Support"
	defaultType     = "Task"
	defaultTemplate = "chore"
)

var issueCmd = &cobra.Command{
	Use:   "issue",
	Short: "Issue actions",
}

var issueCreateCmd = &cobra.Command{
	Use:   "create <summary>",
	Short: "Create a new Jira issue",
	Args:  cobra.ExactArgs(1),
	RunE:  runIssueCreate,
}

func init() {
	f := issueCreateCmd.Flags()
	f.StringSliceP("label", "l", nil, "issue label (repeatable)")
	f.StringP("component", "C", "", "component name")
	f.StringP("parent", "P", "", "parent issue key (creates a sub-task)")
	f.StringP("project", "p", "", "project key (defaults to DEFAULT_PROJECT from configuration)")
	f.StringP(
		"assignee",
		"a",
		"",
		"assignee (accountId, email, or legacy name; defaults to DEFAULT_USER for DEFAULT_PROJECT)",
	)
	f.String("template", defaultTemplate, "template name under $DOTFILES/templates/work/")
	f.String("story-points", "1", "story point estimate (MRI only)")
	f.String("story-points-dev", "1", "story points desarrollo (MRI only)")
	f.String("type", defaultType, "issue type name")

	issueCmd.AddCommand(issueCreateCmd)
}

func runIssueCreate(cmd *cobra.Command, args []string) error {
	summary := args[0]

	project, _ := cmd.Flags().GetString("project")
	if !cmd.Flags().Changed("project") {
		project = cfg.DefaultProject
	}

	component, _ := cmd.Flags().GetString("component")
	parent, _ := cmd.Flags().GetString("parent")

	assignee, _ := cmd.Flags().GetString("assignee")
	if !cmd.Flags().Changed("assignee") {
		if project == cfg.DefaultProject {
			assignee = cfg.DefaultUser
		} else {
			assignee = ""
		}
	}

	issueType, _ := cmd.Flags().GetString("type")
	templateName, _ := cmd.Flags().GetString("template")
	storyPoints, _ := cmd.Flags().GetString("story-points")
	storyPointsDev, _ := cmd.Flags().GetString("story-points-dev")
	labels, _ := cmd.Flags().GetStringSlice("label")

	labels = dedupeLabels(labels)
	if len(labels) == 0 {
		labels = []string{defaultLabel}
	}

	description, err := readTemplate(templateName)
	if err != nil {
		return err
	}

	fields := &jira.IssueFields{
		Project:     jira.Project{Key: project},
		Summary:     summary,
		Description: description,
		Type:        jira.IssueType{Name: issueType},
		Labels:      labels,
	}

	if component != "" {
		fields.Components = []*jira.Component{{Name: component}}
	}
	if parent != "" {
		fields.Parent = &jira.Parent{Key: parent}
	}
	if assignee != "" {
		user, err := jiraclient.ResolveAssignee(assignee)
		if err != nil {
			return err
		}
		fields.Assignee = user
	}

	if project == "MRI" {
		fields.Unknowns, err = mriStoryPoints(storyPoints, storyPointsDev)
		if err != nil {
			return err
		}
	}

	created, _, err := client.Issue.Create(&jira.Issue{Fields: fields})
	if err != nil {
		return err
	}
	fmt.Printf("Created %s: %s\n", created.Key, created.Fields.Summary)
	return nil
}

func readTemplate(name string) (string, error) {
	filename := templateFilename(name)
	dotfiles := os.Getenv("DOTFILES")
	if dotfiles == "" {
		home, _ := os.UserHomeDir()
		dotfiles = filepath.Join(home, "dotfiles")
	}
	path := filepath.Join(dotfiles, "templates", "work", filename)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("reading template %s: %w", path, err)
	}
	return string(b), nil
}

// templateFilename mirrors shortcuts::jira::templates: every name resolves to
// chore.md. Kept as a switch so extra templates can be wired later.
func templateFilename(name string) string {
	switch name {
	case "chore":
		return "chore.md"
	default:
		return "chore.md"
	}
}

func mriStoryPoints(estimate, dev string) (map[string]interface{}, error) {
	estID, err := resolveCustomFieldID("story-point-estimate")
	if err != nil {
		return nil, err
	}
	devID, err := resolveCustomFieldID("story-points-desarrollo")
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		estID: estimate,
		devID: dev,
	}, nil
}

func dedupeLabels(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, l := range in {
		if _, ok := seen[l]; ok {
			continue
		}
		seen[l] = struct{}{}
		out = append(out, l)
	}
	return out
}

func resolveCustomFieldID(name string) (string, error) {
	fields, _, err := client.Field.GetList()
	if err != nil {
		return "", fmt.Errorf("listing custom fields: %w", err)
	}

	normalized := normalizeFieldName(name)
	for _, f := range fields {
		if normalizeFieldName(f.Name) == normalized || f.Key == name || f.ID == name {
			return f.ID, nil
		}
	}

	var candidates []string
	for _, f := range fields {
		if f.Custom && strings.Contains(strings.ToLower(f.Name), "story") {
			candidates = append(candidates, fmt.Sprintf("%s (%s)", f.Name, f.ID))
		}
	}
	return "", fmt.Errorf(
		"custom field %q not found; story-related candidates: %s",
		name,
		strings.Join(candidates, ", "),
	)
}

func normalizeFieldName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	return strings.NewReplacer("-", " ", "_", " ", ".", " ").Replace(s)
}

func toUser(ref string) *jira.User {
	switch {
	case strings.Contains(ref, "@"):
		return &jira.User{EmailAddress: ref}
	case isAccountID(ref):
		return &jira.User{AccountID: ref}
	default:
		return &jira.User{Name: ref}
	}
}

func isAccountID(ref string) bool {
	if ref == "" {
		return false
	}
	for _, r := range ref {
		if !(unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_' || r == ':') {
			return false
		}
	}
	return true
}
