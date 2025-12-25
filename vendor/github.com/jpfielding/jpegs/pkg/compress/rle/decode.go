package rle

import (
	"encoding/binary"
	"errors"
	"fmt"
	"image"
)

// Decode decodes DICOM RLE compressed data.
// width and height must be provided as the RLE stream does not allow determining them.
func Decode(data []byte, width, height int) (image.Image, error) {
	if len(data) < 64 {
		return nil, errors.New("rle: data too short for header")
	}

	// Read Header
	numSegments := binary.LittleEndian.Uint32(data[0:4])
	offsets := make([]uint32, 15)
	for i := 0; i < 15; i++ {
		offsets[i] = binary.LittleEndian.Uint32(data[4+i*4 : 8+i*4])
	}

	if numSegments == 0 {
		return nil, errors.New("rle: zero segments")
	}
	if numSegments > 15 {
		return nil, fmt.Errorf("rle: invalid segment count %d", numSegments)
	}

	// Decode Segments
	segments := make([][]byte, numSegments)
	numPixels := width * height

	for i := uint32(0); i < numSegments; i++ {
		start := offsets[i]
		var end uint32
		if i < numSegments-1 {
			end = offsets[i+1]
		} else {
			end = uint32(len(data))
		}

		if start > uint32(len(data)) || end > uint32(len(data)) || start > end {
			return nil, fmt.Errorf("rle: invalid segment offset/length for segment %d", i)
		}

		segData := data[start:end]
		// Each segment (whether 8-bit or 16-bit split) should decode to exactly numPixels bytes
		// (e.g. High Byte Plane is numPixels bytes, Low Byte Plane is numPixels bytes)
		decoded, err := decodePackBits(segData, numPixels)
		if err != nil {
			return nil, fmt.Errorf("rle: failed to decode segment %d (start=%d, end=%d, len=%d): %w", i, start, end, len(data), err)
		}
		if len(decoded) != numPixels {
			return nil, fmt.Errorf("rle: decoded segment %d size %d does not match expected pixels %d", i, len(decoded), numPixels)
		}
		segments[i] = decoded
	}

	// Reconstruct Image
	// Assume 1 segment = 8-bit Gray
	// Assume 2 segments = 16-bit Gray (High, Low)
	// Other cases not supported for now (e.g. RGB)

	if numSegments == 1 {
		// 8-bit Gray
		if len(segments[0]) != numPixels {
			return nil, fmt.Errorf("rle: decoded size %d does not match expected pixels %d", len(segments[0]), numPixels)
		}
		// Length check is now done above for each segment
		img := image.NewGray(image.Rect(0, 0, width, height))
		copy(img.Pix, segments[0])
		return img, nil

	} else if numSegments == 2 {
		// 16-bit Gray (High, Low)
		// Length checks are now done above for each segment
		img := image.NewGray16(image.Rect(0, 0, width, height))

		high := segments[0]
		low := segments[1]

		// Reinterleave
		// image.Gray16 Pix is BigEndian uint16s ([Hi, Lo, Hi, Lo...])
		// So we can copy byte by byte

		j := 0
		for i := 0; i < numPixels; i++ {
			img.Pix[j] = high[i]
			img.Pix[j+1] = low[i]
			j += 2
		}
		return img, nil
	}

	return nil, fmt.Errorf("rle: unsupported number of segments %d", numSegments)
}
