// Package cliutil collects boilerplate shared across cobra commands in cmd/.
// Each helper here is deliberately small and orthogonal so that RunE bodies can
// focus on command-specific orchestration rather than plumbing.
package cliutil

import (
	"log/slog"
	"os"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/tmpl"
	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// currentVersion は実行中の tazuna バイナリのバージョンです。
// main から SetVersionInfo 経由で SetCurrentVersion により注入されます。
// 注入されない場合（go test / go run など）はデフォルトの "dev" のままとなり、
// minimumSupportedTazunaVersion との比較はスキップされます。
var currentVersion = "dev"

// SetCurrentVersion は実行中の tazuna のバージョンを記録します。
// cmd.SetVersionInfo から呼ばれることを想定しています。
func SetCurrentVersion(v string) {
	if v != "" {
		currentVersion = v
	}
}

// ParseLogLevel maps the textual log level used by Tazuna's --log-level flag to
// a slog.Level. Unknown values fall back to slog.LevelInfo.
func ParseLogLevel(s string) slog.Level {
	switch strings.ToLower(s) {
	case "debug":
		return slog.LevelDebug
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// NewLogger reads the persistent --log-level flag and returns a slog.Logger
// that writes to stderr.
func NewLogger(cmd *cobra.Command) (*slog.Logger, error) {
	logLevelS, err := cmd.Flags().GetString("log-level")
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: ParseLogLevel(logLevelS)})), nil
}

// LoadTazunaYAML reads path, renders it as a Go template, and decodes it as a
// v1.Tazuna document.
//
// environment は -e/--environment フラグの値で、テンプレート内の {{ .Environment }}
// に注入されます。フラグが渡されていない場合は空文字列を渡してください。
func LoadTazunaYAML(path, environment string) (*v1.Tazuna, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	rendered, err := tmpl.Render(path, data, tmpl.Data{Environment: environment})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	tazuna := v1.Tazuna{}
	if err := yaml.Unmarshal(rendered, &tazuna); err != nil {
		return nil, errors.WithStack(err)
	}
	if err := CheckMinimumSupportedVersion(&tazuna, currentVersion); err != nil {
		return nil, errors.WithStack(err)
	}
	return &tazuna, nil
}

// ResolveContextMatches は environment に応じて有効な context_matches パターンと
// 評価モードを返します。
//
//   - environment が空文字列の場合、ルート直下の spec.context_matches /
//     spec.context_match_mode をそのまま返します。
//   - environment が指定された場合、spec.environments[environment] の値を使います。
//     該当する環境が宣言されていなければエラーになります。環境の context_match_mode が
//     空なら、ルート直下の context_match_mode を継承します。
func ResolveContextMatches(spec v1.TazunaSpec, environment string) ([]string, v1.ContextMatchMode, error) {
	if environment == "" {
		return spec.ContextMatches, spec.ContextMatchMode, nil
	}

	env, ok := spec.Environments[environment]
	if !ok {
		return nil, "", errors.Errorf(
			"environment %q is not declared under spec.environments", environment)
	}

	mode := env.ContextMatchMode
	if mode == "" {
		mode = spec.ContextMatchMode
	}
	return env.ContextMatches, mode, nil
}

// Environment は永続フラグ -e/--environment の値を読み取ります。フラグが未登録の
// サブコマンドでは空文字列を返します。
func Environment(cmd *cobra.Command) string {
	env, err := cmd.Flags().GetString("environment")
	if err != nil {
		return ""
	}
	return env
}

// CheckMinimumSupportedVersion は spec.minimumSupportedTazunaVersion と実行中の
// tazuna バージョン current を比較します。current が制約を満たさない場合はエラーを
// 返します。
//
//   - minimumSupportedTazunaVersion が未指定なら制約なしとして nil を返します。
//   - minimumSupportedTazunaVersion が semver として不正なら設定エラーを返します。
//   - current が semver としてパースできない場合（"dev" などのローカルビルド）は
//     比較をスキップして nil を返します。これによりローカル開発がブロックされません。
func CheckMinimumSupportedVersion(tazuna *v1.Tazuna, current string) error {
	if tazuna == nil {
		return errors.New("tazuna is nil")
	}

	minRaw := strings.TrimSpace(tazuna.Spec.MinimumSupportedTazunaVersion)
	if minRaw == "" {
		return nil
	}

	minVersion, err := semver.NewVersion(minRaw)
	if err != nil {
		return errors.Errorf("spec.minimumSupportedTazunaVersion %q is not a valid semver: %v", minRaw, err)
	}

	curVersion, err := semver.NewVersion(strings.TrimSpace(current))
	if err != nil {
		// dev ビルドなど semver でないバージョンは比較できないためゲートをスキップする
		return nil
	}

	if curVersion.LessThan(minVersion) {
		return errors.Errorf(
			"this tazuna.yaml requires tazuna >= %s, but the running tazuna is %s; please upgrade tazuna",
			minVersion, current,
		)
	}

	return nil
}

// NewK8sClient builds a controller-runtime client from the ambient kubeconfig.
func NewK8sClient() (client.Client, error) {
	restConfig, err := ctrl.GetConfig()
	if err != nil {
		return nil, errors.WithStack(err)
	}
	c, err := client.New(restConfig, client.Options{})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return c, nil
}
