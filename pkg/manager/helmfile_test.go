package manager_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/stretchr/testify/assert"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// TestHelmfile_Build_EnvironmentName は tazuna の -e/--environment が
// helmfile テンプレートの {{ .Environment.Name }} に伝播することを確認する。
func TestHelmfile_Build_EnvironmentName(t *testing.T) {
	client := fake.NewFakeClient()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manifest := v1.Manifest{
		Path: "testdata/helmfile-environment/helmfile.yaml.gotmpl",
	}

	t.Run("environment is propagated", func(t *testing.T) {
		m := manager.NewHelmfile(client, nil).WithEnvironment("production")
		out, err := m.Build(context.Background(), logger, manifest)
		assert.NoError(t, err)
		assert.Contains(t, out, `label-from-static: "production"`)
	})

	t.Run("empty environment falls back to default", func(t *testing.T) {
		m := manager.NewHelmfile(client, nil)
		out, err := m.Build(context.Background(), logger, manifest)
		assert.NoError(t, err)
		assert.Contains(t, out, `label-from-static: "default"`)
	})
}

// TestHelmfile_Build_RejectsEnvFunction は helmfile テンプレートで sprig の
// env / expandenv が使えないことを確認する。ORAS 経由で取得したリモートの
// helmfile が実行者の環境変数を窃取するのを防ぐためのガード。
func TestHelmfile_Build_RejectsEnvFunction(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-env/helmfile.yaml.gotmpl",
	}

	_, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), `function "env" not defined`)
}

func TestHelmfile_ConstructHelmfileVars_OpFieldLookup(t *testing.T) {
	client := fake.NewFakeClient()

	fakeOpClient := op.NewFakeClient()
	fakeOpClient.Vaults["test-vault"] = []op.Item{
		{
			ID: "test-item",
			Fields: []op.ItemField{
				{
					ID:    "password",
					Label: "password-label",
					Value: "super-secret",
				},
				{
					ID:    "empty-field",
					Label: "empty-label",
					Value: "",
				},
			},
		},
	}
	m := manager.NewHelmfile(client, fakeOpClient)

	opVarManifest := func(key, field string) v1.Manifest {
		return v1.Manifest{
			Path: "testdata/helmfile/helmfile.yaml",
			Helmfile: &v1.ManifestHelmfile{
				Vars: map[string]v1.HelmFileVar{
					"myVar": {
						From: v1.HelmFileVarFromOp,
						Op: &v1.OnePasswordVaultSelector{
							Key:   key,
							Vault: "test-vault",
							Item:  "test-item",
							Field: field,
						},
					},
				},
			},
		}
	}

	t.Run("field found by id", func(t *testing.T) {
		manifest := opVarManifest("id", "password")
		vars, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.NoError(t, err)
		assert.Equal(t, "super-secret", vars["myVar"])
	})

	t.Run("field found by label", func(t *testing.T) {
		manifest := opVarManifest("label", "password-label")
		vars, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.NoError(t, err)
		assert.Equal(t, "super-secret", vars["myVar"])
	})

	t.Run("field not found returns error", func(t *testing.T) {
		manifest := opVarManifest("id", "no-such-field")
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "op field no-such-field not found in item test-item")
	})

	t.Run("unknown from returns error", func(t *testing.T) {
		manifest := v1.Manifest{
			Path: "testdata/helmfile/helmfile.yaml",
			Helmfile: &v1.ManifestHelmfile{
				Vars: map[string]v1.HelmFileVar{
					"myVar": {
						From: "sttic", // typo of "static"
					},
				},
			},
		}
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unsupported From field: sttic")
	})

	t.Run("field with empty value is allowed", func(t *testing.T) {
		// field が存在して値が空文字のケースは「not found」とは区別して許容する
		manifest := opVarManifest("id", "empty-field")
		vars, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.NoError(t, err)
		assert.Equal(t, "", vars["myVar"])
	})
}
