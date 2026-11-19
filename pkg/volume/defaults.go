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
