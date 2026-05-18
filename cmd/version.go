package cmd

import (
	"fmt"
	"runtime"

	"github.com/spf13/cobra"
)

// Build-time metadata, populated by SetVersionInfo from package main.
// Defaults keep `go run` / `go test` output meaningful before injection.
var (
	versionString = "dev"
	commitString  = "none"
	dateString    = "unknown"
)

// SetVersionInfo wires release metadata from main into the cmd package.
// Called once from main.main before Execute.
func SetVersionInfo(version, commit, date string) {
	if version != "" {
		versionString = version
	}
	if commit != "" {
		commitString = commit
	}
	if date != "" {
		dateString = date
	}
	rootCmd.Version = versionString
	rootCmd.SetVersionTemplate(versionTemplate())
}

func versionTemplate() string {
	return fmt.Sprintf("tazuna %s (commit %s, built %s, %s/%s)\n",
		versionString, commitString, dateString, runtime.GOOS, runtime.GOARCH)
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version, commit, and build date",
	Long: `Print build-time metadata embedded in the binary.

The version string is the release tag (or "dev" for local builds).
The commit hash and build date are injected by goreleaser at release time.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		_, err := fmt.Fprint(cmd.OutOrStdout(), versionTemplate())
		return err
	},
}

func init() {
	// Make --version work on the root command. SetVersionInfo overrides this
	// later with the injected value; the placeholder keeps `tazuna --version`
	// usable for unit tests that never call SetVersionInfo.
	rootCmd.Version = versionString
	rootCmd.SetVersionTemplate(versionTemplate())
	rootCmd.AddCommand(versionCmd)
}
