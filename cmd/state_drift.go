package cmd

import (
	"context"
	"os"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
)

var stateDriftCmd = &cobra.Command{
	Use:   "drift",
	Short: "Detect drift between the state ConfigMap and the live cluster",
	Long: `Compare the resources recorded in the state ConfigMap with what currently exists on
the live cluster, and report resources whose content has been mutated outside of
tazuna or that have been removed from the cluster.

Unlike "state diff" (which compares declared manifests against the stored state),
"state drift" fetches each resource recorded in the state from the cluster and
recomputes its content hash to surface manual changes such as ad-hoc
"kubectl apply" or out-of-band deletes.

Detection categories:
  live-drifted  the live object's content hash no longer matches the stored hash
  live-missing  the live object recorded in the state could not be found on the cluster

Manifests with no saved state (never applied / synced) and GenesisSecret manifests
(which are always-sync and have no meaningful drift) are skipped. Parallel
manifests are also skipped since their children are not state-tracked.

Examples:
  tazuna state drift
  tazuna state drift -f tazuna.yaml
  tazuna state drift -f tazuna.yaml --otlp-endpoint=localhost:4317`,
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

		k8sClient, err := cliutil.NewK8sClient()
		if err != nil {
			return err
		}

		r := runner.NewTazunaRunner(logger, k8sClient, nil)

		tazuna, err := cliutil.LoadTazunaYAML(path)
		if err != nil {
			return err
		}

		if err := r.StateDrift(ctx, *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateDriftCmd)
}
