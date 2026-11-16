package volume

import (
	"bytes"
	"image"
	"image/jpeg"
)

// extractPixelsFromImage extracts grayscale pixel values from an image.Image
func extractPixelsFromImage(img image.Image) []int {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]int, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, _, _, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// RGBA returns 16-bit values (0-65535), scale to int
			pixels[y*width+x] = int(r)
		}
	}
	return pixels
}

// decodeJPEGFrame decodes a standard JPEG compressed frame.
// Note: DICOM-specific formats (JPEG-LS, JPEG 2000, RLE) are handled by the dicom library.
func decodeJPEGFrame(data []byte, width, height int, signed bool) []int {
	// Standard JPEG decoding
	if img, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
		return extractPixelsFromImage(img)
	}
	return nil
}
