package manager_test

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/stretchr/testify/assert"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// newFakeDiscoveryServer は discovery client が叩く最小限のエンドポイントを返す
// API server を立てる。core v1 に加えて CRD 相当の example.com/v1 を公開する。
func newFakeDiscoveryServer(t *testing.T) *httptest.Server {
	t.Helper()
	responses := map[string]any{
		"/version": map[string]any{
			"major":      "1",
			"minor":      "30",
			"gitVersion": "v1.30.0",
		},
		"/api": map[string]any{
			"kind":     "APIVersions",
			"versions": []string{"v1"},
		},
		"/api/v1": map[string]any{
			"kind":         "APIResourceList",
			"groupVersion": "v1",
			"resources":    []any{},
		},
		"/apis": map[string]any{
			"kind": "APIGroupList",
			"groups": []any{
				map[string]any{
					"name": "example.com",
					"versions": []any{
						map[string]any{"groupVersion": "example.com/v1", "version": "v1"},
					},
					"preferredVersion": map[string]any{"groupVersion": "example.com/v1", "version": "v1"},
				},
			},
		},
		"/apis/example.com/v1": map[string]any{
			"kind":         "APIResourceList",
			"groupVersion": "example.com/v1",
			"resources": []any{
				map[string]any{"name": "widgets", "namespaced": true, "kind": "Widget", "verbs": []string{"get", "list"}},
			},
		},
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, ok := responses[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestHelmfile_Build_ClusterCapabilities は restConfig が設定されているとき、
// cluster から discover した APIVersions が kubeVersion の明示有無に関係なく
// .Capabilities.APIVersions に注入されることを確認する。kubeVersion を明示すると
// CRD の有無で分岐する chart のリソース (ServiceMonitor 等) が黙って render
// されなくなっていた問題の回帰テスト。
func TestHelmfile_Build_ClusterCapabilities(t *testing.T) {
	srv := newFakeDiscoveryServer(t)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("without kubeVersion uses discovered KubeVersion and APIVersions", func(t *testing.T) {
		m := manager.NewHelmfile(fake.NewFakeClient(), nil).WithRESTConfig(&rest.Config{Host: srv.URL})
		manifest := v1.Manifest{
			Path:     "testdata/helmfile-capabilities/helmfile.yaml",
			Helmfile: &v1.ManifestHelmfile{},
		}

		out, err := m.Build(context.Background(), logger, manifest)
		assert.NoError(t, err)
		assert.Contains(t, out, `kube-version: "v1.30.0"`)
		assert.Contains(t, out, `has-example-api: "true"`)
	})

	t.Run("with kubeVersion prefers it and still uses discovered APIVersions", func(t *testing.T) {
		m := manager.NewHelmfile(fake.NewFakeClient(), nil).WithRESTConfig(&rest.Config{Host: srv.URL})
		manifest := v1.Manifest{
			Path:     "testdata/helmfile-capabilities/helmfile.yaml",
			Helmfile: &v1.ManifestHelmfile{KubeVersion: "1.29.0"},
		}

		out, err := m.Build(context.Background(), logger, manifest)
		assert.NoError(t, err)
		assert.Contains(t, out, `kube-version: "v1.29.0"`)
		assert.Contains(t, out, `has-example-api: "true"`)
	})

	t.Run("without restConfig falls back to DefaultCapabilities", func(t *testing.T) {
		m := manager.NewHelmfile(fake.NewFakeClient(), nil)
		manifest := v1.Manifest{
			Path:     "testdata/helmfile-capabilities/helmfile.yaml",
			Helmfile: &v1.ManifestHelmfile{KubeVersion: "1.29.0"},
		}

		out, err := m.Build(context.Background(), logger, manifest)
		assert.NoError(t, err)
		assert.Contains(t, out, `kube-version: "v1.29.0"`)
		assert.Contains(t, out, `has-example-api: "false"`)
	})
}
