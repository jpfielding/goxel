package jpeg2k

import (
	"fmt"
	"math/bits"
)

// EBCOT tier-1 block coder (ITU-T T.800 Annex D) for reversible code-blocks.
// Ported from dicos.rs crates/jpeg2k/src/ebcot.rs.
//
// One tier1 state machine drives both encoding and decoding, so the scan
// order and every context rule exist exactly once. Coding runs in 4-row
// stripes (Figure D.2): columns left to right, rows top to bottom within a
// column. Each bit-plane gets significance propagation (D.3.1), magnitude
// refinement (D.3.3) and cleanup with run-length mode (D.4); the most
// significant plane carries cleanup only, so a block with nb magnitude
// bit-planes has 3*nb-2 passes.

// codedBlock is a tier-1 coded code-block plus what tier-2 must signal.
type codedBlock struct {
	data      []byte // single MQ codeword segment; empty for an all-zero block
	passes    int    // 3*bitplanes-2, or 0
	bitplanes int    // magnitude bit-planes, 0 for an all-zero block
}

// encodeCodeBlock codes w*h row-major signed subband samples.
func encodeCodeBlock(coeffs []int32, w, h int, band bandKind) codedBlock {
	var maxMag uint32
	for _, v := range coeffs {
		maxMag = max(maxMag, absU32(v))
	}
	if maxMag == 0 {
		return codedBlock{}
	}
	nb := 32 - bits.LeadingZeros32(maxMag)

	t := newTier1(w, h, band, nil)
	// A neighbour's sign is only consulted once it is significant, and it is
	// coded at that moment, so pre-seeding every negative sample's SIGN flag
	// matches what the decoder reconstructs.
	for y := range h {
		for x := range w {
			v := coeffs[y*w+x]
			t.mag[y*w+x] = absU32(v)
			if v < 0 {
				t.flags[t.idx(x, y)] |= flagSign
			}
		}
	}

	t.cleanupPass(nb - 1)
	for p := nb - 2; p >= 0; p-- {
		t.sigPropPass(p)
		t.magRefPass(p)
		t.cleanupPass(p)
	}
	t.enc.flush()
	return codedBlock{data: t.enc.bytes(), passes: 3*nb - 2, bitplanes: nb}
}

// decodeCodeBlock reconstructs w*h signed samples from a codeword. Any pass
// prefix 1..3*nb-2 is honoured; malformed data never reads out of bounds
// (the MQ decoder feeds 1-bits past the end).
func decodeCodeBlock(data []byte, w, h int, band bandKind, bitplanes, passes int) ([]int32, error) {
	if bitplanes == 0 {
		if passes != 0 {
			return nil, fmt.Errorf("%w: code-block has 0 bit-planes but %d passes", ErrInvalidData, passes)
		}
		return make([]int32, w*h), nil
	}
	// The top plane shifts by nb-1 and the magnitude must fit int32.
	if bitplanes < 0 || bitplanes >= 32 {
		return nil, fmt.Errorf("%w: %d code-block bit-planes exceed the int32 coefficient range", ErrInvalidData, bitplanes)
	}
	nb := bitplanes
	if maxPasses := 3*nb - 2; passes <= 0 || passes > maxPasses {
		return nil, fmt.Errorf("%w: %d passes out of range 1..%d for %d bit-planes", ErrInvalidData, passes, maxPasses, nb)
	}

	t := newTier1(w, h, band, newMQDecoder(data))
	t.cleanupPass(nb - 1)
	done := 1
	for p := nb - 2; p >= 0 && done < passes; p-- {
		t.sigPropPass(p)
		if done++; done >= passes {
			break
		}
		t.magRefPass(p)
		if done++; done >= passes {
			break
		}
		t.cleanupPass(p)
		done++
	}

	out := make([]int32, w*h)
	for y := range h {
		for x := range w {
			m := int32(t.mag[y*w+x])
			if t.flags[t.idx(x, y)]&flagSign != 0 {
				m = -m
			}
			out[y*w+x] = m
		}
	}
	return out, nil
}

func absU32(v int32) uint32 {
	if v < 0 {
		return uint32(-int64(v))
	}
	return uint32(v)
}

// Per-sample coding flags.
const (
	flagSig     uint8 = 1 << 0 // σ: significant
	flagVisited uint8 = 1 << 1 // π: coded in this plane's SPP
	flagRefined uint8 = 1 << 2 // σ': refined at least once
	flagSign    uint8 = 1 << 3 // negative
)

// tier1 is the shared coding state. Exactly one of enc / dec is set.
type tier1 struct {
	enc      *mqEncoder
	dec      *mqDecoder
	contexts [numMQContexts]mqState
	// flags has a one-sample insignificant border so neighbourhood queries
	// need no edge tests.
	flags  []uint8
	stride int
	mag    []uint32 // source magnitudes on encode, reconstruction on decode
	w, h   int
	band   bandKind
}

func newTier1(w, h int, band bandKind, dec *mqDecoder) *tier1 {
	t := &tier1{
		dec:      dec,
		contexts: defaultContexts(),
		flags:    make([]uint8, (w+2)*(h+2)),
		stride:   w + 2,
		mag:      make([]uint32, w*h),
		w:        w,
		h:        h,
		band:     band,
	}
	if dec == nil {
		t.enc = newMQEncoder()
	}
	return t
}

func (t *tier1) idx(x, y int) int { return (y+1)*t.stride + x + 1 }

// code is the single coding primitive: on encode it codes known in ctx and
// returns it; on decode it ignores known and returns the decoded bit.
func (t *tier1) code(ctx int, known uint8) uint8 {
	if t.enc != nil {
		t.enc.encode(known, &t.contexts[ctx])
		return known
	}
	return t.dec.decode(&t.contexts[ctx])
}

func (t *tier1) sig(i int) int { return int(t.flags[i] & flagSig) }

// neighbors returns the significant horizontal, vertical and diagonal
// neighbour counts of the sample at i.
func (t *tier1) neighbors(i int) (h, v, d int) {
	s := t.stride
	h = t.sig(i-1) + t.sig(i+1)
	v = t.sig(i-s) + t.sig(i+s)
	d = t.sig(i-s-1) + t.sig(i-s+1) + t.sig(i+s-1) + t.sig(i+s+1)
	return h, v, d
}

// zcLL is the zero-coding context for LL and LH bands (T.800 Table D.1).
func zcLL(h, v, d int) int {
	switch {
	case h == 2:
		return 8
	case h == 1 && v >= 1:
		return 7
	case h == 1 && d >= 1:
		return 6
	case h == 1:
		return 5
	case v == 2:
		return 4
	case v == 1:
		return 3
	case d >= 2:
		return 2
	case d == 1:
		return 1
	default:
		return 0
	}
}

// zcHH is the zero-coding context for HH bands (T.800 Table D.1).
func zcHH(h, v, d int) int {
	hv := h + v
	switch {
	case d >= 3:
		return 8
	case d == 2 && hv >= 1:
		return 7
	case d == 2:
		return 6
	case d == 1 && hv >= 2:
		return 5
	case d == 1 && hv == 1:
		return 4
	case d == 1:
		return 3
	case hv >= 2:
		return 2
	case hv == 1:
		return 1
	default:
		return 0
	}
}

func (t *tier1) zcContext(i int) int {
	h, v, d := t.neighbors(i)
	switch t.band {
	case bandHL:
		return zcLL(v, h, d)
	case bandHH:
		return zcHH(h, v, d)
	default: // LL, LH
		return zcLL(h, v, d)
	}
}

// signContrib is 0 for an insignificant neighbour, else +1 / -1 by sign.
func (t *tier1) signContrib(i int) int {
	switch {
	case t.flags[i]&flagSig == 0:
		return 0
	case t.flags[i]&flagSign != 0:
		return -1
	default:
		return 1
	}
}

func clampUnit(v int) int { return max(-1, min(1, v)) }

// scContext returns the sign-coding context offset and XOR bit (T.800
// Tables D.2 and D.3).
func (t *tier1) scContext(i int) (ctx int, xor uint8) {
	s := t.stride
	hc := clampUnit(t.signContrib(i-1) + t.signContrib(i+1))
	vc := clampUnit(t.signContrib(i-s) + t.signContrib(i+s))
	if hc < 0 || (hc == 0 && vc < 0) {
		hc, vc, xor = -hc, -vc, 1
	}
	// Now hc in {0, 1}; hc == 0 implies vc in {0, 1}.
	if hc == 0 {
		return vc, xor
	}
	return 3 + vc, xor
}

// mrContext is the magnitude-refinement context (T.800 Table D.4).
func (t *tier1) mrContext(i int) int {
	switch {
	case t.flags[i]&flagRefined != 0:
		return 2
	case t.zcAny(i):
		return 1
	default:
		return 0
	}
}

func (t *tier1) zcAny(i int) bool {
	h, v, d := t.neighbors(i)
	return h+v+d > 0
}

// codeSign codes the sign of the just-significant sample at i.
func (t *tier1) codeSign(i int) {
	ctx, xor := t.scContext(i)
	var known uint8
	if t.enc != nil && t.flags[i]&flagSign != 0 {
		known = 1
	}
	if t.code(ctxSCStart+ctx, known^xor)^xor == 1 {
		t.flags[i] |= flagSign
	}
}

// codeSignificance codes the plane-p significance bit of an insignificant
// sample, then its sign if it became significant.
func (t *tier1) codeSignificance(x, y, p, zc int) {
	i := t.idx(x, y)
	ci := y*t.w + x
	if t.code(ctxZCStart+zc, uint8(t.mag[ci]>>p&1)) == 1 {
		t.flags[i] |= flagSig
		t.mag[ci] |= 1 << p
		t.codeSign(i)
	}
}

// sigPropPass is the significance-propagation pass (D.3.1): insignificant
// samples with a significant neighbour (live state) are coded and marked
// visited.
func (t *tier1) sigPropPass(p int) {
	for y0 := 0; y0 < t.h; y0 += 4 {
		rows := min(t.h-y0, 4)
		for x := range t.w {
			for r := range rows {
				i := t.idx(x, y0+r)
				if t.flags[i]&flagSig != 0 {
					continue
				}
				zc := t.zcContext(i)
				if zc == 0 {
					continue
				}
				t.flags[i] |= flagVisited
				t.codeSignificance(x, y0+r, p, zc)
			}
		}
	}
}

// magRefPass is the magnitude-refinement pass (D.3.3): significant samples
// not visited by this plane's SPP.
func (t *tier1) magRefPass(p int) {
	for y0 := 0; y0 < t.h; y0 += 4 {
		rows := min(t.h-y0, 4)
		for x := range t.w {
			for r := range rows {
				y := y0 + r
				i := t.idx(x, y)
				if t.flags[i]&flagSig == 0 || t.flags[i]&flagVisited != 0 {
					continue
				}
				ci := y*t.w + x
				bit := t.code(ctxMRStart+t.mrContext(i), uint8(t.mag[ci]>>p&1))
				t.mag[ci] |= uint32(bit) << p
				t.flags[i] |= flagRefined
			}
		}
	}
}

// cleanupPass is the cleanup pass (D.4) with run-length mode; it codes the
// remaining insignificant, unvisited samples and then clears π everywhere.
func (t *tier1) cleanupPass(p int) {
	for y0 := 0; y0 < t.h; y0 += 4 {
		rows := min(t.h-y0, 4)
		for x := range t.w {
			if rows == 4 && t.rlEligible(x, y0) {
				t.cleanupRL(x, y0, p)
				continue
			}
			for r := range rows {
				i := t.idx(x, y0+r)
				if t.flags[i]&(flagSig|flagVisited) != 0 {
					continue
				}
				t.codeSignificance(x, y0+r, p, t.zcContext(i))
			}
		}
	}
	for i := range t.flags {
		t.flags[i] &^= flagVisited
	}
}

// rlEligible reports whether a full stripe column qualifies for run-length
// mode: all four samples insignificant, unvisited, with zero ZC context.
func (t *tier1) rlEligible(x, y0 int) bool {
	for r := range 4 {
		i := t.idx(x, y0+r)
		if t.flags[i]&(flagSig|flagVisited) != 0 || t.zcContext(i) != 0 {
			return false
		}
	}
	return true
}

// cleanupRL codes one run-length-eligible stripe column (D.4.2).
func (t *tier1) cleanupRL(x, y0, p int) {
	var runKnown, rKnown uint8
	if t.enc != nil {
		for r := range 4 {
			if t.mag[(y0+r)*t.w+x]>>p&1 == 1 {
				runKnown, rKnown = 1, uint8(r)
				break
			}
		}
	}
	if t.code(ctxRunLength, runKnown) == 0 {
		return
	}
	// Two uniform-context bits, MSB first: the first significant row.
	b1 := t.code(ctxUniform, rKnown>>1&1)
	b0 := t.code(ctxUniform, rKnown&1)
	r := int(b1<<1 | b0)

	i := t.idx(x, y0+r)
	t.flags[i] |= flagSig
	t.mag[(y0+r)*t.w+x] |= 1 << p
	t.codeSign(i)

	for rr := r + 1; rr < 4; rr++ {
		t.codeSignificance(x, y0+rr, p, t.zcContext(t.idx(x, y0+rr)))
	}
}
