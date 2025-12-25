package jpegli

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

// TestRoundTrip8 tests encode/decode roundtrip for 8-bit images
func TestRoundTrip8(t *testing.T) {
	// Create test image
	width, height := 64, 64
	img := image.NewGray(image.Rect(0, 0, width, height))

	// Fill with gradient pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			val := uint8((x + y) % 256)
			img.SetGray(x, y, color.Gray{Y: val})
		}
	}

	// Encode
	var buf bytes.Buffer
	if err := Encode(&buf, img, nil); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	t.Logf("Encoded size: %d bytes", buf.Len())

	// Decode
	decoded, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify dimensions
	bounds := decoded.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("Dimension mismatch: got %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}

	// Verify pixels
	mismatchCount := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origR, _, _, _ := img.At(x, y).RGBA()
			decR, _, _, _ := decoded.At(x, y).RGBA()
			orig := int(origR >> 8)
			dec := int(decR >> 8)
			if orig != dec {
				if mismatchCount == 0 {
					t.Errorf("First mismatch at (%d,%d): orig=%d, dec=%d", x, y, orig, dec)
				}
				mismatchCount++
			}
		}
	}

	if mismatchCount > 0 {
		t.Errorf("Total mismatches: %d/%d pixels (%.2f%%)", mismatchCount, width*height, float64(mismatchCount)*100/float64(width*height))
	} else {
		t.Logf("SUCCESS: All %d pixels match after roundtrip", width*height)
	}
}

// TestRoundTrip16 tests encode/decode roundtrip for 16-bit images
func TestRoundTrip16(t *testing.T) {
	// Create test image
	width, height := 64, 64
	img := image.NewGray16(image.Rect(0, 0, width, height))

	// Fill with gradient pattern
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			val := uint16((x*256 + y*512) % 65536)
			img.SetGray16(x, y, color.Gray16{Y: val})
		}
	}

	// Encode
	var buf bytes.Buffer
	if err := Encode(&buf, img, nil); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	t.Logf("Encoded size: %d bytes", buf.Len())

	// Decode
	decoded, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify dimensions
	bounds := decoded.Bounds()
	if bounds.Dx() != width || bounds.Dy() != height {
		t.Errorf("Dimension mismatch: got %dx%d, want %dx%d", bounds.Dx(), bounds.Dy(), width, height)
	}

	// Verify pixels
	mismatchCount := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origR, _, _, _ := img.At(x, y).RGBA()
			decR, _, _, _ := decoded.At(x, y).RGBA()
			if origR != decR {
				if mismatchCount == 0 {
					t.Errorf("First mismatch at (%d,%d): orig=%d, dec=%d", x, y, origR, decR)
				}
				mismatchCount++
			}
		}
	}

	if mismatchCount > 0 {
		t.Errorf("Total mismatches: %d/%d pixels (%.2f%%)", mismatchCount, width*height, float64(mismatchCount)*100/float64(width*height))
	} else {
		t.Logf("SUCCESS: All %d pixels match after roundtrip", width*height)
	}
}

// TestRoundTripDICOSData tests with real DICOS-like data (312x312, 16-bit)
func TestRoundTripDICOSData(t *testing.T) {
	width, height := 312, 312
	img := image.NewGray16(image.Rect(0, 0, width, height))

	// Fill with pattern similar to CT data
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			// Simulate CT scan pattern with varying intensity
			dist := (x-width/2)*(x-width/2) + (y-height/2)*(y-height/2)
			val := uint16(16000 - dist%8000)
			img.SetGray16(x, y, color.Gray16{Y: val})
		}
	}

	// Encode
	var buf bytes.Buffer
	if err := Encode(&buf, img, nil); err != nil {
		t.Fatalf("Encode failed: %v", err)
	}

	origSize := width * height * 2
	compressedSize := buf.Len()
	ratio := float64(origSize) / float64(compressedSize)
	t.Logf("Original: %d bytes, Compressed: %d bytes, Ratio: %.2fx", origSize, compressedSize, ratio)

	// Decode
	decoded, err := Decode(&buf)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	// Verify
	mismatchCount := 0
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			origR, _, _, _ := img.At(x, y).RGBA()
			decR, _, _, _ := decoded.At(x, y).RGBA()
			if origR != decR {
				if mismatchCount == 0 {
					t.Errorf("First mismatch at (%d,%d): orig=%d, dec=%d", x, y, origR, decR)
				}
				mismatchCount++
			}
		}
	}

	if mismatchCount > 0 {
		t.Errorf("Total mismatches: %d/%d pixels", mismatchCount, width*height)
	} else {
		t.Logf("SUCCESS: All %d pixels match after roundtrip", width*height)
	}
}
