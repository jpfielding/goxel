//go:build !js

package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// modifiers reports the keyboard modifiers held now.
func modifiers() fyne.KeyModifier {
	if d, ok := fyne.CurrentApp().Driver().(desktop.Driver); ok {
		return d.CurrentKeyModifiers()
	}
	return 0
}

// panModifierDown reports whether a plain drag should pan the 3D view: Shift.
func panModifierDown() bool { return modifiers()&fyne.KeyModifierShift != 0 }

// zoomModifierDown reports whether scrolling should zoom the 2D slice: Ctrl
// (or Cmd on a Mac).
func zoomModifierDown() bool {
	return modifiers()&(fyne.KeyModifierControl|fyne.KeyModifierSuper) != 0
}

// takePinch returns the zoom ratio of a touch pinch waiting to be applied; the
// desktop has none.
func takePinch() float64 { return 0 }

// publishTouchArea tells the page where the 3D view is; the desktop needs no
// touch shim.
func publishTouchArea(fyne.Position, fyne.Size) {}
