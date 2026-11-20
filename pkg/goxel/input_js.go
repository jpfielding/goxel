//go:build js

package goxel

import (
	"syscall/js"

	"fyne.io/fyne/v2"
)

// Fyne's browser driver reports neither key modifiers nor touches. The page's
// shim (cmd/web/index.html) keeps them in window.goxel:
//
//	shift, ctrl  a modifier key is held (or a pinch is under way: ctrl)
//	pan          two fingers are down: a drag pans
//	pinch        the ratio of the last pinch step (pinchAt: when), 0 when taken
//	view         the 3D view's {x, y, w, h}, where two fingers may start a pan

// shim returns window.goxel, or undefined when the page has no shim.
func shim() js.Value { return js.Global().Get("goxel") }

func flag(name string) bool {
	g := shim()
	if g.IsUndefined() || g.IsNull() {
		return false
	}
	return g.Get(name).Truthy()
}

// panModifierDown reports whether a plain drag should pan the 3D view: Shift
// held, or two fingers down.
func panModifierDown() bool { return flag("shift") || flag("pan") }

// zoomModifierDown reports whether scrolling should zoom the 2D slice: Ctrl
// (or Cmd) held, or a pinch under way.
func zoomModifierDown() bool { return flag("ctrl") }

// takePinch returns the zoom ratio of the touch pinch step waiting to be
// applied (greater than 1 spreads), or 0, and clears it.
func takePinch() float64 {
	g := shim()
	if g.IsUndefined() || g.IsNull() {
		return 0
	}
	r := g.Get("pinch").Float()
	g.Set("pinch", 0)
	// A ratio nothing took (no zoomable view under the fingers) goes stale.
	if js.Global().Get("performance").Call("now").Float()-g.Get("pinchAt").Float() > 250 {
		return 0
	}
	return r
}

// publishTouchArea tells the shim where the 3D view is, in page pixels, so
// two fingers pan it only there.
func publishTouchArea(pos fyne.Position, size fyne.Size) {
	g := shim()
	if g.IsUndefined() || g.IsNull() {
		return
	}
	g.Set("view", map[string]any{
		"x": float64(pos.X), "y": float64(pos.Y), "w": float64(size.Width), "h": float64(size.Height),
	})
}
