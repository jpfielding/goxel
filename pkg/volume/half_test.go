package volume

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFloat16Bits(t *testing.T) {
	cases := []struct {
		in   float32
		want uint16
	}{
		{0, 0x0000},
		{float32(math.Copysign(0, -1)), 0x8000},
		{1, 0x3c00},
		{-2, 0xc000},
		{0.5, 0x3800},
		{65504, 0x7bff},
		{1e6, 0x7c00},
		{float32(math.Inf(-1)), 0xfc00},
		{6.103515625e-05, 0x0400},       // smallest normal
		{5.960464477539063e-08, 0x0001}, // smallest subnormal
		{1e-9, 0x0000},
		{1.0 / 65535, 0x0100}, // a density step of 1 survives as a subnormal-adjacent value
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, float16Bits(c.in), "float16Bits(%g)", c.in)
	}
	assert.True(t, float16Bits(float32(math.NaN()))&0x7c00 == 0x7c00 && float16Bits(float32(math.NaN()))&0x3ff != 0)
}

func TestFloat16BitsRelativeError(t *testing.T) {
	// Gradients are stored as signed diff/65535; check precision across that range.
	for _, d := range []float32{1, 7, 100, 1000, 12345, 65535} {
		v := d / 65535
		h := float16Bits(v)
		got := halfToFloat(h)
		assert.InEpsilonf(t, v, got, 1e-3, "value %g", v)
	}
}

func halfToFloat(h uint16) float32 {
	sign := float32(1)
	if h&0x8000 != 0 {
		sign = -1
	}
	exp := int(h>>10) & 0x1f
	mant := float32(h & 0x3ff)
	if exp == 0 {
		return sign * mant * float32(math.Pow(2, -24))
	}
	return sign * (1 + mant/1024) * float32(math.Pow(2, float64(exp-15)))
}

func TestDownsample2x(t *testing.T) {
	// 3 x 2 x 2: odd X keeps one output column built from the first two samples.
	data := []uint16{
		0, 2, 100,
		4, 6, 100,
		8, 10, 100,
		12, 14, 100,
	}
	out, nx, ny, nz := downsample2x(data, 3, 2, 2)
	assert.Equal(t, []int{1, 1, 1}, []int{nx, ny, nz})
	assert.Equal(t, []uint16{7}, out)

	out, nx, ny, nz = downsample2x([]uint16{5}, 1, 1, 1)
	assert.Equal(t, []int{1, 1, 1}, []int{nx, ny, nz})
	assert.Equal(t, []uint16{5}, out)
}
