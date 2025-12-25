package jpegls

import (
	"bufio"
	"fmt"
	"io"
)

// BitReader handles reading bits from an underlying io.Reader.
type BitReader struct {
	r     *bufio.Reader
	bits  uint64
	nBits int
}

// NewBitReader creates a new BitReader.
func NewBitReader(r io.Reader) *BitReader {
	var br *bufio.Reader
	if b, ok := r.(*bufio.Reader); ok {
		br = b
	} else {
		br = bufio.NewReader(r)
	}
	return &BitReader{r: br}
}

// fill ensures there are at least n bits in the buffer.
func (br *BitReader) fill(n int) error {
	for br.nBits < n {
		b, err := br.r.ReadByte()
		if err != nil {
			return err
		}

		if b == 0xFF {
			next, err := br.r.ReadByte()
			if err != nil {
				return err
			}
			if next == 0x00 {
				// Byte stuffing: 0xFF 0x00 -> 0xFF
			} else {
				// Marker found!
				// We unread 'next' so it remains in stream for marker parser.
				// 0xFF (b) is already consumed. The marker parser must account for this.
				// However, standard markers are FFxx. If we already ate FF,
				// the marker parser will see xx.
				// To be safe, we unread 'next' and return error.
				br.r.UnreadByte()
				return fmt.Errorf("marker encountered (0xFF%02X)", next)
			}
		}

		br.bits = (br.bits << 8) | uint64(b)
		br.nBits += 8
	}
	return nil
}

// ReadBits reads n bits (up to 32).
func (br *BitReader) ReadBits(n int) (uint32, error) {
	if n == 0 {
		return 0, nil
	}
	if br.nBits < n {
		if err := br.fill(n); err != nil {
			return 0, err
		}
	}

	shift := br.nBits - n
	mask := uint64(1)<<n - 1
	if n == 64 { // Should not happen for ReadBits(32) but for completeness
		mask = ^uint64(0)
	}
	val := (br.bits >> shift) & mask
	br.nBits -= n
	return uint32(val), nil
}

// ReadBit reads a single bit.
func (br *BitReader) ReadBit() (uint32, error) {
	return br.ReadBits(1)
}

// ReadGolomb reads a Golomb-Rice code with parameter k.
// Returns the decoded value (unsigned).
// Handles the "limit" if necessary (JPEG-LS has escape codes).
// For now, standard Golomb-Rice: Unary(q) zeros followed by 1, then k bits.
func (br *BitReader) ReadGolomb(k int) (uint32, error) {
	// Read unary q (zeros until 1)
	var q uint32
	for {
		b, err := br.ReadBit()
		if err != nil {
			return 0, err
		}
		if b == 1 {
			break
		}
		q++
		// Safety limit?
		if q > 65536 {
			// Abnormal for standard images
			return 0, fmt.Errorf("golomb q overflow")
		}
	}

	if k == 0 {
		return q, nil
	}

	r, err := br.ReadBits(k)
	if err != nil {
		return 0, err
	}

	return (q << k) | r, nil
}

// BitWriter handles writing bits to an underlying io.Writer.
type BitWriter struct {
	w     *bufio.Writer
	bits  uint64
	nBits int
}

// NewBitWriter creates a new BitWriter.
func NewBitWriter(w io.Writer) *BitWriter {
	var bw *bufio.Writer
	if b, ok := w.(*bufio.Writer); ok {
		bw = b
	} else {
		bw = bufio.NewWriter(w)
	}
	return &BitWriter{w: bw}
}

// WriteBits writes n bits (val) to the stream.
func (bw *BitWriter) WriteBits(val uint32, n int) error {
	bw.bits = (bw.bits << n) | (uint64(val) & ((1 << n) - 1))
	bw.nBits += n

	for bw.nBits >= 8 {
		shift := bw.nBits - 8
		b := byte(bw.bits >> shift)
		if err := bw.w.WriteByte(b); err != nil {
			return err
		}

		// Byte stuffing
		if b == 0xFF {
			// Write 0x00 to escape
			if err := bw.w.WriteByte(0x00); err != nil {
				return err
			}
		}

		bw.nBits -= 8
	}
	return nil
}

// WriteBit writes a single bit.
func (bw *BitWriter) WriteBit(bit uint32) error {
	return bw.WriteBits(bit, 1)
}

// Flush writes remaining bits (padded with 0 if needed) and flushes the writer.
func (bw *BitWriter) Flush() error {
	if bw.nBits > 0 {
		// Pad with zero by shifting left to byte boundary
		shift := 8 - bw.nBits
		b := byte(bw.bits << shift)
		if err := bw.w.WriteByte(b); err != nil {
			return err
		}
		if b == 0xFF {
			if err := bw.w.WriteByte(0x00); err != nil {
				return err
			}
		}
		bw.nBits = 0
	}
	return bw.w.Flush()
}

// WriteGolomb writes a Golomb-Rice code for val (non-negative).
// NOTE: JPEG-LS maps Signed Error -> Non-Negative Code using Mapping (A.5.3).
// This function writes the MAPPED value.
func (bw *BitWriter) WriteGolomb(k int, val uint32) error {
	// val = q * 2^k + r
	q := val >> k
	r := val & ((1 << k) - 1)

	// Write Unary q (q zeros followed by 1)
	for i := uint32(0); i < q; i++ {
		if err := bw.WriteBit(0); err != nil {
			return err
		}
	}
	if err := bw.WriteBit(1); err != nil {
		return err
	}

	// Write k bits of r
	if k > 0 {
		if err := bw.WriteBits(r, k); err != nil {
			return err
		}
	}
	return nil
}
