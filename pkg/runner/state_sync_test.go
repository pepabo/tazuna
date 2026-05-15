package runner

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestBuildUnstructuredFromStateKey_Namespaced(t *testing.T) {
	t.Parallel()

	obj, err := buildUnstructuredFromStateKey("my-manifest/apps/v1/Deployment/default/nginx")
	require.NoError(t, err)

	assert.Equal(t, schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}, obj.GroupVersionKind())
	assert.Equal(t, "nginx", obj.GetName())
	assert.Equal(t, "default", obj.GetNamespace())
}

func TestBuildUnstructuredFromStateKey_ClusterScoped(t *testing.T) {
	t.Parallel()

	obj, err := buildUnstructuredFromStateKey("my-manifest/rbac.authorization.k8s.io/v1/ClusterRole/admin")
	require.NoError(t, err)

	assert.Equal(t, schema.GroupVersionKind{Group: "rbac.authorization.k8s.io", Version: "v1", Kind: "ClusterRole"}, obj.GroupVersionKind())
	assert.Equal(t, "admin", obj.GetName())
	assert.Equal(t, "", obj.GetNamespace())
}

func TestBuildUnstructuredFromStateKey_CoreGroup(t *testing.T) {
	t.Parallel()

	obj, err := buildUnstructuredFromStateKey("my-manifest//v1/ConfigMap/kube-system/coredns")
	require.NoError(t, err)

	assert.Equal(t, schema.GroupVersionKind{Group: "", Version: "v1", Kind: "ConfigMap"}, obj.GroupVersionKind())
	assert.Equal(t, "coredns", obj.GetName())
	assert.Equal(t, "kube-system", obj.GetNamespace())
}

func TestBuildUnstructuredFromStateKey_InvalidKey(t *testing.T) {
	t.Parallel()

	_, err := buildUnstructuredFromStateKey("invalid-key")
	assert.Error(t, err)
}

func TestGetGitCommitHash(t *testing.T) {
	t.Parallel()

	hash := getGitCommitHash(context.Background())
	// gitリポジトリ内で実行されている場合、40文字のhex文字列が返る
	if hash != "" {
		assert.Len(t, hash, 40)
	}
}
