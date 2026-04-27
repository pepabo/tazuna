//go:build integration

package resource

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = corev1.AddToScheme(s)
	return s
}

func newUnstructuredConfigMap(ns, name string, data map[string]string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	obj.SetNamespace(ns)
	obj.SetName(name)
	if data != nil {
		dataMap := make(map[string]interface{}, len(data))
		for k, v := range data {
			dataMap[k] = v
		}
		_ = unstructured.SetNestedMap(obj.Object, dataMap, "data")
	}
	return obj
}

func TestCreateOrUpdateForObject_Create(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(newScheme()).Build()

	obj := newUnstructuredConfigMap("default", "test-cm", map[string]string{"key": "value"})

	if err := CreateOrUpdateForObject(ctx, c, obj); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// 作成されたことを確認
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	if err := c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-cm"}, got); err != nil {
		t.Fatalf("failed to get created object: %v", err)
	}
}

func TestCreateOrUpdateForObject_Update(t *testing.T) {
	ctx := context.Background()

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cm",
		},
		Data: map[string]string{"key": "old"},
	}

	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(existing).Build()

	obj := newUnstructuredConfigMap("default", "test-cm", map[string]string{"key": "new"})

	if err := CreateOrUpdateForObject(ctx, c, obj); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// 更新されたことを確認
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	if err := c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-cm"}, got); err != nil {
		t.Fatalf("failed to get updated object: %v", err)
	}
	dataMap, _, _ := unstructured.NestedStringMap(got.Object, "data")
	if dataMap["key"] != "new" {
		t.Fatalf("expected data key=new, got %v", dataMap["key"])
	}
}

func TestDeleteObject_Exists(t *testing.T) {
	ctx := context.Background()

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "test-cm",
		},
	}

	c := fake.NewClientBuilder().WithScheme(newScheme()).WithObjects(existing).Build()

	obj := newUnstructuredConfigMap("default", "test-cm", nil)

	if err := DeleteObject(ctx, c, obj); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}

	// 削除されたことを確認
	got := &unstructured.Unstructured{}
	got.SetGroupVersionKind(schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"})
	err := c.Get(ctx, types.NamespacedName{Namespace: "default", Name: "test-cm"}, got)
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
	if client.IgnoreNotFound(err) != nil {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestDeleteObject_NotFound(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(newScheme()).Build()

	obj := newUnstructuredConfigMap("default", "nonexistent", nil)

	// NotFoundの場合はエラーにならない
	if err := DeleteObject(ctx, c, obj); err != nil {
		t.Fatalf("expected nil for not found, got %v", err)
	}
}
