package jpeg2k

// MQ binary adaptive arithmetic coder (ITU-T T.800 Annex C), used by the
// EBCOT tier-1 coder. Ported from dicos.rs crates/jpeg2k/src/mq.rs.
//
// Both directions use the normative interval convention (identical to
// OpenJPEG's opj_mqc_*): the LPS sub-interval sits at the base of [C, C+A),
// so coding an MPS advances C by Qe. The codewords are bit-for-bit
// interoperable with any conformant coder.

// mqEntry is one row of the probability estimation table (T.800 Table C.2).
type mqEntry struct {
	qe   uint32
	nmps uint8
	nlps uint8
	swi  bool
}

var mqTable = [47]mqEntry{
	{0x5601, 1, 1, true},
	{0x3401, 2, 6, false},
	{0x1801, 3, 9, false},
	{0x0AC1, 4, 12, false},
	{0x0521, 5, 29, false},
	{0x0221, 38, 33, false},
	{0x5601, 7, 6, true},
	{0x5401, 8, 14, false},
	{0x4801, 9, 14, false},
	{0x3801, 10, 14, false},
	{0x3001, 11, 17, false},
	{0x2401, 12, 18, false},
	{0x1C01, 13, 20, false},
	{0x1601, 29, 21, false},
	{0x5601, 15, 14, true},
	{0x5401, 16, 14, false},
	{0x5101, 17, 15, false},
	{0x4801, 18, 16, false},
	{0x3801, 19, 17, false},
	{0x3401, 20, 18, false},
	{0x3001, 21, 19, false},
	{0x2801, 22, 19, false},
	{0x2401, 23, 20, false},
	{0x2201, 24, 21, false},
	{0x1C01, 25, 22, false},
	{0x1801, 26, 23, false},
	{0x1601, 27, 24, false},
	{0x1401, 28, 25, false},
	{0x1201, 29, 26, false},
	{0x1101, 30, 27, false},
	{0x0AC1, 31, 28, false},
	{0x09C1, 32, 29, false},
	{0x08A1, 33, 30, false},
	{0x0521, 34, 31, false},
	{0x0441, 35, 32, false},
	{0x02A1, 36, 33, false},
	{0x0221, 37, 34, false},
	{0x0141, 38, 35, false},
	{0x0111, 39, 36, false},
	{0x0085, 40, 37, false},
	{0x0049, 41, 38, false},
	{0x0025, 42, 39, false},
	{0x0015, 43, 40, false},
	{0x0009, 44, 41, false},
	{0x0005, 45, 42, false},
	{0x0001, 45, 43, false},
	{0x5601, 46, 46, false},
}

// mqState is the adaptive state of one context: a table index and the MPS.
type mqState struct {
	index uint8
	mps   uint8
}

// EBCOT tier-1 context labels.
const (
	numMQContexts = 19
	ctxZCStart    = 0  // zero coding, 0..8
	ctxSCStart    = 9  // sign coding, 9..13
	ctxMRStart    = 14 // magnitude refinement, 14..16
	ctxRunLength  = 17
	ctxUniform    = 18
)

// defaultContexts returns the initial EBCOT context states (T.800 Annex D):
// state 0 / MPS 0 everywhere except the all-insignificant zero-coding
// context (state 4), the run-length context (state 3) and the uniform
// context (state 46).
func defaultContexts() [numMQContexts]mqState {
	var c [numMQContexts]mqState
	c[ctxZCStart] = mqState{index: 4}
	c[ctxRunLength] = mqState{index: 3}
	c[ctxUniform] = mqState{index: 46}
	return c
}

// mqEncoder is the MQ arithmetic encoder.
type mqEncoder struct {
	out  []byte
	a    uint32 // interval size
	c    uint32 // lower bound
	t    int    // bit counter
	l    int    // output length counter
	temp byte   // pending output byte
}

func newMQEncoder() *mqEncoder {
	return &mqEncoder{a: 0x8000, t: 12, l: -1}
}

// encode codes bit in ctx.
func (e *mqEncoder) encode(bit uint8, ctx *mqState) {
	entry := &mqTable[ctx.index]
	qe := entry.qe
	e.a -= qe
	if bit == ctx.mps {
		if e.a&0x8000 != 0 {
			e.c += qe
			return
		}
		if e.a < qe {
			e.a = qe
		} else {
			e.c += qe
		}
		ctx.index = entry.nmps
		e.renorm()
		return
	}
	if e.a < qe {
		e.c += qe
	} else {
		e.a = qe
	}
	if entry.swi {
		ctx.mps = 1 - ctx.mps
	}
	ctx.index = entry.nlps
	e.renorm()
}

func (e *mqEncoder) renorm() {
	for e.a < 0x8000 {
		e.a <<= 1
		e.c <<= 1
		e.t--
		if e.t == 0 {
			e.putByte()
		}
	}
}

// putByte is BYTEOUT (T.800 C.2.6) with carry propagation into temp.
func (e *mqEncoder) putByte() {
	emitTemp := func() {
		if e.l >= 0 {
			e.out = append(e.out, e.temp)
		}
	}
	switch {
	case e.temp == 0xFF:
		emitTemp()
		e.temp = byte(e.c >> 20)
		e.c &= 0xFFFFF
		e.t = 7
	case e.c&0x8000000 == 0:
		emitTemp()
		e.temp = byte(e.c >> 19)
		e.c &= 0x7FFFF
		e.t = 8
	default:
		e.temp++
		e.c &= 0x7FFFFFF
		emitTemp()
		if e.temp == 0xFF {
			e.temp = byte(e.c >> 20)
			e.c &= 0xFFFFF
			e.t = 7
		} else {
			e.temp = byte(e.c >> 19)
			e.c &= 0x7FFFF
			e.t = 8
		}
	}
	e.l++
}

func (e *mqEncoder) setBits() {
	tempc := e.c + e.a
	e.c |= 0xFFFF
	if e.c >= tempc {
		e.c -= 0x8000
	}
}

// flush terminates the codeword (T.800 C.2.9). A final 0xFF is not emitted.
func (e *mqEncoder) flush() {
	e.setBits()
	e.c <<= uint(e.t)
	e.putByte()
	e.c <<= uint(e.t)
	e.putByte()
	if e.temp != 0xFF {
		e.out = append(e.out, e.temp)
	}
}

// bytes returns the codeword written so far.
func (e *mqEncoder) bytes() []byte { return e.out }

// mqDecoder is the MQ arithmetic decoder. Past the end of data it feeds
// synthetic 1-bits (as at a marker) and never reads out of bounds.
type mqDecoder struct {
	data []byte
	pos  int
	a    uint32
	c    uint32
	t    int
	b    byte
}

func newMQDecoder(data []byte) *mqDecoder {
	d := &mqDecoder{data: data}
	d.b = d.nextByte()
	d.c = uint32(d.b) << 16
	d.getByte()
	d.c <<= 7
	d.t -= 7
	d.a = 0x8000
	return d
}

// nextByte consumes the next byte, or returns 0xFF at end of data without
// advancing.
func (d *mqDecoder) nextByte() byte {
	if d.pos >= len(d.data) {
		return 0xFF
	}
	b := d.data[d.pos]
	d.pos++
	return b
}

func (d *mqDecoder) peekByte() byte {
	if d.pos >= len(d.data) {
		return 0xFF
	}
	return d.data[d.pos]
}

// getByte is BYTEIN (T.800 C.3.4, Figure C.19).
func (d *mqDecoder) getByte() {
	if d.b == 0xFF {
		b1 := d.peekByte()
		if b1 > 0x8F {
			// Marker (or end of data): stay put and feed 1-bits.
			d.c += 0xFF00
			d.t = 8
			return
		}
		d.pos++
		d.b = b1
		d.c += uint32(b1) << 9
		d.t = 7
		return
	}
	b1 := d.peekByte()
	if d.pos < len(d.data) {
		d.pos++
	}
	d.b = b1
	d.c += uint32(b1) << 8
	d.t = 8
}

// decode returns the next bit coded in ctx.
func (d *mqDecoder) decode(ctx *mqState) uint8 {
	entry := &mqTable[ctx.index]
	qe := entry.qe
	d.a -= qe
	if d.c>>16 < qe {
		var bit uint8
		if d.a < qe {
			bit = ctx.mps
			ctx.index = entry.nmps
		} else {
			bit = 1 - ctx.mps
			if entry.swi {
				ctx.mps = 1 - ctx.mps
			}
			ctx.index = entry.nlps
		}
		d.a = qe
		d.renorm()
		return bit
	}
	d.c -= qe << 16
	if d.a&0x8000 != 0 {
		return ctx.mps
	}
	var bit uint8
	if d.a < qe {
		bit = 1 - ctx.mps
		if entry.swi {
			ctx.mps = 1 - ctx.mps
		}
		ctx.index = entry.nlps
	} else {
		bit = ctx.mps
		ctx.index = entry.nmps
	}
	d.renorm()
	return bit
}

func (d *mqDecoder) renorm() {
	for d.a < 0x8000 {
		if d.t == 0 {
			d.getByte()
		}
		d.a <<= 1
		d.c <<= 1
		d.t--
	}
}
