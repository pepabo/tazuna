//go:build integration

package runner_test

import (
	"bytes"
	"context"
	"log/slog"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// makeStatusTazuna は status テスト用に単一 manifest の v1.Tazuna を組み立てる。
func makeStatusTazuna(name string) v1.Tazuna {
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

// seedConfigMapState は ConfigMap を 2 個 fake client に直接 Create し、
// 対応する state entry を保存する。ConfigMap は readiness 判定では即 Ready 扱いになるため、
// "全部 Ready になる" ケースの検証に使う。
func seedConfigMapState(t *testing.T, c client.Client, manifestName string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, state.EnsureNamespace(ctx, c))

	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-one", Namespace: "default"},
		Data:       map[string]string{"k": "v1"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "cm-two", Namespace: "default"},
		Data:       map[string]string{"k": "v2"},
	}
	require.NoError(t, c.Create(ctx, cm1))
	require.NoError(t, c.Create(ctx, cm2))

	store := state.NewConfigMapStateStore(c)
	entries := map[string]state.StateEntry{}
	for _, name := range []string{"cm-one", "cm-two"} {
		key := state.StateKey{
			ManifestName: manifestName,
			Group:        "",
			Version:      "v1",
			Kind:         "ConfigMap",
			Namespace:    "default",
			Name:         name,
		}
		entries[key.String()] = state.StateEntry{ContentHash: "dummy"}
	}
	require.NoError(t, store.Save(ctx, manifestName, &state.StateData{
		Metadata: state.StateMetadata{LastSyncedAt: time.Now().UTC().Format(time.RFC3339)},
		Entries:  entries,
	}))
}

// TestStatus_AllReady は ConfigMap を 2 個 apply (即 Ready 扱い) した後で
// Status を実行し、両方 Ready と表示されることを確認する。
func TestStatus_AllReady(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	seedConfigMapState(t, c, "cm-manifest")

	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeStatusTazuna("cm-manifest")

	var buf bytes.Buffer
	err := r.Status(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: cm-manifest")
	assert.Contains(t, out, string(runner.ResourceStatusReady))
	assert.Contains(t, out, "cm-one")
	assert.Contains(t, out, "cm-two")
	assert.NotContains(t, out, string(runner.ResourceStatusMissing))
	assert.NotContains(t, out, string(runner.ResourceStatusNotReady))
}

// TestStatus_Missing は state に記録されたリソースを直接 Delete した場合、
// Status で Missing と表示されることを確認する。
func TestStatus_Missing(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	seedConfigMapState(t, c, "cm-manifest")

	// ライブの ConfigMap を直接削除する
	cm := &corev1.ConfigMap{}
	require.NoError(t, c.Get(context.Background(),
		types.NamespacedName{Namespace: "default", Name: "cm-one"}, cm))
	require.NoError(t, c.Delete(context.Background(), cm))

	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeStatusTazuna("cm-manifest")

	var buf bytes.Buffer
	err := r.Status(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: cm-manifest")
	assert.Contains(t, out, string(runner.ResourceStatusMissing))
	assert.Contains(t, out, "cm-one")
}

// TestStatus_NotReady は Deployment{spec.replicas: 3, status.readyReplicas: 0} を
// fake client に直接 Put し、state に対応エントリを書いて、Status で NotReady と
// 判定されることを確認する。
func TestStatus_NotReady(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	ctx := context.Background()
	require.NoError(t, state.EnsureNamespace(ctx, c))

	// spec.replicas=3 だが readyReplicas は 0 のままなので NotReady
	replicas := int32(3)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "not-ready-dep", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
		},
	}
	require.NoError(t, c.Create(ctx, dep))

	store := state.NewConfigMapStateStore(c)
	key := state.StateKey{
		ManifestName: "dep-manifest",
		Group:        "apps",
		Version:      "v1",
		Kind:         "Deployment",
		Namespace:    "default",
		Name:         "not-ready-dep",
	}
	require.NoError(t, store.Save(ctx, "dep-manifest", &state.StateData{
		Metadata: state.StateMetadata{LastSyncedAt: time.Now().UTC().Format(time.RFC3339)},
		Entries: map[string]state.StateEntry{
			key.String(): {ContentHash: "dummy"},
		},
	}))

	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeStatusTazuna("dep-manifest")

	var buf bytes.Buffer
	err := r.Status(ctx, tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: dep-manifest")
	assert.Contains(t, out, string(runner.ResourceStatusNotReady))
	assert.Contains(t, out, "Deployment")
	assert.Contains(t, out, "not-ready-dep")
}

// TestStatus_NoState は state ConfigMap が無い manifest に対し
// "(no state)" が出力されることを確認する。
func TestStatus_NoState(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := makeStatusTazuna("no-state-manifest")

	var buf bytes.Buffer
	err := r.Status(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.Contains(t, out, "Manifest: no-state-manifest")
	assert.Contains(t, out, "(no state)")
}

// TestStatus_SkipParallel は parallel manifest がスキップされ警告ログが出ることを確認する。
func TestStatus_SkipParallel(t *testing.T) {
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
	err := r.Status(context.Background(), tazuna,
		filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)
	require.NoError(t, err)

	out := buf.String()
	assert.NotContains(t, out, "Manifest: parallel-parent")

	assert.Contains(t, logBuf.String(), "parallel manifest is not supported for status",
		"warn log for parallel manifest should be emitted")
}
