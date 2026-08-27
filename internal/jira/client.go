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

	"github.com/J0AlvareZ/no-more/nm-jira/internal/auth"
	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	jira "github.com/andygrunwald/go-jira"
	"golang.org/x/oauth2"
)

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
