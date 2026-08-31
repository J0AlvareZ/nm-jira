package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// workflowStates is the ordered MRI workflow. Issues move forward one step at
// a time by issuing transitions named "To <state>".
var workflowStates = []string{
	"To Do",
	"Do Today",
	"In Progress",
	"In Review",
	"Approved",
	"Done",
}

var statusCmd = &cobra.Command{
	Use:   "status <issue> <target>",
	Short: "Move an issue to a target status by chaining transitions",
	Args:  cobra.ExactArgs(2),
	RunE:  runStatus,
}

func runStatus(cmd *cobra.Command, args []string) error {
	issueKey, err := resolveIssueKey(args[0], cfg.DefaultProject)
	if err != nil {
		return err
	}
	target := strings.TrimSpace(args[1])

	issue, _, err := client.Issue.Get(issueKey, nil)
	if err != nil {
		return fmt.Errorf("getting issue %s: %w", issueKey, err)
	}

	current := ""
	if issue.Fields != nil && issue.Fields.Status != nil {
		current = issue.Fields.Status.Name
	}
	if current == "" {
		return fmt.Errorf("could not determine current status of %s", issueKey)
	}

	currentIdx := indexOf(workflowStates, current)
	targetIdx := indexOf(workflowStates, target)
	if currentIdx < 0 {
		return fmt.Errorf("current status %q is not in the known workflow", current)
	}
	if targetIdx < 0 {
		return fmt.Errorf("target status %q is not in the known workflow", target)
	}
	if targetIdx <= currentIdx {
		return fmt.Errorf("target %q is not ahead of current %q", target, current)
	}

	for i := currentIdx + 1; i <= targetIdx; i++ {
		next := workflowStates[i]
		transitionName := next

		fmt.Printf("moving to %q\n", transitionName)

		if err := doTransition(issueKey, transitionName); err != nil {
			return fmt.Errorf("moving to %q: %w", transitionName, err)
		}
		time.Sleep(500 * time.Millisecond)
	}

	fmt.Printf("%s is now %q\n", issueKey, target)
	return nil
}

func doTransition(issueKey, name string) error {
	transitions, _, err := client.Issue.GetTransitions(issueKey)
	if err != nil {
		return err
	}
	for _, t := range transitions {
		if t.To.Name == name {
			_, err := client.Issue.DoTransition(issueKey, t.ID)
			return err
		}
	}
	return fmt.Errorf("no transition available to %q", name)
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
