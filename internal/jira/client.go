package jiraclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/J0AlvareZ/no-more/nm-jira/internal/auth"
	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	jira "github.com/andygrunwald/go-jira"
	"golang.org/x/oauth2"
)

// WorklogEntry is a worklog together with the issue data needed for reports.
type WorklogEntry struct {
	Started          time.Time
	IssueKey         string
	IssueSummary     string
	TimeSpentSeconds int
}

// WorklogSearchResult contains the worklogs that could be retrieved. Errors
// are per issue so callers can still present the successfully retrieved data.
type WorklogSearchResult struct {
	Entries []WorklogEntry
	Errors  []error
}

type ClientConfig struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
}

var (
	httpClient *http.Client
	baseURL    string
)

// NewClient uses a persisted OAuth session.
func NewClient(cfg ClientConfig) (*jira.Client, error) {
	session, err := auth.LoadSession()
	if err != nil {
		return nil, err
	}
	if session == nil {
		return nil, fmt.Errorf("no local OAuth session; run jira auth login")
	}
	oauthCfg := config.Config{BaseURL: cfg.BaseURL, ClientID: cfg.ClientID, ClientSecret: cfg.ClientSecret}
	source, err := auth.NewPersistentTokenSource(oauthCfg, session)
	if err != nil {
		return nil, err
	}
	httpClient = oauth2.NewClient(context.Background(), source)
	baseURL = fmt.Sprintf("https://api.atlassian.com/ex/jira/%s", session.CloudID)
	return jira.NewClient(httpClient, baseURL)
}

// SearchJQL searches issues by JQL against the new /rest/api/3/search/jql
// endpoint, which returns only issue IDs (the legacy /search endpoints are
// removed on this instance). Each ID is then fetched to get its fields.
func SearchJQL(jql string, maxResults int) ([]jira.Issue, error) {
	body, _ := json.Marshal(map[string]interface{}{
		"jql":        jql,
		"maxResults": maxResults,
	})

	resp, err := doJSON(http.MethodPost, baseURL+"/rest/api/3/search/jql", body)
	if err != nil {
		return nil, err
	}

	var out struct {
		Issues []struct {
			ID string `json:"id"`
		} `json:"issues"`
	}
	if err := json.Unmarshal(resp, &out); err != nil {
		return nil, err
	}

	issues := make([]jira.Issue, 0, len(out.Issues))
	for _, it := range out.Issues {
		issue, err := GetIssue(it.ID)
		if err != nil {
			return nil, err
		}
		issues = append(issues, *issue)
	}
	return issues, nil
}

// SearchWorklogs returns an account's worklogs in [start, end). Jira's search
// endpoint only yields issue IDs, so worklogs are fetched once per matching
// issue (rather than once per worklog) and both search and worklog responses
// are paginated.
func SearchWorklogs(accountID string, start, end time.Time) (WorklogSearchResult, error) {
	if strings.TrimSpace(accountID) == "" {
		return WorklogSearchResult{}, fmt.Errorf("account ID is required")
	}

	jql := fmt.Sprintf(
		`worklogAuthor = %q AND worklogDate >= %q AND worklogDate < %q`,
		accountID,
		start.Format("2006-01-02"),
		end.Format("2006-01-02"),
	)

	issueIDs, err := searchIssueIDs(jql)
	if err != nil {
		return WorklogSearchResult{}, err
	}

	result := WorklogSearchResult{}
	for _, issueID := range issueIDs {
		issue, err := GetIssue(issueID)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("fetching issue %s: %w", issueID, err))
			continue
		}

		entries, err := worklogsForIssue(issue.ID, issue.Key, summaryOfIssue(*issue), accountID, start, end)
		if err != nil {
			result.Errors = append(result.Errors, fmt.Errorf("fetching worklogs for %s: %w", issue.Key, err))
			continue
		}
		result.Entries = append(result.Entries, entries...)
	}

	return result, nil
}

func searchIssueIDs(jql string) ([]string, error) {
	var issueIDs []string
	nextPageToken := ""
	for {
		payload := map[string]interface{}{"jql": jql, "maxResults": 100}
		if nextPageToken != "" {
			payload["nextPageToken"] = nextPageToken
		}
		body, _ := json.Marshal(payload)
		resp, err := doJSON(http.MethodPost, baseURL+"/rest/api/3/search/jql", body)
		if err != nil {
			return nil, err
		}
		var page struct {
			Issues []struct {
				ID string `json:"id"`
			} `json:"issues"`
			NextPageToken string `json:"nextPageToken"`
		}
		if err := json.Unmarshal(resp, &page); err != nil {
			return nil, err
		}
		for _, issue := range page.Issues {
			issueIDs = append(issueIDs, issue.ID)
		}
		if page.NextPageToken == "" {
			return issueIDs, nil
		}
		nextPageToken = page.NextPageToken
	}
}

func worklogsForIssue(issueID, issueKey, summary, accountID string, start, end time.Time) ([]WorklogEntry, error) {
	const pageSize = 100
	var entries []WorklogEntry
	for startAt := 0; ; {
		path := fmt.Sprintf("%s/rest/api/3/issue/%s/worklog?startAt=%d&maxResults=%d", baseURL, url.PathEscape(issueID), startAt, pageSize)
		body, err := doJSON(http.MethodGet, path, nil)
		if err != nil {
			return nil, err
		}
		var page struct {
			StartAt  int `json:"startAt"`
			MaxResults int `json:"maxResults"`
			Total    int `json:"total"`
			Worklogs []struct {
				Author struct {
					AccountID string `json:"accountId"`
				} `json:"author"`
				Started          string `json:"started"`
				TimeSpentSeconds int    `json:"timeSpentSeconds"`
			} `json:"worklogs"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			return nil, err
		}
		for _, worklog := range page.Worklogs {
			if worklog.Author.AccountID != accountID {
				continue
			}
			started, err := time.Parse("2006-01-02T15:04:05.999-0700", worklog.Started)
			if err != nil {
				return nil, fmt.Errorf("parsing worklog date %q: %w", worklog.Started, err)
			}
			started = started.In(start.Location())
			if started.Before(start) || !started.Before(end) {
				continue
			}
			entries = append(entries, WorklogEntry{Started: started, IssueKey: issueKey, IssueSummary: summary, TimeSpentSeconds: worklog.TimeSpentSeconds})
		}
		startAt += len(page.Worklogs)
		if startAt >= page.Total || len(page.Worklogs) == 0 {
			return entries, nil
		}
	}
}

func summaryOfIssue(issue jira.Issue) string {
	if issue.Fields == nil {
		return ""
	}
	return issue.Fields.Summary
}

// GetIssue fetches a single issue by ID (or key) with the fields the CLI uses.
func GetIssue(id string) (*jira.Issue, error) {
	url := fmt.Sprintf("%s/rest/api/3/issue/%s?fields=summary,status,priority,assignee,labels", baseURL, id)
	resp, err := doJSON(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	var issue jira.Issue
	if err := json.Unmarshal(resp, &issue); err != nil {
		return nil, err
	}
	return &issue, nil
}

// doJSON performs an authenticated HTTP request and returns the raw response body.
func doJSON(method, url string, body []byte) ([]byte, error) {
	req, err := http.NewRequest(method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("request failed: status %d: %s", resp.StatusCode, string(b))
	}
	return b, nil
}

// ResolveAssignee resolves a user reference (accountId, "me", email, or
// display name) to a Jira User with a valid accountId for Cloud.
func ResolveAssignee(ref string) (*jira.User, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, nil
	}

	if looksLikeAccountID(ref) {
		return &jira.User{AccountID: ref}, nil
	}

	switch strings.ToLower(ref) {
	case "me", "self", "current":
		id, err := currentUserID()
		if err != nil {
			return nil, err
		}
		return &jira.User{AccountID: id}, nil
	}

	query := url.QueryEscape(ref)
	path := fmt.Sprintf("%s/rest/api/3/user/search?query=%s&maxResults=10", baseURL, query)
	body, err := doJSON(http.MethodGet, path, nil)
	if err != nil {
		return nil, fmt.Errorf("searching user %q: %w", ref, err)
	}

	var users []struct {
		AccountID    string `json:"accountId"`
		EmailAddress string `json:"emailAddress"`
		DisplayName  string `json:"displayName"`
	}
	if err := json.Unmarshal(body, &users); err != nil {
		return nil, fmt.Errorf("parsing user search for %q: %w", ref, err)
	}

	// Exact email match wins when the reference is an email.
	if strings.Contains(ref, "@") {
		for _, u := range users {
			if strings.EqualFold(u.EmailAddress, ref) {
				return &jira.User{AccountID: u.AccountID}, nil
			}
		}
	}
	if len(users) > 0 {
		return &jira.User{AccountID: users[0].AccountID}, nil
	}
	return nil, fmt.Errorf("no Jira user found for %q", ref)
}

func currentUserID() (string, error) {
	body, err := doJSON(http.MethodGet, baseURL+"/rest/api/3/myself", nil)
	if err != nil {
		return "", fmt.Errorf("fetching current user: %w", err)
	}
	var u struct {
		AccountID string `json:"accountId"`
	}
	if err := json.Unmarshal(body, &u); err != nil {
		return "", fmt.Errorf("parsing current user: %w", err)
	}
	if u.AccountID == "" {
		return "", fmt.Errorf("current user response has no accountId")
	}
	return u.AccountID, nil
}

func looksLikeAccountID(s string) bool {
	// Jira Cloud accountIds are 24 hex chars (new) or contain ":" (legacy).
	if strings.Contains(s, ":") {
		return true
	}
	if len(s) != 24 {
		return false
	}
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') && (r < 'A' || r > 'F') {
			return false
		}
	}
	return true
}
