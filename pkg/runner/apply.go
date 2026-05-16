package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"sigs.k8s.io/yaml"
)

func (t *TazunaRunner) Apply(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
) error {
	// includesフィールドがあるマニフェストを展開する
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return errors.WithStack(err)
	}

	// manifest nameのバリデーション警告（移行期間のためエラーにはしない）
	t.warnManifestNameValidation(ctx, tazuna)

	// tazuna.yamlはマニフェストパスがtazuna.yamlからの相対パスで渡されているので、
	// それをcwdからのパスに変換する
	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)

	if err := t.ApplyToCluster(ctx, tazuna); err != nil {
		return errors.WithStack(err)
	}

	return nil

}

func (t *TazunaRunner) ApplyToCluster(
	ctx context.Context,
	tazuna v1.Tazuna,
) error {
	// switchを書かずに処理を分けるためmapにmanagerを詰める
	managers := setupManagers(t.k8sClient, t.opClient, t.orasPullOpts)
	testPlugins := setupTestPlugins(t.k8sClient)

	for _, m := range tazuna.Spec.Manifests {
		if len(t.tags) > 0 {
			// タグが指定されている場合は、tagsに含まれるもののみを適用する
			found := false
			for _, tag := range t.tags {
				found = found || slices.Contains(m.Tags, tag)
			}

			if !found {
				t.logger.InfoContext(ctx, "skip manifest due to tags filter", slog.String("manifest-tags", strings.Join(m.Tags, ",")), slog.String("filter-tags", strings.Join(t.tags, ",")))
				continue
			}
		}
		if err := t.ApplyManifest(ctx, m, managers, testPlugins); err != nil {
			return errors.WithStack(err)
		}
	}

	// すべてのマニフェスト生成終了後もテストを定義できるようにする
	for _, tc := range tazuna.Spec.Tests {
		t.logger.DebugContext(ctx, "validate cluster with test plugin", slog.String("type", string(tc.Type)))
		plug, ok := testPlugins[string(tc.Type)]
		if !ok {
			return fmt.Errorf("unsupported type: %s", tc.Type)
		}

		if err := testplugin.Start(ctx, t.logger, &tc, plug.Run); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

func (t *TazunaRunner) ApplyManifest(
	ctx context.Context,
	manifest v1.Manifest,
	managers map[string]manager.Manager,
	testPlugins map[string]testplugin.Plugin,
) error {
	mgr, ok := managers[string(manifest.Type)]
	if !ok {
		return fmt.Errorf("unsupported type: %s", manifest.Type)
	}
	t.logger.DebugContext(ctx, "apply with manager", slog.String("type", string(manifest.Type)), slog.String("manifest", manifest.Path))

	startMsg := fmt.Sprintf("[%s]", manifest.Type)

	if manifest.Name != "" {
		startMsg += " " + manifest.Name
	}
	if manifest.Description != "" {
		startMsg += " - " + manifest.Description
	}
	t.logger.InfoContext(ctx, startMsg)
	if err := mgr.Apply(ctx, t.logger, manifest); err != nil {
		return errors.WithStack(err)
	}

	for _, tc := range manifest.Tests {
		t.logger.DebugContext(ctx, "validate cluster with test plugin", slog.String("type", string(tc.Type)))
		plug, ok := testPlugins[string(tc.Type)]
		if !ok {
			return fmt.Errorf("unsupported type: %s", tc.Type)
		}

		if err := testplugin.Start(ctx, t.logger, &tc, plug.Run); err != nil {
			return errors.WithStack(err)
		}
	}

	t.logger.InfoContext(ctx, startMsg+" ... done")
	return nil
}

// expandIncludes はincludesフィールドを持つマニフェストを展開します
func (t *TazunaRunner) expandIncludes(ctx context.Context, tazuna *v1.Tazuna, tazunaYAMLPath string) error {
	baseDir := filepath.Dir(tazunaYAMLPath)
	var expandedManifests []v1.Manifest

	for _, manifest := range tazuna.Spec.Manifests {
		if len(manifest.Includes) > 0 {
			// includesが指定されている場合、includeファイルを展開する
			t.logger.DebugContext(ctx, "expanding includes", slog.Int("includeFiles", len(manifest.Includes)))

			for _, include := range manifest.Includes {
				includePath := filepath.Join(baseDir, include.Path)

				// includeファイルを読み込み
				includeData, err := os.ReadFile(includePath)
				if err != nil {
					return errors.Wrapf(err, "failed to open include file: %s", includePath)
				}

				// includeファイルをパースして完全なTazuna構造として読み込む
				var includeTazuna v1.Tazuna
				if err := yaml.Unmarshal(includeData, &includeTazuna); err != nil {
					return errors.Wrapf(err, "failed to parse include file: %s", includePath)
				}

				// includeファイル内でincludesフィールドが使われていないことを確認
				for _, includeManifest := range includeTazuna.Spec.Manifests {
					if len(includeManifest.Includes) > 0 {
						return errors.Errorf("nested includes are not allowed in include file: %s", includePath)
					}
				}

				// includeファイルのマニフェストをメインの配列に追加
				expandedManifests = append(expandedManifests, includeTazuna.Spec.Manifests...)
			}
		} else {
			// includesが指定されていない場合、そのままマニフェストを追加
			expandedManifests = append(expandedManifests, manifest)
		}
	}

	// 展開されたマニフェスト配列で置き換える
	tazuna.Spec.Manifests = expandedManifests
	return nil
}
