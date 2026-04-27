//go:build integration

package runner_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestListTags_OK(t *testing.T) {
	t.Parallel()
	path := "testdata/tags/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	tags, err := r.ListTags(context.Background(), &tazuna, path)
	assert.NoError(t, err)

	expectedTags := map[string][]string{
		"kustomize1": {"kustomize1"},
		"kustomize2": {"kustomize2"},
	}
	assert.Equal(t, expectedTags, tags)
}

func TestListTags_NoTags(t *testing.T) {
	t.Parallel()
	path := "testdata/ok/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	tags, err := r.ListTags(context.Background(), &tazuna, path)
	assert.NoError(t, err)

	expectedTags := map[string][]string{
		"kustomize": {"kustomize"},
	}
	assert.Equal(t, expectedTags, tags)
}

func TestListTags_MultipleTags(t *testing.T) {
	t.Parallel()
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	tazuna := &v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "manifest1",
					Type: "kustomize",
					Path: "./path1",
					Tags: []string{"tag1", "tag2"},
				},
				{
					Name: "manifest2",
					Type: "kustomize",
					Path: "./path2",
					Tags: []string{"tag2", "tag3"},
				},
			},
		},
	}

	tags, err := r.ListTags(context.Background(), tazuna, "")
	assert.NoError(t, err)

	expectedTags := map[string][]string{
		"tag1": {"manifest1"},
		"tag2": {"manifest1", "manifest2"},
		"tag3": {"manifest2"},
	}
	assert.Equal(t, expectedTags, tags)
}

func TestListTags_WithIncludes(t *testing.T) {
	t.Parallel()
	path := "testdata/include/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	tags, err := r.ListTags(context.Background(), &tazuna, path)
	assert.NoError(t, err)

	expectedTags := map[string][]string{
		"nginx":  {"kustomize deployment"},
		"secret": {"test secret"},
	}
	assert.Equal(t, expectedTags, tags)
}
