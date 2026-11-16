package jpegls

import (
	"errors"
	"fmt"
	"math/bits"
)

// errEndOfData is returned by BitReader when the input is exhausted mid-scan.
var errEndOfData = errors.New("unexpected end of JPEG-LS data")

// errBadCode is returned for an entropy code no conformant encoder emits.
var errBadCode = errors.New("invalid entropy code")

// markerError reports a marker (0xFFxx, xx >= 0x80) met inside the
// entropy-coded scan. The value is the full 16-bit marker code.
type markerError int

func (m markerError) Error() string {
	return fmt.Sprintf("marker 0x%04X encountered in entropy-coded scan", int(m))
}

// BitReader reads bits from a byte slice, handling T.87 single-bit stuffing
// (T.87 A.1): the byte following a 0xFF data byte carries only 7 data bits.
// A byte >= 0x80 following 0xFF is a marker and ends the scan.
type BitReader struct {
	data []byte
	pos  int
	// bits is the MSB-first accumulator; the low nBits bits are valid.
	bits  uint64
	nBits int
	// lastFF records that the most recently consumed byte was 0xFF.
	lastFF bool
	// marker is the marker that terminated the scan, or 0.
	marker int
}

// NewBitReader creates a BitReader over data.
func NewBitReader(data []byte) *BitReader {
	return &BitReader{data: data}
}

// Pos is the byte offset of the first unconsumed input byte.
func (br *BitReader) Pos() int { return br.pos }

// MarkerPos is the byte offset of the marker terminating the scan. If the
// final consumed data byte was 0xFF, its mandatory stuff byte has not been
// pulled into the accumulator yet and the marker follows it.
func (br *BitReader) MarkerPos() int {
	if br.lastFF {
		return br.pos + 1
	}
	return br.pos
}

// fill tops the accumulator up to at least n bits.
func (br *BitReader) fill(n int) error {
	for br.nBits < n {
		if br.marker != 0 {
			return markerError(br.marker)
		}
		if br.pos >= len(br.data) {
			return errEndOfData
		}
		b := br.data[br.pos]
		if br.lastFF {
			// Stuffed byte: its MSB is the stuffed zero, 7 data bits follow.
			br.pos++
			br.bits = br.bits<<7 | uint64(b&0x7F)
			br.nBits += 7
			br.lastFF = false
			continue
		}
		if b == 0xFF && br.pos+1 < len(br.data) && br.data[br.pos+1] >= 0x80 {
			br.marker = 0xFF00 | int(br.data[br.pos+1])
			return markerError(br.marker)
		}
		br.pos++
		br.bits = br.bits<<8 | uint64(b)
		br.nBits += 8
		br.lastFF = b == 0xFF
	}
	return nil
}

// ReadBits reads n bits (0..32), returned right-justified.
func (br *BitReader) ReadBits(n int) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if err := br.fill(n); err != nil {
		return 0, err
	}
	br.nBits -= n
	return uint32(br.bits>>br.nBits) & uint32(uint64(1)<<n-1), nil
}

// ReadBit reads a single bit.
func (br *BitReader) ReadBit() (uint32, error) {
	return br.ReadBits(1)
}

// readUnary counts the 0 bits before the terminating 1; a run longer than
// limit is rejected.
func (br *BitReader) readUnary(limit int) (int, error) {
	count := 0
	for {
		if br.nBits == 0 {
			if err := br.fill(1); err != nil {
				return 0, err
			}
		}
		// Left-justify the valid window so its next bit sits at bit 63.
		window := br.bits << (64 - br.nBits)
		zeros := min(bits.LeadingZeros64(window), br.nBits)
		if zeros < br.nBits {
			br.nBits -= zeros + 1
			count += zeros
			if count > limit {
				return 0, errBadCode
			}
			return count, nil
		}
		count += br.nBits
		br.nBits = 0
		if count > limit {
			return 0, errBadCode
		}
	}
}

// ReadLimitedGolomb reads a length-limited Golomb code (T.87 A.5.3).
func (br *BitReader) ReadLimitedGolomb(k, glimit, qbpp int) (uint32, error) {
	escape := glimit - qbpp - 1
	count, err := br.readUnary(escape)
	if err != nil {
		return 0, err
	}
	if count < escape {
		r, err := br.ReadBits(k)
		if err != nil {
			return 0, err
		}
		return uint32(count)<<k | r, nil
	}
	// Escape: the code carries (value - 1) in qbpp bits.
	v, err := br.ReadBits(qbpp)
	if err != nil {
		return 0, err
	}
	return v + 1, nil
}

// BitWriter accumulates bits into an in-memory buffer, applying T.87
// single-bit stuffing: after an emitted 0xFF the next byte carries 7 bits.
type BitWriter struct {
	buf    []byte
	bits   uint64
	nBits  int
	lastFF bool
}

// NewBitWriter creates a BitWriter appending to buf (which may be nil).
func NewBitWriter(buf []byte) *BitWriter {
	return &BitWriter{buf: buf}
}

func (bw *BitWriter) drain() {
	for {
		width := 8
		if bw.lastFF {
			width = 7
		}
		if bw.nBits < width {
			return
		}
		bw.nBits -= width
		b := byte(bw.bits>>bw.nBits) & (0xFF >> (8 - width))
		bw.buf = append(bw.buf, b)
		bw.lastFF = b == 0xFF
	}
}

// WriteBits writes the low n bits (0..32) of val, MSB first.
func (bw *BitWriter) WriteBits(val uint32, n int) {
	if n == 0 {
		return
	}
	bw.bits = bw.bits<<n | uint64(val)&(uint64(1)<<n-1)
	bw.nBits += n
	bw.drain()
}

// WriteBit writes a single bit.
func (bw *BitWriter) WriteBit(bit uint32) {
	bw.WriteBits(bit, 1)
}

// writeUnary writes q zero bits followed by a one.
func (bw *BitWriter) writeUnary(q int) {
	for ; q >= 32; q -= 32 {
		bw.WriteBits(0, 32)
	}
	bw.WriteBits(1, q+1)
}

// WriteLimitedGolomb writes a length-limited Golomb code (T.87 A.5.3).
func (bw *BitWriter) WriteLimitedGolomb(k int, val uint32, glimit, qbpp int) {
	q := int(val >> k)
	escape := glimit - qbpp - 1
	if q < escape {
		bw.writeUnary(q)
		bw.WriteBits(val, k)
		return
	}
	bw.writeUnary(escape)
	bw.WriteBits(val-1, qbpp)
}

// Flush zero-pads to a byte boundary. A final 0xFF data byte still gets its
// mandatory stuff byte so the following marker is not misread.
func (bw *BitWriter) Flush() {
	if bw.nBits > 0 {
		width := 8
		if bw.lastFF {
			width = 7
		}
		b := byte(bw.bits<<(width-bw.nBits)) & (0xFF >> (8 - width))
		bw.buf = append(bw.buf, b)
		bw.bits, bw.nBits = 0, 0
		bw.lastFF = b == 0xFF
	}
	if bw.lastFF {
		bw.buf = append(bw.buf, 0x00)
		bw.lastFF = false
	}
}

// Bytes returns the buffer written so far (call Flush first).
func (bw *BitWriter) Bytes() []byte { return bw.buf }
