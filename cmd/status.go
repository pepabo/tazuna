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
  tazuna status -f tazuna.yaml`,
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

		if err := r.Status(cmd.Context(), *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(statusCmd)
}
