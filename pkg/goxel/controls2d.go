package goxel

import (
	"fmt"
	"log/slog"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// slicePanel is the desktop right column: header, image well with caption,
// then the Slice, 2D windowing and Image groups.
func (v *Viewer) slicePanel() fyne.CanvasObject {
	header := container.NewBorder(nil, nil,
		container.NewCenter(text("2D slice", 15, colHeading, true)),
		container.NewHBox(
			widget.NewButton("Fit image", v.fitImage),
			widget.NewButton("Focus slice", v.toggleFocusSlice),
		))
	controls := container.NewVBox(v.sliceGroup(), hairline(), v.windowingGroup2D(), hairline(), v.imageGroup())
	body := container.NewBorder(header, controls, nil, nil, v.sliceWell())
	edge := filled(colHairline)
	edge.SetMinSize(fyne.NewSize(1, 1))
	return container.NewStack(filled(colPanel),
		container.NewBorder(nil, nil, edge, nil, inset(12, 8, body)))
}

// sliceFocusView: the slice fills the workspace with its groups docked below.
func (v *Viewer) sliceFocusView() fyne.CanvasObject {
	header := container.NewBorder(nil, nil,
		container.NewCenter(text("2D slice", 15, colHeading, true)),
		container.NewHBox(
			widget.NewButton("Fit image", v.fitImage),
			widget.NewButton("Exit focus", v.toggleFocusSlice),
		))
	groups := container.NewGridWithColumns(3, v.sliceGroup(), v.windowingGroup2D(), v.imageGroup())
	return panel(colPanel, 0, inset(12, 8, container.NewBorder(header, groups, nil, nil, v.sliceWell())))
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

// sliceWell: the black image well with a caption footer.
func (v *Viewer) sliceWell() fyne.CanvasObject {
	caption := text(v.sliceCaption(), 12, colMuted, false)
	caption.Alignment = fyne.TextAlignTrailing
	v.onSync(func() {
		caption.Text = v.sliceCaption()
		caption.Refresh()
	})
	clip := container.NewScroll(v.interactiveView)
	clip.Direction = container.ScrollNone
	footer := panel(colWell, 0, inset(8, 6, shrinkable(caption)))
	return container.NewBorder(nil, footer, nil, nil, clip)
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
	r := newSliderRow(v.st.layout.compact(), "Slice", 0, float64(max(v.dicox.MaxSlice(), 1)), 1,
		func(x float64) string { return fmt.Sprintf("%.0f", x+1) }, float64(v.st.sliceIndex),
		func(x float64) { v.setSlice(int(x)) }, nil)
	sync := func() {
		r.slider.Max = float64(max(v.dicox.MaxSlice(), 1))
		r.SetValue(float64(v.st.sliceIndex))
		if v.st.composite {
			r.slider.Disable()
		} else {
			r.slider.Enable()
		}
	}
	sync()
	v.onSync(sync)
	return r
}

// sliceGroup: plane picker, slider and Previous / Next.
func (v *Viewer) sliceGroup() fyne.CanvasObject {
	prev := widget.NewButton("Previous", func() { v.stepSlice(-1) })
	next := widget.NewButton("Next", func() { v.stepSlice(1) })
	return container.NewVBox(sectionTitle("Slice", ""), v.planePicker().box, v.sliceRow().obj,
		container.NewGridWithColumns(2, prev, next))
}

// windowingGroup2D: centre and width in stored values. The 2D view is
// re-rasterised on the CPU, so the values commit when a drag ends.
func (v *Viewer) windowingGroup2D() fyne.CanvasObject {
	c := v.st.layout.compact()
	commit := func() {
		v.dicox.SetWindowLevel(int(v.st.windowCenter2D))
		v.dicox.SetWindowWidth(int(v.st.windowWidth2D))
		v.refresh2D()
	}
	center := newSliderRow(c, "Center", 0, 65535, 1, f0, v.st.windowCenter2D,
		func(x float64) { v.st.windowCenter2D = x }, func(float64) { commit() })
	width := newSliderRow(c, "Width", 1, 65535, 1, f0, v.st.windowWidth2D,
		func(x float64) { v.st.windowWidth2D = x }, func(float64) { commit() })
	return container.NewVBox(sectionTitle("2D windowing", "stored values"), center.obj, width.obj)
}

// zoomRow: 50-500 % in steps of 25.
func (v *Viewer) zoomRow() *sliderRow {
	r := newSliderRow(v.st.layout.compact(), "Zoom", 50, 500, 25,
		func(x float64) string { return fmt.Sprintf("%.0f%%", x) }, v.st.zoom2D,
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
			v.set2DSource(s)
		}
	})
	sel.Selected = v.currentVolumeKey
	return container.NewBorder(nil, nil, fixedWidth(64, widget.NewLabel("Volume")), nil, sel)
}

func (v *Viewer) compositeToggle() fyne.CanvasObject {
	return newCheck("Composite projection", v.st.composite, func(b bool) {
		v.setComposite(b)
		v.sync()
	})
}

// imageGroup: zoom, volume, composite projection and Add Finding (desktop).
func (v *Viewer) imageGroup() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Image", ""), v.zoomRow().obj}
	if p := v.volumePicker(); p != nil {
		items = append(items, p)
	}
	items = append(items, v.compositeToggle())
	if !v.st.layout.compact() {
		add := widget.NewButton("Add Finding", func() {
			v.isAnnotating = !v.isAnnotating
			slog.Info("Annotation Mode Toggled", "active", v.isAnnotating)
		})
		items = append(items, container.NewHBox(layout.NewSpacer(), add))
	}
	return container.NewVBox(items...)
}
