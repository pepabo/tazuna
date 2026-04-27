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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestWaitUntil_Run(t *testing.T) {
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
			name: "condition satisfied",
			spec: &v1.TestPluginSpec{
				WaitUntil: &v1.WaitUntilArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace: "example",
					Name:      "example-app",
					Condition: "object.status.readyReplicas >= 1",
				},
			},
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				obj := newDeployment("example", "example-app", deploymentGVK, map[string]interface{}{
					"readyReplicas": int64(1),
				})
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					WithRuntimeObjects(obj).
					Build()
			},
		},
		{
			name: "condition not satisfied",
			spec: &v1.TestPluginSpec{
				WaitUntil: &v1.WaitUntilArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace: "example",
					Name:      "example-app",
					Condition: "object.status.readyReplicas >= 3",
				},
			},
			expectError:   true,
			errorContains: "condition",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				obj := newDeployment("example", "example-app", deploymentGVK, map[string]interface{}{
					"readyReplicas": int64(1),
				})
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					WithRuntimeObjects(obj).
					Build()
			},
		},
		{
			name: "args undefined",
			spec: &v1.TestPluginSpec{
				WaitUntil: nil,
			},
			expectError:   true,
			errorContains: ".spec.waitUntil is not defined",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				return fake.NewClientBuilder().Build()
			},
		},
		{
			name: "invalid CEL expression",
			spec: &v1.TestPluginSpec{
				WaitUntil: &v1.WaitUntilArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace: "example",
					Name:      "example-app",
					Condition: "invalid %%% expr",
				},
			},
			expectError:   true,
			errorContains: "compile",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				obj := newDeployment("example", "example-app", deploymentGVK, nil)
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					WithRuntimeObjects(obj).
					Build()
			},
		},
		{
			name: "resource not found",
			spec: &v1.TestPluginSpec{
				WaitUntil: &v1.WaitUntilArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace: "example",
					Name:      "not-exist",
					Condition: "object.status.readyReplicas >= 1",
				},
			},
			expectError: true,
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					Build()
			},
		},
		{
			name: "non-bool CEL expression",
			spec: &v1.TestPluginSpec{
				WaitUntil: &v1.WaitUntilArgs{
					Resource: v1.WaitUntilResource{
						APIVersion: "apps/v1",
						Kind:       "Deployment",
					},
					Namespace: "example",
					Name:      "example-app",
					Condition: "object.metadata.name",
				},
			},
			expectError:   true,
			errorContains: "is not a bool type",
			clientGenerator: func(ctx context.Context, t *testing.T) client.Client {
				obj := newDeployment("example", "example-app", deploymentGVK, nil)
				return fake.NewClientBuilder().
					WithRESTMapper(newRESTMapper()).
					WithRuntimeObjects(obj).
					Build()
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			c := tt.clientGenerator(context.Background(), t)
			waitUntil := testplugin.NewWaitUntil(c)
			err := waitUntil.Run(context.Background(), slog.New(slog.NewTextHandler(io.Discard, nil)), tt.spec)

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

func TestEvaluateCEL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		expression  string
		object      map[string]interface{}
		expected    bool
		expectError bool
	}{
		{
			name:       "numeric comparison true",
			expression: "object.status.readyReplicas >= 1",
			object: map[string]interface{}{
				"status": map[string]interface{}{
					"readyReplicas": int64(3),
				},
			},
			expected: true,
		},
		{
			name:       "numeric comparison false",
			expression: "object.status.readyReplicas >= 1",
			object: map[string]interface{}{
				"status": map[string]interface{}{
					"readyReplicas": int64(0),
				},
			},
			expected: false,
		},
		{
			name:       "string comparison",
			expression: "object.status.phase == 'Running'",
			object: map[string]interface{}{
				"status": map[string]interface{}{
					"phase": "Running",
				},
			},
			expected: true,
		},
		{
			name:        "invalid expression",
			expression:  "invalid %%% expr",
			object:      map[string]interface{}{},
			expectError: true,
		},
		{
			name:        "non-bool result",
			expression:  "object.metadata.name",
			object:      map[string]interface{}{"metadata": map[string]interface{}{"name": "test"}},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result, err := testplugin.EvaluateCEL(tt.expression, tt.object)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func newDeployment(namespace, name string, gvk schema.GroupVersionKind, status map[string]interface{}) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetNamespace(namespace)
	obj.SetName(name)
	if status != nil {
		obj.Object["status"] = status
	}
	return obj
}
