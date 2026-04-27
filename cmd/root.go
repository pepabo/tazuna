package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "tazuna",
	Short: "Tame your multi-cluster fleet!!!",
	Long: `Owns cluster bootstrap and handles everything required to reach Production Ready.

Main subcommands:
  apply                    Apply manifests defined in tazuna.yaml to the cluster
  build                    Generate the manifests that would be applied and print them to stdout
  check                    Validate tazuna.yaml
  destroy                  Delete tazuna-managed resources from the cluster
  tags                     List the tags defined in tazuna.yaml
  secret-to-genesissecret  Save existing Secrets to 1Password and generate GenesisSecret`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Help()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %+v\n", err)
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().StringP("kustomize", "k", "", "Process kustomize directory")
	rootCmd.PersistentFlags().StringP("file-path", "f", "tazuna.yaml", "Path to tazuna.yaml")
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "log level(debug/info/warn/error)")
}
