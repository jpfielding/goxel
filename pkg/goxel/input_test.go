package goxel

import (
	"image"
	"image/color"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestShelfNarrowMeasuresTheColumnInside(t *testing.T) {
	// Not in focus the column is what the sidebar and slice panel leave, less
	// the shelf's padding at each side.
	assert.True(t, shelfNarrow(1100, false))
	assert.False(t, shelfNarrow(1600, false))
	// In focus the column is the whole window: 752 is the first width with
	// 720 inside the padding.
	assert.True(t, shelfNarrow(751, true))
	assert.False(t, shelfNarrow(752, true))
}

func TestCaptionPositionSplitsOnTheSeparator(t *testing.T) {
	assert.Equal(t, "Coronal 129 / 256", captionPosition("Coronal 129 / 256"+captionSep+"680 × 200 mm"))
	assert.Equal(t, "  ·  ", captionSep, "two spaces either side, as roxel")
}

func TestRotatedCWTurnsTheLeftEdgeIntoTheTop(t *testing.T) {
	src := image.NewGray(image.Rect(0, 0, 3, 2)) // 3 wide, 2 high
	src.SetGray(0, 0, color.Gray{10})            // top-left
	src.SetGray(2, 0, color.Gray{20})            // top-right
	src.SetGray(0, 1, color.Gray{30})            // bottom-left
	src.SetGray(2, 1, color.Gray{40})            // bottom-right
	r := rotatedCW{src}
	assert.Equal(t, image.Rect(0, 0, 2, 3), r.Bounds())
	gray := func(x, y int) uint8 { return color.GrayModel.Convert(r.At(x, y)).(color.Gray).Y }
	// Turned clockwise: the old bottom-left is the new top-left, the old
	// top-left the new top-right, the old top-right the new bottom-right.
	assert.Equal(t, uint8(30), gray(0, 0))
	assert.Equal(t, uint8(10), gray(1, 0))
	assert.Equal(t, uint8(20), gray(1, 2))
	assert.Equal(t, uint8(40), gray(0, 2))
}

func TestQualityStepsFollowRoxel(t *testing.T) {
	assert.Equal(t, 1.5, qualityFast.stepVoxels())
	assert.Equal(t, 1.0, qualityBalanced.stepVoxels())
	assert.Equal(t, 0.5, qualityHigh.stepVoxels())
}

func TestNewStateShowsFindingBoxesWithNoSelection(t *testing.T) {
	s := newUIState()
	assert.True(t, s.showFindingBoxes)
	assert.Equal(t, -1, s.selectedFinding)
}
