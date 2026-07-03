package runner

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

func (t *TazunaRunner) Build(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
) (string, error) {
	// includesフィールドがあるマニフェストを展開する
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return "", errors.WithStack(err)
	}

	// include展開後にもvalidationを実行し、include先ファイルのtypo等を検知する
	if err := validateExpandedSpec(&tazuna, tazunaYAMLPath); err != nil {
		return "", err
	}

	// manifest nameのバリデーション警告（移行期間のためエラーにはしない）
	t.warnManifestNameValidation(ctx, tazuna)

	// tazuna.yamlはマニフェストパスがtazuna.yamlからの相対パスで渡されているので、
	// それをcwdからのパスに変換する
	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)
	t.providersBaseDir = baseDir

	managers, err := setupManagers(t.k8sClient, t.opClient, t.orasPullOpts, tazuna.Spec.Providers, t.providersBaseDir, t.environment)
	if err != nil {
		return "", errors.Wrap(err, "failed to setup managers")
	}

	var allOutputs []string

	for _, m := range tazuna.Spec.Manifests {
		// タグフィルタリングのチェック
		if !matchesTags(m, t.tags) {
			t.logger.InfoContext(ctx, "skip manifest due to tags filter", slog.String("manifest-tags", strings.Join(m.Tags, ",")), slog.String("filter-tags", strings.Join(t.tags, ",")))
			continue
		}

		mgr, ok := managers[string(m.Type)]
		if !ok {
			return "", fmt.Errorf("manager %s not found", m.Type)
		}

		out, err := mgr.Build(ctx, t.logger, m)
		if err != nil {
			return "", errors.WithStack(err)
		}

		if out != "" {
			allOutputs = append(allOutputs, out)
		}
	}

	return strings.Join(allOutputs, "\n---\n"), nil

}
