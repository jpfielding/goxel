package jpeg2k

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForward1D_Inverse1D_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		signal []int
	}{
		{
			name:   "simple 4 elements",
			signal: []int{1, 2, 3, 4},
		},
		{
			name:   "simple 8 elements",
			signal: []int{1, 2, 3, 4, 5, 6, 7, 8},
		},
		{
			name:   "odd length",
			signal: []int{1, 2, 3, 4, 5},
		},
		{
			name:   "constant signal",
			signal: []int{100, 100, 100, 100},
		},
		{
			name:   "alternating",
			signal: []int{0, 255, 0, 255, 0, 255, 0, 255},
		},
		{
			name:   "ramp",
			signal: []int{0, 16, 32, 48, 64, 80, 96, 112},
		},
		{
			name:   "two elements",
			signal: []int{100, 200},
		},
		{
			name:   "three elements",
			signal: []int{10, 20, 30},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := make([]int, len(tt.signal))
			copy(original, tt.signal)

			// Forward transform
			Forward1D(tt.signal)

			// Inverse transform
			Inverse1D(tt.signal)

			// Should match original
			assert.Equal(t, original, tt.signal)
		})
	}
}

func TestForward1D_KnownValues(t *testing.T) {
	// Test that constant signal produces zero high-pass coefficients
	signal := []int{100, 100, 100, 100}
	Forward1D(signal)

	// First two are low-pass (should be ~100)
	// Last two are high-pass (should be ~0 for constant)
	assert.Equal(t, 0, signal[2], "high-pass coeff should be 0")
	assert.Equal(t, 0, signal[3], "high-pass coeff should be 0")
}

func TestForward2D_Inverse2D_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
	}{
		{"4x4", 4, 4},
		{"8x8", 8, 8},
		{"4x8", 4, 8},
		{"8x4", 8, 4},
		{"5x5 odd", 5, 5},
		{"7x3 odd", 7, 3},
		{"16x16", 16, 16},
		{"2x2 minimum", 2, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test image with gradient
			data := make([]int, tt.width*tt.height)
			for y := 0; y < tt.height; y++ {
				for x := 0; x < tt.width; x++ {
					data[y*tt.width+x] = x + y*tt.width
				}
			}

			original := make([]int, len(data))
			copy(original, data)

			Forward2D(data, tt.width, tt.height)
			Inverse2D(data, tt.width, tt.height)

			assert.Equal(t, original, data)
		})
	}
}

func TestForward2D_SubbandStructure(t *testing.T) {
	// 8x8 constant image should have zero in all high-pass subbands
	width, height := 8, 8
	data := make([]int, width*height)
	for i := range data {
		data[i] = 100
	}

	Forward2D(data, width, height)

	// LL is top-left 4x4
	// HL is top-right 4x4
	// LH is bottom-left 4x4
	// HH is bottom-right 4x4

	// HL should be all zeros (high-pass horizontal on constant)
	for y := 0; y < 4; y++ {
		for x := 4; x < 8; x++ {
			assert.Equal(t, 0, data[y*width+x], "HL[%d,%d]", x, y)
		}
	}

	// LH should be all zeros
	for y := 4; y < 8; y++ {
		for x := 0; x < 4; x++ {
			assert.Equal(t, 0, data[y*width+x], "LH[%d,%d]", x, y)
		}
	}

	// HH should be all zeros
	for y := 4; y < 8; y++ {
		for x := 4; x < 8; x++ {
			assert.Equal(t, 0, data[y*width+x], "HH[%d,%d]", x, y)
		}
	}
}

func TestForwardMultiLevel_InverseMultiLevel_RoundTrip(t *testing.T) {
	tests := []struct {
		name   string
		width  int
		height int
		levels int
	}{
		{"16x16 2 levels", 16, 16, 2},
		{"16x16 3 levels", 16, 16, 3},
		{"32x32 4 levels", 32, 32, 4},
		{"64x64 5 levels", 64, 64, 5},
		{"17x17 odd 3 levels", 17, 17, 3},
		{"20x30 rect 2 levels", 20, 30, 2},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data := make([]int, tt.width*tt.height)
			for i := range data {
				data[i] = i % 256
			}

			original := make([]int, len(data))
			copy(original, data)

			ForwardMultiLevel(data, tt.width, tt.height, tt.levels)
			InverseMultiLevel(data, tt.width, tt.height, tt.levels)

			assert.Equal(t, original, data)
		})
	}
}

func TestForwardMultiLevel_LLDimensions(t *testing.T) {
	tests := []struct {
		width, height, levels int
		wantLLW, wantLLH      int
	}{
		{16, 16, 1, 8, 8},
		{16, 16, 2, 4, 4},
		{16, 16, 3, 2, 2},
		{16, 16, 4, 1, 1},
		{17, 17, 1, 9, 9},
		{17, 17, 2, 5, 5},
		{64, 64, 5, 2, 2},
	}

	for _, tt := range tests {
		data := make([]int, tt.width*tt.height)
		llW, llH := ForwardMultiLevel(data, tt.width, tt.height, tt.levels)
		assert.Equal(t, tt.wantLLW, llW, "LL width for %dx%d @ %d levels", tt.width, tt.height, tt.levels)
		assert.Equal(t, tt.wantLLH, llH, "LL height for %dx%d @ %d levels", tt.width, tt.height, tt.levels)
	}
}

func TestGetSubbandBounds(t *testing.T) {
	// 64x64 image with 3 decomposition levels
	width, height, numLevels := 64, 64, 3

	// Level 1 (highest resolution, works on full 64x64)
	ll1 := GetSubbandBounds(width, height, 1, numLevels, SubbandLL)
	hl1 := GetSubbandBounds(width, height, 1, numLevels, SubbandHL)
	lh1 := GetSubbandBounds(width, height, 1, numLevels, SubbandLH)
	hh1 := GetSubbandBounds(width, height, 1, numLevels, SubbandHH)

	assert.Equal(t, SubbandBounds{0, 0, 32, 32}, ll1)
	assert.Equal(t, SubbandBounds{32, 0, 64, 32}, hl1)
	assert.Equal(t, SubbandBounds{0, 32, 32, 64}, lh1)
	assert.Equal(t, SubbandBounds{32, 32, 64, 64}, hh1)

	// Level 2 (works on 32x32 LL from level 1)
	ll2 := GetSubbandBounds(width, height, 2, numLevels, SubbandLL)
	hl2 := GetSubbandBounds(width, height, 2, numLevels, SubbandHL)
	lh2 := GetSubbandBounds(width, height, 2, numLevels, SubbandLH)
	hh2 := GetSubbandBounds(width, height, 2, numLevels, SubbandHH)

	assert.Equal(t, SubbandBounds{0, 0, 16, 16}, ll2)
	assert.Equal(t, SubbandBounds{16, 0, 32, 16}, hl2)
	assert.Equal(t, SubbandBounds{0, 16, 16, 32}, lh2)
	assert.Equal(t, SubbandBounds{16, 16, 32, 32}, hh2)

	// Level 3 (lowest resolution, works on 16x16 LL from level 2)
	ll3 := GetSubbandBounds(width, height, 3, numLevels, SubbandLL)
	hl3 := GetSubbandBounds(width, height, 3, numLevels, SubbandHL)

	assert.Equal(t, SubbandBounds{0, 0, 8, 8}, ll3)
	assert.Equal(t, SubbandBounds{8, 0, 16, 8}, hl3)
}

func TestExtractInsertSubband(t *testing.T) {
	width, height := 8, 8
	data := make([]int, width*height)
	for i := range data {
		data[i] = i
	}

	// Extract HL subband (top-right 4x4)
	bounds := SubbandBounds{4, 0, 8, 4}
	extracted := ExtractSubband(data, width, height, bounds)

	assert.Equal(t, 16, len(extracted))
	assert.Equal(t, 4, extracted[0])  // data[0*8 + 4]
	assert.Equal(t, 5, extracted[1])  // data[0*8 + 5]
	assert.Equal(t, 12, extracted[4]) // data[1*8 + 4]

	// Modify and insert back
	for i := range extracted {
		extracted[i] = 999
	}
	InsertSubband(data, width, bounds, extracted)

	// Verify inserted
	for y := 0; y < 4; y++ {
		for x := 4; x < 8; x++ {
			assert.Equal(t, 999, data[y*width+x])
		}
	}
}

func TestDWT_LargeImage(t *testing.T) {
	// Test with a realistically sized image
	width, height := 256, 256
	levels := 5

	data := make([]int, width*height)
	for i := range data {
		data[i] = i % 65536
	}

	original := make([]int, len(data))
	copy(original, data)

	ForwardMultiLevel(data, width, height, levels)
	InverseMultiLevel(data, width, height, levels)

	assert.Equal(t, original, data)
}

func BenchmarkForward2D(b *testing.B) {
	width, height := 512, 512
	data := make([]int, width*height)
	for i := range data {
		data[i] = i % 256
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		Forward2D(data, width, height)
	}
}

func BenchmarkForwardMultiLevel(b *testing.B) {
	width, height, levels := 512, 512, 5
	data := make([]int, width*height)
	for i := range data {
		data[i] = i % 256
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		dataCopy := make([]int, len(data))
		copy(dataCopy, data)
		ForwardMultiLevel(dataCopy, width, height, levels)
	}
}
