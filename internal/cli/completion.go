package cli

import (
	"fmt"
	"os"

	"github.com/jasonraimondi/plan-bender/internal/config"
	"github.com/spf13/cobra"
)

// NewCompletionCmd creates the completion command.
func NewCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:       "completion [bash|zsh|fish]",
		Short:     "Generate shell completion scripts",
		Args:      cobra.ExactArgs(1),
		ValidArgs: []string{"bash", "zsh", "fish"},
		RunE: func(cmd *cobra.Command, args []string) error {
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(cmd.OutOrStdout(), true)
			case "zsh":
				return cmd.Root().GenZshCompletion(cmd.OutOrStdout())
			case "fish":
				return cmd.Root().GenFishCompletion(cmd.OutOrStdout(), true)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}
	return cmd
}

// SlugCompletionFunc returns a ValidArgsFunction that completes plan slugs.
func SlugCompletionFunc() cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, toComplete string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) > 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		root, err := os.Getwd()
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		cfg, err := config.Load(root)
		if err != nil {
			return nil, cobra.ShellCompDirectiveError
		}
		entries, err := os.ReadDir(cfg.PlansDir)
		if err != nil {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		var slugs []string
		for _, e := range entries {
			if e.IsDir() {
				slugs = append(slugs, e.Name())
			}
		}
		return slugs, cobra.ShellCompDirectiveNoFileComp
	}
}
