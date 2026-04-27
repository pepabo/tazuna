//go:build integration

package testplugin_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/testplugin"
	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestExistNonExist_Run(t *testing.T) {
	t.Parallel()
	deploymentGVK := schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"}
	newRESTMapper := func() meta.RESTMapper {
		m := meta.NewDefaultRESTMapper([]schema.GroupVersion{{Group: "apps", Version: "v1"}})
		m.Add(deploymentGVK, meta.RESTScopeNamespace)
		return m
	}

	tests := []struct {
		name            string
		spec            *v1.TestPluginSpec
		clientGenerator func(ctx context.Context, t *testing.T) client.Client
		expectError     bool
		errorContains   string
	}{
		{
			name: "resource exists and shouldExist=true succeeds",
			spec: &v1.TestPluginSpec{
				ExistNonExist: &v1.ExistNonExistArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace:   "example",
					Name:        "example-app",
					ShouldExist: true,
				},
			},
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				obj := newDeployment("example", "example-app", deploymentGVK, nil)
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					WithRuntimeObjects(obj).
					Build()
			},
		},
		{
			name: "resource exists and shouldExist=false fails",
			spec: &v1.TestPluginSpec{
				ExistNonExist: &v1.ExistNonExistArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace:   "example",
					Name:        "example-app",
					ShouldExist: false,
				},
			},
			expectError:   true,
			errorContains: "resource exists",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				obj := newDeployment("example", "example-app", deploymentGVK, nil)
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					WithRuntimeObjects(obj).
					Build()
			},
		},
		{
			name: "resource missing and shouldExist=true fails",
			spec: &v1.TestPluginSpec{
				ExistNonExist: &v1.ExistNonExistArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace:   "example",
					Name:        "not-exist",
					ShouldExist: true,
				},
			},
			expectError:   true,
			errorContains: "resource not found",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					Build()
			},
		},
		{
			name: "resource missing and shouldExist=false succeeds",
			spec: &v1.TestPluginSpec{
				ExistNonExist: &v1.ExistNonExistArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace:   "example",
					Name:        "not-exist",
					ShouldExist: false,
				},
			},
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					Build()
			},
		},
		{
			name: "args undefined",
			spec: &v1.TestPluginSpec{
				ExistNonExist: nil,
			},
			expectError:   true,
			errorContains: ".spec.existNonExist is not defined",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				return fake.NewClientBuilder().Build()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.clientGenerator(context.Background(), t)
			plugin := testplugin.NewExistNonExist(c)
			err := plugin.Run(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), tt.spec)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
