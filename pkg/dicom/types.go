package dicom

import (
	"encoding/binary"
	"fmt"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
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

// FindElement returns an element by tag
func (ds *Dataset) FindElement(group, element uint16) (*Element, bool) {
	elem, ok := ds.Elements[Tag{Group: group, Element: element}]
	return elem, ok
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
