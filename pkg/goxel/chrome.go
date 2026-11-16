package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// appBar: mark, name, and the open study's file name (middle-elided) on the right.
func (v *Viewer) appBar() fyne.CanvasObject {
	compact := v.st.layout.compact()
	markSize, nameSize := float32(24), float32(21)
	if compact {
		markSize, nameSize = 22, 19
	}
	left := container.NewHBox(
		container.NewCenter(markImage(markSize)),
		container.NewCenter(text("goxel", nameSize, colHeading, false)),
	)
	var right fyne.CanvasObject = layout.NewSpacer()
	if v.st.loadedName != "" && v.loaded() {
		maxRunes := 64
		if compact {
			maxRunes = 28
		}
		name := text(elideMiddle(v.st.loadedName, maxRunes), 13, colMuted, false)
		name.Alignment = fyne.TextAlignTrailing
		right = shrinkable(container.NewVBox(layout.NewSpacer(), name, layout.NewSpacer()))
	}
	vpad := float32(8)
	if compact {
		vpad = 6
	}
	row := container.NewBorder(nil, nil, left, nil, right)
	return panel(colPanel, 0, inset(16, vpad, minHeight(28, row)))
}

// navigationText is the status bar's one line of hints (roxel's wording).
func navigationText(st *uiState, touch bool) string {
	if st.focusSlice {
		if touch {
			return "Drag to pan  ·  Pinch to zoom"
		}
		return "Drag to pan  ·  Zoom with the slider  ·  Esc to exit focus"
	}
	switch {
	case touch:
		return "Drag to orbit  ·  Pinch to zoom  ·  Use Pan on the Camera page to pan"
	case st.focusVolume:
		return "Drag to orbit  ·  Shift + drag to pan  ·  Scroll to zoom  ·  Esc to exit focus"
	default:
		return "Drag to orbit  ·  Shift + drag to pan  ·  Scroll to zoom  ·  Arrow keys step slices"
	}
}

func (v *Viewer) statusBar() fyne.CanvasObject {
	t := text(navigationText(&v.st, hasTouch()), 12, colMuted, false)
	return panel(colBG, 0, inset(16, 5, shrinkable(t)))
}

// emptyState is the first-use card: what this is and the one action.
func (v *Viewer) emptyState() fyne.CanvasObject {
	compact := v.st.layout.compact()
	markSize := float32(64)
	if compact {
		markSize = 56
	}
	title := text("Open a DICOM study", 22, colHeading, false)
	title.Alignment = fyne.TextAlignCenter
	intro := note("See a CT scan as a 3D volume and as 2D slices.")
	intro.Alignment = fyne.TextAlignCenter

	formats := "DICOM .dcm or .dcs files, or a study folder."
	switch {
	case !canOpenFolder:
		formats = "DICOM .dcm or .dcs files. Try the sample to see a CT study."
	case compact:
		formats = "DICOM .dcm or .dcs files."
	}
	fmtNote := note(formats)
	fmtNote.Alignment = fyne.TextAlignCenter
	fmtNote.SizeName = theme.SizeNameCaptionText

	card := container.NewVBox(
		container.NewCenter(markImage(markSize)),
		container.NewCenter(title),
		intro,
		layout.NewSpacer(),
		v.openButtons(true),
		v.loadStatus(),
		fmtNote,
	)
	width := float32(320)
	if compact {
		width = 340
	}
	top := float32(0.22)
	return panel(colBG, 24, container.NewVScroll(container.New(&centredColumn{width: width, top: top}, card)))
}

// centredColumn centres one child horizontally at a fixed width and places
// it a fraction of the way down.
type centredColumn struct {
	width, top float32
}

func (c *centredColumn) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		w := min(c.width, size.Width)
		h := o.MinSize().Height
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos((size.Width-w)/2, max((size.Height-h)*c.top, 0)))
	}
}

func (c *centredColumn) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(min(c.width, 240), h)
}

// openButtons: "Open file…" (primary) and "Open folder…" on desktop or
// "Load sample" in the browser.
func (v *Viewer) openButtons(primary bool) fyne.CanvasObject {
	busy := v.st.loadingFile != ""
	open := widget.NewButton("Open file…", v.openFile)
	if primary {
		open.Importance = widget.HighImportance
	}
	var second *widget.Button
	if canOpenFolder {
		second = widget.NewButton("Open folder…", v.openFolder)
	} else {
		second = widget.NewButton("Load sample", v.LoadSample)
	}
	if busy {
		open.Disable()
		second.Disable()
	}
	h := float32(32)
	if primary || v.st.layout.compact() {
		h = touchTarget
	}
	return container.NewVBox(minHeight(h, open), minHeight(h, second))
}

// loadStatus: a spinner while loading, or the last error with a retry hint.
func (v *Viewer) loadStatus() fyne.CanvasObject {
	switch {
	case v.st.loadingFile != "":
		bar := widget.NewProgressBarInfinite()
		return container.NewVBox(note("Loading "+elideMiddle(v.st.loadingFile, 40)+"…"), bar)
	case v.st.loadError != "":
		msg := widget.NewLabel(v.st.loadError)
		msg.Wrapping = fyne.TextWrapWord
		msg.Importance = widget.DangerImportance
		return container.NewVBox(msg, note("Choose a DICOM .dcm or .dcs file to try again."))
	}
	return layout.NewSpacer()
}
