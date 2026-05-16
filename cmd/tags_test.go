package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func resetTagsFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_ = rootCmd.PersistentFlags().Set("file-path", "tazuna.yaml")
		_ = rootCmd.PersistentFlags().Set("log-level", "info")
	})
}

func runTags(t *testing.T, args []string) error {
	t.Helper()
	resetTagsFlags(t)
	tagsCmd.SetOut(io.Discard)
	tagsCmd.SetErr(io.Discard)
	if err := tagsCmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	tagsCmd.SetContext(context.Background())
	return tagsCmd.RunE(tagsCmd, []string{})
}

func TestTagsCmd_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - name: web
    type: kustomize
    path: ./web
    tags:
    - frontend
  - name: api
    type: kustomize
    path: ./api
    tags:
    - backend
`), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	if err := runTags(t, []string{"-f", path}); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestTagsCmd_ValidationFails(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte(`apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - name: bad
`), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	if err := runTags(t, []string{"-f", path}); err == nil {
		t.Fatal("expected validation error, got nil")
	}
}

func TestTagsCmd_MissingFile(t *testing.T) {
	if err := runTags(t, []string{"-f", filepath.Join(t.TempDir(), "missing.yaml")}); err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}
