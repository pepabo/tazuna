package v1

import (
	"testing"

	"sigs.k8s.io/yaml"
)

func roundTripTazuna(t *testing.T, src Tazuna) Tazuna {
	t.Helper()
	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got Tazuna
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
	}
	return got
}

func TestTazunaSpec_RoundTrip(t *testing.T) {
	t.Parallel()
	src := Tazuna{
		Spec: TazunaSpec{
			ContextMatches:   []string{"^prod-.*$", "^staging-.*$"},
			ContextMatchMode: ContextMatchModeAND,
			Manifests: []Manifest{
				{
					Name: "app",
					Type: ManifestTypeKustomize,
					Path: "./app",
					Kustomize: &ManifestKustomize{
						DefaultNamespace: "default",
					},
				},
			},
			Tests: []TestPluginSpec{
				{
					Type: TestPluginTypeWaitUntil,
					WaitUntil: &WaitUntilArgs{
						Resource:  WaitUntilResource{APIVersion: "apps/v1", Kind: "Deployment"},
						Namespace: "default",
						Name:      "app",
						Condition: "Available",
					},
				},
			},
		},
	}

	got := roundTripTazuna(t, src)
	if len(got.Spec.ContextMatches) != 2 {
		t.Errorf("ContextMatches len = %d, want 2", len(got.Spec.ContextMatches))
	}
	if got.Spec.ContextMatchMode != ContextMatchModeAND {
		t.Errorf("ContextMatchMode = %q, want %q", got.Spec.ContextMatchMode, ContextMatchModeAND)
	}
	if len(got.Spec.Manifests) != 1 || got.Spec.Manifests[0].Name != "app" {
		t.Errorf("Manifests = %+v", got.Spec.Manifests)
	}
	if got.Spec.Manifests[0].Kustomize == nil || got.Spec.Manifests[0].Kustomize.DefaultNamespace != "default" {
		t.Errorf("Kustomize = %+v", got.Spec.Manifests[0].Kustomize)
	}
	if len(got.Spec.Tests) != 1 || got.Spec.Tests[0].Type != TestPluginTypeWaitUntil {
		t.Errorf("Tests = %+v", got.Spec.Tests)
	}
	if got.Spec.Tests[0].WaitUntil == nil || got.Spec.Tests[0].WaitUntil.Name != "app" {
		t.Errorf("Tests[0].WaitUntil = %+v", got.Spec.Tests[0].WaitUntil)
	}
}

func TestTazunaSpec_RoundTrip_OmitContextFields(t *testing.T) {
	t.Parallel()
	src := Tazuna{
		Spec: TazunaSpec{
			Manifests: []Manifest{{Name: "a", Type: ManifestTypeKustomize, Path: "./a"}},
		},
	}
	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	out := string(b)
	if containsAny(out, "context_matches", "context_match_mode") {
		t.Errorf("expected omitempty fields not to appear, got:\n%s", out)
	}
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
	}
	return false
}

func TestManifest_RoundTrip_AllTypes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		src    Manifest
		verify func(t *testing.T, m Manifest)
	}{
		{
			name: "kustomize",
			src: Manifest{
				Name: "k", Type: ManifestTypeKustomize, Path: "./k",
				Kustomize: &ManifestKustomize{DefaultNamespace: "ns1"},
			},
			verify: func(t *testing.T, m Manifest) {
				if m.Type != ManifestTypeKustomize {
					t.Errorf("Type = %q", m.Type)
				}
				if m.Kustomize == nil || m.Kustomize.DefaultNamespace != "ns1" {
					t.Errorf("Kustomize = %+v", m.Kustomize)
				}
			},
		},
		{
			name: "helmfile",
			src: Manifest{
				Name: "h", Type: ManifestTypeHelmfile, Path: "./h",
				Helmfile: &ManifestHelmfile{
					IncludeCRDs:      true,
					DefaultNamespace: "hns",
					ExtraValueFiles:  []string{"a.yaml", "b.yaml"},
					Wait:             true,
					TimeoutSeconds:   60,
					KubeVersion:      "1.30.0",
				},
			},
			verify: func(t *testing.T, m Manifest) {
				if m.Helmfile == nil {
					t.Fatal("Helmfile nil")
				}
				if !m.Helmfile.IncludeCRDs || !m.Helmfile.Wait {
					t.Errorf("flags lost: %+v", m.Helmfile)
				}
				if m.Helmfile.TimeoutSeconds != 60 || m.Helmfile.KubeVersion != "1.30.0" {
					t.Errorf("Helmfile = %+v", m.Helmfile)
				}
				if len(m.Helmfile.ExtraValueFiles) != 2 {
					t.Errorf("ExtraValueFiles = %+v", m.Helmfile.ExtraValueFiles)
				}
			},
		},
		{
			name: "genesissecret",
			src: Manifest{
				Name: "g", Type: ManifestTypeGenesisSecret, Path: "./g.yaml",
				GenesisSecret: &ManifestGenesisSecret{},
			},
			verify: func(t *testing.T, m Manifest) {
				if m.Type != ManifestTypeGenesisSecret {
					t.Errorf("Type = %q", m.Type)
				}
				if m.GenesisSecret == nil {
					t.Fatal("GenesisSecret nil")
				}
			},
		},
		{
			name: "parallel",
			src: Manifest{
				Name: "p", Type: ManifestTypeParallel,
				Parallel: &ManifestParallel{
					Children: []Manifest{
						{Name: "c1", Type: ManifestTypeKustomize, Path: "./c1"},
						{Name: "c2", Type: ManifestTypeKustomize, Path: "./c2"},
					},
				},
			},
			verify: func(t *testing.T, m Manifest) {
				if m.Parallel == nil || len(m.Parallel.Children) != 2 {
					t.Errorf("Parallel = %+v", m.Parallel)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := yaml.Marshal(tt.src)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			var got Manifest
			if err := yaml.Unmarshal(b, &got); err != nil {
				t.Fatalf("yaml.Unmarshal: %v\nyaml:\n%s", err, string(b))
			}
			tt.verify(t, got)
		})
	}
}

func TestManifest_Includes_RoundTrip(t *testing.T) {
	t.Parallel()
	src := Manifest{
		Name: "inc",
		Includes: []IncludeFile{
			{Path: "child1.yaml"},
			{Path: "child2.yaml"},
		},
	}
	b, err := yaml.Marshal(src)
	if err != nil {
		t.Fatalf("yaml.Marshal: %v", err)
	}
	var got Manifest
	if err := yaml.Unmarshal(b, &got); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if len(got.Includes) != 2 {
		t.Fatalf("Includes len = %d", len(got.Includes))
	}
	if got.Includes[0].Path != "child1.yaml" || got.Includes[1].Path != "child2.yaml" {
		t.Errorf("Includes = %+v", got.Includes)
	}
}

func TestHelmFileVar_RoundTrip(t *testing.T) {
	t.Parallel()
	staticVal := "static-value"
	envName := "MY_VAR"
	tests := []struct {
		name   string
		src    HelmFileVar
		verify func(t *testing.T, v HelmFileVar)
	}{
		{
			name: "static",
			src:  HelmFileVar{From: HelmFileVarFromStatic, Static: &staticVal},
			verify: func(t *testing.T, v HelmFileVar) {
				if v.From != HelmFileVarFromStatic {
					t.Errorf("From = %q", v.From)
				}
				if v.Static == nil || *v.Static != "static-value" {
					t.Errorf("Static = %v", v.Static)
				}
			},
		},
		{
			name: "env",
			src:  HelmFileVar{From: HelmFileVarFromEnv, Env: &envName},
			verify: func(t *testing.T, v HelmFileVar) {
				if v.From != HelmFileVarFromEnv {
					t.Errorf("From = %q", v.From)
				}
				if v.Env == nil || *v.Env != "MY_VAR" {
					t.Errorf("Env = %v", v.Env)
				}
			},
		},
		{
			name: "op",
			src: HelmFileVar{
				From: HelmFileVarFromOp,
				Op: &OnePasswordVaultSelector{
					Key: HelmFileVarOpKeyLabel, Vault: "v", Item: "i", Field: "f",
				},
			},
			verify: func(t *testing.T, v HelmFileVar) {
				if v.From != HelmFileVarFromOp {
					t.Errorf("From = %q", v.From)
				}
				if v.Op == nil {
					t.Fatal("Op nil")
				}
				if v.Op.Key != HelmFileVarOpKeyLabel || v.Op.Vault != "v" {
					t.Errorf("Op = %+v", v.Op)
				}
			},
		},
		{
			name: "staticSlice",
			src:  HelmFileVar{From: HelmFileVarFromStatic, StaticSlice: []string{"a", "b"}},
			verify: func(t *testing.T, v HelmFileVar) {
				if len(v.StaticSlice) != 2 {
					t.Errorf("StaticSlice = %+v", v.StaticSlice)
				}
			},
		},
		{
			name: "staticMap",
			src:  HelmFileVar{From: HelmFileVarFromStatic, StaticMap: map[string]string{"k": "v"}},
			verify: func(t *testing.T, v HelmFileVar) {
				if v.StaticMap["k"] != "v" {
					t.Errorf("StaticMap = %+v", v.StaticMap)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			b, err := yaml.Marshal(tt.src)
			if err != nil {
				t.Fatalf("yaml.Marshal: %v", err)
			}
			var got HelmFileVar
			if err := yaml.Unmarshal(b, &got); err != nil {
				t.Fatalf("yaml.Unmarshal: %v", err)
			}
			tt.verify(t, got)
		})
	}
}
