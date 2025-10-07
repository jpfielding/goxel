//go:generate fyne bundle -o bundle.go icon.png

package main

import (
	"context"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/storage"

	"github.com/jpfielding/goxel/pkg/logging"
)

var (
	GitSHA string = "NA"
)

func main() {
	// register sigterm for graceful shutdown
	ctx, cnc := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cnc()
	go func() {
		defer cnc() // this cnc is from notify and removes the signal so subsequent ctrl-c will restore kill functions
		<-ctx.Done()
	}()
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	ctx = logging.AppendCtx(ctx, slog.Group("goxel", slog.String("git", GitSHA)))
	a := app.New()
	a.SetIcon(resourceIconPng)

	ui := makeUI(a)
	if len(os.Args) > 1 {
		path := os.Args[1]

		info, err := os.Stat(path)
		if err == nil && info.IsDir() {
			dir, err := storage.ListerForURI(storage.NewFileURI(path))
			if err != nil {
				log.Println("Failed to open folder at path:", path)
				return
			}
			ui.loadDir(dir)
		} else {
			r, err := os.Open(path)
			if err != nil {
				log.Println("Failed to load file at path:", path)
				return
			}
			ui.loadFile(r, fileLength(path))
		}
	}

	ui.loadKeys()
	ui.win.ShowAndRun()
}
