package tfstate

import (
	"context"
	"log/slog"

	"github.com/cockroachdb/errors"
	v1 "github.com/pepabo/tazuna/api/v1"
)

// Backend は tfstate の生バイト列を読み出す抽象です。
//
// 新しい terraform backend (gcs / azurerm / http など) に対応するには、
// この interface を実装した型と、それを生成する BackendFactory を用意して
// DefaultFactories (または NewResolver に渡す Factories) に登録します。
type Backend interface {
	// Read は tfstate の JSON をそのまま返します。
	Read(ctx context.Context) ([]byte, error)
	// LogAttrs はログやエラーメッセージに出す「どこを読んでいるか」の情報を返します。
	// 認証情報や state の中身は含めてはいけません。
	LogAttrs() []slog.Attr
}

// BackendFactory は TFStateBackend の設定から Backend を生成します。
// baseDir は tazuna.yaml のディレクトリで、相対パスの解決に使います。
//
// Factory の中ではネットワークアクセスや認証情報のロードを行わず、設定の検査と
// 保持だけを行ってください。実際の I/O は Backend.Read まで遅延させることで、
// tfstate を参照しない実行 (tazuna check 等) で backend に触れずに済みます。
type BackendFactory func(cfg v1.TFStateBackend, baseDir string) (Backend, error)

// Factories は backend type から BackendFactory への対応表です。
type Factories map[v1.TFStateBackendType]BackendFactory

// DefaultFactories は組み込みの backend 一覧を返します。
func DefaultFactories() Factories {
	return Factories{
		v1.TFStateBackendTypeS3:    NewS3Backend,
		v1.TFStateBackendTypeLocal: NewLocalBackend,
	}
}

// New は cfg.Type に対応する Factory で Backend を生成します。
func (f Factories) New(cfg v1.TFStateBackend, baseDir string) (Backend, error) {
	if cfg.Type == "" {
		return nil, errors.New("backend.type is required")
	}
	factory, ok := f[cfg.Type]
	if !ok {
		return nil, errors.Errorf("unsupported tfstate backend type %q (supported: %s)", cfg.Type, f.typeNames())
	}
	b, err := factory(cfg, baseDir)
	if err != nil {
		return nil, errors.Wrapf(err, "%s backend", cfg.Type)
	}
	return b, nil
}

// Supports は backend type が登録済みかを返します。
func (f Factories) Supports(t v1.TFStateBackendType) bool {
	_, ok := f[t]
	return ok
}

func (f Factories) typeNames() string {
	names := make([]string, 0, len(f))
	for t := range f {
		names = append(names, string(t))
	}
	return candidates(sortedStrings(names))
}
