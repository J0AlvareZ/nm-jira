package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	jiraclient "github.com/J0AlvareZ/no-more/nm-jira/internal/jira"
	jira "github.com/andygrunwald/go-jira"
)

var (
	client *jira.Client
	cfg    config.Config
)

func Execute() error {
	return rootCmd.Execute()
}

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

var rootCmd = &cobra.Command{
	Use:   "nm-jira",
	Short: "Jira CLI for day-to-day issue actions",
	Long: "A Go CLI that migrates the zsh jira helpers to a single binary.\n\n" +
		"Configuration is loaded from config.toml at " +
		"os.UserConfigDir()/nm-jira/config.toml, then .env in " +
		"the current directory, then shell environment variables. Earlier " +
		"sources win per key.\n\n" +
		"Run jira setup to configure the Jira site and optional defaults, then sign in with OAuth.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if isAuthCommand(cmd) || cmd == setupCmd {
			return nil
		}
		resolvedConfig, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
		cfg = resolvedConfig

		c, err := jiraclient.NewClient(jiraclient.ClientConfig{
			BaseURL:      cfg.BaseURL,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
		})
		if err != nil {
			return fmt.Errorf("building jira client: %w", err)
		}
		client = c
		return nil
	},
}

func init() {
	rootCmd.Version = version
	rootCmd.SetVersionTemplate(
		fmt.Sprintf("nm-jira version {{.Version}} (commit %s built %s)\n", commit, date),
	)
	rootCmd.AddCommand(issueCmd)
	rootCmd.AddCommand(workloadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(authCmd)
	rootCmd.AddCommand(setupCmd)
	rootCmd.AddCommand(commentCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(editCmd)
	rootCmd.AddCommand(suppCmd)
	rootCmd.AddCommand(spCmd)
}

func isAuthCommand(cmd *cobra.Command) bool {
	for c := cmd; c != nil; c = c.Parent() {
		if c == authCmd {
			return true
		}
	}
	return false
}
