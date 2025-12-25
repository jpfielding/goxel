package volume

import (
	"image"
	"image/color"
	"log/slog"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Vec3 represents a 3D vector
type Vec3 struct {
	X, Y, Z float64
}

func (v Vec3) Add(o Vec3) Vec3    { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) Sub(o Vec3) Vec3    { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v Vec3) Mul(s float64) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }
func (v Vec3) Dot(o Vec3) float64 { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }
func (v Vec3) Len() float64       { return math.Sqrt(v.Dot(v)) }
func (v Vec3) Normalize() Vec3 {
	l := v.Len()
	if l == 0 {
		return v
	}
	return v.Mul(1 / l)
}

// FindingMode defines how finding volumes are visualized
type FindingMode int

const (
	FindingModeNone      FindingMode = iota // Don't show finding
	FindingModeHighlight                    // Show finding with bright color overlay
	FindingModeSubtract                     // Make finding regions transparent (see through)
)

// CPUVolumeRenderer performs CPU-based volume ray casting as a Fyne widget
type CPUVolumeRenderer struct {
	widget.BaseWidget

	// Primary volume data (HE in composite mode)
	VolumeData       []uint16
	DimX, DimY, DimZ int

	// Secondary volume data (LE in composite mode)
	SecondaryVolumeData       []uint16
	SecDimX, SecDimY, SecDimZ int

	// Finding volume data (overlay mask)
	FindingVolumeData                     []uint16
	FindingDimX, FindingDimY, FindingDimZ int
	FindingMode                           FindingMode

	// Finding bounding boxes for wireframe rendering
	Findings []FindingInfo

	// Composite mode: use HE/LE ratio for material colorization
	CompositeMode bool

	// Camera controls
	RotationX, RotationY float64
	Zoom                 float64

	// Rendering parameters
	WindowLevel      float64
	WindowWidth      float64
	AlphaScale       float64
	ScaleZ           float64 // Z-axis scaling factor
	RescaleIntercept float64 // Offset applied to raw pixel values (default 0)

	// Quality settings (0=Fast, 1=Medium, 2=High)
	QualityMode      int
	MaxResolution    int
	StepSize         float64
	DensityThreshold float64 // Clips voxels below this normalized density (0-1)

	// Transfer function (256 RGBA colors)
	TransferFunc []color.RGBA

	// Rendered image
	RenderWidth, RenderHeight int
	Rendered                  *image.RGBA
	Img                       *canvas.Image

	// State
	Mu          sync.RWMutex
	NeedsRender bool
}

// NewCPUVolumeRenderer creates a new CPU-based volume renderer
func NewCPUVolumeRenderer() *CPUVolumeRenderer {
	vr := &CPUVolumeRenderer{
		RotationX:     0.3,
		RotationY:     0.5,
		Zoom:          1.0,
		AlphaScale:    0.05,
		WindowLevel:   40.0,
		WindowWidth:   380.0,
		ScaleZ:        1.0,
		RenderWidth:   512,
		RenderHeight:  512,
		QualityMode:   2, // High by default
		MaxResolution: 1024,
		StepSize:         0.003,
		DensityThreshold: 0.03, // Match GPU default
		NeedsRender:      true,
	}
	vr.ExtendBaseWidget(vr)
	vr.createTransferFunction()
	vr.Rendered = image.NewRGBA(image.Rect(0, 0, vr.RenderWidth, vr.RenderHeight))
	return vr
}

// SetQuality sets the rendering quality mode (0=Fast, 1=Medium, 2=High)
func (vr *CPUVolumeRenderer) SetQuality(mode int) {
	vr.Mu.Lock()
	vr.QualityMode = mode
	switch mode {
	case 0: // Fast
		vr.MaxResolution = 400
		vr.StepSize = 0.012
	case 1: // Medium
		vr.MaxResolution = 768
		vr.StepSize = 0.005
	case 2: // High
		vr.MaxResolution = 1024
		vr.StepSize = 0.003
	}
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// SetWindowLevel sets the window level for CT visualization
func (vr *CPUVolumeRenderer) SetWindowLevel(level float64) {
	vr.Mu.Lock()
	vr.WindowLevel = level
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// SetWindowWidth sets the window width for CT visualization
func (vr *CPUVolumeRenderer) SetWindowWidth(width float64) {
	vr.Mu.Lock()
	vr.WindowWidth = width
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// SetScaleZ sets the Z-axis scaling factor
func (vr *CPUVolumeRenderer) SetScaleZ(scale float64) {
	vr.Mu.Lock()
	vr.ScaleZ = scale
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// SetRescaleIntercept sets the rescale intercept (offset applied to raw pixel values)
func (vr *CPUVolumeRenderer) SetRescaleIntercept(intercept float64) {
	vr.Mu.Lock()
	vr.RescaleIntercept = intercept
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// Params returns a snapshot of render parameters.
func (vr *CPUVolumeRenderer) Params() RenderParams {
	vr.Mu.RLock()
	defer vr.Mu.RUnlock()
	return RenderParams{
		WindowLevel:       vr.WindowLevel,
		WindowWidth:       vr.WindowWidth,
		AlphaScale:        vr.AlphaScale,
		ScaleZ:            vr.ScaleZ,
		RescaleIntercept:  vr.RescaleIntercept,
		StepSize:          vr.StepSize,
		DensityThreshold:  vr.DensityThreshold,
		AmbientIntensity:  0,
		DiffuseIntensity:  0,
		SpecularIntensity: 0,
	}
}

// SetParams updates render parameters in a single lock.
func (vr *CPUVolumeRenderer) SetParams(p RenderParams) {
	vr.Mu.Lock()
	vr.WindowLevel = p.WindowLevel
	vr.WindowWidth = p.WindowWidth
	vr.AlphaScale = p.AlphaScale
	vr.ScaleZ = p.ScaleZ
	vr.RescaleIntercept = p.RescaleIntercept
	if p.StepSize > 0 {
		vr.StepSize = p.StepSize
	}
	if p.DensityThreshold >= 0 {
		vr.DensityThreshold = p.DensityThreshold
	}
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// LoadVolume loads frame data into the volume
// For encapsulated (JPEG-compressed) frames, imageRows, imageCols, and pixelRep must be provided
func (vr *CPUVolumeRenderer) LoadVolume(pd *PixelData, imageRows, imageCols int, pixelRep int) error {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()

	if pd == nil || len(pd.Frames) == 0 {
		return nil
	}

	// Get dimensions - must be provided
	if imageRows > 0 && imageCols > 0 {
		vr.DimX = imageCols
		vr.DimY = imageRows
	} else {
		slog.Warn("Cannot load volume: rows/cols are zero", slog.Int("rows", imageRows), slog.Int("cols", imageCols))
		return nil
	}
	vr.DimZ = len(pd.Frames)

	if vr.DimX == 0 || vr.DimY == 0 {
		return nil
	}

	// Allocate volume data
	totalSize := vr.DimX * vr.DimY * vr.DimZ
	vr.VolumeData = make([]uint16, totalSize)

	// Copy frame data into volume
	signed := pixelRep == 1

	for z, f := range pd.Frames {
		sliceOffset := z * vr.DimX * vr.DimY

		if pd.IsEncapsulated {
			// Decode JPEG-compressed frame
			decodedData := decodeJPEGFrame(f.CompressedData, vr.DimX, vr.DimY, signed)
			for i, val := range decodedData {
				if i < vr.DimX*vr.DimY {
					uval := uint16(val)
					if val < 0 {
						uval = 0
					}
					vr.VolumeData[sliceOffset+i] = uval
				}
			}
		} else {
			// Native (uncompressed) frame - flat uint16 array
			// Assuming f.Data length matches rows*cols
			if len(f.Data) > 0 {
				copy(vr.VolumeData[sliceOffset:], f.Data)
			}
		}
	}

	vr.NeedsRender = true
	return nil
}

// LoadVolumeFromData loads volume data from raw uint16 slice
func (vr *CPUVolumeRenderer) LoadVolumeFromData(data []uint16, dimX, dimY, dimZ int) {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()

	vr.DimX = dimX
	vr.DimY = dimY
	vr.DimZ = dimZ
	vr.VolumeData = make([]uint16, len(data))
	copy(vr.VolumeData, data)

	vr.NeedsRender = true
}

// LoadSecondaryVolume loads the secondary (LE) volume for dual-energy compositing
func (vr *CPUVolumeRenderer) LoadSecondaryVolume(pd *PixelData, imageRows, imageCols int, pixelRep int) error {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()

	if pd == nil || len(pd.Frames) == 0 {
		return nil
	}

	// Get dimensions
	if imageRows > 0 && imageCols > 0 {
		vr.SecDimX = imageCols
		vr.SecDimY = imageRows
	} else {
		return nil
	}
	vr.SecDimZ = len(pd.Frames)

	if vr.SecDimX == 0 || vr.SecDimY == 0 {
		return nil
	}

	// Allocate secondary volume data
	totalSize := vr.SecDimX * vr.SecDimY * vr.SecDimZ
	vr.SecondaryVolumeData = make([]uint16, totalSize)

	// Copy frame data
	signed := pixelRep == 1

	for z, f := range pd.Frames {
		sliceOffset := z * vr.SecDimX * vr.SecDimY

		if pd.IsEncapsulated {
			decodedData := decodeJPEGFrame(f.CompressedData, vr.SecDimX, vr.SecDimY, signed)
			for i, val := range decodedData {
				if i < vr.SecDimX*vr.SecDimY {
					uval := uint16(val)
					if val < 0 {
						uval = 0
					}
					vr.SecondaryVolumeData[sliceOffset+i] = uval
				}
			}
		} else {
			if len(f.Data) > 0 {
				copy(vr.SecondaryVolumeData[sliceOffset:], f.Data)
			}
		}
	}

	vr.NeedsRender = true
	return nil
}

// SetCompositeMode enables or disables HE/LE ratio-based material colorization
func (vr *CPUVolumeRenderer) SetCompositeMode(enabled bool) {
	vr.Mu.Lock()
	vr.CompositeMode = enabled
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// HasSecondaryVolume returns true if a secondary volume is loaded
func (vr *CPUVolumeRenderer) HasSecondaryVolume() bool {
	vr.Mu.RLock()
	defer vr.Mu.RUnlock()
	return len(vr.SecondaryVolumeData) > 0
}

// LoadFindingVolume loads a finding overlay volume for visualization
func (vr *CPUVolumeRenderer) LoadFindingVolume(pd *PixelData, imageRows, imageCols int, pixelRep int) error {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()

	if pd == nil || len(pd.Frames) == 0 {
		return nil
	}

	// Get dimensions
	if imageRows > 0 && imageCols > 0 {
		vr.FindingDimX = imageCols
		vr.FindingDimY = imageRows
	} else {
		return nil
	}
	vr.FindingDimZ = len(pd.Frames)

	if vr.FindingDimX == 0 || vr.FindingDimY == 0 {
		return nil
	}

	// Allocate finding volume data
	totalSize := vr.FindingDimX * vr.FindingDimY * vr.FindingDimZ
	vr.FindingVolumeData = make([]uint16, totalSize)

	// Copy frame data
	signed := pixelRep == 1

	for z, f := range pd.Frames {
		sliceOffset := z * vr.FindingDimX * vr.FindingDimY

		// Finding data is typically native
		if len(f.Data) > 0 {
			// Need to handle signed/pixel rep mapping if needed?
			// Previous code iterates pixels.
			// Direct copy is faster if format matches.
			// But code had signed handling.

			for i, val := range f.Data {
				if i >= len(vr.FindingVolumeData[sliceOffset:]) {
					break
				}

				finalVal := val
				if signed {
					signedVal := int16(val)
					finalVal = uint16(int(signedVal) + 32768)
				}
				// no clamp needed for uint16 source?

				vr.FindingVolumeData[sliceOffset+i] = finalVal
			}
		}
	}

	vr.NeedsRender = true
	return nil
}

// LoadFindingVolumeFromData loads a finding volume from raw voxel data
func (vr *CPUVolumeRenderer) LoadFindingVolumeFromData(data []uint16, dimX, dimY, dimZ int) {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()

	vr.FindingDimX = dimX
	vr.FindingDimY = dimY
	vr.FindingDimZ = dimZ
	vr.FindingVolumeData = make([]uint16, len(data))
	copy(vr.FindingVolumeData, data)

	vr.NeedsRender = true
}

// SetFindingMode sets how finding volumes are visualized
func (vr *CPUVolumeRenderer) SetFindingMode(mode FindingMode) {
	vr.Mu.Lock()
	vr.FindingMode = mode
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// HasFindingVolume returns true if a finding volume is loaded
func (vr *CPUVolumeRenderer) HasFindingVolume() bool {
	vr.Mu.RLock()
	defer vr.Mu.RUnlock()
	return len(vr.FindingVolumeData) > 0
}

// GetFindingMode returns the current finding visualization mode
func (vr *CPUVolumeRenderer) GetFindingMode() FindingMode {
	vr.Mu.RLock()
	defer vr.Mu.RUnlock()
	return vr.FindingMode
}

// AddFinding adds a finding with bounding box for wireframe rendering
func (vr *CPUVolumeRenderer) AddFinding(name string, findingType int, bbox BoundingBox3D, c color.RGBA) {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()

	slog.Info("AddFinding called", "name", name, "bbox", bbox, "current_count", len(vr.Findings))

	vr.Findings = append(vr.Findings, FindingInfo{
		Name:    name,
		Type:    findingType,
		BBox:    bbox,
		Color:   c,
		Visible: true,
	})
	vr.NeedsRender = true
}

// ClearFindings removes all finding bounding boxes
func (vr *CPUVolumeRenderer) ClearFindings() {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()
	slog.Info("ClearFindings called")
	vr.Findings = nil
	vr.NeedsRender = true
}

// GetFindings returns a copy of findings for UI display
func (vr *CPUVolumeRenderer) GetFindings() []FindingInfo {
	vr.Mu.RLock()
	defer vr.Mu.RUnlock()
	result := make([]FindingInfo, len(vr.Findings))
	copy(result, vr.Findings)
	return result
}

// SetFindingVisible sets visibility of a specific finding by index
func (vr *CPUVolumeRenderer) SetFindingVisible(index int, visible bool) {
	vr.Mu.Lock()
	defer vr.Mu.Unlock()
	if index >= 0 && index < len(vr.Findings) {
		vr.Findings[index].Visible = visible
		vr.NeedsRender = true
	}
}

func (vr *CPUVolumeRenderer) createTransferFunction() {
	vr.TransferFunc = CreateTransferFunction(TransferFunctionDefault)
}

// SetTransferFunctionType updates the transfer function based on the provided type
func (vr *CPUVolumeRenderer) SetTransferFunctionType(tfType TransferFunctionType) {
	vr.Mu.Lock()
	vr.TransferFunc = CreateTransferFunction(tfType)
	vr.NeedsRender = true
	vr.Mu.Unlock()
}

// sampleVolumeTrilinear samples the volume at normalized coordinates [0,1] using trilinear interpolation
func sampleVolumeTrilinear(volumeData []uint16, dimX, dimY, dimZ int, x, y, z float64) uint16 {
	if x < 0 || x >= 1 || y < 0 || y >= 1 || z < 0 || z >= 1 {
		return 0
	}

	// Trilinear interpolation for smooth sampling
	fx := x * float64(dimX-1)
	fy := y * float64(dimY-1)
	fz := z * float64(dimZ-1)

	x0 := int(fx)
	y0 := int(fy)
	z0 := int(fz)
	x1 := min(x0+1, dimX-1)
	y1 := min(y0+1, dimY-1)
	z1 := min(z0+1, dimZ-1)

	xd := fx - float64(x0)
	yd := fy - float64(y0)
	zd := fz - float64(z0)

	// Sample 8 corners
	sliceSize := dimX * dimY
	idx000 := z0*sliceSize + y0*dimX + x0
	idx001 := z1*sliceSize + y0*dimX + x0
	idx010 := z0*sliceSize + y1*dimX + x0
	idx011 := z1*sliceSize + y1*dimX + x0
	idx100 := z0*sliceSize + y0*dimX + x1
	idx101 := z1*sliceSize + y0*dimX + x1
	idx110 := z0*sliceSize + y1*dimX + x1
	idx111 := z1*sliceSize + y1*dimX + x1

	c000 := float64(volumeData[idx000])
	c001 := float64(volumeData[idx001])
	c010 := float64(volumeData[idx010])
	c011 := float64(volumeData[idx011])
	c100 := float64(volumeData[idx100])
	c101 := float64(volumeData[idx101])
	c110 := float64(volumeData[idx110])
	c111 := float64(volumeData[idx111])

	// Interpolate along X
	c00 := c000*(1-xd) + c100*xd
	c01 := c001*(1-xd) + c101*xd
	c10 := c010*(1-xd) + c110*xd
	c11 := c011*(1-xd) + c111*xd

	// Interpolate along Y
	c0 := c00*(1-yd) + c10*yd
	c1 := c01*(1-yd) + c11*yd

	// Interpolate along Z
	return uint16(c0*(1-zd) + c1*zd)
}

// IntersectBox computes ray-box intersection with arbitrary max bounds (min is always 0,0,0)
func IntersectBox(origin, dir, maxBounds Vec3) (tNear, tFar float64, hit bool) {
	invDir := Vec3{1 / dir.X, 1 / dir.Y, 1 / dir.Z}

	t1 := (0 - origin.X) * invDir.X
	t2 := (maxBounds.X - origin.X) * invDir.X
	t3 := (0 - origin.Y) * invDir.Y
	t4 := (maxBounds.Y - origin.Y) * invDir.Y
	t5 := (0 - origin.Z) * invDir.Z
	t6 := (maxBounds.Z - origin.Z) * invDir.Z

	tNear = max(max(min(t1, t2), min(t3, t4)), min(t5, t6))
	tFar = min(min(max(t1, t2), max(t3, t4)), max(t5, t6))

	return tNear, tFar, tFar >= tNear && tFar >= 0
}

// Render performs ray casting and renders to the image
func (vr *CPUVolumeRenderer) Render() {
	vr.Mu.Lock()
	if !vr.NeedsRender || len(vr.VolumeData) == 0 {
		vr.Mu.Unlock()
		return
	}

	// Copy parameters under lock
	rotX := vr.RotationX
	rotY := vr.RotationY
	zoom := vr.Zoom
	wl := float64(vr.WindowLevel)
	ww := float64(vr.WindowWidth)
	alpha := vr.AlphaScale
	width := vr.RenderWidth
	height := vr.RenderHeight
	dimX, dimY, dimZ := vr.DimX, vr.DimY, vr.DimZ
	volumeData := vr.VolumeData
	transferFunc := vr.TransferFunc

	// Composite mode parameters
	compositeMode := vr.CompositeMode && len(vr.SecondaryVolumeData) > 0
	secondaryData := vr.SecondaryVolumeData
	secDimX, secDimY, secDimZ := vr.SecDimX, vr.SecDimY, vr.SecDimZ

	// Finding volume parameters
	findingMode := vr.FindingMode
	findingData := vr.FindingVolumeData
	findingDimX, findingDimY, findingDimZ := vr.FindingDimX, vr.FindingDimY, vr.FindingDimZ
	hasFinding := len(findingData) > 0 && findingMode != FindingModeNone

	// Copy finding bounding boxes for wireframe rendering
	findings := make([]FindingInfo, len(vr.Findings))
	copy(findings, vr.Findings)

	// Copy ScaleZ safely
	scaleZ := vr.ScaleZ
	if scaleZ <= 0 {
		scaleZ = 1.0
	}

	// Copy rescale intercept, density threshold, and step size
	rescaleIntercept := vr.RescaleIntercept
	densityThreshold := vr.DensityThreshold
	stepSize := vr.StepSize

	vr.NeedsRender = false
	vr.Mu.Unlock()

	// Camera setup - Turntable style rotation
	azimuth := rotX
	elevation := rotY

	// Camera position using turntable coordinates
	// Adjust center for scaled Z
	center := Vec3{0.5, 0.5, 0.5 * scaleZ}

	camPos := Vec3{
		X: math.Sin(azimuth) * math.Cos(elevation) * zoom,
		Y: math.Sin(elevation) * zoom,
		Z: math.Cos(azimuth) * math.Cos(elevation) * zoom,
	}
	// Scale camera distance slightly to fit scaled object?
	// Usually zoom handles it.

	camPos = camPos.Add(center)

	// Camera basis vectors
	forward := center.Sub(camPos).Normalize()

	// Right vector computed from azimuth to avoid inversion at 180° elevation
	right := Vec3{
		X: math.Cos(azimuth),
		Y: 0,
		Z: -math.Sin(azimuth),
	}

	// Up vector = cross(forward, right) - consistent up direction
	up := Vec3{
		X: forward.Y*right.Z - forward.Z*right.Y,
		Y: forward.Z*right.X - forward.X*right.Z,
		Z: forward.X*right.Y - forward.Y*right.X,
	}.Normalize()

	// Ray marching parameters
	windowMin := wl - ww/2
	windowMax := wl + ww/2
	windowRange := windowMax - windowMin

	// Render each pixel
	fov := 0.8
	aspectRatio := float64(width) / float64(height)

	// Pre-compute inverse dimensions for sampling
	invDimX := 1.0 / float64(dimX-1)
	invDimY := 1.0 / float64(dimY-1)
	invDimZ := 1.0 / float64(dimZ-1)

	// Parallel rendering - process rows concurrently
	numWorkers := 8
	rowsPerWorker := (height + numWorkers - 1) / numWorkers
	var wg sync.WaitGroup

	for w := 0; w < numWorkers; w++ {
		wg.Add(1)
		startRow := w * rowsPerWorker
		endRow := min(startRow+rowsPerWorker, height)

		go func(startY, endY int) {
			defer wg.Done()

			for py := startY; py < endY; py++ {
				v := (0.5 - float64(py)/float64(height)) * fov

				for px := 0; px < width; px++ {
					// Compute ray direction
					u := (float64(px)/float64(width) - 0.5) * fov * aspectRatio
					rayDir := forward.Add(right.Mul(u)).Add(up.Mul(v)).Normalize()

					// Intersect with volume bounding box (scaled)
					boxMax := Vec3{1.0, 1.0, scaleZ}
					tNear, tFar, hit := IntersectBox(camPos, rayDir, boxMax)
					if !hit {
						vr.Rendered.SetRGBA(px, py, color.RGBA{25, 25, 30, 255})
						continue
					}

					if tNear < 0 {
						tNear = 0
					}

					// Accumulate color along ray
					var accR, accG, accB, accA float64
					t := tNear

					for t < tFar && accA < 0.95 {
						pos := camPos.Add(rayDir.Mul(t))

						// Trilinear volume sampling for smooth rendering
						// Normalize Z coord by ScaleZ for texture lookup (0..1)
						if pos.X >= 0 && pos.X < 1 && pos.Y >= 0 && pos.Y < 1 && pos.Z >= 0 && pos.Z < scaleZ {
							// Sample primary (HE) volume
							sample := sampleVolumeTrilinear(volumeData, dimX, dimY, dimZ, pos.X, pos.Y, pos.Z/scaleZ)
							density := float64(sample) + rescaleIntercept

							// Compute normalized density and apply threshold (matching GPU path)
							normalized := (density - windowMin) / windowRange
							if normalized < 0 {
								normalized = 0
							} else if normalized > 1 {
								normalized = 1
							}

							if normalized > densityThreshold {
								var sampleColor color.RGBA
								var sampleA float64

								if compositeMode {
									// Dual-energy composite mode: use HE/LE ratio for material colorization
									leSample := sampleVolumeTrilinear(secondaryData, secDimX, secDimY, secDimZ, pos.X, pos.Y, pos.Z/scaleZ)
									leDensity := float64(leSample)

									// Compute HE/LE ratio (clamped to avoid division issues)
									var ratio float64
									if leDensity > 10 {
										ratio = density / leDensity
									} else {
										ratio = 1.0 // Default for very low LE signal
									}

									// Material colormap: Grayscale + Finding Highlights
									// Most materials shown as neutral gray tones
									// Only security-relevant materials get color:
									// - High-Z metals (ratio > 1.15) -> Red
									// - Dense materials like explosives-range (ratio 1.05-1.15) -> Orange
									var r, g, b uint8

									// Base grayscale from density
									gray := uint8(140 + normalized*80) // 140-220 range

									if ratio > 1.20 {
										// Metals - bright red
										r, g, b = 255, 60, 50
									} else if ratio > 1.10 {
										// Dense materials - orange highlight
										r, g, b = 255, 140, 40
									} else if ratio > 1.02 {
										// Slightly dense - subtle warm tint
										r, g, b = gray+30, gray, gray-20
									} else if ratio < 0.85 {
										// Very low-Z (air pockets, foam) - subtle blue tint
										r, g, b = gray-20, gray, gray+20
									} else {
										// Normal organic materials - neutral gray
										r, g, b = gray, gray, gray
									}

									sampleColor = color.RGBA{R: r, G: g, B: b, A: uint8(normalized * 255)}
									sampleA = normalized * alpha * 1.5 // Boost alpha in composite mode
								} else {
									// Standard mode: use transfer function
									if normalized > 0.01 {
										tfIdx := int(normalized * 255)
										sampleColor = transferFunc[tfIdx]
										sampleA = float64(sampleColor.A) / 255.0 * alpha
									}
								}

								// Apply finding visualization if enabled
								if hasFinding && sampleA > 0.001 {
									// Sample finding volume at same position
									// Map from primary volume space to finding volume space
									sampleX := pos.X
									sampleY := pos.Y
									sampleZ := pos.Z
									findingX := sampleX * float64(findingDimX-1)
									findingY := sampleY * float64(findingDimY-1)
									findingZ := sampleZ * float64(findingDimZ-1)

									fx := int(findingX)
									fy := int(findingY)
									fz := int(findingZ)

									if fx >= 0 && fx < findingDimX && fy >= 0 && fy < findingDimY && fz >= 0 && fz < findingDimZ {
										findingIdx := fz*findingDimX*findingDimY + fy*findingDimX + fx
										if findingIdx >= 0 && findingIdx < len(findingData) {
											findingVal := findingData[findingIdx]

											if findingVal > 0 {
												// This voxel is in a finding region
												switch findingMode {
												case FindingModeHighlight:
													// Add bright red overlay to finding regions
													findingIntensity := float64(findingVal) / 65535.0
													sampleColor = color.RGBA{
														R: uint8(min(255, int(float64(sampleColor.R)*(1-findingIntensity*0.6)+255*findingIntensity*0.6))),
														G: uint8(float64(sampleColor.G) * (1 - findingIntensity*0.4)),
														B: uint8(float64(sampleColor.B) * (1 - findingIntensity*0.4)),
														A: sampleColor.A,
													}
													sampleA = sampleA * (1 + findingIntensity*0.5) // Boost alpha
												case FindingModeSubtract:
													// Make finding regions transparent to see through
													sampleA = 0
												}
											}
										}
									}
								}

								if sampleA > 0.001 {
									// Front-to-back compositing
									oneMinusAccA := 1 - accA
									accR += oneMinusAccA * sampleA * float64(sampleColor.R) / 255.0
									accG += oneMinusAccA * sampleA * float64(sampleColor.G) / 255.0
									accB += oneMinusAccA * sampleA * float64(sampleColor.B) / 255.0
									accA += oneMinusAccA * sampleA
								}
							}
						}

						t += stepSize
					}

					// Background blend
					bgR, bgG, bgB := 0.1, 0.1, 0.12
					finalR := accR + (1-accA)*bgR
					finalG := accG + (1-accA)*bgG
					finalB := accB + (1-accA)*bgB

					vr.Rendered.SetRGBA(px, py, color.RGBA{
						R: uint8(min(255, int(finalR*255))),
						G: uint8(min(255, int(finalG*255))),
						B: uint8(min(255, int(finalB*255))),
						A: 255,
					})
				}
			}
		}(startRow, endRow)
	}

	wg.Wait()

	// Draw finding bounding boxes as wireframes after volume rendering
	if len(findings) > 0 {
		// Precompute inverse dimensions for coordinate normalization
		invDimXf := 1.0 / float64(dimX)
		invDimYf := 1.0 / float64(dimY)
		invDimZf := 1.0 / float64(dimZ)

		if len(findings) > 0 {
			slog.Info("Render loop: drawing findings", "count", len(findings))
		}

		for i, finding := range findings {
			if !finding.Visible {
				slog.Info("Finding skipping (not visible)", "index", i)
				continue
			}

			// Convert bounding box to normalized [0,1] coordinates
			minX := float64(finding.BBox.X) * invDimXf
			minY := float64(finding.BBox.Y) * invDimYf
			minZ := float64(finding.BBox.Z) * invDimZf
			maxX := float64(finding.BBox.X+finding.BBox.Width) * invDimXf
			maxY := float64(finding.BBox.Y+finding.BBox.Height) * invDimYf
			maxZ := float64(finding.BBox.Z+finding.BBox.Depth) * invDimZf

			slog.Info("Drawing finding BBox", "index", i, "minX", minX, "maxX", maxX, "color", finding.Color)

			// 8 corners of the bounding box (centered at 0.5, 0.5, 0.5)
			corners := []Vec3{
				{minX - 0.5, minY - 0.5, minZ - 0.5},
				{maxX - 0.5, minY - 0.5, minZ - 0.5},
				{maxX - 0.5, maxY - 0.5, minZ - 0.5},
				{minX - 0.5, maxY - 0.5, minZ - 0.5},
				{minX - 0.5, minY - 0.5, maxZ - 0.5},
				{maxX - 0.5, minY - 0.5, maxZ - 0.5},
				{maxX - 0.5, maxY - 0.5, maxZ - 0.5},
				{minX - 0.5, maxY - 0.5, maxZ - 0.5},
			}

			// Project corners to 2D screen coordinates
			projectedCorners := make([]struct{ X, Y int }, 8)
			for i, corner := range corners {
				// Apply same rotation as camera
				cosA, sinA := math.Cos(azimuth), math.Sin(azimuth)
				cosE, sinE := math.Cos(elevation), math.Sin(elevation)

				rotated := Vec3{
					X: corner.X*cosA - corner.Z*sinA,
					Y: corner.Y,
					Z: corner.X*sinA + corner.Z*cosA,
				}
				rotated2 := Vec3{
					X: rotated.X,
					Y: rotated.Y*cosE - rotated.Z*sinE,
					Z: rotated.Y*sinE + rotated.Z*cosE,
				}

				// Perspective projection
				perspZ := rotated2.Z + zoom
				if perspZ < 0.1 {
					perspZ = 0.1
				}
				screenX := int(float64(width)/2 + rotated2.X*float64(width)*0.8/perspZ)
				screenY := int(float64(height)/2 - rotated2.Y*float64(height)*0.8/perspZ)

				projectedCorners[i] = struct{ X, Y int }{screenX, screenY}
			}

			// Draw 12 edges of the wireframe box
			edges := [][2]int{
				{0, 1}, {1, 2}, {2, 3}, {3, 0}, // Bottom face
				{4, 5}, {5, 6}, {6, 7}, {7, 4}, // Top face
				{0, 4}, {1, 5}, {2, 6}, {3, 7}, // Vertical edges
			}

			for _, edge := range edges {
				p1, p2 := projectedCorners[edge[0]], projectedCorners[edge[1]]
				drawLine(vr.Rendered, p1.X, p1.Y, p2.X, p2.Y, finding.Color, 2)
			}
		}
	}

	// Suppress unused variable warnings
	_ = invDimX
	_ = invDimY
	_ = invDimZ
}

// drawLine draws a line on the image using Bresenham's algorithm with thickness
func drawLine(img *image.RGBA, x0, y0, x1, y1 int, c color.RGBA, thickness int) {
	bounds := img.Bounds()

	dx := abs(x1 - x0)
	dy := abs(y1 - y0)
	sx, sy := 1, 1
	if x0 >= x1 {
		sx = -1
	}
	if y0 >= y1 {
		sy = -1
	}
	err := dx - dy

	for {
		// Draw pixel with thickness
		for ty := -thickness / 2; ty <= thickness/2; ty++ {
			for tx := -thickness / 2; tx <= thickness/2; tx++ {
				px, py := x0+tx, y0+ty
				if px >= bounds.Min.X && px < bounds.Max.X && py >= bounds.Min.Y && py < bounds.Max.Y {
					img.SetRGBA(px, py, c)
				}
			}
		}

		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// CreateRenderer creates a renderer for the volume widget
func (vr *CPUVolumeRenderer) CreateRenderer() fyne.WidgetRenderer {
	vr.Img = canvas.NewImageFromImage(vr.Rendered)
	vr.Img.FillMode = canvas.ImageFillContain
	vr.Img.ScaleMode = canvas.ImageScaleFastest

	return &cpuVolumeWidgetRenderer{
		vr:  vr,
		img: vr.Img,
	}
}

// Dragged handles mouse drag for rotation
func (vr *CPUVolumeRenderer) Dragged(e *fyne.DragEvent) {
	vr.Mu.Lock()

	// Horizontal drag controls azimuth (continuous 360°) - inverted for natural feel
	vr.RotationX -= float64(e.Dragged.DX) * 0.01
	for vr.RotationX < 0 {
		vr.RotationX += 2 * math.Pi
	}
	for vr.RotationX >= 2*math.Pi {
		vr.RotationX -= 2 * math.Pi
	}

	// Vertical drag controls elevation (continuous 360°) - inverted for natural feel
	vr.RotationY -= float64(e.Dragged.DY) * 0.01
	for vr.RotationY < 0 {
		vr.RotationY += 2 * math.Pi
	}
	for vr.RotationY >= 2*math.Pi {
		vr.RotationY -= 2 * math.Pi
	}

	vr.NeedsRender = true
	vr.Mu.Unlock()

	go func() {
		vr.Render()
		fyne.Do(func() {
			vr.Refresh()
		})
	}()
}

// DragEnd handles drag end
func (vr *CPUVolumeRenderer) DragEnd() {}

// Scrolled handles mouse scroll for zoom
func (vr *CPUVolumeRenderer) Scrolled(e *fyne.ScrollEvent) {
	vr.Mu.Lock()
	vr.Zoom -= float64(e.Scrolled.DY) * 0.05
	if vr.Zoom < 0.5 {
		vr.Zoom = 0.5
	}
	if vr.Zoom > 5 {
		vr.Zoom = 5
	}
	vr.NeedsRender = true
	vr.Mu.Unlock()

	go func() {
		vr.Render()
		fyne.Do(func() {
			vr.Refresh()
		})
	}()
}

// MouseIn handles mouse enter
func (vr *CPUVolumeRenderer) MouseIn(*desktop.MouseEvent) {}

// MouseMoved handles mouse move
func (vr *CPUVolumeRenderer) MouseMoved(*desktop.MouseEvent) {}

// MouseOut handles mouse exit
func (vr *CPUVolumeRenderer) MouseOut() {}

// Cursor returns the cursor for this widget
func (vr *CPUVolumeRenderer) Cursor() desktop.Cursor {
	return desktop.DefaultCursor
}

// MinSize returns the minimum size
func (vr *CPUVolumeRenderer) MinSize() fyne.Size {
	return fyne.NewSize(300, 300)
}

// cpuVolumeWidgetRenderer implements fyne.WidgetRenderer
type cpuVolumeWidgetRenderer struct {
	vr  *CPUVolumeRenderer
	img *canvas.Image
}

func (r *cpuVolumeWidgetRenderer) Layout(size fyne.Size) {
	r.img.Resize(size)

	// Update render resolution based on size (capped by quality setting)
	r.vr.Mu.Lock()
	maxRes := r.vr.MaxResolution
	r.vr.Mu.Unlock()

	newW := min(int(size.Width), maxRes)
	newH := min(int(size.Height), maxRes)

	r.vr.Mu.Lock()
	if newW != r.vr.RenderWidth || newH != r.vr.RenderHeight {
		r.vr.RenderWidth = newW
		r.vr.RenderHeight = newH
		r.vr.Rendered = image.NewRGBA(image.Rect(0, 0, newW, newH))
		r.vr.NeedsRender = true
	}
	r.vr.Mu.Unlock()
}

func (r *cpuVolumeWidgetRenderer) MinSize() fyne.Size {
	return fyne.NewSize(300, 300)
}

func (r *cpuVolumeWidgetRenderer) Refresh() {
	r.vr.Render()
	r.img.Image = r.vr.Rendered
	canvas.Refresh(r.img)
}

func (r *cpuVolumeWidgetRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.img}
}

func (r *cpuVolumeWidgetRenderer) Destroy() {}

// Show3DWindow creates and shows the 3D volume viewer window using CPU rendering
func ShowCPU3DWindow(a fyne.App, pd *PixelData, windowLevel, windowWidth float64, imageRows, imageCols, pixelRep int) {
	vr := NewCPUVolumeRenderer()
	vr.SetWindowLevel(windowLevel)
	vr.SetWindowWidth(windowWidth)
	_ = vr.LoadVolume(pd, imageRows, imageCols, pixelRep)

	win := a.NewWindow("3D Volume Viewer (CPU)")

	// Controls
	alphaSlider := widget.NewSlider(0.01, 0.5)
	alphaSlider.Value = vr.AlphaScale
	alphaSlider.Step = 0.01
	alphaSlider.OnChanged = func(val float64) {
		vr.Mu.Lock()
		vr.AlphaScale = val
		vr.NeedsRender = true
		vr.Mu.Unlock()
		go func() {
			vr.Render()
			fyne.Do(func() {
				vr.Refresh()
			})
		}()
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
		go func() {
			vr.Render()
			fyne.Do(func() {
				vr.Refresh()
			})
		}()
	})
	presets.PlaceHolder = "Material Preset..."

	resetBtn := widget.NewButton("Reset View", func() {
		vr.Mu.Lock()
		vr.RotationX = 0.3
		vr.RotationY = 0.5
		vr.Zoom = 2.0
		vr.NeedsRender = true
		vr.Mu.Unlock()
		go func() {
			vr.Render()
			fyne.Do(func() {
				vr.Refresh()
			})
		}()
	})

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
		// Force layout update to apply new resolution
		vr.Mu.Lock()
		vr.RenderWidth = 0 // Force resize recalculation
		vr.Mu.Unlock()
		go func() {
			vr.Render()
			fyne.Do(func() {
				vr.Refresh()
			})
		}()
	})
	qualitySelect.Selected = "High"

	controls := container.NewVBox(
		widget.NewLabel("Drag to rotate • Scroll to zoom"),
		widget.NewForm(
			widget.NewFormItem("Opacity", alphaSlider),
			widget.NewFormItem("Quality", qualitySelect),
			widget.NewFormItem("Preset", presets),
		),
		resetBtn,
	)

	// Initial render
	go func() {
		vr.Render()
		fyne.Do(func() {
			vr.Refresh()
		})
	}()

	content := container.NewBorder(nil, controls, nil, nil, vr)
	win.SetContent(content)
	win.Resize(fyne.NewSize(600, 650))
	win.Show()
}
