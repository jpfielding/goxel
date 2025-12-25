package dicom

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Reader reads DICOM/DICOM files
type Reader struct {
	r              io.Reader
	transferSyntax string
	explicitVR     bool
	littleEndian   bool
}

// NewReader creates a new DICOM reader
func NewReader(r io.Reader) *Reader {
	return &Reader{
		r:            r,
		explicitVR:   true,
		littleEndian: true,
	}
}

// Parse reads a complete DICOM file
func Parse(r io.Reader) (*Dataset, error) {
	reader := NewReader(r)
	return reader.ReadDataset()
}

// ReadDataset reads the complete dataset
func (r *Reader) ReadDataset() (*Dataset, error) {
	ds := &Dataset{
		Elements: make(map[Tag]*Element),
	}

	// Read preamble (128 bytes) and DICM magic
	preamble := make([]byte, 128)
	if _, err := io.ReadFull(r.r, preamble); err != nil {
		return nil, fmt.Errorf("failed to read preamble: %w", err)
	}

	magic := make([]byte, 4)
	if _, err := io.ReadFull(r.r, magic); err != nil {
		return nil, fmt.Errorf("failed to read DICM magic: %w", err)
	}
	if string(magic) != "DICM" {
		return nil, errors.New("invalid DICOM file: missing DICM magic")
	}

	// Group 0002 (File Meta Information) is ALWAYS Explicit VR Little Endian
	r.explicitVR = true
	r.littleEndian = true

	// Read dataset elements
	for {
		tag, err := r.readTag()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read tag: %w", err)
		}

		// Transition from File Meta to Dataset?
		if tag.Group != 0x0002 && r.transferSyntax == "" {
			// If we hit a non-meta tag but haven't seen TransferSyntaxUID yet,
			// or if we just finished meta group, we need to update settings.
			// Default to Implicit VR if no File Meta was found
			r.transferSyntax = "1.2.840.10008.1.2" // Implicit VR Little Endian
			r.updateTransferSyntax()
		}

		elem, err := r.readElementWithTag(tag)
		if err != nil {
			return nil, fmt.Errorf("failed to read element %v: %w", tag, err)
		}

		ds.Elements[elem.Tag] = elem

		// If this was TransferSyntaxUID, update settings for the REST of the file
		if tag.Group == 0x0002 && tag.Element == 0x0010 {
			if tsStr, ok := elem.Value.(string); ok {
				r.transferSyntax = tsStr
				r.updateTransferSyntax()
			}
		}
	}

	return ds, nil
}

// readElementWithTag reads a DICOM element after the tag has been read
func (r *Reader) readElementWithTag(tag Tag) (*Element, error) {
	var vr string
	var vl uint32

	if r.explicitVR {
		// Read VR (2 bytes)
		vrBytes := make([]byte, 2)
		if _, err := io.ReadFull(r.r, vrBytes); err != nil {
			return nil, err
		}
		vr = string(vrBytes)

		// Check if VR uses 4-byte VL or 2-byte VL + 2 reserved bytes
		if isLongVR(vr) {
			// Reserved 2 bytes
			reserved := make([]byte, 2)
			if _, err := io.ReadFull(r.r, reserved); err != nil {
				return nil, err
			}
			// VL is 4 bytes
			if err := binary.Read(r.r, binary.LittleEndian, &vl); err != nil {
				return nil, err
			}
		} else {
			// VL is 2 bytes
			var vl16 uint16
			if err := binary.Read(r.r, binary.LittleEndian, &vl16); err != nil {
				return nil, err
			}
			vl = uint32(vl16)
		}
	} else {
		// Implicit VR: VL is always 4 bytes, VR is determined by tag
		if err := binary.Read(r.r, binary.LittleEndian, &vl); err != nil {
			return nil, err
		}
		vr = getImplicitVR(tag)
	}

	// Read value
	value, err := r.readValue(tag, vr, vl)
	if err != nil {
		return nil, err
	}

	return &Element{
		Tag:   tag,
		VR:    vr,
		Value: value,
	}, nil
}

// readTag reads a DICOM tag
func (r *Reader) readTag() (Tag, error) {
	var group, element uint16
	if err := binary.Read(r.r, binary.LittleEndian, &group); err != nil {
		return Tag{}, err
	}
	if err := binary.Read(r.r, binary.LittleEndian, &element); err != nil {
		return Tag{}, err
	}
	return Tag{Group: group, Element: element}, nil
}

// readValue reads the value based on VR and VL
func (r *Reader) readValue(tag Tag, vr string, vl uint32) (interface{}, error) {
	// Handle undefined length
	if vl == 0xFFFFFFFF {
		return r.readUndefinedLengthValue(tag, vr)
	}

	// Read fixed-length value
	data := make([]byte, vl)
	if _, err := io.ReadFull(r.r, data); err != nil {
		return nil, err
	}

	// Parse based on VR
	return parseValue(vr, data)
}

// readUndefinedLengthValue handles pixel data and sequences with undefined length
func (r *Reader) readUndefinedLengthValue(tag Tag, _ string) (interface{}, error) {
	// This is typically used for encapsulated pixel data
	if tag.Group == 0x7FE0 && tag.Element == 0x0010 {
		return r.readEncapsulatedPixelData()
	}

	// Handle sequences with undefined length (VR = SQ)
	// Skip the entire sequence by reading until Sequence Delimitation Item
	return r.skipUndefinedLengthSequence()
}

// skipUndefinedLengthSequence skips over a sequence with undefined length
// Reads until Sequence Delimitation Item (FFFE,E0DD)
func (r *Reader) skipUndefinedLengthSequence() (interface{}, error) {
	for {
		itemTag, err := r.readTag()
		if err != nil {
			if err == io.EOF {
				return nil, nil // End of file is OK
			}
			return nil, fmt.Errorf("reading sequence item tag: %w", err)
		}

		// Check for sequence/item delimiters (these have 4-byte zero length, no VR)
		if itemTag.Group == 0xFFFE {
			var delimLen uint32
			if err := binary.Read(r.r, binary.LittleEndian, &delimLen); err != nil {
				return nil, fmt.Errorf("reading delimiter length: %w", err)
			}

			switch itemTag.Element {
			case 0xE0DD: // Sequence Delimitation
				return nil, nil
			case 0xE00D: // Item Delimitation
				// Continue reading
				continue
			case 0xE000: // Item Start
				if delimLen != 0xFFFFFFFF && delimLen > 0 {
					// Fixed length item - skip entire content
					if _, err := io.CopyN(io.Discard, r.r, int64(delimLen)); err != nil {
						return nil, fmt.Errorf("skipping item data: %w", err)
					}
				}
				// Undefined length items - continue reading nested elements
				continue
			}
		}

		// Regular element - parse VR and skip
		var vl uint32
		if r.explicitVR {
			var vrBytes [2]byte
			if _, err := io.ReadFull(r.r, vrBytes[:]); err != nil {
				return nil, fmt.Errorf("reading VR: %w", err)
			}
			vr := string(vrBytes[:])

			if isLongVR(vr) {
				// Skip 2 reserved bytes, then 4-byte length
				var reserved uint16
				binary.Read(r.r, binary.LittleEndian, &reserved)
				binary.Read(r.r, binary.LittleEndian, &vl)
			} else {
				// 2-byte length
				var vl16 uint16
				binary.Read(r.r, binary.LittleEndian, &vl16)
				vl = uint32(vl16)
			}
		} else {
			// Implicit VR - 4 byte length
			binary.Read(r.r, binary.LittleEndian, &vl)
		}

		// Skip element value
		if vl != 0xFFFFFFFF && vl > 0 {
			if _, err := io.CopyN(io.Discard, r.r, int64(vl)); err != nil {
				return nil, fmt.Errorf("skipping element value: %w", err)
			}
		} else if vl == 0xFFFFFFFF {
			// Nested undefined length - recursively skip
			if _, err := r.skipUndefinedLengthSequence(); err != nil {
				return nil, err
			}
		}
	}
}

// readEncapsulatedPixelData reads encapsulated (compressed) pixel data
func (r *Reader) readEncapsulatedPixelData() (*PixelData, error) {
	pd := &PixelData{
		IsEncapsulated: true,
		Frames:         []Frame{},
	}

	// Read Basic Offset Table (Item Tag FFFE,E000)
	botTag, err := r.readTag()
	if err != nil {
		return nil, err
	}
	if botTag.Group != 0xFFFE || botTag.Element != 0xE000 {
		return nil, fmt.Errorf("expected BOT item tag, got %v", botTag)
	}

	var botLength uint32
	if err := binary.Read(r.r, binary.LittleEndian, &botLength); err != nil {
		return nil, err
	}

	// Read BOT offsets
	if botLength > 0 {
		numOffsets := botLength / 4
		pd.Offsets = make([]uint32, numOffsets)
		for i := range pd.Offsets {
			if err := binary.Read(r.r, binary.LittleEndian, &pd.Offsets[i]); err != nil {
				return nil, err
			}
		}
	}

	// Read frames until Sequence Delimitation Item
	for {
		itemTag, err := r.readTag()
		if err != nil {
			return nil, err
		}

		// Check for Sequence Delimitation Item (FFFE,E0DD)
		if itemTag.Group == 0xFFFE && itemTag.Element == 0xE0DD {
			// Read and discard length (should be 0)
			var delimLength uint32
			if err := binary.Read(r.r, binary.LittleEndian, &delimLength); err != nil {
				return nil, err
			}
			break
		}

		// Regular Item (FFFE,E000)
		if itemTag.Group != 0xFFFE || itemTag.Element != 0xE000 {
			return nil, fmt.Errorf("expected item tag, got %v", itemTag)
		}

		var itemLength uint32
		if err := binary.Read(r.r, binary.LittleEndian, &itemLength); err != nil {
			return nil, err
		}

		// Read frame data
		frameData := make([]byte, itemLength)
		if _, err := io.ReadFull(r.r, frameData); err != nil {
			return nil, err
		}

		pd.Frames = append(pd.Frames, Frame{
			CompressedData: frameData,
		})
	}

	return pd, nil
}

// updateTransferSyntax updates reader settings based on transfer syntax
func (r *Reader) updateTransferSyntax() {
	switch r.transferSyntax {
	case "1.2.840.10008.1.2": // Implicit VR Little Endian
		r.explicitVR = false
		r.littleEndian = true
	case "1.2.840.10008.1.2.1": // Explicit VR Little Endian
		r.explicitVR = true
		r.littleEndian = true
	case "1.2.840.10008.1.2.4.80": // JPEG-LS Lossless
		r.explicitVR = true
		r.littleEndian = true
	case "1.2.840.10008.1.2.4.70": // JPEG Lossless (Process 14 SV1)
		r.explicitVR = true
		r.littleEndian = true
	case "1.2.840.10008.1.2.5": // RLE Lossless
		r.explicitVR = true
		r.littleEndian = true
	case "1.2.840.10008.1.2.4.90", "1.2.840.10008.1.2.4.91": // JPEG 2000
		r.explicitVR = true
		r.littleEndian = true
	}
}

// Helper functions

// isLongVR returns true if VR uses 4-byte VL (OB, OD, OF, OL, OW, SQ, UC, UR, UT, UN)
func isLongVR(vr string) bool {
	switch vr {
	case "OB", "OD", "OF", "OL", "OW", "SQ", "UC", "UR", "UT", "UN":
		return true
	}
	return false
}

// getImplicitVR returns VR for a tag when using Implicit VR transfer syntax
func getImplicitVR(tag Tag) string {
	// For now, return a default - in production, use a tag dictionary
	switch {
	case tag.Group == 0x0002: // File Meta Information
		return "UL"
	case tag.Group == 0x7FE0 && tag.Element == 0x0010:
		return "OW" // Pixel Data
	case tag.Group == 0x0028: // Image Pixel Module
		switch tag.Element {
		case 0x0010, 0x0011, 0x0100, 0x0101, 0x0102, 0x0103, 0x0002:
			return "US"
		case 0x0008:
			return "IS" // Number of Frames
		case 0x0030, 0x1050, 0x1051, 0x1052, 0x1053, 0x1054:
			return "DS" // Spacing, Windowing, Rescale
		case 0x0004:
			return "CS" // Photometric Interpretation
		}
	case tag.Group == 0x0008: // General Information
		switch tag.Element {
		case 0x0016, 0x0018:
			return "UI"
		case 0x0060, 0x0008, 0x0080:
			return "CS"
		}
	}
	return "UN" // Unknown
}

// parseValue converts raw bytes to typed value based on VR
func parseValue(vr string, data []byte) (interface{}, error) {
	switch vr {
	case "UI", "SH", "LO", "ST", "LT", "UT", "PN", "CS", "DA", "TM", "DT", "AS", "IS", "DS":
		// String types - trim null padding
		s := string(data)
		for len(s) > 0 && (s[len(s)-1] == 0 || s[len(s)-1] == ' ') {
			s = s[:len(s)-1]
		}
		return s, nil
	case "US": // Unsigned Short
		if len(data) == 2 {
			return binary.LittleEndian.Uint16(data), nil
		}
		// Multiple values
		values := make([]uint16, len(data)/2)
		for i := range values {
			values[i] = binary.LittleEndian.Uint16(data[i*2:])
		}
		return values, nil
	case "UL": // Unsigned Long
		if len(data) == 4 {
			return binary.LittleEndian.Uint32(data), nil
		}
		values := make([]uint32, len(data)/4)
		for i := range values {
			values[i] = binary.LittleEndian.Uint32(data[i*4:])
		}
		return values, nil
	case "SS": // Signed Short
		if len(data) == 2 {
			return int16(binary.LittleEndian.Uint16(data)), nil
		}
	case "SL": // Signed Long
		if len(data) == 4 {
			return int32(binary.LittleEndian.Uint32(data)), nil
		}
	case "FL": // Float
		if len(data) == 4 {
			var f float32
			binary.Read(bytes.NewReader(data), binary.LittleEndian, &f)
			return f, nil
		}
	case "FD": // Double
		if len(data) == 8 {
			var f float64
			binary.Read(bytes.NewReader(data), binary.LittleEndian, &f)
			return f, nil
		}
	case "OB", "OW", "UN":
		// Binary data
		return data, nil
	}
	return data, nil
}
