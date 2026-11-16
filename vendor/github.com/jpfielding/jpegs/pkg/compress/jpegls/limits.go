package jpegls

import (
	"errors"
	"fmt"
	"math"
)

// MaxDecodedBytes caps the decoded image size (width × height × components ×
// bytes per sample) that Decode will allocate. Headers declaring more are
// rejected with ErrImageTooLarge before any pixel buffer is allocated.
// Raise it for legitimately larger images; values <= 0 disable the cap.
var MaxDecodedBytes int64 = 1 << 30 // 1 GiB

// ErrImageTooLarge reports a header whose decoded size exceeds MaxDecodedBytes.
var ErrImageTooLarge = errors.New("jpegls: decoded image exceeds MaxDecodedBytes")

// checkDecodedSize validates header-derived dimensions before allocation.
// It rejects non-positive values, multiplication overflow, and sizes above
// MaxDecodedBytes.
func checkDecodedSize(width, height, components, bytesPerSample int) error {
	if width <= 0 || height <= 0 || components <= 0 || bytesPerSample <= 0 {
		return fmt.Errorf("jpegls: invalid image geometry %dx%d, %d components, %d bytes/sample", width, height, components, bytesPerSample)
	}
	size := int64(1)
	for _, f := range []int{width, height, components, bytesPerSample} {
		if size > math.MaxInt64/int64(f) {
			return fmt.Errorf("%w: %dx%dx%dx%d overflows", ErrImageTooLarge, width, height, components, bytesPerSample)
		}
		size *= int64(f)
	}
	if MaxDecodedBytes > 0 && size > MaxDecodedBytes {
		return fmt.Errorf("%w: %dx%dx%dx%d = %d bytes > %d", ErrImageTooLarge, width, height, components, bytesPerSample, size, MaxDecodedBytes)
	}
	return nil
}
