//go:build integration

package runner_test

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/runner"
	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func TestDestroyResourcesOnCluster_OK(t *testing.T) {
	t.Parallel()
	path := "testdata/ok/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx-deployment",
		},
	}
	err := client.Create(context.Background(), &dep)
	assert.NoError(t, err)

	r := runner.NewTazunaRunner(logger, client, nil)

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	baseDir := filepath.Dir(path)
	r.ConvertManifestPathFromCwd(baseDir, &tazuna)

	svc := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx",
		},
	}
	err = client.Create(context.Background(), &svc)
	assert.NoError(t, err)

	err = r.DestroyResourcesOnCluster(context.Background(), tazuna)
	assert.NoError(t, err)

	// Check if the resources are deleted
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx-deployment",
	}, &dep)
	assert.True(t, apierrors.IsNotFound(err))
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",

		Name: "nginx",
	}, &svc)
	assert.True(t, apierrors.IsNotFound(err))
}

func TestDestroyResourcesOnCluster_WithTags(t *testing.T) {
	t.Parallel()
	path := "testdata/tags/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()

	// Create test resources that should be destroyed (nginx1)
	dep1 := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx1",
		},
	}
	err := client.Create(context.Background(), &dep1)
	assert.NoError(t, err)

	// Create test resources that should NOT be destroyed (nginx2)
	dep2 := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx2",
		},
	}
	err = client.Create(context.Background(), &dep2)
	assert.NoError(t, err)

	// Create runner with tag filter - only "kustomize1" tagged manifests should be processed
	r := runner.NewTazunaRunner(logger, client, nil, runner.WithTags([]string{"kustomize1"}))

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	baseDir := filepath.Dir(path)
	r.ConvertManifestPathFromCwd(baseDir, &tazuna)

	err = r.DestroyResourcesOnCluster(context.Background(), tazuna)
	assert.NoError(t, err)

	// Check that nginx1 (from kustomize1 tag) was destroyed
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx1",
	}, &dep1)
	assert.True(t, apierrors.IsNotFound(err))

	// Check that nginx2 (not tagged with kustomize1) still exists
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx2",
	}, &dep2)
	assert.NoError(t, err) // Should still exist because it wasn't tagged with kustomize1
}

func TestDestroyResourcesOnCluster_WithNonMatchingTags(t *testing.T) {
	t.Parallel()
	path := "testdata/tags/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()

	// Create test resource
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx1",
		},
	}
	err := client.Create(context.Background(), &dep)
	assert.NoError(t, err)

	// Create runner with non-matching tag filter
	r := runner.NewTazunaRunner(logger, client, nil, runner.WithTags([]string{"nonexistent-tag"}))

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	baseDir := filepath.Dir(path)
	r.ConvertManifestPathFromCwd(baseDir, &tazuna)

	err = r.DestroyResourcesOnCluster(context.Background(), tazuna)
	assert.NoError(t, err)

	// Check that resource still exists (should not be destroyed due to tag filter)
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx1",
	}, &dep)
	assert.NoError(t, err) // Should still exist because tag didn't match
}

func TestDestroyResourcesOnCluster_WithNoTagsSpecified(t *testing.T) {
	t.Parallel()
	path := "testdata/tags/tazuna.yaml"
	logger := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{}))
	client := fake.NewClientBuilder().Build()

	// Create test resource
	dep := appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "default",
			Name:      "nginx1",
		},
	}
	err := client.Create(context.Background(), &dep)
	assert.NoError(t, err)

	// Create runner with no tag filter (empty tags)
	r := runner.NewTazunaRunner(logger, client, nil, runner.WithTags([]string{}))

	f, err := os.Open(path)
	assert.NoError(t, err)
	defer func() {
		if cerr := f.Close(); cerr != nil {
			assert.NoError(t, cerr)
		}
	}()

	tazuna := v1.Tazuna{}
	err = yaml.NewDecoder(f).Decode(&tazuna)
	assert.NoError(t, err)

	baseDir := filepath.Dir(path)
	r.ConvertManifestPathFromCwd(baseDir, &tazuna)

	err = r.DestroyResourcesOnCluster(context.Background(), tazuna)
	assert.NoError(t, err)

	// Check that resource was destroyed (no tag filter means all manifests are processed)
	err = client.Get(context.Background(), types.NamespacedName{
		Namespace: "default",
		Name:      "nginx1",
	}, &dep)
	assert.True(t, apierrors.IsNotFound(err))
}
