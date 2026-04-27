package runner

import (
	"context"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func (t *TazunaRunner) ListTags(ctx context.Context, tazuna *v1.Tazuna, tazunaYAMLPath string) (map[string][]string, error) {
	if err := t.expandIncludes(ctx, tazuna, tazunaYAMLPath); err != nil {
		return nil, err
	}

	tags := make(map[string][]string)
	for _, m := range tazuna.Spec.Manifests {
		for _, tag := range m.Tags {
			if _, exists := tags[tag]; !exists {
				tags[tag] = []string{}
			}

			tags[tag] = append(tags[tag], m.Name)
		}
	}

	return tags, nil
}
