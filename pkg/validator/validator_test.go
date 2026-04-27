package validator

import (
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
)

func TestValidateStaticVar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		helmVar   *v1.HelmFileVar
		varName   string
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid static field",
			helmVar: &v1.HelmFileVar{
				From:   v1.HelmFileVarFromStatic,
				Static: stringPtr("test-value"),
			},
			varName:   "testVar",
			expectErr: false,
		},
		{
			name: "valid staticSlice field",
			helmVar: &v1.HelmFileVar{
				From:        v1.HelmFileVarFromStatic,
				StaticSlice: []string{"value1", "value2"},
			},
			varName:   "testVar",
			expectErr: false,
		},
		{
			name: "valid staticMap field",
			helmVar: &v1.HelmFileVar{
				From:      v1.HelmFileVarFromStatic,
				StaticMap: map[string]string{"key1": "value1"},
			},
			varName:   "testVar",
			expectErr: false,
		},
		{
			name: "no static fields",
			helmVar: &v1.HelmFileVar{
				From: v1.HelmFileVarFromStatic,
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has From static but no static/staticSlice/staticMap field",
		},
		{
			name: "multiple static fields - static and staticSlice",
			helmVar: &v1.HelmFileVar{
				From:        v1.HelmFileVarFromStatic,
				Static:      stringPtr("test-value"),
				StaticSlice: []string{"value1", "value2"},
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has From static but multiple static fields are set",
		},
		{
			name: "multiple static fields - all three",
			helmVar: &v1.HelmFileVar{
				From:        v1.HelmFileVarFromStatic,
				Static:      stringPtr("test-value"),
				StaticSlice: []string{"value1", "value2"},
				StaticMap:   map[string]string{"key1": "value1"},
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has From static but multiple static fields are set",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateStaticVar(tt.helmVar, tt.varName)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error message to contain '%s', but got: %s", tt.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateHelmFileVar(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		helmVar   *v1.HelmFileVar
		varName   string
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid env var",
			helmVar: &v1.HelmFileVar{
				From: v1.HelmFileVarFromEnv,
				Env:  stringPtr("TEST_ENV"),
			},
			varName:   "testVar",
			expectErr: false,
		},
		{
			name: "valid static var",
			helmVar: &v1.HelmFileVar{
				From:   v1.HelmFileVarFromStatic,
				Static: stringPtr("test-value"),
			},
			varName:   "testVar",
			expectErr: false,
		},
		{
			name: "valid op var",
			helmVar: &v1.HelmFileVar{
				From: v1.HelmFileVarFromOp,
				Op: &v1.OnePasswordVaultSelector{
					Key:   v1.HelmFileVarOpKeyID,
					Vault: "test-vault",
					Item:  "test-item",
					Field: "test-field",
				},
			},
			varName:   "testVar",
			expectErr: false,
		},
		{
			name: "missing From field",
			helmVar: &v1.HelmFileVar{
				Static: stringPtr("test-value"),
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has no From field",
		},
		{
			name: "invalid From field",
			helmVar: &v1.HelmFileVar{
				From:   "invalid",
				Static: stringPtr("test-value"),
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has unsupported From field",
		},
		{
			name: "env From but no env field",
			helmVar: &v1.HelmFileVar{
				From: v1.HelmFileVarFromEnv,
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has From env but no env field",
		},
		{
			name: "op From but no op field",
			helmVar: &v1.HelmFileVar{
				From: v1.HelmFileVarFromOp,
			},
			varName:   "testVar",
			expectErr: true,
			errMsg:    "has From op but no op field",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateHelmFileVar(tt.helmVar, tt.varName)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error message to contain '%s', but got: %s", tt.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateManifest(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		manifest  *v1.Manifest
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid kustomize manifest",
			manifest: &v1.Manifest{
				Path: "/path/to/kustomize",
				Type: v1.ManifestTypeKustomize,
			},
			expectErr: false,
		},
		{
			name: "valid helmfile manifest",
			manifest: &v1.Manifest{
				Path: "/path/to/helmfile",
				Type: v1.ManifestTypeHelmfile,
				Helmfile: &v1.ManifestHelmfile{
					Vars: map[string]v1.HelmFileVar{
						"testVar": {
							From:   v1.HelmFileVarFromStatic,
							Static: stringPtr("test-value"),
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name: "missing path",
			manifest: &v1.Manifest{
				Type: v1.ManifestTypeKustomize,
			},
			expectErr: true,
			errMsg:    "manifest path is required",
		},
		{
			name: "missing type",
			manifest: &v1.Manifest{
				Path: "/path/to/manifest",
			},
			expectErr: true,
			errMsg:    "manifest type is required",
		},
		{
			name: "invalid type",
			manifest: &v1.Manifest{
				Path: "/path/to/manifest",
				Type: "invalid",
			},
			expectErr: true,
			errMsg:    "unsupported manifest type",
		},
		{
			name: "invalid helmfile vars",
			manifest: &v1.Manifest{
				Path: "/path/to/helmfile",
				Type: v1.ManifestTypeHelmfile,
				Helmfile: &v1.ManifestHelmfile{
					Vars: map[string]v1.HelmFileVar{
						"testVar": {
							From: v1.HelmFileVarFromStatic,
							// missing static fields
						},
					},
				},
			},
			expectErr: true,
			errMsg:    "helmfile var 'testVar' validation failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateManifest(tt.manifest, "")
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error message to contain '%s', but got: %s", tt.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

// Helper functions
func stringPtr(s string) *string {
	return &s
}

func TestValidateManifestWithIncludes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		manifest  *v1.Manifest
		basePath  string
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid includes manifest",
			manifest: &v1.Manifest{
				Name:        "test includes",
				Description: "includes other manifests",
				Includes: []v1.IncludeFile{
					{Path: "kustomize.yaml"},
					{Path: "genesissecret.yaml"},
				},
			},
			basePath:  "",
			expectErr: false,
		},
		{
			name: "empty includes",
			manifest: &v1.Manifest{
				Name:     "test includes",
				Includes: []v1.IncludeFile{},
			},
			basePath:  "",
			expectErr: true,
			errMsg:    "includes is specified but empty",
		},
		{
			name: "includes with empty path",
			manifest: &v1.Manifest{
				Name: "test includes",
				Includes: []v1.IncludeFile{
					{Path: "valid.yaml"},
					{Path: ""}, // empty path
				},
			},
			basePath:  "",
			expectErr: true,
			errMsg:    "include[1].path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateManifestWithIncludes(tt.manifest, tt.basePath)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error message to contain '%s', but got: %s", tt.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateManifest_WithIncludes(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		manifest  *v1.Manifest
		basePath  string
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid manifest with includes",
			manifest: &v1.Manifest{
				Name:        "test includes",
				Description: "includes other manifests",
				Includes: []v1.IncludeFile{
					{Path: "kustomize.yaml"},
				},
				// 以下のフィールドはincludesが指定されている場合無視される
				Path: "/should/be/ignored",
				Type: v1.ManifestTypeKustomize,
			},
			basePath:  "",
			expectErr: false,
		},
		{
			name: "manifest without includes should validate normally",
			manifest: &v1.Manifest{
				Path: "/path/to/kustomize",
				Type: v1.ManifestTypeKustomize,
			},
			basePath:  "",
			expectErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateManifest(tt.manifest, tt.basePath)
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error message to contain '%s', but got: %s", tt.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestValidateTazunaSpec_ContextMatches(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		spec      *v1.TazunaSpec
		expectErr bool
		errMsg    string
	}{
		{
			name: "valid regex pattern",
			spec: &v1.TazunaSpec{
				ContextMatches: []string{"^kind-.*$"},
			},
			expectErr: false,
		},
		{
			name: "multiple valid patterns",
			spec: &v1.TazunaSpec{
				ContextMatches: []string{"^kind-.*$", "^prod-.*$"},
			},
			expectErr: false,
		},
		{
			name: "empty context_matches (skip validation)",
			spec: &v1.TazunaSpec{
				ContextMatches: nil,
			},
			expectErr: false,
		},
		{
			name: "invalid regex pattern",
			spec: &v1.TazunaSpec{
				ContextMatches: []string{"[invalid"},
			},
			expectErr: true,
			errMsg:    "context_matches[0] is not a valid regex",
		},
		{
			name: "second pattern invalid",
			spec: &v1.TazunaSpec{
				ContextMatches: []string{"^valid$", "[invalid"},
			},
			expectErr: true,
			errMsg:    "context_matches[1] is not a valid regex",
		},
		{
			name: "valid context_match_mode or",
			spec: &v1.TazunaSpec{
				ContextMatches:   []string{"^kind-.*$"},
				ContextMatchMode: v1.ContextMatchModeOR,
			},
			expectErr: false,
		},
		{
			name: "valid context_match_mode and",
			spec: &v1.TazunaSpec{
				ContextMatches:   []string{"^kind-.*$"},
				ContextMatchMode: v1.ContextMatchModeAND,
			},
			expectErr: false,
		},
		{
			name: "invalid context_match_mode",
			spec: &v1.TazunaSpec{
				ContextMatches:   []string{"^kind-.*$"},
				ContextMatchMode: "xor",
			},
			expectErr: true,
			errMsg:    "context_match_mode must be 'or' or 'and'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateTazunaSpec(tt.spec, "")
			if tt.expectErr {
				if err == nil {
					t.Errorf("expected error but got nil")
					return
				}
				if tt.errMsg != "" {
					if !containsString(err.Error(), tt.errMsg) {
						t.Errorf("expected error message to contain '%s', but got: %s", tt.errMsg, err.Error())
					}
				}
			} else {
				if err != nil {
					t.Errorf("expected no error but got: %v", err)
				}
			}
		})
	}
}

func containsString(haystack, needle string) bool {
	return len(haystack) >= len(needle) &&
		(haystack == needle || len(needle) == 0 ||
			func() bool {
				for i := 0; i <= len(haystack)-len(needle); i++ {
					if haystack[i:i+len(needle)] == needle {
						return true
					}
				}
				return false
			}())
}
