package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
	jira "github.com/andygrunwald/go-jira"
)

const (
	defaultLabel = "Support"
	defaultType  = "Task"
)

type editTemplateFunc func(initial string) (string, error)

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
	f.String("template", "", "template name under the user config templates directory")
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
	storyPoints, _ := cmd.Flags().GetString("story-points")
	storyPointsDev, _ := cmd.Flags().GetString("story-points-dev")
	labels, _ := cmd.Flags().GetStringSlice("label")

	labels = dedupeLabels(labels)
	if len(labels) == 0 {
		labels = []string{defaultLabel}
	}

	description, err := resolveIssueDescription(cmd, config.ConfigDir, func(initial string) (string, error) {
		return openInEditor(cfg.Editor, initial)
	})
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
		estimate, err := parseStoryPoints("--story-points", storyPoints)
		if err != nil {
			return err
		}
		dev, err := parseStoryPoints("--story-points-dev", storyPointsDev)
		if err != nil {
			return err
		}
		fields.Unknowns, err = mriStoryPoints(estimate, dev)
		if err != nil {
			return err
		}
	}

	created, resp, err := client.Issue.Create(&jira.Issue{Fields: fields})
	if err != nil {
		return createIssueError(resp, err)
	}

	if created == nil {
		return fmt.Errorf("creating issue: Jira returned an empty response")
	}

	if created.Key == "" {
		return fmt.Errorf("creating issue: Jira response did not include an issue key")
	}

	fmt.Printf("Created %s: %s\n", created.Key, created.Fields.Summary)
	return nil
}

func validateTemplateName(name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("template name must not be empty")
	}
	if strings.ContainsAny(name, `/\\`) {
		return fmt.Errorf("template name %q must not contain path separators", name)
	}
	if strings.EqualFold(filepath.Ext(name), ".md") {
		return fmt.Errorf("template name %q must not include the .md extension", name)
	}
	return nil
}

func templateDirectory(configDir string) string {
	return filepath.Join(configDir, "templates")
}

func availableTemplateNames(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func resolveTemplateDescription(name, templatesDir string, edit editTemplateFunc) (string, error) {
	if err := validateTemplateName(name); err != nil {
		return "", err
	}

	available, err := availableTemplateNames(templatesDir)
	if err != nil {
		return "", fmt.Errorf(
			"template %q is unavailable: cannot read templates directory %q; available templates: none: %w",
			name,
			templatesDir,
			err,
		)
	}
	if len(available) == 0 {
		return "", fmt.Errorf(
			"template %q is unavailable: templates directory %q has no .md templates; available templates: none",
			name,
			templatesDir,
		)
	}

	path := filepath.Join(templatesDir, name+".md")
	contents, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf(
			"template %q not found at %q; available templates: %s",
			name,
			path,
			strings.Join(available, ", "),
		)
	}

	return edit(string(contents))
}

func resolveIssueDescription(
	cmd *cobra.Command,
	configDir func() (string, error),
	edit editTemplateFunc,
) (string, error) {
	if !cmd.Flags().Changed("template") {
		return "", nil
	}

	name, _ := cmd.Flags().GetString("template")
	dir, err := configDir()
	if err != nil {
		return "", err
	}

	return resolveTemplateDescription(name, templateDirectory(dir), edit)
}

func parseStoryPoints(flag, value string) (int, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return 0, fmt.Errorf("%s must be a non-negative integer (0, 1, 2, ...); got empty value", flag)
	}
	n, err := strconv.Atoi(trimmed)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a non-negative integer (0, 1, 2, ...); got %q", flag, value)
	}
	return n, nil
}

func mriStoryPoints(estimate, dev int) (map[string]interface{}, error) {
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

// createIssueError builds a safe creation error message from Jira's structured
// error response. It extracts only the "errorMessages" and "errors" fields and
// never exposes the raw response body, tokens, Authorization, payload, summary,
// or accountId. When the response is missing or not structured JSON, it falls
// back to the original error, which only contains the HTTP status code.
func createIssueError(resp *jira.Response, err error) error {
	if resp == nil || resp.Body == nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	body, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	var structured struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if json.Unmarshal(body, &structured) != nil {
		return fmt.Errorf("creating issue: %w", err)
	}

	details := make([]string, 0, len(structured.ErrorMessages)+len(structured.Errors))
	details = append(details, structured.ErrorMessages...)
	keys := make([]string, 0, len(structured.Errors))
	for k := range structured.Errors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		details = append(details, fmt.Sprintf("%s: %s", k, structured.Errors[k]))
	}

	if len(details) == 0 {
		return fmt.Errorf("creating issue: %w", err)
	}
	return fmt.Errorf("creating issue (status %d): %s", resp.StatusCode, strings.Join(details, "; "))
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
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' && r != ':' {
			return false
		}
	}
	return true
}
