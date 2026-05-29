//go:build integration

package runner_test

import (
	"bytes"
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/pepabo/tazuna/pkg/manifest"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// planBuildManager は plan のテスト専用の manager で、Build() が任意の YAML を返す。
// kustomize 等の本物の manager を介さず、レンダリング結果を直接固定するため、
// fake client 経由で確認したい diff の挙動だけを孤立して検証できる。
type planBuildManager struct {
	buildOut string
}

func (f *planBuildManager) Apply(_ context.Context, _ *slog.Logger, _ v1.Manifest) error {
	return nil
}

func (f *planBuildManager) Destroy(_ context.Context, _ *slog.Logger, _ v1.Manifest) error {
	return nil
}

func (f *planBuildManager) Build(_ context.Context, _ *slog.Logger, _ v1.Manifest) (string, error) {
	return f.buildOut, nil
}

// makePlanKustomizeTazuna は plan テスト用に kustomize 型 manifest 1 件のみを持つ
// v1.Tazuna を組み立てる。Path はテスト用 manager が無視するため何でも良い。
func makePlanKustomizeTazuna(name string) v1.Tazuna {
	return v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: name,
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/ok/kustomize",
				},
			},
		},
	}
}

// TestPlan_NewResource はライブクラスタに存在しないリソースが Build() 出力に
// 含まれている場合、"+ Kind/ns/name (to be created)" が出力されることを縛る。
func TestPlan_NewResource(t *testing.T) {
	t.Parallel()

	yaml := `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: default
spec:
  replicas: 3
`
	managers := map[string]manager.Manager{
		string(v1.ManifestTypeKustomize): &planBuildManager{buildOut: yaml},
	}

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil, runner.WithManagersOverride(managers))
	tazuna := makePlanKustomizeTazuna("kustomize")

	var buf bytes.Buffer
	err := r.Plan(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: kustomize")
	assert.Contains(t, out, "+ Deployment/default/nginx-deployment (to be created)")
	assert.NotContains(t, out, "No changes detected.")
}

// TestPlan_ModifiedField はライブクラスタに既存のリソースを置き、Build() 出力で
// 1 フィールドだけ違う desired を宣言したとき、"~ Kind/ns/name" と unified diff
// にそのフィールドが含まれることを縛る。
//
// 注: Deployment 等 typed-known kind は fake client の tracker が defaulting で
// selector/strategy などを補ってしまい、replicas 以外にも差分が乗ってしまう。
// 「狙ったフィールドだけ差分になる」ことを確認するため、defaulting がほぼ無い
// ConfigMap で検証する。
func TestPlan_ModifiedField(t *testing.T) {
	t.Parallel()

	// live: data.replicas="1"
	liveYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  replicas: "1"
`
	c := fake.NewClientBuilder().Build()
	seedUnstructured(t, c, liveYAML)

	// desired: data.replicas="3" (差分は replicas のみ)
	desiredYAML := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  replicas: "3"
`
	managers := map[string]manager.Manager{
		string(v1.ManifestTypeKustomize): &planBuildManager{buildOut: desiredYAML},
	}

	r := runner.NewTazunaRunner(discardLogger(), c, nil, runner.WithManagersOverride(managers))
	tazuna := makePlanKustomizeTazuna("kustomize")

	var buf bytes.Buffer
	err := r.Plan(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: kustomize")
	assert.Contains(t, out, "~ ConfigMap/default/app-config")
	// unified diff で replicas の変更が含まれることを確認する
	assert.Contains(t, out, "replicas")
	// 旧値 ("1") から新値 ("3") への遷移であることをそれぞれ確認
	assert.Contains(t, out, `-  "replicas": "1"`)
	assert.Contains(t, out, `+  "replicas": "3"`)
	assert.NotContains(t, out, "to be created")
	assert.NotContains(t, out, "No changes detected.")
}

// TestPlan_NoChange はライブクラスタが desired と完全一致するとき、
// "No changes detected." だけが出力され manifest ヘッダが出ないことを縛る。
//
// 注: fake client の tracker は typed-known kind (Deployment 等) に対し
// unstructured で Create しても typed defaulting で selector/strategy 等を
// nil として埋めるため、Deployment では live と desired が一致しない。
// ConfigMap は単純な data フィールドのみで defaulting がほぼ無いため、
// "完全一致" の検証に向いている。
func TestPlan_NoChange(t *testing.T) {
	t.Parallel()

	yaml := `apiVersion: v1
kind: ConfigMap
metadata:
  name: app-config
  namespace: default
data:
  foo: bar
`
	c := fake.NewClientBuilder().Build()
	// live にも同じオブジェクトを作成して desired と一致させる
	seedUnstructured(t, c, yaml)

	managers := map[string]manager.Manager{
		string(v1.ManifestTypeKustomize): &planBuildManager{buildOut: yaml},
	}

	r := runner.NewTazunaRunner(discardLogger(), c, nil, runner.WithManagersOverride(managers))
	tazuna := makePlanKustomizeTazuna("kustomize")

	var buf bytes.Buffer
	err := r.Plan(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "No changes detected.")
	assert.NotContains(t, out, "Manifest: kustomize")
}

// TestPlan_SkipParallel は parallel manifest がスキップされ警告ログが出ることを縛る。
func TestPlan_SkipParallel(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	logBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := runner.NewTazunaRunner(logger, c, nil)

	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "parallel-parent",
					Type: v1.ManifestTypeParallel,
					Parallel: &v1.ManifestParallel{
						Children: []v1.Manifest{
							{
								Name: "parallel-child-kustomize",
								Type: v1.ManifestTypeKustomize,
								Path: "testdata/ok/kustomize",
							},
						},
					},
				},
			},
		},
	}

	var buf bytes.Buffer
	err := r.Plan(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "No changes detected.")
	assert.NotContains(t, out, "Manifest: parallel-parent")

	assert.Contains(t, logBuf.String(), "parallel manifest is not supported for plan",
		"warn log for parallel manifest should be emitted")
}

// TestPlan_SkipGenesisSecret は GenesisSecret manifest がスキップされ、
// plan の対象外であることを縛る。GenesisSecret は always-sync 扱いのため
// 「事前にフィールド差分を見る」という plan の概念に当てはまらない。
func TestPlan_SkipGenesisSecret(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	logBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := runner.NewTazunaRunner(logger, c, nil)

	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "genesis-secret-manifest",
					Type: v1.ManifestTypeGenesisSecret,
					Path: "testdata/include/secret.yaml",
				},
			},
		},
	}

	var buf bytes.Buffer
	err := r.Plan(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "No changes detected.")
	assert.NotContains(t, out, "Manifest: genesis-secret-manifest")
	assert.NotContains(t, out, "to be created")

	assert.Contains(t, logBuf.String(), "genesis-secret manifest is always-sync",
		"debug log noting GenesisSecret is skipped should be emitted")
}

// seedUnstructured は YAML をパースして unstructured で live に Create する。
// Build() 出力と同じ Unstructured 経路で揃えるため、テストの "完全一致" 条件を
// 作るのに使う。
func seedUnstructured(t *testing.T, c client.Client, yamlStr string) {
	t.Helper()
	objects, err := manifest.ConvertManifestsToObjects([]byte(yamlStr), "")
	require.NoError(t, err)
	ctx := context.Background()
	for _, obj := range objects {
		require.NoError(t, c.Create(ctx, obj))
	}
}
