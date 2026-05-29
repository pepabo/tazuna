//go:build integration

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestApplyToCluster_MultipleProviders は tazuna.yaml の spec.providers[] で envfile
// provider を宣言し、別 manifest がそれぞれ 1Password (built-in default-op) と envfile を
// 参照する構成で、両方の Secret が apply されることを保証する。
func TestApplyToCluster_MultipleProviders(t *testing.T) {
	t.Parallel()

	// 1Password fake setup
	opc := op.NewFakeClient()
	opc.Vaults["my-vault"] = []op.Item{
		{
			ID:    "op-item",
			Title: "op-item",
			Fields: []op.ItemField{
				{ID: "OP_KEY", Label: "op-key", Value: "op-value-from-1p"},
			},
		},
	}

	// envfile setup
	tempDir := t.TempDir()
	envPath := filepath.Join(tempDir, "secrets.env")
	envContent := "ENV_KEY=value-from-envfile\n"
	require.NoError(t, os.WriteFile(envPath, []byte(envContent), 0o600))

	// GenesisSecret manifests を 2 つ用意する。1 つは default-op (1Password) を、
	// もう 1 つは "my-envfile" を参照する。
	opManifestPath := filepath.Join(tempDir, "op-secret.yaml")
	envManifestPath := filepath.Join(tempDir, "env-secret.yaml")

	opManifest := `apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
metadata:
  name: op-secret
  namespace: default
spec:
  provider: ""
  secrets:
    - uri: op://example.1password.com/my-vault/op-item
      items:
        OP_KEY:
          mapTo: from-1p
  outputs:
    - kubernetesSecret:
        name: op-secret
        namespace: default
        type: Opaque
`
	envManifest := `apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
metadata:
  name: env-secret
  namespace: default
spec:
  provider: my-envfile
  secrets:
    - items:
        ENV_KEY:
          mapTo: from-envfile
  outputs:
    - kubernetesSecret:
        name: env-secret
        namespace: default
        type: Opaque
`
	require.NoError(t, os.WriteFile(opManifestPath, []byte(opManifest), 0o600))
	require.NoError(t, os.WriteFile(envManifestPath, []byte(envManifest), 0o600))

	// envfile provider の path は tazuna.yaml ディレクトリ (tempDir) からの相対パス
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Providers: []v1.ProviderConfig{
				{
					Name: "my-envfile",
					Type: v1.ProviderTypeEnvFile,
					EnvFile: &v1.EnvFileProviderConfig{
						Path: "secrets.env",
					},
				},
			},
			Manifests: []v1.Manifest{
				{
					Name: "op-secret",
					Type: v1.ManifestTypeGenesisSecret,
					Path: "op-secret.yaml",
				},
				{
					Name: "env-secret",
					Type: v1.ManifestTypeGenesisSecret,
					Path: "env-secret.yaml",
				},
			},
		},
	}

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, opc)

	// Apply() を tazuna.yaml ベースで呼ぶことで baseDir 解決パス (envfile.path) を通る
	tazunaYAMLPath := filepath.Join(tempDir, "tazuna.yaml")
	require.NoError(t, r.Apply(context.Background(), tazuna, tazunaYAMLPath))

	// 1Password 側 Secret
	opSecret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "op-secret"}, opSecret))
	assert.Equal(t, "op-value-from-1p", opSecret.StringData["from-1p"])

	// envfile 側 Secret
	envSecret := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "env-secret"}, envSecret))
	assert.Equal(t, "value-from-envfile", envSecret.StringData["from-envfile"])
}

// TestApplyToCluster_DefaultProviderFallback は GenesisSecret の .spec.provider が
// 空文字のときに built-in "default-op" にフォールバックして 1Password で fetch される
// ことを保証する。既存 fixture の挙動 (testdata/include/secret.yaml と同等) との
// 後方互換性を担保するテスト。
func TestApplyToCluster_DefaultProviderFallback(t *testing.T) {
	t.Parallel()

	opc := op.NewFakeClient()
	opc.Vaults["v"] = []op.Item{
		{
			ID:    "item",
			Title: "item",
			Fields: []op.ItemField{
				{ID: "KEY", Label: "key", Value: "v1"},
			},
		},
	}

	tempDir := t.TempDir()
	manifestPath := filepath.Join(tempDir, "secret.yaml")
	manifestContent := `apiVersion: tazuna.pepabo.com/v1
kind: GenesisSecret
metadata:
  name: s
  namespace: default
spec:
  secrets:
    - uri: op://host/v/item
      items:
        KEY:
          mapTo: out
  outputs:
    - kubernetesSecret:
        name: default-fallback
        namespace: default
        type: Opaque
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(manifestContent), 0o600))

	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "s",
					Type: v1.ManifestTypeGenesisSecret,
					Path: "secret.yaml",
				},
			},
		},
	}

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, opc)
	require.NoError(t, r.Apply(context.Background(), tazuna, filepath.Join(tempDir, "tazuna.yaml")))

	got := &corev1.Secret{}
	require.NoError(t, c.Get(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "default-fallback"}, got))
	assert.Equal(t, "v1", got.StringData["out"])
}
