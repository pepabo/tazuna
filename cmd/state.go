package cmd

import (
	"github.com/spf13/cobra"
)

var stateCmd = &cobra.Command{
	Use:   "state",
	Short: "State management commands",
	Long:  `Subcommands that operate on the tazuna state ConfigMap stored in the cluster.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(stateCmd)
}
