package jpegli

import (
	"bufio"
	"errors"
	"fmt"
	"image"
	"image/color"
	"io"
	"log/slog"
)

// decodeScan decodes the compressed scan data
func (d *Decoder) decodeScan() (image.Image, error) {
	// Read SOS parameters
	if err := d.readSOS(); err != nil {
		return nil, err
	}

	// Debug: check remaining bytes in reader
	if br, ok := d.r.(interface{ Len() int }); ok {
		slog.Debug("remaining bytes after SOS", slog.Int("bytes", br.Len()))
	}

	// Create bit reader for entropy-coded data
	br := newBitReader(d.r)

	// Allocate image based on precision
	var img image.Image
	if d.precision <= 8 {
		img = image.NewGray(image.Rect(0, 0, d.width, d.height))
	} else {
		img = image.NewGray16(image.Rect(0, 0, d.width, d.height))
	}

	// Get Huffman table for first component
	tableIdx := 0
	if len(d.compInfo) > 0 {
		tableIdx = d.compInfo[0].tableIndex
	}
	ht := d.dcTables[tableIdx]
	if ht == nil {
		return nil, errors.New("missing Huffman table")
	}

	// Maximum value based on precision
	maxVal := (1 << d.precision) - 1

	// Previous row for prediction
	prevRow := make([]int, d.width)
	currRow := make([]int, d.width)

	// Restart counter
	restartCounter := 0
	mcuCount := 0

	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			// Check for restart marker
			if d.restartInterval > 0 && mcuCount > 0 && mcuCount%d.restartInterval == 0 {
				if err := br.alignToByte(); err != nil {
					return nil, err
				}
				// Read restart marker
				b1, _ := br.readByte()
				b2, _ := br.readByte()
				if b1 != 0xFF || (b2&0xF8) != 0xD0 {
					// Put bytes back or skip
					slog.Debug("missed restart marker", slog.Int("x", x), slog.Int("y", y))
				}
				restartCounter++
				// Reset prediction
				for i := range prevRow {
					prevRow[i] = 0
				}
			}

			// Decode Huffman symbol (# of bits for difference)
			ssss, err := d.decodeHuffman(br, ht)
			if err != nil {
				if err == io.EOF {
					// End of data
					totalPixels := y*d.width + x
					expectedPixels := d.width * d.height
					// If we have most of the image, return what we have
					if float64(totalPixels) > float64(expectedPixels)*0.99 {
						slog.Warn("Premature EOF in scan data, returning partial image",
							slog.Int("decoded", totalPixels),
							slog.Int("expected", expectedPixels))
						// Fill remaining pixels in this row with last value
						for k := x; k < d.width; k++ {
							if k > 0 {
								currRow[k] = currRow[k-1]
							}
						}
						return img, nil
					}
					return nil, fmt.Errorf("premature EOF at pixel (%d,%d), decoded %d/%d pixels (%.1f%%)",
						x, y, totalPixels, expectedPixels, float64(totalPixels)*100/float64(expectedPixels))
				}
				// Check if we're close enough to EOF to return partial image
				totalPixels := y*d.width + x
				expectedPixels := d.width * d.height
				if float64(totalPixels) > float64(expectedPixels)*0.99 {
					slog.Warn("Huffman error near end of image, returning partial",
						slog.Int("decoded", totalPixels),
						slog.Int("expected", expectedPixels))
					// Fill remaining pixels with last value
					lastVal := 0
					if x > 0 {
						lastVal = currRow[x-1]
					}
					for k := x; k < d.width; k++ {
						currRow[k] = lastVal
					}
					// Write filled pixels to image
					if grayImg, ok := img.(*image.Gray); ok {
						for k := x; k < d.width; k++ {
							grayImg.SetGray(k, y, color.Gray{Y: uint8(currRow[k])})
						}
					} else if gray16Img, ok := img.(*image.Gray16); ok {
						for k := x; k < d.width; k++ {
							gray16Img.SetGray16(k, y, color.Gray16{Y: uint16(currRow[k])})
						}
					}
					return img, nil
				}
				offset := int64(0)
				if br != nil {
					offset = br.totalBits / 8
				}
				slog.Error("decodeHuffman failed", slog.Int("x", x), slog.Int("y", y), slog.Int64("offset", offset), slog.Any("error", err))
				return nil, err
			}

			// Read additional bits for the difference value
			var diff int
			if ssss > 0 {
				bits, err := br.readBits(ssss)
				if err != nil {
					if err == io.EOF {
						totalPixels := y*d.width + x
						expectedPixels := d.width * d.height
						if float64(totalPixels) > float64(expectedPixels)*0.99 {
							slog.Warn("Premature EOF reading difference bits, returning partial image",
								slog.Int("decoded", totalPixels),
								slog.Int("expected", expectedPixels))
							return img, nil
						}
					}
					return nil, err
				}
				// Convert to signed value
				diff = extend(bits, ssss)
			}

			// Get prediction
			pred := d.predict(currRow, prevRow, x, y)

			// Apply point transform
			if d.pointTrans > 0 {
				diff <<= d.pointTrans
			}

			// Reconstruct pixel value
			val := (pred + diff) & maxVal

			currRow[x] = val
			mcuCount++

			// Store in image
			if grayImg, ok := img.(*image.Gray); ok {
				grayImg.SetGray(x, y, color.Gray{Y: uint8(val)})
			} else if gray16Img, ok := img.(*image.Gray16); ok {
				gray16Img.SetGray16(x, y, color.Gray16{Y: uint16(val)})
			}
		}
		// Swap rows
		prevRow, currRow = currRow, prevRow
		for i := range currRow {
			currRow[i] = 0
		}
	}

	return img, nil
}

// predict computes the predicted value based on the selected predictor
func (d *Decoder) predict(currRow, prevRow []int, x, y int) int {
	var Ra, Rb, Rc int

	// Get neighbor values
	if x > 0 {
		Ra = currRow[x-1] // Left
	}
	if y > 0 {
		Rb = prevRow[x] // Above
		if x > 0 {
			Rc = prevRow[x-1] // Above-left
		}
	}

	// First row/column handling
	if y == 0 && x == 0 {
		// First pixel: predict from half max value
		return 1 << (d.precision - 1)
	}
	if y == 0 {
		// First row: use left neighbor
		return Ra
	}
	if x == 0 {
		// First column: use above neighbor
		return Rb
	}

	// Apply selected predictor
	switch d.predictor {
	case 0:
		return 0 // No prediction
	case 1:
		return Ra // Left
	case 2:
		return Rb // Above
	case 3:
		return Rc // Above-left
	case 4:
		return Ra + Rb - Rc // Linear
	case 5:
		return Ra + (Rb-Rc)/2
	case 6:
		return Rb + (Ra-Rc)/2
	case 7:
		return (Ra + Rb) / 2 // Average
	default:
		return Ra // Default to left
	}
}

// extend converts a partial bit sequence to a signed integer
func extend(bits, ssss int) int {
	if ssss == 0 {
		return 0
	}
	vt := 1 << (ssss - 1)
	if bits < vt {
		// Negative value
		return bits - (1<<ssss - 1)
	}
	return bits
}

// decodeHuffman decodes a single Huffman symbol
func (d *Decoder) decodeHuffman(br *bitReader, ht *huffmanTable) (int, error) {
	// Try 8-bit lookup first
	peek, err := br.peekBits(8)
	if err != nil && err != io.EOF {
		return 0, err
	}

	// Ensure peek is within bounds (0-255)
	peek = peek & 0xFF

	lookup := ht.lookup[peek]

	if lookup >= 0 {
		size := int(lookup >> 8)
		value := int(lookup & 0xFF)
		br.consumeBits(size)
		return value, nil
	}

	// Slow path: decode bit by bit
	code := 0
	for size := 1; size <= 16; size++ {
		bit, err := br.readBit()
		if err != nil {
			return 0, err
		}
		code = code<<1 | bit

		// Search for matching code
		codeIdx := 0
		for i := 1; i < size; i++ {
			codeIdx += ht.bits[i]
		}
		for i := 0; i < ht.bits[size]; i++ {
			if ht.codes[codeIdx+i] == uint16(code) {
				return int(ht.values[codeIdx+i]), nil
			}
		}
	}

	return 0, fmt.Errorf("invalid Huffman code: reached 16 bits without match, code=%b", code)
}

// bitReader reads bits from a byte stream, handling byte stuffing (0xFF00 -> 0xFF)
type bitReader struct {
	r         *bufio.Reader
	buf       uint32
	bits      int
	eof       bool
	totalBits int64
}

func newBitReader(r io.Reader) *bitReader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &bitReader{r: br}
}

func (b *bitReader) fillBits() error {
	for b.bits < 16 && !b.eof {
		c, err := b.r.ReadByte()
		if err != nil {
			if err == io.EOF {
				b.eof = true
				return nil
			}
			return err
		}

		// Handle byte stuffing
		if c == 0xFF {
			// Peek next byte to see if it's stuffed 0x00 or a marker
			nextBytes, err := b.r.Peek(1)
			if err != nil {
				if err == io.EOF {
					// 0xFF at very end of data
					b.eof = true
					return nil
				}
				return err
			}
			next := nextBytes[0]

			if next == 0x00 {
				// Stuffed byte - use 0xFF
				b.r.Discard(1) // Consume the 0x00
				b.buf = b.buf<<8 | 0xFF
				b.bits += 8
			} else if next >= 0xD0 && next <= 0xD7 {
				// Restart marker - ignore but must remain byte-aligned
				b.r.Discard(1) // Consume the marker byte
				continue
			} else {
				// Other marker (like EOI) - stop reading data
				b.r.UnreadByte() // Put back the 0xFF
				b.eof = true
				return nil
			}
		} else {
			b.buf = b.buf<<8 | uint32(c)
			b.bits += 8
		}
	}
	return nil
}

func (b *bitReader) readBit() (int, error) {
	if b.bits < 1 {
		if err := b.fillBits(); err != nil {
			return 0, err
		}
	}
	if b.bits < 1 {
		// EOF reached. Pad with 0s.
		b.totalBits++
		return 0, nil
	}
	b.bits--
	b.totalBits++
	return int((b.buf >> b.bits) & 1), nil
}

func (b *bitReader) readBits(n int) (int, error) {
	if n == 0 {
		return 0, nil
	}
	for b.bits < n {
		if err := b.fillBits(); err != nil {
			return 0, err
		}
		if b.eof && b.bits < n {
			// EOF reached. Pad with 0s.
			validBitsVal := int(b.buf & ((1 << b.bits) - 1))
			missingBits := n - b.bits
			// Shift valid bits to MSB of result, lower bits are 0s
			result := validBitsVal << missingBits

			b.bits = 0 // Consumed all valid
			b.totalBits += int64(n)
			return result, nil
		}
	}
	b.bits -= n
	b.totalBits += int64(n)
	mask := (1 << n) - 1
	return int((b.buf >> b.bits) & uint32(mask)), nil
}

func (b *bitReader) peekBits(n int) (int, error) {
	for b.bits < n {
		if err := b.fillBits(); err != nil {
			return 0, err
		}
		if b.eof && b.bits < n {
			// Pad with 0s and mask to requested bits
			// Current valid bits (at bottom of buffer) shifted left
			// Lower bits are 0s by default
			val := int(b.buf) << (n - b.bits)

			mask := (1 << n) - 1
			return val & mask, nil
		}
	}
	mask := (1 << n) - 1
	return int((b.buf >> (b.bits - n)) & uint32(mask)), nil
}

func (b *bitReader) consumeBits(n int) {
	b.bits -= n
	if b.bits < 0 && b.eof {
		b.bits = 0
	}
	b.totalBits += int64(n)
}

func (b *bitReader) alignToByte() error {
	b.bits = b.bits & ^7 // Clear remaining bits in current byte
	return nil
}

func (b *bitReader) readByte() (byte, error) {
	return b.r.ReadByte()
}
