//go:build integration

package manager_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
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

// stdoutFakeProvider は accessKeyID と secretAccessKey の両方を返す fake provider。
type stdoutFakeProvider struct{}

func (p *stdoutFakeProvider) Fetch(ctx context.Context, s v1.GenesisSecretGenerate) (map[string]string, error) {
	out := map[string]string{}
	if _, ok := s.Items["accessKeyID"]; ok {
		out["AWS_ACCESS_KEY_ID"] = "AKIAEXAMPLE"
	}
	if _, ok := s.Items["secretAccessKey"]; ok {
		out["AWS_SECRET_ACCESS_KEY"] = "supersecret"
	}
	return out, nil
}

func newRegistryWithFake(t *testing.T, p genesissecret.SecretProvider) *genesissecret.ProviderRegistry {
	t.Helper()
	r := genesissecret.NewProviderRegistry()
	if err := r.Register(v1.DefaultOnePasswordProviderName, p); err != nil {
		t.Fatalf("failed to register fake provider: %v", err)
	}
	return r
}

func TestGenesisSecret_Apply_StdoutOutput(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()
	registry := newRegistryWithFake(t, &stdoutFakeProvider{})
	var buf bytes.Buffer
	m := manager.NewGenesisSecret(client, registry).WithStdout(&buf)

	manifest := v1.Manifest{Path: "testdata/genesis_secret_stdout.yaml"}
	_, err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)

	out := buf.String()
	// sorted KEY=VALUE 形式で出ること
	assert.Equal(t, "AWS_ACCESS_KEY_ID=AKIAEXAMPLE\nAWS_SECRET_ACCESS_KEY=supersecret\n", out)

	// クラスタリソースは作られていないこと
	var sec corev1.Secret
	err = client.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "sample-secret"}, &sec)
	assert.Error(t, err) // NotFound
}

func TestGenesisSecret_Build_StdoutReturnsEmpty(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()
	registry := newRegistryWithFake(t, &stdoutFakeProvider{})
	m := manager.NewGenesisSecret(client, registry)

	manifest := v1.Manifest{Path: "testdata/genesis_secret_stdout.yaml"}
	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	// stdout 出力は cluster manifest を生まないので Build は空文字
	assert.Equal(t, "", out)
}

func TestGenesisSecret_Destroy_StdoutIsNoop(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()
	registry := newRegistryWithFake(t, &stdoutFakeProvider{})
	m := manager.NewGenesisSecret(client, registry)

	manifest := v1.Manifest{Path: "testdata/genesis_secret_stdout.yaml"}
	err := m.Destroy(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
}

func TestGenesisSecret_Apply_StdoutAndK8sBothSetRejected(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()
	registry := newRegistryWithFake(t, &stdoutFakeProvider{})
	var buf bytes.Buffer
	m := manager.NewGenesisSecret(client, registry).WithStdout(&buf)

	manifest := v1.Manifest{Path: "testdata/genesis_secret_stdout_and_k8s.yaml"}
	_, err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.Error(t, err)
	assert.True(t, strings.Contains(err.Error(), "both stdout and kubernetesSecret"))
}
