package cmd

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// resetCheckFlags clears state that ParseFlags leaves on the package-level
// command tree, so successive tests do not see stale values.
func resetCheckFlags(t *testing.T) {
	t.Helper()
	t.Cleanup(func() {
		_ = checkCmd.Flags().Set("fix", "false")
		_ = rootCmd.PersistentFlags().Set("file-path", "tazuna.yaml")
		_ = rootCmd.PersistentFlags().Set("log-level", "info")
	})
}

func writeYAML(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}
	return path
}

func runCheck(t *testing.T, args []string) error {
	t.Helper()
	resetCheckFlags(t)
	checkCmd.SetOut(io.Discard)
	checkCmd.SetErr(io.Discard)
	if err := checkCmd.ParseFlags(args); err != nil {
		t.Fatalf("ParseFlags: %v", err)
	}
	checkCmd.SetContext(context.Background())
	return checkCmd.RunE(checkCmd, []string{})
}

func TestCheckCmd_Success(t *testing.T) {
	path := writeYAML(t, `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - name: kustomize-app
    type: kustomize
    path: ./kustomize
`)
	if err := runCheck(t, []string{"-f", path}); err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestCheckCmd_InvalidYAML(t *testing.T) {
	path := writeYAML(t, "::: not yaml :::")
	err := runCheck(t, []string{"-f", path})
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
}

func TestCheckCmd_MissingFile(t *testing.T) {
	err := runCheck(t, []string{"-f", filepath.Join(t.TempDir(), "missing.yaml")})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestCheckCmd_ValidationFails(t *testing.T) {
	// manifest has no type / path → ValidateTazunaWithBasePath fails
	path := writeYAML(t, `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - name: bad
`)
	err := runCheck(t, []string{"-f", path})
	if err == nil {
		t.Fatal("expected validation error, got nil")
	}
	if !strings.Contains(err.Error(), "validation failed") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestCheckCmd_Fix(t *testing.T) {
	// manifest without name → --fix assigns one and writes the file back
	path := writeYAML(t, `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - type: kustomize
    path: ./kustomize
`)
	if err := runCheck(t, []string{"-f", path, "--fix"}); err != nil {
		t.Fatalf("--fix returned error: %v", err)
	}
	fixed, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if !strings.Contains(string(fixed), "name:") {
		t.Errorf("expected a name to be assigned, got:\n%s", fixed)
	}
}

func TestCheckCmd_Fix_RefusesTemplateExpressions(t *testing.T) {
	// Go template 式を含むファイルは --fix で描画結果に固定化されてしまうため拒否する
	body := `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - type: kustomize
    path: ./kustomize/{{ .Environment }}
`
	path := writeYAML(t, body)
	err := runCheck(t, []string{"-f", path, "--fix"})
	if err == nil {
		t.Fatal("expected --fix to be refused for template expressions, got nil")
	}
	if !strings.Contains(err.Error(), "template expressions") {
		t.Errorf("unexpected error message: %v", err)
	}
	// ファイルが書き換えられていないことを確認
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	if string(after) != body {
		t.Errorf("file was modified despite refusal:\n%s", after)
	}
}

func TestCheckCmd_Fix_RefusesIncludes(t *testing.T) {
	// includes を持つファイルは --fix で展開結果がインライン化されてしまうため拒否する
	dir := t.TempDir()
	includePath := filepath.Join(dir, "include.yaml")
	if err := os.WriteFile(includePath, []byte(`spec:
  manifests:
  - name: kustomize-app
    type: kustomize
    path: ./kustomize
`), 0o644); err != nil {
		t.Fatalf("write include file: %v", err)
	}
	body := `apiVersion: tazuna.pepabo.com/v1
kind: Tazuna
spec:
  manifests:
  - includes:
    - path: include.yaml
`
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write tmp file: %v", err)
	}

	err := runCheck(t, []string{"-f", path, "--fix"})
	if err == nil {
		t.Fatal("expected --fix to be refused for includes, got nil")
	}
	if !strings.Contains(err.Error(), "includes") {
		t.Errorf("unexpected error message: %v", err)
	}
	after, readErr := os.ReadFile(path)
	if readErr != nil {
		t.Fatalf("read back: %v", readErr)
	}
	if string(after) != body {
		t.Errorf("file was modified despite refusal:\n%s", after)
	}
}
