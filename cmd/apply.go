package cmd

import (
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	tazunacontext "github.com/pepabo/tazuna/pkg/context"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
)

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Bootstrap the cluster",
	Long: `Apply the manifests defined in tazuna.yaml to the cluster in order.

Each manifest is processed in declaration order, and tests are run after the apply
when test plugins are configured. The --tags flag limits processing to manifests
that carry the specified tags.

The target cluster is determined by the kubeconfig context.
When context_matches is configured, the current context name is validated.

Examples:
  tazuna apply -f tazuna.yaml
  tazuna apply -f tazuna.yaml --tags web,batch
  tazuna apply -f tazuna.yaml --log-level debug`,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
		}

		logger, err := cliutil.NewLogger(cmd)
		if err != nil {
			return err
		}

		tags := []string{}
		if v, err := cmd.Flags().GetStringSlice("tags"); err == nil {
			tags = v
		}

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

		if len(tazuna.Spec.ContextMatches) > 0 {
			if err := tazunacontext.ValidateCurrentContext(tazuna.Spec.ContextMatches, tazuna.Spec.ContextMatchMode); err != nil {
				return err
			}
		}

		if err := r.Apply(cmd.Context(), *tazuna, path); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	applyCmd.Flags().StringSliceP("tags", "t", []string{}, "Filter manifests by tag; only matching tags are applied")
	addORASPullFlags(applyCmd)
	rootCmd.AddCommand(applyCmd)

}
