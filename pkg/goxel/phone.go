package goxel

import (
	"fmt"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
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
		return container.NewBorder(v.appBar(), nil, nil, nil, v.emptyState())
	}
	if v.st.focusSlice || v.st.focusVolume {
		return v.phoneFocusView()
	}
	var body fyne.CanvasObject
	switch v.st.phoneTab {
	case tabStudy:
		body = v.phonePage(v.studyPage())
	case tabFindings:
		body = v.phonePage(v.findingsSection())
	case tabSlice:
		body = v.withSheet(v.phoneSliceArea(), v.sheetOpenFor(), v.sliceSheet)
	default:
		body = v.withSheet(v.volumeRenderer, v.sheetOpenFor(), v.volumeSheet)
	}
	return container.NewBorder(v.appBar(), v.tabBar(), nil, nil, body)
}

func (v *Viewer) sheetOpenFor() bool { return v.st.sheetOpen }

// phonePage is a full page of reading content; pages hold no sliders, so they scroll.
func (v *Viewer) phonePage(content fyne.CanvasObject) fyne.CanvasObject {
	return panel(colPanel, 0, container.NewVScroll(inset(16, 16, content)))
}

func (v *Viewer) studyPage() fyne.CanvasObject {
	many := len(v.volumeKeys()) > 1
	if v.st.layout == layoutPhoneLandscape {
		right := []fyne.CanvasObject{v.detailsSection()}
		if many {
			right = append(right, hairline(), v.volumesSection())
		}
		return container.NewGridWithColumns(2, v.studySection(), container.NewVBox(right...))
	}
	items := []fyne.CanvasObject{v.studySection(), hairline(), v.detailsSection()}
	if many {
		items = append(items, hairline(), v.volumesSection())
	}
	items = append(items, hairline(), v.metadataSection())
	return container.NewVBox(items...)
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
		return container.NewBorder(nil, nil, nil, container.New(sizedLayout{w: landscapePanel}, s), view)
	}
	return container.NewBorder(nil, container.New(sizedLayout{h: sheetHeight}, s), nil, nil, view)
}

// tabBar: four tabs; the active one has an ACCENT top bar and label, view
// tabs carry a chevron for their sheet, Findings a count badge.
func (v *Viewer) tabBar() fyne.CanvasObject {
	tabs := make([]fyne.CanvasObject, 0, 4)
	for _, t := range []phoneTab{tabStudy, tabFindings, tabSlice, tabView3D} {
		tabs = append(tabs, v.tabButton(t))
	}
	row := container.New(sizedLayout{h: tabRowHeight}, container.NewGridWithColumns(4, tabs...))
	inset := v.st.layout.homeIndicatorInset()
	return panel(colBG, 0, container.NewBorder(hairline(), nil, nil, nil,
		container.NewVBox(row, container.New(sizedLayout{h: inset}, layout.NewSpacer()))))
}

func (v *Viewer) tabButton(t phoneTab) fyne.CanvasObject {
	active := t == v.st.phoneTab
	fg := color.Color(colMuted)
	topBar := filled(color.Transparent)
	if active {
		fg = colAccent
		topBar.FillColor = colAccent
	}
	topBar.SetMinSize(fyne.NewSize(1, 3))
	label := text(t.label(), 14, fg, active)
	row := container.NewHBox(layout.NewSpacer(), container.NewCenter(label))
	if active && t.isView() {
		res := theme.MenuDropUpIcon()
		if v.st.sheetOpen {
			res = theme.MenuDropDownIcon()
		}
		chev := canvas.NewImageFromResource(theme.NewPrimaryThemedResource(res))
		chev.SetMinSize(fyne.NewSize(14, 14))
		row.Add(container.NewCenter(chev))
	}
	if n := v.findingCount(); t == tabFindings && n > 0 {
		badge := canvas.NewCircle(colDanger)
		count := text(fmt.Sprintf("%d", n), 11, colBG, true)
		count.Alignment = fyne.TextAlignCenter
		row.Add(container.NewCenter(container.New(sizedLayout{w: 16, h: 16}, container.NewStack(badge, container.NewCenter(count)))))
	}
	row.Add(layout.NewSpacer())
	content := container.NewBorder(topBar, nil, nil, nil, container.NewCenter(row))
	return newTapArea(content, func() {
		v.st.selectTab(t)
		v.rebuild()
		if t == tabView3D {
			v.volumeRenderer.FitView()
		}
	})
}

// pageSelector is the sheet's segmented page picker.
func pageSelector(labels []string, selected int, onSelect func(int)) fyne.CanvasObject {
	return minHeight(touchTarget, newSegmented(labels, selected, onSelect).box)
}

// buttonRow is a row of equal touch buttons.
func buttonRow(buttons ...*widget.Button) fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(buttons))
	for i, b := range buttons {
		objs[i] = b
	}
	return minHeight(touchTarget, container.NewGridWithColumns(len(objs), objs...))
}

// touchRow is a label then content, at touch height.
func touchRow(label string, content fyne.CanvasObject) fyne.CanvasObject {
	l := widget.NewLabel(label)
	return minHeight(touchTarget, container.NewBorder(nil, nil, fixedWidth(96, container.NewVBox(layout.NewSpacer(), l, layout.NewSpacer())), nil, content))
}

// volumeSheet: Camera / Slice / Windowing / Display / Lighting, or the band
// editor while it is open.
func (v *Viewer) volumeSheet() fyne.CanvasObject {
	if v.st.bandsOpen && v.st.preset == paletteMaterials {
		done := widget.NewButton("Done", func() {
			v.st.bandsOpen = false
			v.rebuild()
		})
		done.Importance = widget.HighImportance
		return container.NewBorder(minHeight(touchTarget, container.NewBorder(nil, nil,
			container.NewCenter(text("Material bands", 15, colHeading, true)), done)), nil, nil, nil,
			container.NewVScroll(v.bandsEditor()))
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
		page = container.NewVBox(minHeight(touchTarget, v.slicePlaneTools()),
			minHeight(touchTarget, v.planePicker().box), v.sliceRow().obj)
	case pageWindowing:
		c := v.st.layout.compact()
		center := newSliderRow(c, "Center", 0, 65535, 1, f0, v.st.windowCenter3D, func(x float64) {
			v.st.windowCenter3D = x
			v.apply3DWindow()
		}, nil)
		width := newSliderRow(c, "Width", 1, 65535, 1, f0, v.st.windowWidth3D, func(x float64) {
			v.st.windowWidth3D = x
			v.apply3DWindow()
		}, nil)
		page = container.NewVBox(center.obj, width.obj, v.opacityRow().obj)
	case pageDisplay:
		legend := v.paletteLegend()
		var legendRow fyne.CanvasObject = legend
		if v.st.preset == paletteMaterials {
			legendRow = container.NewBorder(nil, nil, nil, fixedWidth(132, v.bandsButton()), legend)
		}
		page = container.NewVBox(minHeight(touchTarget, v.palettePicker().box), minHeight(touchTarget, legendRow),
			touchRow("Quality", v.qualityPicker().box), v.densityRow().obj)
	default:
		page = v.lightingRows()
	}
	return container.NewBorder(sel, nil, nil, nil, inset(0, 8, page))
}

// cameraPage gives a tap for everything the gestures do.
func (v *Viewer) cameraPage() fyne.CanvasObject {
	vr := v.volumeRenderer
	var rows []fyne.CanvasObject
	if !v.st.focusVolume {
		rows = append(rows, buttonRow(
			widget.NewButton("Fit view", vr.FitView),
			widget.NewButton("Focus volume", v.toggleFocusVolume)))
	}
	rows = append(rows, touchRow("View along", buttonRow(
		widget.NewButton("X", func() { vr.LookAlong(0) }),
		widget.NewButton("Y", func() { vr.LookAlong(1) }),
		widget.NewButton("Z", func() { vr.LookAlong(2) }))))
	mode := 0
	if v.st.dragPans {
		mode = 1
	}
	orbitPan := newSegmented([]string{"Orbit", "Pan"}, mode, func(i int) { v.st.dragPans = i == 1 })
	arrow := func(icon fyne.Resource, dx, dy float64) *widget.Button {
		return widget.NewButtonWithIcon("", icon, func() {
			if v.st.dragPans {
				vr.PanPixels(dx*panStep, dy*panStep)
			} else {
				vr.Orbit(-dx*orbitStep, -dy*orbitStep)
			}
		})
	}
	arrows := container.NewGridWithColumns(4,
		arrow(theme.NavigateBackIcon(), -1, 0), arrow(theme.NavigateNextIcon(), 1, 0),
		arrow(theme.MoveUpIcon(), 0, -1), arrow(theme.MoveDownIcon(), 0, 1))
	rows = append(rows, minHeight(touchTarget, container.NewGridWithColumns(2, orbitPan.box, arrows)))
	rows = append(rows, touchRow("Zoom", buttonRow(
		widget.NewButton("-", func() { vr.ZoomBy(1.25) }),
		widget.NewButton("+", func() { vr.ZoomBy(1 / 1.25) }))))
	return container.NewVBox(rows...)
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
		page = container.NewVBox(center.obj, width.obj, v.zoomRow().obj)
	case pageVolume:
		picker := v.volumePicker()
		if picker == nil {
			picker = layout.NewSpacer()
		}
		page = container.NewVBox(minHeight(touchTarget, picker),
			note("Changes the 2D slice only. Pick the 3D volume under Study."))
	default:
		rows := []fyne.CanvasObject{minHeight(touchTarget, v.planePicker().box), v.sliceRow().obj,
			minHeight(touchTarget, v.compositeToggle())}
		if !v.st.focusSlice {
			rows = append(rows, buttonRow(
				widget.NewButton("Fit image", v.fitImage),
				widget.NewButton("Focus slice", v.toggleFocusSlice)))
		}
		page = container.NewVBox(rows...)
	}
	return container.NewBorder(sel, nil, nil, nil, inset(0, 8, page))
}

// phoneFocusView: the view fills the screen above one action bar; "Adjust
// display" opens the same paged controls the tab view uses.
func (v *Viewer) phoneFocusView() fyne.CanvasObject {
	slice := v.st.focusSlice
	exit := widget.NewButton("Exit focus", func() {
		v.st.focusSlice, v.st.focusVolume, v.st.focusAdjustOpen = false, false, false
		v.rebuild()
		v.volumeRenderer.FitView()
	})
	fitLabel, fit := "Fit view", v.volumeRenderer.FitView
	if slice {
		fitLabel, fit = "Fit image", v.fitImage
	}
	adjust := widget.NewButton("Adjust display", func() {
		v.st.focusAdjustOpen = !v.st.focusAdjustOpen
		v.rebuild()
	})
	if v.st.focusAdjustOpen {
		adjust.Importance = widget.SuccessImportance
	}
	bar := panel(colBG, 0, container.NewVBox(
		inset(16, 8, buttonRow(exit, widget.NewButton(fitLabel, fit), adjust)),
		container.New(sizedLayout{h: v.st.layout.homeIndicatorInset()}, layout.NewSpacer())))

	view, sheet := fyne.CanvasObject(v.volumeRenderer), v.volumeSheet
	if slice {
		view, sheet = v.phoneSliceArea(), v.sliceSheet
	}
	if v.st.focusAdjustOpen {
		view = v.withSheet(view, true, sheet)
	} else if slice && !v.st.composite && v.dicox.MaxSlice() > 0 {
		// Stepping through slices is the main thing to do here: keep it out.
		row := panel(colPanel, 0, inset(16, 8, v.sliceRow().obj))
		view = container.NewBorder(nil, row, nil, nil, view)
	}
	return container.NewBorder(nil, bar, nil, nil, view)
}
