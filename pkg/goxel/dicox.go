package goxel

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
)

// ViewOrientation defines the plane for MPR viewing
type ViewOrientation int

const (
	Axial    ViewOrientation = iota // Top-down view (X-Y plane, scrolling Z)
	Coronal                         // Front view (X-Z plane, scrolling Y)
	Sagittal                        // Side view (Y-Z plane, scrolling X)
)

func (v ViewOrientation) String() string {
	switch v {
	case Axial:
		return "Axial (Top-Down)"
	case Coronal:
		return "Coronal (Front)"
	case Sagittal:
		return "Sagittal (Side)"
	default:
		return "Unknown"
	}
}

// isJPEGLossless checks if JPEG data is lossless (SOF3) or baseline (SOF0)
// by scanning for the SOF marker. Returns true for lossless, false for lossy.
func isJPEGLossless(data []byte) bool {
	for i := 0; i < len(data)-1; i++ {
		if data[i] == 0xFF {
			switch data[i+1] {
			case 0xC0: // SOF0 - Baseline DCT (lossy)
				return false
			case 0xC3: // SOF3 - Lossless
				return true
			}
		}
	}
	return false // Default to lossy if unknown
}

type DICOXImage struct {
	level int
	width int

	// For native (uncompressed) frames
	nativeData []uint16 // Flattened pixel data
	nativeRows int
	nativeCols int

	// For decoded encapsulated frames
	decoded     [][]int
	decodedRows int
	decodedCols int
	isEncoded   bool

	// Volume data for MPR
	volume           [][][]int // [z][y][x] = pixel value
	volX, volY, volZ int       // Volume dimensions
	orientation      ViewOrientation
	sliceIndex       int
	hasVolume        bool

	// Composite view mode (MIP - Maximum Intensity Projection)
	compositeMode bool

	// Transfer function for trimat coloring (density -> RGBA)
	transferFunc []color.RGBA

	// Alpha scale for composite rendering (synced from 3D view)
	alphaScale float64
}

// SetFrame sets native (uncompressed) frame data
func (d *DICOXImage) SetFrame(data []uint16, rows, cols int, bitsAllocated, pixelRep int) {
	d.nativeData = data
	d.nativeRows = rows
	d.nativeCols = cols

	// Handle signed data conversion if necessary?
	// Storing as uint16 is fine, At() logic will interpret.
	// But previous logic handled signed conversion during decode or getpixel?
	// GetPixel returned ints.
	// Here we store raw uint16. We might need logic in At() to handle signed.

	d.isEncoded = false
	// d.frame was removed
}

// SetFrameWithMeta is legacy adapter - replaced by direct SetFrame
// We keep signature if needed but implementation uses SetFrame
// Actually, we should replace callers. But if not possible yet:
func (d *DICOXImage) SetFrameWithMeta(data []uint16, rows, cols int, bitsAllocated, pixelRep int) {
	d.SetFrame(data, rows, cols, bitsAllocated, pixelRep)
}

// SetEncodedFrame decodes and sets JPEG-compressed frame data.
// Note: DICOM-specific compression formats are handled by the dicom library during loading.
// This function handles standard JPEG as a fallback.
func (d *DICOXImage) SetEncodedFrame(data []byte, rows, cols, bitsAllocated int, pixelRepresentation int) error {
	slog.Info("SetEncodedFrame called", slog.Int("rows", rows), slog.Int("cols", cols),
		slog.Int("bitsAllocated", bitsAllocated), slog.Int("pixelRep", pixelRepresentation),
		slog.Int("dataLen", len(data)))

	// Standard JPEG decoder for baseline/lossy JPEG
	r := bytes.NewReader(data)
	img, err := jpeg.Decode(r)
	if err == nil {
		slog.Info("Standard JPEG decode succeeded")
		bounds := img.Bounds()
		d.decodedRows = bounds.Dy()
		d.decodedCols = bounds.Dx()
		d.decoded = make([][]int, d.decodedRows*d.decodedCols)

		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				i := (y-bounds.Min.Y)*d.decodedCols + (x - bounds.Min.X)
				gray := color.Gray16Model.Convert(img.At(x, y)).(color.Gray16)
				d.decoded[i] = []int{int(gray.Y)}
			}
		}
		d.isEncoded = true
		return nil
	}

	slog.Warn("JPEG decode failed, creating placeholder", slog.Any("error", err))
	d.decodedRows = rows
	d.decodedCols = cols
	d.decoded = make([][]int, rows*cols)
	for i := range d.decoded {
		d.decoded[i] = []int{0}
	}
	d.isEncoded = true
	return nil
}

func (d *DICOXImage) WindowLevel() int {
	return d.level
}

func (d *DICOXImage) SetWindowLevel(level int) {
	d.level = level
}

func (d *DICOXImage) WindowWidth() int {
	return d.width
}

func (d *DICOXImage) SetWindowWidth(width int) {
	d.width = width
}

func (d *DICOXImage) ColorModel() color.Model {
	if d.compositeMode {
		return color.RGBAModel
	}
	return color.Gray16Model
}

// SetTransferFunction sets the trimat color lookup table for composite rendering
func (d *DICOXImage) SetTransferFunction(tf []color.RGBA) {
	d.transferFunc = tf
	slog.Debug("Transfer function set for 2D view", slog.Int("colors", len(tf)))
}

// SetAlphaScale sets the opacity scale for composite rendering (synced from 3D view)
func (d *DICOXImage) SetAlphaScale(scale float64) {
	d.alphaScale = scale
}

// GetAlphaScale returns the current alpha scale
func (d *DICOXImage) GetAlphaScale() float64 {
	if d.alphaScale <= 0 {
		return 0.08 // Default matches 3D view
	}
	return d.alphaScale
}

// Width returns the image width (cols)
func (d *DICOXImage) Width() int {
	if d.hasVolume {
		return d.volX
	}
	if d.isEncoded {
		return d.decodedCols
	}
	return d.nativeCols
}

// Height returns the image height (rows)
func (d *DICOXImage) Height() int {
	if d.hasVolume {
		return d.volY
	}
	if d.isEncoded {
		return d.decodedRows
	}
	return d.nativeRows
}

// Depth returns the volume depth (slices/frames)
func (d *DICOXImage) Depth() int {
	if d.hasVolume {
		return d.volZ
	}
	return 1
}

func (d *DICOXImage) Bounds() image.Rectangle {
	// MPR mode
	if d.hasVolume {
		switch d.orientation {
		case Axial:
			return image.Rect(0, 0, d.volX, d.volY)
		case Coronal:
			return image.Rect(0, 0, d.volX, d.volZ)
		case Sagittal:
			return image.Rect(0, 0, d.volY, d.volZ)
		}
	}
	if d.isEncoded {
		return image.Rect(0, 0, d.decodedCols, d.decodedRows)
	}
	if len(d.nativeData) > 0 {
		return image.Rect(0, 0, d.nativeCols, d.nativeRows)
	}
	return image.Rectangle{}
}

var mipLogOnce bool

func (d *DICOXImage) At(x, y int) color.Color {
	windowMin := int32(d.level) - int32(d.width)/2
	windowMax := windowMin + int32(d.width)

	var raw int32

	// MPR mode - sample from volume
	if d.hasVolume {
		var val int
		if d.compositeMode {
			// Log once to confirm composite is active
			if !mipLogOnce && x == 0 && y == 0 {
				mipLogOnce = true
				slog.Info("Alpha composite rendering active", slog.String("orientation", d.orientation.String()),
					slog.Int("volX", d.volX), slog.Int("volY", d.volY), slog.Int("volZ", d.volZ))
			}

			// Alpha compositing with trimat colors (same as 3D view)
			// Front-to-back: C_out = C_in + (1 - A_in) * C_slice * A_slice
			var accumR, accumG, accumB, accumA float64

			// Use window/level for transfer function lookup (matches 3D shader)
			wMin := float64(windowMin)
			wRange := float64(d.width)
			if wRange == 0 {
				wRange = 1
			}

			// Alpha scale factor (synced from 3D view opacity slider)
			alphaScale := d.GetAlphaScale()

			// Helper to get trimat color from transfer function (same as 3D view)
			// Uses window/level normalization: norm = (density - windowMin) / windowWidth
			// Returns RGBA with alpha from transfer function (same as 3D shader)
			getColor := func(density float64) (r, g, b, a float64) {
				if len(d.transferFunc) > 0 {
					// Window/level normalization (matches GPU shader)
					norm := (density - wMin) / wRange
					if norm < 0 {
						norm = 0
					}
					if norm > 1 {
						norm = 1
					}
					idx := int(norm * float64(len(d.transferFunc)-1))
					c := d.transferFunc[idx]
					return float64(c.R) / 255.0, float64(c.G) / 255.0, float64(c.B) / 255.0, float64(c.A) / 255.0
				}
				// Fallback: grayscale using window/level
				norm := (density - wMin) / wRange
				if norm < 0 {
					norm = 0
				}
				if norm > 1 {
					norm = 1
				}
				return norm, norm, norm, 1.0
			}

			// Apply alpha adjustment same as 3D shader: pow(sampleColor.a, 1.5) * alphaScale
			adjustAlpha := func(tfAlpha float64) float64 {
				if tfAlpha <= 0 {
					return 0
				}
				// pow(alpha, 1.5) matches GPU shader
				adjusted := math.Pow(tfAlpha, 1.5) * alphaScale
				if adjusted > 1 {
					adjusted = 1
				}
				return adjusted
			}

			switch d.orientation {
			case Axial:
				// Project through Z (front to back)
				for z := 0; z < d.volZ && accumA < 0.98; z++ {
					if z < len(d.volume) && y < len(d.volume[z]) && x < len(d.volume[z][y]) {
						density := float64(d.volume[z][y][x])
						// Skip if below window minimum (transparent/air)
						if density > wMin {
							r, g, b, tfAlpha := getColor(density)
							// Use transfer function's alpha (same as 3D shader)
							alpha := adjustAlpha(tfAlpha)
							if alpha > 0 {
								// Standard front-to-back compositing
								accumR += (1 - accumA) * r * alpha
								accumG += (1 - accumA) * g * alpha
								accumB += (1 - accumA) * b * alpha
								accumA += (1 - accumA) * alpha
							}
						}
					}
				}
			case Coronal:
				// Project through Y (front to back)
				volZ := y
				for volY := 0; volY < d.volY && accumA < 0.98; volY++ {
					if volZ < len(d.volume) && volY < len(d.volume[volZ]) && x < len(d.volume[volZ][volY]) {
						density := float64(d.volume[volZ][volY][x])
						if density > wMin {
							r, g, b, tfAlpha := getColor(density)
							// Use transfer function's alpha (same as 3D shader)
							alpha := adjustAlpha(tfAlpha)
							if alpha > 0 {
								accumR += (1 - accumA) * r * alpha
								accumG += (1 - accumA) * g * alpha
								accumB += (1 - accumA) * b * alpha
								accumA += (1 - accumA) * alpha
							}
						}
					}
				}
			case Sagittal:
				// Project through X (front to back)
				volZ := y
				volY := x
				for volX := 0; volX < d.volX && accumA < 0.98; volX++ {
					if volZ < len(d.volume) && volY < len(d.volume[volZ]) && volX < len(d.volume[volZ][volY]) {
						density := float64(d.volume[volZ][volY][volX])
						if density > wMin {
							r, g, b, tfAlpha := getColor(density)
							// Use transfer function's alpha (same as 3D shader)
							alpha := adjustAlpha(tfAlpha)
							if alpha > 0 {
								accumR += (1 - accumA) * r * alpha
								accumG += (1 - accumA) * g * alpha
								accumB += (1 - accumA) * b * alpha
								accumA += (1 - accumA) * alpha
							}
						}
					}
				}
			}

			// Blend with white background: final = accumColor + (1-accumA) * white
			finalR := accumR + (1-accumA)*1.0
			finalG := accumG + (1-accumA)*1.0
			finalB := accumB + (1-accumA)*1.0

			// Clamp
			if finalR > 1 {
				finalR = 1
			}
			if finalG > 1 {
				finalG = 1
			}
			if finalB > 1 {
				finalB = 1
			}

			return color.RGBA{
				R: uint8(finalR * 255),
				G: uint8(finalG * 255),
				B: uint8(finalB * 255),
				A: 255,
			}
		} else {
			// Single slice mode
			switch d.orientation {
			case Axial:
				// X-Y plane at Z=sliceIndex
				if d.sliceIndex < len(d.volume) && y < len(d.volume[d.sliceIndex]) && x < len(d.volume[d.sliceIndex][y]) {
					val = d.volume[d.sliceIndex][y][x]
				}
			case Coronal:
				// X-Z plane at Y=sliceIndex (y param is Z, x param is X)
				z := y
				if z < len(d.volume) && d.sliceIndex < len(d.volume[z]) && x < len(d.volume[z][d.sliceIndex]) {
					val = d.volume[z][d.sliceIndex][x]
				}
			case Sagittal:
				// Y-Z plane at X=sliceIndex (y param is Z, x param is Y)
				z := y
				yCoord := x
				if z < len(d.volume) && yCoord < len(d.volume[z]) && d.sliceIndex < len(d.volume[z][yCoord]) {
					val = d.volume[z][yCoord][d.sliceIndex]
				}
			}
		}
		raw = int32(val)
	} else if d.isEncoded {
		i := y*d.decodedCols + x
		if i >= len(d.decoded) || len(d.decoded[i]) == 0 {
			return color.Gray16{Y: 0}
		}
		raw = int32(d.decoded[i][0])
	} else {
		if len(d.nativeData) == 0 {
			return color.Gray16{Y: 0}
		}
		idx := y*d.nativeCols + x
		if idx < 0 || idx >= len(d.nativeData) {
			return color.Black
		}
		val := d.nativeData[idx]
		raw = int32(val)
	}

	// Grayscale output with window/level
	if raw < windowMin {
		return color.Gray16{Y: 0}
	} else if raw >= windowMax {
		return color.Gray16{Y: 0xffff}
	}

	val := float32(raw-windowMin) / float32(d.width)
	return color.Gray16{Y: uint16(float32(0xffff) * val)}
}

func NewDICOXImage(f []uint16, rows, cols, level, width int) *DICOXImage {
	return &DICOXImage{nativeData: f, nativeRows: rows, nativeCols: cols, width: width, level: level, orientation: Axial}
}

// SetVolume stores the complete volume data for MPR viewing
func (d *DICOXImage) SetVolume(volume [][][]int, dimX, dimY, dimZ int) {
	d.volume = volume
	d.volX = dimX
	d.volY = dimY
	d.volZ = dimZ
	d.hasVolume = true
	d.sliceIndex = dimZ / 2 // Start in the middle
	slog.Info("Volume set for MPR", slog.Int("x", dimX), slog.Int("y", dimY), slog.Int("z", dimZ),
		slog.Int("windowLevel", int(d.level)), slog.Int("windowWidth", int(d.width)))
}

// SetOrientation changes the viewing orientation
func (d *DICOXImage) SetOrientation(orient ViewOrientation) {
	d.orientation = orient
	// Reset to middle slice for new orientation
	switch orient {
	case Axial:
		d.sliceIndex = d.volZ / 2
	case Coronal:
		d.sliceIndex = d.volY / 2
	case Sagittal:
		d.sliceIndex = d.volX / 2
	}
	slog.Info("Orientation changed", slog.String("orient", orient.String()), slog.Int("slice", d.sliceIndex))
}

// Orientation returns the current view orientation
func (d *DICOXImage) Orientation() ViewOrientation {
	return d.orientation
}

// SliceIndex returns the current slice index
func (d *DICOXImage) SliceIndex() int {
	return d.sliceIndex
}

// SetSliceIndex sets the current slice for MPR viewing
func (d *DICOXImage) SetSliceIndex(idx int) {
	d.sliceIndex = idx
}

// MaxSlice returns the maximum slice index for the current orientation
func (d *DICOXImage) MaxSlice() int {
	if !d.hasVolume {
		return 0
	}
	switch d.orientation {
	case Axial:
		return d.volZ - 1
	case Coronal:
		return d.volY - 1
	case Sagittal:
		return d.volX - 1
	}
	return 0
}

// HasVolume returns true if volume data is loaded
func (d *DICOXImage) HasVolume() bool {
	return d.hasVolume
}

// SetCompositeMode enables/disables MIP (Maximum Intensity Projection) composite view.
// When enabled, At() returns the maximum density value along the viewing direction.
func (d *DICOXImage) SetCompositeMode(enabled bool) {
	d.compositeMode = enabled
	mipLogOnce = false // Reset log flag to show status on next render
	slog.Info("DICOXImage composite mode changed", slog.Bool("enabled", enabled),
		slog.Bool("hasVolume", d.hasVolume), slog.String("orientation", d.orientation.String()))
}

// CompositeMode returns whether composite view is enabled.
func (d *DICOXImage) CompositeMode() bool {
	return d.compositeMode
}
