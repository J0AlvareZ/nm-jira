package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",

	DisableFlagsInUseLine: true,

	ValidArgs: []string{
		"bash",
		"zsh",
		"fish",
		"powershell",
	},

	Args: cobra.MatchAll(
		cobra.ExactArgs(1),
		cobra.OnlyValidArgs,
	),

	RunE: func(cmd *cobra.Command, args []string) error {
		output := cmd.OutOrStdout()

		switch args[0] {
		case "bash":
			return cmd.Root().GenBashCompletion(output)

		case "zsh":
			return cmd.Root().GenZshCompletion(output)

		case "fish":
			return cmd.Root().GenFishCompletion(output, true)

		case "powershell":
			return cmd.Root().GenPowerShellCompletionWithDesc(output)

		default:
			return fmt.Errorf("unsupported shell: %s", args[0])

		}
	},
}
