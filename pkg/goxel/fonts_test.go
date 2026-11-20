package goxel

import (
	"encoding/binary"
	"math"
	"testing"

	"fyne.io/fyne/v2/test"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchUnitsPerEm(t *testing.T) {
	out, ok := patchUnitsPerEm(ubuntuLightTTF, 1.04)
	require.True(t, ok)
	require.Len(t, out, len(ubuntuLightTTF))
	// Ubuntu Light is a 1000-unit font; its head table is where the patch lands.
	tables := int(binary.BigEndian.Uint16(out[4:6]))
	for i := 0; i < tables; i++ {
		rec := 12 + 16*i
		if string(out[rec:rec+4]) == "head" {
			at := int(binary.BigEndian.Uint32(out[rec+8:])) + headUnitsPerEmOffset
			assert.Equal(t, uint16(1040), binary.BigEndian.Uint16(out[at:]))
		}
	}
	assert.NotEqual(t, ubuntuLightTTF, out)
}

func TestFractionalSizesAreSetAtTheirOwnWidth(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	a.Settings().SetTheme(NewTheme())

	src, px := faceFor(13)
	assert.Nil(t, src)
	assert.Equal(t, float32(13), px)
	src, px = faceFor(13.5)
	assert.NotNil(t, src)
	assert.Equal(t, float32(14), px)

	// "Composite projection" is 129.7 px wide at 13.5 in egui: the sum of its
	// advances over a 1000-unit em. Fyne alone would set it at 14 (134.4).
	got := float64(measure("Composite projection", 13.5).Width)
	assert.InDelta(t, 129.7, got, 0.5)
	assert.InDelta(t, 124.9, float64(measure("Composite projection", 13).Width), 0.5)
	assert.False(t, math.IsNaN(got))
}
