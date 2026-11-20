package goxel

import (
	_ "embed"
	"encoding/binary"
	"fmt"
	"math"
	"sync"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// Ubuntu Light is egui's proportional face, so roxel's type and goxel's set
// at the same sizes read alike. It is distributed under the Ubuntu Font
// Licence 1.0; the licence text is fonts/UFL.txt.
//
//go:embed fonts/Ubuntu-Light.ttf
var ubuntuLightTTF []byte

var ubuntuLight = fyne.NewStaticResource("Ubuntu-Light.ttf", ubuntuLightTTF)

// Fyne's text shaper rounds a font size up to a whole number of pixels when it
// lays glyphs out, so text at roxel's 13.5, 14.5 or 12.5 would be set at 14, 15
// or 13: about 3.5 % wider than egui sets it. For a fractional size goxel asks
// for the next whole size in a copy of the face whose units-per-em is scaled by
// the same ratio, which gives the glyphs back their intended size and advance.
// The copy exists only in memory.
var (
	scaledFacesMu sync.Mutex
	scaledFaces   = map[float32]fyne.Resource{}
)

// headUnitsPerEmOffset is the offset of unitsPerEm in the font's 'head' table.
const headUnitsPerEmOffset = 18

// scaledFace returns the Ubuntu Light copy for a fractional size, or nil if the
// font's head table cannot be found.
func scaledFace(size float32) fyne.Resource {
	scaledFacesMu.Lock()
	defer scaledFacesMu.Unlock()
	if r, ok := scaledFaces[size]; ok {
		return r
	}
	var res fyne.Resource
	if data, ok := patchUnitsPerEm(ubuntuLightTTF, float64(math.Ceil(float64(size)))/float64(size)); ok {
		res = fyne.NewStaticResource(fmt.Sprintf("Ubuntu-Light@%.2f.ttf", size), data)
	}
	scaledFaces[size] = res
	return res
}

// patchUnitsPerEm returns a copy of a TrueType font with unitsPerEm multiplied
// by ratio, which shrinks every glyph and advance by 1/ratio.
func patchUnitsPerEm(ttf []byte, ratio float64) ([]byte, bool) {
	if len(ttf) < 12 {
		return nil, false
	}
	tables := int(binary.BigEndian.Uint16(ttf[4:6]))
	for i := 0; i < tables; i++ {
		rec := 12 + 16*i
		if rec+16 > len(ttf) {
			return nil, false
		}
		if string(ttf[rec:rec+4]) != "head" {
			continue
		}
		at := int(binary.BigEndian.Uint32(ttf[rec+8:])) + headUnitsPerEmOffset
		if at+2 > len(ttf) {
			return nil, false
		}
		out := append([]byte(nil), ttf...)
		upem := float64(binary.BigEndian.Uint16(out[at:]))
		binary.BigEndian.PutUint16(out[at:], uint16(math.Round(upem*ratio)))
		return out, true
	}
	return nil, false
}

// faceFor is the font source and pixel size to give a canvas.Text so that it is
// set at size (see above); a whole size needs neither.
func faceFor(size float32) (fyne.Resource, float32) {
	if size == float32(math.Trunc(float64(size))) {
		return nil, size
	}
	if r := scaledFace(size); r != nil {
		return r, float32(math.Ceil(float64(size)))
	}
	return nil, size
}

// setFace sets t's size, and the face that makes that size exact.
func setFace(t *canvas.Text, size float32) {
	t.FontSource, t.TextSize = faceFor(size)
}

// measure is the size of s set at size in the regular face, as setFace draws it.
func measure(s string, size float32) fyne.Size {
	src, px := faceFor(size)
	sz, _ := fyne.CurrentApp().Driver().RenderedTextSize(s, px, fyne.TextStyle{}, src)
	return sz
}
