package goxel

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLayoutFromSize(t *testing.T) {
	cases := []struct {
		w, h float32
		want layoutClass
	}{
		{1440, 900, layoutDesktop},
		{1100, 720, layoutDesktop},
		{393, 852, layoutPhonePortrait},
		{852, 393, layoutPhoneLandscape},
		{390, 300, layoutPhonePortrait}, // narrow wins over short
		{1024, 768, layoutDesktop},
	}
	for _, c := range cases {
		assert.Equalf(t, c.want, layoutFromSize(c.w, c.h), "%vx%v", c.w, c.h)
	}
}

func TestSlicePanelWidth(t *testing.T) {
	assert.InDelta(t, 336, slicePanelWidth(800), 0.01, "floor")
	assert.InDelta(t, 336, slicePanelWidth(1100), 0.01, "3D column keeps its minimum")
	assert.InDelta(t, 468, slicePanelWidth(1300), 0.01, "36% of the window")
	assert.InDelta(t, 518.4, slicePanelWidth(1440), 0.01)
	assert.InDelta(t, 520, slicePanelWidth(2560), 0.01, "cap")
	// Never squeeze the 3D column below its minimum while above the floor.
	assert.InDelta(t, 336, slicePanelWidth(1000), 0.01)
}

func TestShelfIsNarrow(t *testing.T) {
	assert.True(t, shelfIsNarrow(1100))
	assert.True(t, shelfIsNarrow(1440))
	assert.False(t, shelfIsNarrow(1600))
}

func TestCutawayPlane(t *testing.T) {
	cases := []struct {
		o          ViewOrientation
		nx, ny, nz float32
	}{
		{Axial, 0, 0, 1},
		{Coronal, 0, 1, 0},
		{Sagittal, 1, 0, 0},
	}
	for _, c := range cases {
		nx, ny, nz, d := cutawayPlane(c.o, 9, 20, false)
		assert.Equal(t, []float32{c.nx, c.ny, c.nz}, []float32{nx, ny, nz}, c.o.String())
		assert.InDelta(t, -0.475, d, 1e-6)

		rx, ry, rz, rd := cutawayPlane(c.o, 9, 20, true)
		assert.Equal(t, []float32{-c.nx, -c.ny, -c.nz}, []float32{rx, ry, rz}, c.o.String()+" reversed")
		assert.InDelta(t, 0.475, rd, 1e-6)
	}
	// A texel exactly at the slice centre sits on the plane.
	_, _, nz, d := cutawayPlane(Axial, 0, 1, false)
	assert.InDelta(t, 0, 0.5*nz+d, 1e-6)
}

func TestSelectTab(t *testing.T) {
	s := newUIState()
	assert.Equal(t, tabView3D, s.phoneTab)
	assert.True(t, s.sheetOpen)

	s.selectTab(tabView3D) // active view tab toggles its sheet
	assert.False(t, s.sheetOpen)
	s.selectTab(tabView3D)
	assert.True(t, s.sheetOpen)

	s.sheetOpen = false
	s.selectTab(tabSlice) // another tab opens with its controls
	assert.Equal(t, tabSlice, s.phoneTab)
	assert.True(t, s.sheetOpen)

	s.selectTab(tabStudy)
	s.selectTab(tabStudy) // pages have no sheet to toggle
	assert.Equal(t, tabStudy, s.phoneTab)
	assert.True(t, s.sheetOpen)
}

func TestExitFocus(t *testing.T) {
	s := newUIState()
	assert.False(t, s.exitFocus())
	s.focusVolume, s.focusSlice, s.focusAdjustOpen = true, true, true
	assert.True(t, s.exitFocus())
	assert.False(t, s.focusSlice, "slice focus leaves first")
	assert.True(t, s.focusVolume)
	assert.False(t, s.focusAdjustOpen)
	assert.True(t, s.exitFocus())
	assert.False(t, s.focusVolume)
}

func TestElideMiddle(t *testing.T) {
	assert.Equal(t, "short.dcm", elideMiddle("short.dcm", 20))
	assert.Equal(t, "abcd…6789", elideMiddle("abcdefghij0123456789", 9))
}

func TestFitDistance(t *testing.T) {
	// A unit cube seen straight on fills the height with a 6% margin: its
	// near face is half a unit closer than the centre.
	assert.InDelta(t, 0.5*1.06/shaderFov+0.5, fitDistance(1, 1, 0, 0, 1), 1e-9)
	// A wider viewport leaves the height fit alone; a narrower one needs more room.
	assert.InDelta(t, fitDistance(1, 1, 0, 0, 1), fitDistance(1, 1, 0, 0, 2), 1e-9)
	assert.Greater(t, fitDistance(1, 1, 0, 0, 0.5), fitDistance(1, 1, 0, 0, 1))
	// A slab seen along its thin side needs less distance than the cube.
	assert.Less(t, fitDistance(1, 0.2, 0, 0, 1.5), fitDistance(1, 1, 0, 0, 1.5))
}

func TestSlicePlaneCorners(t *testing.T) {
	c := slicePlaneCorners(Axial, 0, 2, 1, 0.5)
	assert.Equal(t, [3]float64{0, 0, 0.125}, c[0])
	assert.Equal(t, [3]float64{1, 1, 0.125}, c[2])
	c = slicePlaneCorners(Coronal, 1, 2, 1, 0.5)
	assert.Equal(t, [3]float64{1, 0.75, 0.5}, c[2])
	c = slicePlaneCorners(Coronal, 1, 2, 0.5, 2)
	assert.Equal(t, [3]float64{1, 0.375, 2}, c[2], "the plane sits inside the physical box")
	c = slicePlaneCorners(Sagittal, 1, 4, 1, 2)
	assert.Equal(t, [3]float64{0.375, 1, 2}, c[2])
}

func TestWindowRangeFor(t *testing.T) {
	r := windowRangeFor(238, 2524)
	assert.InDelta(t, -1024, r.centerLo, 1e-9)
	assert.InDelta(t, 1500, r.centerHi, 1e-9)
	assert.InDelta(t, 1, r.widthLo, 1e-9)
	assert.InDelta(t, 5048, r.widthHi, 1e-9)
	// The window sits mid-track, so the thumbs start centred.
	assert.InDelta(t, 0.5, (238-r.centerLo)/(r.centerHi-r.centerLo), 1e-9)
	assert.InDelta(t, 0.5, (2524-r.widthLo)/(r.widthHi-r.widthLo), 0.001)
	assert.Equal(t, 50.0, windowStep(5047))
	assert.Equal(t, 2.0, windowStep(399))
	assert.Equal(t, 1.0, windowStep(49))
}

func TestDefaultsFollowRoxel(t *testing.T) {
	s := newUIState()
	assert.Equal(t, paletteGrayscale, s.preset)
	assert.Equal(t, Coronal, s.orientation)
	assert.True(t, s.showSlicePlane)
	assert.InDelta(t, 0.25, s.opacity, 1e-9)
	assert.InDelta(t, 0.35, s.depthCue, 1e-9)
}
