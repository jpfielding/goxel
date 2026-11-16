package jpegli

import "fmt"

// Legacy decoder - FROZEN.
//
// Before this package was T.81 conformant (jpegs <= v0.2.0), its encoder
// differed from T.81 Annex H in two ways that change the scan data:
//
//   - A difference of category SSSS=16 was followed by 16 appended bits
//     (the value -32768 in one's-complement form). T.81 H.1.2.2 says
//     category 16 carries no appended bits.
//   - Predictors 5 and 6 halved (Rb-Rc) and (Ra-Rc) with truncating
//     division instead of an arithmetic shift.
//
// It only ever wrote one component with component ID 1, the fixed 17-symbol
// DC table (see defaultTable), no restart interval and no point transform
// shift. Streams in that format exist in the wild, so this file reproduces
// the scan decode for it; decodeAuto decides when to use it. Only 16-bit
// streams can contain SSSS=16. A legacy stream with predictor 5 or 6 and no
// SSSS=16 parses identically in both formats, so it can not be recognised
// and decodes as T.81 (with different rounding); decode(data, profileLegacy)
// reads it as written.
//
// The bit reader and Huffman decode are shared with the conformant decoder
// (they implement T.81 entropy decoding, which the legacy encoder followed).
// Do not "fix" the prediction or SSSS=16 handling below.

// legacyEligible reports whether the stream's headers match what the legacy
// encoder wrote for a stream that can contain SSSS=16.
func (d *Decoder) legacyEligible(ht *huffmanTable) bool {
	if d.precision != 16 || d.pointTrans != 0 || d.restartInterval != 0 || d.compInfo[0].id != 1 {
		return false
	}
	def := defaultTable()
	return ht.bits == def.bits && string(ht.values) == string(def.values)
}

func predictLegacy(sel, ra, rb, rc int) int {
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
		return ra + (rb-rc)/2
	case 6:
		return rb + (ra-rc)/2
	default:
		return (ra + rb) / 2
	}
}

// decodeLegacyScan decodes a legacy 16-bit scan (P = 16, Pt = 0, no
// restarts) into the big-endian Gray16 buffer pix. Like decodeScan it is
// strict: every sample must be coded and the scan must end at EOI.
func (d *Decoder) decodeLegacyScan(ht *huffmanTable, pix []byte) error {
	if !d.legacyEligible(ht) {
		return fmt.Errorf("%w: stream does not match the legacy encoder's headers", ErrUnsupported)
	}
	w, h := d.width, d.height
	br := &bitReader{data: d.data, pos: d.pos}
	prev, cur := make([]int, w), make([]int, w)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			ssss, err := br.decodeHuffman(ht)
			if err != nil {
				return err
			}
			var diff int
			if ssss > 0 {
				bits, err := br.readBits(ssss)
				if err != nil {
					return err
				}
				// The legacy encoder only produced category 16 for the
				// difference -32768, always appending 0x7FFF.
				if ssss == 16 && bits != 0x7FFF {
					return fmt.Errorf("%w: legacy SSSS=16 appended bits 0x%04X, want 0x7FFF", ErrInvalidData, bits)
				}
				diff = extend(bits, ssss)
			}
			var pred int
			switch {
			case y == 0 && x == 0:
				pred = 1 << 15
			case y == 0:
				pred = cur[x-1]
			case x == 0:
				pred = prev[0]
			default:
				pred = predictLegacy(d.predictor, cur[x-1], prev[x], prev[x-1])
			}
			v := (pred + diff) & 0xFFFF
			cur[x] = v
			storeSample(pix, y*w+x, v, true)
		}
		prev, cur = cur, prev
	}
	if err := br.checkSegmentEnd("last sample"); err != nil {
		return err
	}
	if !br.padOnes() { // the legacy encoder always padded with 1-bits
		return fmt.Errorf("%w: legacy scan padding is not all 1-bits", ErrInvalidData)
	}
	d.pos = br.markerPos
	return d.readTrailer()
}
