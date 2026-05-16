package runner_test

import (
	"context"
	"os"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/yaml"
)

// `tazuna tags` 実行時に nil logger で生成された Runner が
// includes 展開時に panic していたリグレッションを防ぐためのテスト。
func TestListTags_NilLogger_WithIncludes(t *testing.T) {
	t.Parallel()
	path := "testdata/include/tazuna.yaml"
	r := runner.NewTazunaRunner(nil, nil, nil)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	tazuna := v1.Tazuna{}
	err = yaml.Unmarshal(data, &tazuna)
	assert.NoError(t, err)

	tags, err := r.ListTags(context.Background(), &tazuna, path)
	assert.NoError(t, err)

	expectedTags := map[string][]string{
		"nginx":  {"kustomize deployment"},
		"secret": {"test secret"},
	}
	assert.Equal(t, expectedTags, tags)
}

func TestListTags_NilLogger_NoIncludes(t *testing.T) {
	t.Parallel()
	r := runner.NewTazunaRunner(nil, nil, nil)

	tazuna := &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "manifest1",
					Type: "kustomize",
					Path: "./path1",
					Tags: []string{"tag1"},
				},
			},
		},
	}

	tags, err := r.ListTags(context.Background(), tazuna, "")
	assert.NoError(t, err)
	assert.Equal(t, map[string][]string{"tag1": {"manifest1"}}, tags)
}
