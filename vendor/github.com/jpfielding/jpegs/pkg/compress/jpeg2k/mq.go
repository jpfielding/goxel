package jpeg2k

// MQ Arithmetic Coder implementation for JPEG 2000
// Based on ITU-T T.800 Annex C (Arithmetic entropy coding procedure)
//
// This is a simplified implementation suitable for basic JPEG 2000 encoding/decoding.

// MQState represents the state of a context in the MQ coder
type MQState struct {
	Index int // Index into probability estimation table
	MPS   int // Most probable symbol (0 or 1)
}

// Probability estimation state table (ITU-T T.800 Table C.2)
type mqEntry struct {
	qe   uint16 // Probability estimate (Qe)
	nmps int    // Next state if MPS
	nlps int    // Next state if LPS
	swi  int    // Switch MPS and LPS on LPS occurrence
}

var mqTable = []mqEntry{
	{0x5601, 1, 1, 1},
	{0x3401, 2, 6, 0},
	{0x1801, 3, 9, 0},
	{0x0AC1, 4, 12, 0},
	{0x0521, 5, 29, 0},
	{0x0221, 38, 33, 0},
	{0x5601, 7, 6, 1},
	{0x5401, 8, 14, 0},
	{0x4801, 9, 14, 0},
	{0x3801, 10, 14, 0},
	{0x3001, 11, 17, 0},
	{0x2401, 12, 18, 0},
	{0x1C01, 13, 20, 0},
	{0x1601, 29, 21, 0},
	{0x5601, 15, 14, 1},
	{0x5401, 16, 14, 0},
	{0x5101, 17, 15, 0},
	{0x4801, 18, 16, 0},
	{0x3801, 19, 17, 0},
	{0x3401, 20, 18, 0},
	{0x3001, 21, 19, 0},
	{0x2801, 22, 19, 0},
	{0x2401, 23, 20, 0},
	{0x2201, 24, 21, 0},
	{0x1C01, 25, 22, 0},
	{0x1801, 26, 23, 0},
	{0x1601, 27, 24, 0},
	{0x1401, 28, 25, 0},
	{0x1201, 29, 26, 0},
	{0x1101, 30, 27, 0},
	{0x0AC1, 31, 28, 0},
	{0x09C1, 32, 29, 0},
	{0x08A1, 33, 30, 0},
	{0x0521, 34, 31, 0},
	{0x0441, 35, 32, 0},
	{0x02A1, 36, 33, 0},
	{0x0221, 37, 34, 0},
	{0x0141, 38, 35, 0},
	{0x0111, 39, 36, 0},
	{0x0085, 40, 37, 0},
	{0x0049, 41, 38, 0},
	{0x0025, 42, 39, 0},
	{0x0015, 43, 40, 0},
	{0x0009, 44, 41, 0},
	{0x0005, 45, 42, 0},
	{0x0001, 45, 43, 0},
	{0x5601, 46, 46, 0},
}

// MQEncoder implements the MQ arithmetic encoder
type MQEncoder struct {
	output []byte
	A      uint32 // Interval size
	C      uint32 // Lower bound
	t      int    // Bit counter
	L      int    // Output length
	T      byte   // Temporary byte
	Lmax   int    // Max length
}

// NewMQEncoder creates a new MQ encoder
func NewMQEncoder() *MQEncoder {
	e := &MQEncoder{
		output: make([]byte, 0, 4096),
		A:      0x8000,
		C:      0,
		t:      12,
		L:      -1,
		Lmax:   1 << 30,
	}
	return e
}

// Encode encodes a bit with context
func (e *MQEncoder) Encode(bit int, ctx *MQState) {
	entry := &mqTable[ctx.Index]
	qe := uint32(entry.qe)
	e.A -= qe

	if bit == ctx.MPS {
		if e.A < 0x8000 {
			if e.A < qe {
				e.C += e.A
				e.A = qe
			}
			ctx.Index = entry.nmps
			e.renorm()
		}
	} else {
		if e.A >= qe {
			e.C += e.A
			e.A = qe
		}
		if entry.swi != 0 {
			ctx.MPS = 1 - ctx.MPS
		}
		ctx.Index = entry.nlps
		e.renorm()
	}
}

func (e *MQEncoder) renorm() {
	for e.A < 0x8000 {
		e.A <<= 1
		e.C <<= 1
		e.t--
		if e.t == 0 {
			e.putByte()
		}
	}
}

func (e *MQEncoder) putByte() {
	if e.L >= 0 {
		if e.C < 0x8000000 { // No carry
			e.emit(e.T)
		} else { // Carry
			e.T++
			if e.T == 0 { // Wrapped from 0xFF
				e.emit(0xFF)
				e.emit(0x00)
				e.T = 0
			} else {
				e.emit(e.T - 1)
			}
			e.C &= 0x7FFFFFF
		}
	}
	e.T = byte(e.C >> 19)
	e.C &= 0x7FFFF
	if e.T == 0xFF {
		e.t = 7
	} else {
		e.t = 8
	}
	e.L++
}

func (e *MQEncoder) emit(b byte) {
	if len(e.output) < e.Lmax {
		e.output = append(e.output, b)
	}
}

// Flush finishes encoding
func (e *MQEncoder) Flush() {
	e.setbits()
	e.C <<= e.t
	e.putByte()
	e.C <<= e.t
	e.putByte()
	e.emit(e.T)
	// Emit stuffing if needed
	if e.T == 0xFF {
		e.emit(0x00)
	}
}

func (e *MQEncoder) setbits() {
	temp := e.C + e.A - 1
	temp &= 0xFFFF0000
	if temp < e.C {
		temp += 0x8000
	}
	e.C = temp
}

// Bytes returns encoded data
func (e *MQEncoder) Bytes() []byte {
	return e.output
}

// Reset resets encoder state
func (e *MQEncoder) Reset() {
	e.output = e.output[:0]
	e.A = 0x8000
	e.C = 0
	e.t = 12
	e.L = -1
	e.T = 0
}

// MQDecoder implements the MQ arithmetic decoder
type MQDecoder struct {
	data []byte
	pos  int
	A    uint32
	C    uint32
	t    int
	b    byte
}

// NewMQDecoder creates a new MQ decoder
func NewMQDecoder(data []byte) *MQDecoder {
	d := &MQDecoder{
		data: data,
		pos:  0,
		A:    0x8000,
	}
	d.init()
	return d
}

func (d *MQDecoder) init() {
	d.b = d.nextByte()
	d.C = uint32(d.b) << 16
	d.getByte()
	d.C <<= 7
	d.t -= 7
	d.A = 0x8000
}

func (d *MQDecoder) nextByte() byte {
	if d.pos >= len(d.data) {
		return 0xFF
	}
	b := d.data[d.pos]
	d.pos++
	return b
}

func (d *MQDecoder) getByte() {
	if d.b == 0xFF {
		b := d.nextByte()
		if b > 0x8F {
			d.pos--
			d.t = 8
		} else {
			d.b = b
			d.C += uint32(d.b) << 9
			d.t = 7
		}
	} else {
		d.b = d.nextByte()
		d.C += uint32(d.b) << 8
		d.t = 8
	}
}

// Decode decodes a bit with context
func (d *MQDecoder) Decode(ctx *MQState) int {
	entry := &mqTable[ctx.Index]
	qe := uint32(entry.qe)
	d.A -= qe

	chigh := d.C >> 16
	if chigh < d.A {
		if d.A < 0x8000 {
			return d.mpsExchange(ctx, entry, qe)
		}
		return ctx.MPS
	}
	return d.lpsExchange(ctx, entry, qe)
}

func (d *MQDecoder) mpsExchange(ctx *MQState, entry *mqEntry, qe uint32) int {
	var bit int
	if d.A < qe {
		bit = 1 - ctx.MPS
		if entry.swi != 0 {
			ctx.MPS = 1 - ctx.MPS
		}
		ctx.Index = entry.nlps
	} else {
		bit = ctx.MPS
		ctx.Index = entry.nmps
	}
	d.renorm()
	return bit
}

func (d *MQDecoder) lpsExchange(ctx *MQState, entry *mqEntry, qe uint32) int {
	d.C -= d.A << 16
	var bit int
	if d.A < qe {
		bit = ctx.MPS
		d.A = qe
		ctx.Index = entry.nmps
	} else {
		bit = 1 - ctx.MPS
		d.A = qe
		if entry.swi != 0 {
			ctx.MPS = 1 - ctx.MPS
		}
		ctx.Index = entry.nlps
	}
	d.renorm()
	return bit
}

func (d *MQDecoder) renorm() {
	for d.A < 0x8000 {
		if d.t == 0 {
			d.getByte()
		}
		d.A <<= 1
		d.C <<= 1
		d.t--
	}
}

// ResetContexts creates initialized contexts
func ResetContexts(n int) []MQState {
	contexts := make([]MQState, n)
	return contexts
}

// InitContext initializes a context
func InitContext(ctx *MQState, index int, mps int) {
	ctx.Index = index
	ctx.MPS = mps
}

// Context constants for EBCOT
const (
	NumMQContexts  = 19
	CtxUniform     = 18
	CtxRunLength   = 17
	CtxMagRefFirst = 14
	CtxMagRef      = 15
	CtxMagRefNull  = 16
	CtxSignStart   = 9
)

// SetupDefaultContexts returns EBCOT-initialized contexts
func SetupDefaultContexts() []MQState {
	contexts := make([]MQState, NumMQContexts)
	contexts[CtxUniform] = MQState{Index: 46, MPS: 0}
	return contexts
}
