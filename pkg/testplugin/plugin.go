package testplugin

import (
	"context"
	"log/slog"

	v1 "github.com/pepabo/tazuna/api/v1"
)

type Plugin interface {
	Type() v1.TestPluginType
	Run(ctx context.Context, logger *slog.Logger, spec *v1.TestPluginSpec) error
}
