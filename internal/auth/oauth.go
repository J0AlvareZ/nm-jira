package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	"golang.org/x/oauth2"
)

const (
	authorizeURL = "https://auth.atlassian.com/authorize"
	scopes       = "read:jira-work write:jira-work offline_access"
)

// tokenURL and resourceURL are variables (not constants) so tests can point
// them at an httptest server without contacting the real Atlassian endpoints.
var (
	tokenURL        = "https://auth.atlassian.com/oauth/token"
	resourceURL     = "https://api.atlassian.com/oauth/token/accessible-resources"
	redirectURI     = "http://127.0.0.1:8080/callback"
	defaultClientID = "DDNNU9siRwp9fYyHbyyUOYaa4akQtjHa"
)

// EmbeddedClientSecret is injected for distribution builds with:
// go build -ldflags "-X github.com/J0AlvareZ/no-more/nm-jira/internal/auth.EmbeddedClientSecret=<secret>"
var EmbeddedClientSecret string

type LoginOptions struct {
	NoBrowser bool
	Code      string
	Output    io.Writer
}
type callbackResult struct {
	Code string
	Err  error
}

func OAuthConfig(cfg config.Config) (*oauth2.Config, error) {
	clientID := strings.TrimSpace(cfg.ClientID)
	if clientID == "" {
		clientID = defaultClientID
	}
	clientSecret := strings.TrimSpace(cfg.ClientSecret)
	if clientSecret == "" {
		clientSecret = strings.TrimSpace(EmbeddedClientSecret)
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("OAuth client secret is not configured; set JIRA_CLIENT_SECRET for development or use a distribution build with an embedded client secret")
	}
	return &oauth2.Config{ClientID: clientID, ClientSecret: clientSecret, RedirectURL: redirectURI, Endpoint: oauth2.Endpoint{AuthURL: authorizeURL, TokenURL: tokenURL}, Scopes: strings.Fields(scopes)}, nil
}

// authCodeURL builds the authorization URL for the 3LO code flow, including
// the audience, scopes, state, consent prompt and PKCE S256 challenge.
func authCodeURL(oauthCfg *oauth2.Config, state, verifier string) string {
	challenge := pkceChallenge(verifier)
	return oauthCfg.AuthCodeURL(state,
		oauth2.SetAuthURLParam("audience", "api.atlassian.com"),
		oauth2.SetAuthURLParam("prompt", "consent"),
		oauth2.SetAuthURLParam("code_challenge", challenge),
		oauth2.SetAuthURLParam("code_challenge_method", "S256"),
	)
}

func Login(ctx context.Context, cfg config.Config, options LoginOptions) (*Session, error) {
	if err := cfg.ValidateOAuthLogin(); err != nil {
		return nil, err
	}
	oauthCfg, err := OAuthConfig(cfg)
	if err != nil {
		return nil, err
	}
	if err := validateLoopbackRedirect(redirectURI); err != nil {
		return nil, err
	}
	state, err := randomURLValue()
	if err != nil {
		return nil, err
	}
	verifier, err := randomURLValue()
	if err != nil {
		return nil, err
	}
	authURL := authCodeURL(oauthCfg, state, verifier)
	output := options.Output
	if output == nil {
		output = io.Discard
	}
	if _, err := fmt.Fprintf(output, "Open this URL to authorize Jira CLI:\n%s\n", authURL); err != nil {
		return nil, fmt.Errorf("writing authorization URL: %w", err)
	}
	if !options.NoBrowser {
		_ = OpenBrowser(authURL)
	}
	code := options.Code
	if code == "" {
		result, err := waitForCallback(ctx, redirectURI, state)
		if err != nil {
			return nil, err
		}
		if result.Err != nil {
			return nil, result.Err
		}
		code = result.Code
	}
	token, err := oauthCfg.Exchange(ctx, code, oauth2.SetAuthURLParam("code_verifier", verifier))
	if err != nil {
		return nil, fmt.Errorf("exchanging OAuth authorization code: %w", err)
	}
	cloudID, siteURL, err := selectResource(ctx, http.DefaultClient, token.AccessToken, cfg.BaseURL)
	if err != nil {
		return nil, err
	}
	session := &Session{Version: 1, SiteURL: siteURL, CloudID: cloudID, AccessToken: token.AccessToken, RefreshToken: token.RefreshToken, Expiry: token.Expiry, Scope: scopes}
	if err := SaveSession(*session); err != nil {
		return nil, err
	}
	return session, nil
}

func validateLoopbackRedirect(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("OAuth loopback redirect must be a loopback URL: %w", err)
	}
	host := strings.Trim(u.Hostname(), "[]")
	if u.Scheme != "http" || u.Port() == "" || u.Path == "" || (host != "localhost" && host != "127.0.0.1" && host != "::1") {
		return fmt.Errorf("OAuth loopback redirect must be an http loopback URL with port and callback path")
	}
	return nil
}

func waitForCallback(ctx context.Context, redirectURI, expectedState string) (callbackResult, error) {
	u, _ := url.Parse(redirectURI)
	listener, err := net.Listen("tcp", u.Host)
	if err != nil {
		return callbackResult{}, fmt.Errorf("listening for OAuth callback: %w", err)
	}
	defer func() { _ = listener.Close() }()
	result := make(chan callbackResult, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != u.Path {
			http.NotFound(w, r)
			return
		}
		if oauthErr := r.URL.Query().Get("error"); oauthErr != "" {
			result <- callbackResult{Err: fmt.Errorf("OAuth authorization failed: %s", oauthErr)}
			http.Error(w, "Authorization failed; return to the terminal.", http.StatusBadRequest)
			return
		}
		if r.URL.Query().Get("state") != expectedState {
			result <- callbackResult{Err: fmt.Errorf("OAuth callback state did not match")}
			http.Error(w, "Invalid OAuth state.", http.StatusBadRequest)
			return
		}
		code := r.URL.Query().Get("code")
		if code == "" {
			result <- callbackResult{Err: fmt.Errorf("OAuth callback did not include a code")}
			http.Error(w, "Missing authorization code.", http.StatusBadRequest)
			return
		}
		result <- callbackResult{Code: code}
		_, _ = fmt.Fprintln(w, "Authorization complete. You can return to the terminal.")
	})}
	go func() { _ = server.Serve(listener) }()
	defer func() { _ = server.Close() }()
	select {
	case r := <-result:
		return r, nil
	case <-ctx.Done():
		return callbackResult{}, ctx.Err()
	}
}

func selectResource(ctx context.Context, client *http.Client, accessToken, baseURL string) (string, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, resourceURL, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)
	resp, err := client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("loading accessible Jira resources: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("loading accessible Jira resources: status %d", resp.StatusCode)
	}
	var resources []struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
		return "", "", fmt.Errorf("decoding accessible Jira resources: %w", err)
	}
	target := normalizeSiteURL(baseURL)
	for _, resource := range resources {
		if normalizeSiteURL(resource.URL) == target {
			return resource.ID, resource.URL, nil
		}
	}
	return "", "", fmt.Errorf("no accessible Jira resource matches JIRA_BASE_URL %q", baseURL)
}

func normalizeSiteURL(raw string) string {
	return strings.TrimRight(strings.ToLower(strings.TrimSpace(raw)), "/")
}

func randomURLValue() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generating OAuth random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func pkceChallenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

type PersistentTokenSource struct {
	mu      sync.Mutex
	source  oauth2.TokenSource
	session *Session
}

func NewPersistentTokenSource(cfg config.Config, session *Session) (*PersistentTokenSource, error) {
	oauthCfg, err := OAuthConfig(cfg)
	if err != nil {
		return nil, err
	}
	token := &oauth2.Token{AccessToken: session.AccessToken, RefreshToken: session.RefreshToken, Expiry: session.Expiry}
	return &PersistentTokenSource{source: oauthCfg.TokenSource(context.Background(), token), session: session}, nil
}

func (p *PersistentTokenSource) Token() (*oauth2.Token, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	token, err := p.source.Token()
	if err != nil {
		return nil, err
	}
	if token.AccessToken != p.session.AccessToken || token.RefreshToken != p.session.RefreshToken || !token.Expiry.Equal(p.session.Expiry) {
		p.session.AccessToken, p.session.RefreshToken, p.session.Expiry = token.AccessToken, token.RefreshToken, token.Expiry
		if err := SaveSession(*p.session); err != nil {
			return nil, err
		}
	}
	return token, nil
}

func SessionStatus() (*Session, error) { return LoadSession() }
func SessionExpired(session *Session) bool {
	return session == nil || (!session.Expiry.IsZero() && time.Now().After(session.Expiry))
}
