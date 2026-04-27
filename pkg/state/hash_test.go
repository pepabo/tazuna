package state_test

import (
	"testing"

	"github.com/pepabo/tazuna/pkg/state"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestComputeContentHash(t *testing.T) {
	t.Run("returns same hash for identical object", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-cm",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		}

		hash1, err := state.ComputeContentHash(obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		hash2, err := state.ComputeContentHash(obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hash1 != hash2 {
			t.Errorf("same object should produce same hash, got %s and %s", hash1, hash2)
		}
	})

	t.Run("returns different hashes for different objects", func(t *testing.T) {
		obj1 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-cm-1",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value1",
				},
			},
		}

		obj2 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-cm-2",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value2",
				},
			},
		}

		hash1, err := state.ComputeContentHash(obj1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		hash2, err := state.ComputeContentHash(obj2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hash1 == hash2 {
			t.Error("different objects should produce different hashes")
		}
	})

	t.Run("server-side field differences do not affect hash", func(t *testing.T) {
		obj1 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-cm",
					"namespace": "default",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		}

		obj2 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":              "test-cm",
					"namespace":         "default",
					"resourceVersion":   "12345",
					"uid":               "abc-def-ghi",
					"creationTimestamp": "2026-01-01T00:00:00Z",
					"generation":        int64(3),
					"managedFields": []interface{}{
						map[string]interface{}{
							"manager": "kubectl",
						},
					},
					"selfLink": "/api/v1/namespaces/default/configmaps/test-cm",
				},
				"data": map[string]interface{}{
					"key": "value",
				},
			},
		}

		hash1, err := state.ComputeContentHash(obj1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		hash2, err := state.ComputeContentHash(obj2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hash1 != hash2 {
			t.Errorf("server-side fields should not affect hash, got %s and %s", hash1, hash2)
		}
	})

	t.Run("status differences do not affect hash", func(t *testing.T) {
		obj1 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "test-deploy",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
			},
		}

		obj2 := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"metadata": map[string]interface{}{
					"name":      "test-deploy",
					"namespace": "default",
				},
				"spec": map[string]interface{}{
					"replicas": int64(3),
				},
				"status": map[string]interface{}{
					"readyReplicas":     int64(3),
					"availableReplicas": int64(3),
				},
			},
		}

		hash1, err := state.ComputeContentHash(obj1)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		hash2, err := state.ComputeContentHash(obj2)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if hash1 != hash2 {
			t.Errorf("status should not affect hash, got %s and %s", hash1, hash2)
		}
	})

	t.Run("does not mutate the original object", func(t *testing.T) {
		obj := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":            "test-cm",
					"namespace":       "default",
					"resourceVersion": "12345",
				},
				"status": map[string]interface{}{
					"phase": "Active",
				},
			},
		}

		_, err := state.ComputeContentHash(obj)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		// 元のオブジェクトのフィールドが残っていることを確認
		metadata := obj.Object["metadata"].(map[string]interface{})
		if _, ok := metadata["resourceVersion"]; !ok {
			t.Error("original object's resourceVersion should not be removed")
		}
		if _, ok := obj.Object["status"]; !ok {
			t.Error("original object's status should not be removed")
		}
	})
}
