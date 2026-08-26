package config

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/BurntSushi/toml"
)

const (
	EnvBaseURL        = "JIRA_BASE_URL"
	EnvAPIToken       = "JIRA_API_TOKEN"
	EnvEmail          = "JIRA_EMAIL"
	EnvDefaultProject = "DEFAULT_PROJECT"
	EnvDefaultUser    = "DEFAULT_USER"
	EnvClientID       = "JIRA_CLIENT_ID"
	EnvClientSecret   = "JIRA_CLIENT_SECRET"
	EnvRedirectURI    = "JIRA_REDIRECT_URI"

	configDirectory = "nm-jira"
	configFilename  = "config.toml"
	legacyConfigDirectory = "no-more-interfaz-jira"
)

var configKeys = []string{
	EnvBaseURL,
	EnvAPIToken,
	EnvEmail,
	EnvDefaultProject,
	EnvDefaultUser,
	EnvClientID,
	EnvClientSecret,
	EnvRedirectURI,
}

type Config struct {
	BaseURL        string `toml:"JIRA_BASE_URL"`
	APIToken       string `toml:"JIRA_API_TOKEN"`
	Email          string `toml:"JIRA_EMAIL"`
	DefaultProject string `toml:"DEFAULT_PROJECT"`
	DefaultUser    string `toml:"DEFAULT_USER"`
	ClientID       string `toml:"JIRA_CLIENT_ID"`
	ClientSecret   string `toml:"JIRA_CLIENT_SECRET"`
	RedirectURI    string `toml:"JIRA_REDIRECT_URI"`
}

type fileConfig struct {
	BaseURL        *string `toml:"JIRA_BASE_URL"`
	APIToken       *string `toml:"JIRA_API_TOKEN"`
	Email          *string `toml:"JIRA_EMAIL"`
	DefaultProject *string `toml:"DEFAULT_PROJECT"`
	DefaultUser    *string `toml:"DEFAULT_USER"`
	ClientID       *string `toml:"JIRA_CLIENT_ID"`
	ClientSecret   *string `toml:"JIRA_CLIENT_SECRET"`
	RedirectURI    *string `toml:"JIRA_REDIRECT_URI"`
}

func DefaultPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config directory: %w", err)
	}

	return filepath.Join(configDir, configDirectory, configFilename), nil
}

func legacyPath() (string, error) {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("resolving user config directory: %w", err)
	}
	return filepath.Join(configDir, legacyConfigDirectory, configFilename), nil
}

func Load() (Config, error) {
	configPath, err := DefaultPath()
	if err != nil {
		return Config{}, err
	}
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		configPath, err = legacyPath()
		if err != nil {
			return Config{}, err
		}
	} else if err != nil {
		return Config{}, fmt.Errorf("stating config file %q: %w", configPath, err)
	}

	workingDir, err := os.Getwd()
	if err != nil {
		return Config{}, fmt.Errorf("resolving current working directory: %w", err)
	}

	return LoadWithSources(configPath, filepath.Join(workingDir, ".env"))
}

func LoadWithSources(configPath, dotenvPath string) (Config, error) {
	return loadWithSources(configPath, dotenvPath, os.LookupEnv, os.Stderr)
}

func loadWithSources(
	configPath, dotenvPath string,
	lookupEnv func(string) (string, bool),
	warningWriter io.Writer,
) (Config, error) {
	fileValues, err := loadTOML(configPath, warningWriter)
	if err != nil {
		return Config{}, err
	}

	dotenvValues, err := loadDotenv(dotenvPath)
	if err != nil {
		return Config{}, err
	}

	envValues := make(map[string]string, len(configKeys))
	for _, key := range configKeys {
		if value, ok := lookupEnv(key); ok {
			envValues[key] = value
		}
	}

	values := resolve(fileValues, dotenvValues, envValues)
	cfg := Config{
		BaseURL:        values[EnvBaseURL],
		APIToken:       values[EnvAPIToken],
		Email:          values[EnvEmail],
		DefaultProject: values[EnvDefaultProject],
		DefaultUser:    values[EnvDefaultUser],
		ClientID:       values[EnvClientID],
		ClientSecret:   values[EnvClientSecret],
		RedirectURI:    values[EnvRedirectURI],
	}
	return cfg, nil
}

func loadTOML(path string, warningWriter io.Writer) (map[string]string, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("stating config file %q: %w", path, err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		fmt.Fprintf(
			warningWriter,
			"warning: config file %q has permissions %04o; expected 0600 because it may contain an API Token\n",
			path,
			info.Mode().Perm(),
		)
	}

	var decoded fileConfig
	metadata, err := toml.DecodeFile(path, &decoded)
	if err != nil {
		return nil, fmt.Errorf("decoding config file %q: %w", path, err)
	}

	undecoded := metadata.Undecoded()
	if len(undecoded) > 0 {
		keys := make([]string, 0, len(undecoded))
		for _, key := range undecoded {
			keys = append(keys, key.String())
		}
		return nil, fmt.Errorf(
			"unknown key(s) in config file %q: %s",
			path,
			strings.Join(keys, ", "),
		)
	}

	values := make(map[string]string, len(configKeys))
	if decoded.BaseURL != nil {
		values[EnvBaseURL] = *decoded.BaseURL
	}
	if decoded.APIToken != nil {
		values[EnvAPIToken] = *decoded.APIToken
	}
	if decoded.Email != nil {
		values[EnvEmail] = *decoded.Email
	}
	if decoded.DefaultProject != nil {
		values[EnvDefaultProject] = *decoded.DefaultProject
	}
	if decoded.DefaultUser != nil {
		values[EnvDefaultUser] = *decoded.DefaultUser
	}
	if decoded.ClientID != nil {
		values[EnvClientID] = *decoded.ClientID
	}
	if decoded.ClientSecret != nil {
		values[EnvClientSecret] = *decoded.ClientSecret
	}
	if decoded.RedirectURI != nil {
		values[EnvRedirectURI] = *decoded.RedirectURI
	}

	return values, nil
}

func loadDotenv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("reading .env file %q: %w", path, err)
	}

	values := make(map[string]string)
	for lineNumber, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return nil, fmt.Errorf("parsing .env file %q: line %d is not KEY=value", path, lineNumber+1)
		}

		key = strings.TrimSpace(key)
		if key == "" {
			return nil, fmt.Errorf("parsing .env file %q: line %d has an empty key", path, lineNumber+1)
		}

		values[key] = strings.TrimSpace(value)
	}
	return values, nil
}

func resolve(sources ...map[string]string) map[string]string {
	values := make(map[string]string, len(configKeys))
	for _, key := range configKeys {
		for _, source := range sources {
			if value, ok := source[key]; ok {
				values[key] = value
				break
			}
		}
	}
	return values
}

func (c Config) Validate() error {
	return c.validateBaseURL()
}

func (c Config) ValidateOAuthLogin() error {
	return c.validateBaseURL()
}

func (c Config) ValidateBasicAuth() error {
	if err := c.validateBaseURL(); err != nil {
		return err
	}
	for key, value := range map[string]string{EnvEmail: c.Email, EnvAPIToken: c.APIToken} {
		if strings.TrimSpace(value) == "" {
			return requiredError(key)
		}
	}
	return nil
}

func (c Config) ValidateDefaults() error {
	for key, value := range map[string]string{EnvDefaultProject: c.DefaultProject, EnvDefaultUser: c.DefaultUser} {
		if strings.TrimSpace(value) == "" {
			return requiredError(key)
		}
	}
	return nil
}

func requiredError(key string) error {
	return fmt.Errorf("%s is required; consulted config.toml, .env, and environment variable", key)
}

func (c Config) validateBaseURL() error {
	if strings.TrimSpace(c.BaseURL) == "" {
		return requiredError(EnvBaseURL)
	}
	parsedURL, err := url.Parse(c.BaseURL)
	if err != nil {
		return fmt.Errorf(
			"%s must be an absolute http or https URL: %w",
			EnvBaseURL,
			err,
		)
	}
	if !parsedURL.IsAbs() || parsedURL.Host == "" ||
		(parsedURL.Scheme != "http" && parsedURL.Scheme != "https") {
		return fmt.Errorf(
			"%s mus be an absolute http or https URL", EnvBaseURL)
	}

	return nil
}
