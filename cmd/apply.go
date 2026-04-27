package cmd

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	tazunacontext "github.com/pepabo/tazuna/pkg/context"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// applyCmd represents the apply command
var applyCmd = &cobra.Command{
	Use:   "apply",
	Short: "Bootstrap the cluster",
	Long: `Apply the manifests defined in tazuna.yaml to the cluster in order.

Each manifest is processed in declaration order, and tests are run after the apply
when test plugins are configured. The --tags flag limits processing to manifests
that carry the specified tags.

The target cluster is determined by the kubeconfig context.
When context_matches is configured, the current context name is validated.

Examples:
  tazuna apply -f tazuna.yaml
  tazuna apply -f tazuna.yaml --tags web,batch
  tazuna apply -f tazuna.yaml --log-level debug`,
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

		if len(tazuna.Spec.ContextMatches) > 0 {
			if err := tazunacontext.ValidateCurrentContext(tazuna.Spec.ContextMatches, tazuna.Spec.ContextMatchMode); err != nil {
				return err
			}
		}

		if err := r.Apply(cmd.Context(), tazuna, path); err != nil {
			return errors.WithStack(err)
		}
		return nil
	},
}

func init() {
	applyCmd.Flags().StringSliceP("tags", "t", []string{}, "Filter manifests by tag; only matching tags are applied")
	addORASPullFlags(applyCmd)
	rootCmd.AddCommand(applyCmd)

}
