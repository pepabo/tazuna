package resource

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newWorkloadObject(kind string, spec, status map[string]any) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "apps/v1",
		"kind":       kind,
		"metadata":   map[string]any{"name": "test", "namespace": "default"},
	}}
	if spec != nil {
		obj.Object["spec"] = spec
	}
	if status != nil {
		obj.Object["status"] = status
	}
	return obj
}

func TestIsDeploymentReady(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		spec   map[string]any
		status map[string]any
		want   bool
	}{
		{
			name: "explicit zero replicas is ready",
			spec: map[string]any{"replicas": int64(0)},
			want: true,
		},
		{
			name: "missing spec.replicas (default 1) with no status is not ready",
			// spec.replicas 欠落はデフォルト 1 なので、status が伴わない限り
			// ready ではない (欠落を 0 と同一視しない)
			spec: map[string]any{},
			want: false,
		},
		{
			name: "missing spec.replicas with ready status is ready",
			spec: map[string]any{},
			status: map[string]any{
				"replicas":          int64(1),
				"readyReplicas":     int64(1),
				"availableReplicas": int64(1),
			},
			want: true,
		},
		{
			name: "replicas not yet ready",
			spec: map[string]any{"replicas": int64(2)},
			status: map[string]any{
				"replicas":          int64(2),
				"readyReplicas":     int64(1),
				"availableReplicas": int64(1),
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := IsDeploymentReady(newWorkloadObject("Deployment", tt.spec, tt.status))
			assert.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestIsStatefulSetReady(t *testing.T) {
	t.Parallel()

	t.Run("explicit zero replicas is ready", func(t *testing.T) {
		t.Parallel()
		got, err := IsStatefulSetReady(newWorkloadObject("StatefulSet",
			map[string]any{"replicas": int64(0)}, nil))
		assert.NoError(t, err)
		assert.True(t, got)
	})

	t.Run("missing spec.replicas with no status is not ready", func(t *testing.T) {
		t.Parallel()
		got, err := IsStatefulSetReady(newWorkloadObject("StatefulSet",
			map[string]any{}, nil))
		assert.NoError(t, err)
		assert.False(t, got)
	})

	t.Run("missing spec.replicas with ready status is ready", func(t *testing.T) {
		t.Parallel()
		got, err := IsStatefulSetReady(newWorkloadObject("StatefulSet",
			map[string]any{},
			map[string]any{"replicas": int64(1), "readyReplicas": int64(1)}))
		assert.NoError(t, err)
		assert.True(t, got)
	})
}
