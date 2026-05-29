package validator

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func TestFixManifestNames_Basic(t *testing.T) {
	manifests := []v1.Manifest{
		{Type: v1.ManifestTypeKustomize, Path: "./manifests/monitoring"},
		{Type: v1.ManifestTypeHelmfile, Path: "./charts/example"},
	}

	FixManifestNames(manifests)

	if manifests[0].Name != "kustomize-monitoring" {
		t.Errorf("expected kustomize-monitoring, got %s", manifests[0].Name)
	}
	if manifests[1].Name != "helmfile-example" {
		t.Errorf("expected helmfile-example, got %s", manifests[1].Name)
	}
}

func TestFixManifestNames_PreservesExistingNames(t *testing.T) {
	manifests := []v1.Manifest{
		{Name: "my-custom-name", Type: v1.ManifestTypeKustomize, Path: "./monitoring"},
		{Type: v1.ManifestTypeHelmfile, Path: "./example"},
	}

	FixManifestNames(manifests)

	if manifests[0].Name != "my-custom-name" {
		t.Errorf("expected my-custom-name, got %s", manifests[0].Name)
	}
	if manifests[1].Name != "helmfile-example" {
		t.Errorf("expected helmfile-example, got %s", manifests[1].Name)
	}
}

func TestFixManifestNames_DuplicateAvoidance(t *testing.T) {
	manifests := []v1.Manifest{
		{Type: v1.ManifestTypeKustomize, Path: "./a/monitoring"},
		{Type: v1.ManifestTypeKustomize, Path: "./b/monitoring"},
		{Type: v1.ManifestTypeKustomize, Path: "./c/monitoring"},
	}

	FixManifestNames(manifests)

	if manifests[0].Name != "kustomize-monitoring" {
		t.Errorf("expected kustomize-monitoring, got %s", manifests[0].Name)
	}
	if manifests[1].Name != "kustomize-monitoring-2" {
		t.Errorf("expected kustomize-monitoring-2, got %s", manifests[1].Name)
	}
	if manifests[2].Name != "kustomize-monitoring-3" {
		t.Errorf("expected kustomize-monitoring-3, got %s", manifests[2].Name)
	}
}

func TestFixManifestNames_DuplicateWithExisting(t *testing.T) {
	manifests := []v1.Manifest{
		{Name: "kustomize-monitoring", Type: v1.ManifestTypeKustomize, Path: "./monitoring"},
		{Type: v1.ManifestTypeKustomize, Path: "./other/monitoring"},
	}

	FixManifestNames(manifests)

	if manifests[0].Name != "kustomize-monitoring" {
		t.Errorf("expected kustomize-monitoring, got %s", manifests[0].Name)
	}
	if manifests[1].Name != "kustomize-monitoring-2" {
		t.Errorf("expected kustomize-monitoring-2, got %s", manifests[1].Name)
	}
}

func TestFixManifestNames_EmptyPath(t *testing.T) {
	manifests := []v1.Manifest{
		{Type: v1.ManifestTypeKustomize, Path: "."},
		{Type: v1.ManifestTypeHelmfile, Path: ""},
	}

	FixManifestNames(manifests)

	if manifests[0].Name != "kustomize" {
		t.Errorf("expected kustomize, got %s", manifests[0].Name)
	}
	if manifests[1].Name != "helmfile" {
		t.Errorf("expected helmfile, got %s", manifests[1].Name)
	}
}

func TestFixManifestNames_EmptyTypeAndPath(t *testing.T) {
	manifests := []v1.Manifest{
		{Type: "", Path: ""},
	}

	FixManifestNames(manifests)

	if manifests[0].Name != "manifest" {
		t.Errorf("expected manifest, got %s", manifests[0].Name)
	}
}

func TestExtractDirName(t *testing.T) {
	tests := []struct {
		path string
		want string
	}{
		{"./manifests/monitoring", "monitoring"},
		{"./charts/example/", "example"},
		{".", ""},
		{"", ""},
		{"monitoring", "monitoring"},
		{"./path.with.dots", "path-with-dots"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := extractDirName(tt.path)
			if got != tt.want {
				t.Errorf("extractDirName(%q) = %q, want %q", tt.path, got, tt.want)
			}
		})
	}
}
