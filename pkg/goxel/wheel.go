package goxel

import (
	"math"
	"runtime"

	"fyne.io/fyne/v2"
)

// Pointer speeds of the 3D view, as roxel has them. roxel counts physical
// pixels; Fyne reports logical ones, so callers multiply by pixelScale first.
const (
	// orbitPerPixel is the turn, in radians, of a one-pixel drag.
	orbitPerPixel = 0.005
	// panPerPixel is the pan of a one-pixel drag, as a fraction of the camera
	// distance.
	panPerPixel = 0.0015
	// zoomPerNotch is the zoom of one wheel notch: 10 %.
	zoomPerNotch = 0.1
	// maxNotches caps one scroll event, so no single event (a flick, a driver
	// that multiplies its deltas) can throw the camera to a zoom limit.
	maxNotches = 2
)

// pixelScale is the number of device pixels in one logical pixel on the canvas
// that shows o: Fyne's canvas scale times its texture scale (the Retina factor
// on macOS, where the canvas scale stays 1). It is 1 when o is on no canvas.
func pixelScale(o fyne.CanvasObject) float64 {
	c := fyne.CurrentApp().Driver().CanvasForObject(o)
	if c == nil {
		return 1
	}
	const span = 1000
	px, _ := c.PixelCoordinateForPosition(fyne.NewPos(span, 0))
	if px <= 0 {
		return 1
	}
	return float64(px) / span
}

// wheelNotches turns the vertical delta of a Fyne scroll event into wheel
// notches as roxel counts them: one per line of a line-stepped wheel, one per
// 100 device pixels of a pixel-stepped wheel or trackpad. Fyne scales the
// delta differently on each platform; goos is runtime.GOOS and scale is
// pixelScale.
func wheelNotches(goos string, dy, scale float64) float64 {
	switch goos {
	case "js":
		// The browser driver passes deltaY in CSS pixels, times 0.2, and times
		// ten more when it is 5 or over: a wheel notch of 100 arrives as 200,
		// a trackpad step of 3 as 0.6. Undo both.
		css := dy * 5
		if math.Abs(dy) >= 1 {
			css = dy / 2
		}
		return css * scale / 100
	case "darwin":
		// macOS deltas arrive as points (GLFW's tenths, times Fyne's ten).
		return dy * scale / 100
	default:
		// One line per notch, times Fyne's 25.
		return dy / 25
	}
}

// wheelZoom is the factor one scroll event of the given notches zooms by
// (above 1 zooms in): 10 % a notch, as in roxel, and at most maxNotches.
func wheelZoom(notches float64) float64 {
	return 1 + math.Max(-maxNotches, math.Min(maxNotches, notches))*zoomPerNotch
}

// scrollZoom is the zoom factor of a scroll event over o.
func scrollZoom(o fyne.CanvasObject, e *fyne.ScrollEvent) float64 {
	return wheelZoom(wheelNotches(runtime.GOOS, float64(e.Scrolled.DY), pixelScale(o)))
}
