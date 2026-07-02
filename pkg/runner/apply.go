package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/resource"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"github.com/pepabo/tazuna/pkg/tmpl"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// runnerTracerName is the OpenTelemetry tracer name used by all TazunaRunner
// top-level spans. Combined with managerTracerName ("tazuna/manager") this
// builds a 3-level trace tree:  tazuna.* (runner) -> Kustomize/Helmfile.* etc.
const runnerTracerName = "tazuna/runner"

// recordRunnerSpanErr marks span as failed with err and records it.
// pkg/manager 側の recordSpanError と同一処理だが、循環依存を避けるため別実装。
func recordRunnerSpanErr(span trace.Span, err error) {
	if err == nil {
		return
	}
	span.RecordError(err)
	span.SetStatus(codes.Error, err.Error())
}

func (t *TazunaRunner) Apply(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
) (retErr error) {
	ctx, span := otel.Tracer(runnerTracerName).Start(ctx, "tazuna.Apply",
		trace.WithAttributes(
			attribute.String("tazuna.yaml.path", tazunaYAMLPath),
			attribute.Bool("apply.sync", t.applyOpts.Sync),
			attribute.Bool("apply.prune", t.applyOpts.Prune),
			attribute.Bool("apply.atomic", t.applyOpts.Atomic),
			attribute.Int("manifests.count", len(tazuna.Spec.Manifests)),
		))
	defer func() {
		recordRunnerSpanErr(span, retErr)
		span.End()
	}()

	// --prune は --sync 必須。Runner 層でもガードしておくことで、
	// CLI 以外から呼ばれた場合 (テスト等) の安全策にする。
	if t.applyOpts.Prune && !t.applyOpts.Sync {
		return errors.New("prune requires sync mode")
	}
	if t.applyOpts.Atomic && !t.applyOpts.Sync {
		return errors.New("atomic requires sync mode")
	}

	// includesフィールドがあるマニフェストを展開する
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return errors.WithStack(err)
	}

	// include展開後にもvalidationを実行し、include先ファイルのtypo等を検知する
	if err := validateExpandedSpec(&tazuna, tazunaYAMLPath); err != nil {
		return err
	}

	// manifest nameのバリデーション。--sync / dependsOn 使用時は不正名が
	// state の上書きや誤 prune・誤った依存解決につながるためエラーに昇格する。
	// それ以外は移行期間のため警告に留める。
	strictNames := t.applyOpts.Sync || anyDependsOn(tazuna.Spec.Manifests)
	if err := t.validateManifestNames(ctx, tazuna, strictNames); err != nil {
		return err
	}

	// tazuna.yamlはマニフェストパスがtazuna.yamlからの相対パスで渡されているので、
	// それをcwdからのパスに変換する
	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)
	// providersBaseDir は envfile provider の相対 path 解決に使う
	t.providersBaseDir = baseDir

	if err := t.ApplyToCluster(ctx, tazuna); err != nil {
		return errors.WithStack(err)
	}

	return nil

}

func (t *TazunaRunner) ApplyToCluster(
	ctx context.Context,
	tazuna v1.Tazuna,
) (retErr error) {
	ctx, span := otel.Tracer(runnerTracerName).Start(ctx, "tazuna.ApplyToCluster",
		trace.WithAttributes(
			attribute.Int("manifests.count", len(tazuna.Spec.Manifests)),
		))
	defer func() {
		recordRunnerSpanErr(span, retErr)
		span.End()
	}()

	// switchを書かずに処理を分けるためmapにmanagerを詰める。
	// テストから WithManagersOverride で差し替えられた場合はそちらを優先する。
	var managers map[string]manager.Manager
	if t.managersOverride != nil {
		managers = t.managersOverride
	} else {
		m, err := setupManagers(t.k8sClient, t.opClient, t.orasPullOpts, tazuna.Spec.Providers, t.providersBaseDir, t.environment)
		if err != nil {
			return errors.Wrap(err, "failed to setup managers")
		}
		managers = m
	}
	testPlugins := setupTestPlugins(t.k8sClient)

	// state ConfigMapを書き込むためのstoreを準備する。
	// apply完了後にmanifest単位でstateを保存し、`state list` / `state diff` から
	// 適用済みリソースを追跡できるようにする。
	// state ConfigMapはtazuna namespace配下に作成されるが、namespaceの存在保証は
	// ConfigMapStateStore.Save 側で行うため、ここでは明示的に ensure しない。
	store := state.NewConfigMapStateStore(t.k8sClient)
	// providersBaseDir は tazuna.yaml のディレクトリ (Apply の入口で設定済み)。
	// cwd ではなく tazuna.yaml 側のリポジトリの commit hash を記録する。
	gitCommit := getGitCommitHash(ctx, t.providersBaseDir)

	// sync モードでは atomic 時の state を一旦バッファし、全 manifest 処理後に保存する。
	// DAG モードでは同一層内のマニフェストが並列に書き込むため mutex で保護する。
	pendingSaves := make(map[string]*state.StateData)
	var pendingSavesMu sync.Mutex

	// dependsOn によるトポロジカル順序で manifests を層に分割する。
	// dependsOn が一切使われていなければ層数 = manifest 数となり、従来の宣言順
	// 順次実行と完全に同じ挙動になるため後方互換性が保たれる。
	layers, err := ResolveDependencyOrder(tazuna.Spec.Manifests)
	if err != nil {
		return errors.Wrap(err, "failed to resolve manifest dependency order")
	}

	for layerIdx, layer := range layers {
		// 各層内のマニフェストを並列実行する。1 マニフェストしか入っていない層
		// (= dependsOn 未使用時の従来挙動) でも同じコードパスを通すことで挙動を
		// シンプルに保つ。
		errCh := make(chan error, len(layer))
		var wg sync.WaitGroup
		for _, m := range layer {
			// タグが指定されている場合は、tagsに含まれるもののみを適用する
			if !matchesTags(m, t.tags) {
				t.logger.InfoContext(ctx, "skip manifest due to tags filter", slog.String("manifest-tags", strings.Join(m.Tags, ",")), slog.String("filter-tags", strings.Join(t.tags, ",")))
				continue
			}

			wg.Add(1)
			go func(m v1.Manifest) {
				defer wg.Done()
				if t.applyOpts.Sync {
					if err := t.SyncManifest(ctx, m, managers, testPlugins, store, gitCommit, pendingSaves, &pendingSavesMu); err != nil {
						errCh <- errors.WithStack(err)
					}
				} else {
					if err := t.ApplyManifest(ctx, m, managers, testPlugins, store, gitCommit); err != nil {
						errCh <- errors.WithStack(err)
					}
				}
			}(m)
		}

		// 層内の全 goroutine が完了するのを待ってからエラーを集約する。
		// 途中で goroutine を cancel するところまではしない (タスク要件)。
		wg.Wait()
		close(errCh)

		var errs []error
		for e := range errCh {
			errs = append(errs, e)
		}
		if len(errs) > 0 {
			return errors.Wrapf(errors.Join(errs...), "layer %d apply failed", layerIdx)
		}
	}

	// atomicモード時はここで一括保存
	if t.applyOpts.Sync && t.applyOpts.Atomic {
		for name, sd := range pendingSaves {
			if err := store.Save(ctx, name, sd); err != nil {
				return errors.Wrapf(err, "failed to save state for manifest %q", name)
			}
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
	// Apply は適用対象となった render 済みオブジェクトを返す。これを state hash 計算に
	// 再利用することで、saveManifestState 内で Build() を再度呼ぶ二重 render を避ける。
	objects, err := mgr.Apply(ctx, t.logger, m)
	if err != nil {
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
	// Apply が返した render 済みオブジェクトをそのまま使い、manifest単位のContent Hashを記録する。
	if err := t.saveManifestState(ctx, m, objects, store, gitCommit); err != nil {
		return errors.WithStack(err)
	}

	t.logger.InfoContext(ctx, startMsg+" ... done")
	return nil
}

// SyncManifest は --sync モードでの manifest 適用処理。
// Build() で生成したマニフェストと既存 state を比較し、差分のあるリソースのみを
// CreateOrUpdate する。Prune が有効ならば removed リソースを Delete する。
// テストは差分の有無に関わらず実行する (旧 state sync の挙動を維持)。
// Atomic が有効な場合、state 保存は pendingSaves に詰めるだけにし、
// 呼び出し元の ApplyToCluster で全 manifest 処理後にまとめて保存される。
func (t *TazunaRunner) SyncManifest(
	ctx context.Context,
	m v1.Manifest,
	managers map[string]manager.Manager,
	testPlugins map[string]testplugin.Plugin,
	store state.StateStore,
	gitCommit string,
	pendingSaves map[string]*state.StateData,
	pendingSavesMu *sync.Mutex,
) error {
	// manifest名が未設定のものは state key を作れないためスキップする
	if m.Name == "" {
		t.logger.WarnContext(ctx, "manifest has no name, skipping state sync", slog.String("type", string(m.Type)))
		return nil
	}

	mgr, ok := managers[string(m.Type)]
	if !ok {
		return fmt.Errorf("manager %s not found", m.Type)
	}

	out, err := mgr.Build(ctx, t.logger, m)
	if err != nil {
		return errors.Wrapf(err, "failed to build manifest %q", m.Name)
	}

	defaultNs := getDefaultNamespace(m)
	objects, err := manifest.ConvertManifestsToObjects([]byte(out), defaultNs)
	if err != nil {
		return errors.Wrapf(err, "failed to convert manifests to objects for %q", m.Name)
	}

	// 現在のエントリとオブジェクトのマッピングを構築
	currentEntries := make(map[string]state.StateEntry, len(objects))
	objectsByKey := make(map[string]client.Object, len(objects))
	for _, obj := range objects {
		key := state.NewStateKey(m.Name, obj)
		uns, ok := obj.(*unstructured.Unstructured)
		if !ok {
			continue
		}
		hash, err := state.ComputeContentHash(uns)
		if err != nil {
			return errors.Wrapf(err, "failed to compute content hash for %s", key.String())
		}
		currentEntries[key.String()] = state.StateEntry{ContentHash: hash}
		objectsByKey[key.String()] = obj
	}

	// 既存ステートを取得
	stateData, err := store.Get(ctx, m.Name)
	if err != nil {
		return errors.Wrapf(err, "failed to get state for manifest %q", m.Name)
	}

	// GenesisSecretの場合、全キーをalways-syncとして扱う
	var alwaysSyncKeys map[string]bool
	if m.Type == v1.ManifestTypeGenesisSecret {
		alwaysSyncKeys = make(map[string]bool, len(currentEntries))
		for key := range currentEntries {
			alwaysSyncKeys[key] = true
		}
	}

	diffEntries := state.ComputeDiff(stateData, currentEntries, alwaysSyncKeys)

	if len(diffEntries) > 0 {
		// 新しいステートデータを構築（既存エントリをコピー）
		newEntries := make(map[string]state.StateEntry, len(stateData.Entries))
		for k, v := range stateData.Entries {
			newEntries[k] = v
		}

		for _, entry := range diffEntries {
			resourceDisplay := formatDiffResourceKey(entry.Key)

			switch entry.DiffType {
			case state.DiffTypeAdded, state.DiffTypeModified, state.DiffTypeAlwaysSync:
				obj, ok := objectsByKey[entry.Key]
				if !ok {
					return fmt.Errorf("object not found for key %s", entry.Key)
				}
				if err := resource.CreateOrUpdateForObject(ctx, t.k8sClient, obj); err != nil {
					return errors.Wrapf(err, "failed to apply resource %s", resourceDisplay)
				}
				t.logger.InfoContext(ctx, "synced resource",
					slog.String("manifest", m.Name),
					slog.String("diff", string(entry.DiffType)),
					slog.String("resource", resourceDisplay))
				newEntries[entry.Key] = state.StateEntry{ContentHash: entry.NewHash}

			case state.DiffTypeRemoved:
				if t.applyOpts.Prune {
					// StateKeyからunstructuredオブジェクトを構築して削除
					obj, err := buildUnstructuredFromStateKey(entry.Key)
					if err != nil {
						return errors.Wrapf(err, "failed to build object from state key %s", entry.Key)
					}
					if err := resource.DeleteObject(ctx, t.k8sClient, obj); err != nil {
						return errors.Wrapf(err, "failed to delete resource %s", resourceDisplay)
					}
					t.logger.InfoContext(ctx, "pruned resource",
						slog.String("manifest", m.Name),
						slog.String("resource", resourceDisplay))
					delete(newEntries, entry.Key)
				} else {
					t.logger.WarnContext(ctx, "skipped removed resource; pass --prune to delete",
						slog.String("manifest", m.Name),
						slog.String("resource", resourceDisplay))
				}
			}
		}

		// helmfile の wait: true は mgr.Apply() 経由 (通常 apply) でしか実行されない
		// ため、--sync でも同じ挙動になるようここで尊重する。差分ゼロの場合は
		// リソースが既に適用済みであるため待機しない。
		if wait, timeoutSeconds := manifestWaitConfig(m); wait {
			if err := resource.WaitForReady(ctx, t.k8sClient, t.logger, objects, timeoutSeconds); err != nil {
				return errors.Wrapf(err, "failed to wait for resources of manifest %q", m.Name)
			}
		}

		// ステートを保存（atomicモード時は後でまとめて保存）
		newStateData := &state.StateData{
			Metadata: state.StateMetadata{
				LastSyncedAt:  time.Now().UTC().Format(time.RFC3339),
				GitCommitHash: gitCommit,
			},
			Entries: newEntries,
		}
		if t.applyOpts.Atomic {
			// DAG モードでは同一層内で並列に書き込むため mutex でガードする。
			if pendingSavesMu != nil {
				pendingSavesMu.Lock()
				pendingSaves[m.Name] = newStateData
				pendingSavesMu.Unlock()
			} else {
				pendingSaves[m.Name] = newStateData
			}
		} else {
			if err := store.Save(ctx, m.Name, newStateData); err != nil {
				return errors.Wrapf(err, "failed to save state for manifest %q", m.Name)
			}
		}
	}

	// テストプラグインは差分の有無にかかわらず毎回実行する (旧 state sync 互換)
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

	return nil
}

// manifestWaitConfig は manifest に wait 設定があるかどうかとタイムアウト秒数を返す。
// helmfile manifest 本体と、ORAS の helmfile delegate の双方に対応する。
func manifestWaitConfig(m v1.Manifest) (bool, int) {
	switch m.Type {
	case v1.ManifestTypeHelmfile:
		if m.Helmfile != nil {
			return m.Helmfile.Wait, m.Helmfile.TimeoutSeconds
		}
	case v1.ManifestTypeORAS:
		if m.ORAS != nil && m.ORAS.Delegate.Type == v1.ORASDelegateTypeHelmfile && m.ORAS.Delegate.Helmfile != nil {
			return m.ORAS.Delegate.Helmfile.Wait, m.ORAS.Delegate.Helmfile.TimeoutSeconds
		}
	}
	return false, 0
}

// saveManifestState は適用済みmanifestのstateをConfigMapに保存する。
// 通常 apply (--sync なし) で使われる。Apply() が返した render 済みオブジェクトを
// そのまま受け取り (再 Build による二重 render を回避)、manifest単位の Content Hash を
// 記録して `state list` / `state diff` から drift を可視化できるようにする。
func (t *TazunaRunner) saveManifestState(
	ctx context.Context,
	m v1.Manifest,
	objects []client.Object,
	store state.StateStore,
	gitCommit string,
) error {
	// manifest名が未設定のものはstate keyを作れないためスキップする
	if m.Name == "" {
		t.logger.WarnContext(ctx, "manifest has no name, skipping state save", slog.String("type", string(m.Type)))
		return nil
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

	for _, m := range tazuna.Spec.Manifests {
		if len(m.Includes) > 0 {
			// includesが指定されている場合、includeファイルを展開する
			t.logger.DebugContext(ctx, "expanding includes", slog.Int("includeFiles", len(m.Includes)))

			for _, include := range m.Includes {
				includePath := filepath.Join(baseDir, include.Path)

				// includeファイルを読み込み
				includeData, err := os.ReadFile(includePath)
				if err != nil {
					return errors.Wrapf(err, "failed to open include file: %s", includePath)
				}

				// tazuna.yaml 本体と同じく include ファイルも Go template として描画し、
				// {{ .Environment }} を解決してからパースする。
				rendered, err := tmpl.Render(includePath, includeData, tmpl.Data{Environment: t.environment})
				if err != nil {
					return errors.WithStack(err)
				}

				// includeファイルをパースして完全なTazuna構造として読み込む
				var includeTazuna v1.Tazuna
				if err := yaml.Unmarshal(rendered, &includeTazuna); err != nil {
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
			expandedManifests = append(expandedManifests, m)
		}
	}

	// 展開されたマニフェスト配列で置き換える
	tazuna.Spec.Manifests = expandedManifests
	return nil
}
