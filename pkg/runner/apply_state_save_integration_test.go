//go:build integration

package runner_test

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

// discardLogger は副作用のないロガーを返す
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

// loadTazunaForTest はテスト用にtazuna.yamlを読み込み、manifestパスをcwdからの相対パスに変換する
func loadTazunaForTest(t *testing.T, r *runner.TazunaRunner, path string) v1.Tazuna {
	t.Helper()
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	tazuna := v1.Tazuna{}
	require.NoError(t, yaml.Unmarshal(data, &tazuna))
	r.ConvertManifestPathFromCwd(filepath.Dir(path), &tazuna)
	return tazuna
}

// TestApplyToCluster_StateSaved_HappyPath_Kustomize は kustomize manifest を apply 後に
// state ConfigMap が manifest 単位で正しく書き込まれることを保証する。
// 各 Entry の ContentHash が空でないこと・LastSyncedAt が RFC3339 でパース可能なこと・
// state key が NewStateKey 由来のフォーマット (namespaced は6パート) であることを縛る。
func TestApplyToCluster_StateSaved_HappyPath_Kustomize(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := loadTazunaForTest(t, r, "testdata/ok/tazuna.yaml")

	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	store := state.NewConfigMapStateStore(c)
	data, err := store.Get(context.Background(), "kustomize")
	require.NoError(t, err, "state for manifest %q should be retrievable", "kustomize")

	// kustomize manifest からは少なくとも Deployment と Service の2エントリが書かれる
	assert.NotEmpty(t, data.Entries, "state entries must not be empty after apply")
	assert.GreaterOrEqual(t, len(data.Entries), 2, "expected at least Deployment and Service entries")

	for key, entry := range data.Entries {
		assert.NotEmpty(t, entry.ContentHash, "content hash must not be empty for %s", key)
		parsed, perr := state.ParseStateKey(key)
		require.NoError(t, perr, "state key must be parseable: %s", key)
		assert.Equal(t, "kustomize", parsed.ManifestName, "manifest name in key must match")
		// kustomize 配下は namespaced (default ns) のみなので6パート
		parts := strings.Split(key, "/")
		assert.Len(t, parts, 6, "namespaced state key must be 6 parts: %s", key)
	}

	// LastSyncedAt が RFC3339 でパース可能
	_, terr := time.Parse(time.RFC3339, data.Metadata.LastSyncedAt)
	assert.NoError(t, terr, "LastSyncedAt must be RFC3339 format: %q", data.Metadata.LastSyncedAt)
}

// TestApplyToCluster_StateSaved_EmptyManifestName は manifest.Name が空のときに
// state ConfigMap が一切書かれないこと (saveManifestState がスキップする) を縛る。
// また警告ログが出ることもベストエフォートで確認する。
func TestApplyToCluster_StateSaved_EmptyManifestName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	logBuf := &bytes.Buffer{}
	logger := slog.New(slog.NewTextHandler(logBuf, &slog.HandlerOptions{Level: slog.LevelDebug}))
	r := runner.NewTazunaRunner(logger, c, nil)

	// 名前なしの kustomize manifest を直接組み立てる
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "",
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/ok/kustomize",
				},
			},
		},
	}

	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	// 名前なしなので "" で Get しても空であること
	store := state.NewConfigMapStateStore(c)
	data, err := store.Get(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, data.Entries, "no state must be saved when manifest.Name is empty")

	// 念のため tazuna namespace 配下の ConfigMap 一覧を見て、state ConfigMap が存在しないことも確認
	cms := &corev1.ConfigMapList{}
	require.NoError(t, c.List(context.Background(), cms, client.InNamespace(state.TazunaNamespace)))
	for _, cm := range cms.Items {
		assert.NotContains(t, cm.Name, "tazuna-state-", "no tazuna-state-* ConfigMap should exist, but found %s", cm.Name)
	}

	// 警告ログが出ていること (slog の TextHandler は key=value で出力するので部分一致でゆるく確認)
	assert.Contains(t, logBuf.String(), "manifest has no name", "warn log for empty manifest name should be emitted")
}

// TestApplyToCluster_StateSaved_MultipleManifestsIndependent は複数 manifest が
// それぞれ別の tazuna-state-<name> ConfigMap に独立して保存されることを縛る。
func TestApplyToCluster_StateSaved_MultipleManifestsIndependent(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)

	// 既存 testdata の異なる kustomize ディレクトリを 2 つ流用する。
	// それぞれに独自の Deployment が含まれているため、エントリ集合は独立する。
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "manifest-a",
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/ok/kustomize",
				},
				{
					Name: "manifest-b",
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/tags/kustomize1",
				},
			},
		},
	}

	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	store := state.NewConfigMapStateStore(c)

	a, err := store.Get(context.Background(), "manifest-a")
	require.NoError(t, err)
	b, err := store.Get(context.Background(), "manifest-b")
	require.NoError(t, err)

	assert.NotEmpty(t, a.Entries, "manifest-a must have state entries")
	assert.NotEmpty(t, b.Entries, "manifest-b must have state entries")

	// 各エントリのキーには自分の manifest 名が prefix として含まれる
	for key := range a.Entries {
		assert.True(t, strings.HasPrefix(key, "manifest-a/"),
			"manifest-a state key must be prefixed with its manifest name: %s", key)
	}
	for key := range b.Entries {
		assert.True(t, strings.HasPrefix(key, "manifest-b/"),
			"manifest-b state key must be prefixed with its manifest name: %s", key)
	}

	// 物理的にも別 ConfigMap になっている
	cms := &corev1.ConfigMapList{}
	require.NoError(t, c.List(context.Background(), cms, client.InNamespace(state.TazunaNamespace)))
	cmNames := map[string]bool{}
	for _, cm := range cms.Items {
		cmNames[cm.Name] = true
	}
	assert.True(t, cmNames["tazuna-state-manifest-a"], "ConfigMap for manifest-a must exist")
	assert.True(t, cmNames["tazuna-state-manifest-b"], "ConfigMap for manifest-b must exist")
}

// TestApplyToCluster_StateSaved_IdempotentOnReapply は同じ manifest を 2 回 apply しても
// state のエントリ集合 (key と ContentHash) が完全に一致することを縛る。
// LastSyncedAt は更新されるので比較から外す。
func TestApplyToCluster_StateSaved_IdempotentOnReapply(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)
	tazuna := loadTazunaForTest(t, r, "testdata/ok/tazuna.yaml")

	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	store := state.NewConfigMapStateStore(c)
	first, err := store.Get(context.Background(), "kustomize")
	require.NoError(t, err)
	require.NotEmpty(t, first.Entries)

	// 2 回目の apply
	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	second, err := store.Get(context.Background(), "kustomize")
	require.NoError(t, err)

	// エントリ集合 (key と hash) が完全に一致すること
	assert.Equal(t, first.Entries, second.Entries,
		"state entries must be identical across re-applies for idempotency")
}

// TestApplyToCluster_StateSaved_ApplyFailureNoState は apply 自体が失敗する manifest で
// state ConfigMap が作られないことを縛る。
func TestApplyToCluster_StateSaved_ApplyFailureNoState(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)

	// kustomize で存在しないパスを指定 → kustomizer.Run がエラーになり Apply が失敗する。
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{
					Name: "broken-manifest",
					Type: v1.ManifestTypeKustomize,
					Path: "testdata/does-not-exist-on-purpose",
				},
			},
		},
	}

	err := r.ApplyToCluster(context.Background(), tazuna)
	assert.Error(t, err, "ApplyToCluster must fail when manifest path does not exist")

	// state ConfigMap が作られていないこと (NotFound 相当 = empty StateData)
	store := state.NewConfigMapStateStore(c)
	data, gerr := store.Get(context.Background(), "broken-manifest")
	require.NoError(t, gerr)
	assert.Empty(t, data.Entries, "no state must be saved when apply fails")

	// ConfigMap が物理的にも存在しないこと
	cms := &corev1.ConfigMapList{}
	require.NoError(t, c.List(context.Background(), cms, client.InNamespace(state.TazunaNamespace)))
	for _, cm := range cms.Items {
		assert.NotEqual(t, "tazuna-state-broken-manifest", cm.Name,
			"failed manifest must not produce a state ConfigMap")
	}
}

// TestApplyToCluster_StateSaved_GenesisSecret は GenesisSecret manifest を opClient nil /
// secrets 空 / outputs ありの状態で apply して state が書かれること、
// かつ state key の Kind が "Secret" になることを縛る。
func TestApplyToCluster_StateSaved_GenesisSecret(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	r := runner.NewTazunaRunner(discardLogger(), c, nil)

	// 既存 fixture (testdata/include/secret.yaml) を直接参照する単純な v1.Tazuna を組む。
	// secret.yaml には .spec.secrets が無く outputs だけなので、opClient nil でも問題なく動く。
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

	store := state.NewConfigMapStateStore(c)
	data, err := store.Get(context.Background(), "genesis-secret-manifest")
	require.NoError(t, err)
	require.NotEmpty(t, data.Entries, "GenesisSecret manifest must produce at least one state entry")

	// state key の Kind が "Secret" であること
	foundSecretKind := false
	for key, entry := range data.Entries {
		assert.NotEmpty(t, entry.ContentHash, "content hash must not be empty for %s", key)
		parsed, perr := state.ParseStateKey(key)
		require.NoError(t, perr, "state key must be parseable: %s", key)
		if parsed.Kind == "Secret" {
			foundSecretKind = true
		}
	}
	assert.True(t, foundSecretKind, "at least one state entry must have Kind=Secret")

	// 副作用として fake client 上に Secret が実体化していることもラフに確認
	// (genesis_secret manager は controllerutil.CreateOrUpdate で作る)
	secret := &corev1.Secret{}
	gerr := c.Get(context.Background(),
		client.ObjectKey{Namespace: "default", Name: "test-secret-expanded"}, secret)
	if meta.IsNoMatchError(gerr) {
		t.Skip("fake client scheme does not contain Secret kind; skipping")
	}
	assert.NoError(t, gerr, "secret object should be created in cluster by genesissecret manager")
}
