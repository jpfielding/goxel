package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRayStepFollowsTheVoxelAndTheDistance(t *testing.T) {
	v := &ActiveVolume{DimX: 256, DimY: 256, DimZ: 128}
	// Half a voxel (the smallest, 0.6875/256) at distance 1.
	assert.InDelta(t, 0.6875/256*0.5, rayStep(0, 0.5, v, 0.6875, 0.4, 1), 1e-9)
	// Finer further away, and never outside 0.0005-0.02.
	assert.Less(t, rayStep(0, 0.5, v, 1, 1, 4), rayStep(0, 0.5, v, 1, 1, 1))
	assert.InDelta(t, 0.0005, rayStep(0, 0.001, v, 1, 1, 5), 1e-12)
	assert.InDelta(t, 0.02, rayStep(0, 1000, v, 1, 1, 1), 1e-12)
	// Without a voxel step the plain step grows with the distance.
	assert.InDelta(t, 0.002, rayStep(0.001, 0, v, 1, 1, 2), 1e-12)
}
