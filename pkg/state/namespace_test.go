package state_test

import (
	"context"
	"testing"

	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
)

// TestEnsureNamespace_ConcurrentCreateRace は Get で NotFound → Create で
// AlreadyExists となるレース（別 goroutine が先に作成した場合）をシミュレートし、
// EnsureNamespace がエラーにならないことを確認する。
func TestEnsureNamespace_ConcurrentCreateRace(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().WithInterceptorFuncs(interceptor.Funcs{
		Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*corev1.Namespace); ok {
				return apierrors.NewAlreadyExists(
					schema.GroupResource{Resource: "namespaces"}, obj.GetName())
			}
			return cl.Create(ctx, obj, opts...)
		},
	}).Build()

	err := state.EnsureNamespace(context.Background(), c)
	assert.NoError(t, err)
}

// TestConfigMapStateStore_Save_ConcurrentBootstrap は namespace 未作成の状態から
// 複数 goroutine が同時に Save した場合でも全 Save が成功することを確認する
// (dependsOn 使用時の同一層並列 apply による bootstrap のシミュレーション)。
func TestConfigMapStateStore_Save_ConcurrentBootstrap(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	store := state.NewConfigMapStateStore(c)

	const workers = 8
	errs := make(chan error, workers)
	for i := range workers {
		name := string(rune('a' + i))
		go func() {
			errs <- store.Save(context.Background(), "manifest-"+name, &state.StateData{
				Entries: map[string]state.StateEntry{},
			})
		}()
	}
	for range workers {
		assert.NoError(t, <-errs)
	}
}
