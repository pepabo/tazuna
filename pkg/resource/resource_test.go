package resource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestConfigMapObject(namespace, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetAPIVersion("v1")
	obj.SetKind("ConfigMap")
	obj.SetNamespace(namespace)
	obj.SetName(name)
	return obj
}

func TestWaitForDeletion_AlreadyDeleted(t *testing.T) {
	t.Parallel()
	c := fake.NewClientBuilder().Build()

	err := WaitForDeletion(context.Background(), c, newTestConfigMapObject("default", "gone"))
	assert.NoError(t, err)
}

func TestWaitForDeletion_TimesOutOnStuckResource(t *testing.T) {
	t.Parallel()
	// 削除されないリソース (finalizer が詰まった状態のシミュレーション) に対して
	// 無期限にハングせずタイムアウトエラーを返すことを確認する。
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Namespace: "default", Name: "stuck"},
	}
	c := fake.NewClientBuilder().WithObjects(cm).Build()

	start := time.Now()
	err := waitForDeletion(context.Background(), c, newTestConfigMapObject("default", "stuck"), 2*time.Second)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "giving up waiting for deletion")
	assert.Less(t, time.Since(start), 10*time.Second)
}
