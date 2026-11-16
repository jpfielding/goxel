//go:build js

package volume

import (
	"errors"
	"image"
	"image/color"
)

// GPUVolumeRenderer is the browser counterpart of the desktop OpenGL renderer.
// This build has no WebGL2 back end yet: NewGPUVolumeRenderer always fails so
// callers fall back to the CPU ray caster.
type GPUVolumeRenderer struct{}

// NewGPUVolumeRenderer reports that no GPU renderer is available in the browser.
func NewGPUVolumeRenderer(width, height int) (*GPUVolumeRenderer, error) {
	return nil, errors.New("webgl2 renderer not implemented")
}

func (gr *GPUVolumeRenderer) SetColorOpacityPreset(presetName string) {}

func (gr *GPUVolumeRenderer) GetRendered() image.Image {
	return nil
}

func (gr *GPUVolumeRenderer) ForceRender() {}

func (gr *GPUVolumeRenderer) Destroy() {}

func (gr *GPUVolumeRenderer) SetTransferFunction(tfType TransferFunctionType) {}

func (gr *GPUVolumeRenderer) SetAlphaScale(alpha float64) {}

func (gr *GPUVolumeRenderer) SetBackgroundColor(r, g, b, a float64) {}

func (gr *GPUVolumeRenderer) SetMaterialThresholds(t1, t2 int) {}

func (gr *GPUVolumeRenderer) SetMaterialBands(bands []ColorBand) {}

func (gr *GPUVolumeRenderer) SetClippingPlane(nx, ny, nz, d float32, enable bool) {}

func (gr *GPUVolumeRenderer) SetDensityThreshold(threshold float64) {}

func (gr *GPUVolumeRenderer) GetDensityThreshold() float64 {
	return 0
}

func (gr *GPUVolumeRenderer) SetLighting(ambient, diffuse, specular float64) {}

func (gr *GPUVolumeRenderer) GetLighting() (ambient, diffuse, specular float64) { return }

func (gr *GPUVolumeRenderer) SetVolumeAlpha(index int, alpha float64) {}

func (gr *GPUVolumeRenderer) SetScaleZ(scale float64) {}

func (gr *GPUVolumeRenderer) SetPan(px, py float64) {}

func (gr *GPUVolumeRenderer) GetPan() (float64, float64) { return 0, 0 }

func (gr *GPUVolumeRenderer) AdjustPan(dx, dy float64) {}

func (gr *GPUVolumeRenderer) SetRotation(rotX, rotY float64) {}

func (gr *GPUVolumeRenderer) SetZoom(zoom float64) {}

func (gr *GPUVolumeRenderer) GetRotation() (float64, float64) { return 0, 0 }

func (gr *GPUVolumeRenderer) GetZoom() float64 {
	return 0
}

func (gr *GPUVolumeRenderer) GetAlphaScale() float64 {
	return 0
}

func (gr *GPUVolumeRenderer) ClearVolumes() {}

func (gr *GPUVolumeRenderer) SetWindowLevel(level float64) {}

func (gr *GPUVolumeRenderer) SetWindowWidth(width float64) {}

func (gr *GPUVolumeRenderer) SetRescaleIntercept(intercept float64) {}

func (gr *GPUVolumeRenderer) Params() RenderParams {
	return RenderParams{}
}

func (gr *GPUVolumeRenderer) SetParams(p RenderParams) {}

func (gr *GPUVolumeRenderer) AddVolume(pd *PixelData, imageRows, imageCols int, pixelRep int, tint color.RGBA, wl, ww, intercept, alphaScale float64) error {
	return nil
}

func (gr *GPUVolumeRenderer) AddVolumeFromData(data []uint16, dimX, dimY, dimZ int, tint color.RGBA, wl, ww, intercept, alphaScale float64) {
}

func (gr *GPUVolumeRenderer) Render() *image.RGBA {
	return nil
}

func (gr *GPUVolumeRenderer) AddFinding(name string, classID int, bbox BoundingBox3D, c color.RGBA, refDimX, refDimY, refDimZ int) {
}

func (gr *GPUVolumeRenderer) ClearFindings() {}

func (gr *GPUVolumeRenderer) SetFindingMode(mode int) {}
