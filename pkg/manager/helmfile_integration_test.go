//go:build integration

package manager_test

import (
	"context"
	"os"
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

// TestHelmfile_Build_OCIChart は chart: oci://... 参照が helm registry client 経由で
// pull され、render できることを検証する。public ECR への network access を要するため
// integration タグ下でのみ実行する。
func TestHelmfile_Build_OCIChart(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-oci/helmfile.yaml",
		Helmfile: &v1.ManifestHelmfile{
			IncludeCRDs: true,
		},
	}

	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	// karpenter-crd チャートは CRD を含むため、render 結果に CRD が現れる。
	assert.Contains(t, out, "kind: CustomResourceDefinition")
	assert.Contains(t, out, "karpenter")
}

// TestHelmfile_Build_HTTPRepoChart は repositories[] で宣言した HTTP(S) repository の
// `<alias>/<chart>` 参照が index.yaml 経由で pull され、render できることを検証する。
// 報告のあった argo-cd (https://argoproj.github.io/argo-helm) の構成を再現する。
// public HTTP repository への network access を要するため integration タグ下でのみ実行する。
func TestHelmfile_Build_HTTPRepoChart(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-http-repo/helmfile.yaml",
		Helmfile: &v1.ManifestHelmfile{
			// argo-cd チャートは kubeVersion >=1.25.0 を要求するため明示的に指定する
			// (helm のデフォルト v1.20.0 では render に失敗する)。
			KubeVersion: "1.30.0",
		},
	}

	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	// argo-cd チャートは各種 Deployment を含むため、render 結果に現れる。
	assert.Contains(t, out, "kind: Deployment")
	assert.Contains(t, out, "argocd")
}

// TestHelmfile_Build_HTTPRepoChart_IgnoresLocalCwdCollision は repositories[] で
// 明示的に宣言された HTTP(S) repository の `<alias>/<chart>` 参照が、たまたま
// プロセスのカレントディレクトリに同名のディレクトリ (chart とは無関係のもの、
// 例えば kustomize 用マニフェスト置き場) が存在していても repository からの
// pull を優先することを検証する。
//
// helm.sh/helm/v3/pkg/action.ChartPathOptions.LocateChart は「cwd に chartName
// と同名のファイル/ディレクトリがあれば repository を無視してそれをローカル
// chart として扱う」という互換動作 (helm issue #7862) を内蔵しており、tazuna
// build を実行する manifests リポジトリ側にたまたま同名ディレクトリが存在する
// だけで `Chart.yaml file is missing` エラーになる実例が報告されている。
func TestHelmfile_Build_HTTPRepoChart_IgnoresLocalCwdCollision(t *testing.T) {
	// chart 名 "argo-cd" と同名の、Chart.yaml を持たないディレクトリを
	// プロセスの cwd (このテストバイナリの実行ディレクトリ = pkg/manager) に作る。
	collidingDir := "argo-cd"
	if _, err := os.Stat(collidingDir); err == nil {
		t.Fatalf("%s already exists in cwd; refusing to overwrite", collidingDir)
	}
	if err := os.Mkdir(collidingDir, 0o755); err != nil {
		t.Fatalf("failed to create colliding dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(collidingDir)
	})

	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-http-repo/helmfile.yaml",
		Helmfile: &v1.ManifestHelmfile{
			KubeVersion: "1.30.0",
		},
	}

	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	assert.Contains(t, out, "kind: Deployment")
	assert.Contains(t, out, "argocd")
}

// TestHelmfile_Build_OCIRepoChart は repositories[] で宣言した OCI repository の
// `<alias>/<chart>` 参照が helm registry client 経由で pull され、render できることを
// 検証する。public ECR への network access を要するため integration タグ下でのみ実行する。
func TestHelmfile_Build_OCIRepoChart(t *testing.T) {
	client := fake.NewFakeClient()
	m := manager.NewHelmfile(client, nil)

	manifest := v1.Manifest{
		Path: "testdata/helmfile-oci-repo/helmfile.yaml",
		Helmfile: &v1.ManifestHelmfile{
			IncludeCRDs: true,
		},
	}

	out, err := m.Build(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
	assert.NoError(t, err)
	assert.Contains(t, out, "kind: CustomResourceDefinition")
	assert.Contains(t, out, "karpenter")
}

func stringPtr(s string) *string {
	return &s
}
