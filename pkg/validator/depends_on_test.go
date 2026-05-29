package validator

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateDependsOn_OK(t *testing.T) {
	t.Parallel()
	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"a"}},
		{Name: "c", DependsOn: []string{"a", "b"}},
	}
	require.NoError(t, ValidateDependsOn(manifests))
}

func TestValidateDependsOn_UnknownReference(t *testing.T) {
	t.Parallel()
	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b", DependsOn: []string{"ghost"}},
	}
	err := ValidateDependsOn(manifests)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown manifest")
	assert.Contains(t, err.Error(), "ghost")
}

func TestValidateDependsOn_SelfReference(t *testing.T) {
	t.Parallel()
	manifests := []v1.Manifest{
		{Name: "a", DependsOn: []string{"a"}},
	}
	err := ValidateDependsOn(manifests)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "depends on itself")
}

func TestValidateDependsOn_Circular(t *testing.T) {
	t.Parallel()
	manifests := []v1.Manifest{
		{Name: "a", DependsOn: []string{"b"}},
		{Name: "b", DependsOn: []string{"c"}},
		{Name: "c", DependsOn: []string{"a"}},
	}
	err := ValidateDependsOn(manifests)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "circular dependency")
}

func TestValidateDependsOn_NoDependsOn(t *testing.T) {
	t.Parallel()
	manifests := []v1.Manifest{
		{Name: "a"},
		{Name: "b"},
		{Name: "c"},
	}
	require.NoError(t, ValidateDependsOn(manifests))
}
