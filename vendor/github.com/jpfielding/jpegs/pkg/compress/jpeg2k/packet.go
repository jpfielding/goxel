package jpeg2k

import (
	"fmt"
	"math/bits"
)

// Tier-2 packet-header codec (ITU-T T.800 B.10), single quality layer.
// Ported from dicos.rs crates/jpeg2k/src/packet.rs.
//
// For this profile (LRCP, 1 layer, 1 component, 1 precinct per band) there
// is one packet per resolution covering its bands in order (LL at r=0; HL,
// LH, HH above). Header bits are MSB first with B.10.1 stuffing: the byte
// after 0xFF carries 7 bits, and a header never ends in 0xFF. Pass counts
// use T.800 Table B.4 (as OpenJPEG's opj_t2_{put,get}numpasses).

// packetBitWriter writes MSB-first bits with B.10.1 0xFF stuffing.
type packetBitWriter struct {
	buf    []byte
	cur    byte
	filled int
	cap    int // 8, or 7 after a 0xFF byte
}

func newPacketBitWriter() *packetBitWriter { return &packetBitWriter{cap: 8} }

func (w *packetBitWriter) putBit(b uint8) {
	w.cur = w.cur<<1 | b&1
	w.filled++
	if w.filled == w.cap {
		w.emit()
	}
}

// putBits writes the low n bits of v, MSB first.
func (w *packetBitWriter) putBits(v uint64, n int) {
	for i := n - 1; i >= 0; i-- {
		w.putBit(uint8(v >> i & 1))
	}
}

func (w *packetBitWriter) emit() {
	w.buf = append(w.buf, w.cur)
	w.cap = 8
	if w.cur == 0xFF {
		w.cap = 7
	}
	w.cur, w.filled = 0, 0
}

// finish pads the last byte with zeros and appends 0x00 after a final 0xFF.
func (w *packetBitWriter) finish() []byte {
	if w.filled > 0 {
		w.cur <<= w.cap - w.filled
		w.emit()
	}
	if n := len(w.buf); n > 0 && w.buf[n-1] == 0xFF {
		w.buf = append(w.buf, 0x00)
	}
	return w.buf
}

// packetBitReader mirrors packetBitWriter with strict bounds: running out of
// data returns ErrInvalidData.
type packetBitReader struct {
	data   []byte
	pos    int
	cur    byte
	avail  int
	prevFF bool
}

func (r *packetBitReader) readBit() (uint8, error) {
	if r.avail == 0 {
		if r.pos >= len(r.data) {
			return 0, fmt.Errorf("%w: packet header truncated", ErrInvalidData)
		}
		r.cur = r.data[r.pos]
		r.pos++
		r.avail = 8
		if r.prevFF {
			r.avail = 7
		}
		r.prevFF = r.cur == 0xFF
	}
	r.avail--
	return r.cur >> r.avail & 1, nil
}

func (r *packetBitReader) readBits(n int) (uint64, error) {
	var v uint64
	for range n {
		b, err := r.readBit()
		if err != nil {
			return 0, err
		}
		v = v<<1 | uint64(b)
	}
	return v, nil
}

// consumed is the header length, including the 0x00 that must follow a
// header ending exactly on a 0xFF byte.
func (r *packetBitReader) consumed() (int, error) {
	n := r.pos
	if r.avail == 0 && r.pos > 0 && r.data[r.pos-1] == 0xFF {
		if r.pos >= len(r.data) {
			return 0, fmt.Errorf("%w: packet header truncated at stuffing byte", ErrInvalidData)
		}
		n++
	}
	return n, nil
}

// writeNumPasses codes 1..164 per T.800 Table B.4.
func writeNumPasses(w *packetBitWriter, n int) {
	switch {
	case n == 1:
		w.putBit(0)
	case n == 2:
		w.putBits(0b10, 2)
	case n <= 5:
		w.putBits(0xc|uint64(n-3), 4)
	case n <= 36:
		w.putBits(0x1e0|uint64(n-6), 9)
	default:
		w.putBits(0xff80|uint64(n-37), 16)
	}
}

func readNumPasses(r *packetBitReader) (int, error) {
	for _, step := range []struct{ bits, base, escape int }{
		{1, 1, 1}, {1, 2, 1}, {2, 3, 3}, {5, 6, 31}, {7, 37, -1},
	} {
		f, err := r.readBits(step.bits)
		if err != nil {
			return 0, err
		}
		if int(f) != step.escape {
			return step.base + int(f), nil
		}
	}
	panic("unreachable") // the last step has no escape
}

func floorLog2(v int) int { return bits.Len64(uint64(v)) - 1 }

// precinctState is the per-precinct packet state that persists across
// packets (one per band in this profile).
type precinctState struct {
	incl, zbp *tagTree
	lblock    []int // per code-block, starts at 3 (B.10.7.1)
}

func newPrecinctState(gridW, gridH int) *precinctState {
	lb := make([]int, gridW*gridH)
	for i := range lb {
		lb[i] = 3
	}
	return &precinctState{incl: newTagTree(gridW, gridH), zbp: newTagTree(gridW, gridH), lblock: lb}
}

// blockContribution is one code-block's entry recovered from a header; its
// body is the next length bytes after the header, in band-then-raster order.
type blockContribution struct {
	passes        int
	zeroBitplanes int
	length        int
}

// writePacket appends one packet (header then bodies) for bands to out.
// blocks[i] holds one entry per code-block of bands[i]; a nil entry or one
// with no bit-planes is not included. mb[i] is Mb for bands[i].
func writePacket(states []*precinctState, bands []band, blocks [][]*codedBlock, mb []int, out []byte) ([]byte, error) {
	included := func(c *codedBlock) bool { return c != nil && c.bitplanes > 0 }
	any := false
	for _, bb := range blocks {
		for _, c := range bb {
			any = any || included(c)
		}
	}
	w := newPacketBitWriter()
	if !any {
		w.putBit(0)
		return append(out, w.finish()...), nil
	}
	w.putBit(1)

	for bi, b := range bands {
		st, bb := states[bi], blocks[bi]
		// Populate every leaf before coding any node.
		for k, c := range bb {
			cx, cy := k%b.gridW, k/b.gridW
			nb, incl := 0, uint32(1)
			if included(c) {
				nb, incl = c.bitplanes, 0
			}
			if nb > mb[bi] {
				return nil, fmt.Errorf("%w: %d bit-planes exceed Mb %d (band %v)", ErrInvalidData, nb, mb[bi], b.kind)
			}
			st.incl.set(cx, cy, incl)
			st.zbp.set(cx, cy, uint32(mb[bi]-nb))
		}
		for k, c := range bb {
			cx, cy := k%b.gridW, k/b.gridW
			st.incl.encode(w.putBit, cx, cy, 1)
			if !included(c) {
				continue
			}
			zbp := uint32(mb[bi] - c.bitplanes)
			for t := uint32(1); ; t++ {
				st.zbp.encode(w.putBit, cx, cy, t)
				if zbp < t {
					break
				}
			}
			writeNumPasses(w, c.passes)

			passLog2 := floorLog2(c.passes)
			need := 1
			if len(c.data) > 0 {
				need = floorLog2(len(c.data)) + 1
			}
			inc := max(need-(st.lblock[k]+passLog2), 0)
			st.lblock[k] += inc
			for range inc {
				w.putBit(1)
			}
			w.putBit(0)
			w.putBits(uint64(len(c.data)), st.lblock[k]+passLog2)
		}
	}
	out = append(out, w.finish()...)
	for _, bb := range blocks {
		for _, c := range bb {
			if included(c) {
				out = append(out, c.data...)
			}
		}
	}
	return out, nil
}

// readPacket parses one packet header for bands. It returns per-band,
// per-block contributions (nil when not included) and the header length.
func readPacket(states []*precinctState, bands []band, mb []int, data []byte) ([][]*blockContribution, int, error) {
	r := &packetBitReader{data: data}
	out := make([][]*blockContribution, len(bands))
	for bi, b := range bands {
		out[bi] = make([]*blockContribution, len(b.cbs))
	}
	nonEmpty, err := r.readBit()
	if err != nil {
		return nil, 0, err
	}
	if nonEmpty == 0 {
		n, err := r.consumed()
		return out, n, err
	}

	for bi, b := range bands {
		st := states[bi]
		for k := range b.cbs {
			cx, cy := k%b.gridW, k/b.gridW
			incl, err := st.incl.decode(r.readBit, cx, cy, 1)
			if err != nil {
				return nil, 0, err
			}
			if !incl {
				continue
			}
			zbp := -1
			for t := uint32(1); zbp < 0; t++ {
				if t > uint32(mb[bi])+1 {
					return nil, 0, fmt.Errorf("%w: zero bit-plane count exceeds Mb %d (band %v)", ErrInvalidData, mb[bi], b.kind)
				}
				ok, err := st.zbp.decode(r.readBit, cx, cy, t)
				if err != nil {
					return nil, 0, err
				}
				if ok {
					zbp = int(t - 1)
				}
			}
			passes, err := readNumPasses(r)
			if err != nil {
				return nil, 0, err
			}
			inc := 0
			for {
				bit, err := r.readBit()
				if err != nil {
					return nil, 0, err
				}
				if bit == 0 {
					break
				}
				if inc++; inc > 32 {
					return nil, 0, fmt.Errorf("%w: Lblock increment runaway", ErrInvalidData)
				}
			}
			st.lblock[k] += inc
			field := st.lblock[k] + floorLog2(passes)
			if field > 63 {
				return nil, 0, fmt.Errorf("%w: code-block length field of %d bits", ErrInvalidData, field)
			}
			length, err := r.readBits(field)
			if err != nil {
				return nil, 0, err
			}
			if length > uint64(len(data)) {
				return nil, 0, fmt.Errorf("%w: code-block length %d exceeds the %d-byte buffer", ErrInvalidData, length, len(data))
			}
			out[bi][k] = &blockContribution{passes: passes, zeroBitplanes: zbp, length: int(length)}
		}
	}
	n, err := r.consumed()
	if err != nil {
		return nil, 0, err
	}
	return out, n, nil
}
