package runner

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSanitizeForDiff_NamespaceStripsServerSideFields(t *testing.T) {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": "karpenter",
			"labels": map[string]any{
				"kubernetes.io/metadata.name": "karpenter",
			},
		},
		"spec": map[string]any{
			"finalizers": []any{"kubernetes"},
		},
	}}

	got := sanitizeForDiff(live)

	if _, found, _ := unstructured.NestedMap(got.Object, "metadata", "labels"); found {
		t.Errorf("metadata.labels should be removed when only the server-added label was present, got: %v", got.Object)
	}
	if _, found, _ := unstructured.NestedFieldNoCopy(got.Object, "spec"); found {
		t.Errorf("empty spec should be removed after stripping server-added finalizers, got: %v", got.Object)
	}
}

func TestSanitizeForDiff_NamespacePreservesUserLabelsAndOtherFinalizers(t *testing.T) {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Namespace",
		"metadata": map[string]any{
			"name": "monitoring",
			"labels": map[string]any{
				"kubernetes.io/metadata.name": "monitoring",
				"team":                        "platform",
			},
		},
		"spec": map[string]any{
			"finalizers": []any{"kubernetes", "example.com/custom"},
		},
	}}

	got := sanitizeForDiff(live)

	labels := got.GetLabels()
	if _, ok := labels["kubernetes.io/metadata.name"]; ok {
		t.Errorf("server-added label should be stripped, got: %v", labels)
	}
	if labels["team"] != "platform" {
		t.Errorf("user labels should be preserved, got: %v", labels)
	}

	finalizers, ok, _ := unstructured.NestedStringSlice(got.Object, "spec", "finalizers")
	if !ok {
		t.Fatalf("finalizers should be preserved when not exactly [kubernetes]")
	}
	if len(finalizers) != 2 {
		t.Errorf("finalizers should be untouched when more than just 'kubernetes' is present, got: %v", finalizers)
	}
}

func TestSanitizeForDiff_NonNamespaceUntouched(t *testing.T) {
	live := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]any{
			"name":      "cm",
			"namespace": "default",
			"labels": map[string]any{
				"kubernetes.io/metadata.name": "keep-me",
			},
		},
	}}

	got := sanitizeForDiff(live)

	labels := got.GetLabels()
	if labels["kubernetes.io/metadata.name"] != "keep-me" {
		t.Errorf("non-Namespace resources must not be touched by the Namespace sanitizer, got: %v", labels)
	}
}
