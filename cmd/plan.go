package cmd

import (
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	tazunacontext "github.com/pepabo/tazuna/pkg/context"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
)

// planCmd represents the plan command
var planCmd = &cobra.Command{
	Use:   "plan",
	Short: "Show what would change if apply were run",
	Long: `Build manifests, compare them against the live cluster, and display per-field diffs.

The diff is computed client-side: each manifest's Build() output is rendered, every
object is fetched from the live cluster, and util/diff.Diff produces a unified diff
of the desired and current states.

Although the slogan is "server-side dry-run", the actual implementation runs a
client-side comparison. This trade-off keeps the command testable against the
controller-runtime fake client, which does not fully support server-side apply
with dry-run.

Resources that do not yet exist on the cluster are reported as "to be created".
GenesisSecret manifests are skipped because they are always-sync by design.

Examples:
  tazuna plan -f tazuna.yaml
  tazuna plan -f tazuna.yaml --tags web,batch
  tazuna plan -f tazuna.yaml --log-level debug
  tazuna plan -f tazuna.yaml --otlp-endpoint=localhost:4317`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		_, shutdownTracer, err := cliutil.SetupTracerFromCmd(ctx, cmd)
		if err != nil {
			return err
		}
		defer cliutil.ShutdownTracerWithWarn(shutdownTracer)

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

		environment := cliutil.Environment(cmd)
		r := runner.NewTazunaRunner(
			logger,
			k8sClient,
			&op.CommandClient{},
			runner.WithTags(tags),
			runner.WithORASPullOptions(orasOpts),
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

		// plan はライブクラスタへ GET を行うため、apply / destroy と同様に
		// context_matches で意図しないクラスタへの実行を防ぐ。
		contextMatches, contextMatchMode, err := cliutil.ResolveContextMatches(tazuna.Spec, environment)
		if err != nil {
			return err
		}
		if len(contextMatches) > 0 {
			if err := tazunacontext.ValidateCurrentContext(contextMatches, contextMatchMode); err != nil {
				return err
			}
		}

		if err := r.Plan(ctx, *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	addTagsFlag(planCmd, "Filter manifests by tag; only matching tags are planned")
	addORASPullFlags(planCmd)
	rootCmd.AddCommand(planCmd)
}
