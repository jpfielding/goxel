package jpegli

import (
	"fmt"
	"image"
	"io"
)

// Options configures JPEG Lossless encoding. The zero value encodes with
// predictor 1, no point transform, no restarts and a precision derived from
// the image.
type Options struct {
	// Predictor is the selection value (SOS Ss), 1..7. Zero means 1.
	Predictor int
	// PointTransform (SOS Al), 0..Precision-1. Samples are shifted right by
	// it before coding, so the low PointTransform bits are lost.
	PointTransform int
	// Precision is the sample precision P (SOF3), 2..16. Zero means 8 for
	// *image.Gray and 16 otherwise. Every sample must fit in P bits.
	Precision int
	// RestartIntervalRows emits a restart marker every this many rows; zero
	// disables restarts. RestartIntervalRows × width must fit the 16-bit DRI
	// field.
	RestartIntervalRows int
}

// Encode writes img to w as a T.81 Annex H (Process 14) JPEG Lossless
// stream: one component, the fixed 17-symbol DC Huffman table, SOI through
// EOI. *image.Gray and *image.Gray16 samples are coded as stored; for other
// image types the red channel of RGBA() is scaled to the precision. A nil
// opts uses the Options zero value.
func Encode(w io.Writer, img image.Image, opts *Options) error {
	var o Options
	if opts != nil {
		o = *opts
	}
	b := img.Bounds()
	width, height := b.Dx(), b.Dy()
	if width < 1 || height < 1 || width > 65535 || height > 65535 {
		return fmt.Errorf("%w: dimensions %dx%d (allowed 1..65535)", ErrInvalidParameter, width, height)
	}
	if o.Precision == 0 {
		o.Precision = 16
		if _, ok := img.(*image.Gray); ok {
			o.Precision = 8
		}
	}
	if o.Predictor == 0 {
		o.Predictor = 1
	}
	switch {
	case o.Precision < 2 || o.Precision > 16:
		return fmt.Errorf("%w: precision %d (allowed 2..16)", ErrInvalidParameter, o.Precision)
	case o.Predictor < 1 || o.Predictor > 7:
		return fmt.Errorf("%w: predictor %d (allowed 1..7)", ErrInvalidParameter, o.Predictor)
	case o.PointTransform < 0 || o.PointTransform >= o.Precision:
		return fmt.Errorf("%w: point transform %d (allowed 0..%d)", ErrInvalidParameter, o.PointTransform, o.Precision-1)
	case o.RestartIntervalRows < 0 || o.RestartIntervalRows > 65535 || o.RestartIntervalRows*width > 65535:
		return fmt.Errorf("%w: restart interval %d rows x width %d does not fit the 16-bit DRI field", ErrInvalidParameter, o.RestartIntervalRows, width)
	}

	ht := defaultTable()
	out := make([]byte, 0, 64+width*height*o.Precision/8)
	out = append(out, 0xFF, 0xD8) // SOI
	// JFIF APP0: version 1.1, no units, density 1x1, no thumbnail.
	out = append(out, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00)
	// SOF3: P, Y, X, Nf = 1, component 1 sampled 1x1, Tq = 0.
	out = append(out, 0xFF, 0xC3, 0x00, 0x0B, byte(o.Precision),
		byte(height>>8), byte(height), byte(width>>8), byte(width), 1, 1, 0x11, 0)
	// DHT: DC table 0.
	out = append(out, 0xFF, 0xC4, 0x00, byte(2+1+16+len(ht.values)), 0x00)
	for l := 1; l <= 16; l++ {
		out = append(out, byte(ht.bits[l]))
	}
	out = append(out, ht.values...)
	if o.RestartIntervalRows > 0 {
		ri := o.RestartIntervalRows * width
		out = append(out, 0xFF, 0xDD, 0x00, 0x04, byte(ri>>8), byte(ri))
	}
	// SOS: Ns = 1, component 1 with DC table 0, Ss = predictor, Se = 0,
	// Ah = 0, Al = point transform.
	out = append(out, 0xFF, 0xDA, 0x00, 0x08, 1, 1, 0x00, byte(o.Predictor), 0, byte(o.PointTransform))

	out, err := encodeScan(out, img, ht, o)
	if err != nil {
		return err
	}
	out = append(out, 0xFF, 0xD9) // EOI
	if _, err := w.Write(out); err != nil {
		return fmt.Errorf("jpegli: write: %w", err)
	}
	return nil
}

// defaultTable is the fixed DC table the encoder writes: 17 symbols
// (SSSS 0..16) with code lengths 2, 3×5, then 4..14.
func defaultTable() *huffmanTable {
	ht := &huffmanTable{
		bits:   [17]int{0, 0, 1, 5, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 1, 0, 0},
		values: []byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
	}
	generateHuffmanCodes(ht)
	return ht
}

// rowSamples writes row y (relative to the bounds) of img into dst.
func rowSamples(img image.Image, y, precision int, dst []int) {
	b := img.Bounds()
	switch m := img.(type) {
	case *image.Gray:
		row := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := range dst {
			dst[x] = int(row[x])
		}
	case *image.Gray16:
		row := m.Pix[m.PixOffset(b.Min.X, b.Min.Y+y):]
		for x := range dst {
			dst[x] = int(row[2*x])<<8 | int(row[2*x+1])
		}
	default:
		for x := range dst {
			r, _, _, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			dst[x] = int(r >> (16 - precision))
		}
	}
}

// encodeScan appends the entropy-coded scan to out. It mirrors decodeScan:
// reduced domain P' = P - Pt, prediction reset at each restart.
func encodeScan(out []byte, img image.Image, ht *huffmanTable, o Options) ([]byte, error) {
	b := img.Bounds()
	w, h, pt := b.Dx(), b.Dy(), o.PointTransform
	pp := o.Precision - pt

	var code, size [17]int
	for k, v := range ht.values {
		code[v], size[v] = int(ht.codes[k]), ht.sizes[k]
	}
	bw := bitWriter{out: out}
	src := make([]int, w)
	prev, cur := make([]int, w), make([]int, w)
	rst := 0
	for y := 0; y < h; y++ {
		first := y == 0
		if o.RestartIntervalRows > 0 && y > 0 && y%o.RestartIntervalRows == 0 {
			bw.flush()
			bw.out = append(bw.out, 0xFF, byte(0xD0+rst))
			rst = (rst + 1) & 7
			first = true
		}
		rowSamples(img, y, o.Precision, src)
		for x := 0; x < w; x++ {
			if src[x]>>o.Precision != 0 {
				return nil, fmt.Errorf("%w: sample %d at (%d,%d) exceeds precision %d", ErrInvalidParameter, src[x], x, y, o.Precision)
			}
			v := src[x] >> pt
			cur[x] = v
			var pred int
			switch {
			case first && x == 0:
				pred = 1 << (pp - 1)
			case first:
				pred = cur[x-1]
			case x == 0:
				pred = prev[0]
			default:
				pred = predict(o.Predictor, cur[x-1], prev[x], prev[x-1])
			}
			// T.81 H.1.2.1: the difference is taken modulo 2^16, not 2^P'.
			// Below P' = 16 it never wraps; at 16, 32768 codes as -32768
			// (category 16). Decoders such as libjpeg-turbo reconstruct
			// modulo 2^16, so wrapping modulo 2^P' corrupts their output.
			diff := (v - pred) & 0xFFFF
			if diff >= 0x8000 {
				diff -= 0x10000
			}
			ssss := categorize(diff)
			bw.writeBits(code[ssss], size[ssss])
			// T.81 H.1.2.2: category 16 (difference 32768) has no
			// appended bits.
			if ssss > 0 && ssss < 16 {
				if diff < 0 {
					diff += 1<<ssss - 1
				}
				bw.writeBits(diff, ssss)
			}
		}
		prev, cur = cur, prev
	}
	bw.flush()
	return bw.out, nil
}

// categorize returns the SSSS category (bit length of |diff|).
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

// bitWriter appends bits MSB first with 0xFF 0x00 byte stuffing.
type bitWriter struct {
	out  []byte
	acc  uint32
	bits int
}

func (b *bitWriter) writeBits(val, n int) {
	b.acc = b.acc<<n | uint32(val)&(1<<n-1)
	b.bits += n
	for b.bits >= 8 {
		b.bits -= 8
		c := byte(b.acc >> b.bits)
		b.out = append(b.out, c)
		if c == 0xFF {
			b.out = append(b.out, 0x00)
		}
	}
}

// flush pads the partial byte with 1-bits (T.81 F.1.2.3) and byte-aligns.
func (b *bitWriter) flush() {
	if b.bits > 0 {
		b.writeBits(1<<(8-b.bits)-1, 8-b.bits)
	}
	b.acc = 0
}
