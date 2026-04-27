package manifest

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestConvertManifestsToObjects(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name          string
		input         []byte
		expectedCount int
		expectError   bool
	}{
		{
			name: "Single valid manifest",
			input: []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: value
`),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name: "Multiple valid manifests",
			input: []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: value
---
apiVersion: v1
kind: Secret
metadata:
  name: test-secret
data:
  password: cGFzc3dvcmQ=
`),
			expectedCount: 2,
			expectError:   false,
		},
		{
			name: "Manifest without kind",
			input: []byte(`
apiVersion: v1
metadata:
  name: invalid-manifest
`),
			expectedCount: 0,
			expectError:   false,
		},
		{
			name: "Empty manifest",
			input: []byte(`
---
`),
			expectedCount: 0,
			expectError:   false,
		},
		{
			name: "The YAML string has triple hyphens",
			input: []byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: |-
    permission is rw-rw----
---
`),
			expectedCount: 1,
			expectError:   false,
		},
		{
			name: "kind: List should be expanded to individual items",
			input: []byte(`
apiVersion: v1
kind: List
items:
  - apiVersion: networking.k8s.io/v1
    kind: IngressClass
    metadata:
      name: alb
    spec:
      controller: ingress.k8s.aws/alb
  - apiVersion: elbv2.k8s.aws/v1beta1
    kind: IngressClassParams
    metadata:
      name: alb
    spec:
      group:
        name: default
`),
			expectedCount: 2,
			expectError:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			objects, err := ConvertManifestsToObjects(tt.input, "")

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedCount, len(objects))

				for _, obj := range objects {
					_, ok := obj.(*unstructured.Unstructured)
					assert.True(t, ok, "Object should be of type *unstructured.Unstructured")
				}
			}
		})
	}
}

func TestConvertManifestsToObjects_WithNamespace(t *testing.T) {
	t.Parallel()

	objects, err := ConvertManifestsToObjects([]byte(`
apiVersion: v1
kind: ConfigMap
metadata:
  name: test-configmap
data:
  key: value
`), "test-namespace")

	assert.NoError(t, err)
	assert.Equal(t, 1, len(objects))
	assert.Equal(t, "test-namespace", objects[0].GetNamespace())
	assert.Equal(t, "test-configmap", objects[0].GetName())
}

func TestConvertManifestsToObjects_ListWithNamespace(t *testing.T) {
	t.Parallel()

	objects, err := ConvertManifestsToObjects([]byte(`
apiVersion: v1
kind: List
items:
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: test-configmap1
    data:
      key: value1
  - apiVersion: v1
    kind: ConfigMap
    metadata:
      name: test-configmap2
    data:
      key: value2
`), "test-namespace")

	assert.NoError(t, err)
	assert.Equal(t, 2, len(objects))
	assert.Equal(t, "test-namespace", objects[0].GetNamespace())
	assert.Equal(t, "test-configmap1", objects[0].GetName())
	assert.Equal(t, "test-namespace", objects[1].GetNamespace())
	assert.Equal(t, "test-configmap2", objects[1].GetName())
}
