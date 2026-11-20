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
	return edged(bar, edgeBottom)
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
		return "Drag to pan  ·  Ctrl + scroll to zoom  ·  Esc to exit focus"
	}
	switch {
	case touch:
		return "Drag to orbit  ·  Pinch to zoom  ·  Two fingers to pan"
	case st.focusVolume:
		return "Drag to orbit  ·  Shift + drag to pan  ·  Scroll to zoom  ·  Esc to exit focus"
	default:
		return "Drag to orbit  ·  Shift + drag to pan  ·  Scroll to zoom  ·  Arrow keys step slices"
	}
}

func (v *Viewer) statusBar() fyne.CanvasObject {
	t := text(navigationText(&v.st, hasTouch()), 12, colMuted)
	return edged(panel(colBG, 0, inset(16, 5, shrinkable(leftCentre(t)))), edgeTop)
}

// emptyState is the first-use card: what this is and the one action. In
// landscape on a phone it is too short to stack, so the message is on the
// left and the actions on the right.
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

	// Gaps are roxel's: its row spacing (8) plus the space it adds by hand.
	message := vstack(0, container.NewCenter(markImage(markSize)), gap(20),
		container.NewCenter(title), gap(12), intro)
	actions := []fyne.CanvasObject{v.openButtons(true), gap(12)}
	if v.st.loadingFile != "" || v.st.loadError != "" {
		actions = append(actions, v.loadStatus(), gap(8))
	}
	actions = append(actions, fmtNote)
	acts := vstack(0, actions...)

	if v.st.layout == layoutPhoneLandscape {
		return panel(colBG, 24, container.NewVScroll(container.New(&splitEmpty{gap: 10}, message, acts)))
	}
	width := float32(280)
	if compact {
		width = 340
	}
	top := float32(0.26)
	if compact {
		top = 0.2
	}
	card := vstack(0, message, gap(28), acts)
	return panel(colBG, 24, container.NewVScroll(container.New(&centredColumn{width: width, top: top}, card)))
}

// gap is empty vertical space of height h in a vstack(0).
func gap(h float32) fyne.CanvasObject { return minHeight(h, vstack(0)) }

// splitEmpty lays out the landscape empty state: two equal columns, the
// message left and the actions right (at most 320 wide, a little lower).
type splitEmpty struct{ gap float32 }

func (l *splitEmpty) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	message, actions := objs[0], objs[1]
	col := (size.Width - l.gap) / 2
	mh, ah := message.MinSize().Height, actions.MinSize().Height
	top := max((size.Height-190)/2, 4)
	message.Resize(fyne.NewSize(col, mh))
	message.Move(fyne.NewPos(0, top))
	aw := min(col, 320)
	actions.Resize(fyne.NewSize(aw, ah))
	actions.Move(fyne.NewPos(col+l.gap+(col-aw)/2, top+30))
}

func (l *splitEmpty) MinSize(objs []fyne.CanvasObject) fyne.Size {
	h := max(objs[0].MinSize().Height, objs[1].MinSize().Height+30)
	return fyne.NewSize(240, h+4)
}

// centredColumn centres one child horizontally at a fixed width and places
// it a fraction of the area's height down (at least 16 from the top).
type centredColumn struct {
	width, top float32
}

func (c *centredColumn) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		w := min(c.width, size.Width)
		h := o.MinSize().Height
		o.Resize(fyne.NewSize(w, h))
		o.Move(fyne.NewPos((size.Width-w)/2, max(size.Height*c.top, 16)))
	}
}

func (c *centredColumn) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(min(c.width, 240), h+16)
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

// loadStatus: a spinner while loading, or the last error with a retry hint;
// nil when there is neither.
func (v *Viewer) loadStatus() fyne.CanvasObject {
	switch {
	case v.st.loadingFile != "":
		bar := widget.NewProgressBarInfinite()
		return vstack(4, note("Loading "+elideMiddle(v.st.loadingFile, 40)+"…"), bar)
	case v.st.loadError != "":
		return vstack(4, newWrapped(v.st.loadError, bodySize(), colDanger), note("Choose a DICOM .dcm or .dcs file to try again."))
	}
	return nil
}
