package manager

import (
	"context"
	"log/slog"

	v1 "github.com/pepabo/tazuna/api/v1"
)

type Manager interface {
	Apply(ctx context.Context, logger *slog.Logger, m v1.Manifest) error
	Destroy(ctx context.Context, logger *slog.Logger, m v1.Manifest) error
	Build(ctx context.Context, logger *slog.Logger, m v1.Manifest) (string, error)
}
