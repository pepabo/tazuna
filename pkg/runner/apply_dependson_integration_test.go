//go:build integration

package runner

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// fakeDAGManager は依存解決と並列実行の挙動を検証するための manager 実装。
// Apply() の開始時刻と完了順を観測できるようフックを公開する。
type fakeDAGManager struct {
	// onApply は manifest 名を引数に取り、Apply() 開始直後に呼ばれる。
	// nil ならば何もしない。
	onApply func(name string) error
	// sleep を指定するとその時間だけ Apply() がブロックする。
	// 同一層で並列実行されることを timing で観測するために使う。
	sleep time.Duration
}

func (f *fakeDAGManager) Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) ([]client.Object, error) {
	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	if f.onApply != nil {
		return nil, f.onApply(m.Name)
	}
	return nil, nil
}

func (f *fakeDAGManager) Destroy(_ context.Context, _ *slog.Logger, _ v1.Manifest) error {
	return nil
}

func (f *fakeDAGManager) Build(_ context.Context, _ *slog.Logger, _ v1.Manifest) (string, error) {
	// state 保存用に Build() が呼ばれるが、空文字列なら 0 オブジェクトに変換され
	// state は空エントリで保存される (副作用なし)。
	return "", nil
}

func discardLoggerForDAG() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
}

// TestApply_DependsOn_OrderRespected は 3 階層のチェーンを逆順に宣言した状態で
// apply しても dependsOn が定義する順序 (a -> b -> c) で適用されることを検証する。
// 各 Apply() 呼び出し時に atomic counter を進めて記録する。
func TestApply_DependsOn_OrderRespected(t *testing.T) {
	t.Parallel()

	var counter int32
	order := make(map[string]int32, 3)
	var mu sync.Mutex

	record := func(name string) error {
		v := atomic.AddInt32(&counter, 1)
		mu.Lock()
		order[name] = v
		mu.Unlock()
		return nil
	}

	managers := map[string]manager.Manager{
		string(v1.ManifestTypeKustomize): &fakeDAGManager{onApply: record},
	}

	c := fake.NewClientBuilder().Build()
	r := NewTazunaRunner(discardLoggerForDAG(), c, nil, WithManagersOverride(managers))

	// 宣言順は逆 (c, b, a) だが dependsOn により a -> b -> c の順で適用されるはず。
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "c", Type: v1.ManifestTypeKustomize, DependsOn: []string{"b"}},
				{Name: "b", Type: v1.ManifestTypeKustomize, DependsOn: []string{"a"}},
				{Name: "a", Type: v1.ManifestTypeKustomize},
			},
		},
	}

	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))

	assert.Equal(t, int32(1), order["a"], "a must apply first")
	assert.Equal(t, int32(2), order["b"], "b must apply second")
	assert.Equal(t, int32(3), order["c"], "c must apply third")
}

// TestApply_DependsOn_ParallelInLayer は同一層に属する 2 マニフェストが
// 同時に走り始めることを timing で検証する。各 Apply() は 200ms sleep するため、
// 直列実行なら 400ms 以上かかる。並列なら 200ms ちょっとで完了する。
func TestApply_DependsOn_ParallelInLayer(t *testing.T) {
	t.Parallel()

	const sleepPerApply = 200 * time.Millisecond

	var startTimes sync.Map // name -> time.Time
	record := func(name string) error {
		startTimes.Store(name, time.Now())
		return nil
	}

	// 開始時刻を sleep 前に観測したいので、専用の fakeDAGManagerStartObs を使う。
	mgr := &fakeDAGManagerStartObs{sleep: sleepPerApply, onStart: record}

	managers := map[string]manager.Manager{
		string(v1.ManifestTypeKustomize): mgr,
	}

	c := fake.NewClientBuilder().Build()
	r := NewTazunaRunner(discardLoggerForDAG(), c, nil, WithManagersOverride(managers))

	// b と c は a に依存。a 完了後に b と c が同一層で並列に走るはず。
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "a", Type: v1.ManifestTypeKustomize},
				{Name: "b", Type: v1.ManifestTypeKustomize, DependsOn: []string{"a"}},
				{Name: "c", Type: v1.ManifestTypeKustomize, DependsOn: []string{"a"}},
			},
		},
	}

	start := time.Now()
	require.NoError(t, r.ApplyToCluster(context.Background(), tazuna))
	elapsed := time.Since(start)

	// 直列実行なら 3 * 200ms = 600ms 以上だが、a の後で b, c が並列なら
	// 概ね 400ms 強で完了する。timeout は緩めに 550ms とする。
	assert.Less(t, elapsed, 550*time.Millisecond,
		"layer 1 (b, c) must run in parallel; observed elapsed %v", elapsed)

	bStart, ok := startTimes.Load("b")
	require.True(t, ok)
	cStart, ok := startTimes.Load("c")
	require.True(t, ok)

	// b と c の開始時刻差が小さい (= 同時に走り始めた) ことを確認する。
	diff := bStart.(time.Time).Sub(cStart.(time.Time))
	if diff < 0 {
		diff = -diff
	}
	assert.Less(t, diff, 100*time.Millisecond,
		"b and c in the same layer must start nearly simultaneously; diff=%v", diff)
}

// fakeDAGManagerStartObs は Apply 開始時刻を観測するための manager。
// onStart が sleep の前に呼ばれる点が fakeDAGManager との違い。
type fakeDAGManagerStartObs struct {
	sleep   time.Duration
	onStart func(name string) error
}

func (f *fakeDAGManagerStartObs) Apply(_ context.Context, _ *slog.Logger, m v1.Manifest) ([]client.Object, error) {
	if f.onStart != nil {
		if err := f.onStart(m.Name); err != nil {
			return nil, err
		}
	}
	if f.sleep > 0 {
		time.Sleep(f.sleep)
	}
	return nil, nil
}

func (f *fakeDAGManagerStartObs) Destroy(_ context.Context, _ *slog.Logger, _ v1.Manifest) error {
	return nil
}

func (f *fakeDAGManagerStartObs) Build(_ context.Context, _ *slog.Logger, _ v1.Manifest) (string, error) {
	return "", nil
}

// TestApply_DependsOn_LayerFailureStopsNextLayer は層 N のマニフェスト適用で
// エラーが発生したとき、層 N+1 に属するマニフェストは適用されないことを縛る。
func TestApply_DependsOn_LayerFailureStopsNextLayer(t *testing.T) {
	t.Parallel()

	var aApplied, bApplied, cApplied atomic.Bool

	failingManager := &fakeDAGManager{
		onApply: func(name string) error {
			switch name {
			case "a":
				aApplied.Store(true)
				return nil
			case "b":
				bApplied.Store(true)
				return errors.New("simulated failure in layer 1")
			case "c":
				cApplied.Store(true)
				return nil
			}
			return nil
		},
	}

	managers := map[string]manager.Manager{
		string(v1.ManifestTypeKustomize): failingManager,
	}

	c := fake.NewClientBuilder().Build()
	r := NewTazunaRunner(discardLoggerForDAG(), c, nil, WithManagersOverride(managers))

	// 層 0: a, 層 1: b (これがエラー), 層 2: c
	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "a", Type: v1.ManifestTypeKustomize},
				{Name: "b", Type: v1.ManifestTypeKustomize, DependsOn: []string{"a"}},
				{Name: "c", Type: v1.ManifestTypeKustomize, DependsOn: []string{"b"}},
			},
		},
	}

	err := r.ApplyToCluster(context.Background(), tazuna)
	require.Error(t, err, "ApplyToCluster must propagate layer failure")
	assert.Contains(t, err.Error(), "layer 1 apply failed")

	assert.True(t, aApplied.Load(), "layer 0 (a) must have been applied")
	assert.True(t, bApplied.Load(), "layer 1 (b) must have been attempted")
	assert.False(t, cApplied.Load(), "layer 2 (c) must NOT be applied after layer 1 failure")
}
