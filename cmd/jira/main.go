package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	cmd "github.com/J0AlvareZ/no-more/nm-jira/cmd/jira/cmd"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		os.Interrupt,
		syscall.SIGTERM,
	)
	defer stop()

	if err := cmd.Execute(ctx); err != nil {
		_, _ = os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
