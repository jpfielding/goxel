package goxel

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

func f0(x float64) string { return fmt.Sprintf("%.0f", x) }
func f2(x float64) string { return fmt.Sprintf("%.2f", x) }

// focusButton is "Focus volume" / "Exit focus" with roxel's bracket icon; what
// names the view it enlarges in the tooltip.
func focusButton(label string, focused bool, what string, onTap func()) *button {
	icon, tip := iconEnlarge, "Enlarge the "+what+" to fill the workspace"
	if focused {
		icon, tip = iconReduce, "Back to the workspace (Esc)"
	}
	return newIconButton(icon, label, onTap).withTip(tip)
}

// volumeHeader is the 3D column's two header rows: title and camera actions,
// then the slice-plane tools (with the plane picker and slice slider in focus).
func (v *Viewer) volumeHeader() fyne.CanvasObject {
	focusLabel := "Focus volume"
	if v.st.focusVolume {
		focusLabel = "Exit focus"
	}
	actions := hrow(16,
		hrow(8, rowLabel("View along"), v.viewAlongButtons()),
		hrow(8, newButton("Fit view", v.volumeRenderer.FitView).withTip("Center and fit the whole volume"),
			focusButton(focusLabel, v.st.focusVolume, "3D view", v.toggleFocusVolume)))
	row1 := minHeight(28, border(nil, nil, pageTitle("3D view"), actions, nil))
	row2 := minHeight(32, v.slicePlaneTools())
	return edged(panel(colPanel, 0, inset(16, 10, vstack(8, row1, row2))), edgeBottom)
}

func (v *Viewer) viewAlongButtons() fyne.CanvasObject {
	axis := func(label string, a int) fyne.CanvasObject {
		return newButton(label, func() { v.volumeRenderer.LookAlong(a) }).sized(30, 0).
			withTip("Look along the " + label + " axis")
	}
	return hrow(4, axis("X", 0), axis("Y", 1), axis("Z", 2))
}

func (v *Viewer) toggleFocusVolume() {
	v.st.focusVolume = !v.st.focusVolume
	v.st.focusAdjustOpen = false
	v.rebuild()
	v.volumeRenderer.FitView()
}

// slicePlaneChecks builds the slice-plane outline toggle and the cutaway
// pair, and keeps them enabled only when there is a slice to follow.
func (v *Viewer) slicePlaneChecks() (plane, cut, reverse *checkbox, unavailable *canvas.Text) {
	plane = newCheck("Slice plane", v.st.showSlicePlane, func(b bool) {
		v.st.showSlicePlane = b
		v.volumeRenderer.Refresh()
	}).withTip("Outline the 2D slice in the 3D view")
	reverse = newCheck("Reverse cutaway", v.st.reverseCutaway, func(b bool) {
		v.st.reverseCutaway = b
		v.applyCutaway()
	}).withTip("Hide the other side instead")
	cut = newCheck("Cutaway", v.st.cutaway, func(b bool) {
		v.st.cutaway = b
		if b {
			reverse.Enable()
		} else {
			reverse.Disable()
		}
		v.applyCutaway()
	}).withTip("Hide the volume on one side of the slice plane")
	unavailable = text("Needs a single 2D slice of this volume", 12, colMuted)
	sync := func() {
		if v.cutawayAvailable() {
			plane.Enable()
			cut.Enable()
			if v.st.cutaway {
				reverse.Enable()
			} else {
				reverse.Disable()
			}
			unavailable.Hide()
		} else {
			plane.Disable()
			cut.Disable()
			reverse.Disable()
			unavailable.Show()
		}
	}
	sync()
	v.onSync(sync)
	return plane, cut, reverse, unavailable
}

// cutawayGap is the space between "Cutaway" and its dependent "Reverse
// cutaway": roxel's 8 px item spacing plus the box's 6 px of padding.
const cutawayGap = 14

// slicePlaneTools (desktop header): "Slice plane", then "Cutaway" with its
// dependent "Reverse cutaway" enclosed with it in one frame. In focus the
// plane picker and the slice slider (at most 340 wide) join them, because the
// 2D panel that normally holds them is hidden.
func (v *Viewer) slicePlaneTools() fyne.CanvasObject {
	plane, cut, reverse, unavailable := v.slicePlaneChecks()
	items := []fyne.CanvasObject{plane, frame(colHairline, 6, 10, 2, hrow(cutawayGap, cut, reverse)), unavailable}
	if !v.st.focusVolume || !v.cutawayAvailable() {
		return hrow(14, items...)
	}
	items = append(items[:2], v.planePicker())
	left := hrow(14, items...)
	if v.dicox.MaxSlice() < 1 {
		return left
	}
	return container.New(cappedLayout{gap: 14, max: 340}, left, v.sliceRow().obj)
}

// cappedLayout places [left, last]: left at its own width, then last, at most
// max wide, a gap after it.
type cappedLayout struct{ gap, max float32 }

func (l cappedLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	left, last := objs[0], objs[1]
	lw := left.MinSize().Width
	left.Move(fyne.NewPos(0, (size.Height-left.MinSize().Height)/2))
	left.Resize(fyne.NewSize(lw, left.MinSize().Height))
	w := min(l.max, max(size.Width-lw-l.gap, 0))
	h := last.MinSize().Height
	last.Move(fyne.NewPos(lw+l.gap, (size.Height-h)/2))
	last.Resize(fyne.NewSize(w, h))
}

func (l cappedLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	left, last := objs[0].MinSize(), objs[1].MinSize()
	return fyne.NewSize(left.Width+l.gap+last.Width, max(left.Height, last.Height))
}

// slicePlanePage (compact): the three options one to a touch row, the
// dependent one indented beneath its parent.
func (v *Viewer) slicePlanePage() fyne.CanvasObject {
	plane, cut, reverse, unavailable := v.slicePlaneChecks()
	return vstack(8, minHeight(touchTarget, plane), minHeight(touchTarget, cut),
		minHeight(touchTarget, inset(34, 0, reverse)), unavailable)
}

// shelf is the columns under the 3D view (two when narrow, with lighting
// behind a menu button).
func (v *Viewer) shelf() fyne.CanvasObject {
	cols := []fyne.CanvasObject{v.windowingGroup3D(), v.paletteGroup()}
	if !v.narrowShelf {
		cols = append(cols, vstack(6, sectionTitle("Lighting", ""), v.lightingRows()))
	}
	// On very wide windows the rows keep a usable length.
	body := container.New(&cappedCentre{max: shelfMaxWidth}, columns(24, cols...))
	return edged(panel(colPanel, 0, inset(16, 12, body)), edgeTop)
}

// shelfMaxWidth is the widest the shelf's columns grow.
const shelfMaxWidth = 1440

// cappedCentre centres its child at most max wide.
type cappedCentre struct{ max float32 }

func (l *cappedCentre) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	w := min(size.Width, l.max)
	for _, o := range objs {
		o.Move(fyne.NewPos((size.Width-w)/2, 0))
		o.Resize(fyne.NewSize(w, size.Height))
	}
}

func (l *cappedCentre) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	return m
}

// window3DRanges returns what the 3D windowing rows' tracks cover and accept.
func (v *Viewer) windowRows(center, width *float64, r windowRange, apply func(committed bool)) []fyne.CanvasObject {
	c := newSliderRow(whole("Center", r.centerLo, r.centerHi, r.step).limit(-65535, 65535), *center, func(x float64) {
		*center = x
		apply(false)
	}, func(float64) { apply(true) })
	w := newSliderRow(whole("Width", r.widthLo, r.widthHi, r.step).limit(1, 65536), *width, func(x float64) {
		*width = x
		apply(false)
	}, func(float64) { apply(true) })
	return []fyne.CanvasObject{c.obj, w.obj}
}

// windowingGroup3D: centre/width, opacity and density.
func (v *Viewer) windowingGroup3D() fyne.CanvasObject {
	rows := append([]fyne.CanvasObject{sectionTitle("3D windowing", "stored values")},
		v.window3DRows()...)
	rows = append(rows, v.opacityRow().obj, v.densityRow().obj)
	return vstack(6, rows...)
}

// window3DRows are the centre and width rows of the 3D windowing.
func (v *Viewer) window3DRows() []fyne.CanvasObject {
	return v.windowRows(&v.st.windowCenter3D, &v.st.windowWidth3D, v.st.range3D, func(bool) {
		v.apply3DWindow()
	})
}

func (v *Viewer) opacityRow() *sliderRow {
	return newSliderRow(fraction("Opacity", 0, 1), v.st.opacity, func(x float64) {
		v.st.opacity = x
		v.applyOpacity()
	}, nil)
}

func (v *Viewer) densityRow() *sliderRow {
	return newSliderRow(fraction("Density", 0, 1), v.st.density, func(x float64) {
		v.st.density = x
		v.applyDensity()
	}, nil)
}

// paletteGroup: Materials / Threat / Grayscale, legend, quality and the
// buttons that open the band editor and, on a narrow shelf, the lighting rows.
func (v *Viewer) paletteGroup() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Palette", ""), natural(v.palettePicker()), v.paletteLegend(),
		hrow(8, rowLabel("Quality"), v.qualityPicker())}
	var menus []fyne.CanvasObject
	if v.narrowShelf {
		btn := newButton("Lighting", nil)
		btn.onTap = func() {
			v.showPopupBelow(btn, vstack(6, sectionTitle("Lighting", ""), v.lightingRows()), 300)
		}
		menus = append(menus, btn)
	}
	if v.st.preset == paletteMaterials {
		menus = append(menus, v.bandsButton())
	}
	if len(menus) > 0 {
		items = append(items, hrow(8, menus...))
	}
	return vstack(6, items...)
}

func (v *Viewer) palettePicker() *segmented {
	return newSegmented(paletteLabels, int(v.st.preset), func(i int) {
		v.st.preset = palettePreset(i)
		v.applyPalette()
		v.rebuild() // legend and band button depend on the palette
	})
}

func (v *Viewer) qualityPicker() *segmented {
	return newSegmented(qualityLabels, int(v.st.quality), func(i int) {
		v.st.quality = renderQuality(i)
		v.applyQuality()
	})
}

// bandsButton opens the band editor (Materials only).
func (v *Viewer) bandsButton() *button {
	btn := newButton("Material bands", nil)
	btn.onTap = func() {
		if v.st.layout.compact() {
			v.st.bandsOpen = true
			v.rebuild()
			return
		}
		v.showPopupBelow(btn, v.bandsEditor(), 340)
	}
	return btn
}

// legendHeight is fixed, so switching palettes moves nothing.
const legendHeight = 34

// legendLayout places what a palette's colours mean in the fixed-height strip.
type legendLayout struct {
	preset palettePreset
	dots   []*canvas.Circle
	names  []*canvas.Text
	bar    *canvas.Image
	frame  *canvas.Rectangle
	labels [3]*canvas.Text // low, "Stored value", high
}

func (l *legendLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(60, legendHeight) }

func (l *legendLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	switch l.preset {
	case paletteMaterials:
		// Dot-and-name pairs on one line, or two balanced lines: no name is orphaned.
		const gap = 12
		widths := make([]float32, len(l.names))
		for i, n := range l.names {
			widths[i] = 14 + n.MinSize().Width
		}
		run := func(w []float32) float32 {
			var t float32
			for _, x := range w {
				t += x
			}
			return t + gap*float32(max(len(w)-1, 0))
		}
		first := len(widths)
		if run(widths) > size.Width {
			best := float32(math.MaxFloat32)
			for k := 1; k < len(widths); k++ {
				if longest := max(run(widths[:k]), run(widths[k:])); longest < best {
					best, first = longest, k
				}
			}
		}
		line := textHeight(12) + 4
		lines := float32(1)
		if first != len(widths) {
			lines = 2
		}
		top := (legendHeight - line*lines) / 2
		x, y := float32(0), top
		for i := range l.names {
			if i == first {
				x, y = 0, top+line
			}
			l.dots[i].Move(fyne.NewPos(x+0.5, y+line/2-4.5))
			l.dots[i].Resize(fyne.NewSize(9, 9))
			ts := l.names[i].MinSize()
			l.names[i].Move(fyne.NewPos(x+14, y+(line-ts.Height)/2))
			l.names[i].Resize(ts)
			x += widths[i] + gap
		}
	case paletteGrayscale:
		w := min(size.Width, 360)
		l.frame.Move(fyne.NewPos(0, 3))
		l.frame.Resize(fyne.NewSize(w, 9))
		l.bar.Move(fyne.NewPos(0, 3))
		l.bar.Resize(fyne.NewSize(w, 9))
		for i, t := range l.labels {
			ts := t.MinSize()
			x := [3]float32{0, (w - ts.Width) / 2, w - ts.Width}[i]
			t.Move(fyne.NewPos(x, legendHeight-ts.Height))
			t.Resize(ts)
		}
	default:
		l.dots[0].Move(fyne.NewPos(0.5, legendHeight/2-4.5))
		l.dots[0].Resize(fyne.NewSize(9, 9))
		ts := l.names[0].MinSize()
		l.names[0].Move(fyne.NewPos(14, (legendHeight-ts.Height)/2))
		l.names[0].Resize(ts)
	}
}

// grayRamp is the grayscale palette's ramp, as drawn in the legend.
func grayRamp() *image.Gray {
	const n = 256
	img := image.NewGray(image.Rect(0, 0, n, 1))
	for i := 0; i < n; i++ {
		img.SetGray(i, 0, color.Gray{uint8(0.9*math.Pow(float64(i)/(n-1), 0.65)*255 + 0.5)})
	}
	return img
}

// paletteLegend is a fixed-height strip explaining the colours.
func (v *Viewer) paletteLegend() fyne.CanvasObject {
	l := &legendLayout{preset: v.st.preset}
	var objs []fyne.CanvasObject
	switch v.st.preset {
	case paletteMaterials:
		for _, b := range v.st.bands {
			if b.IsTransparent {
				continue
			}
			l.dots = append(l.dots, canvas.NewCircle(color.NRGBA{b.Color.R, b.Color.G, b.Color.B, 255}))
			l.names = append(l.names, text(b.Name, 12, colText))
		}
	case paletteGrayscale:
		l.bar = canvas.NewImageFromImage(grayRamp())
		l.bar.FillMode = canvas.ImageFillStretch
		l.bar.ScaleMode = canvas.ImageScaleSmooth
		l.frame = rrect(color.Transparent, colHairline, 0, 1)
		lo := v.st.windowCenter3D - v.st.windowWidth3D/2
		hi := v.st.windowCenter3D + v.st.windowWidth3D/2
		l.labels = [3]*canvas.Text{text(f0(lo), 12, colMuted), text("Stored value", 12, colMuted), text(f0(hi), 12, colMuted)}
		objs = append(objs, l.bar, l.frame, l.labels[0], l.labels[1], l.labels[2])
	default:
		l.dots = []*canvas.Circle{canvas.NewCircle(color.NRGBA{235, 80, 70, 255})}
		l.names = []*canvas.Text{text("Dense material in red", 12, colText)}
	}
	for i := range l.dots {
		objs = append(objs, l.dots[i], l.names[i])
	}
	return container.New(l, objs...)
}

// bandsEditor: band thresholds strip, then one opacity slider per band.
func (v *Viewer) bandsEditor() fyne.CanvasObject {
	bands := v.st.bands
	maxDensity := float64(bands[len(bands)-1].Threshold)
	strip := NewMultiRangeSlider(0, maxDensity, bands)
	strip.Compact = v.st.layout.compact()
	strip.OnChanged = func([]int) {
		v.debounced("bands", 60*time.Millisecond, v.applyPalette)
	}
	items := []fyne.CanvasObject{sectionTitle("Band thresholds", "stored values"), strip, sectionTitle("Band opacity", "")}
	for i := range bands {
		if bands[i].IsTransparent {
			continue
		}
		items = append(items, v.bandOpacityRow(i, bands[i].Name, strip.GetBandAlpha(i)).obj)
	}
	return vstack(6, items...)
}

// bandOpacityRow is one band's opacity multiplier, 0 to 2 times.
func (v *Viewer) bandOpacityRow(i int, label string, alpha float64) *sliderRow {
	return newSliderRow(fraction(label, 0, 2).withUnit("×"), alpha, func(x float64) {
		v.st.bands[i].Alpha = x
		v.debounced("bands", 60*time.Millisecond, v.applyPalette)
	}, nil)
}

// lightingRows: depth cue, ambient, diffuse and specular.
func (v *Viewer) lightingRows() fyne.CanvasObject {
	row := func(label string, p *float64) fyne.CanvasObject {
		return newSliderRow(fraction(label, 0, 1), *p, func(x float64) {
			*p = x
			v.applyLighting()
		}, nil).obj
	}
	return vstack(6, row("Depth cue", &v.st.depthCue), row("Ambient", &v.st.ambient),
		row("Diffuse", &v.st.diffuse), row("Specular", &v.st.specular))
}

// showPopupBelow shows content in a popup under anchor, at the given width.
func (v *Viewer) showPopupBelow(anchor fyne.CanvasObject, content fyne.CanvasObject, width float32) {
	pop := widget.NewPopUp(frame(colOutline, 6, 12, 10, content), v.win.Canvas())
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	size := fyne.NewSize(width, pop.MinSize().Height)
	canvasSize := v.win.Canvas().Size()
	x := min(pos.X, canvasSize.Width-width-8)
	y := pos.Y + anchor.Size().Height + 4
	if y+size.Height > canvasSize.Height {
		y = max(pos.Y-size.Height-4, 8)
	}
	pop.ShowAtPosition(fyne.NewPos(max(x, 8), y))
	pop.Resize(size)
}
