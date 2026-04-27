package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	tazunacontext "github.com/pepabo/tazuna/pkg/context"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/prompt"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// destroyCmd represents the build command
var destroyCmd = &cobra.Command{
	Use:   "destroy",
	Short: "Delete tazuna-managed resources",
	Long: `Delete the resources managed by tazuna.yaml from the cluster.

Two safety guards are in place:
  1. A confirmation prompt is shown before execution (skip with --force)
  2. The command runs only when the environment variable TAZUNA_DESTROY_EXECUTABLE=true is set

Use the --tags flag to target only manifests with the specified tags.

Examples:
  TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy -f tazuna.yaml
  TAZUNA_DESTROY_EXECUTABLE=true tazuna destroy -f tazuna.yaml --force`,
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
		tags := []string{}
		if v, err := cmd.Flags().GetStringSlice("tags"); err == nil {
			tags = v
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

		if len(tazuna.Spec.ContextMatches) > 0 {
			if err := tazunacontext.ValidateCurrentContext(tazuna.Spec.ContextMatches, tazuna.Spec.ContextMatchMode); err != nil {
				return err
			}
		}

		if v, err := cmd.Flags().GetBool("force"); err == nil && !v {
			ok, err := prompt.YesORNo(os.Stdin, "!!! All resources managed by Tazuna will be deleted !!!\nAre you sure you want to delete them?")
			if err != nil {
				return errors.WithStack(err)
			}

			if !ok {
				return nil
			}
		}

		if v, err := strconv.ParseBool(os.Getenv("TAZUNA_DESTROY_EXECUTABLE")); err != nil || !v {
			logger.Error("TAZUNA_DESTROY_EXECUTABLE is false, skipping destroy")
			return nil
		}

		if err := r.Destroy(cmd.Context(), tazuna, path); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	destroyCmd.Flags().Bool("force", false, "Delete without confirmation")
	destroyCmd.Flags().StringSliceP("tags", "t", []string{}, "Filter manifests by tag; only matching tags are destroyed")
	addORASPullFlags(destroyCmd)
	rootCmd.AddCommand(destroyCmd)
}
