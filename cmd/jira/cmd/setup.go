package cmd

import (
	"context"
	"errors"
	"fmt"

	"charm.land/huh/v2"
	"github.com/J0AlvareZ/no-more/nm-jira/internal/auth"
	"github.com/J0AlvareZ/no-more/nm-jira/internal/config"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Configure Jira access interactively",
	Long:  "Interactively configure Jira access and optional defaults, then sign in with OAuth.",
	Args:  cobra.NoArgs,
	RunE:  runSetup,
}

func init() {
	setupCmd.Flags().Bool("no-browser", false, "do not attempt to open a browser during OAuth setup")
	setupCmd.Flags().String("code", "", "authorization code to exchange during OAuth setup")
}

func runSetup(cmd *cobra.Command, args []string) error {
	cfg := config.Config{}
	if err := runSetupFields(&cfg); err != nil {
		return setupFormError(err)
	}

	confirmed := false
	if err := huh.NewForm(huh.NewGroup(
		huh.NewConfirm().
			Title("Save this configuration?").
			Affirmative("Save").
			Negative("Cancel").
			Value(&confirmed),
	)).Run(); err != nil {
		return setupFormError(err)
	}
	if !confirmed {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), "Setup cancelled; configuration was not changed.")
		return err
	}

	if err := config.Save(cfg); err != nil {
		return fmt.Errorf("saving configuration: %w", err)
	}
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Configuration saved.")
	resolvedCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("loading saved configuration: %w", err)
	}

	noBrowser, _ := cmd.Flags().GetBool("no-browser")
	code, _ := cmd.Flags().GetString("code")
	if _, err := auth.Login(context.Background(), resolvedCfg, auth.LoginOptions{NoBrowser: noBrowser, Code: code, Output: cmd.OutOrStdout()}); err != nil {
		return fmt.Errorf("OAuth login failed after saving configuration: %w\nRetry with jira auth login", err)
	}
	return nil
}

func runSetupFields(cfg *config.Config) error {
	fields := []huh.Field{
		huh.NewInput().Title("JIRA_BASE_URL").Value(&cfg.BaseURL).Validate(validateRequiredURL),
		huh.NewInput().Title("DEFAULT_PROJECT (optional)").Value(&cfg.DefaultProject),
		huh.NewInput().Title("DEFAULT_USER (optional)").Value(&cfg.DefaultUser),
	}
	return huh.NewForm(huh.NewGroup(fields...)).Run()
}

func validateRequiredURL(value string) error {
	if err := (config.Config{BaseURL: value}).Validate(); err != nil {
		return err
	}
	return nil
}

func setupFormError(err error) error {
	if errors.Is(err, huh.ErrUserAborted) {
		return nil
	}
	return err
}
