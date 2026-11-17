package cliutil_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/cmd/internal/cliutil"
)

func TestParseLogLevel(t *testing.T) {
	t.Parallel()
	cases := map[string]slog.Level{
		"debug":   slog.LevelDebug,
		"DEBUG":   slog.LevelDebug,
		"warn":    slog.LevelWarn,
		"error":   slog.LevelError,
		"info":    slog.LevelInfo,
		"":        slog.LevelInfo,
		"unknown": slog.LevelInfo,
	}
	for in, want := range cases {
		t.Run(in, func(t *testing.T) {
			t.Parallel()
			if got := cliutil.ParseLogLevel(in); got != want {
				t.Errorf("ParseLogLevel(%q) = %v, want %v", in, got, want)
			}
		})
	}
}

func TestLoadTazunaYAML(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	yaml := `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests: []
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	got, _, err := cliutil.LoadTazunaYAML(context.Background(), path, "", nil)
	if err != nil {
		t.Fatalf("LoadTazunaYAML returned error: %v", err)
	}
	if got == nil {
		t.Fatal("LoadTazunaYAML returned nil Tazuna")
	}
	if got.APIVersion != "tazuna.pepabo.com/v1" || got.Kind != "Tazuna" {
		t.Errorf("decoded Tazuna mismatch: %+v", got)
	}
}

func TestLoadTazunaYAML_MissingFile(t *testing.T) {
	t.Parallel()
	if _, _, err := cliutil.LoadTazunaYAML(context.Background(), filepath.Join(t.TempDir(), "does-not-exist.yaml"), "", nil); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

// TestLoadTazunaYAML_TemplateEnvironment は tazuna.yaml が Go template として描画され、
// {{ .Environment }} が -e の値で置換されることを確認します。
func TestLoadTazunaYAML_TemplateEnvironment(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	yaml := `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  context_matches:
    - ^{{ .Environment }}-.*$
  manifests: []
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	got, _, err := cliutil.LoadTazunaYAML(context.Background(), path, "staging", nil)
	if err != nil {
		t.Fatalf("LoadTazunaYAML returned error: %v", err)
	}
	if len(got.Spec.ContextMatches) != 1 || got.Spec.ContextMatches[0] != "^staging-.*$" {
		t.Errorf("context_matches = %v, want [^staging-.*$]", got.Spec.ContextMatches)
	}
}

func TestResolveContextMatches(t *testing.T) {
	t.Parallel()

	spec := v1.TazunaSpec{
		ContextMatches:   []string{"^root-.*$"},
		ContextMatchMode: v1.ContextMatchModeAND,
		Environments: map[string]v1.EnvironmentSpec{
			"prod": {
				ContextMatches:   []string{"^prod-.*$"},
				ContextMatchMode: v1.ContextMatchModeOR,
			},
			"staging": {
				// context_match_mode を省略 → root の mode を継承する
				ContextMatches: []string{"^staging-.*$"},
			},
		},
	}

	tests := []struct {
		name        string
		environment string
		wantMatches []string
		wantMode    v1.ContextMatchMode
		expectErr   bool
	}{
		{
			name:        "empty environment uses root",
			environment: "",
			wantMatches: []string{"^root-.*$"},
			wantMode:    v1.ContextMatchModeAND,
		},
		{
			name:        "named environment overrides root",
			environment: "prod",
			wantMatches: []string{"^prod-.*$"},
			wantMode:    v1.ContextMatchModeOR,
		},
		{
			name:        "environment inherits root mode when empty",
			environment: "staging",
			wantMatches: []string{"^staging-.*$"},
			wantMode:    v1.ContextMatchModeAND,
		},
		{
			name:        "unknown environment is an error",
			environment: "does-not-exist",
			expectErr:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			matches, mode, err := cliutil.ResolveContextMatches(spec, tt.environment)
			if tt.expectErr {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mode != tt.wantMode {
				t.Errorf("mode = %q, want %q", mode, tt.wantMode)
			}
			if len(matches) != len(tt.wantMatches) {
				t.Fatalf("matches = %v, want %v", matches, tt.wantMatches)
			}
			for i := range matches {
				if matches[i] != tt.wantMatches[i] {
					t.Errorf("matches[%d] = %q, want %q", i, matches[i], tt.wantMatches[i])
				}
			}
		})
	}
}

func TestCheckMinimumSupportedVersion(t *testing.T) {
	t.Parallel()

	newTazuna := func(min string) *v1.Tazuna {
		return &v1.Tazuna{Spec: v1.TazunaSpec{MinimumSupportedTazunaVersion: min}}
	}

	cases := []struct {
		name    string
		min     string
		current string
		wantErr bool
	}{
		{name: "unset min is always ok", min: "", current: "1.0.0", wantErr: false},
		{name: "current greater than min", min: "1.2.0", current: "1.3.0", wantErr: false},
		{name: "current equal to min", min: "1.2.0", current: "1.2.0", wantErr: false},
		{name: "current less than min", min: "1.2.0", current: "1.1.9", wantErr: true},
		{name: "leading v is accepted on both", min: "v1.2.0", current: "v1.2.0", wantErr: false},
		{name: "leading v on current below min", min: "1.2.0", current: "v1.1.0", wantErr: true},
		{name: "dev current skips the gate", min: "9.9.9", current: "dev", wantErr: false},
		{name: "non-semver current skips the gate", min: "1.2.0", current: "garbage", wantErr: false},
		{name: "invalid min is a config error", min: "not-a-version", current: "1.0.0", wantErr: true},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := cliutil.CheckMinimumSupportedVersion(newTazuna(tc.min), tc.current)
			if tc.wantErr && err == nil {
				t.Fatalf("CheckMinimumSupportedVersion(min=%q, current=%q) = nil, want error", tc.min, tc.current)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("CheckMinimumSupportedVersion(min=%q, current=%q) = %v, want nil", tc.min, tc.current, err)
			}
		})
	}
}

func TestCheckMinimumSupportedVersion_NilTazuna(t *testing.T) {
	t.Parallel()
	if err := cliutil.CheckMinimumSupportedVersion(nil, "1.0.0"); err == nil {
		t.Fatal("expected error for nil tazuna, got nil")
	}
}

// TestLoadTazunaYAML_MinimumVersionGate は、LoadTazunaYAML がデフォルトの
// 実行バージョン("dev")ではゲートを通過することを確認します。SetCurrentVersion で
// 具体的なバージョンを注入すると比較が有効になることもあわせて確認します。
func TestLoadTazunaYAML_MinimumVersionGate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	yaml := `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  minimumSupportedTazunaVersion: "9.9.9"
  manifests: []
`
	if err := os.WriteFile(path, []byte(yaml), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	// デフォルト("dev")ではゲートをスキップするのでエラーにならない。
	if _, _, err := cliutil.LoadTazunaYAML(context.Background(), path, "", nil); err != nil {
		t.Fatalf("LoadTazunaYAML with dev version returned error: %v", err)
	}

	// 実行バージョンを下回る値にすると LoadTazunaYAML がエラーになる。
	cliutil.SetCurrentVersion("1.0.0")
	t.Cleanup(func() { cliutil.SetCurrentVersion("dev") })
	if _, _, err := cliutil.LoadTazunaYAML(context.Background(), path, "", nil); err == nil {
		t.Fatal("expected error when running version is below minimum, got nil")
	}
}

// TestLoadTazunaYAML_TFStateTemplate は 2 パス描画で spec.tfstates を読み取り、
// 同じファイル内の {{ tfstate }} / {{ tfstateJSON }} が解決されることを確認します。
func TestLoadTazunaYAML_TFStateTemplate(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	state := `{"version":4,"serial":1,"outputs":{
  "namespace":{"value":"platform"},
  "tags":{"value":["a","b"]},
  "env_name":{"value":"staging"}
}}`
	if err := os.WriteFile(filepath.Join(dir, "infra.tfstate"), []byte(state), 0o600); err != nil {
		t.Fatalf("write tfstate: %v", err)
	}
	src := `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  tfstates:
    - name: infra
      backend:
        type: local
        local:
          path: ./{{ .Environment }}.tfstate
  manifests:
    - name: app
      type: kustomize
      path: ./app
      kustomize:
        defaultNamespace: {{ tfstate "infra" "output.namespace" }}
      tags: {{ tfstateJSON "infra" "output.tags" }}
`
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write tazuna.yaml: %v", err)
	}

	got, resolver, err := cliutil.LoadTazunaYAML(context.Background(), path, "infra", nil)
	if err != nil {
		t.Fatalf("LoadTazunaYAML returned error: %v", err)
	}
	if resolver == nil || !resolver.Has("infra") {
		t.Fatalf("resolver must contain source infra")
	}
	m := got.Spec.Manifests[0]
	if m.Kustomize == nil || m.Kustomize.DefaultNamespace != "platform" {
		t.Errorf("defaultNamespace = %+v, want platform", m.Kustomize)
	}
	if len(m.Tags) != 2 || m.Tags[0] != "a" || m.Tags[1] != "b" {
		t.Errorf("tags = %v, want [a b]", m.Tags)
	}
	// spec.tfstates の local path も {{ .Environment }} で描画されている
	if p := got.Spec.TFStates[0].Backend.Local.Path; p != "./infra.tfstate" {
		t.Errorf("tfstates[0].backend.local.path = %q", p)
	}
}

func TestLoadTazunaYAML_TFStateErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		src     string
		wantErr string
	}{
		{
			name:    "undeclared source",
			src:     "spec:\n  manifests: []\n  context_matches: ['{{ tfstate \"nope\" \"output.x\" }}']\n",
			wantErr: `tfstate source "nope" is not declared`,
		},
		{
			name:    "invalid tfstates",
			src:     "spec:\n  tfstates:\n    - name: a\n      backend:\n        type: gcs\n  manifests: []\n",
			wantErr: `unsupported tfstate backend type "gcs"`,
		},
		{
			name:    "missing state file",
			src:     "spec:\n  tfstates:\n    - name: a\n      backend:\n        type: local\n        local:\n          path: ./missing.tfstate\n  manifests: []\n  context_matches: ['{{ tfstate \"a\" \"output.x\" }}']\n",
			wantErr: "failed to read tfstate file",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			path := filepath.Join(t.TempDir(), "tazuna.yaml")
			if err := os.WriteFile(path, []byte(tt.src), 0o600); err != nil {
				t.Fatalf("write: %v", err)
			}
			_, _, err := cliutil.LoadTazunaYAML(context.Background(), path, "", nil)
			if err == nil {
				t.Fatalf("expected error containing %q", tt.wantErr)
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

// TestLoadTazunaYAML_NoTFStateAccessWithoutTemplate は template 関数を使わない限り
// backend に一切アクセスしないことを確認します (存在しない path でも成功する)。
func TestLoadTazunaYAML_NoTFStateAccessWithoutTemplate(t *testing.T) {
	t.Parallel()
	src := "spec:\n  tfstates:\n    - name: a\n      backend:\n        type: local\n        local:\n          path: ./missing.tfstate\n  manifests: []\n"
	path := filepath.Join(t.TempDir(), "tazuna.yaml")
	if err := os.WriteFile(path, []byte(src), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, _, err := cliutil.LoadTazunaYAML(context.Background(), path, "", nil); err != nil {
		t.Fatalf("LoadTazunaYAML returned error: %v", err)
	}
}
