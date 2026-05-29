package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Generate a starter tazuna.yaml",
	Long: `Generate a minimal includes-based tazuna.yaml skeleton.

The generated file pins spec.minimumSupportedTazunaVersion to the version of the
tazuna binary that created it, so older binaries refuse to process it. The
manifests list starts empty; fill it with includes entries that point at the
per-component tazuna.yaml files you want to compose.

By default the command refuses to overwrite an existing file. Pass --force to
overwrite it.

Examples:
  tazuna init
  tazuna init -f infra/tazuna.yaml
  tazuna init --force`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path, err := cmd.Flags().GetString("file-path")
		if err != nil {
			return errors.WithStack(err)
		}
		force, err := cmd.Flags().GetBool("force")
		if err != nil {
			return errors.WithStack(err)
		}

		if !force {
			if _, statErr := os.Stat(path); statErr == nil {
				return errors.Errorf("%s already exists; pass --force to overwrite", path)
			} else if !os.IsNotExist(statErr) {
				return errors.WithStack(statErr)
			}
		}

		content := renderInitTemplate(initialMinimumSupportedVersion(versionString))
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			return errors.WithStack(err)
		}

		_, err = fmt.Fprintf(cmd.OutOrStdout(), "created: %s\n", path)
		return err
	},
}

// initialMinimumSupportedVersion は init が埋め込む minimumSupportedTazunaVersion を
// 決めます。実行中の tazuna が semver なら正規化した値を、dev ビルドなど semver で
// なければプレースホルダ "0.0.0" を返します。
func initialMinimumSupportedVersion(current string) string {
	if v, err := semver.NewVersion(strings.TrimSpace(current)); err == nil {
		return v.String()
	}
	return "0.0.0"
}

// renderInitTemplate は init が書き出す tazuna.yaml の中身を組み立てます。
func renderInitTemplate(minVersion string) string {
	return fmt.Sprintf(`apiVersion: %s
kind: %s
spec:
  # この tazuna.yaml を処理するのに必要な tazuna の最小バージョン (semver) です。
  # これを下回る tazuna バイナリは、誤適用を防ぐためエラーで終了します。
  minimumSupportedTazunaVersion: "%s"
  # includes で各コンポーネントの tazuna.yaml を読み込みます。
  # 下の例のように manifests に includes エントリを追加してください。
  #
  #   manifests:
  #     - name: infra
  #       includes:
  #         - path: ./infra/tazuna.yaml
  #         - path: ./addons/tazuna.yaml
  manifests: []
`, v1.TazunaAPIVersion, v1.TazunaKind, minVersion)
}

func init() {
	initCmd.Flags().Bool("force", false, "Overwrite the target file if it already exists")
	rootCmd.AddCommand(initCmd)
}
