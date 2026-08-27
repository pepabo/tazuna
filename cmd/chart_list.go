package cmd

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"text/tabwriter"

	"github.com/cockroachdb/errors"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/spf13/cobra"
)

var chartListCmd = &cobra.Command{
	Use:   "list",
	Short: "List helm charts used by tazuna-managed releases",
	Long: `List helm charts referenced by helmfile manifests in tazuna.yaml.

Each row shows the chart reference, the release name that uses it, the
helmfile.yaml path where it is declared, and the pinned chart version.

When --check-latest is specified, the latest version available in the chart
repository (HTTP(S) helm repository or OCI registry) is looked up and shown
as an additional column. This can take several seconds because it downloads
the full repository index / lists OCI tags for each unique chart source.
Local charts and charts whose repository is not declared show "-" as the
latest version.

Examples:
  tazuna chart list
  tazuna chart list -f tazuna.yaml
  tazuna chart list --check-latest`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()

		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
		}

		checkLatest, err := cmd.Flags().GetBool("check-latest")
		if err != nil {
			return errors.WithStack(err)
		}

		logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
		if l, err := cliutil.NewLogger(cmd); err == nil {
			logger = l
		}

		tazuna, err := cliutil.LoadTazunaYAML(path, cliutil.Environment(cmd))
		if err != nil {
			return err
		}

		r := runner.NewTazunaRunner(logger, nil, nil, runner.WithEnvironment(cliutil.Environment(cmd)))

		items, err := r.ChartList(ctx, *tazuna, path, checkLatest)
		if err != nil {
			return errors.WithStack(err)
		}

		return printChartList(cmd.OutOrStdout(), items, checkLatest)
	},
}

func printChartList(w io.Writer, items []runner.ChartListItem, checkedLatest bool) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if checkedLatest {
		if _, err := fmt.Fprintln(tw, "CHART\tRELEASE\tFILE\tVERSION\tLATEST"); err != nil {
			return errors.WithStack(err)
		}
	} else {
		if _, err := fmt.Fprintln(tw, "CHART\tRELEASE\tFILE\tVERSION"); err != nil {
			return errors.WithStack(err)
		}
	}

	for _, it := range items {
		chart := it.Chart
		if chart == "" {
			chart = "-"
		}
		version := it.Version
		if version == "" {
			version = "-"
		}
		if checkedLatest {
			latest := it.LatestVersion
			if latest == "" {
				latest = "-"
			}
			if it.LatestErr != nil {
				latest = "(error)"
				fmt.Fprintf(os.Stderr, "warn: failed to resolve latest for release %q: %v\n", it.Release, it.LatestErr)
			}
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", chart, it.Release, it.FilePath, version, latest); err != nil {
				return errors.WithStack(err)
			}
		} else {
			if _, err := fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", chart, it.Release, it.FilePath, version); err != nil {
				return errors.WithStack(err)
			}
		}
	}

	return tw.Flush()
}

func init() {
	chartListCmd.Flags().Bool("check-latest", false, "Look up and show the latest version of each chart from its repository")
	chartCmd.AddCommand(chartListCmd)
}
