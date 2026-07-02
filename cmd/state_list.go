package cmd

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
)

var stateListCmd = &cobra.Command{
	Use:   "list",
	Short: "List managed resources and their content hashes",
	Long: `Read the state ConfigMap from the cluster and list the resources tazuna manages
along with their content hashes.

State is fetched using manifest names defined in tazuna.yaml, and the GVK,
namespace/name, and content hash of each resource are displayed.

Examples:
  tazuna state list
  tazuna state list -f tazuna.yaml
  tazuna state list -f tazuna.yaml --otlp-endpoint=localhost:4317`,
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

		if err := r.StateList(ctx, *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateListCmd)
}
