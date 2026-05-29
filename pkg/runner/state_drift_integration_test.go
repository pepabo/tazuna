//go:build integration

package runner_test

import (
	"bytes"
	"context"
	"path/filepath"
	"testing"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// makeDriftTazuna は drift テスト用に kustomize ディレクトリパスを参照する単一 manifest の
// v1.Tazuna を組み立てる。
func makeDriftTazuna(name, kustomizePath string) v1.Tazuna {
	return v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: name,
					Type: v1.ManifestTypeKustomize,
					Path: kustomizePath,
				},
			},
		},
	}
}

// seedLiveStateForDrift はライブクラスタに直接 Deployment と Service を作り、それぞれから
// 算出した ContentHash を state ConfigMap に保存する。
// apply 経由ではなくライブ由来でハッシュを揃えることで、drift 検出ロジックそのものを
// fake client の typed-default 挙動から切り離して検証する。
func seedLiveStateForDrift(t *testing.T, c client.Client, manifestName string) {
	t.Helper()
	ctx := context.Background()

	require.NoError(t, state.EnsureNamespace(ctx, c))

	dep := &appsv1.Deployment{}
	dep.SetName("nginx-deployment")
	dep.SetNamespace("default")
	dep.Spec.Replicas = func(i int32) *int32 { return &i }(3)
	require.NoError(t, c.Create(ctx, dep))

	svc := &corev1.Service{}
	svc.SetName("nginx")
	svc.SetNamespace("default")
	svc.Spec.Selector = map[string]string{"app": "nginx"}
	svc.Spec.Ports = []corev1.ServicePort{{Port: 80}}
	require.NoError(t, c.Create(ctx, svc))

	// ライブから取得した unstructured でハッシュ算出 → state を作る
	store := state.NewConfigMapStateStore(c)
	entries := map[string]state.StateEntry{}
	for _, ref := range []struct {
		gv, kind, ns, name string
	}{
		{"apps/v1", "Deployment", "default", "nginx-deployment"},
		{"v1", "Service", "default", "nginx"},
	} {
		obj := &unstructured.Unstructured{}
		obj.SetAPIVersion(ref.gv)
		obj.SetKind(ref.kind)
		require.NoError(t, c.Get(ctx, types.NamespacedName{Namespace: ref.ns, Name: ref.name}, obj))
		hash, err := state.ComputeContentHash(obj)
		require.NoError(t, err)
		key := state.NewStateKey(manifestName, obj)
		entries[key.String()] = state.StateEntry{ContentHash: hash}
	}

	require.NoError(t, store.Save(ctx, manifestName, &state.StateData{
		Metadata: state.StateMetadata{
			LastSyncedAt: time.Now().UTC().Format(time.RFC3339),
		},
		Entries: entries,
	}))
}

// TestStateDrift_NoDrift は state とライブが一致する状態で drift を実行した場合に
// "No drift detected." が出力されることを縛る。
func TestStateDrift_NoDrift(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	seedLiveStateForDrift(t, c, "kustomize")

	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeDriftTazuna("kustomize", "testdata/ok/kustomize")

	var buf bytes.Buffer
	err := r.StateDrift(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "No drift detected.")
	assert.NotContains(t, out, "live-drifted")
	assert.NotContains(t, out, "live-missing")
}

// TestStateDrift_LiveDrifted はライブリソースを直接書き換えた場合、
// drift で live-drifted として検出されることを縛る。
func TestStateDrift_LiveDrifted(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	seedLiveStateForDrift(t, c, "kustomize")

	// ライブの Deployment を直接書き換える (手動 kubectl apply 相当)
	dep := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "default", Name: "nginx-deployment"}, dep))
	replicas := int32(9)
	dep.Spec.Replicas = &replicas
	require.NoError(t, c.Update(context.Background(), dep))

	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeDriftTazuna("kustomize", "testdata/ok/kustomize")

	var buf bytes.Buffer
	err := r.StateDrift(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: kustomize")
	assert.Contains(t, out, string(runner.DriftTypeDrifted))
	assert.Contains(t, out, "nginx-deployment")
	assert.NotContains(t, out, "No drift detected.")
}

// TestStateDrift_LiveMissing はライブリソースを直接 Delete した場合、
// drift で live-missing として検出されることを縛る。
func TestStateDrift_LiveMissing(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	seedLiveStateForDrift(t, c, "kustomize")

	// ライブの Service を直接削除する (手動 kubectl delete 相当)
	svc := &corev1.Service{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "default", Name: "nginx"}, svc))
	require.NoError(t, c.Delete(context.Background(), svc))

	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeDriftTazuna("kustomize", "testdata/ok/kustomize")

	var buf bytes.Buffer
	err := r.StateDrift(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: kustomize")
	assert.Contains(t, out, string(runner.DriftTypeMissing))
	assert.Contains(t, out, "Service")
	assert.NotContains(t, out, "No drift detected.")
}

// TestStateDrift_NoStateNoOp は state ConfigMap が空の manifest が drift 検知の対象外であり、
// "No drift detected." が出力されることを縛る。
func TestStateDrift_NoStateNoOp(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)

	// apply を一切実行しないので state ConfigMap は空のまま
	tazuna := makeDriftTazuna("no-state-manifest", "testdata/ok/kustomize")

	var buf bytes.Buffer
	err := r.StateDrift(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "No drift detected.")
	assert.NotContains(t, out, "Manifest: no-state-manifest")
}

// TestStateDrift_SkipGenesisSecret は GenesisSecret manifest がスキップされることを縛る。
// GenesisSecret は always-sync 扱いであり drift という概念を持たない。
func TestStateDrift_SkipGenesisSecret(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)

	// 先に GenesisSecret を apply して state を保存しておく。
	// その後 drift を実行しても GenesisSecret は drift 対象外であることを確認する。
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
	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	var buf bytes.Buffer
	err := r.StateDrift(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "No drift detected.")
	assert.NotContains(t, out, "Manifest: genesis-secret-manifest")
	assert.NotContains(t, out, "live-drifted")
	assert.NotContains(t, out, "live-missing")
}
