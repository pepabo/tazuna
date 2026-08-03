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

// TestHelmfile_Build_LocalSubchartPath は `<seg>/<seg>` 形式でも repositories に
// 宣言のない chart 参照は従来どおり baseDir 起点のローカルパスとして解決されることを
// 確認する (リモート repository 対応追加によるデグレ防止)。
func TestHelmfile_Build_LocalSubchartPath(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-localsubchart/helmfile.yaml",
	}

	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	assert.Contains(t, out, "kind: Deployment")
	assert.Contains(t, out, "mylocalchart-nginx")
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

// TestHelmfile_Build_ToYamlFunction は helmfile テンプレート内で `toYaml` が
// 使えることを確認する。一部の helmfile.yaml.gotmpl は
// `{{ .StateValues.xxx | toYaml | indent N }}` の形でリスト値を values に
// 埋め込んでおり、これが `function "toYaml" not defined` で失敗しないことを
// 保証する回帰テスト。
func TestHelmfile_Build_ToYamlFunction(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-toyaml/helmfile.yaml.gotmpl",
		Helmfile: &v1.ManifestHelmfile{
			Vars: map[string]v1.HelmFileVar{
				"namespaces": {
					From:        v1.HelmFileVarFromStatic,
					StaticSlice: []string{"batch", "batch-integration"},
				},
			},
		},
	}

	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	assert.Contains(t, out, `namespace-count: "2"`)
	assert.Contains(t, out, `first-namespace: "batch"`)
	assert.Contains(t, out, `second-namespace: "batch-integration"`)
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
