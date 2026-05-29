//go:build integration

package runner_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// copyDir はテストフィクスチャを書き換え可能な一時ディレクトリにコピーする。
// sync mode のテストで manifest の差分を表現するため、コピー先で deployment.yaml を
// 書き換えたり削除したりして動作確認する。
func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(dst, 0o755))
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		srcPath := filepath.Join(src, e.Name())
		dstPath := filepath.Join(dst, e.Name())
		if e.IsDir() {
			copyDir(t, srcPath, dstPath)
			continue
		}
		data, err := os.ReadFile(srcPath)
		require.NoError(t, err)
		require.NoError(t, os.WriteFile(dstPath, data, 0o644))
	}
}

// makeSyncTazuna は与えられた kustomize ディレクトリパスを参照する単一 manifest の
// v1.Tazuna を組み立てる。テスト側で manifest の path を制御するのに使う。
func makeSyncTazuna(name, kustomizePath string) v1.Tazuna {
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

// TestApply_SyncMode_DiffOnly は初回 apply で全リソースが書かれ、2 回目を
// --sync で再 apply しても state エントリ集合と LastSyncedAt がいずれも変わらない
// (= 差分ゼロなのでステート保存自体が走らない) ことを縛る。
func TestApply_SyncMode_DiffOnly(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	tazuna := makeSyncTazuna("sync-manifest", "testdata/ok/kustomize")

	// 1 回目: 通常 apply
	r1 := runner.NewTazunaRunner(discardLogger(), c, nil)
	require.NoError(t, r1.ApplyToCluster(context.Background(), tazuna))

	store := state.NewConfigMapStateStore(c)
	first, err := store.Get(context.Background(), "sync-manifest")
	require.NoError(t, err)
	require.NotEmpty(t, first.Entries, "first apply must save state entries")
	firstSyncedAt := first.Metadata.LastSyncedAt

	// 2 回目: --sync で再 apply。差分ゼロなのでステート保存が走らない想定。
	r2 := runner.NewTazunaRunner(discardLogger(), c, nil,
		runner.WithApplyOptions(runner.ApplyOptions{Sync: true}))
	require.NoError(t, r2.ApplyToCluster(context.Background(), tazuna))

	second, err := store.Get(context.Background(), "sync-manifest")
	require.NoError(t, err)

	assert.Equal(t, first.Entries, second.Entries,
		"sync mode must not change state entries when there is no diff")
	assert.Equal(t, firstSyncedAt, second.Metadata.LastSyncedAt,
		"LastSyncedAt must not be updated when no diff was applied")
}

// TestApply_SyncMode_ModifiedResource は既存 state がある状態で manifest を 1 つ書き換え、
// --sync apply で modified 分だけ反映されることを縛る。
func TestApply_SyncMode_ModifiedResource(t *testing.T) {
	t.Parallel()

	// 書き換え可能な kustomize ディレクトリを用意する
	tmp := t.TempDir()
	kustomizeDir := filepath.Join(tmp, "kustomize")
	copyDir(t, "testdata/ok/kustomize", kustomizeDir)

	c := fake.NewClientBuilder().Build()
	tazuna := makeSyncTazuna("sync-modify", kustomizeDir)

	// 1 回目: 通常 apply
	r1 := runner.NewTazunaRunner(discardLogger(), c, nil)
	require.NoError(t, r1.ApplyToCluster(context.Background(), tazuna))

	store := state.NewConfigMapStateStore(c)
	first, err := store.Get(context.Background(), "sync-modify")
	require.NoError(t, err)
	require.NotEmpty(t, first.Entries)

	// Deployment の image を書き換える (Service は変えない)
	deployPath := filepath.Join(kustomizeDir, "deployment.yaml")
	modified := []byte(`apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: default
  labels:
    app: nginx
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.27.0
        ports:
        - containerPort: 80
`)
	require.NoError(t, os.WriteFile(deployPath, modified, 0o644))

	// 2 回目: --sync apply
	r2 := runner.NewTazunaRunner(discardLogger(), c, nil,
		runner.WithApplyOptions(runner.ApplyOptions{Sync: true}))
	require.NoError(t, r2.ApplyToCluster(context.Background(), tazuna))

	second, err := store.Get(context.Background(), "sync-modify")
	require.NoError(t, err)

	// state のエントリ数は同じだが、Deployment エントリの ContentHash は変わっているはず
	require.Len(t, second.Entries, len(first.Entries))
	var deployKey string
	for k := range first.Entries {
		if filepath.Base(k) == "nginx-deployment" {
			deployKey = k
			break
		}
	}
	require.NotEmpty(t, deployKey, "deployment state key must exist")
	assert.NotEqual(t, first.Entries[deployKey].ContentHash, second.Entries[deployKey].ContentHash,
		"Deployment ContentHash must change after modification")

	// 実体の Deployment にも新しい image が反映されている
	dep := &appsv1.Deployment{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "default", Name: "nginx-deployment"}, dep))
	require.Len(t, dep.Spec.Template.Spec.Containers, 1)
	assert.Equal(t, "nginx:1.27.0", dep.Spec.Template.Spec.Containers[0].Image)
}

// TestApply_SyncMode_PruneRemovedResource は manifest から service.yaml を取り除き、
// --sync --prune apply で Service が Delete されることを縛る。
func TestApply_SyncMode_PruneRemovedResource(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	kustomizeDir := filepath.Join(tmp, "kustomize")
	copyDir(t, "testdata/ok/kustomize", kustomizeDir)

	c := fake.NewClientBuilder().Build()
	tazuna := makeSyncTazuna("sync-prune", kustomizeDir)

	// 1 回目: 通常 apply で Deployment + Service が作成される
	r1 := runner.NewTazunaRunner(discardLogger(), c, nil)
	require.NoError(t, r1.ApplyToCluster(context.Background(), tazuna))

	svc := &corev1.Service{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "default", Name: "nginx"}, svc),
		"service must exist after initial apply")

	// service.yaml を kustomization.yaml から取り除き、ファイルも消す
	require.NoError(t, os.Remove(filepath.Join(kustomizeDir, "service.yaml")))
	require.NoError(t, os.WriteFile(
		filepath.Join(kustomizeDir, "kustomization.yaml"),
		[]byte("resources:\n  - deployment.yaml\n"),
		0o644,
	))

	// 2 回目: --sync --prune
	r2 := runner.NewTazunaRunner(discardLogger(), c, nil,
		runner.WithApplyOptions(runner.ApplyOptions{Sync: true, Prune: true}))
	require.NoError(t, r2.ApplyToCluster(context.Background(), tazuna))

	// Service が削除されていること
	err := c.Get(context.Background(),
		types.NamespacedName{Namespace: "default", Name: "nginx"}, &corev1.Service{})
	assert.True(t, apierrors.IsNotFound(err),
		"service must be deleted by prune, got err=%v", err)

	// state からも Service エントリが消えていること
	store := state.NewConfigMapStateStore(c)
	after, err := store.Get(context.Background(), "sync-prune")
	require.NoError(t, err)
	for key := range after.Entries {
		assert.NotContains(t, key, "/Service/", "Service state entry must be removed by prune: %s", key)
	}
}

// TestApply_SyncMode_PruneWithoutSyncErrors は --sync 無しで --prune を有効にした場合に
// Runner 層でエラーになることを縛る。
func TestApply_SyncMode_PruneWithoutSyncErrors(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil,
		runner.WithApplyOptions(runner.ApplyOptions{Sync: false, Prune: true}))

	err := r.Apply(context.Background(),
		makeSyncTazuna("invalid", "testdata/ok/kustomize"),
		"testdata/ok/tazuna.yaml")
	assert.Error(t, err, "prune without sync must be rejected")
}

// TestApply_SyncMode_AtomicMode は atomic フラグで複数 manifest の state 保存が
// 全 manifest 処理完了後にまとめて行われることを縛る。
// 「最後の保存タイミングで全部書かれる」ことを確認するため、apply 後に両方の state が
// 揃って書き込まれていることを検証する。
func TestApply_SyncMode_AtomicMode(t *testing.T) {
	t.Parallel()

	// 2 manifest 構成
	c := fake.NewClientBuilder().Build()
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "atomic-a",
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/ok/kustomize",
				},
				{
					Name: "atomic-b",
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/tags/kustomize1",
				},
			},
		},
	}

	// 初回は通常 apply で state を作っておく
	r1 := runner.NewTazunaRunner(discardLogger(), c, nil)
	require.NoError(t, r1.ApplyToCluster(context.Background(), tazuna))

	store := state.NewConfigMapStateStore(c)
	// 初回の state metadata を控える
	beforeA, err := store.Get(context.Background(), "atomic-a")
	require.NoError(t, err)
	beforeB, err := store.Get(context.Background(), "atomic-b")
	require.NoError(t, err)

	// 何も変えずに --sync --atomic で再 apply。差分ゼロなので保存はそもそも走らないため、
	// 既存の state entries は不変であることが atomic 経路でも保証される。
	r2 := runner.NewTazunaRunner(discardLogger(), c, nil,
		runner.WithApplyOptions(runner.ApplyOptions{Sync: true, Atomic: true}))
	require.NoError(t, r2.ApplyToCluster(context.Background(), tazuna))

	afterA, err := store.Get(context.Background(), "atomic-a")
	require.NoError(t, err)
	afterB, err := store.Get(context.Background(), "atomic-b")
	require.NoError(t, err)

	assert.Equal(t, beforeA.Entries, afterA.Entries,
		"atomic re-apply with no diff must not change atomic-a entries")
	assert.Equal(t, beforeB.Entries, afterB.Entries,
		"atomic re-apply with no diff must not change atomic-b entries")

	// 次に、両 manifest の Deployment image を書き換えて atomic 経路で同時保存される
	// ことを確認する。Path がテストデータ直下なので、書き換え可能なディレクトリにコピーする。
	tmp := t.TempDir()
	dirA := filepath.Join(tmp, "a")
	dirB := filepath.Join(tmp, "b")
	copyDir(t, "testdata/ok/kustomize", dirA)
	copyDir(t, "testdata/tags/kustomize1", dirB)

	modify := func(path, image string) {
		data, err := os.ReadFile(path)
		require.NoError(t, err)
		out := []byte(string(data))
		// シンプルに image: 行のみ置換する
		// nginx:1.14.2 / nginx:1.20.0 などの想定。実装は test 内のリプレースで足りる。
		_ = out
		require.NoError(t, os.WriteFile(path, []byte(image), 0o644))
	}

	// Deployment 全体を上書きしてしまうのが安全 (ファイル全文を新内容で置く)
	modify(filepath.Join(dirA, "deployment.yaml"), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx-deployment
  namespace: default
spec:
  replicas: 3
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
      - name: nginx
        image: nginx:1.29.0
        ports:
        - containerPort: 80
`)
	modify(filepath.Join(dirB, "deployment.yaml"), `apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx1
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx1
  template:
    metadata:
      labels:
        app: nginx1
    spec:
      containers:
      - name: nginx
        image: nginx:1.29.0
        ports:
        - containerPort: 80
`)

	tazuna2 := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "atomic-a", Type: v1.ManifestTypeKustomize, Path: dirA},
				{Name: "atomic-b", Type: v1.ManifestTypeKustomize, Path: dirB},
			},
		},
	}

	r3 := runner.NewTazunaRunner(discardLogger(), c, nil,
		runner.WithApplyOptions(runner.ApplyOptions{Sync: true, Atomic: true}))
	require.NoError(t, r3.ApplyToCluster(context.Background(), tazuna2))

	// atomic 経路でも両 manifest の state が確実に更新されていること
	afterA2, err := store.Get(context.Background(), "atomic-a")
	require.NoError(t, err)
	afterB2, err := store.Get(context.Background(), "atomic-b")
	require.NoError(t, err)

	assert.NotEqual(t, beforeA.Entries, afterA2.Entries,
		"atomic-a entries must change after modified apply")
	assert.NotEqual(t, beforeB.Entries, afterB2.Entries,
		"atomic-b entries must change after modified apply")
}
