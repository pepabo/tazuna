package cmd

import (
	"os"
	"path/filepath"
	"strconv"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/kubecontext"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/prompt"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
)

// destroyCmd represents the build command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Delete tazuna-managed resources",
	Long: `Delete the resources managed by tazuna.yaml from the cluster.

Two safety guards are in place:
  1. A confirmation prompt is shown before execution (skip with --force)
  2. The command runs only when the environment variable TAZUNA_DESTROY_EXECUTABLE=true is set

Use the --tags flag to target only manifests with the specified tags.

Examples:
  TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy -f tazuna.yaml
  TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy -f tazuna.yaml --force
  TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy -f tazuna.yaml --otlp-endpoint=localhost:4317`,
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

		k8sClient, restConfig, err := cliutil.NewK8sClientAndConfig()
		if err != nil {
			return err
		}
		tags := getTags(cmd)
		orasOpts, err := buildORASPullOptions(cmd)
		if err != nil {
			return err
		}
		environment := cliutil.Environment(cmd)
		r := runner.NewTazunaRunner(logger, k8sClient, &op.CommandClient{}, runner.WithTags(tags), runner.WithORASPullOptions(orasOpts), runner.WithEnvironment(environment), runner.WithRESTConfig(restConfig))

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
			if err := kubecontext.ValidateCurrentContext(contextMatches, contextMatchMode); err != nil {
				return err
			}
		}

		// 確認ガード: GetBool のエラーを握り潰すと確認プロンプトなしで destroy に
		// 進む fail-open になるため、エラーは必ず返す。
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return errors.WithStack(err)
		}
		if !force {
			ok, err := prompt.YesORNo(os.Stdin, "!!! All resources managed by Tazuna will be deleted !!!\nAre you sure you want to delete them?")
			if err != nil {
				return errors.WithStack(err)
			}

			if !ok {
				return nil
			}
		}

		if v, err := strconv.ParseBool(os.Getenv("TAZUNA_DESTROY_EXECUTABLE")); err != nil || !v {
			logger.Error("TAZUNA_DESTROY_EXECUTABLE is false, skipping destroy")
			return nil
		}

		if err := r.Destroy(ctx, *tazuna, path); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	destroyCmd.Flags().Bool("force", false, "Delete without confirmation")
	addTagsFlag(destroyCmd, "Filter manifests by tag; only matching tags are destroyed")
	addORASPullFlags(destroyCmd)
	rootCmd.AddCommand(destroyCmd)
}
