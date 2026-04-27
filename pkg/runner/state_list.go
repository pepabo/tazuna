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
)

// StateList はクラスタ上のステートConfigMapを読み込み、管理されているリソースを一覧表示する
func (t *TazunaRunner) StateList(
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

	for i, manifest := range tazuna.Spec.Manifests {
		if manifest.Name == "" {
			t.logger.WarnContext(ctx, "manifest has no name, skipping state list", slog.Int("index", i), slog.String("type", string(manifest.Type)))
			continue
		}

		data, err := store.Get(ctx, manifest.Name)
		if err != nil {
			return errors.Wrapf(err, "failed to get state for manifest %q", manifest.Name)
		}

		if _, err := fmt.Fprintf(w, "Manifest: %s\n", manifest.Name); err != nil {
			return errors.WithStack(err)
		}

		if len(data.Entries) == 0 {
			if _, err := fmt.Fprintf(w, "  No state found\n\n"); err != nil {
				return errors.WithStack(err)
			}
			continue
		}

		if data.Metadata.LastSyncedAt != "" {
			if _, err := fmt.Fprintf(w, "  Last synced: %s\n", data.Metadata.LastSyncedAt); err != nil {
				return errors.WithStack(err)
			}
		}
		if data.Metadata.GitCommitHash != "" {
			if _, err := fmt.Fprintf(w, "  Git commit: %s\n", data.Metadata.GitCommitHash); err != nil {
				return errors.WithStack(err)
			}
		}

		// キーをソートして安定した出力にする
		keys := make([]string, 0, len(data.Entries))
		for k := range data.Entries {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		if _, err := fmt.Fprintf(w, "  %-60s %s\n", "RESOURCE", "HASH"); err != nil {
			return errors.WithStack(err)
		}
		for _, k := range keys {
			entry := data.Entries[k]
			parsed, err := state.ParseStateKey(k)
			if err != nil {
				t.logger.WarnContext(ctx, "failed to parse state key", slog.String("key", k), slog.String("error", err.Error()))
				if _, err := fmt.Fprintf(w, "  %-60s %s\n", k, entry.ContentHash); err != nil {
					return errors.WithStack(err)
				}
				continue
			}
			resource := formatResourceKey(parsed)
			if _, err := fmt.Fprintf(w, "  %-60s %s\n", resource, entry.ContentHash); err != nil {
				return errors.WithStack(err)
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return errors.WithStack(err)
		}
	}

	return nil
}

// formatResourceKey はStateKeyからmanifest名を除いたリソース識別子を返す
func formatResourceKey(key state.StateKey) string {
	if key.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s/%s", key.Group, key.Version, key.Kind, key.Name)
	}
	return fmt.Sprintf("%s/%s/%s/%s/%s", key.Group, key.Version, key.Kind, key.Namespace, key.Name)
}
