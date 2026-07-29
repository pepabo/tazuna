// Package oras はOCI registry (ORAS) からpullしたartifactを
// ローカルに展開し、helmfile / kustomize manager に委譲するための機能を提供します。
//
// このファイルでは pullした tar.gz artifact を安全にローカルディレクトリへ
// 展開する `Extract` を実装します。
package oras

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Limits は extract 時のリソース上限を表します。
// テストや統合テストから上書きできるよう構造体として公開しています。
type Limits struct {
	// MaxTotalBytes は展開後のファイル合計サイズの上限 (bytes) です。
	MaxTotalBytes int64
	// MaxEntries は tar entry 数の上限です。
	MaxEntries int
}

const (
	// defaultMaxTotalBytes は展開後合計サイズ上限 (1 GiB)。
	defaultMaxTotalBytes int64 = 1 << 30
	// defaultMaxEntries は entry 数上限。
	defaultMaxEntries = 10000
)

// DefaultLimits はデフォルトの上限を返します。
func DefaultLimits() Limits {
	return Limits{
		MaxTotalBytes: defaultMaxTotalBytes,
		MaxEntries:    defaultMaxEntries,
	}
}

// Extract は gzip+tar stream を destDir に展開します。
// destDir は呼び出し前に作成済みである必要があります。
//
// 以下の不正な entry を拒否します:
//   - 絶対パス
//   - `..` を含む親ディレクトリ参照 (zip slip)
//   - destDir 配下を脱出する symlink / hardlink
//   - 上限を超える合計サイズ / entry 数
//   - サポート外の type (char/block/fifo 等)
func Extract(r io.Reader, destDir string) error {
	return ExtractWithLimits(r, destDir, DefaultLimits())
}

// ExtractWithLimits は Extract の上限差し替え版です。
func ExtractWithLimits(r io.Reader, destDir string, limits Limits) error {
	absDest, err := filepath.Abs(destDir)
	if err != nil {
		return fmt.Errorf("oras extract: resolve destDir: %w", err)
	}

	gzr, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("oras extract: gzip reader: %w", err)
	}
	defer func() { _ = gzr.Close() }()

	tr := tar.NewReader(gzr)

	var (
		entries    int
		totalBytes int64
	)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("oras extract: tar next: %w", err)
		}

		entries++
		if entries > limits.MaxEntries {
			return fmt.Errorf("oras extract: entry count exceeds limit (%d)", limits.MaxEntries)
		}

		targetPath, err := safeJoin(absDest, header.Name)
		if err != nil {
			return fmt.Errorf("oras extract: %q: %w", header.Name, err)
		}

		mode := header.FileInfo().Mode()

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return fmt.Errorf("oras extract: mkdir %q: %w", targetPath, err)
			}

		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("oras extract: mkdir parent of %q: %w", targetPath, err)
			}

			remaining := max(limits.MaxTotalBytes-totalBytes, 0)
			// 上限+1まで読んで超過を検知する。
			limitedReader := io.LimitReader(tr, remaining+1)

			f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode.Perm())
			if err != nil {
				return fmt.Errorf("oras extract: open %q: %w", targetPath, err)
			}

			n, copyErr := io.Copy(f, limitedReader)
			closeErr := f.Close()
			if copyErr != nil {
				return fmt.Errorf("oras extract: write %q: %w", targetPath, copyErr)
			}
			if closeErr != nil {
				return fmt.Errorf("oras extract: close %q: %w", targetPath, closeErr)
			}
			totalBytes += n
			if totalBytes > limits.MaxTotalBytes {
				return fmt.Errorf("oras extract: total size exceeds limit (%d bytes)", limits.MaxTotalBytes)
			}

		case tar.TypeSymlink, tar.TypeLink:
			// link target が destDir 配下を指すか検証する。
			// 相対pathは header の親ディレクトリ基準で解決する。
			linkname := header.Linkname
			var resolved string
			if filepath.IsAbs(linkname) {
				return fmt.Errorf("oras extract: %q: absolute link target %q is not allowed", header.Name, linkname)
			}
			resolved = filepath.Join(filepath.Dir(targetPath), linkname)
			if _, err := ensureWithin(absDest, resolved); err != nil {
				return fmt.Errorf("oras extract: %q: link target escapes dest: %w", header.Name, err)
			}

			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return fmt.Errorf("oras extract: mkdir parent of %q: %w", targetPath, err)
			}
			if header.Typeflag == tar.TypeSymlink {
				if err := os.Symlink(linkname, targetPath); err != nil {
					return fmt.Errorf("oras extract: symlink %q: %w", targetPath, err)
				}
			} else {
				if err := os.Link(resolved, targetPath); err != nil {
					return fmt.Errorf("oras extract: hardlink %q: %w", targetPath, err)
				}
			}

		default:
			return fmt.Errorf("oras extract: %q: unsupported tar entry type %q", header.Name, string(header.Typeflag))
		}
	}

	return nil
}

// safeJoin は entry name を destDir に結合し、配下を脱出していないことを検証します。
func safeJoin(absDest, name string) (string, error) {
	if name == "" {
		return "", fmt.Errorf("empty entry name")
	}
	if filepath.IsAbs(name) || strings.HasPrefix(name, "/") {
		return "", fmt.Errorf("absolute path is not allowed")
	}
	cleaned := filepath.Clean(name)
	if cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes dest (zip slip)")
	}

	joined := filepath.Join(absDest, cleaned)
	return ensureWithin(absDest, joined)
}

// ensureWithin は target が absDest 配下にあることを検証します。
func ensureWithin(absDest, target string) (string, error) {
	absTarget, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve target: %w", err)
	}
	rel, err := filepath.Rel(absDest, absTarget)
	if err != nil {
		return "", fmt.Errorf("relative path: %w", err)
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes dest")
	}
	return absTarget, nil
}
