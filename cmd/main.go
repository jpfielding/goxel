package main

import (
	"context"
	"image"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/widget"
	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"

	"github.com/jpfielding/goxel/pkg/logging"
)

var (
	GitSHA string = "NA"
)

func loadDicomFile(ctx context.Context, path string, w fyne.Window, canvasImg *canvas.Image) {
	slog.InfoContext(ctx, "loading dicom file", "path", path)

	p, err := dicom.ParseFile(path, nil)
	if err != nil {
		slog.ErrorContext(ctx, "error parsing dicom file", "err", err, "path", path)
		return
	}

	pixelDataElement, err := p.FindElementByTag(tag.PixelData)
	if err != nil {
		slog.ErrorContext(ctx, "error finding pixel data", "err", err)
		return
	}
	pixelData := pixelDataElement.Value.GetValue().(dicom.PixelDataInfo)
	if len(pixelData.Frames) > 0 {
		img, err := pixelData.Frames[0].NativeData.GetImage()
		if err != nil {
			slog.ErrorContext(ctx, "error getting image from frame", "err", err)
			return
		}
		canvasImg.Image = img
		canvasImg.Refresh()
		w.SetTitle(filepath.Base(path))
	} else {
		slog.InfoContext(ctx, "no frames found in dicom file")
	}
}

func main() {
	// register sigterm for graceful shutdown
	ctx, cnc := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cnc()
	go func() {
		defer cnc() // this cnc is from notify and removes the signal so subsequent ctrl-c will restore kill functions
		<-ctx.Done()
	}()
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	ctx = logging.AppendCtx(ctx,
		slog.Group("goxel",
			slog.String("git", GitSHA),
		))

	a := app.New()
	w := a.NewWindow("Goxel")
	w.Resize(fyne.NewSize(600, 400))

	var img image.Image
	canvasImg := canvas.NewImageFromImage(img)
	canvasImg.FillMode = canvas.ImageFillContain
	canvasImg.ScaleMode = canvas.ImageScaleFastest

	openButton := widget.NewButton("Open DICOM file", func() {
		fileDialog := dialog.NewFileOpen(func(uri fyne.URIReadCloser, err error) {
			if err != nil {
				slog.ErrorContext(ctx, "error opening file", "err", err)
				return
			}
			if uri == nil {
				return
			}
			loadDicomFile(ctx, uri.URI().Path(), w, canvasImg)
		}, w)

		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".dcm", ".dcs"}))

		cwd, err := os.Getwd()
		if err != nil {
			slog.ErrorContext(ctx, "error getting current directory", "err", err)
		} else {
			uri := storage.NewFileURI(cwd)
			lister, err := storage.ListerForURI(uri)
			if err != nil {
				slog.ErrorContext(ctx, "error getting lister for uri", "err", err)
			} else {
				fileDialog.SetLocation(lister)
			}
		}
		fileDialog.Show()
	})

	w.SetContent(container.NewVBox(
		openButton,
		canvasImg,
	))

	go func() {
		cwd, err := os.Getwd()
		if err != nil {
			slog.ErrorContext(ctx, "error getting current directory", "err", err)
			return
		}
		var dicomPath string
		err = filepath.Walk(cwd, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			ext := filepath.Ext(path)
			if !info.IsDir() && (ext == ".dcm" || ext == ".dcs") {
				dicomPath = path
				return filepath.SkipDir // stop walking
			}
			return nil
		})
		if err != nil {
			slog.ErrorContext(ctx, "error walking directory", "err", err)
			return
		}

		if dicomPath != "" {
			loadDicomFile(ctx, dicomPath, w, canvasImg)
		} else {
			slog.InfoContext(ctx, "no .dcm or .dcs files found in directory")
		}
	}()

	w.ShowAndRun()
}
