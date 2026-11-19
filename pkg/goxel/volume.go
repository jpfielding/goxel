package goxel

import (
	"image"
	"image/color"
	"log/slog"
	"math"
	"sort"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
	"github.com/jpfielding/goxel/pkg/volume"
)

// VolumeRenderer is a Fyne widget that wraps the GPU volume renderer
// with fallback to CPU rendering if GPU is unavailable
type VolumeRenderer struct {
	widget.BaseWidget

	// GPU renderer (primary)
	gpuRenderer *volume.GPUVolumeRenderer

	// CPU renderer (fallback)
	cpuRenderer *volume.CPUVolumeRenderer

	// Which renderer is active
	useGPU bool

	// Rendered image and canvas
	img *canvas.Image

	// State
	mu sync.RWMutex

	// Dimensions for BBox projection
	dimX, dimY, dimZ int

	// Local findings list for GPU rendering overlay
	findings []volume.FindingInfo

	Align AlignParams

	// Physical box: X is 1, Y and Z are relative to it.
	scaleY, scaleZ float64

	// viewAspect is the widget's width over height: the render fills the
	// widget, so the camera fits the box to this aspect.
	viewAspect float64
	// maxZoom is how far the camera may back off (grows with the fitted box).
	maxZoom float64

	// Channel-based render backpressure (buffer size 1)
	// Non-blocking send drops frames when GPU is busy
	renderChan chan struct{}

	// Ensures Destroy is only called once
	destroyOnce sync.Once

	// DragPans, when set and true, makes a plain drag pan instead of orbit
	// (the compact layouts' Orbit | Pan toggle).
	DragPans func() bool

	// Overlay, when set, returns the slice-plane outline (corners in the
	// renderer's box space, [0,1]x[0,1]x[0,scaleZ]; nil for none) and the
	// corner readout ("" for none). Drawn on the GPU path only.
	Overlay func() (corners [][3]float64, readout string)
}

type AlignParams struct {
	FlipX, FlipY, FlipZ bool
	FlipRotX, FlipRotY  bool
	OffX, OffY, OffZ    float64
}

// NewVolumeRenderer creates a new volume renderer, attempting GPU first
func NewVolumeRenderer() *VolumeRenderer {
	vr := &VolumeRenderer{
		scaleY:     1,
		scaleZ:     1,
		viewAspect: 1,
		maxZoom:    5,
		renderChan: make(chan struct{}, 1), // Buffer size 1 for backpressure
		Align: AlignParams{
			FlipX:    true,
			FlipY:    true,
			FlipZ:    true,
			FlipRotX: true,
		},
	}
	vr.ExtendBaseWidget(vr)

	// Try to initialize the GPU renderer. The browser reads every frame back
	// through readPixels, so it renders at a lower resolution.
	size := 1024
	if fyne.CurrentDevice().IsBrowser() {
		size = 768
	}
	gpuRenderer, err := volume.NewGPUVolumeRenderer(size, size)
	if err != nil {
		slog.Warn("GPU renderer unavailable, falling back to CPU", slog.Any("error", err))
		vr.cpuRenderer = volume.NewCPUVolumeRenderer()
		vr.useGPU = false
	} else {
		slog.Info("GPU volume renderer initialized successfully")
		vr.gpuRenderer = gpuRenderer
		vr.useGPU = true
		gpuRenderer.SetRotation(defaultAzimuth, defaultElevation)

		// Start render goroutine for channel-based backpressure
		go vr.renderLoop()
	}

	return vr
}

// renderLoop consumes from renderChan and renders on background thread
func (vr *VolumeRenderer) renderLoop() {
	for range vr.renderChan {
		if vr.gpuRenderer != nil {
			vr.mu.Lock()
			vr.gpuRenderer.Render() // Blocking render on background thread
			vr.mu.Unlock()
			fyne.Do(func() {
				vr.Refresh()
			})
		}
	}
}

// SetWindowLevel sets the window level for CT visualization
func (vr *VolumeRenderer) SetWindowLevel(level float64) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetWindowLevel(level)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetWindowLevel(level)
	}
}

// SetWindowWidth sets the window width for CT visualization
func (vr *VolumeRenderer) SetWindowWidth(width float64) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetWindowWidth(width)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetWindowWidth(width)
	}
}

// SetTransferFunctionType sets the transfer function type
func (vr *VolumeRenderer) SetTransferFunctionType(tfType volume.TransferFunctionType) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetTransferFunction(tfType)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetTransferFunctionType(tfType)
	}
}

// SetColorOpacityPreset sets the transfer function from a named preset
// Valid presets: "DEFAULT", "FINDING", "MONOCHROME", "LAPTOP_REMOVAL"
func (vr *VolumeRenderer) SetColorOpacityPreset(presetName string) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetColorOpacityPreset(presetName)
	}
	// Note: CPU renderer doesn't support config-based presets yet
}

// SetRescaleIntercept sets the rescale intercept (offset applied to raw pixel values)
func (vr *VolumeRenderer) SetRescaleIntercept(intercept float64) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetRescaleIntercept(intercept)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetRescaleIntercept(intercept)
	}
}

// TriggerRender schedules a render
func (vr *VolumeRenderer) TriggerRender() {
	select {
	case vr.renderChan <- struct{}{}:
	default:
		// Channel full, render already pending
	}
}

// LoadVolume loads frame data into the volume
func (vr *VolumeRenderer) LoadVolume(pd *PixelData, imageRows, imageCols int, pixelRep int) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if pd != nil {
		vr.dimZ = len(pd.Frames)
	}
	vr.dimX = imageCols
	vr.dimY = imageRows

	var err error
	if vr.useGPU && vr.gpuRenderer != nil {
		// Use default white tint for single volume load
		// Default CT Window: WL=40, WW=400, Intercept=0, Alpha=1.0
		err = vr.gpuRenderer.AddVolume(pd, imageRows, imageCols, pixelRep, color.RGBA{255, 255, 255, 255}, 40.0, 400.0, 0.0, 1.0)
	} else if vr.cpuRenderer != nil {
		err = vr.cpuRenderer.LoadVolume(pd, imageRows, imageCols, pixelRep)
	}

	if err == nil {
		vr.TriggerRender()
	}
	return err
}

// AddVolume adds a volume to the renderer
func (vr *VolumeRenderer) AddVolume(pd *PixelData, imageRows, imageCols int, pixelRep int, tint color.RGBA, wl, ww, intercept, alphaScale float64) error {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	var err error
	if vr.useGPU && vr.gpuRenderer != nil {
		err = vr.gpuRenderer.AddVolume(pd, imageRows, imageCols, pixelRep, tint, wl, ww, intercept, alphaScale)
	} else if vr.cpuRenderer != nil {
		// CPU renderer only supports single volume via LoadVolume for now
		err = vr.cpuRenderer.LoadVolume(pd, imageRows, imageCols, pixelRep)
	}

	if err == nil {
		vr.TriggerRender()
	}
	return err
}

// AddVolumeFromData adds raw volume data
func (vr *VolumeRenderer) AddVolumeFromData(data []uint16, dimX, dimY, dimZ int, tint color.RGBA, wl, ww, intercept, alphaScale float64) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.AddVolumeFromData(data, dimX, dimY, dimZ, tint, wl, ww, intercept, alphaScale)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.LoadVolumeFromData(data, dimX, dimY, dimZ)
	}
}

// LoadSecondaryVolume loads the secondary (LE) volume for dual-energy compositing
func (vr *VolumeRenderer) LoadSecondaryVolume(pd *PixelData, imageRows, imageCols int, pixelRep int) error {
	if vr.cpuRenderer != nil {
		return vr.cpuRenderer.LoadSecondaryVolume(pd, imageRows, imageCols, pixelRep)
	}
	// GPU renderer doesn't support dual-energy yet
	return nil
}

// SetCompositeMode enables or disables HE/LE ratio-based material colorization
func (vr *VolumeRenderer) SetCompositeMode(enabled bool) {
	if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetCompositeMode(enabled)
	}
	// GPU renderer doesn't support composite mode yet
}

// HasSecondaryVolume returns true if a secondary volume is loaded
func (vr *VolumeRenderer) HasSecondaryVolume() bool {
	if vr.cpuRenderer != nil {
		return vr.cpuRenderer.HasSecondaryVolume()
	}
	return false
}

// ClearVolumes clears loaded volumes
func (vr *VolumeRenderer) ClearVolumes() {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.ClearVolumes()
	} else if vr.cpuRenderer != nil {
		vr.dimZ = 0
	}
}

// LoadFindingVolume loads a finding overlay volume for visualization
func (vr *VolumeRenderer) LoadFindingVolume(pd *PixelData, imageRows, imageCols int, pixelRep int) error {
	if vr.cpuRenderer != nil {
		return vr.cpuRenderer.LoadFindingVolume(pd, imageRows, imageCols, pixelRep)
	}
	// GPU renderer doesn't support voxel-based finding visualization yet (only BBoxes)
	return nil
}

// LoadFindingVolumeFromData loads a finding volume from raw voxel data
func (vr *VolumeRenderer) LoadFindingVolumeFromData(data []uint16, dimX, dimY, dimZ int) {
	if vr.cpuRenderer != nil {
		vr.cpuRenderer.LoadFindingVolumeFromData(data, dimX, dimY, dimZ)
	}
}

// SetFindingMode sets the finding visualization mode
func (vr *VolumeRenderer) SetFindingMode(mode volume.FindingMode) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetFindingMode(int(mode))
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetFindingMode(mode)
	}
}

// AddFinding adds a finding detection box
// refDimX/Y/Z are the volume dimensions the bbox coordinates are relative to
func (vr *VolumeRenderer) AddFinding(name string, classID int, bbox volume.BoundingBox3D, c color.RGBA, refDimX, refDimY, refDimZ int) {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	// Add to GPU renderer
	// DNA: Disabled GPU lines to avoid Z-fighting and state pollution. Using CPU overlay.
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.AddFinding(name, classID, bbox, c, refDimX, refDimY, refDimZ)
	}

	// Internal list for UI state
	vr.findings = append(vr.findings, volume.FindingInfo{
		Name:    name,
		Type:    classID,
		BBox:    bbox,
		Color:   c,
		Visible: true,
	})

	slog.Info("VolumeRenderer.AddFinding", "name", name, "gpu", vr.useGPU, "refDims", []int{refDimX, refDimY, refDimZ})

	// Add to CPU renderer if active
	if vr.cpuRenderer != nil {
		vr.cpuRenderer.AddFinding(name, classID, bbox, c)
	}
}

// ClearFindings removes all finding bounding boxes
func (vr *VolumeRenderer) ClearFindings() {
	vr.mu.Lock()
	vr.findings = nil
	vr.mu.Unlock()

	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.ClearFindings()
	}

	if vr.cpuRenderer != nil {
		vr.cpuRenderer.ClearFindings()
	}
}

// GetFindings returns a copy of findings for UI display
func (vr *VolumeRenderer) GetFindings() []volume.FindingInfo {
	if vr.cpuRenderer != nil {
		return vr.cpuRenderer.GetFindings()
	}
	return nil
}

// SetFindingVisible sets visibility of a specific finding by index
func (vr *VolumeRenderer) SetFindingVisible(index int, visible bool) {
	if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetFindingVisible(index, visible)
	}
}

// Render performs ray casting
func (vr *VolumeRenderer) Render() {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.Render()
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.Render()
	}
}

// GetAlphaScale returns current alpha scale
func (vr *VolumeRenderer) GetAlphaScale() float64 {
	if vr.useGPU && vr.gpuRenderer != nil {
		return vr.gpuRenderer.GetAlphaScale()
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.Mu.RLock()
		defer vr.cpuRenderer.Mu.RUnlock()
		return vr.cpuRenderer.AlphaScale
	}
	return 0.15
}

// SetAlphaScale sets the opacity scale
func (vr *VolumeRenderer) SetAlphaScale(alpha float64) {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetAlphaScale(alpha)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.AlphaScale = alpha
		vr.cpuRenderer.NeedsRender = true
	}
	// We call TriggerRender outside lock? No, TriggerRender is async (channel send).
	// But defer covers it.
	vr.TriggerRender()
}

// SetVolumeAlpha sets the opacity scale for a specific volume index
func (vr *VolumeRenderer) SetVolumeAlpha(index int, alpha float64) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetVolumeAlpha(index, alpha)
	}
}

// SetDensityThreshold sets the minimum density threshold (0-1 normalized)
// SetDensityThreshold sets the minimum density threshold (0-1 normalized)
// Voxels below this threshold are clipped (invisible)
func (vr *VolumeRenderer) SetDensityThreshold(threshold float64) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetDensityThreshold(threshold)
	}
	vr.TriggerRender()
}

// GetDensityThreshold returns the current density threshold
func (vr *VolumeRenderer) GetDensityThreshold() float64 {
	if vr.useGPU && vr.gpuRenderer != nil {
		return vr.gpuRenderer.GetDensityThreshold()
	}
	return 0.05 // Default
}

// SetLighting sets the lighting parameters (ambient, diffuse, specular)
func (vr *VolumeRenderer) SetLighting(ambient, diffuse, specular float64) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetLighting(ambient, diffuse, specular)
	}
}

// SetClippingPlane sets the clipping plane for the renderer
// Only supported by GPU renderer currently
func (vr *VolumeRenderer) SetClippingPlane(nx, ny, nz, d float32, enable bool) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetClippingPlane(nx, ny, nz, d, enable)
	}
}

// SetMaterialThresholds updates the transfer function boundaries
func (vr *VolumeRenderer) SetMaterialThresholds(t1, t2 int) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetMaterialThresholds(t1, t2)
	}
	vr.TriggerRender()
}

// SetMaterialBands updates the transfer function using variable color bands.
func (vr *VolumeRenderer) SetMaterialBands(bands []volume.ColorBand) {
	vr.mu.Lock()
	defer vr.mu.Unlock()
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetMaterialBands(bands)
	}
	vr.TriggerRender()
}

// GetLighting returns the current lighting parameters
func (vr *VolumeRenderer) GetLighting() (ambient, diffuse, specular float64) {
	if vr.useGPU && vr.gpuRenderer != nil {
		return vr.gpuRenderer.GetLighting()
	}
	return 0.7, 0.5, 0.75 // SIAT-lib defaults
}

// SetBoxScale sets the physical proportions of the volume box: Y and Z
// extents relative to X.
func (vr *VolumeRenderer) SetBoxScale(y, z float64) {
	vr.mu.Lock()
	defer vr.mu.Unlock() // Extend lock scope to cover GPU call
	vr.scaleY, vr.scaleZ = y, z

	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetScaleY(y)
		vr.gpuRenderer.SetScaleZ(z)
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetScaleY(y)
		vr.cpuRenderer.SetScaleZ(z)
	}
	// Trigger update
	fyne.Do(func() { vr.Refresh() })
}

// CreateRenderer creates a Fyne widget renderer
func (vr *VolumeRenderer) CreateRenderer() fyne.WidgetRenderer {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.img = canvas.NewImageFromImage(vr.gpuRenderer.GetRendered())
	} else if vr.cpuRenderer != nil {
		vr.img = canvas.NewImageFromImage(vr.cpuRenderer.Rendered)
	}

	vr.img.FillMode = canvas.ImageFillStretch // Manual layout
	vr.img.ScaleMode = canvas.ImageScaleFastest
	if fyne.CurrentDevice().IsBrowser() {
		// The browser driver has no ImageScaleFastest ("Invalid canvas.ImageScale
		// value"), and ImageScaleSmooth rescales every frame on the CPU with
		// CatmullRom. ImageScalePixels hands the frame straight to the GPU.
		vr.img.ScaleMode = canvas.ImageScalePixels
	}

	r := &volumeWidgetRenderer{
		vr:      vr,
		img:     vr.img,
		readout: canvas.NewText("", colAccent),
		swatch:  canvas.NewLine(colAccent),
	}
	r.readout.TextSize = 12.5
	r.readout.Alignment = fyne.TextAlignTrailing
	r.swatch.StrokeWidth = 1.5
	r.swatch.Hide()
	for i := range r.edges {
		l := canvas.NewLine(color.NRGBA{105, 211, 191, 178})
		l.StrokeWidth = 1
		l.Hide()
		r.edges[i] = l
	}
	for i := range r.axes {
		r.axes[i] = canvas.NewLine(gizmoColors[i])
		r.axes[i].StrokeWidth = 1.5
		r.axes[i].Hide()
		r.letters[i] = canvas.NewText("", gizmoColors[i])
		r.letters[i].TextSize = 12
	}
	return r
}

// isBackgroundAt checks whether the given widget-relative position is over
// the background (empty space) by sampling the last rendered image.
func (vr *VolumeRenderer) isBackgroundAt(pos fyne.Position) bool {
	if vr.img == nil || vr.img.Image == nil {
		return false
	}
	rendered, ok := vr.img.Image.(*image.RGBA)
	if !ok {
		return false
	}
	bounds := rendered.Bounds()
	shown, at := vr.img.Size(), vr.img.Position()
	if shown.Width <= 0 || shown.Height <= 0 {
		return false
	}
	// Map widget coordinates to image coordinates (the image is centred).
	imgX := int(float64(pos.X-at.X) / float64(shown.Width) * float64(bounds.Dx()))
	imgY := int(float64(pos.Y-at.Y) / float64(shown.Height) * float64(bounds.Dy()))
	if imgX < bounds.Min.X || imgX >= bounds.Max.X || imgY < bounds.Min.Y || imgY >= bounds.Max.Y {
		return true // Out of bounds = background
	}
	c := rendered.RGBAAt(imgX, imgY)
	// Background pixels have very low accumulated alpha from ray casting.
	// The GPU renderer clears to bgColor and composites on top.
	// Compare against the background color from config.
	cfg := volume.GetConfig()
	bgR := uint8(cfg.BackgroundColor[0])
	bgG := uint8(cfg.BackgroundColor[1])
	bgB := uint8(cfg.BackgroundColor[2])
	// Allow small tolerance for anti-aliasing
	dr := int(c.R) - int(bgR)
	dg := int(c.G) - int(bgG)
	db := int(c.B) - int(bgB)
	if dr < 0 {
		dr = -dr
	}
	if dg < 0 {
		dg = -dg
	}
	if db < 0 {
		db = -db
	}
	return dr < 5 && dg < 5 && db < 5
}

// Dragged handles mouse drag for rotation (on volume) or pan (on background)
func (vr *VolumeRenderer) Dragged(e *fyne.DragEvent) {
	if vr.useGPU && vr.gpuRenderer != nil {
		// Pan with Shift held (desktop), when the Pan toggle is on, or when
		// dragging on empty background (the only option without modifiers).
		if vr.wantsPan(e.Position) {
			// Pan: translate the look-at center in screen space
			zoom := vr.gpuRenderer.GetZoom()
			panScale := 0.002 * zoom // Scale pan speed by zoom distance
			vr.gpuRenderer.AdjustPan(
				-float64(e.Dragged.DX)*panScale,
				float64(e.Dragged.DY)*panScale,
			)
		} else {
			// Rotate: turntable rotation
			rotX, rotY := vr.gpuRenderer.GetRotation()

			rotX += float64(e.Dragged.DX) * 0.005
			for rotX < 0 {
				rotX += 2 * math.Pi
			}
			for rotX >= 2*math.Pi {
				rotX -= 2 * math.Pi
			}

			rotY -= float64(e.Dragged.DY) * 0.005
			for rotY < 0 {
				rotY += 2 * math.Pi
			}
			for rotY >= 2*math.Pi {
				rotY -= 2 * math.Pi
			}

			vr.gpuRenderer.SetRotation(rotX, rotY)
		}

		// Non-blocking send to render channel - drops frame if GPU busy
		select {
		case vr.renderChan <- struct{}{}:
		default:
		}
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.Dragged(e)
	}
}

// wantsPan reports whether a drag at pos should pan rather than orbit.
func (vr *VolumeRenderer) wantsPan(pos fyne.Position) bool {
	if vr.DragPans != nil && vr.DragPans() {
		return true
	}
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok && d.CurrentKeyModifiers()&fyne.KeyModifierShift != 0 {
		return true
	}
	return vr.isBackgroundAt(pos)
}

// Orbit rotates the camera by the given angles (radians).
func (vr *VolumeRenderer) Orbit(dAz, dEl float64) {
	if vr.gpuRenderer == nil {
		return
	}
	rx, ry := vr.gpuRenderer.GetRotation()
	vr.gpuRenderer.SetRotation(math.Mod(rx+dAz+2*math.Pi, 2*math.Pi), math.Mod(ry+dEl+2*math.Pi, 2*math.Pi))
	vr.TriggerRender()
}

// PanPixels pans the camera as a drag of (dx, dy) screen pixels would.
func (vr *VolumeRenderer) PanPixels(dx, dy float64) {
	if vr.gpuRenderer == nil {
		return
	}
	panScale := 0.002 * vr.gpuRenderer.GetZoom()
	vr.gpuRenderer.AdjustPan(-dx*panScale, dy*panScale)
	vr.TriggerRender()
}

// ZoomBy multiplies the camera distance by f (f < 1 zooms in).
func (vr *VolumeRenderer) ZoomBy(f float64) {
	if vr.gpuRenderer == nil {
		return
	}
	vr.gpuRenderer.SetZoom(math.Max(minZoom, math.Min(vr.maxZoom, vr.gpuRenderer.GetZoom()*f)))
	vr.TriggerRender()
}

// minZoom is the closest the camera may come to the volume's centre.
const minZoom = 0.5

// The 3D view opens from the front and above, like roxel's.
const (
	defaultAzimuth   = 0.25
	defaultElevation = -1.1
)

// shaderFov is the ray-march shader's half-angle tangent (vertical).
const shaderFov = 0.8

// fitDistance is the camera distance at which all eight corners of the box
// [1 x boxY x boxZ] lie inside a viewport of the given aspect, seen from
// (azimuth, elevation), with a 6% margin.
func fitDistance(boxY, boxZ, azimuth, elevation, aspect float64) float64 {
	sinAz, cosAz := math.Sin(azimuth), math.Cos(azimuth)
	sinEl, cosEl := math.Sin(elevation), math.Cos(elevation)
	// Viewing direction (from the camera to the centre) and the camera basis,
	// as in the shader.
	f := [3]float64{-sinAz * cosEl, -sinEl, -cosAz * cosEl}
	r := [3]float64{cosAz, 0, -sinAz}
	u := [3]float64{f[1]*r[2] - f[2]*r[1], f[2]*r[0] - f[0]*r[2], f[0]*r[1] - f[1]*r[0]}
	if l := math.Sqrt(u[0]*u[0] + u[1]*u[1] + u[2]*u[2]); l > 1e-3 {
		u = [3]float64{u[0] / l, u[1] / l, u[2] / l}
	}
	dot := func(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }
	dist := minZoom
	for _, x := range []float64{-0.5, 0.5} {
		for _, y := range []float64{-boxY / 2, boxY / 2} {
			for _, z := range []float64{-boxZ / 2, boxZ / 2} {
				d := [3]float64{x, y, z}
				depth := dot(d, f)
				dist = math.Max(dist, math.Abs(dot(d, r))*1.06/(shaderFov*math.Max(aspect, 0.1))-depth)
				dist = math.Max(dist, math.Abs(dot(d, u))*1.06/shaderFov-depth)
			}
		}
	}
	return dist
}

// FitView centres the volume and fits its box to the view, keeping the angle.
func (vr *VolumeRenderer) FitView() {
	if vr.gpuRenderer == nil {
		vr.ResetView()
		return
	}
	vr.mu.Lock()
	rotX, rotY := vr.gpuRenderer.GetRotation()
	d := fitDistance(vr.scaleY, vr.scaleZ, rotX, rotY, vr.viewAspect)
	vr.maxZoom = math.Max(5, d*2)
	vr.mu.Unlock()
	vr.gpuRenderer.SetPan(0, 0)
	vr.gpuRenderer.SetZoom(d)
	vr.TriggerRender()
}

// LookAlong points the camera down volume axis 0 (X), 1 (Y) or 2 (Z), then fits.
func (vr *VolumeRenderer) LookAlong(axis int) {
	if vr.gpuRenderer == nil {
		return
	}
	switch axis {
	case 0:
		vr.gpuRenderer.SetRotation(math.Pi/2, 0)
	case 1:
		vr.gpuRenderer.SetRotation(0, -math.Pi/2+0.01)
	default:
		vr.gpuRenderer.SetRotation(0, 0)
	}
	vr.FitView()
}

// IsGPU reports whether the GPU renderer is active (else the CPU fallback).
func (vr *VolumeRenderer) IsGPU() bool { return vr.useGPU && vr.gpuRenderer != nil }

// SetDepthCue sets how much samples darken with ray depth (0 = off).
func (vr *VolumeRenderer) SetDepthCue(v float64) {
	if vr.gpuRenderer != nil {
		vr.gpuRenderer.SetDepthCue(v)
	}
}

// Redraw renders the current state: queued on the GPU path, inline on CPU.
func (vr *VolumeRenderer) Redraw() {
	if vr.IsGPU() {
		vr.TriggerRender()
		return
	}
	if vr.cpuRenderer != nil {
		vr.cpuRenderer.NeedsRender = true
		vr.cpuRenderer.Render()
		vr.Refresh()
	}
}

// DragEnd handles drag end - ensures final state is rendered
func (vr *VolumeRenderer) DragEnd() {
	if vr.useGPU && vr.gpuRenderer != nil {
		// Ensure one final render request is queued
		select {
		case vr.renderChan <- struct{}{}:
		default:
			// Channel full, request already pending
		}
	}
}

// Scrolled handles mouse scroll for zoom
func (vr *VolumeRenderer) Scrolled(e *fyne.ScrollEvent) {
	if vr.useGPU && vr.gpuRenderer != nil {
		zoom := vr.gpuRenderer.GetZoom()
		zoom -= float64(e.Scrolled.DY) * 0.05
		zoom = math.Max(minZoom, math.Min(vr.maxZoom, zoom))
		vr.gpuRenderer.SetZoom(zoom)

		// Non-blocking send to render channel - drops frame if GPU busy
		select {
		case vr.renderChan <- struct{}{}:
			// Render request queued
		default:
			// Channel full - GPU busy, skip this frame
		}
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.Scrolled(e)
	}
}

// MouseIn handles mouse enter
func (vr *VolumeRenderer) MouseIn(*desktop.MouseEvent) {}

// MouseMoved handles mouse move
func (vr *VolumeRenderer) MouseMoved(*desktop.MouseEvent) {}

// MouseOut handles mouse exit
func (vr *VolumeRenderer) MouseOut() {}

// Cursor returns the cursor for this widget
func (vr *VolumeRenderer) Cursor() desktop.Cursor {
	return desktop.DefaultCursor
}

// MinSize returns the minimum size; kept small so phone layouts can shrink.
func (vr *VolumeRenderer) MinSize() fyne.Size {
	return fyne.NewSize(120, 120)
}

// ResetView resets camera to default position
func (vr *VolumeRenderer) ResetView() {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetRotation(defaultAzimuth, defaultElevation)
		vr.gpuRenderer.SetZoom(fitDistance(vr.scaleY, vr.scaleZ, defaultAzimuth, defaultElevation, vr.viewAspect))
		vr.gpuRenderer.ForceRender()
	} else if vr.cpuRenderer != nil {
		vr.cpuRenderer.RotationX = 0.3
		vr.cpuRenderer.RotationY = 0.5
		vr.cpuRenderer.Zoom = 2.0
		vr.cpuRenderer.NeedsRender = true
	}
}

// SetBackgroundColor sets the background color of the 3D view
func (vr *VolumeRenderer) SetBackgroundColor(c color.Color) {
	if vr.gpuRenderer == nil {
		return
	}
	r, g, b, a := c.RGBA()
	vr.gpuRenderer.SetBackgroundColor(float64(r)/65535.0, float64(g)/65535.0, float64(b)/65535.0, float64(a)/65535.0)
	vr.TriggerRender()
}

// SetQuality sets the rendering quality mode (0=Fast, 1=Medium, 2=High).
// On the GPU path it sets the ray step size.
func (vr *VolumeRenderer) SetQuality(mode int) {
	if vr.cpuRenderer != nil {
		vr.cpuRenderer.SetQuality(mode)
	}
	if vr.gpuRenderer != nil {
		vr.gpuRenderer.SetStepSize(renderQuality(mode).stepSize())
	}
}

// SetTransferFunction changes the colormap/transfer function used for rendering
func (vr *VolumeRenderer) SetTransferFunction(tfType volume.TransferFunctionType) {
	if vr.useGPU && vr.gpuRenderer != nil {
		vr.gpuRenderer.SetTransferFunction(tfType)
	}
	// CPU renderer would need similar implementation if needed
}

// Destroy cleans up resources
func (vr *VolumeRenderer) Destroy() {
	vr.destroyOnce.Do(func() {
		// Stop render loop
		close(vr.renderChan)

		if vr.gpuRenderer != nil {
			vr.gpuRenderer.Destroy()
		}
	})
}

// volumeWidgetRenderer implements fyne.WidgetRenderer
type volumeWidgetRenderer struct {
	vr      *VolumeRenderer
	img     *canvas.Image
	edges   [4]*canvas.Line
	readout *canvas.Text
	swatch  *canvas.Line // line sample before the readout, tying it to the outline
	axes    [3]*canvas.Line
	letters [3]*canvas.Text
}

// Gizmo colours: X pink, Y the accent, Z blue, as on roxel's corner gizmo.
var gizmoColors = [3]color.NRGBA{{231, 145, 135, 255}, {105, 211, 191, 255}, {143, 177, 225, 255}}

// cameraBasis returns the camera position, viewing direction and the screen
// right and screen-down vectors, with the shader's camera model. The 3D image
// is shown without a vertical flip, so +u is down the screen.
func (vr *VolumeRenderer) cameraBasis() (cam, f, r, u [3]float64) {
	g := vr.gpuRenderer
	rotX, rotY := g.GetRotation()
	zoom := g.GetZoom()
	panX, panY := g.GetPan()
	sinAz, cosAz := math.Sin(rotX), math.Cos(rotX)
	sinEl, cosEl := math.Sin(rotY), math.Cos(rotY)
	c := [3]float64{0.5 + panX, vr.scaleY*0.5 + panY, vr.scaleZ * 0.5}
	cam = [3]float64{sinAz*cosEl*zoom + c[0], sinEl*zoom + c[1], cosAz*cosEl*zoom + c[2]}
	f = [3]float64{c[0] - cam[0], c[1] - cam[1], c[2] - cam[2]}
	fl := math.Sqrt(f[0]*f[0] + f[1]*f[1] + f[2]*f[2])
	f = [3]float64{f[0] / fl, f[1] / fl, f[2] / fl}
	r = [3]float64{cosAz, 0, -sinAz}
	u = [3]float64{f[1]*r[2] - f[2]*r[1], f[2]*r[0] - f[0]*r[2], f[0]*r[1] - f[1]*r[0]}
	if ul := math.Sqrt(u[0]*u[0] + u[1]*u[1] + u[2]*u[2]); ul > 0.001 {
		u = [3]float64{u[0] / ul, u[1] / ul, u[2] / ul}
	}
	return cam, f, r, u
}

func dot3(a, b [3]float64) float64 { return a[0]*b[0] + a[1]*b[1] + a[2]*b[2] }

// projectToImage maps a point in the renderer's box space to normalised
// image coordinates with the same camera model as the ray-march shader.
func (vr *VolumeRenderer) projectToImage(p [3]float64) (x, y float64, ok bool) {
	cam, f, r, u := vr.cameraBasis()
	d := [3]float64{p[0] - cam[0], p[1] - cam[1], p[2] - cam[2]}
	depth := dot3(d, f)
	if depth <= 1e-6 {
		return 0, 0, false
	}
	ux, uy := dot3(d, r)/(depth*shaderFov*math.Max(vr.viewAspect, 0.1)), dot3(d, u)/(depth*shaderFov)
	return (ux + 1) / 2, (uy + 1) / 2, true
}

// layoutOverlay positions the slice-plane outline, its readout and the axis
// gizmo over the image.
func (r *volumeWidgetRenderer) layoutOverlay(size fyne.Size) {
	for _, e := range r.edges {
		e.Hide()
	}
	r.swatch.Hide()
	r.readout.Text = ""
	for i := range r.axes {
		r.axes[i].Hide()
		r.letters[i].Text = ""
	}
	if !r.vr.IsGPU() {
		return
	}
	r.layoutGizmo(size)
	if r.vr.Overlay == nil {
		return
	}
	corners, readout := r.vr.Overlay()
	r.readout.Text = readout
	r.readout.Color = colAccent
	ts := fyne.MeasureText(readout, r.readout.TextSize, r.readout.TextStyle)
	pos := fyne.NewPos(size.Width-ts.Width-16, size.Height-ts.Height-14)
	r.readout.Move(pos)
	r.readout.Resize(ts)
	if readout != "" && len(corners) == 4 {
		y := pos.Y + ts.Height/2
		r.swatch.Position1, r.swatch.Position2 = fyne.NewPos(pos.X-26, y), fyne.NewPos(pos.X-8, y)
		r.swatch.Show()
	}
	if len(corners) != 4 {
		return
	}
	at, shown := r.img.Position(), r.img.Size()
	var pts [4]fyne.Position
	for i, c := range corners {
		x, y, ok := r.vr.projectToImage(c)
		if !ok {
			return
		}
		pts[i] = fyne.NewPos(at.X+float32(x)*shown.Width, at.Y+float32(y)*shown.Height)
	}
	for i, e := range r.edges {
		e.Position1, e.Position2 = pts[i], pts[(i+1)%4]
		e.Show()
	}
}

// layoutGizmo draws the X / Y / Z axes in the lower-left corner: the "View
// along" buttons are named for them.
func (r *volumeWidgetRenderer) layoutGizmo(size fyne.Size) {
	if size.Width < 120 || size.Height < 120 {
		return
	}
	_, _, right, down := r.vr.cameraBasis()
	origin := fyne.NewPos(40, size.Height-40)
	for i, axis := range [3][3]float64{{1, 0, 0}, {0, 1, 0}, {0, 0, 1}} {
		dx, dy := dot3(axis, right), dot3(axis, down)
		n := math.Hypot(dx, dy)
		end := fyne.NewPos(origin.X+float32(dx*20), origin.Y+float32(dy*20))
		r.axes[i].Position1, r.axes[i].Position2 = origin, end
		r.axes[i].Show()
		// An axis pointing at the viewer has no direction on screen.
		ox, oy := 0.0, -1.0
		if n > 0.05 {
			ox, oy = dx/n, dy/n
		}
		reach := math.Max(n*20+9, 17)
		r.letters[i].Text = [3]string{"X", "Y", "Z"}[i]
		ts := fyne.MeasureText(r.letters[i].Text, r.letters[i].TextSize, r.letters[i].TextStyle)
		r.letters[i].Move(fyne.NewPos(origin.X+float32(ox*reach)-ts.Width/2, origin.Y+float32(oy*reach)-ts.Height/2))
		r.letters[i].Resize(ts)
	}
}

// renderSize is the GL target for a widget of the given size: its aspect,
// with the long side at the platform's resolution.
func renderSize(size fyne.Size) (int, int) {
	longSide := 1024
	if fyne.CurrentDevice().IsBrowser() {
		longSide = 768
	}
	if size.Width <= 0 || size.Height <= 0 {
		return longSide, longSide
	}
	if size.Width >= size.Height {
		return longSide, max(int(float32(longSide)*size.Height/size.Width), 64)
	}
	return max(int(float32(longSide)*size.Width/size.Height), 64), longSide
}

func (r *volumeWidgetRenderer) Layout(size fyne.Size) {
	vr := r.vr
	if vr.IsGPU() {
		// The render fills the widget: the camera fits the box to its aspect.
		r.img.Move(fyne.NewPos(0, 0))
		r.img.Resize(size)
		w, h := renderSize(size)
		vr.gpuRenderer.SetRenderSize(w, h)
		if aspect := float64(size.Width / max(size.Height, 1)); aspect != vr.viewAspect && size.Width > 0 {
			vr.viewAspect = aspect
			vr.FitView()
		}
		r.layoutOverlay(size)
		return
	}
	// CPU fallback: a square image, aspect-fit and centred.
	aspect := float32(1.0)
	if vr.dimX > 0 && vr.dimY > 0 {
		aspect = float32(vr.dimX) / float32(vr.dimY)
	}
	renderW := size.Width
	renderH := size.Width / aspect
	if renderH > size.Height {
		renderH = size.Height
		renderW = size.Height * aspect
	}
	r.img.Move(fyne.NewPos((size.Width-renderW)/2, (size.Height-renderH)/2))
	r.img.Resize(fyne.NewSize(renderW, renderH))
}

func (r *volumeWidgetRenderer) MinSize() fyne.Size {
	return fyne.NewSize(120, 120)
}

func (r *volumeWidgetRenderer) Refresh() {
	// r.vr.Render() // Disabled: Render should be triggered by events, not by repaint
	if r.vr.useGPU && r.vr.gpuRenderer != nil {
		r.img.Image = r.vr.gpuRenderer.GetRendered() // Get base image
		// Draw finding overlays
		if rgba, ok := r.img.Image.(*image.RGBA); ok {
			// Deprecated: GPU renderer handles finding drawing internally now
			// r.vr.drawFindings(rgba)
			_ = rgba // keep variable usage
		}
	} else if r.vr.cpuRenderer != nil {
		r.img.Image = r.vr.cpuRenderer.Rendered
	}
	r.layoutOverlay(r.vr.Size())
	for _, e := range r.edges {
		canvas.Refresh(e)
	}
	for i := range r.axes {
		canvas.Refresh(r.axes[i])
		canvas.Refresh(r.letters[i])
	}
	canvas.Refresh(r.swatch)
	canvas.Refresh(r.readout)
	canvas.Refresh(r.img)
}

func (r *volumeWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.img, r.edges[0], r.edges[1], r.edges[2], r.edges[3], r.swatch, r.readout,
		r.axes[0], r.axes[1], r.axes[2], r.letters[0], r.letters[1], r.letters[2]}
}

func (r *volumeWidgetRenderer) Destroy() {
	r.vr.Destroy()
}

// Show3DWindow creates and shows the 3D volume viewer window
func Show3DWindow(a fyne.App, pd *PixelData, windowLevel, windowWidth float64, imageRows, imageCols, pixelRep int) {
	vr := NewVolumeRenderer()
	vr.SetWindowLevel(windowLevel)
	vr.SetWindowWidth(windowWidth)
	_ = vr.LoadVolume(pd, imageRows, imageCols, pixelRep)

	rendererType := "GPU"
	if !vr.useGPU {
		rendererType = "CPU"
	}
	win := a.NewWindow("3D Volume Viewer (" + rendererType + ")")

	// Controls
	alphaSlider := widget.NewSlider(0.01, 0.5)
	alphaSlider.Value = vr.GetAlphaScale()
	alphaSlider.Step = 0.01
	alphaDebounce := newDebouncer(50 * time.Millisecond)
	alphaSlider.OnChanged = func(val float64) {
		valCopy := val
		alphaDebounce.call(func() {
			fyne.Do(func() {
				vr.SetAlphaScale(valCopy)
				vr.Render()
				vr.Refresh()
			})
		})
	}

	presets := widget.NewSelect([]string{
		"General Baggage",
		"Organic Materials",
		"Metals/High Density",
		"Electronics",
		"Explosives Detection",
	}, func(name string) {
		var level, width float64
		switch name {
		case "General Baggage":
			level, width = 40, 380
		case "Organic Materials":
			level, width = 30, 200
		case "Metals/High Density":
			level, width = 200, 800
		case "Electronics":
			level, width = 60, 300
		case "Explosives Detection":
			level, width = 35, 250
		}
		vr.SetWindowLevel(level)
		vr.SetWindowWidth(width)
		// Render on main thread
		vr.Render()
		vr.Refresh()
	})
	presets.PlaceHolder = "Material Preset..."

	resetBtn := widget.NewButton("Reset View", func() {
		vr.ResetView()
		// Render on main thread
		vr.Render()
		vr.Refresh()
	})

	// Quality selector (CPU renderer only)
	qualitySelect := widget.NewSelect([]string{
		"Fast",
		"Medium",
		"High",
	}, func(name string) {
		var mode int
		switch name {
		case "Fast":
			mode = 0
		case "Medium":
			mode = 1
		case "High":
			mode = 2
		}
		vr.SetQuality(mode)
		vr.Render()
		vr.Refresh()
	})
	qualitySelect.Selected = "High"

	// Layout:
	// Top: Quality (Left), Opacity (Stretch Center), Preset (Right)
	// Center: VR
	// Bottom: Reset

	qualityBox := container.NewHBox()
	if !vr.useGPU {
		qualityBox.Add(widget.NewLabel("Quality:"))
		qualityBox.Add(qualitySelect)
	}

	presetBox := container.NewHBox(widget.NewLabel("Preset:"), presets)
	opacityBox := container.NewBorder(nil, nil, widget.NewLabel("Opacity:"), nil, alphaSlider)

	topBar := container.NewBorder(nil, nil, qualityBox, presetBox, opacityBox)

	bottomBar := container.NewHBox(
		layout.NewSpacer(),
		resetBtn,
	)

	// Initial render - defer to after window shows
	vr.Render()

	content := container.NewBorder(topBar, bottomBar, nil, nil, vr)
	win.SetContent(content)
	win.Resize(fyne.NewSize(600, 650))
	win.Show()
}

// Show3DWindowComposite creates a 3D viewer with multiple volume selection
// NOTE: volumes map now stores composite info compatible with new types.
func Show3DWindowComposite(a fyne.App, volumes map[string]*CompositeVolume, windowLevel, windowWidth float64) {
	if len(volumes) == 0 {
		return
	}

	vr := NewVolumeRenderer()
	vr.SetWindowLevel(windowLevel)
	vr.SetWindowWidth(windowWidth)

	// Get sorted volume names
	var volumeNames []string
	for name := range volumes {
		volumeNames = append(volumeNames, name)
	}
	sort.Strings(volumeNames)

	// Find HE and LE volumes for dual-energy compositing
	var heVolume, leVolume *CompositeVolume
	for _, name := range volumeNames {
		v := volumes[name]
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "_he") {
			heVolume = v
		} else if strings.Contains(nameLower, "_le") {
			leVolume = v
		}
	}

	// Load primary volume (prefer HE)
	var initialVolume *CompositeVolume
	if heVolume != nil {
		initialVolume = heVolume
	} else if len(volumeNames) > 0 {
		initialVolume = volumes[volumeNames[0]]
	} else {
		return
	}

	// Assuming CompositeVolume struct has PixelData field now instead of Frames
	_ = vr.LoadVolume(initialVolume.PixelData, initialVolume.Rows, initialVolume.Cols, initialVolume.PixelRep)

	// Load secondary (LE) volume if available for dual-energy
	dualEnergyAvailable := heVolume != nil && leVolume != nil
	if dualEnergyAvailable {
		_ = vr.LoadSecondaryVolume(leVolume.PixelData, leVolume.Rows, leVolume.Cols, leVolume.PixelRep)
		vr.SetCompositeMode(true) // Enable by default when both are available
	}

	// Find finding volumes (files containing "finding" in name)
	var findingVolumes []*CompositeVolume
	var findingNames []string
	for _, name := range volumeNames {
		v := volumes[name]
		nameLower := strings.ToLower(name)
		if strings.Contains(nameLower, "finding") {
			findingVolumes = append(findingVolumes, v)
			findingNames = append(findingNames, name)
		}
	}

	rendererType := "GPU"
	if !vr.useGPU {
		rendererType = "CPU"
	}
	win := a.NewWindow("3D Composite Viewer (" + rendererType + ")")

	// Layers Selection Logic (With Opacity Sliders)
	volumeAlphas := make(map[string]float64)
	for name, v := range volumes {
		volumeAlphas[name] = v.Alpha
	}

	selectedLayersMap := make(map[string]bool)
	// Default selection: HE and LE if available, or first
	if heVolume != nil {
		selectedLayersMap[heVolume.Name] = true
	}
	if leVolume != nil {
		selectedLayersMap[leVolume.Name] = true
	}
	// Fallback
	hasSel := false
	for _, v := range selectedLayersMap {
		if v {
			hasSel = true
			break
		}
	}
	if !hasSel && len(volumeNames) > 0 {
		selectedLayersMap[volumeNames[0]] = true
	}

	updateVolumes := func(selected []string) {
		vr.ClearVolumes()
		if len(selected) > 0 {
			// Set params based on first volume
			if firstVol := volumes[selected[0]]; firstVol != nil {
				vr.SetWindowLevel(firstVol.WindowCenter)
				vr.SetWindowWidth(firstVol.WindowWidth)
				vr.SetRescaleIntercept(firstVol.RescaleIntercept)
			}
			// Load all
			for _, name := range selected {
				if v := volumes[name]; v != nil {
					alpha := v.Alpha
					if a, ok := volumeAlphas[name]; ok {
						alpha = a
					}
					_ = vr.AddVolume(v.PixelData, v.Rows, v.Cols, v.PixelRep, v.Color, v.WindowCenter, v.WindowWidth, v.RescaleIntercept, alpha)
				}
			}
		}
		vr.Render()
		vr.Refresh()
	}

	refreshLayers := func() {
		var sel []string
		for _, n := range volumeNames {
			if selectedLayersMap[n] {
				sel = append(sel, n)
			}
		}
		updateVolumes(sel)
	}

	layersBtn := widget.NewButton("Layers...", func() {
		var items []fyne.CanvasObject
		for _, name := range volumeNames {
			n := name

			slider := widget.NewSlider(0.0, 1.0)
			slider.Step = 0.05
			if a, ok := volumeAlphas[n]; ok {
				slider.Value = a
			} else {
				slider.Value = 1.0
			}

			alphaDebounce := newDebouncer(50 * time.Millisecond)
			slider.OnChanged = func(val float64) {
				valCopy := val
				volumeAlphas[n] = valCopy
				if selectedLayersMap[n] {
					idx := 0
					found := false
					for _, v := range volumeNames {
						if selectedLayersMap[v] {
							if v == n {
								found = true
								break
							}
							idx++
						}
					}
					if found {
						alphaDebounce.call(func() {
							fyne.Do(func() {
								vr.SetVolumeAlpha(idx, valCopy)
								vr.Render()
								vr.Refresh()
							})
						})
					}
				}
			}

			check := widget.NewCheck(n, func(b bool) {
				selectedLayersMap[n] = b
				if !b {
					slider.Disable()
				} else {
					slider.Enable()
				}
				refreshLayers()
			})
			check.Checked = selectedLayersMap[n]
			if !check.Checked {
				slider.Disable()
			}

			row := container.NewBorder(nil, nil, check, nil, slider)
			items = append(items, row)
		}
		content := container.NewVScroll(container.NewVBox(items...))
		content.SetMinSize(fyne.NewSize(350, 400))
		d := dialog.NewCustom("Select Layers & Opacity", "Close", content, win)
		d.Show()
	})

	refreshLayers()

	// Opacity control
	alphaSlider := widget.NewSlider(0.01, 0.5)
	alphaSlider.Value = vr.GetAlphaScale()
	alphaSlider.Step = 0.01
	alphaDebounce := newDebouncer(50 * time.Millisecond)
	alphaSlider.OnChanged = func(val float64) {
		valCopy := val
		alphaDebounce.call(func() {
			fyne.Do(func() {
				vr.SetAlphaScale(valCopy)
				vr.Render()
				vr.Refresh()
			})
		})
	}

	// Material presets
	presets := widget.NewSelect([]string{
		"General Baggage",
		"Organic Materials",
		"Metals/High Density",
		"Electronics",
		"Explosives Detection",
	}, func(name string) {
		var level, width float64
		switch name {
		case "General Baggage":
			level, width = 40, 380
		case "Organic Materials":
			level, width = 30, 200
		case "Metals/High Density":
			level, width = 200, 800
		case "Electronics":
			level, width = 60, 300
		case "Explosives Detection":
			level, width = 35, 250
		}
		vr.SetWindowLevel(level)
		vr.SetWindowWidth(width)
		vr.Render()
		vr.Refresh()
	})
	presets.PlaceHolder = "Material Preset..."

	resetBtn := widget.NewButton("Reset View", func() {
		vr.ResetView()
		vr.Render()
		vr.Refresh()
	})

	// Quality selector (CPU renderer only)
	qualitySelect := widget.NewSelect([]string{"Fast", "Medium", "High"}, func(name string) {
		var mode int
		switch name {
		case "Fast":
			mode = 0
		case "Medium":
			mode = 1
		case "High":
			mode = 2
		}
		vr.SetQuality(mode)
		vr.Render()
		vr.Refresh()
	})
	qualitySelect.Selected = "High"

	// Finding mode selector (only if finding volumes exist)
	// Finding Logic (Popup)
	var findingsBtn *widget.Button
	if len(findingNames) > 0 {
		var currentFindingMode = "Highlight"
		selectedFindingsMap := make(map[string]bool)
		// Default select first found finding
		if len(findingNames) > 0 {
			selectedFindingsMap[findingNames[0]] = true
		}

		// Palette for indicators
		findingPalette := []color.RGBA{
			{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 0, 255},
			{255, 0, 255, 255}, {0, 255, 255, 255}, {255, 128, 0, 255}, {128, 255, 0, 255},
		}

		var updateFindings func()
		updateFindings = func() {
			// Find visible findings
			var visibleVerified []*CompositeVolume
			for _, name := range findingNames {
				if selectedFindingsMap[name] {
					visibleVerified = append(visibleVerified, volumes[name])
				}
			}

			if len(visibleVerified) == 0 {
				vr.SetFindingMode(volume.FindingModeNone)
			} else {
				// Load first active finding volume (Voxel Renderer limitation)
				// In future, could merge or use AddVolume for multiple findings
				v := visibleVerified[0]
				_ = vr.LoadFindingVolume(v.PixelData, v.Rows, v.Cols, v.PixelRep)

				// Apply mode
				switch currentFindingMode {
				case "Highlight":
					vr.SetFindingMode(volume.FindingModeHighlight)
				case "Subtract":
					vr.SetFindingMode(volume.FindingModeSubtract)
				default:
					vr.SetFindingMode(volume.FindingModeNone)
				}
			}
			vr.Render()
			vr.Refresh()
		}

		findingsBtn = widget.NewButton("Findings...", func() {
			// Mode Selector
			modeSelect := widget.NewSelect([]string{"Highlight", "Subtract", "None"}, func(s string) {
				currentFindingMode = s
				updateFindings()
			})
			modeSelect.SetSelected(currentFindingMode)

			// Finding List
			var listItems []fyne.CanvasObject
			for i, name := range findingNames {
				n := name // capture
				c := findingPalette[i%len(findingPalette)]

				// Color Box
				rect := canvas.NewRectangle(c)
				rect.SetMinSize(fyne.NewSize(16, 16))
				rect.CornerRadius = 2

				// Checkbox
				check := widget.NewCheck(n, func(b bool) {
					selectedFindingsMap[n] = b
					updateFindings()
				})
				check.Checked = selectedFindingsMap[n]

				listItems = append(listItems, container.NewHBox(rect, check))
			}

			listContent := container.NewVBox(listItems...)
			scroll := container.NewVScroll(listContent)
			scroll.SetMinSize(fyne.NewSize(300, 300))

			content := container.NewVBox(
				widget.NewLabel("Display Mode"),
				modeSelect,
				widget.NewSeparator(),
				widget.NewLabel("Select Findings"),
				scroll,
			)

			d := dialog.NewCustom("Finding Assessment", "Close", content, win)
			d.Show()
		})

		// Initial Load
		updateFindings()
	}

	// Layout Reorganization
	// Top: Quality (Left), Opacity (Stretch), Preset (Right)
	// Bottom: Layers | Findings | Spacer | Reset

	// Ensure High Quality Default
	qualitySelect.SetSelected("High")

	qualityBox := container.NewHBox()
	if !vr.useGPU {
		qualityBox.Add(widget.NewLabel("Quality:"))
		qualityBox.Add(qualitySelect)
	}

	presetBox := container.NewHBox(widget.NewLabel("Preset:"), presets)
	opacityBox := container.NewBorder(nil, nil, widget.NewLabel("Opacity:"), nil, alphaSlider)

	topBar := container.NewBorder(nil, nil, qualityBox, presetBox, opacityBox)

	// Bottom Bar
	bottomLeft := container.NewHBox(layersBtn)
	if findingsBtn != nil {
		bottomLeft.Add(findingsBtn)
	}

	bottomBar := container.NewHBox(
		bottomLeft,
		layout.NewSpacer(),
		resetBtn,
	)

	vr.Render()

	content := container.NewBorder(topBar, bottomBar, nil, nil, vr)
	win.SetContent(content)
	win.Resize(fyne.NewSize(600, 700))
	win.Show()
}
