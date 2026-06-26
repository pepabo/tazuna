//go:build integration

package manager_test

import (
	"context"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/op"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"io"
	"log/slog"

	appsv1 "k8s.io/api/apps/v1"
)

func TestHelmfile_Apply(t *testing.T) {
	client := fake.NewFakeClient()

	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile/helmfile.yaml",
	}
	// 冪等性が担保されていることのテストをするために、テスト関数を定義して複数回呼ぶ
	testFn := func(t *testing.T) {
		_, err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
		assert.NoError(t, err)

		dep := appsv1.Deployment{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "mylocalchart-nginx",
		}, &dep)
		assert.NoError(t, err)
	}

	testFn(t)
	testFn(t)
}

func TestHelmfile_Build_WithVars(t *testing.T) {
	client := fake.NewFakeClient()

	fakeClient := op.NewFakeClient()
	fakeClient.Vaults["test-vault"] = []op.Item{
		{
			ID: "test-item",
			Fields: []op.ItemField{
				{
					ID:    "test-field",
					Value: "label-from-op",
				},
			},
		},
	}
	m := manager.NewHelmfile(client, fakeClient)

	staticValue := "label-from-static"
	manifest := v1.Manifest{
		Path: "testdata/helmfile/with_vars.yaml.gotmpl",
		Helmfile: &v1.ManifestHelmfile{
			Vars: map[string]v1.HelmFileVar{
				"labelFromStatic": {
					From:   v1.HelmFileVarFromStatic,
					Static: &staticValue,
				},
				"labelFromOp": {
					From: v1.HelmFileVarFromOp,
					Op: &v1.OnePasswordVaultSelector{
						Key:   "id",
						Vault: "test-vault",
						Item:  "test-item",
						Field: "test-field",
					},
				},
			},
		},
	}

	// 冪等性が担保されていることのテストをするために、テスト関数を定義して複数回呼ぶ
	testFn := func(t *testing.T) {
		_, err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
		assert.NoError(t, err)

		dep := appsv1.Deployment{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "mylocalchart-nginx",
		}, &dep)
		assert.NoError(t, err)

		// 書き換わっていることを確:w
		assert.Equal(t, "label-from-static", dep.Spec.Template.Labels["label-from-static"])
		assert.Equal(t, "label-from-op", dep.Spec.Template.Labels["label-from-op"])
	}

	testFn(t)
	testFn(t)
}

func TestHelmfile_ConstructHelmfileVars_StaticSliceAndMap(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	tests := []struct {
		name      string
		manifest  v1.Manifest
		wantVars  map[string]any
		wantError bool
		errorMsg  string
	}{
		{
			name: "staticSlice",
			manifest: v1.Manifest{
				Path: "testdata/helmfile/helmfile.yaml",
				Helmfile: &v1.ManifestHelmfile{
					Vars: map[string]v1.HelmFileVar{
						"mySlice": {
							From:        v1.HelmFileVarFromStatic,
							StaticSlice: []string{"value1", "value2", "value3"},
						},
					},
				},
			},
			wantVars: map[string]any{
				"mySlice": []string{"value1", "value2", "value3"},
			},
		},
		{
			name: "staticMap",
			manifest: v1.Manifest{
				Path: "testdata/helmfile/helmfile.yaml",
				Helmfile: &v1.ManifestHelmfile{
					Vars: map[string]v1.HelmFileVar{
						"myMap": {
							From: v1.HelmFileVarFromStatic,
							StaticMap: map[string]string{
								"key1": "value1",
								"key2": "value2",
							},
						},
					},
				},
			},
			wantVars: map[string]any{
				"myMap": map[string]string{
					"key1": "value1",
					"key2": "value2",
				},
			},
		},
		{
			name: "multiple static fields error",
			manifest: v1.Manifest{
				Path: "testdata/helmfile/helmfile.yaml",
				Helmfile: &v1.ManifestHelmfile{
					Vars: map[string]v1.HelmFileVar{
						"invalid": {
							From:        v1.HelmFileVarFromStatic,
							Static:      stringPtr("value"),
							StaticSlice: []string{"value1", "value2"},
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "multiple static fields are set",
		},
		{
			name: "no static field error",
			manifest: v1.Manifest{
				Path: "testdata/helmfile/helmfile.yaml",
				Helmfile: &v1.ManifestHelmfile{
					Vars: map[string]v1.HelmFileVar{
						"invalid": {
							From: v1.HelmFileVarFromStatic,
						},
					},
				},
			},
			wantError: true,
			errorMsg:  "no static/staticSlice/staticMap field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars, err := m.ConstructHelmfileVars(context.Background(), &tt.manifest)
			if tt.wantError {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorMsg)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.wantVars, vars)
			}
		})
	}
}

func TestHelmfile_Apply_WithExtraValueFiles(t *testing.T) {
	client := fake.NewFakeClient()

	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile/helmfile-with-extra-values.yaml",
		Helmfile: &v1.ManifestHelmfile{
			ExtraValueFiles: []string{"./extra-values.yaml"},
		},
	}

	// 冪等性が担保されていることのテストをするために、テスト関数を定義して複数回呼ぶ
	testFn := func(t *testing.T) {
		_, err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
		assert.NoError(t, err)

		dep := appsv1.Deployment{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "mylocalchart-nginx",
		}, &dep)
		assert.NoError(t, err)

		// extraValueFilesから追加されたラベルが反映されていることを確認
		assert.Equal(t, "from-extra-values", dep.Spec.Template.Labels["extra-label"])
	}

	testFn(t)
	testFn(t)
}

func TestHelmfile_ConstructHelmfileVars_WithHint(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	t.Run("hint injects default values", func(t *testing.T) {
		staticValue := "my-label"
		manifest := v1.Manifest{
			Path: "testdata/helmfile-with-hint/helmfile.yaml.gotmpl",
			Helmfile: &v1.ManifestHelmfile{
				Vars: map[string]v1.HelmFileVar{
					"labelFromStatic": {
						From:   v1.HelmFileVarFromStatic,
						Static: &staticValue,
					},
				},
			},
		}
		vars, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.NoError(t, err)
		assert.Equal(t, "my-label", vars["labelFromStatic"])
		assert.Equal(t, "production", vars["environment"])
	})

	t.Run("hint errors on missing required var", func(t *testing.T) {
		manifest := v1.Manifest{
			Path: "testdata/helmfile-with-hint/helmfile.yaml.gotmpl",
			Helmfile: &v1.ManifestHelmfile{
				Vars: map[string]v1.HelmFileVar{},
			},
		}
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "required but not provided")
	})

	t.Run("hint validates type mismatch", func(t *testing.T) {
		manifest := v1.Manifest{
			Path: "testdata/helmfile-with-hint/helmfile.yaml.gotmpl",
			Helmfile: &v1.ManifestHelmfile{
				Vars: map[string]v1.HelmFileVar{
					"labelFromStatic": {
						From:        v1.HelmFileVarFromStatic,
						StaticSlice: []string{"a", "b"},
					},
				},
			},
		}
		_, err := m.ConstructHelmfileVars(context.Background(), &manifest)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "expects type string")
	})
}

func stringPtr(s string) *string {
	return &s
}
