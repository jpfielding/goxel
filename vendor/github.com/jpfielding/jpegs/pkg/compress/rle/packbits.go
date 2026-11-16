package rle

import (
	"bytes"
	"fmt"
)

// PackBits implementation for DICOM RLE
// Reference: DICOM PS3.5 Annex G

// encodePackBitsRows compresses plane, a width-wide byte plane, one row at a
// time and concatenates the packets. PS3.5 G.3.1 requires each row to be
// encoded separately, so no Replicate or Literal Run crosses a row boundary.
func encodePackBitsRows(plane []byte, width int) []byte {
	if width <= 0 {
		return nil
	}
	var out []byte
	for start := 0; start < len(plane); start += width {
		out = append(out, encodePackBits(plane[start:min(start+width, len(plane))])...)
	}
	return out
}

// encodePackBits compresses data using the PackBits algorithm.
func encodePackBits(data []byte) []byte {
	if len(data) == 0 {
		return nil
	}

	var buf bytes.Buffer
	i := 0
	for i < len(data) {
		runLen := 1
		for i+runLen < len(data) && runLen < 128 && data[i+runLen] == data[i] {
			runLen++
		}

		if runLen > 1 {
			// Replicate run: header -(runLen-1), then the byte.
			buf.WriteByte(byte(int8(-(runLen - 1))))
			buf.WriteByte(data[i])
			i += runLen
			continue
		}

		// Literal run: extend until three identical bytes start or 128 bytes.
		litLen := 1
		for i+litLen < len(data) && litLen < 128 {
			if i+litLen+2 < len(data) &&
				data[i+litLen] == data[i+litLen+1] &&
				data[i+litLen] == data[i+litLen+2] {
				break
			}
			litLen++
		}
		buf.WriteByte(byte(litLen - 1))
		buf.Write(data[i : i+litLen])
		i += litLen
	}
	return buf.Bytes()
}

// decodePackBits decodes PackBits compressed data. If expectedLen > 0 it
// stops reading packets once at least expectedLen bytes were produced, so a
// trailing pad byte is ignored. Packets that end early return ErrTruncated
// with the offset of the shortfall within data.
func decodePackBits(data []byte, expectedLen int) ([]byte, error) {
	var buf bytes.Buffer
	// A PackBits run of 2 input bytes expands to at most 128 output bytes, so
	// cap the pre-grow by what the input can produce, never by the caller's
	// (possibly bogus) expectedLen alone.
	if expectedLen > 0 {
		buf.Grow(min(expectedLen, len(data)*64))
	}

	i := 0
	for i < len(data) {
		if expectedLen > 0 && buf.Len() >= expectedLen {
			break
		}

		n := int8(data[i])
		i++

		switch {
		case n == -128: // no-op
		case n >= 0: // literal run of n+1 bytes
			count := int(n) + 1
			if i+count > len(data) {
				return nil, fmt.Errorf("%w: literal run at offset %d needs %d bytes, have %d", ErrTruncated, i, count, len(data)-i)
			}
			buf.Write(data[i : i+count])
			i += count
		default: // replicate the next byte -n+1 times
			if i >= len(data) {
				return nil, fmt.Errorf("%w: replicate run at offset %d has no value byte", ErrTruncated, i)
			}
			val := data[i]
			i++
			for range int(-n) + 1 {
				buf.WriteByte(val)
			}
		}
	}
	return buf.Bytes(), nil
}
