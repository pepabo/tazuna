package cmd

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
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
Parallel and GenesisSecret manifests are skipped: parallel managers do not
support Build() and GenesisSecret is always-sync by design.

Examples:
  tazuna plan -f tazuna.yaml
  tazuna plan -f tazuna.yaml --tags web,batch
  tazuna plan -f tazuna.yaml --log-level debug`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		r := runner.NewTazunaRunner(
			logger,
			k8sClient,
			&op.CommandClient{},
			runner.WithTags(tags),
		)

		tazuna, err := cliutil.LoadTazunaYAML(path)
		if err != nil {
			return err
		}

		if err := r.Plan(cmd.Context(), *tazuna, path, os.Stdout); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	addTagsFlag(planCmd, "Filter manifests by tag; only matching tags are planned")
	rootCmd.AddCommand(planCmd)
}
