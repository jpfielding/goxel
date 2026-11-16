//go:build js

// Command web is the browser (GOOS=js GOARCH=wasm) entry point for goxel.
// It has no CLI, signal handling or log files; it opens the viewer and
// auto-loads the bundled sample so first-time visitors see a study.
package main

import (
	"log/slog"
	"os"

	"fyne.io/fyne/v2/app"
	"github.com/jpfielding/goxel/pkg/goxel"
	"github.com/jpfielding/goxel/pkg/logging"
)

// GitSHA is set at build time with -ldflags "-X main.GitSHA=...".
var GitSHA = "NA"

func main() {
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	slog.Info("goxel web starting", "git", GitSHA)

	a := app.New()
	a.SetIcon(goxel.ResourceIconPng)
	ui := goxel.NewViewer(a)
	ui.LoadSample()
	ui.LoadKeys()
	ui.Run()
}
