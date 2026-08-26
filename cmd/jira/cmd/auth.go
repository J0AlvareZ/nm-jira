package cmd

import (
	"context"
	"fmt"
	"time"

	"github.com/J0AlvareZ/no-more/nm-jira/internal/auth"
	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	"github.com/spf13/cobra"
)

var authCmd = &cobra.Command{Use: "auth", Short: "Manage the local OAuth session"}

var authLoginCmd = &cobra.Command{
	Use: "login", Short: "Sign in with Atlassian OAuth",
	RunE: func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return fmt.Errorf("loading configuration: %w", err)
		}
		noBrowser, _ := cmd.Flags().GetBool("no-browser")
		code, _ := cmd.Flags().GetString("code")
		if _, err := auth.Login(context.Background(), cfg, auth.LoginOptions{NoBrowser: noBrowser, Code: code, Output: cmd.OutOrStdout()}); err != nil {
			return err
		}
		return nil
	},
}

var (
	authLogoutCmd = &cobra.Command{Use: "logout", Short: "Remove the local OAuth session", RunE: func(cmd *cobra.Command, args []string) error { return auth.DeleteSession() }}
	authStatusCmd = &cobra.Command{Use: "status", Short: "Show local authentication status", RunE: func(cmd *cobra.Command, args []string) error {
		session, err := auth.SessionStatus()
		if err != nil {
			return err
		}
		if session == nil {
			_, err = fmt.Fprintln(cmd.OutOrStdout(), "Authentication mode: Basic auth (no local OAuth session)")
			return err
		}
		state := "valid"
		if auth.SessionExpired(session) {
			state = "expired"
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "Authentication mode: OAuth\nSite: %s\nExpiry: %s\nStatus: %s\n", session.SiteURL, session.Expiry.Format(time.RFC3339), state)
		return err
	}}
)

func init() {
	authLoginCmd.Flags().Bool("no-browser", false, "do not attempt to open a browser")
	authLoginCmd.Flags().String("code", "", "authorization code to exchange")
	authCmd.AddCommand(authLoginCmd, authLogoutCmd, authStatusCmd)
}
