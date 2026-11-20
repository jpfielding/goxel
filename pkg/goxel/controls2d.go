package goxel

import (
	"fmt"
	"image/color"
	"log/slog"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// wellFooterHeight is the strip under the image, inside the well, for the
// scale bar and caption. The scan is never drawn under it.
const wellFooterHeight = 30

// slicePanelLayout is the desktop right column's layout: header, the well
// sized to the tallest slice that fits, then the control groups right under it.
type slicePanelLayout struct {
	tallest   func(width float32) float32 // tallest fitted slice at this width
	hasPicker bool                        // the Image group has a Volume row
}

// desktopControlsHeight is the room roxel keeps for the three groups under the
// well (30 more when the Image group has a Volume row): the well gets the rest.
const desktopControlsHeight = 326

const (
	panelHeaderGap = 8
	panelGroupGap  = 10
)

func (l slicePanelLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	header, well, controls := objs[0], objs[1], objs[2]
	hh := header.MinSize().Height
	ch := controls.MinSize().Height
	header.Move(fyne.NewPos(0, 0))
	header.Resize(fyne.NewSize(size.Width, hh))
	reserve := float32(desktopControlsHeight)
	if l.hasPicker {
		reserve += 30
	}
	room := size.Height - hh - panelHeaderGap - reserve
	tall := l.tallest(size.Width) + wellFooterHeight + 2
	wh := max(min(room, tall), 140)
	well.Move(fyne.NewPos(0, hh+panelHeaderGap))
	well.Resize(fyne.NewSize(size.Width, wh))
	y := hh + panelHeaderGap + wh + panelGroupGap
	controls.Move(fyne.NewPos(0, y))
	controls.Resize(fyne.NewSize(size.Width, ch))
}

func (l slicePanelLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(objs[2].MinSize().Width,
		objs[0].MinSize().Height+panelHeaderGap+140+panelGroupGap+objs[2].MinSize().Height)
}

// slicePanel is the desktop right column: header, image well with caption,
// then the Slice, 2D windowing and Image groups.
func (v *Viewer) slicePanel() fyne.CanvasObject {
	header := v.sliceHeader("Focus slice")
	controls := vstack(panelGroupGap, v.sliceGroup(), v.windowingGroup2D(), v.imageGroup())
	body := container.New(slicePanelLayout{tallest: v.tallestFit, hasPicker: len(v.sliceSourceKeys()) > 1}, header, v.sliceWell(), controls)
	return edged(panel(colPanel, 0, inset(16, 16, body)), edgeLeft)
}

// sliceHeader mirrors the 3D view's header: title and the two image actions.
func (v *Viewer) sliceHeader(focusLabel string) fyne.CanvasObject {
	focus := focusButton(focusLabel, v.st.focusSlice, "2D slice", v.toggleFocusSlice)
	return minHeight(28, border(nil, nil, pageTitle("2D slice"),
		hrow(8, newButton("Fit image", v.fitImage).withTip("Show the whole slice"), focus), nil))
}

// sliceFocusView: the slice fills the workspace with its groups docked below.
func (v *Viewer) sliceFocusView() fyne.CanvasObject {
	label := "Exit focus"
	header := panel(colPanel, 0, inset(16, 10, v.sliceHeader(label)))
	// On very wide windows the groups keep a usable length, as the 3D shelf's do.
	dock := container.New(&cappedCentre{max: shelfMaxWidth}, columns(24, v.sliceGroup(), v.windowingGroup2D(), v.imageGroup()))
	groups := panel(colPanel, 0, inset(16, 12, dock))
	well := panel(colPanel, 0, inset(16, 0, v.sliceWell()))
	return border(edged(header, edgeBottom), edged(groups, edgeTop), nil, nil, well)
}

func (v *Viewer) toggleFocusSlice() {
	v.st.focusSlice = !v.st.focusSlice
	v.st.focusAdjustOpen = false
	v.rebuild()
	v.volumeRenderer.FitView()
}

func (v *Viewer) fitImage() {
	v.st.zoom2D = 100
	v.interactiveView.SetZoom(100)
	v.sync()
}

// zoomChanged follows a zoom made by a gesture: the Zoom slider moves with it.
func (v *Viewer) zoomChanged(percent float64) {
	v.st.zoom2D = percent
	v.sync()
}

// sliceSizeMM is the slice's physical size for the current orientation, in
// millimetres: the image is drawn in these proportions, not in voxels.
func (v *Viewer) sliceSizeMM() (w, h float64) {
	if cv := v.compositeVolumes[v.currentVolumeKey]; cv != nil && cv.PixelData != nil {
		return v.planeExtentMM(cv)
	}
	if b := v.dicox.Bounds(); b.Dx() > 0 && b.Dy() > 0 {
		return float64(b.Dx()), float64(b.Dy())
	}
	return 1, 1
}

// tallestFit is the tallest a fitted slice can be at this width over the
// three planes, so switching planes never moves the controls.
func (v *Viewer) tallestFit(width float32) float32 {
	cv := v.compositeVolumes[v.currentVolumeKey]
	if cv == nil || cv.PixelData == nil {
		return width * 0.6
	}
	vx, vy, vz := spacingOr1(cv.VoxelSizeX), spacingOr1(cv.VoxelSizeY), spacingOr1(cv.VoxelSizeZ)
	x, y, z := float64(cv.Cols)*vx, float64(cv.Rows)*vy, float64(len(cv.PixelData.Frames))*vz
	var tall float64
	for _, p := range [][2]float64{{x, y}, {x, z}, {y, z}} {
		tall = math.Max(tall, float64(width)/math.Max(p[0], 1)*p[1])
	}
	return float32(tall)
}

// sliceWell: the black image well with its scale bar and caption footer.
func (v *Viewer) sliceWell() fyne.CanvasObject {
	clip := container.NewScroll(v.interactiveView)
	clip.Direction = container.ScrollNone
	footer := newWellFooter(v.interactiveView.pixelsPerMM, v.sliceCaption)
	v.interactiveView.OnLayout = footer.Refresh
	v.onSync(footer.Refresh)
	return container.NewStack(rrect(colWell, color.Transparent, 6, 0), border(nil, footer, nil, nil, clip))
}

// planePicker: Axial / Coronal / Sagittal.
func (v *Viewer) planePicker() *segmented {
	s := newSegmented(orientationLabels, int(v.st.orientation), func(i int) {
		v.setOrientation(ViewOrientation(i))
	})
	v.onSync(func() { s.Select(int(v.st.orientation)) })
	return s
}

// sliceRow is the slice slider; its range follows the orientation.
func (v *Viewer) sliceRow() *sliderRow {
	spec := func() sliderSpec {
		n := v.dicox.MaxSlice() + 1
		s := whole("Slice", 0, float64(max(n-1, 1)), 1)
		s.offset, s.total = 1, n
		return s
	}
	r := newSliderRow(spec(), float64(v.st.sliceIndex),
		func(x float64) { v.setSlice(int(x)) }, nil)
	sync := func() {
		s := spec()
		r.SetRange(s.lo, s.hi, s.total)
		r.SetValue(float64(v.st.sliceIndex))
		r.SetEnabled(!v.st.composite && v.dicox.MaxSlice() > 0)
	}
	sync()
	v.onSync(sync)
	return r
}

// pager is "Previous" / "Next" under the slice track, for exact single-slice steps.
func (v *Viewer) pager() fyne.CanvasObject {
	prev := newButton("Previous", func() { v.stepSlice(-1) }).withTip("Previous slice")
	next := newButton("Next", func() { v.stepSlice(1) }).withTip("Next slice")
	sync := func() {
		scrub := !v.st.composite && v.dicox.MaxSlice() > 0
		if scrub && v.st.sliceIndex > 0 {
			prev.Enable()
		} else {
			prev.Disable()
		}
		if scrub && v.st.sliceIndex < v.dicox.MaxSlice() {
			next.Enable()
		} else {
			next.Disable()
		}
	}
	sync()
	v.onSync(sync)
	// Start where the slice rail starts (the track is inset by its thumb).
	return inset(64+7, 0, hrow(8, prev, next))
}

// sliceGroup: plane picker, slider and Previous / Next.
func (v *Viewer) sliceGroup() fyne.CanvasObject {
	return vstack(6, sectionTitle("Slice", ""), natural(v.planePicker()), v.sliceRow().obj, v.pager())
}

// windowingGroup2D: centre and width in stored values. The 2D view is
// re-rasterised on the CPU, so the values commit when a drag ends.
func (v *Viewer) windowingGroup2D() fyne.CanvasObject {
	rows := append([]fyne.CanvasObject{sectionTitle("2D windowing", "stored values")}, v.window2DRows()...)
	return vstack(6, rows...)
}

// window2DRows are the centre and width rows of the 2D windowing.
func (v *Viewer) window2DRows() []fyne.CanvasObject {
	return v.windowRows(&v.st.windowCenter2D, &v.st.windowWidth2D, v.st.range2D, func(committed bool) {
		if committed {
			v.commitWindow2D()
		}
	})
}

// commitWindow2D applies the 2D window to the image.
func (v *Viewer) commitWindow2D() {
	v.dicox.SetWindowLevel(int(v.st.windowCenter2D))
	v.dicox.SetWindowWidth(int(v.st.windowWidth2D))
	v.refresh2D()
}

// zoomRow: 50-500 %.
func (v *Viewer) zoomRow() *sliderRow {
	r := newSliderRow(whole("Zoom", 50, 500, 25).withUnit("%"), v.st.zoom2D,
		func(x float64) {
			v.st.zoom2D = x
			v.interactiveView.SetZoom(x)
		}, nil)
	v.onSync(func() { r.SetValue(v.st.zoom2D) })
	return r
}

// volumePicker chooses what the 2D view shows (only when there is a choice).
func (v *Viewer) volumePicker() fyne.CanvasObject {
	keys := v.sliceSourceKeys()
	if len(keys) < 2 {
		return nil
	}
	sel := widget.NewSelect(keys, func(s string) {
		if s != v.currentVolumeKey {
			v.set2DSource(s, false)
			v.rebuild() // the windowing rows follow the source
		}
	})
	sel.Selected = v.currentVolumeKey
	return flexRow(8, 1, fixedWidth(64, rowLabel("Volume")), sel)
}

func (v *Viewer) compositeToggle() *checkbox {
	return newCheck("Composite projection", v.st.composite, func(b bool) {
		v.setComposite(b)
		v.sync()
	}).withTip("Blend every slice along this plane into one image")
}

// addFindingButton draws a rectangle on the slice to record a finding. It is a
// toggle: lit while the next drag will draw. It is not in the interface until
// findings can be stored; the annotation code stays for when they can.
func (v *Viewer) addFindingButton() *button { //nolint:unused
	b := newButton("Add Finding", nil)
	b.onTap = func() {
		v.isAnnotating = !v.isAnnotating
		b.SetSelected(v.isAnnotating)
		slog.Info("Annotation Mode Toggled", "active", v.isAnnotating)
	}
	b.SetSelected(v.isAnnotating)
	v.onSync(func() { b.SetSelected(v.isAnnotating) })
	return b
}

// imageGroup: zoom, volume and composite projection.
func (v *Viewer) imageGroup() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Image", ""), v.zoomRow().obj}
	if p := v.volumePicker(); p != nil {
		items = append(items, p)
	}
	return vstack(6, append(items, v.compositeToggle())...)
}

// --- Footer of the image well ------------------------------------------------------

// wellFooter is the strip under the slice: a scale bar on the left, the
// caption on the right.
type wellFooter struct {
	widget.BaseWidget
	scale   func() float32 // pixels per millimetre of the image as drawn
	caption func() string
}

func newWellFooter(scale func() float32, caption func() string) *wellFooter {
	f := &wellFooter{scale: scale, caption: caption}
	f.ExtendBaseWidget(f)
	return f
}

func (f *wellFooter) CreateRenderer() fyne.WidgetRenderer {
	r := &wellFooterRenderer{f: f, rule: filled(color.NRGBA{48, 48, 48, 255}),
		bar: line(color.White, 1.5), left: line(color.White, 1.5), right: line(color.White, 1.5),
		length: text("", 12, color.White), caption: text("", 12, colMuted)}
	return r
}

type wellFooterRenderer struct {
	f                *wellFooter
	rule             *canvas.Rectangle
	bar, left, right *canvas.Line
	length, caption  *canvas.Text
}

func (r *wellFooterRenderer) MinSize() fyne.Size { return fyne.NewSize(1, wellFooterHeight) }

// scaleBarLength is a round length (1, 2 or 5 x 10^n mm) whose bar is at most
// maxPx wide: it returns the length in mm and the bar's width in pixels.
func scaleBarLength(pixelsPerMM, maxPx float32) (mm, width float32) {
	target := maxPx / pixelsPerMM
	base := float32(math.Pow(10, math.Floor(math.Log10(float64(target)))))
	mm = base
	for _, n := range []float32{5, 2, 1} {
		if n*base <= target {
			mm = n * base
			break
		}
	}
	return mm, mm * pixelsPerMM
}

func (r *wellFooterRenderer) Layout(size fyne.Size) {
	r.rule.Move(fyne.NewPos(0, 0))
	r.rule.Resize(fyne.NewSize(size.Width, 1))
	y := size.Height / 2
	x := float32(12)
	r.bar.Hide()
	r.left.Hide()
	r.right.Hide()
	r.length.Text = ""
	if s := r.f.scale(); s > 0 && !math.IsInf(float64(s), 0) && !math.IsNaN(float64(s)) {
		mm, w := scaleBarLength(s, 72)
		r.bar.Position1, r.bar.Position2 = fyne.NewPos(x, y), fyne.NewPos(x+w, y)
		r.left.Position1, r.left.Position2 = fyne.NewPos(x, y-4), fyne.NewPos(x, y+4)
		r.right.Position1, r.right.Position2 = fyne.NewPos(x+w, y-4), fyne.NewPos(x+w, y+4)
		r.bar.Show()
		r.left.Show()
		r.right.Show()
		r.length.Text = fmt.Sprintf("%g mm", mm)
		ts := measure(r.length.Text, 12)
		r.length.Move(fyne.NewPos(x+w+8, y-ts.Height/2))
		r.length.Resize(ts)
		x += w + 8 + ts.Width + 16
	}
	// The longest caption that fits beside the scale bar.
	room := size.Width - 12 - x
	r.caption.Text = ""
	full := r.f.caption()
	for _, c := range []string{full, captionPosition(full)} {
		if textWidth(c, 12) <= room {
			r.caption.Text = c
			break
		}
	}
	ts := measure(r.caption.Text, 12)
	r.caption.Move(fyne.NewPos(size.Width-12-ts.Width, y-ts.Height/2))
	r.caption.Resize(ts)
}

func (r *wellFooterRenderer) Refresh() {
	r.Layout(r.f.Size())
	canvas.Refresh(r.rule)
}

func (r *wellFooterRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rule, r.bar, r.left, r.right, r.length, r.caption}
}

func (r *wellFooterRenderer) Destroy() {}
