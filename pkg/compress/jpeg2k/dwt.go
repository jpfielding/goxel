package jpeg2k

// DWT implements the 5/3 reversible discrete wavelet transform
// as specified in ITU-T T.800 Annex F.

// Forward1D performs a 1D forward 5/3 wavelet transform in-place.
// Input signal is replaced with low-pass coefficients followed by high-pass coefficients.
// len(signal) must be at least 2.
func Forward1D(signal []int) {
	n := len(signal)
	if n < 2 {
		return
	}

	// Split into even (low) and odd (high) samples
	// Using lifting scheme:
	// 1. Predict: d[i] = x[2i+1] - floor((x[2i] + x[2i+2]) / 2)
	// 2. Update:  s[i] = x[2i] + floor((d[i-1] + d[i] + 2) / 4)

	// Temporary storage for the transform
	half := (n + 1) / 2 // Number of low-pass coefficients
	low := make([]int, half)
	high := make([]int, n-half)

	// Copy even samples to low, odd samples to high
	for i := 0; i < half; i++ {
		low[i] = signal[2*i]
	}
	for i := 0; i < len(high); i++ {
		high[i] = signal[2*i+1]
	}

	// Predict step (high-pass)
	for i := 0; i < len(high); i++ {
		left := low[i]
		right := left // Symmetric extension
		if i+1 < half {
			right = low[i+1]
		}
		high[i] -= (left + right) / 2
	}

	// Update step (low-pass)
	for i := 0; i < half; i++ {
		left := 0
		if i > 0 {
			left = high[i-1]
		} else if len(high) > 0 {
			left = high[0] // Symmetric extension
		}
		right := left
		if i < len(high) {
			right = high[i]
		}
		low[i] += (left + right + 2) / 4
	}

	// Pack results: low coefficients first, then high
	copy(signal[:half], low)
	copy(signal[half:], high)
}

// Inverse1D performs a 1D inverse 5/3 wavelet transform in-place.
// Input has low-pass coefficients followed by high-pass coefficients.
func Inverse1D(signal []int) {
	n := len(signal)
	if n < 2 {
		return
	}

	half := (n + 1) / 2
	low := make([]int, half)
	high := make([]int, n-half)

	// Unpack: low coefficients first, then high
	copy(low, signal[:half])
	copy(high, signal[half:])

	// Inverse update step
	for i := 0; i < half; i++ {
		left := 0
		if i > 0 {
			left = high[i-1]
		} else if len(high) > 0 {
			left = high[0]
		}
		right := left
		if i < len(high) {
			right = high[i]
		}
		low[i] -= (left + right + 2) / 4
	}

	// Inverse predict step
	for i := 0; i < len(high); i++ {
		left := low[i]
		right := left
		if i+1 < half {
			right = low[i+1]
		}
		high[i] += (left + right) / 2
	}

	// Interleave: even positions get low, odd positions get high
	for i := 0; i < half; i++ {
		signal[2*i] = low[i]
	}
	for i := 0; i < len(high); i++ {
		signal[2*i+1] = high[i]
	}
}

// Forward2D performs a 2D forward 5/3 wavelet transform.
// The image is transformed in-place, producing LL, HL, LH, HH subbands.
// After transform:
//   - LL (top-left): low-pass in both dimensions
//   - HL (top-right): high-pass horizontal, low-pass vertical
//   - LH (bottom-left): low-pass horizontal, high-pass vertical
//   - HH (bottom-right): high-pass in both dimensions
func Forward2D(data []int, width, height int) {
	if width < 2 || height < 2 {
		return
	}

	// Transform rows
	row := make([]int, width)
	for y := 0; y < height; y++ {
		offset := y * width
		copy(row, data[offset:offset+width])
		Forward1D(row)
		copy(data[offset:offset+width], row)
	}

	// Transform columns
	col := make([]int, height)
	for x := 0; x < width; x++ {
		// Extract column
		for y := 0; y < height; y++ {
			col[y] = data[y*width+x]
		}
		Forward1D(col)
		// Put back column
		for y := 0; y < height; y++ {
			data[y*width+x] = col[y]
		}
	}
}

// Inverse2D performs a 2D inverse 5/3 wavelet transform.
// Reconstructs the original image from LL, HL, LH, HH subbands.
func Inverse2D(data []int, width, height int) {
	if width < 2 || height < 2 {
		return
	}

	// Inverse transform columns first
	col := make([]int, height)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			col[y] = data[y*width+x]
		}
		Inverse1D(col)
		for y := 0; y < height; y++ {
			data[y*width+x] = col[y]
		}
	}

	// Inverse transform rows
	row := make([]int, width)
	for y := 0; y < height; y++ {
		offset := y * width
		copy(row, data[offset:offset+width])
		Inverse1D(row)
		copy(data[offset:offset+width], row)
	}
}

// ForwardMultiLevel performs multi-level 2D DWT decomposition.
// Each level transforms the LL subband from the previous level.
// Returns dimensions of the final LL subband.
func ForwardMultiLevel(data []int, width, height, levels int) (llWidth, llHeight int) {
	llWidth = width
	llHeight = height

	for level := 0; level < levels; level++ {
		if llWidth < 2 || llHeight < 2 {
			break
		}

		// Transform the LL region from previous level (or full image for level 0)
		forwardLLRegion(data, width, llWidth, llHeight)

		llWidth = (llWidth + 1) / 2
		llHeight = (llHeight + 1) / 2
	}

	return llWidth, llHeight
}

// InverseMultiLevel performs multi-level 2D inverse DWT reconstruction.
// Levels are processed in reverse order.
func InverseMultiLevel(data []int, width, height, levels int) {
	// Calculate LL dimensions at each level
	dims := make([][2]int, levels+1)
	dims[0] = [2]int{width, height}
	for i := 1; i <= levels; i++ {
		dims[i] = [2]int{(dims[i-1][0] + 1) / 2, (dims[i-1][1] + 1) / 2}
	}

	// Reconstruct from smallest to largest
	for level := levels - 1; level >= 0; level-- {
		llWidth := dims[level][0]
		llHeight := dims[level][1]

		if llWidth < 2 || llHeight < 2 {
			continue
		}

		inverseLLRegion(data, width, llWidth, llHeight)
	}
}

// forwardLLRegion transforms only the top-left region of the image
func forwardLLRegion(data []int, stride, width, height int) {
	if width < 2 || height < 2 {
		return
	}

	// Transform rows in the region
	row := make([]int, width)
	for y := 0; y < height; y++ {
		offset := y * stride
		copy(row, data[offset:offset+width])
		Forward1D(row)
		copy(data[offset:offset+width], row)
	}

	// Transform columns in the region
	col := make([]int, height)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			col[y] = data[y*stride+x]
		}
		Forward1D(col)
		for y := 0; y < height; y++ {
			data[y*stride+x] = col[y]
		}
	}
}

// inverseLLRegion reconstructs only the top-left region
func inverseLLRegion(data []int, stride, width, height int) {
	if width < 2 || height < 2 {
		return
	}

	// Inverse columns first
	col := make([]int, height)
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			col[y] = data[y*stride+x]
		}
		Inverse1D(col)
		for y := 0; y < height; y++ {
			data[y*stride+x] = col[y]
		}
	}

	// Inverse rows
	row := make([]int, width)
	for y := 0; y < height; y++ {
		offset := y * stride
		copy(row, data[offset:offset+width])
		Inverse1D(row)
		copy(data[offset:offset+width], row)
	}
}

// SubbandBounds returns the bounds of a subband at a given resolution level.
// level 0 is the highest resolution (original size).
// For level > 0, returns bounds of LL, HL, LH, or HH subband.
type SubbandBounds struct {
	X0, Y0 int // Top-left corner
	X1, Y1 int // Bottom-right corner (exclusive)
}

// GetSubbandBounds calculates the bounds of a specific subband.
// level is numbered from 1 (highest detail) to numLevels (lowest LL).
// Level numbering: higher level = lower resolution (more halvings applied).
func GetSubbandBounds(width, height, level, numLevels int, subband Subband) SubbandBounds {
	if level < 1 || level > numLevels {
		return SubbandBounds{}
	}

	// Calculate dimensions of the region at this level
	// Level N works on dimensions that have been halved (level-1) times
	w, h := width, height
	for i := 1; i < level; i++ {
		w = (w + 1) / 2
		h = (h + 1) / 2
	}

	halfW := (w + 1) / 2
	halfH := (h + 1) / 2

	switch subband {
	case SubbandLL:
		return SubbandBounds{0, 0, halfW, halfH}
	case SubbandHL:
		return SubbandBounds{halfW, 0, w, halfH}
	case SubbandLH:
		return SubbandBounds{0, halfH, halfW, h}
	case SubbandHH:
		return SubbandBounds{halfW, halfH, w, h}
	default:
		return SubbandBounds{}
	}
}

// ExtractSubband extracts a subband from the transformed data.
func ExtractSubband(data []int, width, height int, bounds SubbandBounds) []int {
	subW := bounds.X1 - bounds.X0
	subH := bounds.Y1 - bounds.Y0
	result := make([]int, subW*subH)

	for y := 0; y < subH; y++ {
		for x := 0; x < subW; x++ {
			srcIdx := (bounds.Y0+y)*width + (bounds.X0 + x)
			dstIdx := y*subW + x
			result[dstIdx] = data[srcIdx]
		}
	}
	return result
}

// InsertSubband inserts a subband back into the transformed data.
func InsertSubband(data []int, width int, bounds SubbandBounds, subband []int) {
	subW := bounds.X1 - bounds.X0
	subH := bounds.Y1 - bounds.Y0

	for y := 0; y < subH; y++ {
		for x := 0; x < subW; x++ {
			srcIdx := y*subW + x
			dstIdx := (bounds.Y0+y)*width + (bounds.X0 + x)
			data[dstIdx] = subband[srcIdx]
		}
	}
}
