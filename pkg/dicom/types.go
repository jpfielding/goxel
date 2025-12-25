package dicom

import (
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
	"github.com/jpfielding/goxel/pkg/dicom/transfer"
)

// Dataset represents a complete DICOM dataset
type Dataset struct {
	Elements map[Tag]*Element
}

// Element represents a single DICOM element
type Element struct {
	Tag   Tag
	VR    string      // Value Representation
	Value interface{} // Parsed value
}

// Tag alias to avoid duplication
type Tag = tag.Tag

// PixelData represents pixel data (native or encapsulated)
type PixelData struct {
	IsEncapsulated bool
	Frames         []Frame
	Offsets        []uint32 // Basic Offset Table for encapsulated data
}

// Frame represents a single frame of pixel data
type Frame struct {
	// For native (uncompressed) data
	Data []uint16

	// For encapsulated (compressed) data
	CompressedData []byte
}

// GetFlatData returns all pixel data flattened into a single slice.
// Only valid for native pixel data.
func (pd *PixelData) GetFlatData() []uint16 {
	if pd.IsEncapsulated {
		return nil
	}
	var totalPixels int
	for _, f := range pd.Frames {
		totalPixels += len(f.Data)
	}
	res := make([]uint16, totalPixels)
	offset := 0
	for _, f := range pd.Frames {
		copy(res[offset:], f.Data)
		offset += len(f.Data)
	}
	return res
}

// GetFrame returns the frame at the specified index.
func (pd *PixelData) GetFrame(index int) (*Frame, error) {
	if index < 0 || index >= len(pd.Frames) {
		return nil, fmt.Errorf("frame index %d out of bounds (0-%d)", index, len(pd.Frames)-1)
	}
	return &pd.Frames[index], nil
}

// NumFrames returns the number of frames in the pixel data.
func (pd *PixelData) NumFrames() int {
	return len(pd.Frames)
}

// IsCompressed returns true if the pixel data is encapsulated (compressed).
func (pd *PixelData) IsCompressed() bool {
	return pd.IsEncapsulated
}

// HasFrames returns true if the pixel data contains at least one frame.
func (pd *PixelData) HasFrames() bool {
	return len(pd.Frames) > 0
}

// FrameSize returns the number of pixels in the first frame, or 0 if no frames exist.
// For compressed data, the size is unknown until decompression.
func (pd *PixelData) FrameSize() int {
	if len(pd.Frames) == 0 || pd.IsEncapsulated {
		return 0
	}
	return len(pd.Frames[0].Data)
}

// TotalPixels returns the total number of pixels across all frames.
// Returns 0 for encapsulated (compressed) data.
func (pd *PixelData) TotalPixels() int {
	if pd.IsEncapsulated {
		return 0
	}
	total := 0
	for _, f := range pd.Frames {
		total += len(f.Data)
	}
	return total
}

// FindElement returns an element by tag
func (ds *Dataset) FindElement(group, element uint16) (*Element, bool) {
	elem, ok := ds.Elements[Tag{Group: group, Element: element}]
	return elem, ok
}

// Rows returns the number of rows (image height).
func (ds *Dataset) Rows() int {
	if elem, ok := ds.FindElement(tag.Rows.Group, tag.Rows.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// Columns returns the number of columns (image width).
func (ds *Dataset) Columns() int {
	if elem, ok := ds.FindElement(tag.Columns.Group, tag.Columns.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// NumberOfFrames returns the number of frames. Defaults to 1.
func (ds *Dataset) NumberOfFrames() int {
	if elem, ok := ds.FindElement(tag.NumberOfFrames.Group, tag.NumberOfFrames.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
		if s, ok := elem.GetString(); ok {
			var n int
			fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
			return n
		}
	}
	return 1
}

// BitsAllocated returns bits allocated per sample. Defaults to 16.
func (ds *Dataset) BitsAllocated() int {
	if elem, ok := ds.FindElement(tag.BitsAllocated.Group, tag.BitsAllocated.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 16
}

// PixelRepresentation returns 0 for unsigned, 1 for signed. Defaults to 0.
func (ds *Dataset) PixelRepresentation() int {
	if elem, ok := ds.FindElement(tag.PixelRepresentation.Group, tag.PixelRepresentation.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// Modality returns the modality string.
func (ds *Dataset) Modality() string {
	if elem, ok := ds.FindElement(tag.Modality.Group, tag.Modality.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// TransferSyntax returns the dataset transfer syntax. Defaults to Explicit VR Little Endian.
func (ds *Dataset) TransferSyntax() transfer.Syntax {
	if elem, ok := ds.FindElement(tag.TransferSyntaxUID.Group, tag.TransferSyntaxUID.Element); ok {
		if s, ok := elem.GetString(); ok {
			return transfer.FromUID(strings.TrimSpace(s))
		}
	}
	return transfer.ExplicitVRLittleEndian
}

// IsEncapsulated returns true if the dataset's pixel data is encapsulated (compressed).
func (ds *Dataset) IsEncapsulated() bool {
	return ds.TransferSyntax().IsEncapsulated()
}

// GetString returns a string value from an element
func (elem *Element) GetString() (string, bool) {
	if s, ok := elem.Value.(string); ok {
		return s, true
	}
	return "", false
}

// GetUint16 returns a uint16 value from an element
func (elem *Element) GetUint16() (uint16, bool) {
	if u, ok := elem.Value.(uint16); ok {
		return u, true
	}
	return 0, false
}

// GetUint32 returns a uint32 value from an element
func (elem *Element) GetUint32() (uint32, bool) {
	if u, ok := elem.Value.(uint32); ok {
		return u, true
	}
	return 0, false
}

// GetInt returns an int value from an element
func (elem *Element) GetInt() (int, bool) {
	switch v := elem.Value.(type) {
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case int:
		return v, true
	case int32:
		return int(v), true
	case string:
		var i int
		if _, err := fmt.Sscanf(v, "%d", &i); err == nil {
			return i, true
		}
	case []byte:
		if len(v) == 2 {
			return int(binary.LittleEndian.Uint16(v)), true
		}
		if len(v) == 4 {
			return int(binary.LittleEndian.Uint32(v)), true
		}
	}
	return 0, false
}

// GetInts returns a slice of ints from an element
func (elem *Element) GetInts() ([]int, bool) {
	switch v := elem.Value.(type) {
	case []uint16:
		res := make([]int, len(v))
		for i, val := range v {
			res[i] = int(val)
		}
		return res, true
	case []uint32:
		res := make([]int, len(v))
		for i, val := range v {
			res[i] = int(val)
		}
		return res, true
	case []int:
		return v, true
	case []byte:
		if len(v)%2 == 0 {
			res := make([]int, len(v)/2)
			for i := 0; i < len(res); i++ {
				res[i] = int(binary.LittleEndian.Uint16(v[i*2:]))
			}
			return res, true
		}
	}
	return nil, false
}

// GetFloats returns a slice of float64s from an element
func (elem *Element) GetFloats() ([]float64, bool) {
	switch v := elem.Value.(type) {
	case []float32:
		res := make([]float64, len(v))
		for i, val := range v {
			res[i] = float64(val)
		}
		return res, true
	case []float64:
		return v, true
	case float32:
		return []float64{float64(v)}, true
	case float64:
		return []float64{v}, true
	}
	return nil, false
}

// GetPixelData returns pixel data from an element
func (elem *Element) GetPixelData() (*PixelData, bool) {
	if pd, ok := elem.Value.(*PixelData); ok {
		return pd, true
	}
	return nil, false
}
