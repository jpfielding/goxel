package jpegli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeFrame356(t *testing.T) {
	path := filepath.Join("testdata", "frame356.jpg")
	f, err := os.Open(path)
	if err != nil {
		t.Skipf("frame356.jpg not found: %v", err)
	}
	defer f.Close()

	img, err := Decode(f)
	if err != nil {
		t.Fatalf("Decode failed: %v", err)
	}

	bounds := img.Bounds()
	t.Logf("Successfully decoded %dx%d image", bounds.Dx(), bounds.Dy())
}
