package cmd

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show readiness of resources managed by tazuna",
	Long: `Read all state ConfigMaps, fetch each managed resource from the cluster, and report whether it is Ready.

For each manifest declared in tazuna.yaml this command lists the resources that
have been recorded in the state ConfigMap, fetches them live, and prints one of
the statuses: Ready / NotReady / Missing / Error.

Examples:
  tazuna status
  tazuna status -f tazuna.yaml
  tazuna status -f tazuna.yaml --otlp-endpoint=localhost:4317`,
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

		k8sClient, err := cliutil.NewK8sClient()
		if err != nil {
			return err
		}

		r := runner.NewTazunaRunner(logger, k8sClient, nil, runner.WithEnvironment(cliutil.Environment(cmd)))

		tazuna, err := cliutil.LoadTazunaYAML(path, cliutil.Environment(cmd))
		if err != nil {
			return err
		}

		if err := r.Status(ctx, *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
