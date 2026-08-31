package cmd

import (
	"fmt"
	"strings"
)

func resolveIssueKey(ref, defaultProject string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", fmt.Errorf("issue reference cannot be empty")
	}

	isNumber := true
	for _, r := range ref {
		if r < '0' || r > '9' {
			isNumber = false
			break
		}
	}

	if !isNumber {
		return ref, nil
	}

	defaultProject = strings.TrimSpace(defaultProject)
	if defaultProject == "" {
		return "", fmt.Errorf(
			"default project is required for numeric issue reference %q",
			ref,
		)
	}

	return defaultProject + "-" + ref, nil
}
