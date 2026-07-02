package cliutil_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pepabo/tazuna/cmd/internal/cliutil"
)

func TestAtomicWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("creates new file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "out.yaml")
		if err := cliutil.AtomicWriteFile(path, []byte("hello"), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want hello", got)
		}
	})

	t.Run("overwrites existing file", func(t *testing.T) {
		t.Parallel()
		path := filepath.Join(t.TempDir(), "out.yaml")
		if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := cliutil.AtomicWriteFile(path, []byte("new"), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read back: %v", err)
		}
		if string(got) != "new" {
			t.Errorf("got %q, want new", got)
		}
	})

	t.Run("no temp file left behind", func(t *testing.T) {
		t.Parallel()
		dir := t.TempDir()
		path := filepath.Join(dir, "out.yaml")
		if err := cliutil.AtomicWriteFile(path, []byte("data"), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Errorf("expected only the target file in dir, got %d entries", len(entries))
		}
	})
}
