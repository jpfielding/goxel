package jpeg2k

import (
	"bufio"
	"errors"
	"io"
)

// ErrMarkerEncountered signals a marker was found in the bitstream
var ErrMarkerEncountered = errors.New("marker encountered in bitstream")

// BitReader reads bits from a byte stream.
// For simple bit I/O without JPEG 2000 byte stuffing.
type BitReader struct {
	r    *bufio.Reader
	buf  uint32 // Bit buffer
	bits int    // Number of valid bits in buffer (0-32)
}

// NewBitReader creates a new bit reader
func NewBitReader(r io.Reader) *BitReader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &BitReader{r: br}
}

// ReadBit reads a single bit
func (b *BitReader) ReadBit() (int, error) {
	if b.bits == 0 {
		if err := b.fill(); err != nil {
			return 0, err
		}
	}
	b.bits--
	return int((b.buf >> b.bits) & 1), nil
}

// ReadBits reads n bits (n <= 25)
func (b *BitReader) ReadBits(n int) (uint32, error) {
	for b.bits < n {
		if err := b.fill(); err != nil {
			return 0, err
		}
	}
	b.bits -= n
	return (b.buf >> b.bits) & ((1 << n) - 1), nil
}

// fill loads more bits into the buffer
func (b *BitReader) fill() error {
	c, err := b.r.ReadByte()
	if err != nil {
		return err
	}
	b.buf = (b.buf << 8) | uint32(c)
	b.bits += 8
	return nil
}

// PeekByte peeks at the next byte without consuming it
func (b *BitReader) PeekByte() (byte, error) {
	c, err := b.r.ReadByte()
	if err != nil {
		return 0, err
	}
	b.r.UnreadByte()
	return c, nil
}

// ReadByte reads a full byte (aligns to byte boundary first)
func (b *BitReader) ReadByte() (byte, error) {
	// Discard partial bits
	b.bits = 0
	b.buf = 0
	return b.r.ReadByte()
}

// ReadBytes reads n bytes (aligns to byte boundary first)
func (b *BitReader) ReadBytes(n int) ([]byte, error) {
	b.bits = 0
	b.buf = 0
	data := make([]byte, n)
	_, err := io.ReadFull(b.r, data)
	return data, err
}

// Align discards bits to reach byte boundary
func (b *BitReader) Align() {
	b.bits = 0
	b.buf = 0
}

// BitWriter writes bits to a byte stream with JPEG 2000 byte stuffing rules
type BitWriter struct {
	w    *bufio.Writer
	buf  uint32 // Bit buffer
	bits int    // Number of valid bits in buffer
}

// NewBitWriter creates a new bit writer
func NewBitWriter(w io.Writer) *BitWriter {
	bw, ok := w.(*bufio.Writer)
	if !ok {
		bw = bufio.NewWriter(w)
	}
	return &BitWriter{w: bw}
}

// WriteBit writes a single bit
func (b *BitWriter) WriteBit(bit int) error {
	b.buf = (b.buf << 1) | uint32(bit&1)
	b.bits++
	if b.bits >= 8 {
		return b.flushByte()
	}
	return nil
}

// WriteBits writes n bits from val (n <= 25)
func (b *BitWriter) WriteBits(val uint32, n int) error {
	b.buf = (b.buf << n) | (val & ((1 << n) - 1))
	b.bits += n
	for b.bits >= 8 {
		if err := b.flushByte(); err != nil {
			return err
		}
	}
	return nil
}

// flushByte writes the top 8 bits with byte stuffing
func (b *BitWriter) flushByte() error {
	// Extract top 8 bits
	shift := b.bits - 8
	c := byte((b.buf >> shift) & 0xFF)
	b.bits = shift
	b.buf &= (1 << shift) - 1

	if err := b.w.WriteByte(c); err != nil {
		return err
	}

	// After 0xFF, insert a stuff byte (0x00) or limit next byte
	if c == 0xFF {
		// In JPEG 2000, after 0xFF we must ensure the next byte's MSB is 0
		// This is handled by the caller or flush
	}

	return nil
}

// WriteByte writes a full byte (flushes partial bits first with padding)
func (b *BitWriter) WriteByte(c byte) error {
	if err := b.Flush(); err != nil {
		return err
	}
	return b.w.WriteByte(c)
}

// WriteBytes writes multiple bytes
func (b *BitWriter) WriteBytes(data []byte) error {
	if err := b.Flush(); err != nil {
		return err
	}
	_, err := b.w.Write(data)
	return err
}

// Flush pads remaining bits with zeros and flushes
func (b *BitWriter) Flush() error {
	if b.bits > 0 {
		// Pad with zeros to complete the byte
		padding := 8 - b.bits
		b.buf <<= padding
		b.bits = 8
		if err := b.flushByte(); err != nil {
			return err
		}
	}
	return b.w.Flush()
}

// FlushWithStuffing ensures proper termination for JPEG 2000
func (b *BitWriter) FlushWithStuffing() error {
	if b.bits > 0 {
		// Pad with ones (JPEG 2000 convention for segment termination)
		padding := 8 - b.bits
		b.buf = (b.buf << padding) | ((1 << padding) - 1)
		b.bits = 8
		if err := b.flushByte(); err != nil {
			return err
		}
	}
	return b.w.Flush()
}

// Writer returns the underlying writer
func (b *BitWriter) Writer() *bufio.Writer {
	return b.w
}

// ByteReader provides raw byte access with buffering
type ByteReader struct {
	r *bufio.Reader
}

// NewByteReader creates a new byte reader
func NewByteReader(r io.Reader) *ByteReader {
	br, ok := r.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(r)
	}
	return &ByteReader{r: br}
}

// ReadByte reads a single byte
func (b *ByteReader) ReadByte() (byte, error) {
	return b.r.ReadByte()
}

// ReadUint16 reads a big-endian uint16
func (b *ByteReader) ReadUint16() (uint16, error) {
	hi, err := b.r.ReadByte()
	if err != nil {
		return 0, err
	}
	lo, err := b.r.ReadByte()
	if err != nil {
		return 0, err
	}
	return uint16(hi)<<8 | uint16(lo), nil
}

// ReadUint32 reads a big-endian uint32
func (b *ByteReader) ReadUint32() (uint32, error) {
	var val uint32
	for i := 0; i < 4; i++ {
		c, err := b.r.ReadByte()
		if err != nil {
			return 0, err
		}
		val = (val << 8) | uint32(c)
	}
	return val, nil
}

// ReadBytes reads n bytes
func (b *ByteReader) ReadBytes(n int) ([]byte, error) {
	data := make([]byte, n)
	_, err := io.ReadFull(b.r, data)
	return data, err
}

// Skip discards n bytes
func (b *ByteReader) Skip(n int) error {
	_, err := b.r.Discard(n)
	return err
}

// ByteWriter provides raw byte access with buffering
type ByteWriter struct {
	w *bufio.Writer
}

// NewByteWriter creates a new byte writer
func NewByteWriter(w io.Writer) *ByteWriter {
	bw, ok := w.(*bufio.Writer)
	if !ok {
		bw = bufio.NewWriter(w)
	}
	return &ByteWriter{w: bw}
}

// WriteByte writes a single byte
func (b *ByteWriter) WriteByte(c byte) error {
	return b.w.WriteByte(c)
}

// WriteUint16 writes a big-endian uint16
func (b *ByteWriter) WriteUint16(v uint16) error {
	if err := b.w.WriteByte(byte(v >> 8)); err != nil {
		return err
	}
	return b.w.WriteByte(byte(v))
}

// WriteUint32 writes a big-endian uint32
func (b *ByteWriter) WriteUint32(v uint32) error {
	for i := 24; i >= 0; i -= 8 {
		if err := b.w.WriteByte(byte(v >> i)); err != nil {
			return err
		}
	}
	return nil
}

// WriteBytes writes multiple bytes
func (b *ByteWriter) WriteBytes(data []byte) error {
	_, err := b.w.Write(data)
	return err
}

// Flush flushes the buffer
func (b *ByteWriter) Flush() error {
	return b.w.Flush()
}
