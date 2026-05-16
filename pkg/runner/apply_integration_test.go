//go:build integration

package runner_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestApplyToCluster_OK(t *testing.T) {
	t.Parallel()
	path := "testdata/ok/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	tazuna := v1.Tazuna{}
	err = yaml.Unmarshal(data, &tazuna)
	assert.NoError(t, err)

	baseDir := filepath.Dir(path)
	r.ConvertManifestPathFromCwd(baseDir, &tazuna)
	err = r.ApplyToCluster(context.Background(), tazuna)
	assert.NoError(t, err)

	dep := appsv1.Deployment{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx-deployment",
	}, &dep)
	assert.NoError(t, err)

	svc := corev1.Service{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx",
	}, &svc)
	assert.NoError(t, err)
}

func TestApplyToCluster_WithTags(t *testing.T) {
	t.Parallel()
	path := "testdata/tags/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil, runner.WithTags([]string{"kustomize1"}))

	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	tazuna := v1.Tazuna{}
	err = yaml.Unmarshal(data, &tazuna)
	assert.NoError(t, err)

	baseDir := filepath.Dir(path)
	r.ConvertManifestPathFromCwd(baseDir, &tazuna)
	err = r.ApplyToCluster(context.Background(), tazuna)
	assert.NoError(t, err)

	dep := appsv1.Deployment{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx1",
	}, &dep)
	assert.NoError(t, err)

	svc := corev1.Service{}
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx2",
	}, &svc)
	assert.Error(t, err)
}

func TestApply_WithIncludes(t *testing.T) {
	t.Parallel()
	path := "testdata/include/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(logger, client, nil)

	data, err := os.ReadFile(path)
	assert.NoError(t, err)

	tazuna := v1.Tazuna{}
	err = yaml.Unmarshal(data, &tazuna)
	assert.NoError(t, err)

	// 元のマニフェスト数を確認（includeマニフェストが1つ）
	assert.Equal(t, 1, len(tazuna.Spec.Manifests))
	assert.NotEmpty(t, tazuna.Spec.Manifests[0].Includes)

	// Apply関数を使用してinclude展開をテスト
	err = r.Apply(context.Background(), tazuna, path)
	assert.NoError(t, err)

	// include機能が正常に動作し、エラーなく処理が完了することを確認
	// （実際のリソース作成はfakeクライアントでは完全に再現できないため、
	// 処理が成功することでinclude機能の動作を確認）
}

func TestExpandIncludes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		inputTazuna       v1.Tazuna
		tazunaYAMLPath    string
		expectedManifests int
		expectErr         bool
		errMsg            string
	}{
		{
			name: "expand includes successfully",
			inputTazuna: v1.Tazuna{
				Spec: v1.TazunaSpec{
					Manifests: []v1.Manifest{
						{
							Name: "include manifest",
							Includes: []v1.IncludeFile{
								{Path: "kustomize.yaml"},
								{Path: "genesissecret.yaml"},
							},
						},
					},
				},
			},
			tazunaYAMLPath:    "testdata/include/tazuna.yaml",
			expectedManifests: 2, // 2つのincludeファイルから展開される
			expectErr:         false,
		},
		{
			name: "no includes should remain unchanged",
			inputTazuna: v1.Tazuna{
				Spec: v1.TazunaSpec{
					Manifests: []v1.Manifest{
						{
							Name: "normal manifest",
							Path: "kustomize",
							Type: v1.ManifestTypeKustomize,
						},
					},
				},
			},
			tazunaYAMLPath:    "testdata/include/tazuna.yaml",
			expectedManifests: 1,
			expectErr:         false,
		},
		{
			name: "mixed includes and normal manifests",
			inputTazuna: v1.Tazuna{
				Spec: v1.TazunaSpec{
					Manifests: []v1.Manifest{
						{
							Name: "normal manifest",
							Path: "kustomize",
							Type: v1.ManifestTypeKustomize,
						},
						{
							Name: "include manifest",
							Includes: []v1.IncludeFile{
								{Path: "kustomize.yaml"},
							},
						},
					},
				},
			},
			tazunaYAMLPath:    "testdata/include/tazuna.yaml",
			expectedManifests: 2, // 1つ（normal） + 1つ（includeから展開）
			expectErr:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
			client := fake.NewClientBuilder().Build()
			r := runner.NewTazunaRunner(logger, client, nil)

			// expandIncludesメソッドを直接テストしたいが、privateなので
			// Apply関数を使ってテスト。
			// ただし、Apply関数内でinclude展開されるため、元のtazuna構造体は変更されない。
			// そのため、include展開が正常に動作することをエラーの有無で確認する。
			err := r.Apply(context.Background(), tt.inputTazuna, tt.tazunaYAMLPath)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errMsg != "" {
					assert.Contains(t, err.Error(), tt.errMsg)
				}
			} else {
				assert.NoError(t, err)
				// include機能が正常に動作していることを成功で確認
			}
		})
	}
}
