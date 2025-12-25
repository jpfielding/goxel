package goxel

import (
	"fmt"
	"image/color"
	"log/slog"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/jpfielding/goxel/pkg/volume"
)

// Viewer handles the UI state
type Viewer struct {
	win         fyne.Window
	image       *canvas.Image
	dicox       *DICOXImage
	mainContent *fyne.Container // Main content area (placeholder or layout)
	menu        *fyne.MainMenu
	layoutMenu  *fyne.Menu

	// Navigation
	frame  *widget.Label
	slider *widget.Slider
	level  *widget.Entry
	width  *widget.Entry

	// Data
	currentFrame int
	frameCount   int

	// Metadata Display (DICOM fields)
	patientID         *widget.Label
	patientName       *widget.Label
	studyDate         *widget.Label
	studyTime         *widget.Label
	studyDescription  *widget.Label
	seriesDescription *widget.Label
	modality          *widget.Label
	institutionName   *widget.Label
	manufacturer      *widget.Label
	operatorName      *widget.Label

	// File browser state
	currentDir string

	// 3D Rendering
	volumeRenderer *VolumeRenderer
	currentScaleZ  float64 // Current Z scale for aspect ratio adjustment
	zScaleSlider   *LabeledSlider

	// Layout state
	currentLayout string

	// Multi-dataset support
	compositeVolumes map[string]*CompositeVolume
	projections      map[string]*CompositeVolume

	// Active dataset
	currentVolumeKey string
	pixelData        *PixelData // Currently active pixel data for 2D

	// Image Metadata for Slice Adapter
	imageRows           int
	imageCols           int
	bitsAllocated       int
	pixelRepresentation int

	// View orientation for 2D
	viewSelect *widget.Select

	// UI Containers
	layersContainer  *fyne.Container
	findingsContainer *fyne.Container
	accordion        *widget.Accordion

	// Annotation State
	isAnnotating    bool
	interactiveView *Interactive2DView
}

// createSideBySidePanel creates a container with 3D view on left, 2D on right
func (v *Viewer) createSideBySidePanel() fyne.CanvasObject {
	// Create Interactive View for 2D (Right Side)
	if v.interactiveView == nil {
		v.interactiveView = NewInteractive2DView(v.image, v)
		v.interactiveView.OnAnnotationAdded = func(x, y, w, h float32) {
			slog.Info("Annotation Added", "rect", fmt.Sprintf("%.1f,%.1f %.1fx%.1f", x, y, w, h))
			dialog.ShowInformation("ROI Annotated", "ROI bounding box created.", v.win)
		}
	}
	imgWidget := v.interactiveView

	// 2D Controls
	// We need: Volume Selector, Orientation, Window/Level, Slider

	// Slice Slider
	sliceControl := NewLabeledSlider(0, 1)
	sliceControl.Slider.Step = 1
	sliceControl.SetOnChanged(func(val float64) {
		idx := int(val)
		v.setFrame(idx)

		// Ensure imgWidget refreshes
		if v.image != nil {
			v.image.Image = v.dicox // Ensure it points to the right source
			canvas.Refresh(v.image)
		}
		imgWidget.Refresh()
	})
	sliceSlider := sliceControl.Slider // Alias for internal usage

	// v.frame = sliceLabel // Removed as requested
	v.slider = sliceSlider // Store reference for setFrame updates

	// Initial update of slider range based on current orientation
	if v.dicox.HasVolume() {
		var max int
		switch v.dicox.Orientation() {
		case Axial:
			max = v.dicox.Depth() - 1
		case Coronal:
			max = v.dicox.Height() - 1
		case Sagittal:
			max = v.dicox.Width() - 1
		}
		if max < 0 {
			max = 0
		}
		sliceSlider.Max = float64(max)
		sliceSlider.SetValue(float64(v.dicox.sliceIndex))
	}

	// Volume Selector
	var volKeys []string
	if len(v.compositeVolumes) > 0 {
		for k := range v.compositeVolumes {
			volKeys = append(volKeys, k)
		}
		sort.Strings(volKeys)
	}
	// Append projections keys if any
	if len(v.projections) > 0 {
		for k := range v.projections {
			volKeys = append(volKeys, k)
		}
		// Sort again includes projections
		sort.Strings(volKeys)
	}

	volumeSelect := widget.NewSelect(volKeys, func(s string) {
		// Switch active volume
		v.currentVolumeKey = s
		slog.Info("Switching volume", "vol", s)
		// Update Z scale based on selected volume's voxel spacing
		v.updateZScaleForVolume(s)
	})
	if len(volKeys) > 0 {
		volumeSelect.SetSelected(volKeys[0])
	}

	// Orientation Selector
	if v.viewSelect == nil {
		v.viewSelect = widget.NewSelect([]string{"Axial", "Coronal", "Sagittal"}, func(s string) {
			slog.Info("View orientation selected", slog.String("selected", s))
			if !v.dicox.HasVolume() {
				slog.Warn("No volume/data loaded")
				return
			}
			var orient ViewOrientation
			switch s {
			case "Axial":
				orient = Axial
			case "Coronal":
				orient = Coronal
			case "Sagittal":
				orient = Sagittal
			}
			v.dicox.SetOrientation(orient)

			// Reset slider for new dimension
			switch orient {
			case Axial:
				v.slider.Max = float64(v.dicox.Depth() - 1)
			case Coronal:
				v.slider.Max = float64(v.dicox.Height() - 1)
			case Sagittal:
				v.slider.Max = float64(v.dicox.Width() - 1)
			}
			v.slider.SetValue(0)
			v.setFrame(0)
			v.image.Refresh()
		})
	}
	orientSelect := v.viewSelect
	// Set default to match scan_adapter initialization
	// Store the current frame before SetSelected triggers callback that resets to 0
	savedFrame := v.currentFrame
	if v.dicox.HasVolume() {
		switch v.dicox.Orientation() {
		case Axial:
			orientSelect.SetSelected("Axial")
		case Coronal:
			orientSelect.SetSelected("Coronal")
		case Sagittal:
			orientSelect.SetSelected("Sagittal")
		}
		// Restore the saved frame after orientation selection
		v.setFrame(savedFrame)
		sliceSlider.SetValue(float64(savedFrame))
	}

	// Window/Level
	wlEntry := widget.NewEntry()
	wlEntry.SetText(fmt.Sprintf("%d", v.dicox.WindowLevel()))
	wlEntry.OnChanged = func(s string) {
		val, _ := strconv.Atoi(s)
		v.dicox.SetWindowLevel(val)
		v.image.Refresh()
	}
	v.level = wlEntry

	wwEntry := widget.NewEntry()
	wwEntry.SetText(fmt.Sprintf("%d", v.dicox.WindowWidth()))
	wwEntry.OnChanged = func(s string) {
		val, _ := strconv.Atoi(s)
		v.dicox.SetWindowWidth(val)
		v.image.Refresh()
	}
	v.width = wwEntry

	// Bottom Control Bar
	// 3D Controls (Left Side)
	// Multi-Handle Slider for Material Thresholds
	// Supports variable number of bands from config
	materialSlider := NewMultiRangeSliderFromConfig(volume.GetConfig(), "DEFAULT")

	materialSlider.OnChanged = func(thresholds []int) {
		if v.volumeRenderer != nil {
			// First threshold controls density cutoff (Air band)
			if len(thresholds) > 0 && len(materialSlider.Bands) > 0 && materialSlider.Bands[0].IsTransparent {
				// Convert absolute density to normalized 0-1 for SetDensityThreshold
				normalized := float64(thresholds[0]) / materialSlider.Max
				v.volumeRenderer.SetDensityThreshold(normalized)
			}
			// Use the bands-based API for material thresholds
			v.volumeRenderer.SetMaterialBands(materialSlider.Bands)
			v.volumeRenderer.Render()
			v.volumeRenderer.Refresh()
		}
		// Sync transfer function to 2D composite view
		if v.dicox != nil {
			cfg := volume.GetConfig()
			tf := volume.CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", materialSlider.Bands)
			v.dicox.SetTransferFunction(tf)
			if v.image != nil {
				v.image.Refresh()
			}
		}
	}

	// Additional Global Controls
	// Quality
	qualitySelect := widget.NewSelect([]string{"Fast", "Medium", "High"}, func(selected string) {
		switch selected {
		case "Fast":
			v.volumeRenderer.SetQuality(0)
		case "Medium":
			v.volumeRenderer.SetQuality(1)
		case "High":
			v.volumeRenderer.SetQuality(2)
		}
		v.volumeRenderer.Render()
		v.volumeRenderer.Refresh()
	})
	qualitySelect.SetSelected("High")

	// Preset
	presetSelect := widget.NewSelect([]string{"DEFAULT", "THREAT", "MONOCHROME", "LAPTOP_REMOVAL"}, func(selected string) {
		v.volumeRenderer.SetColorOpacityPreset(selected)
		v.volumeRenderer.Render()
		v.volumeRenderer.Refresh()
	})
	presetSelect.SetSelected("DEFAULT")

	// Opacity (shared between 3D and 2D views)
	opacitySlider := NewLabeledSlider(0.0, 1.0)
	opacitySlider.Slider.Step = 0.01
	opacitySlider.SetOnChanged(func(val float64) {
		// Update 3D view
		v.volumeRenderer.SetAlphaScale(val)
		v.volumeRenderer.Render()
		v.volumeRenderer.Refresh()
		// Sync to 2D composite view
		if v.dicox != nil {
			v.dicox.SetAlphaScale(val)
			if v.image != nil {
				v.image.Refresh()
			}
		}
	})
	opacitySlider.SetValue(0.5)

	// Z Scale slider for 3D aspect ratio adjustment
	v.zScaleSlider = NewLabeledSlider(0.1, 5.0)
	v.zScaleSlider.Slider.Step = 0.05
	v.zScaleSlider.SetOnChanged(func(val float64) {
		v.currentScaleZ = val
		v.volumeRenderer.SetScaleZ(val)
		v.volumeRenderer.Render()
		v.volumeRenderer.Refresh()
	})
	v.zScaleSlider.SetValue(1.0) // Default to 1.0, will be updated when data loads
	v.currentScaleZ = 1.0

	// Per-band opacity callback
	materialSlider.OnOpacityChanged = func(bandIdx int, alpha float64) {
		// Re-render with updated band alphas
		if v.volumeRenderer != nil {
			v.volumeRenderer.SetMaterialBands(materialSlider.Bands)
			v.volumeRenderer.Render()
			v.volumeRenderer.Refresh()
		}
		// Sync to 2D
		if v.dicox != nil {
			cfg := volume.GetConfig()
			tf := volume.CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", materialSlider.Bands)
			v.dicox.SetTransferFunction(tf)
			if v.image != nil {
				v.image.Refresh()
			}
		}
	}

	// Per-band opacity sliders (expandable section)
	opacityControls := container.NewVBox()
	for i, band := range materialSlider.Bands {
		bandIdx := i // capture for closure
		bandName := band.Name

		slider := widget.NewSlider(0.0, 2.0)
		slider.Step = 0.05
		slider.Value = materialSlider.GetBandAlpha(bandIdx)

		valLabel := widget.NewLabel(fmt.Sprintf("%.0f%%", slider.Value*100))

		slider.OnChanged = func(val float64) {
			materialSlider.SetBandAlpha(bandIdx, val)
			valLabel.SetText(fmt.Sprintf("%.0f%%", val*100))
		}

		row := container.NewBorder(nil, nil, widget.NewLabel(bandName+":"), valLabel, slider)
		opacityControls.Add(row)
	}

	// Wrap in accordion for expandable UI
	opacityAccordion := widget.NewAccordion(
		widget.NewAccordionItem("Band Opacity", opacityControls),
	)

	// Trigger initial material slider callback AFTER preset selection
	// to ensure bands-based transfer function isn't overwritten by preset
	materialSlider.OnChanged(materialSlider.GetThresholds())

	// BG Color Picker
	bgColorBtn := widget.NewButton("BG Color", func() {
		cfg := volume.GetConfig()

		// Create Sliders
		rSlider := NewLabeledSlider(0, 255)
		gSlider := NewLabeledSlider(0, 255)
		bSlider := NewLabeledSlider(0, 255)

		// Set Initial Values
		rSlider.SetValue(float64(cfg.BackgroundColor[0]))
		gSlider.SetValue(float64(cfg.BackgroundColor[1]))
		bSlider.SetValue(float64(cfg.BackgroundColor[2]))

		// Update Function
		updateColor := func() {
			r := uint8(rSlider.Slider.Value)
			g := uint8(gSlider.Slider.Value)
			b := uint8(bSlider.Slider.Value)
			c := color.RGBA{R: r, G: g, B: b, A: 255}

			v.volumeRenderer.SetBackgroundColor(c)
		}

		// Bind Callbacks
		rSlider.SetOnChanged(func(f float64) { updateColor() })
		gSlider.SetOnChanged(func(f float64) { updateColor() })
		bSlider.SetOnChanged(func(f float64) { updateColor() })

		// Layout
		content := container.NewVBox(
			container.NewBorder(nil, nil, widget.NewLabel("Red"), nil, rSlider),
			container.NewBorder(nil, nil, widget.NewLabel("Green"), nil, gSlider),
			container.NewBorder(nil, nil, widget.NewLabel("Blue"), nil, bSlider),
		)

		d := dialog.NewCustom("Background Color", "Done", content, v.win)
		d.Show()
	})

	// Organize Global Controls - Stacked Vertically for room
	// Note: Threshold slider removed - now controlled by first handle in materialSlider
	globalControls := container.NewVBox(
		container.NewBorder(nil, nil, widget.NewLabel("Quality:"), nil, qualitySelect),
		container.NewBorder(nil, nil, widget.NewLabel("Preset:"), nil, presetSelect),
		container.NewBorder(nil, nil, widget.NewLabel("Opacity:"), nil, opacitySlider),
		container.NewBorder(nil, nil, widget.NewLabel("Z Scale:"), nil, v.zScaleSlider),
		container.NewHBox(layout.NewSpacer(), bgColorBtn),
	)

	// Metadata & Layers
	accordion := func() fyne.CanvasObject {
		var img *canvas.Image = v.image
		var _ *canvas.Image = img
		return v.setupForm()
	}()

	// Combine All 3D Controls
	// Order: MaterialSlider -> Band Opacity (expandable) -> Separator -> Global Controls
	bottom3D := container.NewVBox(
		materialSlider,
		opacityAccordion,
		widget.NewSeparator(),
		globalControls,
	)

	// Wrap 3D view
	panel3D := container.NewBorder(nil, bottom3D, nil, nil, v.volumeRenderer)

	// 2D Controls (Right Side)
	// Stacked layout to prevent bunching

	// Volume Selection
	volRow := container.NewBorder(nil, nil, widget.NewLabel("Volume:"), nil, volumeSelect)

	// View Orientation
	viewRow := container.NewBorder(nil, nil, widget.NewLabel("View:"), nil, orientSelect)

	// Composite View Toggle (MIP projection through all slices)
	compositeCheck := widget.NewCheck("Composite View", func(b bool) {
		v.dicox.SetCompositeMode(b)
		// Hide/show slice slider based on mode
		if b {
			sliceControl.Hide()
		} else {
			sliceControl.Show()
		}
		v.image.Refresh()
	})
	compositeCheck.Checked = true // Default to composite enabled
	v.dicox.SetCompositeMode(true)
	sliceControl.Hide() // Start hidden since composite is default
	v.image.Refresh()   // Ensure image renders with composite mode

	// Window/Level Inputs
	wlRow := container.NewGridWithColumns(2,
		container.NewBorder(nil, nil, widget.NewLabel("W/L:"), nil, wlEntry),
		container.NewBorder(nil, nil, widget.NewLabel("W/W:"), nil, wwEntry),
	)

	// Slice Slider
	sliceRow := container.NewBorder(nil, nil, widget.NewLabel("Slice:"), nil, sliceControl)

	// Add Finding Button
	addFindingBtn := widget.NewButton("Add Finding", func() {
		v.isAnnotating = !v.isAnnotating
		// Update button text or style?
		slog.Info("Annotation Mode Toggled", "active", v.isAnnotating)
	})

	// Combine into a vertical stack
	controls2D := container.NewVBox(
		volRow,
		viewRow,
		compositeCheck,
		wlRow,
		sliceRow,
		addFindingBtn,
	)

	// Wrap 2D view
	panel2D := container.NewBorder(nil, controls2D, nil, nil, imgWidget)

	// Side by side split - 3D panel hugs content tighter
	split := container.NewHSplit(panel3D, panel2D)
	split.SetOffset(0.55) // 55% for 3D (hugs content, still adjustable)

	// Key Handler for this view
	v.win.Canvas().SetOnTypedKey(func(e *fyne.KeyEvent) {
		switch e.Name {
		case fyne.KeyUp, fyne.KeyRight:
			if sliceSlider.Value < sliceSlider.Max {
				sliceSlider.SetValue(sliceSlider.Value + 1)
			}
		case fyne.KeyDown, fyne.KeyLeft:
			if sliceSlider.Value > sliceSlider.Min {
				sliceSlider.SetValue(sliceSlider.Value - 1)
			}
		case fyne.KeyS:
			v.exportScreenshot()
		}
	})

	// Left Panel: Metadata/Layers (Scrollable)
	leftPanel := container.NewVScroll(accordion)
	// Remove fixed MinSize width to allow auto-sizing to content
	leftPanel.SetMinSize(fyne.NewSize(0, 200)) // Min height 200, width auto

	// Main Layout: Left Panel | Split(3D|2D)
	// User requested "minimize the left pane size to the size of the layer/finding width"
	// HSplit takes a ratio. Border(Left, ...) takes the MinSize of the content.
	// Since Left Panel is VScroll(Accordion), we want it to shrink to fit the accordion content.

	// Create main content with Border layout
	// Left: leftPanel (auto-sized to content width)
	// Center: split (3D/2D views)
	mainContent := container.NewBorder(nil, nil, leftPanel, nil, split)

	return mainContent
}
func NewViewer(a fyne.App) *Viewer {
	win := a.NewWindow("DICOM Viewer")

	// Create Labels
	view := &Viewer{
		win:               win,
		patientID:         widget.NewLabel("-"),
		patientName:       widget.NewLabel("-"),
		studyDate:         widget.NewLabel("-"),
		studyTime:         widget.NewLabel("-"),
		studyDescription:  widget.NewLabel("-"),
		seriesDescription: widget.NewLabel("-"),
		modality:          widget.NewLabel("-"),
		institutionName:   widget.NewLabel("-"),
		manufacturer:      widget.NewLabel("-"),
		operatorName:      widget.NewLabel("-"),
	}
	win.SetMainMenu(view.makeMenu())

	// Initialize dicox with default values. These will be updated when a file is loaded.
	view.dicox = NewDICOXImage(nil, 0, 0, 40, 400)

	img := canvas.NewImageFromImage(view.dicox)
	img.FillMode = canvas.ImageFillContain
	view.image = img // Assign the image to the viewer

	// Create 3D renderer as main view
	vr := NewVolumeRenderer()
	vr.SetWindowLevel(40)
	vr.SetWindowWidth(380)
	view.volumeRenderer = vr

	// Placeholder for 3D view until data is loaded
	placeholder := widget.NewLabel("Open a DICOM file or folder to view 3D volume")
	placeholder.Alignment = fyne.TextAlignCenter

	// Main content area - will be updated when data loads
	mainContent := container.NewStack(placeholder)
	view.mainContent = mainContent
	view.win.SetContent(view.mainContent)

	// Initialize info labels for 2D view (setupForm creates them)
	// We call it but don't add to main view - these are used in 2D popup
	_ = view.setupForm()

	// Size window to reasonable default for side-by-side (20% wider for better fit)
	win.Resize(fyne.NewSize(1920, 1000))

	return view
}

// makeMenu creates the main application menu
func (v *Viewer) makeMenu() *fyne.MainMenu {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Open File...", v.openFile),
		fyne.NewMenuItem("Open Folder...", v.openFolder),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Export Screenshot", v.exportScreenshot),
	)

	viewMenu := fyne.NewMenu("View",

		fyne.NewMenuItem("Full Screen", v.fullScreen),
	)

	return fyne.NewMainMenu(fileMenu, viewMenu)
}

// exportScreenshot captures the current window/view and saves to disk
func (v *Viewer) exportScreenshot() {
	// For 3D view, we can get image from renderer
	if v.volumeRenderer != nil {
		// Use file save dialog
		dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
			if err != nil {
				dialog.ShowError(err, v.win)
				return
			}
			if writer == nil {
				return // User cancelled
			}
			defer writer.Close()

			// Capture
			img := v.win.Canvas().Capture()
			_ = img
			// Need real export logic
			slog.Info("Screenshot saved", "path", writer.URI().Path())
		}, v.win)
	}
}

// updateZScaleForVolume recalculates and sets the Z scale based on the selected volume's voxel spacing
func (v *Viewer) updateZScaleForVolume(volName string) {
	if v.volumeRenderer == nil {
		return
	}

	vol, ok := v.compositeVolumes[volName]
	if !ok || vol.PixelData == nil {
		return
	}

	voxelX := vol.VoxelSizeX
	voxelY := vol.VoxelSizeY
	voxelZ := vol.VoxelSizeZ

	// Default to 1.0 if not specified
	if voxelX <= 0 {
		voxelX = 1.0
	}
	if voxelY <= 0 {
		voxelY = 1.0
	}
	if voxelZ <= 0 {
		voxelZ = 1.0
	}

	depth := len(vol.PixelData.Frames)
	if depth == 0 {
		return
	}

	// Physical dimensions in mm
	physicalX := float64(vol.Cols) * voxelX
	_ = float64(vol.Rows) * voxelY // physicalY not used in scale calculation
	physicalZ := float64(depth) * voxelZ

	// Normalize Z relative to X
	scaleZ := physicalZ / physicalX
	if scaleZ < 0.1 {
		scaleZ = 0.1
	}
	if scaleZ > 10.0 {
		scaleZ = 10.0
	}

	slog.Info("Updated Z scale for volume",
		slog.String("volume", volName),
		slog.Float64("voxelX", voxelX),
		slog.Float64("voxelY", voxelY),
		slog.Float64("voxelZ", voxelZ),
		slog.Float64("scaleZ", scaleZ))

	v.volumeRenderer.SetScaleZ(scaleZ)
	v.currentScaleZ = scaleZ
	// Update the Z scale slider to reflect computed value
	if v.zScaleSlider != nil {
		v.zScaleSlider.SetValue(scaleZ)
	}
	v.volumeRenderer.Render()
	v.volumeRenderer.Refresh()
}

// Full screen toggle
func (v *Viewer) fullScreen() {
	v.win.SetFullScreen(!v.win.FullScreen())
}

func (v *Viewer) setFrame(frameIdx int) {
	v.currentFrame = frameIdx
	v.dicox.SetSliceIndex(frameIdx)
	if v.slider != nil {
		v.slider.SetValue(float64(frameIdx))
	}
	if v.frame != nil {
		v.frame.SetText(fmt.Sprintf("%d/%d", frameIdx+1, v.frameCount))
	}
	// Trigger refresh
	v.image.Refresh()
}

func (v *Viewer) openFile() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, v.win)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()

		path := reader.URI().Path()
		v.loadPath(path)
	}, v.win)
}

func (v *Viewer) openFolder() {
	dialog.ShowFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, v.win)
			return
		}
		if list == nil {
			return
		}
		path := list.Path()
		v.loadPath(path)
	}, v.win)
}

func (v *Viewer) loadPath(path string) {
	slog.Info("Loading path", "path", path)

	// Load DICOM file or directory
	scan, err := Load(path)
	if err != nil {
		dialog.ShowError(err, v.win)
		return
	}

	// Use adapter to setup viewer
	if err := v.SetScanData(scan); err != nil {
		dialog.ShowError(err, v.win)
		return
	}
}

// setupForm creates the metadata and layers sidebar
func (v *Viewer) setupForm() fyne.CanvasObject {
	// Metadata Section
	metaForm := widget.NewForm(
		widget.NewFormItem("Patient ID:", v.patientID),
		widget.NewFormItem("Patient Name:", v.patientName),
		widget.NewFormItem("Study Date:", v.studyDate),
		widget.NewFormItem("Study Time:", v.studyTime),
		widget.NewFormItem("Study Desc:", v.studyDescription),
		widget.NewFormItem("Series Desc:", v.seriesDescription),
		widget.NewFormItem("Modality:", v.modality),
		widget.NewFormItem("Institution:", v.institutionName),
		widget.NewFormItem("Manufacturer:", v.manufacturer),
		widget.NewFormItem("Operator:", v.operatorName),
	)

	// Layers / Findings Section
	// We need a container that we can populate dynamically when data loads
	v.layersContainer = container.NewVBox()
	v.findingsContainer = container.NewVBox()

	v.accordion = widget.NewAccordion(
		widget.NewAccordionItem("Metadata", metaForm),
		widget.NewAccordionItem("Layers", v.layersContainer),
		widget.NewAccordionItem("Findings", v.findingsContainer),
	)
	v.accordion.MultiOpen = true

	return v.accordion
}

// UpdateLayers repopulates the layers list
func (v *Viewer) UpdateLayers() {
	if v.layersContainer == nil || v.findingsContainer == nil {
		return
	}
	v.layersContainer.Objects = nil
	v.findingsContainer.Objects = nil

	// Sort keys
	var volKeys []string
	if len(v.compositeVolumes) > 0 {
		for k := range v.compositeVolumes {
			volKeys = append(volKeys, k)
		}
		sort.Strings(volKeys)
	}

	// Palette for finding colors (same as setup3DView)
	findingPalette := []color.RGBA{
		{255, 0, 0, 255},     // Red
		{255, 128, 0, 255},   // Orange
		{255, 0, 255, 255},   // Magenta
		{0, 255, 255, 255},   // Cyan
		{255, 255, 0, 255},   // Yellow
		{128, 0, 255, 255},   // Purple
		{0, 255, 128, 255},   // Teal
		{255, 128, 128, 255}, // Light Red
	}

	// Collect finding keys for "All Findings" toggle
	var findingKeys []string
	findingIdx := 0
	for _, k := range volKeys {
		if strings.Contains(strings.ToLower(k), "finding") {
			findingKeys = append(findingKeys, k)
			// Assign color if not already set
			vol := v.compositeVolumes[k]
			if vol.Color.A == 0 {
				vol.Color = findingPalette[findingIdx%len(findingPalette)]
			}
			findingIdx++
		}
	}

	// Add "All Findings" toggle if there are any findings
	if len(findingKeys) > 0 {
		// Check if all findings are enabled
		allEnabled := true
		for _, k := range findingKeys {
			if !v.compositeVolumes[k].Enabled {
				allEnabled = false
				break
			}
		}

		allFindingsCheck := widget.NewCheck("All Findings", func(b bool) {
			for _, k := range findingKeys {
				v.compositeVolumes[k].Enabled = b
			}
			slog.Info("All findings toggled", "enabled", b)
			v.setup3DView()
			v.UpdateLayers() // Refresh UI to update individual checkboxes
		})
		allFindingsCheck.Checked = allEnabled

		// Add separator and "All Findings" at the top
		v.findingsContainer.Add(allFindingsCheck)
		v.findingsContainer.Add(widget.NewSeparator())
	}

	for _, k := range volKeys {
		vol := v.compositeVolumes[k]
		name := k // capture for closure

		// Build tooltip text with source path info
		tooltipText := name
		if vol.SourcePath != "" {
			tooltipText = fmt.Sprintf("%s\nSource: %s", name, vol.SourcePath)
		}
		// Add voxel spacing info
		if vol.VoxelSizeX > 0 || vol.VoxelSizeY > 0 || vol.VoxelSizeZ > 0 {
			tooltipText += fmt.Sprintf("\nVoxel: %.3f x %.3f x %.3f mm", vol.VoxelSizeX, vol.VoxelSizeY, vol.VoxelSizeZ)
		}

		// Determine Category
		isFinding := strings.Contains(strings.ToLower(k), "finding")

		if isFinding {
			// Create colored indicator rectangle
			colorRect := canvas.NewRectangle(vol.Color)
			colorRect.SetMinSize(fyne.NewSize(16, 16))
			colorRect.CornerRadius = 2

			// Checkbox for Visibility (without the full name to save space)
			displayName := strings.TrimPrefix(name, "finding_")
			check := widget.NewCheck(displayName, func(b bool) {
				vol.Enabled = b
				slog.Info("Finding visibility toggled", "finding", name, "visible", b)
				v.setup3DView()
			})
			check.Checked = vol.Enabled

			// Slider for Opacity
			slider := widget.NewSlider(0.0, 1.0)
			slider.Value = vol.Alpha
			slider.Step = 0.05

			slider.OnChanged = func(val float64) {
				vol.Alpha = val
				v.setup3DView()
			}

			// Row: [ColorRect] [Check] [Slider]
			row := container.NewBorder(nil, nil, container.NewHBox(colorRect, check), nil, slider)
			v.findingsContainer.Add(row)
		} else {
			// Regular layer (non-finding)
			// Create info button that shows volume details on tap
			infoBtn := widget.NewButton("ℹ", nil)
			infoBtn.Importance = widget.LowImportance
			infoBtn.OnTapped = func() {
				dialog.ShowInformation("Volume Info", tooltipText, v.win)
			}

			// Checkbox for Visibility
			check := widget.NewCheck(name, func(b bool) {
				vol.Enabled = b
				slog.Info("Layer visibility toggled", "layer", name, "visible", b)
				v.setup3DView()
			})
			check.Checked = vol.Enabled

			// Slider for Opacity
			slider := widget.NewSlider(0.0, 1.0)
			slider.Value = vol.Alpha
			slider.Step = 0.05

			valLabel := widget.NewLabel(fmt.Sprintf("%.2f", vol.Alpha))

			slider.OnChanged = func(val float64) {
				vol.Alpha = val
				valLabel.SetText(fmt.Sprintf("%.2f", val))
				v.setup3DView()
			}

			// Compact row: [Info] [Check] [Slider] [Value]
			row := container.NewBorder(nil, nil, container.NewHBox(infoBtn, check), valLabel, slider)
			v.layersContainer.Add(row)
		}
	}
	v.layersContainer.Refresh()
	v.findingsContainer.Refresh()

	// Open Findings accordion by default if there are any findings
	if v.accordion != nil && len(findingKeys) > 0 {
		v.accordion.Open(2) // Index 2 = Findings (0=Metadata, 1=Layers, 2=Findings)
	}
}

// updateLayout refreshes the main window content
func (v *Viewer) updateLayout() {
	// Called by SetScanData after loading
	// We want to ensure the side-by-side panel is active

	panel := v.createSideBySidePanel()
	v.mainContent.Objects = []fyne.CanvasObject{panel}

	// Refresh layers list AFTER creating the panel (because createSideBySidePanel re-initializes layout containers)
	v.UpdateLayers()

	v.mainContent.Refresh()

	// Force a final render after panel is fully attached to ensure
	// material slider settings are applied to the display
	if v.volumeRenderer != nil {
		v.volumeRenderer.Render()
		v.volumeRenderer.Refresh()
	}

	// Ensure 2D image is refreshed with composite mode
	if v.image != nil {
		v.image.Refresh()
	}
}

// setup3DView prepares the 3D volume renderer
func (v *Viewer) setup3DView() {
	if v.volumeRenderer == nil {
		return
	}

	v.volumeRenderer.ClearVolumes()
	v.volumeRenderer.ClearFindings()

	// Sort keys for deterministic layering
	var keys []string
	for k := range v.compositeVolumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Palette for finding colors
	findingPalette := []color.RGBA{
		{255, 0, 0, 255},     // Red
		{255, 128, 0, 255},   // Orange
		{255, 0, 255, 255},   // Magenta
		{0, 255, 255, 255},   // Cyan
		{255, 255, 0, 255},   // Yellow
		{128, 0, 255, 255},   // Purple
		{0, 255, 128, 255},   // Teal
		{255, 128, 128, 255}, // Light Red
	}
	findingIdx := 0

	// Get primary volume dimensions for bbox normalization
	// BBox coords from loader are scaled to high_res space, so prefer high_res HE volume
	var refDimX, refDimY, refDimZ int
	refPriority := []string{"he_high_res", "le_high_res", "he_low_res", "le_low_res", "he", "le"}
	for _, priority := range refPriority {
		if vol, ok := v.compositeVolumes[priority]; ok && vol.PixelData != nil && len(vol.PixelData.Frames) > 0 {
			refDimX = vol.Cols
			refDimY = vol.Rows
			refDimZ = len(vol.PixelData.Frames)
			slog.Info("Using reference volume for bbox", "name", priority, "dims", []int{refDimX, refDimY, refDimZ})
			break
		}
	}
	// Fallback to first non-finding volume if priority list didn't match
	if refDimX == 0 {
		for _, k := range keys {
			vol := v.compositeVolumes[k]
			if !strings.Contains(strings.ToLower(k), "finding") && vol.PixelData != nil && len(vol.PixelData.Frames) > 0 {
				refDimX = vol.Cols
				refDimY = vol.Rows
				refDimZ = len(vol.PixelData.Frames)
				slog.Info("Using fallback reference volume for bbox", "name", k, "dims", []int{refDimX, refDimY, refDimZ})
				break
			}
		}
	}

	for _, k := range keys {
		vol := v.compositeVolumes[k]
		isFinding := strings.Contains(strings.ToLower(k), "finding")

		if vol.Enabled {
			if isFinding && vol.BBox != nil {
				// Assign color from palette if not already set
				if vol.Color.A == 0 {
					vol.Color = findingPalette[findingIdx%len(findingPalette)]
				}
				findingIdx++

				// Add finding bounding box to renderer
				v.volumeRenderer.AddFinding(
					k,
					0, // classID
					*vol.BBox,
					vol.Color,
					refDimX,
					refDimY,
					refDimZ,
				)
				slog.Info("Added finding bbox to renderer", "name", k, "bbox", vol.BBox, "color", vol.Color)
			} else {
				// Add regular volume
				_ = v.volumeRenderer.AddVolume(
					vol.PixelData,
					vol.Rows,
					vol.Cols,
					vol.PixelRep,
					vol.Color,
					vol.WindowCenter,
					vol.WindowWidth,
					vol.RescaleIntercept,
					vol.Alpha,
				)
			}
		}
	}

	v.volumeRenderer.Render()
	v.volumeRenderer.Refresh()

	// Initialize transfer function and alpha scale for 2D composite view
	if v.dicox != nil {
		cfg := volume.GetConfig()
		bands := volume.GetColorBandsFromConfig(cfg, "DEFAULT")
		tf := volume.CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", bands)
		v.dicox.SetTransferFunction(tf)
		// Sync alpha scale from 3D renderer (default 0.5 from opacity slider)
		v.dicox.SetAlphaScale(v.volumeRenderer.GetAlphaScale())
	}
}

// Run starts the application event loop
func (v *Viewer) Run() {
	v.win.ShowAndRun()
}

// LoadKeys initializes keyboard shortcuts (No-op as keys are handled in panel creation)
func (v *Viewer) LoadKeys() {
	// Key handlers are set in createSideBySidePanel for specific views
}
