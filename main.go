package main

import "github.com/pepabo/tazuna/cmd"

// Build-time metadata injected via -ldflags "-X main.version=... -X main.commit=... -X main.date=..."
// See .goreleaser.yaml for how these are populated for release builds.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	cmd.Execute()
}
