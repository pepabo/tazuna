package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"sort"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
)

var tagsCmd = &cobra.Command{
	Use:   "tags",
	Short: "List the tags defined in tazuna.yaml",
	Long: `List the tags attached to manifests in tazuna.yaml together with the manifest names associated with each tag.

When --tags is specified, output is restricted to those tag names.

Examples:
  tazuna tags -f tazuna.yaml
  tazuna tags -f tazuna.yaml --tags frontend,backend`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
		}

		tazuna, err := cliutil.LoadTazunaYAML(path)
		if err != nil {
			return err
		}

		if err := validator.ValidateTazuna(tazuna); err != nil {
			return errors.Wrapf(err, "validation failed for tazuna.yaml at %s", path)
		}

		logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
		r := runner.NewTazunaRunner(logger, nil, nil)

		tags, err := r.ListTags(cmd.Context(), tazuna, path)
		if err != nil {
			return errors.Wrapf(err, "failed to list tags for tazuna.yaml at %s", path)
		}

		filter := getTags(cmd)
		printTagsFiltered(cmd.OutOrStdout(), tags, filter)
		return nil
	},
}

// printTagsFiltered writes the tag→names map to w. When filter is non-empty,
// only the listed tag names are emitted; output is sorted for determinism.
func printTagsFiltered(w io.Writer, tags map[string][]string, filter []string) {
	want := tags
	if len(filter) > 0 {
		want = make(map[string][]string, len(filter))
		for _, t := range filter {
			if names, ok := tags[t]; ok {
				want[t] = names
			}
		}
	}
	keys := make([]string, 0, len(want))
	for k := range want {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, tag := range keys {
		fmt.Fprintf(w, "%s:\n", tag)
		for _, name := range want[tag] {
			fmt.Fprintf(w, "- %s\n", name)
		}
	}
}

func init() {
	addTagsFlag(tagsCmd, "Restrict output to the listed tags")
	rootCmd.AddCommand(tagsCmd)
}
