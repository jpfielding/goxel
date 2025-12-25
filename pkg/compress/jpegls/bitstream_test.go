package jpegls

import (
	"bytes"
	"testing"
)

func TestBitReader_ReadBits(t *testing.T) {
	data := []byte{0b10110010, 0b11000011}
	r := bytes.NewReader(data)
	br := NewBitReader(r)

	// Read 3 bits (101) = 5
	val, err := br.ReadBits(3)
	if err != nil {
		t.Fatalf("ReadBits failed: %v", err)
	}
	if val != 5 {
		t.Errorf("Expected 5, got %d", val)
	}

	// Read 5 bits (10010) = 18
	val, err = br.ReadBits(5)
	if err != nil {
		t.Fatalf("ReadBits failed: %v", err)
	}
	if val != 18 {
		t.Errorf("Expected 18, got %d", val)
	}

	// Read 4 bits (1100) = 12
	val, err = br.ReadBits(4)
	if err != nil {
		t.Fatalf("ReadBits failed: %v", err)
	}
	if val != 12 {
		t.Errorf("Expected 12, got %d", val)
	}
}

func TestReadGolomb(t *testing.T) {
	// Test Golomb decoding
	// k=0: Unary q (zeros followed by 1)
	// 0 -> 1 (q=0)
	// 1 -> 01 (q=1)
	// 2 -> 001 (q=2)

	// k=2: Unary q, 2 bits r
	// q=1, r=3 (11) -> 01 11

	data := []byte{
		0b10010001, // 1 (0), 001 (2), 0001?
		0b01110000,
	}
	r := bytes.NewReader(data)
	br := NewBitReader(r)

	// 1: k=0. Bits: 1. q=0. Val=0.
	val, err := br.ReadGolomb(0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 0 {
		t.Errorf("1: Expected 0, got %d", val)
	}

	// 001: k=0. Bits: 001. q=2. Val=2.
	val, err = br.ReadGolomb(0)
	if err != nil {
		t.Fatal(err)
	}
	if val != 2 {
		t.Errorf("2: Expected 2, got %d", val)
	}

	// Remaining bits in byte: 0001 (last 4 bits).
	// Let's assume next is 01 11 ...

	// Test k=2.
	// Next bits: 0001 01 11 ...
	// q: 0001 -> q=3.
	// r: 01 (1).
	// Val = 3<<2 | 1 = 12 + 1 = 13.
	val, err = br.ReadGolomb(2)
	if err != nil {
		t.Fatal(err)
	}
	if val != 13 {
		t.Errorf("3: Expected 13, got %d", val)
	}
}
