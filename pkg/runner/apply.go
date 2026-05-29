package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

	// state ConfigMapを書き込むためのstoreを準備する。
	// apply完了後にmanifest単位でstateを保存し、`state list` / `state diff` から
	// 適用済みリソースを追跡できるようにする。
	store := state.NewConfigMapStateStore(t.k8sClient)
	// state ConfigMapはtazuna namespace配下に作成されるため、事前に存在保証する
	if err := state.EnsureNamespace(ctx, t.k8sClient); err != nil {
		return errors.Wrapf(err, "failed to ensure tazuna namespace")
	}
	gitCommit := getGitCommitHash(ctx)

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
		if err := t.ApplyManifest(ctx, m, managers, testPlugins, store, gitCommit); err != nil {
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
	m v1.Manifest,
	managers map[string]manager.Manager,
	testPlugins map[string]testplugin.Plugin,
	store state.StateStore,
	gitCommit string,
) error {
	mgr, ok := managers[string(m.Type)]
	if !ok {
		return fmt.Errorf("unsupported type: %s", m.Type)
	}
	t.logger.DebugContext(ctx, "apply with manager", slog.String("type", string(m.Type)), slog.String("manifest", m.Path))

	startMsg := fmt.Sprintf("[%s]", m.Type)

	if m.Name != "" {
		startMsg += " " + m.Name
	}
	if m.Description != "" {
		startMsg += " - " + m.Description
	}
	t.logger.InfoContext(ctx, startMsg)
	if err := mgr.Apply(ctx, t.logger, m); err != nil {
		return errors.WithStack(err)
	}

	for _, tc := range m.Tests {
		t.logger.DebugContext(ctx, "validate cluster with test plugin", slog.String("type", string(tc.Type)))
		plug, ok := testPlugins[string(tc.Type)]
		if !ok {
			return fmt.Errorf("unsupported type: %s", tc.Type)
		}

		if err := testplugin.Start(ctx, t.logger, &tc, plug.Run); err != nil {
			return errors.WithStack(err)
		}
	}

	// マニフェストの適用とテスト実行が成功した後にstateを保存する。
	// state_sync.go と同等の構造でmanifest単位のContent Hashを記録する。
	if err := t.saveManifestState(ctx, m, mgr, store, gitCommit); err != nil {
		return errors.WithStack(err)
	}

	t.logger.InfoContext(ctx, startMsg+" ... done")
	return nil
}

// saveManifestState は適用済みmanifestのstateをConfigMapに保存する。
// state_sync.go の保存処理と同じロジックで currentEntries を作成し、
// `state list` / `state diff` から drift を可視化できるようにする。
func (t *TazunaRunner) saveManifestState(
	ctx context.Context,
	m v1.Manifest,
	mgr manager.Manager,
	store state.StateStore,
	gitCommit string,
) error {
	// manifest名が未設定のものはstate keyを作れないためスキップする
	// (state_sync.go と同等の振る舞い)
	if m.Name == "" {
		t.logger.WarnContext(ctx, "manifest has no name, skipping state save", slog.String("type", string(m.Type)))
		return nil
	}

	// parallel マニフェストは現状 state 対応していないためスキップする
	// (state_sync.go と同等の振る舞い)
	if m.Type == v1.ManifestTypeParallel {
		t.logger.WarnContext(ctx, "parallel manifest is not supported for state save, skipping", slog.String("name", m.Name))
		return nil
	}

	out, err := mgr.Build(ctx, t.logger, m)
	if err != nil {
		t.logger.WarnContext(ctx, "failed to build manifest for state save", slog.String("name", m.Name), slog.String("error", err.Error()))
		return errors.Wrapf(err, "failed to build manifest %q for state save", m.Name)
	}

	defaultNs := getDefaultNamespace(m)
	objects, err := manifest.ConvertManifestsToObjects([]byte(out), defaultNs)
	if err != nil {
		t.logger.WarnContext(ctx, "failed to convert manifests to objects for state save", slog.String("name", m.Name), slog.String("error", err.Error()))
		return errors.Wrapf(err, "failed to convert manifests to objects for %q", m.Name)
	}

	currentEntries := make(map[string]state.StateEntry, len(objects))
	for _, obj := range objects {
		key := state.NewStateKey(m.Name, obj)
		uns, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		hash, err := state.ComputeContentHash(uns)
		if err != nil {
			t.logger.WarnContext(ctx, "failed to compute content hash for state save", slog.String("key", key.String()), slog.String("error", err.Error()))
			return errors.Wrapf(err, "failed to compute content hash for %s", key.String())
		}
		currentEntries[key.String()] = state.StateEntry{ContentHash: hash}
	}

	stateData := &state.StateData{
		Metadata: state.StateMetadata{
			LastSyncedAt:  time.Now().UTC().Format(time.RFC3339),
			GitCommitHash: gitCommit,
		},
		Entries: currentEntries,
	}

	if err := store.Save(ctx, m.Name, stateData); err != nil {
		t.logger.WarnContext(ctx, "failed to save state", slog.String("name", m.Name), slog.String("error", err.Error()))
		return errors.Wrapf(err, "failed to save state for manifest %q", m.Name)
	}

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
