package goxel

import (
	"image"
	"testing"

	"fyne.io/fyne/v2"
	"github.com/stretchr/testify/assert"
)

func TestWheelNotchesUndoFynesScaling(t *testing.T) {
	// Browser: a wheel notch is deltaY 100, which Fyne's driver turns into 200;
	// on a 2x screen roxel counts that as 200 device pixels, two notches.
	assert.InDelta(t, 1, wheelNotches("js", 200, 1), 1e-9)
	assert.InDelta(t, 2, wheelNotches("js", 200, 2), 1e-9)
	// A trackpad step of 3 CSS pixels is under the driver's cutoff: 0.6.
	assert.InDelta(t, 0.03, wheelNotches("js", 0.6, 1), 1e-9)
	assert.InDelta(t, -0.06, wheelNotches("js", -0.6, 2), 1e-9)
	// macOS: points, as roxel's trackpad deltas, times the Retina factor.
	assert.InDelta(t, 0.4, wheelNotches("darwin", 20, 2), 1e-9)
	// Elsewhere: 25 a notch.
	assert.InDelta(t, 1, wheelNotches("linux", 25, 2), 1e-9)
	assert.InDelta(t, -1, wheelNotches("windows", -25, 1), 1e-9)
}

func TestWheelZoomIsTenPercentANotchAndCapped(t *testing.T) {
	assert.InDelta(t, 1.1, wheelZoom(1), 1e-9)
	assert.InDelta(t, 0.9, wheelZoom(-1), 1e-9)
	assert.InDelta(t, 1.003, wheelZoom(0.03), 1e-9)
	// No single event reaches a zoom limit, however large its delta.
	assert.InDelta(t, 1.2, wheelZoom(50), 1e-9)
	assert.InDelta(t, 0.8, wheelZoom(-50), 1e-9)
}

func TestTipPositionFollowsEgui(t *testing.T) {
	win := fyne.NewSize(1000, 600)
	anchor, tip := fyne.NewSize(80, 28), fyne.NewSize(200, 40)
	// Under the control, left edges in line.
	assert.Equal(t, fyne.NewPos(100, 132), tipPosition(fyne.NewPos(100, 100), anchor, tip, win))
	// Right edges in line when it would leave the window.
	assert.Equal(t, fyne.NewPos(780, 132), tipPosition(fyne.NewPos(900, 100), anchor, tip, win))
	// Above the control when there is no room below.
	assert.Equal(t, fyne.NewPos(100, 516), tipPosition(fyne.NewPos(100, 560), anchor, tip, win))
}

func TestShadowFadesAcrossTheBlur(t *testing.T) {
	// A 100 x 60 panel: the raster is larger by the blur.
	img := shadowImage(108, 68, fyne.NewSize(108, 68), 6).(*image.RGBA)
	alpha := func(x, y int) uint8 { return img.RGBAAt(x, y).A }
	assert.Equal(t, uint8(shadowAlpha), alpha(54, 34), "full strength inside")
	assert.InDelta(t, shadowAlpha/2, float64(alpha(54, 4)), 8, "half way at the panel's edge")
	assert.Less(t, alpha(54, 0), uint8(10), "gone at the outside of the blur")
	assert.Equal(t, uint8(0), alpha(0, 0), "nothing beyond a rounded corner")
}

func TestSpinnerArcTurnsAndSwings(t *testing.T) {
	from, to := spinnerArc(0)
	assert.Equal(t, from, to, "no length at the start")
	from, to = spinnerArc(1.5) // sin(1.5) is nearly 1: nearly the full 240 degrees
	assert.InDelta(t, 239.4, float64(to-from), 0.1)
	assert.InDelta(t, 270, float64(from), 1e-3, "half a turn on from the right")
}
