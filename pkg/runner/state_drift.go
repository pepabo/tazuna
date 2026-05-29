package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"sort"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/state"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// DriftType はライブクラスタとステートの間で検出された差分の種類を表す
type DriftType string

const (
	// DriftTypeDrifted は保存ハッシュとライブから再計算したハッシュが一致しない状態を表す
	DriftTypeDrifted DriftType = "live-drifted"
	// DriftTypeMissing は保存ステートに存在するリソースがライブクラスタ上で見つからない状態を表す
	DriftTypeMissing DriftType = "live-missing"
)

// DriftEntry は個別リソースの drift 情報を表す
type DriftEntry struct {
	Key        string    // ステートキー文字列
	DriftType  DriftType // drift の種類
	StoredHash string    // 保存されていたハッシュ
	LiveHash   string    // ライブ取得時のハッシュ (Missing なら空)
}

// StateDrift は保存ステートとライブクラスタを比較し、手動変更や削除を検知して出力する
func (t *TazunaRunner) StateDrift(
	ctx context.Context,
	tazuna v1.Tazuna,
	tazunaYAMLPath string,
	w io.Writer,
) error {
	if err := t.expandIncludes(ctx, &tazuna, tazunaYAMLPath); err != nil {
		return errors.WithStack(err)
	}

	baseDir := filepath.Dir(tazunaYAMLPath)
	t.ConvertManifestPathFromCwd(baseDir, &tazuna)

	store := state.NewConfigMapStateStore(t.k8sClient)

	hasDrift := false
	for i, m := range tazuna.Spec.Manifests {
		if m.Name == "" {
			t.logger.WarnContext(ctx, "manifest has no name, skipping state drift", slog.Int("index", i), slog.String("type", string(m.Type)))
			continue
		}

		// parallel manager は Build() をサポートしておらず state も保存されないためスキップ
		if m.Type == v1.ManifestTypeParallel {
			t.logger.WarnContext(ctx, "parallel manifest is not supported for state drift, skipping", slog.String("name", m.Name))
			continue
		}

		// GenesisSecret は always-sync 扱いであり drift という概念を持たないためスキップ
		if m.Type == v1.ManifestTypeGenesisSecret {
			t.logger.DebugContext(ctx, "genesis-secret manifest is always-sync, skipping state drift", slog.String("name", m.Name))
			continue
		}

		stateData, err := store.Get(ctx, m.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get state for manifest %q", m.Name)
		}

		// 保存ステートが空の manifest は apply / sync されていないため drift 検知対象外
		if len(stateData.Entries) == 0 {
			continue
		}

		driftEntries, err := t.detectDriftForManifest(ctx, m.Name, stateData)
		if err != nil {
			return errors.WithStack(err)
		}

		if len(driftEntries) == 0 {
			continue
		}

		hasDrift = true
		if _, err := fmt.Fprintf(w, "Manifest: %s\n", m.Name); err != nil {
			return errors.WithStack(err)
		}
		if _, err := fmt.Fprintf(w, "  %-14s %-60s %s\n", "STATUS", "RESOURCE", "HASH"); err != nil {
			return errors.WithStack(err)
		}

		for _, entry := range driftEntries {
			resource := formatDiffResourceKey(entry.Key)
			hashDisplay := formatDriftHash(entry)
			if _, err := fmt.Fprintf(w, "  %-14s %-60s %s\n", entry.DriftType, resource, hashDisplay); err != nil {
				return errors.WithStack(err)
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return errors.WithStack(err)
		}
	}

	if !hasDrift {
		if _, err := fmt.Fprintln(w, "No drift detected."); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// detectDriftForManifest は保存ステートの各エントリについてライブクラスタを取得し、
// 不一致または存在しないものを DriftEntry として返す。
func (t *TazunaRunner) detectDriftForManifest(
	ctx context.Context,
	manifestName string,
	stateData *state.StateData,
) ([]DriftEntry, error) {
	var entries []DriftEntry

	for keyStr, stored := range stateData.Entries {
		parsed, err := state.ParseStateKey(keyStr)
		if err != nil {
			t.logger.WarnContext(ctx, "failed to parse state key, skipping",
				slog.String("manifest", manifestName),
				slog.String("key", keyStr),
				slog.String("error", err.Error()))
			continue
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

		getErr := t.k8sClient.Get(ctx, client.ObjectKey{Namespace: parsed.Namespace, Name: parsed.Name}, obj)
		if getErr != nil {
			if apierrors.IsNotFound(getErr) {
				entries = append(entries, DriftEntry{
					Key:        keyStr,
					DriftType:  DriftTypeMissing,
					StoredHash: stored.ContentHash,
				})
				continue
			}
			return nil, errors.Wrapf(getErr, "failed to get live resource for %s", keyStr)
		}

		liveHash, err := state.ComputeContentHash(obj)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to compute live content hash for %s", keyStr)
		}

		if liveHash != stored.ContentHash {
			entries = append(entries, DriftEntry{
				Key:        keyStr,
				DriftType:  DriftTypeDrifted,
				StoredHash: stored.ContentHash,
				LiveHash:   liveHash,
			})
		}
	}

	// 安定したソート: DriftType → Key の順
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].DriftType != entries[j].DriftType {
			return driftTypeOrder(entries[i].DriftType) < driftTypeOrder(entries[j].DriftType)
		}
		return entries[i].Key < entries[j].Key
	})

	return entries, nil
}

// driftTypeOrder は DriftType の安定ソート順を返す
func driftTypeOrder(d DriftType) int {
	switch d {
	case DriftTypeDrifted:
		return 0
	case DriftTypeMissing:
		return 1
	default:
		return 2
	}
}

// formatDriftHash は DriftEntry に応じたハッシュ表示文字列を返す
func formatDriftHash(entry DriftEntry) string {
	switch entry.DriftType {
	case DriftTypeMissing:
		return fmt.Sprintf("(stored: %s)", entry.StoredHash)
	default:
		return fmt.Sprintf("%s (stored: %s)", entry.LiveHash, entry.StoredHash)
	}
}
