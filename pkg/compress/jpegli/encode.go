package jpegli

import (
	"encoding/binary"
	"image"
	"io"
)

// Encoder encodes images to JPEG Lossless format
type Encoder struct {
	// Predictor selection (1-7, default 1)
	Predictor int
	// Point transform (0 for lossless)
	PointTransform int
}

// Encode writes img to w in JPEG Lossless format
func Encode(w io.Writer, img image.Image, opts *Encoder) error {
	enc := &encoder{
		w:         w,
		predictor: 1,
	}
	if opts != nil {
		if opts.Predictor >= 1 && opts.Predictor <= 7 {
			enc.predictor = opts.Predictor
		}
		enc.pointTrans = opts.PointTransform
	}
	return enc.encode(img)
}

type encoder struct {
	w          io.Writer
	predictor  int
	pointTrans int
	precision  int
	width      int
	height     int
}

func (e *encoder) encode(img image.Image) error {
	bounds := img.Bounds()
	e.width = bounds.Dx()
	e.height = bounds.Dy()

	// Determine precision from image type
	switch img.(type) {
	case *image.Gray16:
		e.precision = 16
	case *image.Gray:
		e.precision = 8
	default:
		e.precision = 16 // Default to 16-bit
	}

	// Write SOI
	if err := e.writeMarker(MarkerSOI); err != nil {
		return err
	}

	// Write APP0 (JFIF)
	if err := e.writeAPP0(); err != nil {
		return err
	}

	// Write SOF3 (Lossless)
	if err := e.writeSOF3(); err != nil {
		return err
	}

	// Build and write Huffman table
	ht := e.buildHuffmanTable(img)
	if err := e.writeDHT(ht); err != nil {
		return err
	}

	// Write SOS and scan data
	if err := e.writeSOS(img, ht); err != nil {
		return err
	}

	// Write EOI
	return e.writeMarker(MarkerEOI)
}

func (e *encoder) writeMarker(marker int) error {
	return binary.Write(e.w, binary.BigEndian, uint16(marker))
}

func (e *encoder) writeAPP0() error {
	// JFIF APP0 marker
	if err := e.writeMarker(MarkerAPP0); err != nil {
		return err
	}

	// Length (16 bytes total: 2 length + 5 JFIF\0 + 2 version + 1 units + 4 density + 2 thumbnail)
	data := []byte{
		0x00, 0x10, // Length = 16
		0x4A, 0x46, 0x49, 0x46, 0x00, // "JFIF\0"
		0x01, 0x01, // Version 1.1
		0x00,       // Units: no units
		0x00, 0x01, // X density = 1
		0x00, 0x01, // Y density = 1
		0x00, 0x00, // No thumbnail
	}
	_, err := e.w.Write(data)
	return err
}

func (e *encoder) writeSOF3() error {
	if err := e.writeMarker(MarkerSOF3); err != nil {
		return err
	}

	// Length = 2 + 1 + 2 + 2 + 1 + 3*components
	length := 2 + 1 + 2 + 2 + 1 + 3 // 1 component
	data := make([]byte, length)
	data[0] = byte(length >> 8)
	data[1] = byte(length)
	data[2] = byte(e.precision)
	data[3] = byte(e.height >> 8)
	data[4] = byte(e.height)
	data[5] = byte(e.width >> 8)
	data[6] = byte(e.width)
	data[7] = 1 // 1 component

	// Component: ID=1, sampling=1x1, table=0
	data[8] = 1    // Component ID
	data[9] = 0x11 // Sampling: H=1, V=1
	data[10] = 0   // Quantization table (not used in lossless)

	_, err := e.w.Write(data)
	return err
}

// buildHuffmanTable analyzes the image and builds an optimal Huffman table
func (e *encoder) buildHuffmanTable(img image.Image) *huffmanTable {
	// Count occurrences of each SSSS category (0-16)
	counts := make([]int, 17)

	prevRow := make([]int, e.width)
	currRow := make([]int, e.width)
	maxVal := (1 << e.precision) - 1

	for y := 0; y < e.height; y++ {
		for x := 0; x < e.width; x++ {
			// Get pixel value
			val := e.getPixel(img, x, y)
			currRow[x] = val

			// Get prediction
			pred := e.predict(currRow, prevRow, x, y)

			// Calculate difference
			diff := (val - pred) & maxVal
			if diff > maxVal/2 {
				diff -= maxVal + 1 // Convert to signed
			}

			// Get SSSS category
			ssss := categorize(diff)
			counts[ssss]++
		}
		prevRow, currRow = currRow, prevRow
	}

	// Build Huffman table from counts using standard JPEG algorithm
	return buildHuffmanFromCounts(counts)
}

func (e *encoder) getPixel(img image.Image, x, y int) int {
	switch g := img.(type) {
	case *image.Gray16:
		return int(g.Gray16At(x, y).Y)
	case *image.Gray:
		return int(g.GrayAt(x, y).Y)
	default:
		r, _, _, _ := img.At(x, y).RGBA()
		return int(r >> (16 - e.precision))
	}
}

func (e *encoder) predict(currRow, prevRow []int, x, y int) int {
	var Ra, Rb, Rc int

	if x > 0 {
		Ra = currRow[x-1]
	}
	if y > 0 {
		Rb = prevRow[x]
		if x > 0 {
			Rc = prevRow[x-1]
		}
	}

	if y == 0 && x == 0 {
		return 1 << (e.precision - 1)
	}
	if y == 0 {
		return Ra
	}
	if x == 0 {
		return Rb
	}

	switch e.predictor {
	case 1:
		return Ra
	case 2:
		return Rb
	case 3:
		return Rc
	case 4:
		return Ra + Rb - Rc
	case 5:
		return Ra + (Rb-Rc)/2
	case 6:
		return Rb + (Ra-Rc)/2
	case 7:
		return (Ra + Rb) / 2
	default:
		return Ra
	}
}

// categorize returns the SSSS category for a difference value
func categorize(diff int) int {
	if diff < 0 {
		diff = -diff
	}
	ssss := 0
	for diff > 0 {
		diff >>= 1
		ssss++
	}
	return ssss
}

// buildHuffmanFromCounts creates a Huffman table from symbol counts
func buildHuffmanFromCounts(counts []int) *huffmanTable {
	// Extended table to support 16-bit data (SSSS 0-16)
	// This is a simple table with fixed code lengths
	ht := &huffmanTable{}

	// Extended Huffman table for 16-bit lossless JPEG
	// BITS: number of codes of each length (1-16)
	// Use a distribution that covers all 17 possible SSSS values (0-16)
	// Sum must equal 17: 0+1+5+1+1+1+1+1+1+1+1+1+1+1+0+0 = 17
	ht.bits = [17]int{0, 0, 1, 5, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0}
	ht.values = []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}

	// Generate codes
	var totalCodes int
	for i := 1; i <= 16; i++ {
		totalCodes += ht.bits[i]
	}

	ht.codes = make([]uint16, totalCodes)
	ht.sizes = make([]int, totalCodes)

	k := 0
	for i := 1; i <= 16; i++ {
		for j := 0; j < ht.bits[i]; j++ {
			ht.sizes[k] = i
			k++
		}
	}

	code := uint16(0)
	si := ht.sizes[0]
	for k := 0; k < totalCodes; k++ {
		for ht.sizes[k] > si {
			code <<= 1
			si++
		}
		ht.codes[k] = code
		code++
	}

	return ht
}

func (e *encoder) writeDHT(ht *huffmanTable) error {
	if err := e.writeMarker(MarkerDHT); err != nil {
		return err
	}

	// Calculate length: 2 + 1 + 16 + len(values)
	length := 2 + 1 + 16 + len(ht.values)
	data := make([]byte, length)
	data[0] = byte(length >> 8)
	data[1] = byte(length)
	data[2] = 0 // Table class 0 (DC), Table ID 0

	// Copy BITS
	for i := 1; i <= 16; i++ {
		data[2+i] = byte(ht.bits[i])
	}

	// Copy HUFFVAL
	copy(data[19:], ht.values)

	_, err := e.w.Write(data)
	return err
}

func (e *encoder) writeSOS(img image.Image, ht *huffmanTable) error {
	if err := e.writeMarker(MarkerSOS); err != nil {
		return err
	}

	// SOS header
	// Length = 2 + 1 + 2*components + 3
	length := 2 + 1 + 2 + 3 // 1 component
	header := make([]byte, length)
	header[0] = byte(length >> 8)
	header[1] = byte(length)
	header[2] = 1 // 1 component
	header[3] = 1 // Component ID
	header[4] = 0 // DC table 0, AC table 0
	header[5] = byte(e.predictor)
	header[6] = 0 // Se (not used in lossless)
	header[7] = byte(e.pointTrans)

	if _, err := e.w.Write(header); err != nil {
		return err
	}

	// Encode scan data
	return e.encodeScan(img, ht)
}

func (e *encoder) encodeScan(img image.Image, ht *huffmanTable) error {
	bw := newBitWriter(e.w)

	prevRow := make([]int, e.width)
	currRow := make([]int, e.width)
	maxVal := (1 << e.precision) - 1

	for y := 0; y < e.height; y++ {
		for x := 0; x < e.width; x++ {
			// Get pixel value
			val := e.getPixel(img, x, y)
			currRow[x] = val

			// Get prediction
			pred := e.predict(currRow, prevRow, x, y)

			// Calculate difference (handle wraparound)
			diff := (val - pred) & maxVal
			if diff > maxVal/2 {
				diff -= maxVal + 1
			}

			// Encode the difference
			ssss := categorize(diff)
			if err := e.encodeHuffman(bw, ht, ssss); err != nil {
				return err
			}

			// Encode additional bits
			if ssss > 0 {
				if diff < 0 {
					diff = diff + (1 << ssss) - 1
				}
				bw.writeBits(diff, ssss)
			}
		}
		prevRow, currRow = currRow, prevRow
	}

	return bw.flush()
}

func (e *encoder) encodeHuffman(bw *bitWriter, ht *huffmanTable, ssss int) error {
	// Find code for this symbol
	for i, val := range ht.values {
		if int(val) == ssss {
			bw.writeBits(int(ht.codes[i]), ht.sizes[i])
			return nil
		}
	}
	return nil
}

// bitWriter writes bits to an io.Writer with byte stuffing
type bitWriter struct {
	w    io.Writer
	buf  uint32
	bits int
}

func newBitWriter(w io.Writer) *bitWriter {
	return &bitWriter{w: w}
}

func (b *bitWriter) writeBits(val, n int) {
	b.buf = (b.buf << n) | uint32(val&((1<<n)-1))
	b.bits += n

	for b.bits >= 8 {
		b.bits -= 8
		byteVal := byte(b.buf >> b.bits)
		b.w.Write([]byte{byteVal})
		if byteVal == 0xFF {
			b.w.Write([]byte{0x00}) // Byte stuffing
		}
	}
}

func (b *bitWriter) flush() error {
	if b.bits > 0 {
		// Pad with 1s
		b.buf = (b.buf << (8 - b.bits)) | ((1 << (8 - b.bits)) - 1)
		byteVal := byte(b.buf)
		b.w.Write([]byte{byteVal})
		if byteVal == 0xFF {
			b.w.Write([]byte{0x00})
		}
	}
	return nil
}
