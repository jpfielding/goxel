package jpeg2k

import (
	"errors"
	"fmt"
	"math"
)

// MaxDecodedBytes caps the memory a header can make Decode allocate:
// width × height × components × bytes per working sample (4 for the T.800
// int32 coefficient buffer, 8 for the legacy decoder), and separately the
// estimated per-code-block state. Headers declaring more are rejected with
// ErrImageTooLarge before anything sized by them is allocated. Raise it for
// legitimately larger images; values <= 0 disable the cap.
var MaxDecodedBytes int64 = 1 << 30 // 1 GiB

// ErrImageTooLarge reports a header whose decoded size exceeds MaxDecodedBytes.
var ErrImageTooLarge = errors.New("jpeg2k: decoded image exceeds MaxDecodedBytes")

// checkDecodedSize validates header-derived dimensions before allocation.
// It rejects non-positive values, multiplication overflow, and sizes above
// MaxDecodedBytes.
func checkDecodedSize(width, height, components, bytesPerSample int) error {
	if width <= 0 || height <= 0 || components <= 0 || bytesPerSample <= 0 {
		return fmt.Errorf("%w: image geometry %dx%d, %d components, %d bytes/sample", ErrInvalidData, width, height, components, bytesPerSample)
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

// codeBlockStateBytes estimates decoder memory per code-block (geometry,
// packet contribution, tag-tree nodes, Lblock).
const codeBlockStateBytes = 128

// checkCodeBlockState bounds the per-code-block decoder state, counted from
// the geometry formulas without building it. Small code-blocks on a large
// image can otherwise cost more than the coefficient buffer.
func checkCodeBlockState(w, h, levels, xcb, ycb int) error {
	var blocks int64
	for r := 0; r <= levels; r++ {
		// Mirrors buildGeometry: precinct exponent caps the code-block size.
		pp, kinds, n := ppDefault-1, []bandKind{bandHL, bandLH, bandHH}, levels-r+1
		if r == 0 {
			pp, kinds, n = ppDefault, []bandKind{bandLL}, levels
		}
		xe, ye := min(xcb, pp), min(ycb, pp)
		for _, k := range kinds {
			xob, yob, _ := bandOffsets(k)
			b := subbandRect(w, h, n, xob, yob)
			blocks += int64(ceilShift(b.width(), xe)) * int64(ceilShift(b.height(), ye))
		}
	}
	if MaxDecodedBytes > 0 && blocks > MaxDecodedBytes/codeBlockStateBytes {
		return fmt.Errorf("%w: %d code-blocks of decoder state > %d bytes", ErrImageTooLarge, blocks, MaxDecodedBytes)
	}
	return nil
}
