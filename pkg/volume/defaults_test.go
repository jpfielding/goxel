package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestVoxelDiffHasNoZeroLayerAtTheEdges(t *testing.T) {
	// A ramp 0, 10, 20, 30 along x: every voxel, edges included, sees the same
	// slope (per two voxels).
	data := []uint16{0, 10, 20, 30}
	for x := range data {
		assert.Equal(t, 20.0, voxelDiff(data, x, x, len(data), 1), "x=%d", x)
	}
	// A step at the edge is not lost.
	assert.Equal(t, 2000.0, voxelDiff([]uint16{0, 1000, 1000}, 0, 0, 3, 1))
	// A single-voxel axis has no gradient.
	assert.Equal(t, 0.0, voxelDiff([]uint16{5}, 0, 0, 1, 1))
}
