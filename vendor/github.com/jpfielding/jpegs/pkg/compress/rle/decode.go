package rle

import (
	"encoding/binary"
	"fmt"
	"image"
)

// headerSize is the fixed DICOM RLE header: a uint32 segment count followed
// by 15 uint32 segment offsets, all little-endian (PS3.5 G.5).
const headerSize = 64

// Decode decodes DICOM RLE compressed data.
// width and height must be provided as the RLE stream does not allow determining them.
//
// One segment decodes to *image.Gray, two (high byte plane, then low byte
// plane) to *image.Gray16. Segments may contain PackBits runs that cross row
// boundaries; PS3.5 G.3.1 forbids them, but older encoders (including this
// package before it encoded per row) wrote them, so the decoder accepts both.
func Decode(data []byte, width, height int) (image.Image, error) {
	if len(data) < headerSize {
		return nil, fmt.Errorf("%w: header needs %d bytes, have %d", ErrTruncated, headerSize, len(data))
	}

	numSegments := binary.LittleEndian.Uint32(data[0:4])
	var offsets [15]uint32
	for i := range offsets {
		offsets[i] = binary.LittleEndian.Uint32(data[4+i*4 : 8+i*4])
	}

	if numSegments == 0 || numSegments > 15 {
		return nil, fmt.Errorf("%w: segment count %d (allowed 1..15)", ErrInvalidData, numSegments)
	}
	// Segments are byte planes: 1 = 8-bit gray, 2 = 16-bit gray.
	if numSegments > 2 {
		return nil, fmt.Errorf("%w: segment count %d (only 1 or 2)", ErrUnsupported, numSegments)
	}

	// Validate the whole header before decompressing anything: each segment
	// must start after the header, segments must be in order, and every
	// segment must lie inside the data.
	n := uint64(len(data))
	for i := uint32(0); i < numSegments; i++ {
		start, end := uint64(offsets[i]), n
		if i < numSegments-1 {
			end = uint64(offsets[i+1])
		}
		switch {
		case start < headerSize:
			return nil, fmt.Errorf("%w: segment %d offset %d lies inside the %d-byte header", ErrInvalidData, i, start, headerSize)
		case start > n:
			return nil, fmt.Errorf("%w: segment %d offset %d beyond data length %d", ErrTruncated, i, start, n)
		case end > n:
			return nil, fmt.Errorf("%w: segment %d ends at %d beyond data length %d", ErrTruncated, i, end, n)
		case start > end:
			return nil, fmt.Errorf("%w: segment %d offset %d after segment %d offset %d", ErrInvalidData, i, start, i+1, end)
		}
	}

	// Validate caller-supplied geometry before sizing any buffer from it.
	if err := checkDecodedSize(width, height, 1, int(numSegments)); err != nil {
		return nil, err
	}
	numPixels := width * height

	segments := make([][]byte, numSegments)
	for i := uint32(0); i < numSegments; i++ {
		start, end := uint64(offsets[i]), n
		if i < numSegments-1 {
			end = uint64(offsets[i+1])
		}
		decoded, err := decodePackBits(data[start:end], numPixels)
		if err != nil {
			return nil, fmt.Errorf("rle: segment %d at offset %d: %w", i, start, err)
		}
		if len(decoded) != numPixels {
			return nil, fmt.Errorf("%w: segment %d decoded to %d bytes, want %d", ErrDimensionMismatch, i, len(decoded), numPixels)
		}
		segments[i] = decoded
	}

	switch numSegments {
	case 1:
		img := image.NewGray(image.Rect(0, 0, width, height))
		copy(img.Pix, segments[0])
		return img, nil
	default: // 2: high byte plane, low byte plane
		img := image.NewGray16(image.Rect(0, 0, width, height))
		high, low := segments[0], segments[1]
		// image.Gray16 Pix is big-endian uint16s: [Hi, Lo, Hi, Lo, ...].
		for i := 0; i < numPixels; i++ {
			img.Pix[2*i] = high[i]
			img.Pix[2*i+1] = low[i]
		}
		return img, nil
	}
}
