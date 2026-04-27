//go:build integration

package state_test

import (
	"context"
	"testing"

	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestEnsureNamespace_Create(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()

	err := state.EnsureNamespace(context.Background(), c)
	assert.NoError(t, err)

	// namespaceが作成されたことを確認
	ns := &corev1.Namespace{}
	err = c.Get(context.Background(), client.ObjectKey{Name: state.TazunaNamespace}, ns)
	assert.NoError(t, err)
	assert.Equal(t, state.TazunaNamespace, ns.Name)
}

func TestEnsureNamespace_AlreadyExists(t *testing.T) {
	t.Parallel()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: state.TazunaNamespace,
		},
	}
	c := fake.NewClientBuilder().WithObjects(ns).Build()

	err := state.EnsureNamespace(context.Background(), c)
	assert.NoError(t, err)

	// namespaceが依然として存在することを確認
	got := &corev1.Namespace{}
	err = c.Get(context.Background(), client.ObjectKey{Name: state.TazunaNamespace}, got)
	assert.NoError(t, err)
	assert.Equal(t, state.TazunaNamespace, got.Name)
}

func TestEnsureNamespace_Idempotent(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()

	// 2回呼んでもエラーにならないことを確認
	err := state.EnsureNamespace(context.Background(), c)
	assert.NoError(t, err)

	err = state.EnsureNamespace(context.Background(), c)
	assert.NoError(t, err)
}
