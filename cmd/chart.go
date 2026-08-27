package cmd

import (
	"github.com/spf13/cobra"
)

var chartCmd = &cobra.Command{
	Use:   "chart",
	Short: "Helm chart management commands",
	Long:  `Subcommands that operate on helm charts referenced by helmfile manifests in tazuna.yaml.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
}

func init() {
	rootCmd.AddCommand(chartCmd)
}
