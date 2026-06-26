//go:build integration

package manager_test

import (
	"context"
	"testing"

	"io"
	"log/slog"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGenesisSecret_Apply(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()
	provider := &FakeSecretProvider{
		m: map[string]string{
			"accessKeyID": "sampleAccessKeyID",
		},
	}
	registry := genesissecret.NewProviderRegistry()
	// テスト用に "default-op" 名で fake provider を登録し、fixture の空 .spec.provider
	// (default-op フォールバック) で解決されるようにする。
	if err := registry.Register(v1.DefaultOnePasswordProviderName, provider); err != nil {
		t.Fatalf("failed to register provider: %v", err)
	}
	manager := manager.NewGenesisSecret(client, registry)

	manifest := v1.Manifest{
		// fakeProviderはここの中身は見ない
		Path: "testdata/genesis_secret.yaml",
	}

	// 冪等性が担保されていることのテストをするために、テスト関数を定義して複数回呼ぶ
	testFn := func(t *testing.T) {
		_, err := manager.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
		assert.NoError(t, err)

		key := types.NamespacedName{
			Namespace: "default",
			Name:      "sample-secret",
		}
		secret := corev1.Secret{}
		err = client.Get(context.Background(), key, &secret)
		assert.NoError(t, err)

		v, ok := secret.StringData["accessKeyID"]
		assert.True(t, ok)
		assert.Equal(t, "sampleAccessKeyID", v)

		v, ok = secret.GetLabels()["foo"]
		assert.True(t, ok)
		assert.True(t, ok)
		assert.Equal(t, "bar", v)

		v, ok = secret.GetAnnotations()["foo"]
		assert.True(t, ok)
		assert.True(t, ok)
		assert.Equal(t, "bar", v)

		assert.Equal(t, corev1.SecretTypeTLS, secret.Type)
	}

	testFn(t)
	testFn(t)
}

type FakeSecretProvider struct {
	m map[string]string
}

// Fetch implements genesissecret.SecretProvider.
func (f *FakeSecretProvider) Fetch(ctx context.Context, s v1.GenesisSecretGenerate) (map[string]string, error) {
	return f.m, nil
}

var _ genesissecret.SecretProvider = &FakeSecretProvider{}
