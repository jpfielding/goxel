package dicom

import (
	"fmt"

	"bytes"

	jpegls "github.com/jpfielding/goxel/pkg/compress/jpegls"
)

// NativeFrame represents decoded pixel data compatible with Goxel's interface
type NativeFrame struct {
	rows int
	cols int
	data []int
}

// Rows returns the number of rows
func (nf *NativeFrame) Rows() int {
	return nf.rows
}

// Cols returns the number of columns
func (nf *NativeFrame) Cols() int {
	return nf.cols
}

// GetPixel returns the pixel value at (x, y)
func (nf *NativeFrame) GetPixel(x, y int) ([]int, error) {
	if x < 0 || x >= nf.cols || y < 0 || y >= nf.rows {
		return nil, fmt.Errorf("pixel coordinates out of bounds")
	}
	idx := y*nf.cols + x
	if idx >= len(nf.data) {
		return nil, fmt.Errorf("pixel index out of bounds")
	}
	return []int{nf.data[idx]}, nil
}

// FrameInfo contains frame metadata for Goxel compatibility
type FrameInfo struct {
	IsEncapsulated   bool
	NativeData       *NativeFrame
	EncapsulatedData *EncapsulatedData
}

// EncapsulatedData holds compressed frame data
type EncapsulatedData struct {
	Data []byte
}

// DecodeFrame decodes a frame (either native or encapsulated) into NativeFrame
func DecodeFrame(fi *FrameInfo, rows, cols, bitsAllocated, pixelRep int) (*NativeFrame, error) {
	if !fi.IsEncapsulated {
		// Already native
		return fi.NativeData, nil
	}

	// Decode JPEG-LS
	decoded, err := jpegls.Decode(bytes.NewReader(fi.EncapsulatedData.Data))
	if err != nil {
		return nil, fmt.Errorf("jpeg-ls decode failed: %w", err)
	}

	// Extract pixel data based on image type
	bounds := decoded.Bounds()
	pixelCount := bounds.Dx() * bounds.Dy()
	data := make([]int, pixelCount)

	// Sample a few pixels to debug
	var minR, maxR uint32 = 0xFFFFFFFF, 0

	// IMPORTANT: Use RGBA() to extract values because our JPEG-LS decoder
	// writes to the image but Gray16At().Y sometimes returns 0
	// The analyze tool uses RGBA() and it works correctly
	for y := 0; y < bounds.Dy(); y++ {
		for x := 0; x < bounds.Dx(); x++ {
			idx := y*bounds.Dx() + x
			r, _, _, _ := decoded.At(x, y).RGBA()
			// RGBA() returns values in 0-65535 range
			data[idx] = int(r)
			if r < minR {
				minR = r
			}
			if r > maxR {
				maxR = r
			}
		}
	}

	fmt.Printf("DecodeFrame: bounds=%dx%d, minR=%d, maxR=%d\n", bounds.Dx(), bounds.Dy(), minR, maxR)

	return &NativeFrame{
		rows: bounds.Dy(),
		cols: bounds.Dx(),
		data: data,
	}, nil
}

// ConvertToFrameInfo converts pkg/dicos frames to Goxel-compatible FrameInfo
func ConvertToFrameInfo(pd *PixelData, rows, cols int) []*FrameInfo {
	frames := make([]*FrameInfo, len(pd.Frames))

	for i, frame := range pd.Frames {
		if pd.IsEncapsulated {
			frames[i] = &FrameInfo{
				IsEncapsulated: true,
				EncapsulatedData: &EncapsulatedData{
					Data: frame.CompressedData,
				},
			}
		} else {
			// Native frame - convert uint16 to int
			data := make([]int, len(frame.Data))
			for j, v := range frame.Data {
				data[j] = int(v)
			}
			frames[i] = &FrameInfo{
				IsEncapsulated: false,
				NativeData: &NativeFrame{
					rows: rows,
					cols: cols,
					data: data,
				},
			}
		}
	}

	return frames
}

// GetValue extracts a typed value from an Element
func (elem *Element) GetValue() interface{} {
	return elem.Value
}

// GetDecodedPixelData convenience function to get all pixel data from a Dataset as uint16 slice
func GetDecodedPixelData(ds *Dataset) ([]uint16, error) {
	// Get Metadata
	rows := GetRows(ds)
	cols := GetColumns(ds)
	bitsAllocated := GetBitsAllocated(ds)
	pixelRep := GetPixelRepresentation(ds)

	// Get Pixel Data Element
	pd, err := ds.GetPixelData()
	if err != nil {
		return nil, err
	}

	// Create FrameInfo
	fi := ConvertToFrameInfo(pd, rows, cols)

	// Decode all frames
	pixelCount := rows * cols * len(fi)
	result := make([]uint16, 0, pixelCount)

	for _, f := range fi {
		decodedFrame, err := DecodeFrame(f, rows, cols, bitsAllocated, pixelRep)
		if err != nil {
			return nil, err
		}

		// Convert []int to []uint16
		for _, v := range decodedFrame.data {
			result = append(result, uint16(v))
		}
	}

	return result, nil
}
