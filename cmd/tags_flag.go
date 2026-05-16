package cmd

import "github.com/spf13/cobra"

// addTagsFlag attaches the standard --tags / -t flag to cmd. Tazuna treats
// "filter the manifest set by tag" as a cross-cutting concern, so every
// subcommand that operates on manifests should register the flag through this
// helper rather than redeclaring it inline.
func addTagsFlag(cmd *cobra.Command, usage string) {
	cmd.Flags().StringSliceP("tags", "t", []string{}, usage)
}

// getTags reads the --tags slice from cmd. A missing flag (for subcommands
// that didn't register one) is treated as "no filter".
func getTags(cmd *cobra.Command) []string {
	tags, err := cmd.Flags().GetStringSlice("tags")
	if err != nil {
		return nil
	}
	return tags
}
