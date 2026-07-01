package cmd

import (
	"context"
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

With --sync, apply switches to differential mode: each manager's Build() output is
diffed against the saved state, and only added/modified resources are applied.
Resources whose hash has not changed are skipped.

With --prune (requires --sync), resources that exist in the state but not in the
current manifest output are deleted from the cluster.

With --atomic (requires --sync), state is saved only after all manifests have been
processed successfully; if any error occurs, the previously stored state is left
untouched.

The target cluster is determined by the kubeconfig context.
When context_matches is configured, the current context name is validated.

With -e/--environment <name>, spec.environments.<name>.context_matches is used
instead of the root context_matches, and {{ .Environment }} in tazuna.yaml is
rendered to <name>.

Examples:
  tazuna apply -f tazuna.yaml
  tazuna apply -f tazuna.yaml --tags web,batch
  tazuna apply -f tazuna.yaml -e production
  tazuna apply -f tazuna.yaml --log-level debug
  tazuna apply -f tazuna.yaml --sync
  tazuna apply -f tazuna.yaml --sync --prune
  tazuna apply -f tazuna.yaml --sync --atomic
  tazuna apply -f tazuna.yaml --otlp-endpoint=localhost:4317`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		_, shutdownTracer, err := cliutil.SetupTracerFromCmd(ctx, cmd)
		if err != nil {
			return err
		}
		defer func() { _ = shutdownTracer(context.Background()) }()

		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
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

		sync, err := cmd.Flags().GetBool("sync")
		if err != nil {
			return errors.WithStack(err)
		}
		prune, err := cmd.Flags().GetBool("prune")
		if err != nil {
			return errors.WithStack(err)
		}
		atomic, err := cmd.Flags().GetBool("atomic")
		if err != nil {
			return errors.WithStack(err)
		}

		// --prune / --atomic は --sync 必須。CLI 層で早期に弾く。
		if prune && !sync {
			return errors.New("--prune requires --sync")
		}
		if atomic && !sync {
			return errors.New("--atomic requires --sync")
		}

		applyOpts := runner.ApplyOptions{
			Sync:   sync,
			Prune:  prune,
			Atomic: atomic,
		}

		environment := cliutil.Environment(cmd)

		r := runner.NewTazunaRunner(
			logger,
			k8sClient,
			&op.CommandClient{},
			runner.WithTags(tags),
			runner.WithORASPullOptions(orasOpts),
			runner.WithApplyOptions(applyOpts),
			runner.WithEnvironment(environment),
		)

		tazuna, err := cliutil.LoadTazunaYAML(path, environment)
		if err != nil {
			return err
		}

		// tazuna.yamlのvalidation（include展開前のバリデーション）
		if err := validator.ValidateTazunaWithBasePath(tazuna, filepath.Dir(path)); err != nil {
			return errors.Wrapf(err, "validation failed for tazuna.yaml at %s", path)
		}

		contextMatches, contextMatchMode, err := cliutil.ResolveContextMatches(tazuna.Spec, environment)
		if err != nil {
			return err
		}
		if len(contextMatches) > 0 {
			if err := tazunacontext.ValidateCurrentContext(contextMatches, contextMatchMode); err != nil {
				return err
			}
		}

		if err := r.Apply(ctx, *tazuna, path); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	addTagsFlag(applyCmd, "Filter manifests by tag; only matching tags are applied")
	addORASPullFlags(applyCmd)
	applyCmd.Flags().Bool("sync", false, "Enable differential apply: only added/modified resources are applied based on the saved state")
	applyCmd.Flags().Bool("prune", false, "Delete resources that exist in the state but not in the current manifest output (requires --sync)")
	applyCmd.Flags().Bool("atomic", false, "Save state only after all manifests succeed; leaves the state untouched on error (requires --sync)")
	rootCmd.AddCommand(applyCmd)

}
