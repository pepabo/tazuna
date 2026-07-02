package cmd

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/tmpl"
	"github.com/pepabo/tazuna/pkg/validator"
	"github.com/spf13/cobra"
	"sigs.k8s.io/yaml"
)

var checkCmd = &cobra.Command{
	Use:   "check",
	Short: "Validate tazuna.yaml",
	Long: `Load tazuna.yaml and run validation after expanding includes.

Checks:
  - spec.manifests[].name is required
  - spec.manifests[].name uses only allowed characters (alphanumeric, underscore, hyphen)
  - spec.manifests[].name uniqueness
  - spec.manifests[].name is not a reserved word

When --fix is specified, manifests with no name get one assigned automatically and
tazuna.yaml is written back. Because the rewrite loses YAML comments, --fix is
refused for files that use includes or Go template expressions.

Examples:
  tazuna check -f tazuna.yaml
  tazuna check -f tazuna.yaml --fix
  tazuna check -f tazuna.yaml --otlp-endpoint=localhost:4317`,
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
		fix, err := cmd.Flags().GetBool("fix")
		if err != nil {
			return errors.WithStack(err)
		}

		tazuna, err := cliutil.LoadTazunaYAML(path, cliutil.Environment(cmd))
		if err != nil {
			return err
		}

		absPath, err := filepath.Abs(path)
		if err != nil {
			return errors.WithStack(err)
		}

		if err := validator.ValidateTazunaWithBasePath(tazuna, filepath.Dir(absPath)); err != nil {
			return errors.Wrapf(err, "validation failed for tazuna.yaml at %s", path)
		}

		// -e が渡されている場合、その環境が spec.environments に宣言されているかを
		// この時点で検証しておくことで、apply/destroy 前に設定ミスを検知できる。
		if _, _, err := cliutil.ResolveContextMatches(tazuna.Spec, cliutil.Environment(cmd)); err != nil {
			return err
		}

		logger, err := cliutil.NewLogger(cmd)
		if err != nil {
			return err
		}
		r := runner.NewTazunaRunner(logger, nil, nil, runner.WithEnvironment(cliutil.Environment(cmd)))

		if fix {
			// --fix は構造体を yaml.Marshal してファイルを上書きするため、
			// Go template 式は描画結果で固定化され、コメントは失われる。
			// 意図しない破壊を防ぐため、これらを含むファイルでは拒否する。
			if err := ensureFixable(path, tazuna); err != nil {
				return errors.Wrapf(err, "check --fix cannot rewrite tazuna.yaml at %s", path)
			}

			if err := r.CheckAndFix(ctx, tazuna, absPath); err != nil {
				return errors.Wrapf(err, "check --fix failed for tazuna.yaml at %s", path)
			}

			out, err := yaml.Marshal(tazuna)
			if err != nil {
				return errors.WithStack(err)
			}
			if err := cliutil.AtomicWriteFile(path, out, 0644); err != nil {
				return errors.WithStack(err)
			}

			fmt.Printf("fixed: %s\n", path)
			return nil
		}

		if err := r.Check(ctx, tazuna, absPath); err != nil {
			return errors.Wrapf(err, "check failed for tazuna.yaml at %s", path)
		}

		fmt.Println("ok")
		return nil
	},
}

// ensureFixable は --fix による書き戻しが元ファイルを破壊しないかを検証します。
// 以下の場合はエラーを返します:
//   - includes を持つ manifest がある（展開結果のインライン化を避けられないため）
//   - Go template 式を含む（描画結果で固定化されてしまうため）
func ensureFixable(path string, tazuna *v1.Tazuna) error {
	for i := range tazuna.Spec.Manifests {
		if len(tazuna.Spec.Manifests[i].Includes) > 0 {
			return errors.Errorf("manifests[%d] uses includes; fix names in the include files directly", i)
		}
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return errors.WithStack(err)
	}
	rendered, err := tmpl.Render(path, raw, tmpl.Data{})
	if err != nil {
		return errors.WithStack(err)
	}
	if !bytes.Equal(raw, rendered) {
		return errors.New("the file contains Go template expressions that would be flattened by rewriting; assign names manually")
	}

	return nil
}

func init() {
	checkCmd.Flags().Bool("fix", false, "Auto-assign names to manifests without a name and write them back")
	rootCmd.AddCommand(checkCmd)
}
