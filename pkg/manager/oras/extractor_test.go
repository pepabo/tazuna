package oras

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type tarEntry struct {
	name     string
	typeflag byte
	mode     int64
	body     []byte
	linkname string
}

// buildTarGz は tarEntry のスライスを gzip圧縮した tar stream に変換する。
func buildTarGz(t *testing.T, entries []tarEntry) io.Reader {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		mode := e.mode
		if mode == 0 {
			mode = 0o644
			if e.typeflag == tar.TypeDir {
				mode = 0o755
			}
		}
		hdr := &tar.Header{
			Name:     e.name,
			Mode:     mode,
			Size:     int64(len(e.body)),
			Typeflag: e.typeflag,
			Linkname: e.linkname,
		}
		if e.typeflag == tar.TypeSymlink || e.typeflag == tar.TypeLink || e.typeflag == tar.TypeDir {
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("write header %q: %v", e.name, err)
		}
		if hdr.Size > 0 {
			if _, err := tw.Write(e.body); err != nil {
				t.Fatalf("write body %q: %v", e.name, err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("close gzip: %v", err)
	}
	return &buf
}

func TestExtract_Success(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "kustomization.yaml", typeflag: tar.TypeReg, body: []byte("resources:\n- deployment.yaml\n")},
		{name: "deployment.yaml", typeflag: tar.TypeReg, body: []byte("kind: Deployment\n")},
		{name: "subdir/", typeflag: tar.TypeDir},
		{name: "subdir/inner.txt", typeflag: tar.TypeReg, body: []byte("hello")},
		{name: "link.yaml", typeflag: tar.TypeSymlink, linkname: "deployment.yaml"},
	})
	if err := Extract(r, dest); err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}

	cases := map[string]string{
		"kustomization.yaml": "resources:\n- deployment.yaml\n",
		"deployment.yaml":    "kind: Deployment\n",
		"subdir/inner.txt":   "hello",
	}
	for name, want := range cases {
		got, err := os.ReadFile(filepath.Join(dest, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		if string(got) != want {
			t.Errorf("%s: got %q, want %q", name, got, want)
		}
	}

	linkPath := filepath.Join(dest, "link.yaml")
	target, err := os.Readlink(linkPath)
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "deployment.yaml" {
		t.Errorf("link target: got %q, want deployment.yaml", target)
	}
}

func TestExtract_RejectsParentTraversal(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "../evil.txt", typeflag: tar.TypeReg, body: []byte("pwn")},
	})
	err := Extract(r, dest)
	if err == nil {
		t.Fatal("expected error for parent traversal, got nil")
	}
	if !strings.Contains(err.Error(), "zip slip") && !strings.Contains(err.Error(), "escapes dest") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsAbsolutePath(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "/etc/passwd", typeflag: tar.TypeReg, body: []byte("root:x:0:0")},
	})
	err := Extract(r, dest)
	if err == nil {
		t.Fatal("expected error for absolute path, got nil")
	}
	if !strings.Contains(err.Error(), "absolute") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsSymlinkEscape(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "link", typeflag: tar.TypeSymlink, linkname: "../../etc/passwd"},
	})
	err := Extract(r, dest)
	if err == nil {
		t.Fatal("expected error for escaping symlink, got nil")
	}
	if !strings.Contains(err.Error(), "escapes dest") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsAbsoluteSymlink(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "link", typeflag: tar.TypeSymlink, linkname: "/etc/passwd"},
	})
	err := Extract(r, dest)
	if err == nil {
		t.Fatal("expected error for absolute symlink, got nil")
	}
	if !strings.Contains(err.Error(), "absolute link target") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsSizeLimit(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "big.txt", typeflag: tar.TypeReg, body: bytes.Repeat([]byte("a"), 100)},
	})
	err := ExtractWithLimits(r, dest, Limits{MaxTotalBytes: 10, MaxEntries: 100})
	if err == nil {
		t.Fatal("expected error for size limit, got nil")
	}
	if !strings.Contains(err.Error(), "size") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsEntryLimit(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "a.txt", typeflag: tar.TypeReg, body: []byte("a")},
		{name: "b.txt", typeflag: tar.TypeReg, body: []byte("b")},
		{name: "c.txt", typeflag: tar.TypeReg, body: []byte("c")},
	})
	err := ExtractWithLimits(r, dest, Limits{MaxTotalBytes: 1024, MaxEntries: 2})
	if err == nil {
		t.Fatal("expected error for entry limit, got nil")
	}
	if !strings.Contains(err.Error(), "entry count") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsCorruptGzip(t *testing.T) {
	dest := t.TempDir()
	err := Extract(strings.NewReader("not a gzip stream"), dest)
	if err == nil {
		t.Fatal("expected error for corrupt gzip, got nil")
	}
	if !strings.Contains(err.Error(), "gzip") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_RejectsUnsupportedType(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "fifo", typeflag: tar.TypeFifo},
	})
	err := Extract(r, dest)
	if err == nil {
		t.Fatal("expected error for unsupported type, got nil")
	}
	if !strings.Contains(err.Error(), "unsupported") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestExtract_CreatesNestedDirs(t *testing.T) {
	dest := t.TempDir()
	r := buildTarGz(t, []tarEntry{
		{name: "a/b/c/file.txt", typeflag: tar.TypeReg, body: []byte("nested")},
	})
	if err := Extract(r, dest); err != nil {
		t.Fatalf("Extract: unexpected error: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dest, "a", "b", "c", "file.txt"))
	if err != nil {
		t.Fatalf("read nested file: %v", err)
	}
	if string(got) != "nested" {
		t.Errorf("got %q, want %q", got, "nested")
	}
}

func TestDefaultLimits(t *testing.T) {
	l := DefaultLimits()
	if l.MaxTotalBytes != 1<<30 {
		t.Errorf("MaxTotalBytes: got %d, want %d", l.MaxTotalBytes, 1<<30)
	}
	if l.MaxEntries != 10000 {
		t.Errorf("MaxEntries: got %d, want %d", l.MaxEntries, 10000)
	}
}

// io.EOF が誤って捕捉されないよう sentinel チェック。
var _ = errors.Is
