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
  plan                     Show per-field diffs between manifests and the live cluster
  status                   Show readiness of resources managed by tazuna
  tags                     List the tags defined in tazuna.yaml
  secret-to-genesissecret  Save existing Secrets to 1Password and generate GenesisSecret
  version                  Print version, commit, and build date`,
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
	rootCmd.PersistentFlags().StringP("file-path", "f", "tazuna.yaml", "Path to tazuna.yaml")
	rootCmd.PersistentFlags().StringP("log-level", "l", "info", "log level(debug/info/warn/error)")
	// OpenTelemetry tracing は短命 CLI 向けに opt-in 設計。
	// --otlp-endpoint が空文字 (default) のときは no-op tracer を使うため外部依存ゼロ。
	// 例: --otlp-endpoint=localhost:4317 で OTLP/gRPC collector に出力する。
	rootCmd.PersistentFlags().String("otlp-endpoint", "", "OTLP/gRPC endpoint to send traces to (e.g. localhost:4317). Empty disables tracing.")
	rootCmd.PersistentFlags().Bool("otlp-insecure", true, "Use plaintext gRPC for the OTLP exporter (no TLS)")
}
