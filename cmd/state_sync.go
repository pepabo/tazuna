package cmd

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
)

var stateSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "Sync resources based on the state",
	Long: `Compare the manifests produced by each manager's Build() with the cluster state and
create or update resources that have been added or changed.

Removed (orphan) resources are skipped by default.
Set the environment variable TAZUNA_STATE_SYNC_DELETE=true to delete them automatically.

The state of successfully synced resources is saved to a ConfigMap.

When --atomic is specified, the state is not updated at all if any resource
encounters an error.

Examples:
  tazuna state sync
  tazuna state sync -f tazuna.yaml
  tazuna state sync --atomic
  TAZUNA_STATE_SYNC_DELETE=true tazuna state sync`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		atomic, err := cmd.Flags().GetBool("atomic")
		if err != nil {
			return errors.WithStack(err)
		}

		opts := runner.StateSyncOptions{
			Atomic: atomic,
		}

		if err := r.StateSync(cmd.Context(), *tazuna, path, os.Stdout, opts); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	stateSyncCmd.Flags().Bool("atomic", false, "Enable atomic mode, which leaves the state untouched when any error occurs")
	stateCmd.AddCommand(stateSyncCmd)
}
