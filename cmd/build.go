package cmd

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// buildCmd represents the build command
var buildCmd = &cobra.Command{
	Use:   "build",
	Short: "Generate the manifests that tazuna would apply",
	Long: `Build the manifests defined in tazuna.yaml and print them to stdout.
The cluster is not actually modified. Useful for previewing or debugging before apply.

Use the --tags flag to target only manifests with the specified tags.

Examples:
  tazuna build -f tazuna.yaml
  tazuna build -f tazuna.yaml --tags web`,
	RunE: func(cmd *cobra.Command, args []string) (err error) {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return err
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

		tags := []string{}
		if v, err := cmd.Flags().GetStringSlice("tags"); err == nil {
			tags = v
		}

		restConfig, err := ctrl.GetConfig()
		if err != nil {
			return errors.WithStack(err)
		}
		k8sClient, err := client.New(restConfig, client.Options{})
		if err != nil {
			return errors.WithStack(err)
		}
		orasOpts, err := buildORASPullOptions(cmd)
		if err != nil {
			return err
		}
		r := runner.NewTazunaRunner(logger, k8sClient, &op.CommandClient{}, runner.WithTags(tags), runner.WithORASPullOptions(orasOpts))

		f, err := os.Open(path)
		if err != nil {
			return errors.WithStack(err)
		}
		defer func() {
			if cerr := f.Close(); cerr != nil {
				err = errors.WithStack(cerr)
			}
		}()

		tazuna := v1.Tazuna{}
		if err := yaml.NewDecoder(f).Decode(&tazuna); err != nil {
			return errors.WithStack(err)
		}

		// tazuna.yamlのvalidation（include展開前のバリデーション）
		if err := validator.ValidateTazunaWithBasePath(&tazuna, filepath.Dir(path)); err != nil {
			return errors.Wrapf(err, "validation failed for tazuna.yaml at %s", path)
		}

		out, err := r.Build(cmd.Context(), tazuna, path)
		if err != nil {
			return errors.WithStack(err)
		}

		fmt.Println(out)
		return nil
	},
}

func init() {
	buildCmd.Flags().String("cluster-name", "kind-tazuna", "cluster name")
	buildCmd.Flags().StringSliceP("tags", "t", []string{}, "Filter manifests by tag; only matching tags are built")
	addORASPullFlags(buildCmd)
	rootCmd.AddCommand(buildCmd)
}
