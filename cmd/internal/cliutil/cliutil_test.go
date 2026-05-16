package cliutil_test

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

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
