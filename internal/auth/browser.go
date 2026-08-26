package auth

import (
	"fmt"
	"os/exec"
	"runtime"
)

func OpenBrowser(rawURL string) error {
	var command string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		command, args = "open", []string{rawURL}
	case "windows":
		command, args = "rundll32", []string{"url.dll,FileProtocolHandler", rawURL}
	default:
		command, args = "xdg-open", []string{rawURL}
	}
	if err := exec.Command(command, args...).Start(); err != nil {
		return fmt.Errorf("opening browser: %w", err)
	}
	return nil
}
