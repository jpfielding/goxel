package jpegls_test

import (
	"bytes"
	"image"
	"image/color"
	"testing"

	"github.com/jpfielding/goxel/pkg/compress/jpegls"
)

// TestRoundTrip16 tests encoding and decoding a 16-bit grayscale image
// and verifies pixel values match exactly (lossless)
func TestRoundTrip16(t *testing.T) {
	width, height := 312, 312

	// Create test image with known pattern
	original := image.NewGray16(image.Rect(0, 0, width, height))

	// Fill with various patterns to test edge cases
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Create a gradient + some high-contrast patterns
			var val uint16
			if x < 100 && y < 100 {
				// Top-left: solid black
				val = 0
			} else if x > 200 && y < 100 {
				// Top-right: solid white
				val = 65535
			} else {
				// Rest: gradient
				val = uint16((x + y*width) % 65536)
			}
			original.SetGray16(x, y, color.Gray16{Y: val})
		}
	}

	// Encode
	var buf bytes.Buffer
	if err := jpegls.Encode(&buf, original, nil); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	t.Logf("Encoded %dx%d to %d bytes", width, height, buf.Len())

	// Decode
	decoded, err := jpegls.Decode(&buf)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify dimensions
	bounds := decoded.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Fatalf("Dimension mismatch: got %dx%d, want %dx%d",
			bounds.Dx(), bounds.Dy(), width, height)
	}

	// Compare pixel values
	mismatches := 0
	var firstMismatch struct {
		x, y      int
		got, want uint16
	}

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origVal := original.Gray16At(x, y).Y

			// Get decoded value using RGBA() since that's proven to work
			r, _, _, _ := decoded.At(x, y).RGBA()
			decodedVal := uint16(r)

			if origVal != decodedVal {
				mismatches++
				if mismatches == 1 {
					firstMismatch.x = x
					firstMismatch.y = y
					firstMismatch.got = decodedVal
					firstMismatch.want = origVal
				}
			}
		}
	}

	if mismatches > 0 {
		t.Errorf("Found %d pixel mismatches out of %d pixels (%.2f%%)",
			mismatches, width*height, float64(mismatches)*100/float64(width*height))
		t.Errorf("First mismatch at (%d, %d): got %d, want %d",
			firstMismatch.x, firstMismatch.y, firstMismatch.got, firstMismatch.want)
	} else {
		t.Logf("All %d pixels match - lossless round-trip successful", width*height)
	}
}

// TestRoundTripRowOrder tests that pixel ordering is preserved correctly
// This specifically tests for row/column transposition issues
func TestRoundTripRowOrder(t *testing.T) {
	width, height := 100, 50 // Asymmetric to detect transposition

	original := image.NewGray16(image.Rect(0, 0, width, height))

	// Create asymmetric pattern: row index encoded in pixels
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Encode (x, y) position into pixel value
			// y * 1000 + x gives unique value for each position
			val := uint16((y*1000 + x) % 65536)
			original.SetGray16(x, y, color.Gray16{Y: val})
		}
	}

	// Encode
	var buf bytes.Buffer
	if err := jpegls.Encode(&buf, original, nil); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	// Decode
	decoded, err := jpegls.Decode(&buf)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Check specific positions
	testCases := []struct {
		x, y    int
		wantVal uint16
	}{
		{0, 0, 0},
		{99, 0, 99},
		{0, 49, 49000 % 65536},
		{50, 25, (25*1000 + 50) % 65536},
	}

	for _, tc := range testCases {
		r, _, _, _ := decoded.At(tc.x, tc.y).RGBA()
		got := uint16(r)
		if got != tc.wantVal {
			t.Errorf("At (%d, %d): got %d, want %d", tc.x, tc.y, got, tc.wantVal)
		}
	}
}
