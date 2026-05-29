package cliutil_test

import (
	"log/slog"
	"os"
	"path/filepath"
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

	got, err := cliutil.LoadTazunaYAML(path)
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
	if _, err := cliutil.LoadTazunaYAML(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatal("expected error for missing file, got nil")
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
	if _, err := cliutil.LoadTazunaYAML(path); err != nil {
		t.Fatalf("LoadTazunaYAML with dev version returned error: %v", err)
	}

	// 実行バージョンを下回る値にすると LoadTazunaYAML がエラーになる。
	cliutil.SetCurrentVersion("1.0.0")
	t.Cleanup(func() { cliutil.SetCurrentVersion("dev") })
	if _, err := cliutil.LoadTazunaYAML(path); err == nil {
		t.Fatal("expected error when running version is below minimum, got nil")
	}
}
