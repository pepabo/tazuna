package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate tazuna.yaml",
	Long: `Load tazuna.yaml and run validation after expanding includes.

Checks:
  - spec.manifests[].name is required
  - spec.manifests[].name uses only allowed characters (alphanumeric, underscore, hyphen)
  - spec.manifests[].name uniqueness
  - spec.manifests[].name is not a reserved word

When --fix is specified, manifests with no name get one assigned automatically and
tazuna.yaml is written back.

Examples:
  tazuna check -f tazuna.yaml
  tazuna check -f tazuna.yaml --fix`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
		}
		fix, err := cmd.Flags().GetBool("fix")
		if err != nil {
			return errors.WithStack(err)
		}

		f, err := os.Open(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				err = errors.WithStack(cerr)
			}
		}()

		tazuna := v1.Tazuna{}
		if err := yaml.NewDecoder(f).Decode(&tazuna); err != nil {
			return errors.WithStack(err)
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return errors.WithStack(err)
		}

		if err := validator.ValidateTazunaWithBasePath(&tazuna, filepath.Dir(absPath)); err != nil {
			return errors.Wrapf(err, "validation failed for tazuna.yaml at %s", path)
		}

		logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
		r := runner.NewTazunaRunner(logger, nil, nil)

		if fix {
			if err := r.CheckAndFix(cmd.Context(), &tazuna, absPath); err != nil {
				return errors.Wrapf(err, "check --fix failed for tazuna.yaml at %s", path)
			}

			out, err := yaml.Marshal(&tazuna)
			if err != nil {
				return errors.WithStack(err)
			}
			if err := os.WriteFile(path, out, 0644); err != nil {
				return errors.WithStack(err)
			}

			fmt.Printf("fixed: %s\n", path)
			return nil
		}

		if err := r.Check(cmd.Context(), &tazuna, absPath); err != nil {
			return errors.Wrapf(err, "check failed for tazuna.yaml at %s", path)
		}

		fmt.Println("ok")
		return nil
	},
}

func init() {
	checkCmd.Flags().Bool("fix", false, "Auto-assign names to manifests without a name and write them back")
	rootCmd.AddCommand(checkCmd)
}
