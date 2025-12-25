package rle

import (
	"bytes"
	"errors"
	"fmt"
)

// PackBits implementation for DICOM RLE
// Reference: DICOM PS3.5 Annex G

// encodePackBits compresses data using PackBits algorithm
func encodePackBits(data []byte) []byte {
	// Simplified PackBits Encoder
	if len(data) == 0 {
		return nil
	}

	var buf bytes.Buffer
	i := 0
	for i < len(data) {
		// Attempt to find run
		runLen := 1
		for i+runLen < len(data) && runLen < 128 && data[i+runLen] == data[i] {
			runLen++
		}

		// Heuristic: only encode as run if length >= 2 (PackBits standard varies, using 2 here)
		// But wait, checks for literal break usually require 3.
		// Let's use strict > 1 (>= 2) for run.
		if runLen > 1 {
			// Write run
			header := int8(-(runLen - 1))
			buf.WriteByte(byte(header))
			buf.WriteByte(data[i])
			i += runLen
		} else {
			// Literal run
			// Consume until we find a run of 3 identical bytes, or 128 chars
			litStart := i
			litLen := 1
			for i+litLen < len(data) && litLen < 128 {
				// Check for run break (3 chars)
				if i+litLen+2 < len(data) &&
					data[i+litLen] == data[i+litLen+1] &&
					data[i+litLen] == data[i+litLen+2] {
					break
				}
				litLen++
			}

			// Write literal
			header := int8(litLen - 1)
			buf.WriteByte(byte(header))
			buf.Write(data[litStart : litStart+litLen])
			if i+litLen == len(data) {
			}
			i += litLen
		}
	}
	return buf.Bytes()
}

// decodePackBits decodes PackBits compressed data
func decodePackBits(data []byte, expectedLen int) ([]byte, error) {
	var buf bytes.Buffer
	if expectedLen > 0 {
		buf.Grow(expectedLen)
	}

	i := 0
	for i < len(data) {
		// Stop if we reached expected length
		if expectedLen > 0 && buf.Len() >= expectedLen {
			break
		}

		n := int8(data[i])
		i++

		if n == -128 {
			// No-op
			continue
		}

		if n >= 0 {
			// Literal run: read n+1 bytes
			count := int(n) + 1
			if i+count > len(data) {
				// If we have an expected length and we already have enough data, maybe we can ignore this?
				// But literal run means we MUST read data. Truncation here is likely error.
				// However, if the command byte itself was the padding byte (00 -> Literal 1),
				// checking expectedLen BEFORE reading command avoids this.
				// The check `if expectedLen > 0 && buf.Len() >= expectedLen` above handles it.

				return nil, fmt.Errorf("rle: compressed data truncated in literal run (i=%d, count=%d, len=%d)", i, count, len(data))
			}
			buf.Write(data[i : i+count])
			i += count
		} else {
			// Replicate run: read 1 byte, repeat -n+1 times
			// count = -n + 1
			count := int(-n) + 1
			if i >= len(data) {
				return nil, errors.New("rle: compressed data truncated in replicate run")
			}
			val := data[i]
			i++
			for k := 0; k < count; k++ {
				buf.WriteByte(val)
			}
		}
	}
	return buf.Bytes(), nil
}
