package rle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPackBitsRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		data []byte
	}{
		{"Empty", nil},
		{"Single", []byte{0xAA}},
		{"Run2", []byte{0xAA, 0xAA}},
		{"Run3", []byte{0xAA, 0xAA, 0xAA}},
		{"Literal", []byte{0x01, 0x02, 0x03}},
		{"Mixed", []byte{0xAA, 0xAA, 0xAA, 0x01, 0x02, 0xBB, 0xBB}},
		{"LongRun", makeBytes(0xCC, 130)},     // > 128
		{"LongLiteral", makeSequence(0, 130)}, // > 128
		{"MaxRun", makeBytes(0xAA, 128)},
		{"MaxRunPlus1", makeBytes(0xAA, 129)},
		{"MaxLiteral", makeSequence(0, 128)},
		{"MaxLiteralPlus1", makeSequence(0, 129)},
		{"Alternating", []byte{0x00, 0x01, 0x00, 0x01, 0x00, 0x01}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			compressed := encodePackBits(tt.data)
			decompressed, err := decodePackBits(compressed, 0)
			require.NoError(t, err)
			assert.Equal(t, tt.data, decompressed, "Roundtrip mismatch")
		})
	}
}

func TestDecodePackBits_Truncated(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		errString string
	}{
		{
			name:      "TruncatedLiteral",
			input:     []byte{0x02, 0x01}, // Literal run of 3 (n=2), but only 1 byte provided
			errString: "rle: compressed data truncated in literal run",
		},
		{
			name:      "TruncatedReplicate",
			input:     []byte{0xFE},                                      // Replicate run (n=-2 -> count=3), but value byte missing
			errString: "rle: compressed data truncated in replicate run", // My implementation checks 'val' read
		},
		{
			name:      "TruncatedLiteralBoundary",
			input:     []byte{0x00}, // Literal run of 1 (n=0), but 0 bytes provided
			errString: "rle: compressed data truncated in literal run",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := decodePackBits(tt.input, 0)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.errString)
		})
	}
}

func makeBytes(val byte, n int) []byte {
	res := make([]byte, n)
	for i := range res {
		res[i] = val
	}
	return res
}

func makeSequence(start byte, n int) []byte {
	res := make([]byte, n)
	val := start
	for i := range res {
		res[i] = val
		val++
	}
	return res
}
