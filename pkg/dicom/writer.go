package dicom

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"math"
	"os"
	"sort"
	"sync/atomic"
)

// WriteFile writes a dataset to a DICOM file
func WriteFile(path string, ds *Dataset) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return Write(f, ds)
}

// Write writes a dataset to a writer using Explicit VR Little Endian
func Write(w io.Writer, ds *Dataset) (int64, error) {
	cw := &CountingWriter{Writer: w}

	// 1. Write Preamble (128 bytes 0x00)
	preamble := make([]byte, 128)
	if _, err := cw.Write(preamble); err != nil {
		return cw.Count.Load(), err
	}

	// 2. Write DICM Magic
	if _, err := cw.Write([]byte("DICM")); err != nil {
		return cw.Count.Load(), err
	}

	// 3. Write Dataset Elements
	return writeDataSetBody(w, ds)
}

func writeDataSetBody(w io.Writer, ds *Dataset) (int64, error) {
	// 3. Collect elements and sort by Tag
	var elements []*Element
	for _, elem := range ds.Elements {
		elements = append(elements, elem)
	}

	sort.Slice(elements, func(i, j int) bool {
		t1 := elements[i].Tag
		t2 := elements[j].Tag
		if t1.Group != t2.Group {
			return t1.Group < t2.Group
		}
		return t1.Element < t2.Element
	})

	cw := &CountingWriter{Writer: w}

	// Write elements
	for _, elem := range elements {
		if _, err := writeElement(cw, elem); err != nil {
			return cw.Count.Load(), fmt.Errorf("failed to write element %v: %w", elem.Tag, err)
		}
	}

	return cw.Count.Load(), nil
}

func writeElement(w io.Writer, elem *Element) (int, error) {
	cw := &CountingWriter{Writer: w}

	// Write Tag
	if err := binary.Write(cw, binary.LittleEndian, elem.Tag.Group); err != nil {
		return int(cw.Count.Load()), err
	}
	if err := binary.Write(cw, binary.LittleEndian, elem.Tag.Element); err != nil {
		return int(cw.Count.Load()), err
	}

	// Write VR
	vr := elem.VR
	if len(vr) != 2 {
		slog.Warn("Invalid VR length, defaulting to UN", "vr", vr, "tag", elem.Tag)
		vr = "UN"
	}
	if _, err := cw.Write([]byte(vr)); err != nil {
		return int(cw.Count.Load()), err
	}

	// Encode Value
	valBytes, isUndefinedLength, err := encodeValue(elem.Value, vr)
	if err != nil {
		return int(cw.Count.Load()), err
	}

	// Write Length and Value
	if isLongVR(vr) {
		// Reserved 2 bytes (0x00)
		if _, err := cw.Write([]byte{0, 0}); err != nil {
			return int(cw.Count.Load()), err
		}

		length := uint32(len(valBytes))
		if isUndefinedLength {
			length = 0xFFFFFFFF
		}

		if err := binary.Write(cw, binary.LittleEndian, length); err != nil {
			return int(cw.Count.Load()), err
		}
	} else {
		// Short VR
		if isUndefinedLength {
			return int(cw.Count.Load()), fmt.Errorf("undefined length not supported for Short VR %s", vr)
		}
		length := uint16(len(valBytes))
		if err := binary.Write(w, binary.LittleEndian, length); err != nil {
			return int(cw.Count.Load()), err
		}
	}

	// Write Value Bytes
	if _, err := cw.Write(valBytes); err != nil {
		return int(cw.Count.Load()), err
	}

	return int(cw.Count.Load()), nil
}

// encodeValue returns encoded bytes and a bool indicating if undefined length used (e.g. encapsulated pixels)
func encodeValue(v interface{}, vr string) ([]byte, bool, error) {
	if v == nil {
		return []byte{}, false, nil
	}

	// Special case: PixelData
	if pd, ok := v.(*PixelData); ok {
		if pd.IsEncapsulated {
			b, err := encodeEncapsulatedPixelData(pd)
			return b, true, err // Undefined Length
		}
		// Native Pixel Data (falls through to []uint16 handling usually)
		// But PixelData struct holds Frames []Frame.
		// We need to flatten native frames.
		return encodeNativePixelData(pd)
	}

	switch val := v.(type) {
	case []*Dataset:
		// Sequence Logic
		if vr == "SQ" {
			b, err := encodeSequence(val)
			return b, true, err // Undefined Length for Sequence is typical/robust
		}
		return nil, false, fmt.Errorf("unexpected []*Dataset for VR %s", vr)
	case string:
		// Pad with space if odd
		b := []byte(val)
		if len(b)%2 != 0 {
			b = append(b, ' ') // Default padding char
		}
		return b, false, nil
	case []string:
		// Multi-valued string (backslash separated)
		joined := ""
		for i, s := range val {
			if i > 0 {
				joined += "\\"
			}
			joined += s
		}
		b := []byte(joined)
		if len(b)%2 != 0 {
			b = append(b, ' ')
		}
		return b, false, nil
	case uint16:
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, val)
		return b, false, nil
	case []uint16:
		b := make([]byte, len(val)*2)
		for i, u := range val {
			binary.LittleEndian.PutUint16(b[i*2:], u)
		}
		return b, false, nil
	case int:
		// Map int to VR size. Usually US or SS or SL or UL.
		// If VR is US, assume uint16.
		switch vr {
		case "US":
			b := make([]byte, 2)
			binary.LittleEndian.PutUint16(b, uint16(val))
			return b, false, nil
		case "UL", "SL":
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, uint32(val))
			return b, false, nil
		}
		// Fallback/Default
		b := make([]byte, 2)
		binary.LittleEndian.PutUint16(b, uint16(val))
		return b, false, nil
	case float64:
		// If DS, encode as string. If FL/FD, binary.
		switch vr {
		case "DS":
			s := fmt.Sprintf("%v", val)
			if len(s)%2 != 0 {
				s += " "
			}
			return []byte(s), false, nil
		case "FD":
			b := make([]byte, 8)
			binary.LittleEndian.PutUint64(b, math.Float64bits(val))
			return b, false, nil
		case "FL":
			b := make([]byte, 4)
			binary.LittleEndian.PutUint32(b, math.Float32bits(float32(val)))
			return b, false, nil
		}
		return nil, false, fmt.Errorf("float64 for VR %s not implemented", vr)
	case []float32:
		b := make([]byte, len(val)*4)
		for i, f := range val {
			binary.LittleEndian.PutUint32(b[i*4:], math.Float32bits(f))
		}
		return b, false, nil
	case []byte:
		return val, false, nil
	}

	return nil, false, fmt.Errorf("unsupported value type %T for VR %s", v, vr)
}

func encodeSequence(datasets []*Dataset) ([]byte, error) {
	var buf bytes.Buffer

	for _, ds := range datasets {
		// Item Tag (FFFE, E000)
		buf.Write([]byte{0xFE, 0xFF, 0x00, 0xE0})

		// Encode Dataset Body to temp buffer to get length
		var dsBuf bytes.Buffer
		if _, err := writeDataSetBody(&dsBuf, ds); err != nil {
			return nil, fmt.Errorf("failed to encode sequence item: %w", err)
		}
		dsBytes := dsBuf.Bytes()

		// Item Length (Explicit)
		length := uint32(len(dsBytes))
		binary.Write(&buf, binary.LittleEndian, length)

		// Item Data
		buf.Write(dsBytes)
	}

	// Sequence Delimitation Item (FFFE, E0DD)
	buf.Write([]byte{0xFE, 0xFF, 0xDD, 0xE0})
	// Length 0
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00})

	return buf.Bytes(), nil
}

func encodeNativePixelData(pd *PixelData) ([]byte, bool, error) {
	// Provide flat byte buffer of native data
	var buf bytes.Buffer
	for _, frame := range pd.Frames {
		// Native data is []uint16 (dicos.Frame)
		// We assume 16-bit.
		for _, pixel := range frame.Data {
			binary.Write(&buf, binary.LittleEndian, pixel)
		}
	}
	return buf.Bytes(), false, nil
}

func encodeEncapsulatedPixelData(pd *PixelData) ([]byte, error) {
	var buf bytes.Buffer

	// 1. Basic Offset Table (Item Tag)
	// Tag FFFE,E000: Item
	buf.Write([]byte{0xFE, 0xFF, 0x00, 0xE0})

	// Length of BOT
	botLen := uint32(len(pd.Offsets) * 4)
	binary.Write(&buf, binary.LittleEndian, botLen)

	// Offsets
	for _, off := range pd.Offsets {
		binary.Write(&buf, binary.LittleEndian, off)
	}

	// 2. Frames (Items)
	for _, frame := range pd.Frames {
		// Item Tag
		buf.Write([]byte{0xFE, 0xFF, 0x00, 0xE0})

		// Length
		itemLen := uint32(len(frame.CompressedData))
		binary.Write(&buf, binary.LittleEndian, itemLen)

		// Data
		buf.Write(frame.CompressedData)
	}

	// 3. Sequence Delimitation Item
	// Tag FFFE,E0DD
	buf.Write([]byte{0xFE, 0xFF, 0xDD, 0xE0})
	// Length 0
	buf.Write([]byte{0x00, 0x00, 0x00, 0x00})

	return buf.Bytes(), nil
}

type CountingWriter struct {
	Count  atomic.Int64
	Writer io.Writer
}

func (c *CountingWriter) Write(p []byte) (int, error) {
	n, err := c.Writer.Write(p)
	if err == nil {
		c.Count.Add(int64(n))
	}
	return n, err
}
