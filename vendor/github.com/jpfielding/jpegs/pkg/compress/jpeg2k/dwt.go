package jpeg2k

// Reversible 5/3 DWT (ITU-T T.800 Annex F) with floor rounding, as OpenJPEG.
// Ported from dicos.rs crates/jpeg2k/src/dwt.rs (the *_conformant paths).
// The frozen truncating transform used by legacy streams is in legacy.go.

// forward1D transforms s in place to [lows..., highs...] with
// whole-sample symmetric extension. low and high are scratch buffers of at
// least ceil(n/2) and floor(n/2). Length < 2 is a no-op.
func forward1D(s, low, high []int32) {
	n := len(s)
	if n < 2 {
		return
	}
	half := (n + 1) / 2
	low, high = low[:half], high[:n-half]
	for i := range low {
		low[i] = s[2*i]
	}
	for i := range high {
		high[i] = s[2*i+1]
	}
	// Predict: d[i] = x[2i+1] - floor((x[2i] + x[2i+2]) / 2).
	for i := range high {
		l, r := low[i], low[i]
		if i+1 < half {
			r = low[i+1]
		}
		high[i] -= (l + r) >> 1
	}
	// Update: s[i] = x[2i] + floor((d[i-1] + d[i] + 2) / 4).
	for i := range low {
		l, r := sideHigh(high, i)
		low[i] += (l + r + 2) >> 2
	}
	copy(s, low)
	copy(s[half:], high)
}

// inverse1D undoes forward1D.
func inverse1D(s, low, high []int32) {
	n := len(s)
	if n < 2 {
		return
	}
	half := (n + 1) / 2
	low, high = low[:half], high[:n-half]
	copy(low, s[:half])
	copy(high, s[half:])
	for i := range low {
		l, r := sideHigh(high, i)
		low[i] -= (l + r + 2) >> 2
	}
	for i := range high {
		l, r := low[i], low[i]
		if i+1 < half {
			r = low[i+1]
		}
		high[i] += (l + r) >> 1
	}
	for i, v := range low {
		s[2*i] = v
	}
	for i, v := range high {
		s[2*i+1] = v
	}
}

// sideHigh returns d[i-1] and d[i] under symmetric extension.
func sideHigh(high []int32, i int) (l, r int32) {
	switch {
	case i > 0:
		l = high[i-1]
	case len(high) > 0:
		l = high[0]
	}
	r = l
	if i < len(high) {
		r = high[i]
	}
	return l, r
}

// dwtScratch holds the per-call buffers reused across rows, columns and
// levels.
type dwtScratch struct {
	line, low, high []int32
}

func newDWTScratch(w, h int) *dwtScratch {
	n := max(w, h)
	return &dwtScratch{line: make([]int32, n), low: make([]int32, (n+1)/2), high: make([]int32, n/2)}
}

// forwardDWT applies levels 2-D levels in place over a width x height
// buffer, each to the LL region left by the previous one. Columns are
// transformed before rows (as T.800 / OpenJPEG; floor rounding makes the
// order observable).
func forwardDWT(data []int32, width, height, levels int) {
	sc := newDWTScratch(width, height)
	w, h := width, height
	for range levels {
		sc.columns(data, width, w, h, forward1D)
		sc.rows(data, width, w, h, forward1D)
		w, h = (w+1)/2, (h+1)/2
	}
}

// inverseDWT undoes forwardDWT (rows, then columns, finest level last).
func inverseDWT(data []int32, width, height, levels int) {
	sc := newDWTScratch(width, height)
	dims := make([][2]int, levels+1)
	dims[0] = [2]int{width, height}
	for i := 1; i <= levels; i++ {
		dims[i] = [2]int{(dims[i-1][0] + 1) / 2, (dims[i-1][1] + 1) / 2}
	}
	for l := levels - 1; l >= 0; l-- {
		w, h := dims[l][0], dims[l][1]
		sc.rows(data, width, w, h, inverse1D)
		sc.columns(data, width, w, h, inverse1D)
	}
}

func (sc *dwtScratch) rows(data []int32, stride, w, h int, f func(s, low, high []int32)) {
	for y := range h {
		f(data[y*stride:y*stride+w], sc.low, sc.high)
	}
}

func (sc *dwtScratch) columns(data []int32, stride, w, h int, f func(s, low, high []int32)) {
	col := sc.line[:h]
	for x := range w {
		for y := range col {
			col[y] = data[y*stride+x]
		}
		f(col, sc.low, sc.high)
		for y, v := range col {
			data[y*stride+x] = v
		}
	}
}
