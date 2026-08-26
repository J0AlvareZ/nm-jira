package main

import (
	"fmt"
	"os"

	cmd "github.com/J0AlvareZ/no-more/nm-jira/cmd/jira/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
