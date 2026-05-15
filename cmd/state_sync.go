package cmd

import (
	"log/slog"
	"os"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
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
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
		}

		logLevelS, err := cmd.Flags().GetString("log-level")
		if err != nil {
			return errors.WithStack(err)
		}
		var logLevel slog.Level
		switch strings.ToLower(logLevelS) {
		case "debug":
			logLevel = slog.LevelDebug
		case "warn":
			logLevel = slog.LevelWarn
		case "error":
			logLevel = slog.LevelError
		default:
			logLevel = slog.LevelInfo
		}
		logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: logLevel}))

		restConfig, err := ctrl.GetConfig()
		if err != nil {
			return errors.WithStack(err)
		}
		k8sClient, err := client.New(restConfig, client.Options{})
		if err != nil {
			return errors.WithStack(err)
		}

		r := runner.NewTazunaRunner(logger, k8sClient, nil)

		f, err := os.Open(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				err = errors.Join(err, errors.WithStack(cerr))
			}
		}()

		tazuna := v1.Tazuna{}
		if err := yaml.NewDecoder(f).Decode(&tazuna); err != nil {
			return errors.WithStack(err)
		}

		atomic, err := cmd.Flags().GetBool("atomic")
		if err != nil {
			return errors.WithStack(err)
		}

		opts := runner.StateSyncOptions{
			Atomic: atomic,
		}

		if err := r.StateSync(cmd.Context(), tazuna, path, os.Stdout, opts); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	stateSyncCmd.Flags().Bool("atomic", false, "Enable atomic mode, which leaves the state untouched when any error occurs")
	stateCmd.AddCommand(stateSyncCmd)
}
