package main

import (
	"os"

	"github.com/hedwigai/cli/internal/command"
)

//go:generate go run ./tools/generate

// Set via ldflags at build time.
var (
	Version    = "dev"
	CommitHash = "unknown"
)

func main() {
	if err := command.Execute(Version, CommitHash); err != nil {
		os.Exit(1)
	}
}
