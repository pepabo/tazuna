package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/resource"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// StateSyncOptions は state sync コマンドのオプションを保持する
type StateSyncOptions struct {
	// Atomic が true の場合、いずれかのリソースでエラーが発生した場合にステートを一切更新しない
	Atomic bool
}

// StateSync は各managerのBuild()で生成したマニフェストとステートを比較し、
// 差分に基づいてリソースのcreate/update/deleteを行い、ステートを更新する
func (t *TazunaRunner) StateSync(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
	w io.Writer,
	opts StateSyncOptions,
) error {
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return errors.WithStack(err)
	}

	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)

	// tazuna namespaceの存在を保証
	if err := state.EnsureNamespace(ctx, t.k8sClient); err != nil {
		return errors.Wrapf(err, "failed to ensure tazuna namespace")
	}

	managers := setupManagers(t.k8sClient, t.opClient, t.orasPullOpts)
	testPlugins := setupTestPlugins(t.k8sClient)
	store := state.NewConfigMapStateStore(t.k8sClient)
	autoDelete := strings.ToLower(os.Getenv("TAZUNA_STATE_SYNC_DELETE")) == "true"
	gitCommit := getGitCommitHash()

	// atomicモード時はステート保存を全manifest処理完了後にまとめて行う
	pendingSaves := make(map[string]*state.StateData)

	syncedAny := false
	for i, m := range tazuna.Spec.Manifests {
		if m.Name == "" {
			t.logger.WarnContext(ctx, "manifest has no name, skipping state sync", slog.Int("index", i), slog.String("type", string(m.Type)))
			continue
		}

		if m.Type == v1.ManifestTypeParallel {
			t.logger.WarnContext(ctx, "parallel manifest is not supported for state sync, skipping", slog.String("name", m.Name))
			continue
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
			syncedAny = true
			if _, err := fmt.Fprintf(w, "Manifest: %s\n", m.Name); err != nil {
				return errors.WithStack(err)
			}

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
					if _, err := fmt.Fprintf(w, "  %-14s %s\n", entry.DiffType, resourceDisplay); err != nil {
						return errors.WithStack(err)
					}
					newEntries[entry.Key] = state.StateEntry{ContentHash: entry.NewHash}

				case state.DiffTypeRemoved:
					if autoDelete {
						// StateKeyからunstructuredオブジェクトを構築して削除
						obj, err := buildUnstructuredFromStateKey(entry.Key)
						if err != nil {
							return errors.Wrapf(err, "failed to build object from state key %s", entry.Key)
						}
						if err := resource.DeleteObject(ctx, t.k8sClient, obj); err != nil {
							return errors.Wrapf(err, "failed to delete resource %s", resourceDisplay)
						}
						if _, err := fmt.Fprintf(w, "  %-14s %s (deleted)\n", entry.DiffType, resourceDisplay); err != nil {
							return errors.WithStack(err)
						}
						delete(newEntries, entry.Key)
					} else {
						if _, err := fmt.Fprintf(w, "  %-14s %s (skipped: set TAZUNA_STATE_SYNC_DELETE=true to delete)\n", entry.DiffType, resourceDisplay); err != nil {
							return errors.WithStack(err)
						}
					}
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
			if opts.Atomic {
				pendingSaves[m.Name] = newStateData
			} else {
				if err := store.Save(ctx, m.Name, newStateData); err != nil {
					return errors.Wrapf(err, "failed to save state for manifest %q", m.Name)
				}
			}

			if _, err := fmt.Fprintln(w); err != nil {
				return errors.WithStack(err)
			}
		}

		// テストプラグインは差分の有無にかかわらず毎回実行する
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
	}

	// atomicモード時はここで一括保存
	if opts.Atomic {
		for name, sd := range pendingSaves {
			if err := store.Save(ctx, name, sd); err != nil {
				return errors.Wrapf(err, "failed to save state for manifest %q", name)
			}
		}
	}

	if !syncedAny {
		if _, err := fmt.Fprintln(w, "No changes to sync."); err != nil {
			return errors.WithStack(err)
		}
	}

	// すべてのマニフェスト処理終了後にグローバルテストを実行する
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

// buildUnstructuredFromStateKey はステートキーからunstructuredオブジェクトを構築する
func buildUnstructuredFromStateKey(keyStr string) (*unstructured.Unstructured, error) {
	parsed, err := state.ParseStateKey(keyStr)
	if err != nil {
		return nil, err
	}

	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   parsed.Group,
		Version: parsed.Version,
		Kind:    parsed.Kind,
	})
	obj.SetName(parsed.Name)
	if parsed.Namespace != "" {
		obj.SetNamespace(parsed.Namespace)
	}

	return obj, nil
}

// getGitCommitHash は現在のgit commit hashを取得する
func getGitCommitHash() string {
	out, err := exec.Command("git", "rev-parse", "HEAD").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}
