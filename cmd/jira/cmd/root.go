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

var rootCmd = &cobra.Command{
	Use:   "jira",
	Short: "Jira CLI for day-to-day issue actions",
	Long: "A Go CLI that migrates the zsh jira helpers to a single binary.\n\n" +
		"Configuration is loaded from config.toml at " +
		"os.UserConfigDir()/no-more-interfaz-jira/config.toml, then .env in " +
		"the current directory, then shell environment variables. Earlier " +
		"sources win per key.\n\n" +
		"OAuth login requires JIRA_BASE_URL, JIRA_CLIENT_ID, JIRA_CLIENT_SECRET, " +
		"and JIRA_REDIRECT_URI. JIRA_API_TOKEN and JIRA_EMAIL remain available " +
		"for Basic auth automation. Secrets are never printed.",
	SilenceUsage:  true,
	SilenceErrors: true,
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if isAuthCommand(cmd) {
			return nil
		}
		resolvedConfig, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
		cfg = resolvedConfig

		c, err := jiraclient.NewClient(jiraclient.ClientConfig{
			BaseURL:      cfg.BaseURL,
			Email:        cfg.Email,
			APIToken:     cfg.APIToken,
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURI:  cfg.RedirectURI,
		})
		if err != nil {
			return fmt.Errorf("building jira client: %w", err)
		}
		client = c
		return nil
	},
}

func init() {
	rootCmd.PersistentFlags().Bool("dry-run", false, "print the curl request instead of executing the command")
	rootCmd.AddCommand(issueCmd)
	rootCmd.AddCommand(workloadCmd)
	rootCmd.AddCommand(listCmd)
	rootCmd.AddCommand(authCmd)
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
