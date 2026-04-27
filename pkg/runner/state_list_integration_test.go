//go:build integration

package runner_test

import (
	"bytes"
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/pepabo/tazuna/pkg/state"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestStateList_WithEntries(t *testing.T) {
	t.Parallel()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tazuna-state-prometheus",
			Namespace: state.TazunaNamespace,
		},
		Data: map[string]string{
			"_metadata": `{"gitCommitHash":"abc123","lastSyncedAt":"2026-04-02T10:00:00Z"}`,
			"prometheus/apps/v1/Deployment/monitoring/prometheus":   `{"contentHash":"aabbcc"}`,
			"prometheus//v1/ConfigMap/monitoring/prometheus-config": `{"contentHash":"ddeeff"}`,
		},
	}

	c := fake.NewClientBuilder().WithObjects(cm).Build()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := runner.NewTazunaRunner(logger, c, nil)

	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "prometheus", Type: v1.ManifestTypeKustomize, Path: "./monitoring"},
			},
		},
	}

	var buf bytes.Buffer
	err := r.StateList(context.Background(), tazuna, filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Manifest: prometheus")
	assert.Contains(t, output, "Last synced: 2026-04-02T10:00:00Z")
	assert.Contains(t, output, "Git commit: abc123")
	assert.Contains(t, output, "apps/v1/Deployment/monitoring/prometheus")
	assert.Contains(t, output, "aabbcc")
	assert.Contains(t, output, "/v1/ConfigMap/monitoring/prometheus-config")
	assert.Contains(t, output, "ddeeff")
}

func TestStateList_NoState(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := runner.NewTazunaRunner(logger, c, nil)

	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Name: "example", Type: v1.ManifestTypeKustomize, Path: "./example"},
			},
		},
	}

	var buf bytes.Buffer
	err := r.StateList(context.Background(), tazuna, filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)

	assert.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Manifest: example")
	assert.Contains(t, output, "No state found")
}

func TestStateList_SkipsManifestWithoutName(t *testing.T) {
	t.Parallel()

	c := fake.NewClientBuilder().Build()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	r := runner.NewTazunaRunner(logger, c, nil)

	tazuna := v1.Tazuna{
		Spec: v1.TazunaSpec{
			Manifests: []v1.Manifest{
				{Type: v1.ManifestTypeKustomize, Path: "./unnamed"},
			},
		},
	}

	var buf bytes.Buffer
	err := r.StateList(context.Background(), tazuna, filepath.Join(t.TempDir(), "tazuna.yaml"), &buf)

	assert.NoError(t, err)
	output := buf.String()
	assert.NotContains(t, output, "Manifest:")
}
