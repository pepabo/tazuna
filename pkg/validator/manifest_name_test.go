package validator

import (
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func TestValidateManifestNames(t *testing.T) {
	tests := []struct {
		name        string
		manifests   []v1.Manifest
		wantErr     bool
		errContains []string
	}{
		{
			name: "all valid",
			manifests: []v1.Manifest{
				{Name: "kustomize-monitoring"},
				{Name: "helmfile-example"},
				{Name: "genesissecret-base"},
			},
			wantErr: false,
		},
		{
			name: "valid names with hyphens and digits",
			manifests: []v1.Manifest{
				{Name: "my-manifest-01"},
				{Name: "0-leading-digit"},
			},
			wantErr: false,
		},
		{
			name: "underscore is not allowed (breaks state key encoding)",
			manifests: []v1.Manifest{
				{Name: "my__app"},
			},
			wantErr:     true,
			errContains: []string{"invalid characters"},
		},
		{
			name: "uppercase is not allowed (breaks ConfigMap name)",
			manifests: []v1.Manifest{
				{Name: "A-Z-test"},
			},
			wantErr:     true,
			errContains: []string{"invalid characters"},
		},
		{
			name: "trailing hyphen is not allowed",
			manifests: []v1.Manifest{
				{Name: "trailing-"},
			},
			wantErr:     true,
			errContains: []string{"invalid characters"},
		},
		{
			name: "too long name",
			manifests: []v1.Manifest{
				{Name: strings.Repeat("a", 241)},
			},
			wantErr:     true,
			errContains: []string{"too long"},
		},
		{
			name: "empty name",
			manifests: []v1.Manifest{
				{Name: ""},
			},
			wantErr:     true,
			errContains: []string{"name is required"},
		},
		{
			name: "invalid characters with dot",
			manifests: []v1.Manifest{
				{Name: "my.manifest"},
			},
			wantErr:     true,
			errContains: []string{"invalid characters"},
		},
		{
			name: "invalid characters with slash",
			manifests: []v1.Manifest{
				{Name: "path/to"},
			},
			wantErr:     true,
			errContains: []string{"invalid characters"},
		},
		{
			name: "invalid characters with space",
			manifests: []v1.Manifest{
				{Name: "has space"},
			},
			wantErr:     true,
			errContains: []string{"invalid characters"},
		},
		{
			name: "reserved name _metadata",
			manifests: []v1.Manifest{
				{Name: "_metadata"},
			},
			wantErr:     true,
			errContains: []string{"reserved"},
		},
		{
			name: "duplicate names",
			manifests: []v1.Manifest{
				{Name: "same-name"},
				{Name: "same-name"},
			},
			wantErr:     true,
			errContains: []string{"duplicated"},
		},
		{
			name: "multiple errors",
			manifests: []v1.Manifest{
				{Name: ""},
				{Name: "valid-name"},
				{Name: "bad.name"},
			},
			wantErr:     true,
			errContains: []string{"name is required", "invalid characters"},
		},
		{
			name: "mix of empty and duplicate",
			manifests: []v1.Manifest{
				{Name: ""},
				{Name: "dup"},
				{Name: "dup"},
			},
			wantErr:     true,
			errContains: []string{"name is required", "duplicated"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateManifestNames(tt.manifests)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateManifestNames() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				for _, contains := range tt.errContains {
					if !strings.Contains(err.Error(), contains) {
						t.Errorf("expected error to contain %q, got %v", contains, err)
					}
				}
			}
		})
	}
}
