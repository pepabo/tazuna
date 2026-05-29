package runner

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"slices"
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

	// manifest nameのバリデーション警告（移行期間のためエラーにはしない）
	t.warnManifestNameValidation(ctx, tazuna)

	// tazuna.yamlはマニフェストパスがtazuna.yamlからの相対パスで渡されているので、
	// それをcwdからのパスに変換する
	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)
	t.providersBaseDir = baseDir

	managers, err := setupManagers(t.k8sClient, t.opClient, t.orasPullOpts, tazuna.Spec.Providers, t.providersBaseDir)
	if err != nil {
		return "", errors.Wrap(err, "failed to setup managers")
	}

	var allOutputs []string

	for _, manifest := range tazuna.Spec.Manifests {
		// タグフィルタリングのチェック
		if len(t.tags) > 0 {
			found := false
			for _, tag := range t.tags {
				found = found || slices.Contains(manifest.Tags, tag)
			}

			if !found {
				t.logger.InfoContext(ctx, "skip manifest due to tags filter", slog.String("manifest-tags", strings.Join(manifest.Tags, ",")), slog.String("filter-tags", strings.Join(t.tags, ",")))
				continue
			}
		}

		manager, ok := managers[string(manifest.Type)]
		if !ok {
			return "", fmt.Errorf("manager %s not found", manifest.Type)
		}

		out, err := manager.Build(ctx, t.logger, manifest)
		if err != nil {
			return "", errors.WithStack(err)
		}

		if out != "" {
			allOutputs = append(allOutputs, out)
		}
	}

	return strings.Join(allOutputs, "\n---\n"), nil

}
