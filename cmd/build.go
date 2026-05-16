package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Generate the manifests that tazuna would apply",
	Long: `Build the manifests defined in tazuna.yaml and print them to stdout.
The cluster is not actually modified. Useful for previewing or debugging before apply.

Use the --tags flag to target only manifests with the specified tags.

Examples:
  tazuna build -f tazuna.yaml
  tazuna build -f tazuna.yaml --tags web`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return err
		}

		logger, err := cliutil.NewLogger(cmd)
		if err != nil {
			return err
		}

		tags := getTags(cmd)

		k8sClient, err := cliutil.NewK8sClient()
		if err != nil {
			return err
		}
		orasOpts, err := buildORASPullOptions(cmd)
		if err != nil {
			return err
		}
		r := runner.NewTazunaRunner(logger, k8sClient, &op.CommandClient{}, runner.WithTags(tags), runner.WithORASPullOptions(orasOpts))

		tazuna, err := cliutil.LoadTazunaYAML(path)
		if err != nil {
			return err
		}

		// tazuna.yamlのvalidation（include展開前のバリデーション）
		if err := validator.ValidateTazunaWithBasePath(tazuna, filepath.Dir(path)); err != nil {
			return errors.Wrapf(err, "validation failed for tazuna.yaml at %s", path)
		}

		out, err := r.Build(cmd.Context(), *tazuna, path)
		if err != nil {
			return errors.WithStack(err)
		}

		fmt.Println(out)
		return nil
	},
}

func init() {
	buildCmd.Flags().String("cluster-name", "kind-tazuna", "cluster name")
	addTagsFlag(buildCmd, "Filter manifests by tag; only matching tags are built")
	addORASPullFlags(buildCmd)
	rootCmd.AddCommand(buildCmd)
}
