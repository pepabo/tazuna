package state_test

import (
	"testing"

	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestNewStateKey(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		manifestName string
		group        string
		version      string
		kind         string
		namespace    string
		objName      string
		wantStr      string
	}{
		{
			name:         "namespaced resource (Deployment)",
			manifestName: "prometheus",
			group:        "apps",
			version:      "v1",
			kind:         "Deployment",
			namespace:    "monitoring",
			objName:      "prometheus",
			wantStr:      "prometheus/apps/v1/Deployment/monitoring/prometheus",
		},
		{
			name:         "core API resource (ConfigMap)",
			manifestName: "prometheus",
			group:        "",
			version:      "v1",
			kind:         "ConfigMap",
			namespace:    "monitoring",
			objName:      "prometheus-config",
			wantStr:      "prometheus//v1/ConfigMap/monitoring/prometheus-config",
		},
		{
			name:         "cluster-scoped resource (ClusterRole)",
			manifestName: "prometheus",
			group:        "rbac.authorization.k8s.io",
			version:      "v1",
			kind:         "ClusterRole",
			namespace:    "",
			objName:      "tazuna-admin",
			wantStr:      "prometheus/rbac.authorization.k8s.io/v1/ClusterRole/tazuna-admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			obj := &unstructured.Unstructured{}
			obj.SetGroupVersionKind(schema.GroupVersionKind{
				Group:   tt.group,
				Version: tt.version,
				Kind:    tt.kind,
			})
			obj.SetNamespace(tt.namespace)
			obj.SetName(tt.objName)

			key := state.NewStateKey(tt.manifestName, obj)
			assert.Equal(t, tt.wantStr, key.String())
		})
	}
}

func TestParseStateKey_Namespaced(t *testing.T) {
	t.Parallel()

	key, err := state.ParseStateKey("prometheus/apps/v1/Deployment/monitoring/prometheus")
	assert.NoError(t, err)
	assert.Equal(t, "prometheus", key.ManifestName)
	assert.Equal(t, "apps", key.Group)
	assert.Equal(t, "v1", key.Version)
	assert.Equal(t, "Deployment", key.Kind)
	assert.Equal(t, "monitoring", key.Namespace)
	assert.Equal(t, "prometheus", key.Name)
}

func TestParseStateKey_CoreAPI(t *testing.T) {
	t.Parallel()

	key, err := state.ParseStateKey("prometheus//v1/ConfigMap/monitoring/prometheus-config")
	assert.NoError(t, err)
	assert.Equal(t, "prometheus", key.ManifestName)
	assert.Equal(t, "", key.Group)
	assert.Equal(t, "v1", key.Version)
	assert.Equal(t, "ConfigMap", key.Kind)
	assert.Equal(t, "monitoring", key.Namespace)
	assert.Equal(t, "prometheus-config", key.Name)
}

func TestParseStateKey_ClusterScoped(t *testing.T) {
	t.Parallel()

	key, err := state.ParseStateKey("prometheus/rbac.authorization.k8s.io/v1/ClusterRole/tazuna-admin")
	assert.NoError(t, err)
	assert.Equal(t, "prometheus", key.ManifestName)
	assert.Equal(t, "rbac.authorization.k8s.io", key.Group)
	assert.Equal(t, "v1", key.Version)
	assert.Equal(t, "ClusterRole", key.Kind)
	assert.Equal(t, "", key.Namespace)
	assert.Equal(t, "tazuna-admin", key.Name)
}

func TestParseStateKey_Invalid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
	}{
		{name: "too few parts", input: "foo/bar/baz"},
		{name: "single part", input: "foo"},
		{name: "empty string", input: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := state.ParseStateKey(tt.input)
			assert.Error(t, err)
		})
	}
}

func TestStateKey_RoundTrip(t *testing.T) {
	t.Parallel()

	tests := []string{
		"prometheus/apps/v1/Deployment/monitoring/prometheus",
		"prometheus//v1/ConfigMap/monitoring/prometheus-config",
		"prometheus/rbac.authorization.k8s.io/v1/ClusterRole/tazuna-admin",
	}

	for _, s := range tests {
		t.Run(s, func(t *testing.T) {
			t.Parallel()
			key, err := state.ParseStateKey(s)
			assert.NoError(t, err)
			assert.Equal(t, s, key.String())
		})
	}
}
