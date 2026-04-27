package runner

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/state"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// StateDiff は各managerのBuild()で生成したマニフェストとステートを比較し、差分を出力する
func (t *TazunaRunner) StateDiff(
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

	managers := setupManagers(t.k8sClient, t.opClient, t.orasPullOpts)
	store := state.NewConfigMapStateStore(t.k8sClient)

	hasDiff := false
	for i, m := range tazuna.Spec.Manifests {
		if m.Name == "" {
			t.logger.WarnContext(ctx, "manifest has no name, skipping state diff", slog.Int("index", i), slog.String("type", string(m.Type)))
			continue
		}

		// parallel managerはBuild()をサポートしていないためスキップ
		if m.Type == v1.ManifestTypeParallel {
			t.logger.WarnContext(ctx, "parallel manifest is not supported for state diff, skipping", slog.String("name", m.Name))
			continue
		}

		mgr, ok := managers[string(m.Type)]
		if !ok {
			return fmt.Errorf("manager %s not found", m.Type)
		}

		// Build()で現在のマニフェストを生成
		out, err := mgr.Build(ctx, t.logger, m)
		if err != nil {
			return errors.Wrapf(err, "failed to build manifest %q", m.Name)
		}

		// YAMLをunstructuredオブジェクトに変換
		defaultNs := getDefaultNamespace(m)
		objects, err := manifest.ConvertManifestsToObjects([]byte(out), defaultNs)
		if err != nil {
			return errors.Wrapf(err, "failed to convert manifests to objects for %q", m.Name)
		}

		// 現在のエントリを構築
		currentEntries := make(map[string]state.StateEntry, len(objects))
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

		// diff算出
		diffEntries := state.ComputeDiff(stateData, currentEntries, alwaysSyncKeys)
		if len(diffEntries) == 0 {
			continue
		}

		hasDiff = true
		if _, err := fmt.Fprintf(w, "Manifest: %s\n", m.Name); err != nil {
			return errors.WithStack(err)
		}
		if _, err := fmt.Fprintf(w, "  %-14s %-60s %s\n", "STATUS", "RESOURCE", "HASH"); err != nil {
			return errors.WithStack(err)
		}

		for _, entry := range diffEntries {
			resource := formatDiffResourceKey(entry.Key)
			hashDisplay := formatDiffHash(entry)
			if _, err := fmt.Fprintf(w, "  %-14s %-60s %s\n", entry.DiffType, resource, hashDisplay); err != nil {
				return errors.WithStack(err)
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return errors.WithStack(err)
		}
	}

	if !hasDiff {
		if _, err := fmt.Fprintln(w, "No changes detected."); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// getDefaultNamespace はmanifestタイプに応じたdefaultNamespaceを返す
func getDefaultNamespace(m v1.Manifest) string {
	switch m.Type {
	case v1.ManifestTypeKustomize:
		if m.Kustomize != nil {
			return m.Kustomize.DefaultNamespace
		}
	case v1.ManifestTypeHelmfile:
		if m.Helmfile != nil {
			return m.Helmfile.DefaultNamespace
		}
	}
	return ""
}

// formatDiffResourceKey はステートキー文字列からmanifest名を除いたリソース識別子を返す
func formatDiffResourceKey(keyStr string) string {
	parsed, err := state.ParseStateKey(keyStr)
	if err != nil {
		return keyStr
	}
	return formatResourceKey(parsed)
}

// formatDiffHash はDiffEntryに応じたハッシュ表示文字列を返す
func formatDiffHash(entry state.DiffEntry) string {
	switch entry.DiffType {
	case state.DiffTypeRemoved:
		return fmt.Sprintf("(was: %s)", entry.OldHash)
	default:
		return entry.NewHash
	}
}
