package volume

// Defaults for a freshly opened volume, shared by every renderer and by the
// UI so the controls start where the renderer is.
const (
	DefaultOpacity  = 0.25
	DefaultDensity  = 0.0
	DefaultAmbient  = 0.65
	DefaultDiffuse  = 0.8
	DefaultSpecular = 0.3
	DefaultDepthCue = 0.35
)

// clampRenderSize keeps a requested render target edge inside a range every
// GL implementation accepts.
func clampRenderSize(n int) int { return max(64, min(n, 4096)) }

// rayStep is the ray-march step in box units (X is 1). With voxels > 0 it is
// that many of the smallest voxel, divided by the camera distance (clamped to
// 0.5-5) and kept within 0.0005-0.02: as roxel steps, finer when the camera is
// far, so thin shells are not stepped over. Otherwise it is the plain step,
// scaled up with the distance.
func rayStep(plain, voxels float64, v *ActiveVolume, scaleY, scaleZ, distance float64) float64 {
	d := max(0.5, min(5, distance))
	if voxels <= 0 || v == nil || v.DimX <= 0 || v.DimY <= 0 || v.DimZ <= 0 {
		return plain * d
	}
	voxel := min(1/float64(v.DimX), min(scaleY/float64(v.DimY), scaleZ/float64(v.DimZ)))
	return max(0.0005, min(0.02, voxel*voxels/d))
}

// voxelDiff is the density gradient of the voxel at idx along one axis, per
// two voxels: the central difference inside the volume and, on the outermost
// layer, the one-sided difference doubled to the same scale. A zero there
// would leave that layer lit with the flat-region (ambient only) model and
// draw a dark line along the face of any shell that reaches the volume's edge.
// pos is the voxel's coordinate on the axis, n the axis length and stride the
// index step along it.
func voxelDiff[T ~uint16](data []T, idx, pos, n, stride int) float64 {
	switch {
	case n < 2:
		return 0
	case pos == 0:
		return 2 * (float64(data[idx+stride]) - float64(data[idx]))
	case pos == n-1:
		return 2 * (float64(data[idx]) - float64(data[idx-stride]))
	}
	return float64(data[idx+stride]) - float64(data[idx-stride])
}
