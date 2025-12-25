package jpeg2k

// RCT implements the Reversible Color Transform for JPEG 2000
// as specified in ITU-T T.800 Annex G

// ForwardRCT applies the reversible color transform (RGB -> YCbCr)
// Input: r, g, b component arrays (same length)
// Output: y, cb, cr component arrays (modified in place using r, g, b buffers)
func ForwardRCT(r, g, b []int) (y, cb, cr []int) {
	n := len(r)
	y = make([]int, n)
	cb = make([]int, n)
	cr = make([]int, n)

	for i := 0; i < n; i++ {
		y[i] = (r[i] + 2*g[i] + b[i]) >> 2 // floor((R + 2G + B) / 4)
		cb[i] = b[i] - g[i]                 // B - G
		cr[i] = r[i] - g[i]                 // R - G
	}

	return y, cb, cr
}

// InverseRCT applies the inverse reversible color transform (YCbCr -> RGB)
// Input: y, cb, cr component arrays
// Output: r, g, b component arrays
func InverseRCT(y, cb, cr []int) (r, g, b []int) {
	n := len(y)
	r = make([]int, n)
	g = make([]int, n)
	b = make([]int, n)

	for i := 0; i < n; i++ {
		g[i] = y[i] - ((cb[i] + cr[i]) >> 2) // Y - floor((Cb + Cr) / 4)
		r[i] = cr[i] + g[i]                   // Cr + G
		b[i] = cb[i] + g[i]                   // Cb + G
	}

	return r, g, b
}

// ForwardRCTInPlace applies RCT in place
func ForwardRCTInPlace(r, g, b []int) {
	for i := range r {
		ri, gi, bi := r[i], g[i], b[i]
		r[i] = (ri + 2*gi + bi) >> 2 // Y
		g[i] = bi - gi                // Cb
		b[i] = ri - gi                // Cr
	}
}

// InverseRCTInPlace applies inverse RCT in place
func InverseRCTInPlace(y, cb, cr []int) {
	for i := range y {
		yi, cbi, cri := y[i], cb[i], cr[i]
		g := yi - ((cbi + cri) >> 2)
		y[i] = cri + g  // R
		cb[i] = g       // G
		cr[i] = cbi + g // B
	}
}

// ApplyRCT applies RCT to multi-component image data
// data is organized as [comp0, comp1, comp2, ...] where each component
// is a flattened 2D array of size width*height
func ApplyRCT(data [][]int) {
	if len(data) < 3 {
		return
	}
	ForwardRCTInPlace(data[0], data[1], data[2])
}

// ApplyInverseRCT applies inverse RCT to multi-component image data
func ApplyInverseRCT(data [][]int) {
	if len(data) < 3 {
		return
	}
	InverseRCTInPlace(data[0], data[1], data[2])
}
