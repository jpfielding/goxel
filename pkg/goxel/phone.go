package goxel

import (
	"fmt"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Compact (phone-sized) layouts, after roxel's ui/phone.rs. A bottom tab bar
// holds four destinations in both orientations. The two views take the main
// area with their controls in a fixed-size, paged (never scrolled) sheet:
// under the view in portrait, beside it in landscape. Study and Findings are
// pages. Focus drops the bars for one action bar at the bottom.

const (
	tabRowHeight   = 49
	sheetBody      = 4*touchTarget + 3*8
	sheetHeight    = 8 + touchTarget + 8 + sheetBody + 8
	landscapePanel = 400
	orbitStep      = 15 * math.Pi / 180
	panStep        = 80
)

// homeIndicatorInset keeps bottom bars clear of a phone's home indicator.
func (l layoutClass) homeIndicatorInset() float32 {
	switch l {
	case layoutPhonePortrait:
		return 34
	case layoutPhoneLandscape:
		return 21
	}
	return 0
}

func (v *Viewer) phoneRoot() fyne.CanvasObject {
	if !v.loaded() {
		return border(v.appBar(), nil, nil, nil, v.emptyState())
	}
	if v.st.focusSlice || v.st.focusVolume {
		return v.phoneFocusView()
	}
	var body fyne.CanvasObject
	switch v.st.phoneTab {
	case tabStudy:
		body = v.phonePage(v.studyPage())
	case tabFindings:
		body = v.phonePage(v.findingsPage())
	case tabSlice:
		body = v.withSheet(v.phoneSliceArea(), v.sheetOpenFor(), v.sliceSheet)
	default:
		body = v.withSheet(v.volumeRenderer, v.sheetOpenFor(), v.volumeSheet)
	}
	return border(v.appBar(), v.tabBar(), nil, nil, body)
}

func (v *Viewer) sheetOpenFor() bool { return v.st.sheetOpen }

// findingsPage is the Findings page. In landscape, which is short, the controls
// stay put on the left while the list scrolls on the right.
func (v *Viewer) findingsPage() fyne.CanvasObject {
	rows := v.findingsList()
	if v.st.layout != layoutPhoneLandscape || rows == nil {
		return v.findingsSection()
	}
	return columns(24, v.findingsSummary(), rows)
}

// phonePage is a full page of reading content; pages hold no sliders, so they scroll.
func (v *Viewer) phonePage(content fyne.CanvasObject) fyne.CanvasObject {
	return panel(colPanel, 0, container.NewVScroll(inset(16, 16, content)))
}

func (v *Viewer) studyPage() fyne.CanvasObject {
	many := len(v.volumeKeys()) > 1
	if v.st.layout == layoutPhoneLandscape {
		right := []fyne.CanvasObject{v.detailsSection()}
		if many {
			right = append(right, rule(), v.volumesSection())
		}
		return columns(24, v.studySection(), vstack(8, right...))
	}
	items := []fyne.CanvasObject{v.studySection(), rule(), v.detailsSection()}
	if many {
		items = append(items, rule(), v.volumesSection())
	}
	items = append(items, rule(), v.metadataSection())
	return vstack(8, items...)
}

// phoneSliceArea is the 2D slice, edge to edge.
func (v *Viewer) phoneSliceArea() fyne.CanvasObject {
	return v.sliceWell()
}

// withSheet places a view with its controls sheet: below in portrait,
// beside it in landscape. The sheet's size is fixed, so paging never
// resizes the view.
func (v *Viewer) withSheet(view fyne.CanvasObject, open bool, sheet func() fyne.CanvasObject) fyne.CanvasObject {
	if !open {
		return view
	}
	s := panel(colPanel, 0, inset(16, 8, sheet()))
	if v.st.layout == layoutPhoneLandscape {
		return border(nil, nil, nil, container.New(sizedLayout{w: landscapePanel}, edged(s, edgeLeft)), view)
	}
	return border(nil, container.New(sizedLayout{h: sheetHeight}, edged(s, edgeTop)), nil, nil, view)
}

// tabBar: four tabs; the active one has an ACCENT top bar and label, view
// tabs carry a chevron for their sheet, Findings a count badge.
func (v *Viewer) tabBar() fyne.CanvasObject {
	tabs := make([]fyne.CanvasObject, 0, 4)
	for _, t := range []phoneTab{tabStudy, tabFindings, tabSlice, tabView3D} {
		tabs = append(tabs, v.tabButton(t))
	}
	row := container.New(sizedLayout{h: tabRowHeight}, inset(8, 0, columns(0, tabs...)))
	home := v.st.layout.homeIndicatorInset()
	return edged(panel(colBG, 0, vstack(0, row, container.New(sizedLayout{h: home}, vstack(0)))), edgeTop)
}

func (v *Viewer) tabButton(t phoneTab) fyne.CanvasObject {
	active := t == v.st.phoneTab
	c := &tabCell{label: t.label(), active: active, toggles: active && t.isView(), open: v.st.sheetOpen}
	if t == tabFindings {
		c.badge = v.findingCount()
	}
	c.onTap = func() {
		v.st.selectTab(t)
		v.rebuild()
		if t == tabView3D {
			v.volumeRenderer.FitView()
		}
	}
	c.ExtendBaseWidget(c)
	return c
}

// tabCell is one tab of the bottom bar. The active tab is raised on a PANEL
// tint with an accent bar; a view's tab carries a chevron saying whether its
// controls are open.
type tabCell struct {
	widget.BaseWidget
	label   string
	active  bool
	toggles bool
	open    bool
	badge   int
	onTap   func()
	pressed bool
	focused bool
}

func (c *tabCell) Tapped(*fyne.PointEvent) { c.onTap() }
func (c *tabCell) MouseDown(*desktop.MouseEvent) {
	c.pressed = true
	unfocus(c) // a tap is not keyboard focus
	c.Refresh()
}
func (c *tabCell) MouseUp(*desktop.MouseEvent) { c.pressed = false; c.Refresh() }
func (c *tabCell) Cursor() desktop.Cursor      { return desktop.PointerCursor }

func (c *tabCell) FocusGained()   { c.focused = true; c.Refresh() }
func (c *tabCell) FocusLost()     { c.focused = false; c.Refresh() }
func (c *tabCell) TypedRune(rune) {}
func (c *tabCell) TypedKey(e *fyne.KeyEvent) {
	if isActivate(e) {
		c.onTap()
		return
	}
	passKey(c, e)
}

func (c *tabCell) CreateRenderer() fyne.WidgetRenderer {
	r := &tabRenderer{c: c, bg: rrect(colPanel, color.Transparent, 8, 0), bar: rrect(colAccent, color.Transparent, 2, 0), ring: newRing(8),
		text: text(c.label, 14, colText), badgeDot: canvas.NewCircle(colDanger), count: text("", 11, colBG)}
	r.chev[0], r.chev[1] = line(colText, 1.6), line(colText, 1.6)
	return r
}

type tabRenderer struct {
	c        *tabCell
	bg, bar  *canvas.Rectangle
	ring     *canvas.Rectangle
	text     *canvas.Text
	chev     [2]*canvas.Line
	badgeDot *canvas.Circle
	count    *canvas.Text
}

func (r *tabRenderer) MinSize() fyne.Size { return fyne.NewSize(40, tabRowHeight) }

func (r *tabRenderer) Layout(size fyne.Size) {
	c := r.c
	r.bg.Move(fyne.NewPos(3, 2))
	r.bg.Resize(fyne.NewSize(size.Width-6, size.Height-6))
	r.ring.Move(fyne.NewPos(3-ringWidth, 2-ringWidth))
	r.ring.Resize(fyne.NewSize(size.Width-6+2*ringWidth, size.Height-6+2*ringWidth))
	r.bar.Move(fyne.NewPos(14, 2))
	r.bar.Resize(fyne.NewSize(size.Width-28, 3))
	ts := fyne.MeasureText(c.label, 14, fyne.TextStyle{})
	extra := float32(0)
	if c.toggles {
		extra = 13
	}
	left := size.Width/2 - (ts.Width+extra)/2
	r.text.Move(fyne.NewPos(left, size.Height/2-ts.Height/2))
	r.text.Resize(ts)
	if c.toggles {
		cx, cy := left+ts.Width+9, size.Height/2+1
		dy := float32(-2.5)
		if c.open {
			dy = 2.5
		}
		a, b, z := fyne.NewPos(cx-4, cy-dy), fyne.NewPos(cx, cy+dy), fyne.NewPos(cx+4, cy-dy)
		r.chev[0].Position1, r.chev[0].Position2 = a, b
		r.chev[1].Position1, r.chev[1].Position2 = b, z
	}
	if c.badge > 0 {
		cx, cy := size.Width-9, float32(13)
		r.badgeDot.Move(fyne.NewPos(cx-8, cy-8))
		r.badgeDot.Resize(fyne.NewSize(16, 16))
		r.count.Text = fmt.Sprintf("%d", c.badge)
		cs := fyne.MeasureText(r.count.Text, 11, fyne.TextStyle{})
		r.count.Move(fyne.NewPos(cx-cs.Width/2, cy-cs.Height/2))
		r.count.Resize(cs)
	}
}

func (r *tabRenderer) Refresh() {
	c := r.c
	fg := color.NRGBA(colText)
	if c.active {
		fg = colAccent
	}
	r.text.Color = fg
	for _, l := range r.chev {
		l.StrokeColor = fg
		if c.toggles {
			l.Show()
		} else {
			l.Hide()
		}
	}
	if c.active || c.pressed {
		r.bg.Show()
	} else {
		r.bg.Hide()
	}
	if c.active {
		r.bar.Show()
	} else {
		r.bar.Hide()
	}
	if c.badge > 0 {
		r.badgeDot.Show()
		r.count.Show()
	} else {
		r.badgeDot.Hide()
		r.count.Hide()
	}
	showWhen(r.ring, c.focused)
	r.Layout(c.Size())
	canvas.Refresh(r.bg)
}

func (r *tabRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.bar, r.text, r.chev[0], r.chev[1], r.badgeDot, r.count, r.ring}
}

func (r *tabRenderer) Destroy() {}

// pageSelector is the sheet's segmented page picker.
func pageSelector(labels []string, selected int, onSelect func(int)) fyne.CanvasObject {
	return newSegmented(labels, selected, onSelect)
}

// buttonRow is a row of equal-width touch buttons filling the row.
func buttonRow(buttons ...*button) fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(buttons))
	for i, b := range buttons {
		objs[i] = b.sized(0, controlHeight())
	}
	gap := float32(8)
	if uiTouch {
		gap = 10
	}
	return columns(gap, objs...)
}

// touchRow is a label cell then content, at touch height.
func touchRow(label string, content fyne.CanvasObject) fyne.CanvasObject {
	return minHeight(touchTarget, flexRow(10, 1, fixedWidth(78, rowLabel(label)), content))
}

// wrapCenterLayout centres a wrapping text vertically in the width it is given.
type wrapCenterLayout struct{}

func (wrapCenterLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Resize(fyne.NewSize(size.Width, size.Height))
		h := min(o.MinSize().Height, size.Height)
		o.Resize(fyne.NewSize(size.Width, h))
		o.Move(fyne.NewPos(0, (size.Height-h)/2))
	}
}

func (wrapCenterLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return fyne.NewSize(40, 0) }

// hint is wrapped muted help text, centred on its row.
func hint(s string) fyne.CanvasObject {
	return container.New(wrapCenterLayout{}, newWrapped(s, 13, colMuted))
}

// volumeSheet: Camera / Slice / Windowing / Display / Lighting, or the band
// editor while it is open.
func (v *Viewer) volumeSheet() fyne.CanvasObject {
	if v.st.bandsOpen && v.st.preset == paletteMaterials {
		return v.bandsPage()
	}
	v.st.bandsOpen = false
	sel := pageSelector(volumePageLabels, int(v.st.volumePage), func(i int) {
		v.st.volumePage = volumePage(i)
		v.rebuild()
	})
	var page fyne.CanvasObject
	switch v.st.volumePage {
	case pageCamera:
		page = v.cameraPage()
	case pageSlicePlane:
		page = v.slicePlaneSheetPage()
	case pageWindowing:
		rows := append([]fyne.CanvasObject{sectionTitle("3D windowing", "stored values")}, v.window3DRows()...)
		page = vstack(8, append(rows, v.opacityRow().obj)...)
	case pageDisplay:
		legend := v.paletteLegend()
		var legendRow fyne.CanvasObject = legend
		if v.st.preset == paletteMaterials {
			legendRow = flexRow(10, 0, legend, v.bandsButton().sized(132, touchTarget))
		}
		page = vstack(8, v.palettePicker(), minHeight(touchTarget, legendRow),
			touchRow("Quality", v.qualityPicker()), v.densityRow().obj)
	default:
		page = v.lightingRows()
	}
	return vstack(8, sel, page)
}

// slicePlaneSheetPage: the slice plane, the cutaway with its dependent option
// indented beneath it, and the slice position that moves both.
func (v *Viewer) slicePlaneSheetPage() fyne.CanvasObject {
	rows := []fyne.CanvasObject{v.slicePlanePage()}
	if v.cutawayAvailable() && v.dicox.MaxSlice() > 0 {
		rows = append(rows, v.sliceRow().obj)
	}
	return vstack(8, rows...)
}

// bandsPage is the compact material-band editor: the thresholds strip, then
// one band at a time so its opacity row fits without scrolling.
func (v *Viewer) bandsPage() fyne.CanvasObject {
	done := newButton("Done", func() {
		v.st.bandsOpen = false
		v.rebuild()
	}).sized(72, touchTarget)
	head := minHeight(touchTarget, hrow(10, done, pageTitle("Material bands")))
	bands := v.st.bands
	strip := NewMultiRangeSlider(0, float64(bands[len(bands)-1].Threshold), bands)
	strip.Compact = true
	strip.OnChanged = func([]int) { v.debounced("bands", 60*time.Millisecond, v.applyPalette) }
	var names []string
	var idx []int
	for i, b := range bands {
		if !b.IsTransparent {
			names = append(names, b.Name)
			idx = append(idx, i)
		}
	}
	v.st.selectedBand = max(0, min(v.st.selectedBand, len(names)-1))
	picker := newSegmented(names, v.st.selectedBand, func(i int) {
		v.st.selectedBand = i
		v.rebuild()
	})
	i := idx[v.st.selectedBand]
	return vstack(8, head, strip, picker, v.bandOpacityRow(i, "Opacity", strip.GetBandAlpha(i)).obj)
}

// cameraPage gives a tap for everything the gestures do.
func (v *Viewer) cameraPage() fyne.CanvasObject {
	vr := v.volumeRenderer
	var rows []fyne.CanvasObject
	if !v.st.focusVolume {
		rows = append(rows, buttonRow(
			newButton("Fit view", vr.FitView).withTip("Center and fit the whole volume"),
			newButton("Focus volume", v.toggleFocusVolume).withTip("Enlarge the 3D view to fill the workspace")))
	}
	rows = append(rows, touchRow("View along", buttonRow(
		newButton("X", func() { vr.LookAlong(0) }).withTip("Look along the X axis"),
		newButton("Y", func() { vr.LookAlong(1) }).withTip("Look along the Y axis"),
		newButton("Z", func() { vr.LookAlong(2) }).withTip("Look along the Z axis"))))
	mode := 0
	if v.st.dragPans {
		mode = 1
	}
	orbitPan := newSegmented([]string{"Orbit", "Pan"}, mode, func(i int) {
		v.st.dragPans = i == 1
		v.rebuild()
	})
	arrow := func(icon iconKind, dx, dy float64) *button {
		name := map[iconKind]string{iconLeft: "left", iconRight: "right", iconUp: "up", iconDown: "down"}[icon]
		verb := "Orbit"
		if v.st.dragPans {
			verb = "Pan"
		}
		return newIconButton(icon, "", func() {
			if v.st.dragPans {
				vr.PanPixels(dx*panStep, dy*panStep)
			} else {
				vr.Orbit(dx*orbitStep, -dy*orbitStep)
			}
		}).sized(touchTarget, touchTarget).withTip(verb + " " + name)
	}
	rows = append(rows, minHeight(touchTarget, flexRow(10, 0, orbitPan,
		arrow(iconLeft, -1, 0), arrow(iconRight, 1, 0), arrow(iconUp, 0, -1), arrow(iconDown, 0, 1))))
	tip := "Or drag to orbit, pinch to zoom, two fingers to pan."
	if v.st.dragPans {
		tip = "Or drag to pan and pinch to zoom."
	}
	zoomOut := newButton("−", func() { vr.ZoomBy(1.25) }).textSize(22).sized(touchTarget, touchTarget).withTip("Zoom out")
	zoomIn := newButton("+", func() { vr.ZoomBy(1 / 1.25) }).textSize(22).sized(touchTarget, touchTarget).withTip("Zoom in")
	rows = append(rows, minHeight(touchTarget, flexRow(10, 3, fixedWidth(78, rowLabel("Zoom")), zoomOut, zoomIn, hint(tip))))
	return vstack(8, rows...)
}

// sliceSheet: Slice / Image / Volume (Volume only when there is a choice).
func (v *Viewer) sliceSheet() fyne.CanvasObject {
	labels := slicePageLabels[:2]
	if len(v.sliceSourceKeys()) > 1 {
		labels = slicePageLabels
	}
	if int(v.st.slicePage) >= len(labels) {
		v.st.slicePage = pageSlice
	}
	sel := pageSelector(labels, int(v.st.slicePage), func(i int) {
		v.st.slicePage = slicePage(i)
		v.rebuild()
	})
	var page fyne.CanvasObject
	switch v.st.slicePage {
	case pageImage:
		rows := append([]fyne.CanvasObject{sectionTitle("2D windowing", "stored values")}, v.window2DRows()...)
		page = vstack(8, append(rows, v.zoomRow().obj)...)
	case pageVolume:
		picker := v.volumePicker()
		var pick fyne.CanvasObject = vstack(0)
		if picker != nil {
			pick = picker
		}
		page = vstack(8, minHeight(touchTarget, pick),
			hint("Changes the 2D slice only. Pick the 3D volume under Study."))
	default:
		rows := []fyne.CanvasObject{v.planePicker(), v.sliceRow().obj,
			minHeight(touchTarget, v.compositeToggle())}
		if !v.st.focusSlice {
			rows = append(rows, buttonRow(
				newButton("Fit image", v.fitImage).withTip("Show the whole slice"),
				newButton("Focus slice", v.toggleFocusSlice).withTip("Enlarge the 2D slice to fill the workspace")))
		}
		page = vstack(8, rows...)
	}
	return vstack(8, sel, page)
}

// phoneFocusView: the view fills the screen above one action bar; "Adjust
// display" opens the same paged controls the tab view uses.
func (v *Viewer) phoneFocusView() fyne.CanvasObject {
	slice := v.st.focusSlice
	exit := newButton("Exit focus", func() {
		v.st.focusSlice, v.st.focusVolume, v.st.focusAdjustOpen = false, false, false
		v.rebuild()
		v.volumeRenderer.FitView()
	})
	fitLabel, fit := "Fit view", v.volumeRenderer.FitView
	if slice {
		fitLabel, fit = "Fit image", v.fitImage
	}
	adjust := newButton("Adjust display", func() {
		v.st.focusAdjustOpen = !v.st.focusAdjustOpen
		v.rebuild()
	})
	adjust.selected = v.st.focusAdjustOpen
	bar := edged(panel(colBG, 0, vstack(0,
		inset(16, 8, buttonRow(exit, newButton(fitLabel, fit), adjust)),
		container.New(sizedLayout{h: v.st.layout.homeIndicatorInset()}, vstack(0)))), edgeTop)

	view, sheet := fyne.CanvasObject(v.volumeRenderer), v.volumeSheet
	if slice {
		view, sheet = v.phoneSliceArea(), v.sliceSheet
	}
	if v.st.focusAdjustOpen {
		view = v.withSheet(view, true, sheet)
	} else if slice && !v.st.composite && v.dicox.MaxSlice() > 0 {
		// Stepping through slices is the main thing to do here: keep it out.
		row := panel(colPanel, 0, inset(16, 8, v.sliceRow().obj))
		view = border(nil, edged(row, edgeTop), nil, nil, view)
	}
	return border(nil, bar, nil, nil, view)
}
