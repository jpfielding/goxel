package jpegli

import (
	"bytes"
	"testing"
)

func TestBitReader_ReadBits_Truncated(t *testing.T) {
	// 1 byte of data: 0xAA (1010 1010) - no stuffing needed
	data := []byte{0xAA}
	r := bytes.NewReader(data)
	br := newBitReader(r)

	// Read 4 bits -> 1010 = 10. OK.
	val, err := br.readBits(4)
	if err != nil {
		t.Fatalf("first read failed: %v", err)
	}
	if val != 10 {
		t.Errorf("expected 10, got %d", val)
	}

	// We have 4 bits left (1010).
	// Try to read 8 bits. We need 4 more. End of stream.
	// New behavior: returns padded value (1010 0000 = 0xA0), nil error.
	val, err = br.readBits(8)

	if err != nil {
		t.Errorf("Expected success (padding), got error: %v", err)
	}
	if val != 0xA0 {
		t.Errorf("Expected 0xA0 (padded), got %X", val)
	}
}

func TestBitReader_PeekBits_Truncated(t *testing.T) {
	// peekBits DOES pad with zeros currently
	data := []byte{0xAA} // 1010 1010
	r := bytes.NewReader(data)
	br := newBitReader(r)

	// Consume 4 bits
	br.readBits(4)

	// 4 bits left: 1010
	// Peek 8 bits.
	val, err := br.peekBits(8)
	if err != nil {
		t.Fatalf("peekBits failed: %v", err)
	}
	// It pads with zeros at the end?
	// The implementation:
	// val := int(b.buf) << (n - b.bits) (shift left by 4)
	// Remaining buf: 1010 (binary) = 10
	// 1010 << 4 = 1010 0000 = 0xA0 = 160

	if val != 0xA0 {
		t.Errorf("Expected 0xA0 (padded), got %X", val)
	}
}
