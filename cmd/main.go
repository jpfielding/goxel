package main

import (
	"context"
	"fmt"
	"image"
	"log/slog"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/tag"

	"github.com/jpfielding/goxel/pkg/logging"
)

var (
	GitSHA string = "NA"
)

func main() {
	// register sigterm for graceful shutdown
	ctx, cnc := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cnc()
	slog.SetDefault(logging.Logger(os.Stdout, false, slog.LevelInfo))
	ctx = logging.AppendCtx(ctx,
		slog.Group("goxel",
			slog.String("git", GitSHA),
		))

	a := app.NewWithID("org.goxel")
	go func() {
		defer cnc() // this cnc is from notify and removes the signal so subsequent ctrl-c will restore kill functions
		<-ctx.Done()
		a.Quit()
	}()
	logLifecycle(ctx, a)
	w := a.NewWindow("Goxel")

	nav := &widget.Tree{}

	var img image.Image
	canvasImg := canvas.NewImageFromImage(img)
	canvasImg.FillMode = canvas.ImageFillContain
	canvasImg.ScaleMode = canvas.ImageScaleFastest

	split := container.NewHSplit(nav, canvasImg)
	split.Offset = 0.2
	w.SetContent(split)

	w.SetMainMenu(makeMenu(ctx, a, w, canvasImg))
	w.SetMaster()
	w.Resize(fyne.NewSize(800, 480))

	w.ShowAndRun()
}

func logLifecycle(ctx context.Context, a fyne.App) {
	a.Lifecycle().SetOnStarted(func() {
		slog.InfoContext(ctx, "Lifecycle: Started")
	})
	a.Lifecycle().SetOnStopped(func() {
		slog.InfoContext(ctx, "Lifecycle: Stopped")
	})
	a.Lifecycle().SetOnEnteredForeground(func() {
		slog.InfoContext(ctx, "Lifecycle: Entered Foreground")
	})
	a.Lifecycle().SetOnExitedForeground(func() {
		slog.InfoContext(ctx, "Lifecycle: Exited Foreground")
	})
}
func makeMenu(ctx context.Context, a fyne.App, w fyne.Window, canvasImg *canvas.Image) *fyne.MainMenu {
	open := func() {
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
		defer fileDialog.Show()
		fileDialog.SetFilter(storage.NewExtensionFileFilter([]string{".dcm", ".dcs"}))
		cwd, err := os.Getwd()
		if err != nil {
			slog.ErrorContext(ctx, "error getting current directory", "err", err)
			return
		}
		uri := storage.NewFileURI(cwd)
		lister, err := storage.ListerForURI(uri)
		if err != nil {
			slog.ErrorContext(ctx, "error getting lister for uri", "err", err)
		} else {
			fileDialog.SetLocation(lister)
		}
		fileDialog.Show()
	}
	fileItem := fyne.NewMenuItem("Open File", open)
	fileItem.Icon = theme.FileIcon()

	openSettings := func() {
		w := a.NewWindow("Goxel Settings")
		// w.SetContent(settings.NewSettings().LoadAppearanceScreen(w))
		w.Resize(fyne.NewSize(440, 520))
		w.Show()
	}
	showAbout := func() {
		w := a.NewWindow("About")
		w.SetContent(widget.NewLabel("About Goxel Demo app..."))
		w.Show()
	}
	aboutItem := fyne.NewMenuItem("About", showAbout)
	settingsItem := fyne.NewMenuItem("Settings", openSettings)
	settingsShortcut := &desktop.CustomShortcut{KeyName: fyne.KeyComma, Modifier: fyne.KeyModifierShortcutDefault}
	settingsItem.Shortcut = settingsShortcut
	w.Canvas().AddShortcut(settingsShortcut, func(shortcut fyne.Shortcut) {
		openSettings()
	})

	cutShortcut := &fyne.ShortcutCut{Clipboard: a.Clipboard()}
	cutItem := fyne.NewMenuItem("Cut", func() {
		shortcutFocused(cutShortcut, a.Clipboard(), w.Canvas().Focused())
	})
	cutItem.Shortcut = cutShortcut
	copyShortcut := &fyne.ShortcutCopy{Clipboard: a.Clipboard()}
	copyItem := fyne.NewMenuItem("Copy", func() {
		shortcutFocused(copyShortcut, a.Clipboard(), w.Canvas().Focused())
	})
	copyItem.Shortcut = copyShortcut
	pasteShortcut := &fyne.ShortcutPaste{Clipboard: a.Clipboard()}
	pasteItem := fyne.NewMenuItem("Paste", func() {
		shortcutFocused(pasteShortcut, a.Clipboard(), w.Canvas().Focused())
	})
	pasteItem.Shortcut = pasteShortcut
	performFind := func() { fmt.Println("Menu Find") }
	findItem := fyne.NewMenuItem("Find", performFind)
	findItem.Shortcut = &desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierAlt | fyne.KeyModifierShift | fyne.KeyModifierControl | fyne.KeyModifierSuper}
	w.Canvas().AddShortcut(findItem.Shortcut, func(shortcut fyne.Shortcut) {
		performFind()
	})

	helpMenu := fyne.NewMenu("Help",
		fyne.NewMenuItem("Documentation", func() {
			u, _ := url.Parse("https://docs.io")
			_ = a.OpenURL(u)
		}),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Support", func() {
			u, _ := url.Parse("https://support/")
			_ = a.OpenURL(u)
		}))

	// a quit item will be appended to our first (File) menu
	file := fyne.NewMenu("File", fileItem)
	device := fyne.CurrentDevice()
	if !device.IsMobile() && !device.IsBrowser() {
		file.Items = append(file.Items, fyne.NewMenuItemSeparator(), settingsItem)
	}
	file.Items = append(file.Items, aboutItem)
	main := fyne.NewMainMenu(
		file,
		fyne.NewMenu("Edit", cutItem, copyItem, pasteItem, fyne.NewMenuItemSeparator(), findItem),
		helpMenu,
	)
	return main
}

func shortcutFocused(s fyne.Shortcut, cb fyne.Clipboard, f fyne.Focusable) {
	switch sh := s.(type) {
	case *fyne.ShortcutCopy:
		sh.Clipboard = cb
	case *fyne.ShortcutCut:
		sh.Clipboard = cb
	case *fyne.ShortcutPaste:
		sh.Clipboard = cb
	}
	if focused, ok := f.(fyne.Shortcutable); ok {
		focused.TypedShortcut(s)
	}
}
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
