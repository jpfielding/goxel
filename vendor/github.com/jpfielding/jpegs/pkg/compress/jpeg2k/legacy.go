package jpeg2k

import (
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
)

// FROZEN legacy decoder. Do not change its output.
//
// Before v0.3.0 this package (and dicos.rs 1.0.0) wrote a non-T.800 format
// with the usual SOC/SIZ/COD/QCD/SOT markers but SOD = 0xFFD3, all-zero QCD
// exponents, and a tile payload of, per component, width u16be, height u16be
// and width*height big-endian int32 coefficients of a truncating-division
// 5/3 DWT (no DC shift). Go wrote Psot = 12 + len, dicos.rs 14 + len. Go
// also wrote 8-bit gray and RGB (3 components, RCT when COD MCT = 1).
//
// testdata/legacy pins the exact pixels; this file must keep reproducing
// them.

// LegacyPolicy selects how decoding treats the legacy format.
type LegacyPolicy int

const (
	// LegacyAuto decodes T.800 streams and falls back to the legacy decoder
	// only when a stream carries the legacy fingerprint (all-zero QCD
	// exponents, SOD 0xFFD3 and a payload of exactly the legacy size). The
	// fingerprint is checked before any packet is decoded.
	LegacyAuto LegacyPolicy = iota
	// StandardOnly decodes T.800 streams only. Use it for untrusted data
	// that claims to be JPEG 2000.
	StandardOnly
	// LegacyOnly decodes legacy streams only.
	LegacyOnly
)

// isLegacy reports whether h carries the legacy fingerprint.
func (h *header) isLegacy() bool {
	if !h.sodLegacy || !allZero(h.exps) {
		return false
	}
	nc := len(h.comps)
	w, ht := uint64(h.width()), uint64(h.height())
	if (nc != 1 && nc != 3) || w > 0xFFFF || ht > 0xFFFF {
		return false
	}
	per := 4 + 4*w*ht
	if uint64(len(h.payload)) != uint64(nc)*per {
		return false
	}
	for c := range uint64(nc) {
		p := h.payload[c*per:]
		if uint64(binary.BigEndian.Uint16(p)) != w || uint64(binary.BigEndian.Uint16(p[2:])) != ht {
			return false
		}
	}
	return true
}

// decodeLegacy reproduces the pre-T.800 decoder. h must satisfy isLegacy.
func decodeLegacy(h *header) (image.Image, error) {
	c0 := h.comps[0]
	for _, c := range h.comps {
		if c != c0 || c.signed || c.precision > 16 || c.dx != 1 || c.dy != 1 {
			return nil, fmt.Errorf("%w: legacy stream with mixed, signed, sub-sampled or >16-bit components", ErrUnsupported)
		}
	}
	if h.xosiz != 0 || h.yosiz != 0 {
		return nil, fmt.Errorf("%w: legacy stream with non-zero origin", ErrInvalidData)
	}
	w, ht, nc := h.width(), h.height(), len(h.comps)
	if err := checkDecodedSize(w, ht, nc, 8); err != nil {
		return nil, err
	}
	comps := make([][]int, nc)
	per := 4 + 4*w*ht
	for c := range comps {
		p := h.payload[c*per+4 : (c+1)*per]
		coeffs := make([]int, w*ht)
		for i := range coeffs {
			coeffs[i] = int(int32(binary.BigEndian.Uint32(p[4*i:])))
		}
		legacyInverseMultiLevel(coeffs, w, ht, int(h.levels))
		comps[c] = coeffs
	}
	if nc == 3 {
		if h.mct != 0 {
			legacyInverseRCT(comps[0], comps[1], comps[2])
		}
		img := image.NewRGBA(image.Rect(0, 0, w, ht))
		for i := range w * ht {
			img.SetRGBA(i%w, i/w, color.RGBA{
				R: uint8(clampInt(comps[0][i], 255)),
				G: uint8(clampInt(comps[1][i], 255)),
				B: uint8(clampInt(comps[2][i], 255)),
				A: 255,
			})
		}
		return img, nil
	}
	if c0.precision <= 8 {
		img := image.NewGray(image.Rect(0, 0, w, ht))
		for i, v := range comps[0] {
			img.Pix[i] = uint8(clampInt(v, 255))
		}
		return img, nil
	}
	img := image.NewGray16(image.Rect(0, 0, w, ht))
	for i, v := range comps[0] {
		binary.BigEndian.PutUint16(img.Pix[2*i:], uint16(clampInt(v, 65535)))
	}
	return img, nil
}

// legacyConfig is the image.Config decodeLegacy would produce.
func legacyConfig(h *header) image.Config {
	m := color.Gray16Model
	switch {
	case len(h.comps) == 3:
		m = color.RGBAModel
	case h.comps[0].precision <= 8:
		m = color.GrayModel
	}
	return image.Config{ColorModel: m, Width: h.width(), Height: h.height()}
}

func clampInt(v, hi int) int { return max(0, min(v, hi)) }

// legacyInverseRCT is the reversible colour transform inverse, in place
// (Y, Cb, Cr -> R, G, B).
func legacyInverseRCT(y, cb, cr []int) {
	for i := range y {
		g := y[i] - (cb[i]+cr[i])>>2
		y[i], cb[i], cr[i] = cr[i]+g, g, cb[i]+g
	}
}

// legacyInverseMultiLevel inverts the legacy multi-level DWT: rows-then-
// columns forward order, levels stopping once a region dimension is < 2.
func legacyInverseMultiLevel(data []int, width, height, levels int) {
	dims := make([][2]int, levels+1)
	dims[0] = [2]int{width, height}
	for i := 1; i <= levels; i++ {
		dims[i] = [2]int{(dims[i-1][0] + 1) / 2, (dims[i-1][1] + 1) / 2}
	}
	for l := levels - 1; l >= 0; l-- {
		w, h := dims[l][0], dims[l][1]
		if w < 2 || h < 2 {
			continue
		}
		col := make([]int, h)
		for x := range w {
			for y := range h {
				col[y] = data[y*width+x]
			}
			legacyInverse1D(col)
			for y := range h {
				data[y*width+x] = col[y]
			}
		}
		for y := range h {
			legacyInverse1D(data[y*width : y*width+w])
		}
	}
}

// legacyInverse1D is the legacy 1-D 5/3 inverse with truncating division.
func legacyInverse1D(s []int) {
	n := len(s)
	if n < 2 {
		return
	}
	half := (n + 1) / 2
	low := append([]int(nil), s[:half]...)
	high := append([]int(nil), s[half:]...)
	for i := range low {
		l := 0
		if i > 0 {
			l = high[i-1]
		} else if len(high) > 0 {
			l = high[0]
		}
		r := l
		if i < len(high) {
			r = high[i]
		}
		low[i] -= (l + r + 2) / 4
	}
	for i := range high {
		l, r := low[i], low[i]
		if i+1 < half {
			r = low[i+1]
		}
		high[i] += (l + r) / 2
	}
	for i, v := range low {
		s[2*i] = v
	}
	for i, v := range high {
		s[2*i+1] = v
	}
}
