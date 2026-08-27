package cmd

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
)

var stateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show the diff between manifests and the cluster state",
	Long: `Compare the manifests produced by each manager's Build() with the state stored in
the cluster, and display added, modified, or removed resources.

GenesisSecret resources are always shown as "always-sync" since they must be synced every time.

Examples:
  tazuna state diff
  tazuna state diff -f tazuna.yaml
  tazuna state diff -f tazuna.yaml --otlp-endpoint=localhost:4317`,
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

		r := runner.NewTazunaRunner(logger, k8sClient, nil, runner.WithEnvironment(cliutil.Environment(cmd)), runner.WithRESTConfig(restConfig))

		tazuna, err := cliutil.LoadTazunaYAML(path, cliutil.Environment(cmd))
		if err != nil {
			return err
		}

		if err := r.StateDiff(ctx, *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateDiffCmd)
}
