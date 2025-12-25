package dicom

import (
	"fmt"
	"image"
	"log/slog"
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
	if len(data) < 2 {
		return nil, fmt.Errorf("compressed data too short: %d bytes", len(data))
	}

	// 1. Use Transfer Syntax if available - lookup via codec map
	tsUID := string(ts)
	if codec := CodecByTransferSyntax(tsUID); codec != nil {
		return codec.Decode(data, cols, rows)
	}

	// 2. Fallback to sniffing if TS is unknown or generic
	var detectedCodec Codec

	if len(data) > 2 {
		// Check for JPEG SOI (FF D8) or J2K SOC (FF 4F) at start
		if data[0] == 0xFF && data[1] == 0xD8 {
			// Scan for SOF marker to determine JPEG type
			for i := 0; i < len(data)-1; i++ {
				if data[i] == 0xFF {
					switch data[i+1] {
					case 0xF7: // JPEG-LS SOF55
						detectedCodec = CodecJPEGLS
					case 0xC3: // JPEG Lossless SOF3
						detectedCodec = CodecJPEGLi
					}
				}
			}
		} else if data[0] == 0xFF && data[1] == 0x4F {
			// JPEG 2000 SOC marker
			detectedCodec = CodecJPEG2000
		}
	}

	if detectedCodec != nil {
		return detectedCodec.Decode(data, cols, rows)
	}

	// Check for RLE (header is 64 bytes)
	if len(data) >= 64 {
		img, err := CodecRLE.Decode(data, cols, rows)
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
	img, err := CodecJPEGLi.Decode(data, cols, rows)
	if err == nil {
		return img, nil
	}

	return CodecJPEGLS.Decode(data, cols, rows)
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
