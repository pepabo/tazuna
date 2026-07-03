package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/pepabo/tazuna/cmd"
)

// Build-time metadata injected via -ldflags "-X main.version=... -X main.commit=... -X main.date=..."
// See .goreleaser.yaml for how these are populated for release builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)

	// SIGINT/SIGTERM で context をキャンセルし、進行中の apply / サブプロセス /
	// OTLP trace flush などの graceful な後始末を可能にする。
	// 2 度目のシグナルは signal.NotifyContext の仕様により即プロセス終了となる。
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cmd.ExecuteContext(ctx)
}
