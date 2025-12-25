package rle

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
)

// Encode encodes an image into DICOM RLE format.
// It supports 8-bit (Gray) and 16-bit (Gray16) images.
// 16-bit images are split into High-Byte and Low-Byte planes for better compression.
func Encode(w io.Writer, img image.Image) error {
	var bounds = img.Bounds()
	var width, height = bounds.Dx(), bounds.Dy()
	var numPixels = width * height

	var segments [][]byte

	switch src := img.(type) {
	case *image.Gray:
		// 8-bit: Single segment
		// Use Pix directly if stride matches?
		// But Pix might have stride padding. Better to extract contiguous bytes?
		// Or just compress row by row? PackBits usually compresses continuous stream.
		// "The RLE algorithm ... operates on ... segments".
		// Segment is usually the whole frame component.
		// We need contiguous bytes.
		data := make([]byte, numPixels)
		i := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			rowOffset := (y - bounds.Min.Y) * src.Stride
			row := src.Pix[rowOffset : rowOffset+width]
			copy(data[i:], row)
			i += width
		}
		segments = append(segments, encodePackBits(data))

	case *image.Gray16:
		// 16-bit: Split into High Byte and Low Byte planes
		highBytes := make([]byte, numPixels)
		lowBytes := make([]byte, numPixels)

		i := 0
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			for x := bounds.Min.X; x < bounds.Max.X; x++ {
				// Gray16At handles bounds checks, but direct slice access is faster if possible.
				// But Pix is BigEndian in Go image types usually?
				// image.Gray16 Pix is []uint8. "Pix holds the image's pixels, as big-endian uint16s."
				// So even bytes = High, odd bytes = Low.
				rowOffset := (y-bounds.Min.Y)*src.Stride + (x-bounds.Min.X)*2
				highBytes[i] = src.Pix[rowOffset]
				lowBytes[i] = src.Pix[rowOffset+1]
				i++
			}
		}

		segments = append(segments, encodePackBits(highBytes)) // Segment 1: High Bytes
		segments = append(segments, encodePackBits(lowBytes))  // Segment 2: Low Bytes

	default:
		return fmt.Errorf("rle: unsupported image type %T", img)
	}

	// Calculate DICOM RLE Header
	// Header is 64 bytes.
	// Byte 0-3: Num Segments (uint32)
	// Byte 4-63: 15 offsets (uint32).
	// Offsets are relative to the start of the Header (byte 0).

	// Pad segments to even length (DICOM RLE requirement)
	for i := range segments {
		if len(segments[i])%2 != 0 {
			segments[i] = append(segments[i], 0x00)
		}
	}

	headerSize := 64
	numSegments := uint32(len(segments))
	if numSegments > 15 {
		return fmt.Errorf("rle: too many segments (%d)", numSegments)
	}

	offsets := make([]uint32, 15)
	currentOffset := uint32(headerSize)

	for i := range segments {
		offsets[i] = currentOffset
		currentOffset += uint32(len(segments[i]))
	}

	// Write Header
	// Num Segments
	if err := binary.Write(w, binary.LittleEndian, numSegments); err != nil {
		return err
	}
	// Offsets
	if err := binary.Write(w, binary.LittleEndian, offsets); err != nil {
		return err
	}

	// Write Segments
	for _, seg := range segments {
		if _, err := w.Write(seg); err != nil {
			return err
		}
	}

	return nil
}
