package manager_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type fakeUnitSecretProvider struct {
	m map[string]string
}

func (f *fakeUnitSecretProvider) Fetch(_ context.Context, _ v1.GenesisSecretGenerate) (map[string]string, error) {
	return f.m, nil
}

var _ genesissecret.SecretProvider = &fakeUnitSecretProvider{}

// TestGenesisSecret_Build_MultipleOutputs は Build が outputs[0] だけでなく
// 全 kubernetesSecret outputs を render することを確認する。--sync モードは
// Build の出力を直接適用するため、2 個目以降の output が失われると
// 通常 apply と適用結果が変わってしまう。
func TestGenesisSecret_Build_MultipleOutputs(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "genesis_secret.yaml")
	err := os.WriteFile(path, []byte(`
spec:
  secrets:
    - uri: op://example.1password.com/vault/item
      items:
        password:
          mapTo: PASSWORD
  outputs:
    - stdout: {}
    - kubernetesSecret:
        namespace: default
        name: first-secret
    - kubernetesSecret:
        namespace: other
        name: second-secret
`), 0o644)
	assert.NoError(t, err)

	registry := genesissecret.NewProviderRegistry()
	err = registry.Register(v1.DefaultOnePasswordProviderName, &fakeUnitSecretProvider{
		m: map[string]string{"PASSWORD": "s3cret"},
	})
	assert.NoError(t, err)

	client := fake.NewFakeClient()
	g := manager.NewGenesisSecret(client, registry).WithStdout(io.Discard)
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	out, err := g.Build(context.Background(), logger, v1.Manifest{Path: path})
	assert.NoError(t, err)
	assert.Contains(t, out, "first-secret")
	assert.Contains(t, out, "second-secret")

	// multi-document YAML として両方のドキュメントが含まれること
	assert.Equal(t, 2, strings.Count(out, "kind: Secret"))

	// Apply でも両方の Secret が作成されること (Build との整合性)
	_, err = g.Apply(context.Background(), logger, v1.Manifest{Path: path})
	assert.NoError(t, err)
	for _, key := range []types.NamespacedName{
		{Namespace: "default", Name: "first-secret"},
		{Namespace: "other", Name: "second-secret"},
	} {
		secret := corev1.Secret{}
		assert.NoError(t, client.Get(context.Background(), key, &secret), "secret %s should exist", key)
		assert.Equal(t, "s3cret", secret.StringData["PASSWORD"])
	}
}

// TestGenesisSecret_Build_StdoutOnly は stdout 出力のみの場合に Build が
// 空文字を返す (state entries が 0 になる) ことを確認する。
func TestGenesisSecret_Build_StdoutOnly(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "genesis_secret.yaml")
	err := os.WriteFile(path, []byte(`
spec:
  secrets:
    - uri: op://example.1password.com/vault/item
      items:
        password:
          mapTo: PASSWORD
  outputs:
    - stdout: {}
`), 0o644)
	assert.NoError(t, err)

	registry := genesissecret.NewProviderRegistry()
	err = registry.Register(v1.DefaultOnePasswordProviderName, &fakeUnitSecretProvider{
		m: map[string]string{"PASSWORD": "s3cret"},
	})
	assert.NoError(t, err)

	g := manager.NewGenesisSecret(fake.NewFakeClient(), registry).WithStdout(io.Discard)
	out, err := g.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), v1.Manifest{Path: path})
	assert.NoError(t, err)
	assert.Empty(t, out)
}
