//go:build integration

package manager_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/manager"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
)

func TestKustomize_Apply(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()

	m := manager.NewKustomize(client)

	manifest := v1.Manifest{
		Path: "testdata/kustomize",
	}
	// 冪等性が担保されていることのテストをするために、テスト関数を定義して複数回呼ぶ
	testFn := func(t *testing.T) {
		err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
		assert.NoError(t, err)

		dep := appsv1.Deployment{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "sample-deployment",
		}, &dep)
		assert.NoError(t, err)

		svc := corev1.Service{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "sample-service",
		}, &svc)
		assert.NoError(t, err)
	}

	testFn(t)
	testFn(t)
}

func TestKustomize_Apply_JobIdempotency(t *testing.T) {
	t.Parallel()
	client := fake.NewFakeClient()

	m := manager.NewKustomize(client)

	manifest := v1.Manifest{
		Path: "testdata/kustomize-with-job",
	}

	testFn := func(t *testing.T) {
		err := m.Apply(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), manifest)
		assert.NoError(t, err)

		job := batchv1.Job{}
		err = client.Get(context.Background(), types.NamespacedName{
			Namespace: "default",
			Name:      "sample-job",
		}, &job)
		assert.NoError(t, err)
	}

	// 2回呼んでもエラーにならないことを確認（Job の冪等性）
	testFn(t)
	testFn(t)
}
