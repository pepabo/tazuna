package genesissecret_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	v1 "github.com/pepabo/tazuna/api/v1"
	"github.com/pepabo/tazuna/pkg/genesissecret"
)

func writeEnvFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("failed to write tempfile: %v", err)
	}
	return path
}

func TestEnvFileProvider_Fetch_Happy(t *testing.T) {
	t.Parallel()
	path := writeEnvFile(t, "USER=alice\nPASS=s3cret\n")
	p := genesissecret.NewEnvFileProvider(path)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{
			"USER": {MapTo: "username"},
			"PASS": {MapTo: "password"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["username"] != "alice" {
		t.Errorf("username = %q, want alice", got["username"])
	}
	if got["password"] != "s3cret" {
		t.Errorf("password = %q, want s3cret", got["password"])
	}
}

func TestEnvFileProvider_Fetch_CommentsAndBlankLines(t *testing.T) {
	t.Parallel()
	content := `# this is a comment
USER=alice

# another comment
PASS=s3cret
`
	path := writeEnvFile(t, content)
	p := genesissecret.NewEnvFileProvider(path)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{
			"USER": {MapTo: "u"},
			"PASS": {MapTo: "p"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["u"] != "alice" || got["p"] != "s3cret" {
		t.Errorf("got = %+v", got)
	}
}

func TestEnvFileProvider_Fetch_Quoting(t *testing.T) {
	t.Parallel()
	content := `SINGLE='alice'
DOUBLE="s3cret"
RAW=plain
EMPTY=""
`
	path := writeEnvFile(t, content)
	p := genesissecret.NewEnvFileProvider(path)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{
			"SINGLE": {MapTo: "s"},
			"DOUBLE": {MapTo: "d"},
			"RAW":    {MapTo: "r"},
			"EMPTY":  {MapTo: "e"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["s"] != "alice" {
		t.Errorf("single-quote stripping failed: %q", got["s"])
	}
	if got["d"] != "s3cret" {
		t.Errorf("double-quote stripping failed: %q", got["d"])
	}
	if got["r"] != "plain" {
		t.Errorf("raw value lost: %q", got["r"])
	}
	if got["e"] != "" {
		t.Errorf("empty value got %q", got["e"])
	}
}

func TestEnvFileProvider_Fetch_MissingKey(t *testing.T) {
	t.Parallel()
	path := writeEnvFile(t, "USER=alice\n")
	p := genesissecret.NewEnvFileProvider(path)

	_, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{
			"NOT_THERE": {MapTo: "x"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing key, got nil")
	}
}

func TestEnvFileProvider_Fetch_FileNotFound(t *testing.T) {
	t.Parallel()
	p := genesissecret.NewEnvFileProvider("/tmp/this-file-does-not-exist-on-purpose.env")

	_, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{
			"USER": {MapTo: "x"},
		},
	})
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestEnvFileProvider_Fetch_InvalidLine(t *testing.T) {
	t.Parallel()
	path := writeEnvFile(t, "thisHasNoEqualsSign\n")
	p := genesissecret.NewEnvFileProvider(path)

	_, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{},
	})
	if err == nil {
		t.Fatal("expected error for invalid line, got nil")
	}
}

func TestEnvFileProvider_Fetch_EmptyItems(t *testing.T) {
	t.Parallel()
	path := writeEnvFile(t, "USER=alice\n")
	p := genesissecret.NewEnvFileProvider(path)

	got, err := p.Fetch(context.Background(), v1.GenesisSecretGenerate{
		Items: map[string]v1.GenesisSecretGenerateItem{},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got = %+v, want empty", got)
	}
}
