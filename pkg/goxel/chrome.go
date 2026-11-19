package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// appBar: mark, name, and the open study's file name (middle-elided) on the right.
func (v *Viewer) appBar() fyne.CanvasObject {
	compact := v.st.layout.compact()
	markSize, nameSize := float32(24), float32(21)
	if compact {
		markSize, nameSize = 22, 19
	}
	left := hrow(8, markImage(markSize), text("goxel", nameSize, colHeading))
	var right fyne.CanvasObject
	if v.st.loadedName != "" && v.loaded() {
		maxRunes := 64
		if compact {
			maxRunes = 28
		}
		right = rightCentre(text(elideMiddle(v.st.loadedName, maxRunes), 13, colMuted))
	}
	vpad := float32(8)
	if compact {
		vpad = 6
	}
	bar := panel(colPanel, 0, inset(16, vpad, minHeight(28, border(nil, nil, left, nil, shrinkable(orSpacer(right))))))
	return border(nil, hairline(), nil, nil, bar)
}

// orSpacer returns o, or an empty object when o is nil.
func orSpacer(o fyne.CanvasObject) fyne.CanvasObject {
	if o == nil {
		return layout.NewSpacer()
	}
	return o
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
	t := text(navigationText(&v.st, hasTouch()), 12, colMuted)
	return border(hairline(), nil, nil, nil, panel(colBG, 0, inset(16, 5, shrinkable(leftCentre(t)))))
}

// emptyState is the first-use card: what this is and the one action.
func (v *Viewer) emptyState() fyne.CanvasObject {
	compact := v.st.layout.compact()
	markSize := float32(64)
	if compact {
		markSize = 56
	}
	title := text("Open a DICOM study", 22, colHeading)
	title.Alignment = fyne.TextAlignCenter
	intro := note("See a CT scan as a 3D volume and as 2D slices.")
	intro.centre = true

	formats := "DICOM .dcm or .dcs files, or a study folder."
	switch {
	case !canOpenFolder:
		formats = "DICOM .dcm or .dcs files. Try the sample to see a CT study."
	case compact:
		formats = "DICOM .dcm or .dcs files."
	}
	fmtNote := note(formats)
	fmtNote.centre = true
	fmtNote.size = 12

	card := vstack(8,
		container.NewCenter(markImage(markSize)),
		container.NewCenter(title),
		intro,
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

// openButtons: "Open file…" (primary on first use) and "Open folder…" on
// desktop or "Load sample" in the browser, each as wide as the column.
func (v *Viewer) openButtons(primary bool) fyne.CanvasObject {
	busy := v.st.loadingFile != ""
	open := newButton("Open file…", v.openFile)
	open.primary = primary
	var second *button
	if canOpenFolder {
		second = newButton("Open folder…", v.openFolder)
	} else {
		second = newButton("Load sample", v.LoadSample)
	}
	if busy {
		open.Disable()
		second.Disable()
	}
	h := float32(30)
	if primary {
		h = touchTarget
	} else if v.st.layout.compact() {
		h = touchTarget
	}
	open.sized(0, h)
	second.sized(0, h)
	return vstack(8, open, second)
}

// loadStatus: a spinner while loading, or the last error with a retry hint.
func (v *Viewer) loadStatus() fyne.CanvasObject {
	switch {
	case v.st.loadingFile != "":
		bar := widget.NewProgressBarInfinite()
		return vstack(4, note("Loading "+elideMiddle(v.st.loadingFile, 40)+"…"), bar)
	case v.st.loadError != "":
		return vstack(4, newWrapped(v.st.loadError, bodySize(), colDanger), note("Choose a DICOM .dcm or .dcs file to try again."))
	}
	return vstack(0)
}
