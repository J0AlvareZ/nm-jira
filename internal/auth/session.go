package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const sessionFilename = "oauth-session.json"

type Session struct {
	Version      int       `json:"version"`
	SiteURL      string    `json:"site_url"`
	CloudID      string    `json:"cloud_id"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
	Scope        string    `json:"scope"`
}

func SessionPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil { return "", fmt.Errorf("resolving user config directory: %w", err) }
	return filepath.Join(dir, "no-more-interfaz-jira", sessionFilename), nil
}

func LoadSession() (*Session, error) {
	path, err := SessionPath(); if err != nil { return nil, err }
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) { return nil, nil }
	if err != nil { return nil, fmt.Errorf("stating OAuth session: %w", err) }
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 { return nil, fmt.Errorf("OAuth session %q has insecure permissions %04o; expected 0600", path, info.Mode().Perm()) }
	data, err := os.ReadFile(path); if err != nil { return nil, fmt.Errorf("reading OAuth session: %w", err) }
	var session Session
	if err := json.Unmarshal(data, &session); err != nil { return nil, fmt.Errorf("decoding OAuth session: %w", err) }
	if session.CloudID == "" || session.AccessToken == "" { return nil, fmt.Errorf("OAuth session is missing cloud_id or access_token") }
	return &session, nil
}

func SaveSession(session Session) error {
	path, err := SessionPath(); if err != nil { return err }
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil { return fmt.Errorf("creating OAuth session directory: %w", err) }
	if runtime.GOOS != "windows" { if err := os.Chmod(dir, 0o700); err != nil { return fmt.Errorf("securing OAuth session directory: %w", err) } }
	data, err := json.MarshalIndent(session, "", "  "); if err != nil { return fmt.Errorf("encoding OAuth session: %w", err) }
	tmp, err := os.CreateTemp(dir, ".oauth-session-*"); if err != nil { return fmt.Errorf("creating OAuth session temp file: %w", err) }
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil { tmp.Close(); return fmt.Errorf("securing OAuth session temp file: %w", err) }
	if _, err := tmp.Write(data); err != nil { tmp.Close(); return fmt.Errorf("writing OAuth session: %w", err) }
	if err := tmp.Close(); err != nil { return fmt.Errorf("closing OAuth session: %w", err) }
	if err := os.Rename(tmpName, path); err != nil { return fmt.Errorf("persisting OAuth session: %w", err) }
	return nil
}

func DeleteSession() error {
	path, err := SessionPath(); if err != nil { return err }
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) { return fmt.Errorf("deleting OAuth session: %w", err) }
	return nil
}
