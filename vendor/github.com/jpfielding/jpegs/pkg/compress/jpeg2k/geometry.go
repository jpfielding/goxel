package jpeg2k

// Resolution, band, precinct and code-block geometry (ITU-T T.800 B.5-B.7)
// for one tile-component. Ported from dicos.rs crates/jpeg2k/src/geometry.rs.
//
// All grid origins are zero (the codestream parser rejects anything else),
// so tile-component coordinates equal image coordinates. The math keeps the
// spec's ceil-division form so it reads against T.800.
//
// The conformant multi-level DWT transforms the LL region left by the
// previous level in place at the full tile stride. One 2-D level over a WxH
// region leaves LL at (0,0), HL at (ceil(W/2),0), LH at (0,ceil(H/2)) and HH
// at (ceil(W/2),ceil(H/2)); mallat records where each band sits in that
// packed buffer.

// ppDefault is the default precinct exponent (2^15 precincts, T.800 A.6.1).
const ppDefault = 15

// rect is the half-open rectangle [x0,x1) x [y0,y1).
type rect struct {
	x0, y0, x1, y1 int
}

func (r rect) width() int  { return max(r.x1-r.x0, 0) }
func (r rect) height() int { return max(r.y1-r.y0, 0) }
func (r rect) area() int   { return r.width() * r.height() }
func (r rect) empty() bool { return r.width() == 0 || r.height() == 0 }

// codeBlockGeom is one clipped code-block in band-native coordinates.
type codeBlockGeom struct {
	rect     rect
	precinct int // owning precinct; 0 for the single default precinct
}

// band is one DWT sub-band of a resolution level.
type band struct {
	kind     bandKind
	rect     rect // band-native, anchored at (0,0)
	gainLog2 int  // 0 LL, 1 HL/LH, 2 HH (T.800 E.1)
	// xOff, yOff locate the band inside the packed (Mallat) tile buffer.
	xOff, yOff int
	cbs        []codeBlockGeom // raster order, clipped, non-empty
	gridW      int             // code-block columns (0 when empty)
	gridH      int             // code-block rows (0 when empty)
}

// resolution is level r in 0..NL (0 = coarsest LL).
type resolution struct {
	r          int
	rect       rect
	bands      []band // [LL] at r == 0, else [HL, LH, HH]
	precinctsW int
	precinctsH int
}

// ceilShift is ceil(v / 2^k) for v >= 0.
func ceilShift(v, k int) int {
	if k == 0 {
		return v
	}
	if k >= 62 {
		if v > 0 {
			return 1
		}
		return 0
	}
	return int((int64(v) + 1<<k - 1) >> k) // int64: k may reach 32
}

// ceilDiv is ceil(a / b) for b > 0, valid for negative a.
func ceilDiv(a, b int64) int64 {
	q := a / b
	if a%b > 0 {
		q++
	}
	return q
}

// subbandRect is T.800 Eq. B-15 for a tile anchored at the origin. nb is the
// decomposition level; (xob, yob) is (0,0) LL, (1,0) HL, (0,1) LH, (1,1) HH.
func subbandRect(tileW, tileH, nb, xob, yob int) rect {
	d := int64(1) << nb
	var half int64
	if nb > 0 {
		half = int64(1) << (nb - 1)
	}
	offX, offY := int64(xob)*half, int64(yob)*half
	c := func(a int64) int { return int(max(ceilDiv(a, d), 0)) }
	return rect{x0: c(-offX), y0: c(-offY), x1: c(int64(tileW) - offX), y1: c(int64(tileH) - offY)}
}

func bandOffsets(kind bandKind) (xob, yob, gain int) {
	switch kind {
	case bandHL:
		return 1, 0, 1
	case bandLH:
		return 0, 1, 1
	case bandHH:
		return 1, 1, 2
	default:
		return 0, 0, 0
	}
}

// codeBlocks partitions a band on the 2^xcb x 2^ycb grid anchored at the
// band origin (T.800 B.7).
func codeBlocks(b rect, xcb, ycb, ppx, ppy, precinctsW int) ([]codeBlockGeom, int, int) {
	w, h := b.width(), b.height()
	if w == 0 || h == 0 {
		return nil, 0, 0
	}
	cbw, cbh := 1<<xcb, 1<<ycb
	gw, gh := ceilShift(w, xcb), ceilShift(h, ycb)
	cbs := make([]codeBlockGeom, 0, gw*gh)
	for row := range gh {
		by0, by1 := row*cbh, min((row+1)*cbh, h)
		prow := by0 >> ppy
		for col := range gw {
			bx0, bx1 := col*cbw, min((col+1)*cbw, w)
			cbs = append(cbs, codeBlockGeom{
				rect:     rect{bx0, by0, bx1, by1},
				precinct: prow*precinctsW + bx0>>ppx,
			})
		}
	}
	return cbs, gw, gh
}

func newBand(kind bandKind, tileW, tileH, nb, xOff, yOff, xcb, ycb, ppx, ppy, precinctsW int) band {
	xob, yob, gain := bandOffsets(kind)
	r := subbandRect(tileW, tileH, nb, xob, yob)
	cbs, gw, gh := codeBlocks(r, xcb, ycb, ppx, ppy, precinctsW)
	return band{kind: kind, rect: r, gainLog2: gain, xOff: xOff, yOff: yOff, cbs: cbs, gridW: gw, gridH: gh}
}

// buildGeometry returns resolutions 0..levels for a tileW x tileH
// tile-component with 2^xcb x 2^ycb code-blocks. Callers validate and size
// check the inputs first; this allocates per code-block.
func buildGeometry(tileW, tileH, levels, xcb, ycb int) []resolution {
	res := make([]resolution, 0, levels+1)
	for r := 0; r <= levels; r++ {
		rw := ceilShift(tileW, levels-r)
		rh := ceilShift(tileH, levels-r)
		pw, ph := 0, 0
		if rw != 0 && rh != 0 {
			pw, ph = ceilShift(rw, ppDefault), ceilShift(rh, ppDefault)
		}
		// Effective exponents (B.7): capped by the precinct exponent, which
		// drops by one at r >= 1.
		pp := ppDefault
		if r > 0 {
			pp = ppDefault - 1
		}
		xe, ye := min(xcb, pp), min(ycb, pp)

		var bands []band
		if r == 0 {
			bands = []band{newBand(bandLL, tileW, tileH, levels, 0, 0, xe, ye, pp, pp, pw)}
		} else {
			n := levels - r + 1
			llW, llH := ceilShift(tileW, n), ceilShift(tileH, n)
			bands = []band{
				newBand(bandHL, tileW, tileH, n, llW, 0, xe, ye, pp, pp, pw),
				newBand(bandLH, tileW, tileH, n, 0, llH, xe, ye, pp, pp, pw),
				newBand(bandHH, tileW, tileH, n, llW, llH, xe, ye, pp, pp, pw),
			}
		}
		res = append(res, resolution{r: r, rect: rect{0, 0, rw, rh}, bands: bands, precinctsW: pw, precinctsH: ph})
	}
	return res
}

// bandKind is a DWT sub-band orientation.
type bandKind uint8

const (
	bandLL bandKind = iota // low-low (approximation)
	bandHL                 // horizontal detail
	bandLH                 // vertical detail
	bandHH                 // diagonal detail
)

func (k bandKind) String() string {
	switch k {
	case bandLL:
		return "LL"
	case bandHL:
		return "HL"
	case bandLH:
		return "LH"
	case bandHH:
		return "HH"
	}
	return "band?"
}
