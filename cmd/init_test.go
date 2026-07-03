package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/spf13/pflag"
)

func TestInitialMinimumSupportedVersion(t *testing.T) {
	cases := map[string]string{
		"v1.4.0":  "1.4.0",
		"1.4.0":   "1.4.0",
		"2.0.0":   "2.0.0",
		"dev":     "0.0.0",
		"":        "0.0.0",
		"unknown": "0.0.0",
	}
	for in, want := range cases {
		if got := initialMinimumSupportedVersion(in); got != want {
			t.Errorf("initialMinimumSupportedVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderInitTemplate(t *testing.T) {
	out := renderInitTemplate("1.4.0")
	for _, want := range []string{
		"apiVersion: " + v1.TazunaAPIVersion,
		"kind: " + v1.TazunaKind,
		`minimumSupportedTazunaVersion: "1.4.0"`,
		"manifests: []",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("rendered template missing %q:\n%s", want, out)
		}
	}
}

func runInit(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	rootCmd.SetErr(&buf)
	rootCmd.SetArgs(args)
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		rootCmd.SetOut(nil)
		rootCmd.SetErr(nil)
		// cobra のフラグ値は Execute() をまたいでプロセス内に保持されるため、
		// デフォルトに戻さないと後続のテスト実行（-count>1 など）に漏れる。
		initCmd.Flags().VisitAll(func(f *pflag.Flag) {
			_ = f.Value.Set(f.DefValue)
			f.Changed = false
		})
	})
	err := rootCmd.Execute()
	return buf.String(), err
}

func TestInitCmd_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")

	if _, err := runInit(t, "init", "-f", path); err != nil {
		t.Fatalf("init returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("generated file not readable: %v", err)
	}
	if !strings.Contains(string(data), "minimumSupportedTazunaVersion:") {
		t.Errorf("generated file missing minimumSupportedTazunaVersion:\n%s", data)
	}
}

func TestInitCmd_RefusesOverwriteWithoutForce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte("sentinel"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if _, err := runInit(t, "init", "-f", path); err == nil {
		t.Fatal("expected error when target exists without --force, got nil")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(data) != "sentinel" {
		t.Errorf("file was overwritten without --force: %q", data)
	}
}

func TestInitCmd_ForceOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tazuna.yaml")
	if err := os.WriteFile(path, []byte("sentinel"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	if _, err := runInit(t, "init", "-f", path, "--force"); err != nil {
		t.Fatalf("init --force returned error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if strings.Contains(string(data), "sentinel") {
		t.Errorf("file was not overwritten with --force: %q", data)
	}
	if !strings.Contains(string(data), "apiVersion: "+v1.TazunaAPIVersion) {
		t.Errorf("overwritten file missing generated content:\n%s", data)
	}
}
