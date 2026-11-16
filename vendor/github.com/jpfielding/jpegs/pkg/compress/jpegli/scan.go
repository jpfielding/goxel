package jpegli

import "fmt"

// bitReader reads the entropy-coded segment of a scan from an in-memory
// stream: MSB first, 0xFF 0x00 unstuffed to 0xFF. It never reads past a
// marker; when it reaches one (or the end of the data) it stops and records
// where, so the caller can handle restarts and the end of the scan. Running
// out of bits is a truncation error, never zero padding.
type bitReader struct {
	data []byte
	pos  int    // next unread byte
	acc  uint64 // bit accumulator; the low n bits are valid
	n    int

	stopped   bool // reached a marker or the end of data
	markerPos int  // offset of the 0xFF that starts the marker (or len(data))
	marker    byte // marker code; 0 when the data ended instead

	restarts bool // a DRI is in effect: RSTn markers are expected
}

func (b *bitReader) fill() {
	for b.n <= 56 && !b.stopped {
		if b.pos >= len(b.data) {
			b.stopped, b.markerPos = true, len(b.data)
			return
		}
		c := b.data[b.pos]
		if c != 0xFF {
			b.acc = b.acc<<8 | uint64(c)
			b.n += 8
			b.pos++
			continue
		}
		k := b.pos + 1
		if k < len(b.data) && b.data[k] == 0x00 { // stuffed 0xFF
			b.acc = b.acc<<8 | 0xFF
			b.n += 8
			b.pos += 2
			continue
		}
		for k < len(b.data) && b.data[k] == 0xFF { // fill bytes before a marker
			k++
		}
		b.stopped, b.markerPos = true, b.pos
		if k < len(b.data) {
			b.marker = b.data[k]
		}
		return
	}
}

// errEnd is the error for running out of entropy-coded data mid-scan: an
// illegal restart marker when the reader stopped at RSTn without a DRI,
// otherwise truncation.
func (b *bitReader) errEnd() error {
	if !b.restarts && b.marker >= 0xD0 && b.marker <= 0xD7 {
		return fmt.Errorf("%w: restart marker RST%d at offset %d without DRI", ErrInvalidData, b.marker-0xD0, b.markerPos)
	}
	return &TruncatedError{Offset: b.markerPos, Context: "entropy-coded scan"}
}

// readBits reads n <= 16 bits.
func (b *bitReader) readBits(n int) (int, error) {
	if b.n < n {
		b.fill()
		if b.n < n {
			return 0, b.errEnd()
		}
	}
	b.n -= n
	return int(b.acc>>b.n) & (1<<n - 1), nil
}

// decodeHuffman decodes one symbol with the T.81 F.2.2.3 procedure, using
// the 8-bit lookup for short codes.
func (b *bitReader) decodeHuffman(ht *huffmanTable) (int, error) {
	if b.n < 16 {
		b.fill()
		if b.n == 0 {
			return 0, b.errEnd()
		}
	}
	// Top 16 bits, zero-padded past the available bits. A code that needs a
	// padded bit is truncation, detected below by comparing its length.
	var peek uint32
	if b.n >= 16 {
		peek = uint32(b.acc>>(b.n-16)) & 0xFFFF
	} else {
		peek = uint32(b.acc<<(16-b.n)) & 0xFFFF
	}
	if e := ht.lookup[peek>>8]; e >= 0 {
		size := int(e >> 8)
		if size > b.n {
			return 0, b.errEnd()
		}
		b.n -= size
		return int(e & 0xFF), nil
	}
	for l := 9; l <= 16; l++ {
		code := int32(peek >> (16 - l))
		if code <= ht.maxcode[l] {
			if l > b.n {
				return 0, b.errEnd()
			}
			b.n -= l
			return int(ht.values[ht.valptr[l]+int(code-ht.mincode[l])]), nil
		}
	}
	if b.n < 16 {
		return 0, b.errEnd()
	}
	return 0, fmt.Errorf("%w: invalid Huffman code 0x%04X near offset %d", ErrInvalidData, peek, b.pos)
}

// checkSegmentEnd verifies that only the padding bits of the current byte
// remain before the marker that ends an entropy-coded segment.
func (b *bitReader) checkSegmentEnd(where string) error {
	b.fill()
	if b.n >= 8 {
		return fmt.Errorf("%w: entropy-coded data continues past the %s (%d unread bits)", ErrInvalidData, where, b.n)
	}
	return nil
}

// peek16 returns the next 16 bits without consuming them; ok is false when
// fewer than 16 remain before the end of the segment.
func (b *bitReader) peek16() (v int, ok bool) {
	if b.n < 16 {
		b.fill()
		if b.n < 16 {
			return 0, false
		}
	}
	return int(b.acc>>(b.n-16)) & 0xFFFF, true
}

// padOnes reports whether the unread bits of the current byte are all 1s,
// the padding T.81 F.1.2.3 prescribes. Call after checkSegmentEnd.
func (b *bitReader) padOnes() bool {
	return b.acc&(1<<b.n-1) == 1<<b.n-1
}

// restart consumes the RSTn marker that must follow a restart interval
// (T.81 H.1.1, B.2.1): discard the padding bits, require RST(want), and
// resume at the next entropy-coded segment.
func (b *bitReader) restart(want int) error {
	if err := b.checkSegmentEnd("restart interval"); err != nil {
		return err
	}
	switch {
	case b.marker == 0:
		return &TruncatedError{Offset: b.markerPos, Context: "restart marker"}
	case int(b.marker) != 0xD0+want:
		return fmt.Errorf("%w: expected RST%d at offset %d, got marker 0xFF%02X", ErrInvalidData, want, b.markerPos, b.marker)
	}
	// Skip 0xFF, any fill bytes, and the marker code.
	k := b.markerPos + 1
	for b.data[k] == 0xFF {
		k++
	}
	b.pos, b.acc, b.n = k+1, 0, 0
	b.stopped, b.marker = false, 0
	return nil
}

// predict applies predictor sel (T.81 Table H.1) to the neighbours Ra
// (left), Rb (above) and Rc (above-left). The divisions by 2 are arithmetic
// right shifts, as in libjpeg-turbo and libjpeg (IJG/Richter); they differ
// from truncating division for negative odd differences.
func predict(sel, ra, rb, rc int) int {
	switch sel {
	case 1:
		return ra
	case 2:
		return rb
	case 3:
		return rc
	case 4:
		return ra + rb - rc
	case 5:
		return ra + (rb-rc)>>1
	case 6:
		return rb + (ra-rc)>>1
	default: // 7, validated by readSOS
		return (ra + rb) >> 1
	}
}

// extend converts SSSS appended bits to a signed difference (T.81 F.2.2.1).
func extend(bits, ssss int) int {
	if bits < 1<<(ssss-1) {
		return bits - (1<<ssss - 1)
	}
	return bits
}

// storeSample writes sample v at index i of an image.Gray (wide=false) or
// big-endian image.Gray16 (wide=true) pixel buffer.
func storeSample(pix []byte, i, v int, wide bool) {
	if wide {
		pix[2*i] = byte(v >> 8)
		pix[2*i+1] = byte(v)
		return
	}
	pix[i] = byte(v)
}

// decodeScan decodes the T.81 Annex H entropy-coded scan starting at d.pos
// into pix and then reads the trailer up to EOI. saw16 reports whether any
// sample had difference category 16; d.legacyMark records whether the 16
// bits after the first one were 0x7FFF, the legacy encoder's appended bits.
//
// The DPCM loop runs in the reduced domain P' = P - Pt; reconstructed
// samples are shifted left by Pt on store. A restart resets prediction:
// the first row of each interval is predicted as the first row of the scan.
func (d *Decoder) decodeScan(ht *huffmanTable, pix []byte, wide bool) (saw16 bool, err error) {
	w, h, pt := d.width, d.height, d.pointTrans
	pp := d.precision - pt
	mask := 1<<pp - 1
	br := &bitReader{data: d.data, pos: d.pos, restarts: d.restartInterval > 0}
	d.legacyMark = false
	restartRows := d.restartInterval / w

	prev, cur := make([]int, w), make([]int, w)
	rst := 0
	for y := 0; y < h; y++ {
		first := y == 0
		if restartRows > 0 && y > 0 && y%restartRows == 0 {
			if err := br.restart(rst); err != nil {
				return saw16, err
			}
			rst = (rst + 1) & 7
			first = true
		}
		for x := 0; x < w; x++ {
			ssss, err := br.decodeHuffman(ht)
			if err != nil {
				return saw16, err
			}
			var diff int
			switch {
			case ssss > 16:
				return saw16, fmt.Errorf("%w: difference category %d at sample (%d,%d)", ErrInvalidData, ssss, x, y)
			case ssss == 16:
				// T.81 H.1.2.2: category 16 is the difference 32768 with
				// no appended bits, possible only when P' = 16.
				if pp != 16 {
					return saw16, fmt.Errorf("%w: difference category 16 at sample (%d,%d) with P' = %d", ErrInvalidData, x, y, pp)
				}
				if !saw16 {
					v, ok := br.peek16()
					d.legacyMark = ok && v == 0x7FFF
				}
				diff, saw16 = 32768, true
			case ssss > 0:
				bits, err := br.readBits(ssss)
				if err != nil {
					return saw16, err
				}
				diff = extend(bits, ssss)
			}

			var pred int
			switch {
			case first && x == 0:
				pred = 1 << (pp - 1)
			case first:
				pred = cur[x-1]
			case x == 0:
				pred = prev[0]
			default:
				pred = predict(d.predictor, cur[x-1], prev[x], prev[x-1])
			}
			v := (pred + diff) & mask
			cur[x] = v
			storeSample(pix, y*w+x, v<<pt, wide)
		}
		prev, cur = cur, prev
	}

	if err := br.checkSegmentEnd("last sample"); err != nil {
		return saw16, err
	}
	d.pos = br.markerPos
	return saw16, d.readTrailer()
}
