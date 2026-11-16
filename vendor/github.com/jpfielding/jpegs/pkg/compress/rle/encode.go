package rle

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"math"
)

// Encode encodes an image into DICOM RLE format.
// It supports 8-bit (*image.Gray, one segment) and 16-bit (*image.Gray16, two
// segments: high byte plane then low byte plane) images.
//
// Each row of each byte plane is PackBits-compressed separately, as PS3.5
// G.3.1 requires, so no run crosses a row boundary. Each segment is padded to
// even length.
func Encode(w io.Writer, img image.Image) error {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	numPixels := width * height

	var planes [][]byte
	switch src := img.(type) {
	case *image.Gray:
		plane := make([]byte, 0, numPixels)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			off := src.PixOffset(bounds.Min.X, y)
			plane = append(plane, src.Pix[off:off+width]...)
		}
		planes = [][]byte{plane}

	case *image.Gray16:
		high := make([]byte, 0, numPixels)
		low := make([]byte, 0, numPixels)
		for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
			off := src.PixOffset(bounds.Min.X, y)
			row := src.Pix[off : off+2*width]
			for x := 0; x < width; x++ {
				high = append(high, row[2*x])
				low = append(low, row[2*x+1])
			}
		}
		planes = [][]byte{high, low}

	default:
		return fmt.Errorf("%w: image type %T (want *image.Gray or *image.Gray16)", ErrUnsupported, img)
	}

	segments := make([][]byte, len(planes))
	for i, p := range planes {
		seg := encodePackBitsRows(p, width)
		if len(seg)%2 != 0 {
			seg = append(seg, 0x00)
		}
		segments[i] = seg
	}

	// Header: uint32 segment count, then 15 uint32 offsets measured from the
	// start of the header (PS3.5 G.5).
	var header [headerSize]byte
	binary.LittleEndian.PutUint32(header[0:4], uint32(len(segments)))
	offset := uint64(headerSize)
	for i, seg := range segments {
		if offset > math.MaxUint32 {
			return fmt.Errorf("%w: segment %d offset %d exceeds 32 bits", ErrUnsupported, i, offset)
		}
		binary.LittleEndian.PutUint32(header[4+4*i:8+4*i], uint32(offset))
		offset += uint64(len(seg))
	}

	if _, err := w.Write(header[:]); err != nil {
		return fmt.Errorf("rle: write header: %w", err)
	}
	for i, seg := range segments {
		if _, err := w.Write(seg); err != nil {
			return fmt.Errorf("rle: write segment %d: %w", i, err)
		}
	}
	return nil
}
