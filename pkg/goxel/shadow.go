package goxel

import (
	"image"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
)

// egui's pop-up shadow, which roxel's tooltips and menus cast: black at 96 of
// 255, six across and ten down, its edge blurred over eight.
const (
	shadowDX    = 6
	shadowDY    = 10
	shadowBlur  = 8
	shadowAlpha = 96
)

// newPopupShadow is the shadow of a panel with the given corner radius. Fyne
// has no blurred shapes, so it is drawn once into a raster at the size it is
// shown; placeShadow puts it under its panel.
func newPopupShadow(radius float32) *canvas.Raster {
	var r *canvas.Raster
	r = canvas.NewRaster(func(w, h int) image.Image { return shadowImage(w, h, r.Size(), radius) })
	return r
}

// placeShadow lays the shadow out for a panel at pos of the given size: offset,
// and larger by the half of the blur that falls outside the panel's outline.
func placeShadow(shadow *canvas.Raster, pos fyne.Position, size fyne.Size) {
	shadow.Move(fyne.NewPos(pos.X+shadowDX-shadowBlur/2, pos.Y+shadowDY-shadowBlur/2))
	shadow.Resize(fyne.NewSize(size.Width+shadowBlur, size.Height+shadowBlur))
}

// shadowImage draws the shadow of a rounded panel into w x h pixels covering
// size: opacity falls off in a straight line across the blur, which is centred
// on the panel's outline, as egui feathers it.
func shadowImage(w, h int, size fyne.Size, radius float32) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	if w <= 0 || h <= 0 || size.Width <= 0 || size.Height <= 0 {
		return img
	}
	sx, sy := float64(size.Width)/float64(w), float64(size.Height)/float64(h)
	// The panel, centred in the raster, as half extents; egui widens the corner
	// radius by half the blur.
	hw, hh := float64(size.Width-shadowBlur)/2, float64(size.Height-shadowBlur)/2
	r := math.Min(float64(radius)+shadowBlur/2, math.Min(hw, hh))
	cx, cy := float64(size.Width)/2, float64(size.Height)/2
	for y := 0; y < h; y++ {
		py := math.Abs((float64(y)+0.5)*sy-cy) - (hh - r)
		row := img.Pix[y*img.Stride:]
		for x := 0; x < w; x++ {
			px := math.Abs((float64(x)+0.5)*sx-cx) - (hw - r)
			// Signed distance to the rounded panel: negative inside.
			d := math.Min(math.Max(px, py), 0) + math.Hypot(math.Max(px, 0), math.Max(py, 0)) - r
			a := 0.5 - d/shadowBlur
			if a <= 0 {
				continue
			}
			row[x*4+3] = uint8(math.Min(a, 1)*shadowAlpha + 0.5)
		}
	}
	return img
}
