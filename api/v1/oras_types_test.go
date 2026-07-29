package v1

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func TestManifestORAS_RoundTrip_Tag(t *testing.T) {
	src := Manifest{
		Name: "example",
		Type: ManifestTypeORAS,
		ORAS: &ManifestORAS{
			Reference: "ghcr.io/example/foo:v1.0.0",
			Target:    "./base",
			Delegate: ORASDelegate{
				Type: ORASDelegateTypeKustomize,
				Kustomize: &ManifestKustomize{
					DefaultNamespace: "example",
				},
			},
		},
	}

	got := marshalUnmarshal(t, src)
	if got.Type != ManifestTypeORAS {
		t.Errorf("Type = %q, want %q", got.Type, ManifestTypeORAS)
	}
	if got.ORAS == nil {
		t.Fatal("ORAS is nil")
	}
	if got.ORAS.Reference != src.ORAS.Reference {
		t.Errorf("Reference = %q, want %q", got.ORAS.Reference, src.ORAS.Reference)
	}
	if got.ORAS.Target != "./base" {
		t.Errorf("Target = %q", got.ORAS.Target)
	}
	if got.ORAS.Auth != nil {
		t.Errorf("Auth = %+v, want nil (omitted)", got.ORAS.Auth)
	}
	if got.ORAS.PlainHTTP {
		t.Errorf("PlainHTTP = true, want false (default)")
	}
	if got.ORAS.InsecureSkipVerify {
		t.Errorf("InsecureSkipVerify = true, want false (default)")
	}
	if got.ORAS.Delegate.Type != ORASDelegateTypeKustomize {
		t.Errorf("Delegate.Type = %q", got.ORAS.Delegate.Type)
	}
	if got.ORAS.Delegate.Kustomize == nil {
		t.Fatal("Delegate.Kustomize is nil")
	}
	if got.ORAS.Delegate.Kustomize.DefaultNamespace != "example" {
		t.Errorf("Delegate.Kustomize.DefaultNamespace = %q", got.ORAS.Delegate.Kustomize.DefaultNamespace)
	}
}

func TestManifestORAS_RoundTrip_Digest(t *testing.T) {
	src := Manifest{
		Type: ManifestTypeORAS,
		ORAS: &ManifestORAS{
			Reference: "ghcr.io/example/foo@sha256:abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789",
			Delegate: ORASDelegate{
				Type:     ORASDelegateTypeHelmfile,
				Helmfile: &ManifestHelmfile{IncludeCRDs: true},
			},
		},
	}

	got := marshalUnmarshal(t, src)
	if got.ORAS.Reference != src.ORAS.Reference {
		t.Errorf("Reference = %q, want %q", got.ORAS.Reference, src.ORAS.Reference)
	}
	if got.ORAS.Target != "" {
		t.Errorf("Target = %q, want empty", got.ORAS.Target)
	}
	if got.ORAS.Delegate.Type != ORASDelegateTypeHelmfile {
		t.Errorf("Delegate.Type = %q", got.ORAS.Delegate.Type)
	}
	if got.ORAS.Delegate.Helmfile == nil {
		t.Fatal("Delegate.Helmfile is nil")
	}
	if !got.ORAS.Delegate.Helmfile.IncludeCRDs {
		t.Error("Delegate.Helmfile.IncludeCRDs = false, want true")
	}
}

func TestManifestORAS_RoundTrip_AuthAndFlags(t *testing.T) {
	src := Manifest{
		Type: ManifestTypeORAS,
		ORAS: &ManifestORAS{
			Reference:          "localhost:5000/example/foo:dev",
			PlainHTTP:          true,
			InsecureSkipVerify: true,
			Auth: &ORASAuth{
				Username: "alice",
				Password: "s3cret",
			},
			Delegate: ORASDelegate{
				Type:      ORASDelegateTypeKustomize,
				Kustomize: &ManifestKustomize{},
			},
		},
	}

	got := marshalUnmarshal(t, src)
	if !got.ORAS.PlainHTTP {
		t.Error("PlainHTTP = false, want true")
	}
	if !got.ORAS.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = false, want true")
	}
	if got.ORAS.Auth == nil {
		t.Fatal("Auth is nil")
	}
	if got.ORAS.Auth.Username != "alice" || got.ORAS.Auth.Password != "s3cret" {
		t.Errorf("Auth = %+v", got.ORAS.Auth)
	}
}

func TestManifestORAS_YAMLUnmarshal_FromDocsExample(t *testing.T) {
	// ドキュメントのサンプル yaml をパース可能であることを確認する
	doc := `
name: example
type: oras
oras:
  reference: ghcr.io/example/example:v1.2.3
  target: ./base
  plainHTTP: false
  delegate:
    type: helmfile
    helmfile:
      includeCRDs: true
      extraValueFiles:
        - values.yaml
`
	var m Manifest
	if err := yaml.Unmarshal([]byte(doc), &m); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if m.Type != ManifestTypeORAS {
		t.Errorf("Type = %q", m.Type)
	}
	if m.ORAS == nil {
		t.Fatal("ORAS is nil")
	}
	if m.ORAS.Reference != "ghcr.io/example/example:v1.2.3" {
		t.Errorf("Reference = %q", m.ORAS.Reference)
	}
	if m.ORAS.Target != "./base" {
		t.Errorf("Target = %q", m.ORAS.Target)
	}
	if m.ORAS.Delegate.Type != ORASDelegateTypeHelmfile {
		t.Errorf("Delegate.Type = %q", m.ORAS.Delegate.Type)
	}
	if m.ORAS.Delegate.Helmfile == nil {
		t.Fatal("Delegate.Helmfile is nil")
	}
	if !m.ORAS.Delegate.Helmfile.IncludeCRDs {
		t.Error("Delegate.Helmfile.IncludeCRDs = false, want true")
	}
	if len(m.ORAS.Delegate.Helmfile.ExtraValueFiles) != 1 || m.ORAS.Delegate.Helmfile.ExtraValueFiles[0] != "values.yaml" {
		t.Errorf("ExtraValueFiles = %+v", m.ORAS.Delegate.Helmfile.ExtraValueFiles)
	}
}

func marshalUnmarshal(t *testing.T, m Manifest) Manifest {
	t.Helper()
	b, err := yaml.Marshal(m)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got Manifest
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
	}
	return got
}
