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

var stateDiffCmd = &cobra.Command{
	Use:   "diff",
	Short: "Show the diff between manifests and the cluster state",
	Long: `Compare the manifests produced by each manager's Build() with the state stored in
the cluster, and display added, modified, or removed resources.

GenesisSecret resources are always shown as "always-sync" since they must be synced every time.

Examples:
  tazuna state diff
  tazuna state diff -f tazuna.yaml`,
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

		if err := r.StateDiff(cmd.Context(), tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	stateCmd.AddCommand(stateDiffCmd)
}
