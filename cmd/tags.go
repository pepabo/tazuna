package cmd

import (
	"fmt"
	"io"
	"log/slog"

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

Examples:
  tazuna tags -f tazuna.yaml`,
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

		for tag, relatedNames := range tags {
			fmt.Printf("%s:\n", tag)
			for _, name := range relatedNames {
				fmt.Printf("- %s\n", name)
			}
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(tagsCmd)
}
