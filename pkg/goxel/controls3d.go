package goxel

import (
	"fmt"
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

func f0(x float64) string { return fmt.Sprintf("%.0f", x) }
func f2(x float64) string { return fmt.Sprintf("%.2f", x) }

// volumeHeader is the 3D column's two header rows: title and camera actions,
// then the slice-plane tools (with the plane picker and slice slider in focus).
func (v *Viewer) volumeHeader() fyne.CanvasObject {
	focusLabel := "Focus volume"
	if v.st.focusVolume {
		focusLabel = "Exit focus"
	}
	row1 := container.NewBorder(nil, nil,
		container.NewCenter(text("3D view", 15, colHeading, true)),
		container.NewHBox(
			container.NewCenter(text("View along", 12, colMuted, false)),
			v.viewAlongButtons(),
			widget.NewButton("Fit view", v.volumeRenderer.FitView),
			widget.NewButton(focusLabel, v.toggleFocusVolume),
		))
	rows := []fyne.CanvasObject{row1, v.slicePlaneTools()}
	if v.st.focusVolume {
		rows = append(rows, container.NewGridWithColumns(2, v.planePicker().box, v.sliceRow().obj))
	}
	return panel(colBG, 0, inset(12, 6, container.NewVBox(rows...)))
}

func (v *Viewer) viewAlongButtons() fyne.CanvasObject {
	return container.NewHBox(
		widget.NewButton("X", func() { v.volumeRenderer.LookAlong(0) }),
		widget.NewButton("Y", func() { v.volumeRenderer.LookAlong(1) }),
		widget.NewButton("Z", func() { v.volumeRenderer.LookAlong(2) }),
	)
}

func (v *Viewer) toggleFocusVolume() {
	v.st.focusVolume = !v.st.focusVolume
	v.st.focusAdjustOpen = false
	v.rebuild()
	v.volumeRenderer.FitView()
}

// slicePlaneTools: the slice-plane outline toggle and the cutaway pair.
func (v *Viewer) slicePlaneTools() fyne.CanvasObject {
	plane := newCheck("Slice plane", v.st.showSlicePlane, func(b bool) { v.st.showSlicePlane = b })
	reverse := newCheck("Reverse cutaway", v.st.reverseCutaway, func(b bool) {
		v.st.reverseCutaway = b
		v.applyCutaway()
	})
	cut := newCheck("Cutaway", v.st.cutaway, func(b bool) {
		v.st.cutaway = b
		if b {
			reverse.Enable()
		} else {
			reverse.Disable()
		}
		v.applyCutaway()
	})
	if !v.st.cutaway {
		reverse.Disable()
	}
	unavailable := container.NewCenter(text("Needs a single 2D slice of this volume", 12, colMuted, false))
	sync := func() {
		if v.cutawayAvailable() {
			plane.Enable()
			cut.Enable()
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
	return container.NewHBox(plane, cut, reverse, unavailable)
}

// shelf is the three columns under the 3D view (two when narrow, with
// lighting behind a menu button).
func (v *Viewer) shelf() fyne.CanvasObject {
	cols := []fyne.CanvasObject{v.windowingGroup3D(), v.paletteGroup()}
	if v.narrowShelf && !v.st.focusVolume {
		btn := widget.NewButton("Lighting", nil)
		btn.OnTapped = func() {
			v.showPopupBelow(btn, container.NewVBox(sectionTitle("Lighting", ""), v.lightingRows()), 320)
		}
		cols[1] = container.NewVBox(cols[1], btn)
	} else {
		cols = append(cols, container.NewVBox(sectionTitle("Lighting", ""), v.lightingRows()))
	}
	return panel(colPanel, 0, inset(12, 8, container.NewGridWithColumns(len(cols), cols...)))
}

// windowingGroup3D: centre/width in stored values, opacity and density.
func (v *Viewer) windowingGroup3D() fyne.CanvasObject {
	c := v.st.layout.compact()
	center := newSliderRow(c, "Center", 0, 65535, 1, f0, v.st.windowCenter3D, func(x float64) {
		v.st.windowCenter3D = x
		v.apply3DWindow()
	}, nil)
	width := newSliderRow(c, "Width", 1, 65535, 1, f0, v.st.windowWidth3D, func(x float64) {
		v.st.windowWidth3D = x
		v.apply3DWindow()
	}, nil)
	return container.NewVBox(sectionTitle("3D windowing", "stored values"), center.obj, width.obj,
		v.opacityRow().obj, v.densityRow().obj)
}

func (v *Viewer) opacityRow() *sliderRow {
	return newSliderRow(v.st.layout.compact(), "Opacity", 0, 1, 0.01, f2, v.st.opacity, func(x float64) {
		v.st.opacity = x
		v.applyOpacity()
	}, nil)
}

func (v *Viewer) densityRow() *sliderRow {
	return newSliderRow(v.st.layout.compact(), "Density", 0, 1, 0.01, f2, v.st.density, func(x float64) {
		v.st.density = x
		v.applyDensity()
	}, nil)
}

// paletteGroup: Materials / Threat / Grayscale, legend, quality and the
// material band editor.
func (v *Viewer) paletteGroup() fyne.CanvasObject {
	return container.NewVBox(sectionTitle("Palette", ""), v.palettePicker().box, v.paletteLegend(),
		container.NewBorder(nil, nil, fixedWidth(64, widget.NewLabel("Quality")), nil, v.qualityPicker().box),
		v.bandsButton())
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
func (v *Viewer) bandsButton() fyne.CanvasObject {
	btn := widget.NewButton("Material bands", nil)
	btn.OnTapped = func() {
		if v.st.layout.compact() {
			v.st.bandsOpen = true
			v.rebuild()
			return
		}
		v.showPopupBelow(btn, v.bandsEditor(), 340)
	}
	if v.st.preset != paletteMaterials {
		btn.Hide()
	}
	return btn
}

// paletteLegend is a fixed-height strip explaining the colours.
func (v *Viewer) paletteLegend() fyne.CanvasObject {
	var body fyne.CanvasObject
	switch v.st.preset {
	case paletteMaterials:
		row := container.NewHBox()
		for _, b := range v.st.bands {
			if b.IsTransparent {
				continue
			}
			dot := canvas.NewCircle(color.NRGBA{b.Color.R, b.Color.G, b.Color.B, 255})
			dot.Resize(fyne.NewSize(10, 10))
			row.Add(container.NewCenter(container.New(sizedLayout{w: 10, h: 10}, dot)))
			row.Add(text(b.Name, 12, colMuted, false))
		}
		body = row
	case paletteGrayscale:
		ramp := canvas.NewHorizontalGradient(color.Black, color.White)
		ramp.SetMinSize(fyne.NewSize(60, 10))
		lo := v.st.windowCenter3D - v.st.windowWidth3D/2
		hi := v.st.windowCenter3D + v.st.windowWidth3D/2
		body = container.NewBorder(nil, nil, text(f0(lo), 12, colMuted, false), text(f0(hi), 12, colMuted, false),
			container.NewPadded(ramp))
	default:
		dot := canvas.NewCircle(color.NRGBA{220, 50, 40, 255})
		body = container.NewHBox(container.NewCenter(container.New(sizedLayout{w: 10, h: 10}, dot)),
			text("Dense material in red", 12, colMuted, false))
	}
	return minHeight(34, container.NewVBox(layout.NewSpacer(), shrinkable(body), layout.NewSpacer()))
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
	items := []fyne.CanvasObject{sectionTitle("Band thresholds", ""), strip, sectionTitle("Band opacity", "")}
	for i := range bands {
		if bands[i].IsTransparent {
			continue
		}
		i := i
		row := newSliderRow(v.st.layout.compact(), bands[i].Name, 0, 2, 0.05,
			func(x float64) string { return fmt.Sprintf("%.0f%%", x*100) }, strip.GetBandAlpha(i),
			func(x float64) {
				v.st.bands[i].Alpha = x
				v.debounced("bands", 60*time.Millisecond, v.applyPalette)
			}, nil)
		items = append(items, row.obj)
	}
	return container.NewVBox(items...)
}

// lightingRows: ambient, diffuse and specular.
func (v *Viewer) lightingRows() fyne.CanvasObject {
	c := v.st.layout.compact()
	row := func(label string, p *float64) fyne.CanvasObject {
		return newSliderRow(c, label, 0, 1, 0.01, f2, *p, func(x float64) {
			*p = x
			v.applyLighting()
		}, nil).obj
	}
	return container.NewVBox(row("Ambient", &v.st.ambient), row("Diffuse", &v.st.diffuse), row("Specular", &v.st.specular))
}

// showPopupBelow shows content in a popup under anchor, at the given width.
func (v *Viewer) showPopupBelow(anchor fyne.CanvasObject, content fyne.CanvasObject, width float32) {
	pop := widget.NewPopUp(inset(12, 10, content), v.win.Canvas())
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
