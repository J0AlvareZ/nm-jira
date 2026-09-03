package cmd

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/table"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
)

type timesheet struct {
	Filter  string
	User    string
	Entries []jiraclient.WorklogEntry
	Errors  []error
	Verbose bool
}

var timesheetCmd = &cobra.Command{
	Use:   "timesheet [today|thisweek|lastweek]",
	Short: "Show your worklogs for a time range",
	Args:  cobra.MaximumNArgs(1),
	RunE:  runTimesheet,
}

func init() {
	timesheetCmd.Flags().Bool("verbose", false, "show details for partial errors")
}

func runTimesheet(cmd *cobra.Command, args []string) error {
	filter := "today"
	if len(args) == 1 {
		filter = args[0]
	}
	start, end, err := timesheetRange(filter, time.Now())
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.DefaultUser) == "" {
		return fmt.Errorf("DEFAULT_USER is required; configure it in config.toml")
	}
	user, err := jiraclient.ResolveAssignee(cfg.DefaultUser)
	if err != nil {
		return fmt.Errorf("resolving assignee %q: %w", cfg.DefaultUser, err)
	}
	if user == nil || user.AccountID == "" {
		return fmt.Errorf("could not resolve assignee %q to an account ID", cfg.DefaultUser)
	}

	verbose, _ := cmd.Flags().GetBool("verbose")
	retrieve := func() (timesheet, error) {
		result, err := jiraclient.SearchWorklogs(user.AccountID, start, end)
		if err != nil {
			return timesheet{}, fmt.Errorf("searching worklogs: %w", err)
		}
		return timesheet{Filter: filter, User: cfg.DefaultUser, Entries: result.Entries, Errors: result.Errors, Verbose: verbose}, nil
	}

	if isTTY(cmd.OutOrStdout()) {
		return runTimesheetTUI(cmd.OutOrStdout(), retrieve)
	}
	report, err := retrieve()
	if err != nil {
		return err
	}
	printTimesheetPlain(cmd.OutOrStdout(), cmd.ErrOrStderr(), report, verbose)
	return nil
}

func timesheetRange(filter string, now time.Time) (time.Time, time.Time, error) {
	localNow := now.In(time.Local)
	day := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	switch filter {
	case "today":
		return day, day.AddDate(0, 0, 1), nil
	case "thisweek", "lastweek":
		monday := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
		if filter == "lastweek" {
			monday = monday.AddDate(0, 0, -7)
		}
		return monday, monday.AddDate(0, 0, 7), nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("invalid timesheet filter %q; expected today, thisweek, or lastweek", filter)
	}
}

func isTTY(writer io.Writer) bool {
	file, ok := writer.(*os.File)
	return ok && term.IsTerminal(int(file.Fd()))
}

func sortedEntries(entries []jiraclient.WorklogEntry) []jiraclient.WorklogEntry {
	sorted := append([]jiraclient.WorklogEntry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Started.Equal(sorted[j].Started) {
			return sorted[i].IssueKey < sorted[j].IssueKey
		}
		return sorted[i].Started.Before(sorted[j].Started)
	})
	return sorted
}

func formatDuration(seconds int) string {
	return (time.Duration(seconds) * time.Second).String()
}

func totalsByDay(entries []jiraclient.WorklogEntry) (map[string]int, int) {
	totals := make(map[string]int)
	total := 0
	for _, entry := range entries {
		day := entry.Started.In(time.Local).Format("2006-01-02")
		totals[day] += entry.TimeSpentSeconds
		total += entry.TimeSpentSeconds
	}
	return totals, total
}

func printTimesheetPlain(out, errOut io.Writer, report timesheet, verbose bool) {
	entries := sortedEntries(report.Entries)
	totals, total := totalsByDay(entries)
	_, _ = fmt.Fprintf(out, "User: %s\nFilter: %s\n\n", report.User, report.Filter)
	_, _ = fmt.Fprintln(out, "DATE\tISSUE\tSUMMARY\tTIME")
	currentDay := ""
	for _, entry := range entries {
		day := entry.Started.In(time.Local).Format("2006-01-02")
		if currentDay != "" && day != currentDay {
			_, _ = fmt.Fprintf(out, "Total %s:\t\t\t%s\n", currentDay, formatDuration(totals[currentDay]))
		}
		currentDay = day
		_, _ = fmt.Fprintf(out, "%s\t%s\t%s\t%s\n", day, entry.IssueKey, entry.IssueSummary, formatDuration(entry.TimeSpentSeconds))
	}
	if currentDay != "" {
		_, _ = fmt.Fprintf(out, "Total %s:\t\t\t%s\n", currentDay, formatDuration(totals[currentDay]))
	} else {
		_, _ = fmt.Fprintln(out, "No worklogs found.")
	}
	_, _ = fmt.Fprintf(out, "\nTotal:\t\t\t%s\n", formatDuration(total))
	if len(report.Errors) > 0 {
		_, _ = fmt.Fprintf(errOut, "warning: %d partial error(s); showing available worklogs\n", len(report.Errors))
		if verbose {
			for _, err := range report.Errors {
				_, _ = fmt.Fprintf(errOut, "warning: %v\n", err)
			}
		}
	}
}

type timesheetLoadedMsg struct{ report timesheet; err error }

type timesheetModel struct {
	loading bool
	report  timesheet
	err     error
	table   table.Model
	width   int
	height  int
	load    tea.Cmd
}

func runTimesheetTUI(out io.Writer, retrieve func() (timesheet, error)) error {
	model := timesheetModel{loading: true, load: func() tea.Msg {
		report, err := retrieve()
		return timesheetLoadedMsg{report: report, err: err}
	}}
	program := tea.NewProgram(model, tea.WithOutput(out))
	_, err := program.Run()
	return err
}

func (m timesheetModel) Init() tea.Cmd { return m.load }

func (m timesheetModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyPressMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		if !m.loading && m.err == nil {
			m.resizeTable()
		}
	case timesheetLoadedMsg:
		m.loading, m.report, m.err = false, msg.report, msg.err
		if msg.err == nil {
			m.table = newTimesheetTable(msg.report.Entries, max(0, m.width-timesheetPanelFrameWidth()), 0)
			m.resizeTable()
		}
	}
	if !m.loading && m.err == nil {
		var cmd tea.Cmd
		m.table, cmd = m.table.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m timesheetModel) View() tea.View {
	var content string
	if m.loading {
		content = timesheetPanel("Loading worklogs…")
		view := tea.NewView(content)
		view.AltScreen = true
		return view
	}
	if m.err != nil {
		content = timesheetPanel("Error: " + m.err.Error() + "\n\nPress q to exit.")
		view := tea.NewView(content)
		view.AltScreen = true
		return view
	}
	header, footer := m.fixedSections()
	sections := []string{header}
	if len(m.report.Entries) == 0 {
		sections = append(sections, "No worklogs found.")
	} else if m.tableHeight() > 0 {
		sections = append(sections, m.table.View())
	}
	sections = append(sections, footer)
	content = joinTimesheetSections(sections...)
	view := tea.NewView(timesheetPanel(content))
	view.AltScreen = true
	return view
}

func (m timesheetModel) fixedSections() (string, string) {
	_, total := totalsByDay(m.report.Entries)
	header := fmt.Sprintf("User: %s  Filter: %s  Total: %s", m.report.User, m.report.Filter, formatDuration(total))
	footer := []string{}
	if len(m.report.Errors) > 0 {
		partial := fmt.Sprintf("Partial result: %d issue(s) could not be read.", len(m.report.Errors))
		if m.report.Verbose {
			for _, err := range m.report.Errors {
				partial += "\n" + err.Error()
			}
		}
		footer = append(footer, partial)
	}
	footer = append(footer, dailyTotalLines(m.report.Entries), "q: quit  ↑/k: up  ↓/j: down  PgUp/PgDn: page")
	return header, joinTimesheetSections(footer...)
}

func joinTimesheetSections(sections ...string) string {
	visible := make([]string, 0, len(sections))
	for _, section := range sections {
		if section != "" {
			visible = append(visible, section)
		}
	}
	return strings.Join(visible, "\n\n")
}

func (m timesheetModel) tableHeight() int {
	header, footer := m.fixedSections()
	chrome := joinTimesheetSections(header, footer)
	// The table replaces the single separator between header and footer with two.
	return max(0, m.height-timesheetPanelFrameHeight()-lipgloss.Height(chrome)-lipgloss.Height("\n\n"))
}

func (m *timesheetModel) resizeTable() {
	m.table.SetWidth(max(0, m.width-timesheetPanelFrameWidth()))
	m.table.SetHeight(m.tableHeight())
}

func dailyTotalLines(entries []jiraclient.WorklogEntry) string {
	totals, _ := totalsByDay(entries)
	days := make([]string, 0, len(totals))
	for day := range totals {
		days = append(days, day)
	}
	sort.Strings(days)
	lines := make([]string, 0, len(days)+1)
	lines = append(lines, "Daily totals:")
	for _, day := range days {
		lines = append(lines, fmt.Sprintf("%s: %s", day, formatDuration(totals[day])))
	}
	return strings.Join(lines, "\n")
}

func newTimesheetTable(entries []jiraclient.WorklogEntry, width, height int) table.Model {
	rows := make([]table.Row, 0, len(entries))
	for _, entry := range sortedEntries(entries) {
		rows = append(rows, table.Row{entry.Started.In(time.Local).Format("2006-01-02"), entry.IssueKey, entry.IssueSummary, formatDuration(entry.TimeSpentSeconds)})
	}
	model := table.New(
		table.WithColumns([]table.Column{{Title: "Date", Width: 12}, {Title: "Issue", Width: 14}, {Title: "Summary", Width: max(20, width-42)}, {Title: "Time", Width: 10}}),
		table.WithRows(rows), table.WithWidth(width), table.WithHeight(height),
	)
	model.Focus()
	return model
}

func timesheetPanel(content string) string {
	return timesheetPanelStyle().Render(content)
}

func timesheetPanelStyle() lipgloss.Style {
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("62")).Padding(1, 2)
}

func timesheetPanelFrameHeight() int { return lipgloss.Height(timesheetPanel("")) }

func timesheetPanelFrameWidth() int { return lipgloss.Width(timesheetPanel("")) }
