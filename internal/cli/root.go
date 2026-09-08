// Package cli provides the Skill Manager command-line interface.
package cli

import "github.com/spf13/cobra"

type rootOptions struct {
	configPath string
}

// NewRootCommand constructs the read-only Skill Manager command tree.
func NewRootCommand() *cobra.Command {
	options := &rootOptions{}
	root := &cobra.Command{
		Use:           "skill-manager",
		Short:         "Discover and manage local directory skills",
		SilenceErrors: true,
		SilenceUsage:  true,
	}
	root.PersistentFlags().StringVar(&options.configPath, "config", "", "path to Skill Manager configuration")
	root.AddCommand(newSearchCommand(options))
	root.AddCommand(newInitCommand())
	root.AddCommand(newProjectCommands(options)...)
	return root
}
