package runner

import (
	"context"
	"slices"

	v1 "github.com/pepabo/tazuna/api/v1"
)

// matchesTags は manifest がタグフィルタに合致するかを返す。
// filter が空なら常に true。manifest のタグのいずれかが filter に
// 含まれれば true (OR 評価)。apply / build / destroy で共通利用する。
func matchesTags(m v1.Manifest, filter []string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, tag := range filter {
		if slices.Contains(m.Tags, tag) {
			return true
		}
	}
	return false
}

func (t *TazunaRunner) ListTags(ctx context.Context, tazuna *v1.Tazuna, tazunaYAMLPath string) (map[string][]string, error) {
	if err := t.expandIncludes(ctx, tazuna, tazunaYAMLPath); err != nil {
		return nil, err
	}

	tags := make(map[string][]string)
	for _, m := range tazuna.Spec.Manifests {
		// 名前未設定の manifest を "" として出力に混ぜない
		if m.Name == "" {
			continue
		}
		for _, tag := range m.Tags {
			if _, exists := tags[tag]; !exists {
				tags[tag] = []string{}
			}

			tags[tag] = append(tags[tag], m.Name)
		}
	}

	return tags, nil
}
