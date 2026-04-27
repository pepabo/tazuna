//go:build integration

package state_test

import (
	"context"
	"testing"

	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestConfigMapStateStore_Get_NotFound(t *testing.T) {
	t.Parallel()
	c := fake.NewClientBuilder().Build()
	store := state.NewConfigMapStateStore(c)

	data, err := store.Get(context.Background(), "prometheus")
	assert.NoError(t, err)
	assert.NotNil(t, data)
	assert.Empty(t, data.Entries)
	assert.Equal(t, state.StateMetadata{}, data.Metadata)
}

func TestConfigMapStateStore_Get_Existing(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tazuna-state-prometheus",
			Namespace: state.TazunaNamespace,
		},
		Data: map[string]string{
			"_metadata": `{"gitCommitHash":"abc123","lastSyncedAt":"2026-04-02T10:00:00Z"}`,
			"prometheus__apps__v1__Deployment__monitoring__prometheus":   `{"contentHash":"hash1"}`,
			"prometheus____v1__ConfigMap__monitoring__prometheus-config": `{"contentHash":"hash2"}`,
		},
	}

	c := fake.NewClientBuilder().WithObjects(cm).Build()
	store := state.NewConfigMapStateStore(c)

	data, err := store.Get(context.Background(), "prometheus")
	assert.NoError(t, err)
	assert.Equal(t, "abc123", data.Metadata.GitCommitHash)
	assert.Equal(t, "2026-04-02T10:00:00Z", data.Metadata.LastSyncedAt)
	assert.Len(t, data.Entries, 2)
	assert.Equal(t, "hash1", data.Entries["prometheus/apps/v1/Deployment/monitoring/prometheus"].ContentHash)
	assert.Equal(t, "hash2", data.Entries["prometheus//v1/ConfigMap/monitoring/prometheus-config"].ContentHash)
}

func TestConfigMapStateStore_Save_Create(t *testing.T) {
	t.Parallel()

	// tazuna namespaceを事前作成
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: state.TazunaNamespace,
		},
	}
	c := fake.NewClientBuilder().WithObjects(ns).Build()
	store := state.NewConfigMapStateStore(c)

	data := &state.StateData{
		Metadata: state.StateMetadata{
			GitCommitHash: "def456",
			LastSyncedAt:  "2026-04-03T12:00:00Z",
		},
		Entries: map[string]state.StateEntry{
			"example/apps/v1/Deployment/example/example-app": {ContentHash: "hashA"},
		},
	}

	err := store.Save(context.Background(), "example", data)
	assert.NoError(t, err)

	// ConfigMapが作成されたことを確認
	cm := &corev1.ConfigMap{}
	err = c.Get(context.Background(), types.NamespacedName{
		Namespace: state.TazunaNamespace,
		Name:      "tazuna-state-example",
	}, cm)
	assert.NoError(t, err)
	assert.Contains(t, cm.Data, "_metadata")
	assert.Contains(t, cm.Data, "example__apps__v1__Deployment__example__example-app")
}

func TestConfigMapStateStore_Save_Update(t *testing.T) {
	t.Parallel()

	existing := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tazuna-state-prometheus",
			Namespace: state.TazunaNamespace,
		},
		Data: map[string]string{
			"_metadata": `{"gitCommitHash":"old","lastSyncedAt":"2026-04-01T00:00:00Z"}`,
			"prometheus__apps__v1__Deployment__monitoring__prometheus": `{"contentHash":"oldHash"}`,
		},
	}

	c := fake.NewClientBuilder().WithObjects(existing).Build()
	store := state.NewConfigMapStateStore(c)

	newData := &state.StateData{
		Metadata: state.StateMetadata{
			GitCommitHash: "new123",
			LastSyncedAt:  "2026-04-03T15:00:00Z",
		},
		Entries: map[string]state.StateEntry{
			"prometheus/apps/v1/Deployment/monitoring/prometheus":   {ContentHash: "newHash"},
			"prometheus//v1/ConfigMap/monitoring/prometheus-config": {ContentHash: "hash2"},
		},
	}

	err := store.Save(context.Background(), "prometheus", newData)
	assert.NoError(t, err)

	// 更新されたConfigMapを確認
	cm := &corev1.ConfigMap{}
	err = c.Get(context.Background(), types.NamespacedName{
		Namespace: state.TazunaNamespace,
		Name:      "tazuna-state-prometheus",
	}, cm)
	assert.NoError(t, err)
	assert.Contains(t, cm.Data, "prometheus____v1__ConfigMap__monitoring__prometheus-config")
	assert.Contains(t, cm.Data["_metadata"], "new123")
}

func TestConfigMapName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "tazuna-state-prometheus", state.ConfigMapName("prometheus"))
	assert.Equal(t, "tazuna-state-example", state.ConfigMapName("example"))
}

func TestConfigMapStateStore_SaveAndGet_RoundTrip(t *testing.T) {
	t.Parallel()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: state.TazunaNamespace,
		},
	}
	c := fake.NewClientBuilder().WithObjects(ns).Build()
	store := state.NewConfigMapStateStore(c)

	original := &state.StateData{
		Metadata: state.StateMetadata{
			GitCommitHash: "abc123",
			LastSyncedAt:  "2026-04-03T10:00:00Z",
		},
		Entries: map[string]state.StateEntry{
			"myapp/apps/v1/Deployment/default/nginx":                     {ContentHash: "hash1"},
			"myapp//v1/Service/default/nginx-svc":                        {ContentHash: "hash2"},
			"myapp/rbac.authorization.k8s.io/v1/ClusterRole/myapp-admin": {ContentHash: "hash3"},
		},
	}

	err := store.Save(context.Background(), "myapp", original)
	assert.NoError(t, err)

	loaded, err := store.Get(context.Background(), "myapp")
	assert.NoError(t, err)
	assert.Equal(t, original.Metadata, loaded.Metadata)
	assert.Equal(t, original.Entries, loaded.Entries)
}
