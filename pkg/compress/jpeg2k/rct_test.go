package jpeg2k

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestForwardRCT(t *testing.T) {
	r := []int{100}
	g := []int{150}
	b := []int{200}
	y, cb, cr := ForwardRCT(r, g, b)

	// Verify RCT formula: Y = floor((R + 2G + B) / 4), Cb = B - G, Cr = R - G
	expectedY := (r[0] + 2*g[0] + b[0]) / 4
	expectedCb := b[0] - g[0]
	expectedCr := r[0] - g[0]

	assert.Equal(t, expectedY, y[0])
	assert.Equal(t, expectedCb, cb[0])
	assert.Equal(t, expectedCr, cr[0])
}

func TestInverseRCT(t *testing.T) {
	y := []int{150}
	cb := []int{50}
	cr := []int{-50}
	r, g, b := InverseRCT(y, cb, cr)

	// Verify inverse: G = Y - floor((Cb + Cr) / 4), R = Cr + G, B = Cb + G
	expectedG := y[0] - (cb[0]+cr[0])/4
	expectedR := cr[0] + expectedG
	expectedB := cb[0] + expectedG

	assert.Equal(t, expectedR, r[0])
	assert.Equal(t, expectedG, g[0])
	assert.Equal(t, expectedB, b[0])
}

func TestRCT_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		r, g, b int
	}{
		{"black", 0, 0, 0},
		{"white", 255, 255, 255},
		{"red", 255, 0, 0},
		{"green", 0, 255, 0},
		{"blue", 0, 0, 255},
		{"gray", 128, 128, 128},
		{"arbitrary", 100, 150, 200},
		{"high contrast", 255, 0, 255},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Forward
			r := []int{tt.r}
			g := []int{tt.g}
			b := []int{tt.b}
			y, cb, cr := ForwardRCT(r, g, b)

			// Inverse
			r2, g2, b2 := InverseRCT(y, cb, cr)

			// Should be exactly lossless
			assert.Equal(t, tt.r, r2[0], "R mismatch")
			assert.Equal(t, tt.g, g2[0], "G mismatch")
			assert.Equal(t, tt.b, b2[0], "B mismatch")
		})
	}
}

func TestApplyRCT(t *testing.T) {
	// Create test RGB data
	r := []int{100, 200, 50, 150}
	g := []int{150, 100, 200, 50}
	b := []int{200, 50, 150, 100}

	// Save originals
	origR := make([]int, len(r))
	origG := make([]int, len(g))
	origB := make([]int, len(b))
	copy(origR, r)
	copy(origG, g)
	copy(origB, b)

	components := [][]int{r, g, b}
	ApplyRCT(components)

	// Verify first pixel transformation
	expectedY := (origR[0] + 2*origG[0] + origB[0]) / 4
	assert.Equal(t, expectedY, components[0][0])

	// Apply inverse and check round-trip
	ApplyInverseRCT(components)

	assert.Equal(t, origR, components[0])
	assert.Equal(t, origG, components[1])
	assert.Equal(t, origB, components[2])
}

func TestForwardRCTInPlace(t *testing.T) {
	r := []int{100, 200}
	g := []int{150, 100}
	b := []int{200, 50}

	// Save originals
	origR := r[0]
	origG := g[0]
	origB := b[0]

	ForwardRCTInPlace(r, g, b)

	// Check first pixel
	expectedY := (origR + 2*origG + origB) / 4
	expectedCb := origB - origG
	expectedCr := origR - origG

	assert.Equal(t, expectedY, r[0])
	assert.Equal(t, expectedCb, g[0])
	assert.Equal(t, expectedCr, b[0])
}

func TestInverseRCTInPlace(t *testing.T) {
	// Start with known YCbCr values
	y := []int{150, 125}
	cb := []int{50, -50}
	cr := []int{-50, 100}

	InverseRCTInPlace(y, cb, cr)

	// Verify reconstruction
	// G = Y - floor((Cb + Cr) / 4)
	// R = Cr + G
	// B = Cb + G
	expectedG := 150 - (50-50)/4 // = 150
	expectedR := -50 + expectedG // = 100
	expectedB := 50 + expectedG  // = 200

	assert.Equal(t, expectedR, y[0])
	assert.Equal(t, expectedG, cb[0])
	assert.Equal(t, expectedB, cr[0])
}

func TestRCT_LargeArray_RoundTrip(t *testing.T) {
	size := 1000
	r := make([]int, size)
	g := make([]int, size)
	b := make([]int, size)

	// Fill with gradient
	for i := 0; i < size; i++ {
		r[i] = i % 256
		g[i] = (i * 2) % 256
		b[i] = (i * 3) % 256
	}

	// Save originals
	origR := make([]int, size)
	origG := make([]int, size)
	origB := make([]int, size)
	copy(origR, r)
	copy(origG, g)
	copy(origB, b)

	// Transform and inverse
	ForwardRCTInPlace(r, g, b)
	InverseRCTInPlace(r, g, b)

	// Compare
	assert.Equal(t, origR, r)
	assert.Equal(t, origG, g)
	assert.Equal(t, origB, b)
}

func TestRCT_MultiplePixels(t *testing.T) {
	// Test a variety of pixel values
	r := []int{0, 255, 128, 64, 192, 100}
	g := []int{0, 255, 128, 128, 32, 150}
	b := []int{0, 255, 128, 192, 64, 200}

	origR := make([]int, len(r))
	origG := make([]int, len(g))
	origB := make([]int, len(b))
	copy(origR, r)
	copy(origG, g)
	copy(origB, b)

	// Forward transform
	y, cb, cr := ForwardRCT(r, g, b)

	// Inverse transform
	r2, g2, b2 := InverseRCT(y, cb, cr)

	// Verify each pixel
	for i := range r {
		assert.Equal(t, origR[i], r2[i], "R mismatch at %d", i)
		assert.Equal(t, origG[i], g2[i], "G mismatch at %d", i)
		assert.Equal(t, origB[i], b2[i], "B mismatch at %d", i)
	}
}
