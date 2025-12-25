package jpeg2k

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBitWriter_WriteBits(t *testing.T) {
	tests := []struct {
		name     string
		writes   []struct{ val uint32; bits int }
		expected []byte
	}{
		{
			name: "single byte",
			writes: []struct{ val uint32; bits int }{
				{0xAB, 8},
			},
			expected: []byte{0xAB},
		},
		{
			name: "two nibbles",
			writes: []struct{ val uint32; bits int }{
				{0xA, 4},
				{0xB, 4},
			},
			expected: []byte{0xAB},
		},
		{
			name: "single bits",
			writes: []struct{ val uint32; bits int }{
				{1, 1}, {0, 1}, {1, 1}, {0, 1},
				{1, 1}, {0, 1}, {1, 1}, {0, 1},
			},
			expected: []byte{0xAA},
		},
		{
			name: "mixed sizes",
			writes: []struct{ val uint32; bits int }{
				{0x7, 3},  // 111
				{0x15, 5}, // 10101
			},
			expected: []byte{0xF5}, // 11110101
		},
		{
			name: "multi-byte",
			writes: []struct{ val uint32; bits int }{
				{0xABCD, 16},
			},
			expected: []byte{0xAB, 0xCD},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			bw := NewBitWriter(&buf)

			for _, w := range tt.writes {
				err := bw.WriteBits(w.val, w.bits)
				require.NoError(t, err)
			}

			err := bw.Flush()
			require.NoError(t, err)

			assert.Equal(t, tt.expected, buf.Bytes())
		})
	}
}

func TestBitReader_ReadBits(t *testing.T) {
	tests := []struct {
		name     string
		data     []byte
		reads    []int // bit counts to read
		expected []uint32
	}{
		{
			name:     "single byte",
			data:     []byte{0xAB},
			reads:    []int{8},
			expected: []uint32{0xAB},
		},
		{
			name:     "two nibbles",
			data:     []byte{0xAB},
			reads:    []int{4, 4},
			expected: []uint32{0xA, 0xB},
		},
		{
			name:     "single bits",
			data:     []byte{0xAA},
			reads:    []int{1, 1, 1, 1, 1, 1, 1, 1},
			expected: []uint32{1, 0, 1, 0, 1, 0, 1, 0},
		},
		{
			name:     "mixed sizes",
			data:     []byte{0xF5}, // 11110101
			reads:    []int{3, 5},
			expected: []uint32{0x7, 0x15},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			br := NewBitReader(bytes.NewReader(tt.data))

			for i, bits := range tt.reads {
				val, err := br.ReadBits(bits)
				require.NoError(t, err)
				assert.Equal(t, tt.expected[i], val, "read %d", i)
			}
		})
	}
}

func TestBitRoundTrip(t *testing.T) {
	// Write various bit patterns and read them back
	var buf bytes.Buffer
	bw := NewBitWriter(&buf)

	// Write a sequence of values
	values := []struct{ val uint32; bits int }{
		{0x7, 3},
		{0x1F, 5},
		{0xABCD, 16},
		{0x1, 1},
		{0x0, 1},
		{0x3FF, 10},
	}

	for _, v := range values {
		err := bw.WriteBits(v.val, v.bits)
		require.NoError(t, err)
	}
	require.NoError(t, bw.Flush())

	// Read back
	br := NewBitReader(bytes.NewReader(buf.Bytes()))

	for i, v := range values {
		got, err := br.ReadBits(v.bits)
		require.NoError(t, err)
		assert.Equal(t, v.val, got, "value %d", i)
	}
}

func TestByteReader_ReadUint16(t *testing.T) {
	data := []byte{0x12, 0x34, 0xAB, 0xCD}
	br := NewByteReader(bytes.NewReader(data))

	val, err := br.ReadUint16()
	require.NoError(t, err)
	assert.Equal(t, uint16(0x1234), val)

	val, err = br.ReadUint16()
	require.NoError(t, err)
	assert.Equal(t, uint16(0xABCD), val)
}

func TestByteReader_ReadUint32(t *testing.T) {
	data := []byte{0x12, 0x34, 0x56, 0x78}
	br := NewByteReader(bytes.NewReader(data))

	val, err := br.ReadUint32()
	require.NoError(t, err)
	assert.Equal(t, uint32(0x12345678), val)
}

func TestByteWriter_WriteUint16(t *testing.T) {
	var buf bytes.Buffer
	bw := NewByteWriter(&buf)

	require.NoError(t, bw.WriteUint16(0x1234))
	require.NoError(t, bw.WriteUint16(0xABCD))
	require.NoError(t, bw.Flush())

	assert.Equal(t, []byte{0x12, 0x34, 0xAB, 0xCD}, buf.Bytes())
}

func TestByteWriter_WriteUint32(t *testing.T) {
	var buf bytes.Buffer
	bw := NewByteWriter(&buf)

	require.NoError(t, bw.WriteUint32(0x12345678))
	require.NoError(t, bw.Flush())

	assert.Equal(t, []byte{0x12, 0x34, 0x56, 0x78}, buf.Bytes())
}
