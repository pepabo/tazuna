package tfstate

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// LocalBackend はローカルファイルの tfstate を読む Backend です。
// terraform の `backend "local"` (および backend 未指定時の terraform.tfstate) に相当します。
type LocalBackend struct {
	path string
}

var _ Backend = (*LocalBackend)(nil)

// NewLocalBackend は BackendFactory です。local.path の相対パスは baseDir から解決します。
func NewLocalBackend(cfg v1.TFStateBackend, baseDir string) (Backend, error) {
	if cfg.Local == nil {
		return nil, errors.New("local config is required for local backend")
	}
	if cfg.Local.Path == "" {
		return nil, errors.New("local.path is required")
	}
	path := cfg.Local.Path
	if !filepath.IsAbs(path) && baseDir != "" {
		path = filepath.Join(baseDir, path)
	}
	return &LocalBackend{path: path}, nil
}

// Read implements Backend.
func (b *LocalBackend) Read(_ context.Context) ([]byte, error) {
	data, err := os.ReadFile(b.path)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read tfstate file %q", b.path)
	}
	return data, nil
}

// LogAttrs implements Backend.
func (b *LocalBackend) LogAttrs() []slog.Attr {
	return []slog.Attr{slog.String("path", b.path)}
}
