package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// appBar: mark, name, and the open study's file name on the right, cut in the
// middle to the room there is; its full path shows on hover.
func (v *Viewer) appBar() fyne.CanvasObject {
	compact := v.st.layout.compact()
	markSize, nameSize := float32(24), float32(21)
	if compact {
		markSize, nameSize = 22, 19
	}
	left := hrow(8, markImage(markSize), text("goxel", nameSize, colHeading))
	var right fyne.CanvasObject
	if v.st.loadedName != "" && v.loaded() {
		right = newFileLabel(v.st.loadedName, v.st.loadedPath)
	}
	vpad := float32(8)
	if compact {
		vpad = 6
	}
	bar := panel(colPanel, 0, inset(16, vpad, minHeight(28, border(nil, nil, left, nil, shrinkable(orSpacer(right))))))
	return edged(bar, edgeBottom)
}

// fileLabel is the open study's name at the right of the app bar: muted, cut
// in the middle to fit the room it is given (as roxel cuts it: both ends of a
// name tell two studies apart), with the full path as its tooltip.
type fileLabel struct {
	widget.BaseWidget
	name, path string
	at         fyne.Position // where the text is drawn, for the pointer
	extent     fyne.Size
	over       bool
}

// The file name's type size, the gap roxel leaves before it and the limits of
// the room it may take.
const (
	fileLabelSize    = 13
	fileLabelGap     = 8
	fileLabelMargin  = 16
	fileLabelMinRoom = 40
	fileLabelMaxRoom = 520
)

func newFileLabel(name, path string) *fileLabel {
	if path == "" {
		path = name
	}
	l := &fileLabel{name: name, path: path}
	l.ExtendBaseWidget(l)
	return l
}

func (l *fileLabel) MouseIn(e *desktop.MouseEvent) { l.MouseMoved(e) }
func (l *fileLabel) MouseOut()                     { l.setOver(false) }

// MouseMoved: the tip belongs to the name, not to the empty bar beside it.
func (l *fileLabel) MouseMoved(e *desktop.MouseEvent) {
	l.setOver(e.Position.X >= l.at.X && e.Position.X <= l.at.X+l.extent.Width)
}

func (l *fileLabel) setOver(over bool) {
	if over == l.over {
		return
	}
	l.over = over
	if over {
		tooltips.hoverRect(l, l.at, l.extent, l.path)
	} else {
		tooltips.hide(l)
	}
}

func (l *fileLabel) CreateRenderer() fyne.WidgetRenderer {
	return &fileLabelRenderer{l: l, text: text("", fileLabelSize, colMuted)}
}

type fileLabelRenderer struct {
	l    *fileLabel
	text *canvas.Text
}

func (r *fileLabelRenderer) MinSize() fyne.Size {
	return fyne.NewSize(1, textHeight(fileLabelSize))
}

func (r *fileLabelRenderer) Layout(size fyne.Size) {
	room := min(max(size.Width-fileLabelGap-fileLabelMargin, fileLabelMinRoom), fileLabelMaxRoom)
	r.text.Text = elideToWidth(r.l.name, fileLabelSize, room)
	ts := measure(r.text.Text, fileLabelSize)
	r.l.at, r.l.extent = fyne.NewPos(size.Width-ts.Width, (size.Height-ts.Height)/2), ts
	r.text.Move(r.l.at)
	r.text.Resize(ts)
}

func (r *fileLabelRenderer) Refresh() {
	r.Layout(r.l.Size())
	canvas.Refresh(r.text)
}

func (r *fileLabelRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.text} }
func (r *fileLabelRenderer) Destroy()                     {}

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

// loadStatus: roxel's spinner beside "Loading <name>…" while loading, or the
// last error with a retry hint; nil when there is neither.
func (v *Viewer) loadStatus() fyne.CanvasObject {
	switch {
	case v.st.loadingFile != "":
		loading := func() bool { return v.st.loadingFile != "" }
		gap := float32(8)
		if uiTouch {
			gap = 10
		}
		// The spinner, then the whole name, wrapped, centred on it.
		return container.New(keyValueLayout{col: controlHeight(), gap: gap, h: controlHeight()},
			newSpinner(loading), bodyNote("Loading "+v.st.loadingFile+"…"))
	case v.st.loadError != "":
		return vstack(4, newWrapped(v.st.loadError, bodySize(), colDanger), note("Choose a DICOM .dcm or .dcs file to try again."))
	}
	return nil
}
