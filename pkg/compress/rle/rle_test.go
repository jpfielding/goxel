package rle

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRLE_RoundTrip_Gray8(t *testing.T) {
	width, height := 100, 100
	img := image.NewGray(image.Rect(0, 0, width, height))

	// Fill with gradient and runs
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create runs
			if x < 50 {
				img.SetGray(x, y, color.Gray{Y: uint8(y)}) // Run of same value per row
			} else {
				img.SetGray(x, y, color.Gray{Y: uint8(x)}) // Gradient
			}
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, img)
	require.NoError(t, err, "Encode failed")

	compressed := buf.Bytes()
	require.NotEmpty(t, compressed, "Compressed data is empty")
	t.Logf("8-bit Compressed size: %d / %d", len(compressed), width*height)

	decoded, err := Decode(compressed, width, height)
	require.NoError(t, err, "Decode failed")

	res, ok := decoded.(*image.Gray)
	require.True(t, ok, "Expected *image.Gray, got %T", decoded)

	// Verify pixels
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			expected := img.GrayAt(x, y)
			got := res.GrayAt(x, y)
			if expected != got {
				assert.Equal(t, expected, got, "Pixel mismatch at (%d, %d)", x, y)
				return // Stop on first error to avoid spam
			}
		}
	}
}

func TestRLE_RoundTrip_Gray16(t *testing.T) {
	width, height := 100, 100
	img := image.NewGray16(image.Rect(0, 0, width, height))

	// Fill with data to verify splitting
	// High byte constant, low byte varying, and vice versa
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// High byte: y
			// Low byte: x
			val := uint16(y)<<8 | uint16(x)
			img.SetGray16(x, y, color.Gray16{Y: val})
		}
	}

	var buf bytes.Buffer
	err := Encode(&buf, img)
	require.NoError(t, err, "Encode failed")

	compressed := buf.Bytes()
	require.NotEmpty(t, compressed, "Compressed data is empty")
	t.Logf("16-bit Compressed size: %d / %d", len(compressed), width*height*2)

	decoded, err := Decode(compressed, width, height)
	require.NoError(t, err, "Decode failed")

	res, ok := decoded.(*image.Gray16)
	require.True(t, ok, "Expected *image.Gray16, got %T", decoded)

	// Verify pixels
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			expected := img.Gray16At(x, y)
			got := res.Gray16At(x, y)
			if expected != got {
				assert.Equal(t, expected, got, "Pixel mismatch at (%d, %d)", x, y)
				return
			}
		}
	}
}
