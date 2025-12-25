package dicom

import (
	"bytes"
	"fmt"
	"image"
	"log/slog"

	jpeg2k "github.com/jpfielding/goxel/pkg/compress/jpeg2k"
	jpegli "github.com/jpfielding/goxel/pkg/compress/jpegli"
	jpegls "github.com/jpfielding/goxel/pkg/compress/jpegls"
	rle "github.com/jpfielding/goxel/pkg/compress/rle"
)

// DecodeVolume decodes all frames from a Dataset into a Volume
// Handles both native (uncompressed) and encapsulated (JPEG-LS, JPEG Lossless) pixel data
func DecodeVolume(ds *Dataset) (*Volume, error) {
	rows := GetRows(ds)
	cols := GetColumns(ds)

	if rows == 0 || cols == 0 {
		return nil, fmt.Errorf("invalid dimensions: %dx%d", cols, rows)
	}

	pd, err := ds.GetPixelData()
	if err != nil {
		return nil, err
	}

	// Use actual frame count from pixel data (more reliable than NumberOfFrames tag)
	numFrames := len(pd.Frames)
	if numFrames == 0 {
		numFrames = GetNumberOfFrames(ds) // Fallback to tag
	}
	if numFrames == 0 {
		numFrames = 1
	}

	vol := NewVolume(cols, rows, numFrames)

	if pd.IsEncapsulated {
		// Determine Transfer Syntax
		ts := GetTransferSyntax(ds)

		// Decode each compressed frame
		for z, frame := range pd.Frames {
			var img image.Image
			// This nested check is redundant but kept as per instruction
			if pd.IsEncapsulated {
				decoded, err := decodeCompressedFrame(frame.CompressedData, rows, cols, ts)
				if err != nil {
					return nil, fmt.Errorf("decoding frame %d: %w", z, err)
				}
				img = decoded
			} else {
				// Should not happen
				return nil, fmt.Errorf("unexpected non-encapsulated data in encapsulated block for frame %d", z)
			}

			bounds := img.Bounds()
			imgWidth := bounds.Dx()
			imgHeight := bounds.Dy()
			sliceOffset := z * vol.Width * vol.Height

			// Log dimension mismatch if any (first frame only)
			if z == 0 && (imgWidth != vol.Width || imgHeight != vol.Height) {
				slog.Warn("Decoded image mismatch",
					"width", imgWidth, "height", imgHeight,
					"expected_width", vol.Width, "expected_height", vol.Height)
			}

			// Extract pixel values using RGBA (proven to work correctly)
			// Use the actual image dimensions for iteration
			for y := 0; y < imgHeight && y < vol.Height; y++ {
				for x := 0; x < imgWidth && x < vol.Width; x++ {
					r, _, _, _ := img.At(x, y).RGBA() // Use 'img' here
					// RGBA() returns 0-65535, store as uint16
					// Use vol.Width for stride (not imgWidth) to match Volume layout
					if idx := sliceOffset + y*vol.Width + x; idx < len(vol.Data) {
						vol.Data[idx] = uint16(r)
					}
				}
			}
		}
	} else {
		// Native pixel data - copy directly
		idx := 0
		for _, frame := range pd.Frames {
			for _, val := range frame.Data {
				if idx < len(vol.Data) {
					vol.Data[idx] = val
					idx++
				}
			}
		}
	}

	return vol, nil
}

// decodeCompressedFrame detects compression type and decodes
func decodeCompressedFrame(data []byte, rows, cols int, ts TransferSyntax) (image.Image, error) {
	if len(data) < 12 && len(data) < 64 { // Allow short data implies not RLE/JPEG anyway, but RLE header check is inside
		// RLE header is 64 bytes. JPEG header is small but valid stream is > 12 usually.
		if len(data) < 2 {
			return nil, fmt.Errorf("compressed data too short: %d bytes", len(data))
		}
	}

	// 1. Use Transfer Syntax if available
	tsUID := string(ts)
	if tsUID == "1.2.840.10008.1.2.4.70" { // JPEG Lossless
		return jpegli.Decode(bytes.NewReader(data))
	}
	if tsUID == "1.2.840.10008.1.2.5" { // RLE Lossless
		return rle.Decode(data, cols, rows)
	}
	if tsUID == "1.2.840.10008.1.2.4.90" { // JPEG 2000
		return jpeg2k.Decode(bytes.NewReader(data))
	}
	// JPEGLS: 1.2.840.10008.1.2.4.80 or .81
	if tsUID == "1.2.840.10008.1.2.4.80" || tsUID == "1.2.840.10008.1.2.4.81" {
		return jpegls.Decode(bytes.NewReader(data))
	}

	// 2. Fallback to sniffing if TS is unknown or generic
	// Check for JPEG SOI marker
	isJPEGLS := false
	isJPEGLossless := false
	isJ2K := false

	// Strict check for JPEG SOI (FF D8) or J2K SOC (FF 4F) at start
	if len(data) > 2 {
		if data[0] == 0xFF && data[1] == 0xF7 {
			// JPEGLS might lack SOI? No standard says FF D8.
			// But existing code checked for F7 anywhere.
		}

		// Scan only if starts with FF D8
		if data[0] == 0xFF && data[1] == 0xD8 {
			// Scan for SOF
			for i := 0; i < len(data)-1; i++ {
				if data[i] == 0xFF {
					switch data[i+1] {
					case 0xF7:
						isJPEGLS = true
					case 0xC3:
						isJPEGLossless = true
					}
				}
			}
		} else if data[0] == 0xFF && data[1] == 0x4F {
			isJ2K = true
		}
	}

	if isJPEGLS {
		return jpegls.Decode(bytes.NewReader(data))
	}

	if isJPEGLossless {
		return jpegli.Decode(bytes.NewReader(data))
	}

	if isJ2K {
		return jpeg2k.Decode(bytes.NewReader(data))
	}

	// Check for RLE
	// RLE Header is 64 bytes. First 4 bytes = uint32 segment count.
	if len(data) >= 64 {
		img, err := rle.Decode(data, cols, rows)
		if err == nil {
			return img, nil
		}

		slog.Debug("RLE decode attempt failed",
			slog.Any("error", err),
			slog.Int("dataLen", len(data)),
			slog.Int("cols", cols),
			slog.Int("rows", rows))
	}

	// Fallback: Try JPEG Lossless first (more common in DICOM), then JPEG-LS
	img, err := jpegli.Decode(bytes.NewReader(data))
	if err == nil {
		return img, nil
	}

	return jpegls.Decode(bytes.NewReader(data))
}

// DecodeFrameData decodes a single frame from pixel data
// Returns raw uint16 pixel values
func DecodeFrameData(pd *PixelData, frameIndex int, rows, cols int, ts TransferSyntax) ([]uint16, error) {
	if frameIndex < 0 || frameIndex >= len(pd.Frames) {
		return nil, fmt.Errorf("frame index %d out of range (0-%d)", frameIndex, len(pd.Frames)-1)
	}

	frame := pd.Frames[frameIndex]
	pixelCount := rows * cols
	data := make([]uint16, pixelCount)

	if pd.IsEncapsulated {
		decoded, err := decodeCompressedFrame(frame.CompressedData, rows, cols, ts)
		if err != nil {
			return nil, fmt.Errorf("decode failed: %w", err)
		}

		bounds := decoded.Bounds()
		for y := 0; y < bounds.Dy() && y < rows; y++ {
			for x := 0; x < bounds.Dx() && x < cols; x++ {
				r, _, _, _ := decoded.At(x, y).RGBA()
				idx := y*cols + x
				if idx < len(data) {
					data[idx] = uint16(r)
				}
			}
		}
	} else {
		// Native - copy directly
		copy(data, frame.Data)
	}

	return data, nil
}

// GetWindowLevel returns the window center and width from the dataset
func GetWindowLevel(ds *Dataset) (center, width int) {
	center, width = 40, 400 // CT soft tissue defaults

	if elem, ok := ds.FindElement(0x0028, 0x1050); ok { // Window Center
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%d", &center)
		} else if v, ok := elem.GetInt(); ok {
			center = v
		}
	}

	if elem, ok := ds.FindElement(0x0028, 0x1051); ok { // Window Width
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%d", &width)
		} else if v, ok := elem.GetInt(); ok {
			width = v
		}
	}

	return
}

// GetPixelSpacing returns the pixel spacing in mm
func GetPixelSpacing(ds *Dataset) (row, col float64) {
	row, col = 1.0, 1.0 // Defaults

	if elem, ok := ds.FindElement(0x0028, 0x0030); ok { // Pixel Spacing
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f\\%f", &row, &col)
		}
	}

	return
}

// GetSliceThickness returns the slice thickness in mm
func GetSliceThickness(ds *Dataset) float64 {
	if elem, ok := ds.FindElement(0x0018, 0x0050); ok {
		if s, ok := elem.GetString(); ok {
			var thickness float64
			fmt.Sscanf(s, "%f", &thickness)
			return thickness
		}
	}
	return 1.0 // Default
}

// GetImagePositionPatient returns the position of the image origin
func GetImagePositionPatient(ds *Dataset) []float64 {
	if elem, ok := ds.FindElement(0x0020, 0x0032); ok {
		if s, ok := elem.GetString(); ok {
			var x, y, z float64
			if _, err := fmt.Sscanf(s, "%f\\%f\\%f", &x, &y, &z); err == nil {
				return []float64{x, y, z}
			}
		}
	}
	// Default to 0,0,0
	return []float64{0.0, 0.0, 0.0}
}

// GetImageOrientationPatient returns the orientation cosines
func GetImageOrientationPatient(ds *Dataset) []float64 {
	if elem, ok := ds.FindElement(0x0020, 0x0037); ok {
		if s, ok := elem.GetString(); ok {
			var r1, r2, r3, c1, c2, c3 float64
			if _, err := fmt.Sscanf(s, "%f\\%f\\%f\\%f\\%f\\%f", &r1, &r2, &r3, &c1, &c2, &c3); err == nil {
				return []float64{r1, r2, r3, c1, c2, c3}
			}
		}
	}
	// Default to Identity
	return []float64{1.0, 0.0, 0.0, 0.0, 1.0, 0.0}
}
