// Package cmd crea un archivo temporal que luego siempre es eliminado
package cmd

import (
	"fmt"
	"os"
	"os/exec"
)

func openInEditor(initial string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "nano"
	}

	f, err := os.CreateTemp("", "jira-*.md")
	if err != nil {
		return "", err
	}

	path := f.Name()
	defer func() { _ = os.Remove(path) }()

	if _, err := f.WriteString(initial); err != nil {
		_ = f.Close()
		return "", err
	}

	if err := f.Close(); err != nil {
		return "", err
	}

	c := exec.Command("sh", "-c", editor+"\"$@\"", "sh", path)
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return "", fmt.Errorf("editor exited with error: %w", err)
	}

	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
