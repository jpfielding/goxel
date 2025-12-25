package dicom

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// String returns a string representation of the Element
func (e *Element) String() string {
	// Format: [Tag] [VR] (Name) ... : Value
	tagName := e.Tag.LookupName()
	if tagName != "" {
		tagName = " " + tagName
	}

	valStr := ""
	switch v := e.Value.(type) {
	case *PixelData:
		valStr = fmt.Sprintf("Pixel Data (%d frames)", len(v.Frames))
	case []uint16:
		if len(v) > 10 {
			valStr = fmt.Sprintf("Array of %d params", len(v))
		} else {
			valStr = fmt.Sprintf("%v", v)
		}
	case []byte:
		if len(v) > 20 {
			valStr = fmt.Sprintf("Binary Data (%d bytes)", len(v))
		} else {
			valStr = fmt.Sprintf("%v", v)
		}
	default:
		valStr = fmt.Sprintf("%v", v)
	}

	return fmt.Sprintf("[%s] %s%s: %s", e.Tag, e.VR, tagName, valStr)
}

// MarshalJSON returns a JSON representation of the Element
func (e *Element) MarshalJSON() ([]byte, error) {
	// We want a cleaner JSON object
	type Alias Element
	return json.Marshal(&struct {
		Tag   string      `json:"tag"`
		Name  string      `json:"name,omitempty"`
		VR    string      `json:"vr"`
		Value interface{} `json:"value"`
	}{
		Tag:   e.Tag.String(),
		Name:  e.Tag.LookupName(),
		VR:    e.VR,
		Value: e.Value,
	})
}

// String returns a string representation of the Dataset
func (ds *Dataset) String() string {
	if ds == nil {
		return "<nil>"
	}
	// Sort by Tag
	var keys []Tag
	for k := range ds.Elements {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Group != keys[j].Group {
			return keys[i].Group < keys[j].Group
		}
		return keys[i].Element < keys[j].Element
	})

	var b strings.Builder
	for _, k := range keys {
		elem := ds.Elements[k]
		b.WriteString(elem.String())
		b.WriteString("\n")
	}
	return b.String()
}

// MarshalJSON returns a JSON representation of the Dataset
// It returns a sorted array of Elements instead of a Map
func (ds *Dataset) MarshalJSON() ([]byte, error) {
	// Sort by Tag
	var keys []Tag
	for k := range ds.Elements {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].Group != keys[j].Group {
			return keys[i].Group < keys[j].Group
		}
		return keys[i].Element < keys[j].Element
	})

	var elements []*Element
	for _, k := range keys {
		elements = append(elements, ds.Elements[k])
	}
	return json.Marshal(elements)
}
