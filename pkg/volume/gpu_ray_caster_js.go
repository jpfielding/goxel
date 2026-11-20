//go:build js

package volume

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"log/slog"
	"math"
	"sync"
	"syscall/js"
	"unsafe"
)

// maxWebVoxels caps the voxel count uploaded to WebGL2. RGBA16F costs 8 bytes
// per voxel, so larger volumes are halved on each axis before upload.
const maxWebVoxels = 32 << 20

// WebGL2 enums (from the WebGL2 specification).
const (
	glTexture2D         = 0x0DE1
	glTexture3D         = 0x806F
	glTexture0          = 0x84C0
	glTexture1          = 0x84C1
	glTextureMinFilter  = 0x2801
	glTextureMagFilter  = 0x2800
	glTextureWrapS      = 0x2802
	glTextureWrapT      = 0x2803
	glTextureWrapR      = 0x8072
	glLinear            = 0x2601
	glClampToEdge       = 0x812F
	glRGBA              = 0x1908
	glRGBA8             = 0x8058
	glRGBA16F           = 0x881A
	glUnsignedByte      = 0x1401
	glHalfFloat         = 0x140B
	glFloat             = 0x1406
	glArrayBuffer       = 0x8892
	glStaticDraw        = 0x88E4
	glFramebuffer       = 0x8D40
	glColorAttachment0  = 0x8CE0
	glFramebufferOK     = 0x8CD5
	glColorBufferBit    = 0x4000
	glBlend             = 0x0BE2
	glOne               = 1
	glOneMinusSrcAlpha  = 0x0303
	glTriangles         = 0x0004
	glVertexShader      = 0x8B31
	glFragmentShader    = 0x8B30
	glCompileStatus     = 0x8B81
	glLinkStatus        = 0x8B82
	glUnpackAlignment   = 0x0CF5
	glUnpackFlipY       = 0x9240
	glUnpackPremultiply = 0x9241
)

const webVertexShaderSource = `#version 300 es
layout (location = 0) in vec2 aPos;
out vec2 TexCoord;
void main() {
    gl_Position = vec4(aPos, 0.0, 1.0);
    TexCoord = (aPos + 1.0) * 0.5;
}
`

// ActiveVolume represents a single volume loaded into the renderer.
type ActiveVolume struct {
	Data             []uint16
	DimX, DimY, DimZ int
	Color            color.RGBA // Tint color (RGB)
	Texture          js.Value   // WebGL texture, or undefined until uploaded

	WindowLevel      float64
	WindowWidth      float64
	RescaleIntercept float64
	AlphaScale       float64
}

// FindingBox represents a wireframe bounding box. The WebGL2 back end stores
// findings but does not draw them yet.
type FindingBox struct {
	Name                      string
	ClassID                   int
	BBox                      BoundingBox3D
	Color                     color.RGBA
	RefDimX, RefDimY, RefDimZ int
}

// GPUVolumeRenderer ray casts volumes with WebGL2 on a private, never-attached
// canvas and reads the result back into an *image.RGBA, mirroring the desktop
// OpenGL renderer. Go's wasm port is single-threaded, so GL calls run inline
// in Render instead of on a worker goroutine.
type GPUVolumeRenderer struct {
	volumes  []*ActiveVolume
	findings []*FindingBox

	rotationX, rotationY float64
	zoom                 float64

	alphaScale        float64
	scaleY            float64
	scaleZ            float64
	stepSize          float64
	stepVoxels        float64 // when above zero, the step in voxels (see rayStep); else stepSize
	densityThreshold  float64
	ambientIntensity  float64
	diffuseIntensity  float64
	specularIntensity float64
	depthCue          float64

	clipPlane  [4]float32
	enableClip bool

	panX, panY float64

	renderWidth, renderHeight int
	wantWidth, wantHeight     int
	bgColor                   [3]float32

	gl          js.Value
	program     js.Value
	vao         js.Value
	vbo         js.Value
	fbo         js.Value
	colorBuffer js.Value
	transferTex js.Value
	uniforms    map[string]js.Value
	readBuf     js.Value // Uint8Array reused for readPixels
	pixels      []byte

	transferFunc     []color.RGBA
	transferFuncType TransferFunctionType

	rendered *image.RGBA

	mu                  sync.RWMutex
	needsRender         bool
	needsTransferUpload bool
	destroyed           bool
}

// NewGPUVolumeRenderer creates a WebGL2 renderer. It returns an error when the
// browser has no WebGL2 or the shaders fail to build; callers then fall back
// to the CPU ray caster.
func NewGPUVolumeRenderer(width, height int) (gr *GPUVolumeRenderer, err error) {
	defer func() {
		if r := recover(); r != nil {
			gr, err = nil, fmt.Errorf("webgl2 init panic: %v", r)
		}
	}()

	gr = &GPUVolumeRenderer{
		rotationX:         0.3,
		rotationY:         0.5,
		zoom:              1.0,
		alphaScale:        DefaultOpacity,
		scaleY:            1.0,
		scaleZ:            1.0,
		stepSize:          0.001,
		densityThreshold:  DefaultDensity,
		ambientIntensity:  DefaultAmbient,
		diffuseIntensity:  DefaultDiffuse,
		specularIntensity: DefaultSpecular,
		depthCue:          DefaultDepthCue,
		renderWidth:       width,
		renderHeight:      height,
		wantWidth:         width,
		wantHeight:        height,
		needsRender:       true,
		rendered:          image.NewRGBA(image.Rect(0, 0, width, height)),
		uniforms:          map[string]js.Value{},
	}
	cfg := GetConfig()
	gr.bgColor = [3]float32{
		float32(cfg.BackgroundColor[0]) / 255.0,
		float32(cfg.BackgroundColor[1]) / 255.0,
		float32(cfg.BackgroundColor[2]) / 255.0,
	}
	gr.createTransferFunction()

	canvas := js.Global().Get("document").Call("createElement", "canvas")
	canvas.Set("width", width)
	canvas.Set("height", height)
	attrs := map[string]any{"antialias": false, "depth": false, "stencil": false, "preserveDrawingBuffer": false, "premultipliedAlpha": false}
	gl := canvas.Call("getContext", "webgl2", attrs)
	if gl.IsNull() || gl.IsUndefined() {
		return nil, errors.New("webgl2 context unavailable")
	}
	gr.gl = gl
	if err := gr.initGLResources(); err != nil {
		return nil, err
	}
	return gr, nil
}

func (gr *GPUVolumeRenderer) initGLResources() error {
	gl := gr.gl
	vs, err := gr.compile(glVertexShader, webVertexShaderSource)
	if err != nil {
		return fmt.Errorf("vertex shader: %w", err)
	}
	fs, err := gr.compile(glFragmentShader, webFragmentHeader+volumeFragmentBody)
	if err != nil {
		return fmt.Errorf("fragment shader: %w", err)
	}
	gr.program = gl.Call("createProgram")
	gl.Call("attachShader", gr.program, vs)
	gl.Call("attachShader", gr.program, fs)
	gl.Call("linkProgram", gr.program)
	if !gl.Call("getProgramParameter", gr.program, glLinkStatus).Bool() {
		return fmt.Errorf("link: %s", gl.Call("getProgramInfoLog", gr.program).String())
	}
	gl.Call("deleteShader", vs)
	gl.Call("deleteShader", fs)

	// Full-screen quad (two triangles), same as desktop.
	verts := js.Global().Get("Float32Array").New(js.ValueOf([]any{
		-1.0, -1.0, 1.0, -1.0, -1.0, 1.0,
		1.0, -1.0, 1.0, 1.0, -1.0, 1.0,
	}))
	gr.vao = gl.Call("createVertexArray")
	gl.Call("bindVertexArray", gr.vao)
	gr.vbo = gl.Call("createBuffer")
	gl.Call("bindBuffer", glArrayBuffer, gr.vbo)
	gl.Call("bufferData", glArrayBuffer, verts, glStaticDraw)
	gl.Call("vertexAttribPointer", 0, 2, glFloat, false, 8, 0)
	gl.Call("enableVertexAttribArray", 0)

	gr.colorBuffer = gl.Call("createTexture")
	gl.Call("bindTexture", glTexture2D, gr.colorBuffer)
	gl.Call("texStorage2D", glTexture2D, 1, glRGBA8, gr.renderWidth, gr.renderHeight)
	gr.fbo = gl.Call("createFramebuffer")
	gl.Call("bindFramebuffer", glFramebuffer, gr.fbo)
	gl.Call("framebufferTexture2D", glFramebuffer, glColorAttachment0, glTexture2D, gr.colorBuffer, 0)
	if st := gl.Call("checkFramebufferStatus", glFramebuffer).Int(); st != glFramebufferOK {
		return fmt.Errorf("framebuffer incomplete: 0x%x", st)
	}

	gl.Call("pixelStorei", glUnpackAlignment, 1)
	gl.Call("pixelStorei", glUnpackFlipY, false)
	gl.Call("pixelStorei", glUnpackPremultiply, false)

	for _, name := range []string{"camPos", "camForward", "camRight", "camUp", "fov", "aspectRatio",
		"windowMin", "windowRange", "alphaScale", "stepSize", "scaleY", "scaleZ", "rescaleIntercept", "tintColor",
		"densityThreshold", "ambientIntensity", "diffuseIntensity", "specularIntensity", "clipPlane",
		"enableClip", "volumeTex", "transferTex", "depthCue"} {
		gr.uniforms[name] = gl.Call("getUniformLocation", gr.program, name)
	}

	gr.readBuf = js.Global().Get("Uint8Array").New(gr.renderWidth * gr.renderHeight * 4)
	gr.pixels = make([]byte, gr.renderWidth*gr.renderHeight*4)
	gr.uploadTransferFunction()
	return nil
}

// resizeTarget reallocates the colour attachment and the read-back buffers.
func (gr *GPUVolumeRenderer) resizeTarget(w, h int) {
	gl := gr.gl
	gl.Call("deleteTexture", gr.colorBuffer)
	gr.colorBuffer = gl.Call("createTexture")
	gl.Call("bindTexture", glTexture2D, gr.colorBuffer)
	gl.Call("texStorage2D", glTexture2D, 1, glRGBA8, w, h)
	gl.Call("bindFramebuffer", glFramebuffer, gr.fbo)
	gl.Call("framebufferTexture2D", glFramebuffer, glColorAttachment0, glTexture2D, gr.colorBuffer, 0)
	gr.readBuf = js.Global().Get("Uint8Array").New(w * h * 4)
	gr.pixels = make([]byte, w*h*4)
}

func (gr *GPUVolumeRenderer) compile(kind int, src string) (js.Value, error) {
	gl := gr.gl
	sh := gl.Call("createShader", kind)
	gl.Call("shaderSource", sh, src)
	gl.Call("compileShader", sh)
	if !gl.Call("getShaderParameter", sh, glCompileStatus).Bool() {
		return js.Undefined(), errors.New(gl.Call("getShaderInfoLog", sh).String())
	}
	return sh, nil
}

func (gr *GPUVolumeRenderer) createTransferFunction() {
	cfg := GetConfig()
	bands := DefaultColorBands()
	gr.transferFunc = CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", bands)
	if len(bands) > 0 && bands[0].IsTransparent {
		gr.densityThreshold = float64(bands[0].Threshold) / float64(bands[len(bands)-1].Threshold)
	}
}

// uploadTransferFunction uploads the table as an RGBA8 N x 1 texture. The
// source is already 8-bit, and RGBA32F + LINEAR would need an extension.
func (gr *GPUVolumeRenderer) uploadTransferFunction() {
	gl := gr.gl
	n := len(gr.transferFunc)
	buf := make([]byte, n*4)
	for i, c := range gr.transferFunc {
		buf[i*4], buf[i*4+1], buf[i*4+2], buf[i*4+3] = c.R, c.G, c.B, c.A
	}
	u8 := js.Global().Get("Uint8Array").New(len(buf))
	js.CopyBytesToJS(u8, buf)
	if gr.transferTex.IsUndefined() || gr.transferTex.IsNull() {
		gr.transferTex = gl.Call("createTexture")
	}
	gl.Call("bindTexture", glTexture2D, gr.transferTex)
	gl.Call("texImage2D", glTexture2D, 0, glRGBA8, n, 1, 0, glRGBA, glUnsignedByte, u8)
	gl.Call("texParameteri", glTexture2D, glTextureMinFilter, glLinear)
	gl.Call("texParameteri", glTexture2D, glTextureMagFilter, glLinear)
	gl.Call("texParameteri", glTexture2D, glTextureWrapS, glClampToEdge)
	gl.Call("texParameteri", glTexture2D, glTextureWrapT, glClampToEdge)
}

// uploadVolume packs density + central-difference gradient into RGBA16F and
// uploads one Z slice at a time so the transient buffer stays one slice big.
// R = density/65535; GBA = signed gradient/65535 (decoded by DECODE_GRAD(v) = v).
func (gr *GPUVolumeRenderer) uploadVolume(vol *ActiveVolume) {
	if len(vol.Data) == 0 {
		return
	}
	data, dimX, dimY, dimZ := vol.Data, vol.DimX, vol.DimY, vol.DimZ
	for dimX*dimY*dimZ > maxWebVoxels {
		slog.Info("Downsampling volume for WebGL2", "from", fmt.Sprintf("%dx%dx%d", dimX, dimY, dimZ))
		data, dimX, dimY, dimZ = downsample2x(data, dimX, dimY, dimZ)
	}

	gl := gr.gl
	if !vol.Texture.IsUndefined() && !vol.Texture.IsNull() {
		gl.Call("deleteTexture", vol.Texture)
	}
	tex := gl.Call("createTexture")
	gl.Call("bindTexture", glTexture3D, tex)
	gl.Call("texStorage3D", glTexture3D, 1, glRGBA16F, dimX, dimY, dimZ)
	gl.Call("texParameteri", glTexture3D, glTextureMinFilter, glLinear)
	gl.Call("texParameteri", glTexture3D, glTextureMagFilter, glLinear)
	gl.Call("texParameteri", glTexture3D, glTextureWrapS, glClampToEdge)
	gl.Call("texParameteri", glTexture3D, glTextureWrapT, glClampToEdge)
	gl.Call("texParameteri", glTexture3D, glTextureWrapR, glClampToEdge)

	sliceSize := dimX * dimY
	half := make([]uint16, sliceSize*4)
	halfBytes := unsafe.Slice((*byte)(unsafe.Pointer(&half[0])), len(half)*2)
	u8 := js.Global().Get("Uint8Array").New(len(halfBytes))
	u16 := js.Global().Get("Uint16Array").New(u8.Get("buffer"))
	const inv = 1.0 / 65535.0
	for z := 0; z < dimZ; z++ {
		for y := 0; y < dimY; y++ {
			for x := 0; x < dimX; x++ {
				idx := z*sliceSize + y*dimX + x
				var dx, dy, dz float32
				if x > 0 && x < dimX-1 {
					dx = float32(data[idx+1]) - float32(data[idx-1])
				}
				if y > 0 && y < dimY-1 {
					dy = float32(data[idx+dimX]) - float32(data[idx-dimX])
				}
				if z > 0 && z < dimZ-1 {
					dz = float32(data[idx+sliceSize]) - float32(data[idx-sliceSize])
				}
				o := (y*dimX + x) * 4
				half[o] = float16Bits(float32(data[idx]) * inv)
				half[o+1] = float16Bits(dx * inv)
				half[o+2] = float16Bits(dy * inv)
				half[o+3] = float16Bits(dz * inv)
			}
		}
		js.CopyBytesToJS(u8, halfBytes)
		gl.Call("texSubImage3D", glTexture3D, 0, 0, 0, z, dimX, dimY, 1, glRGBA, glHalfFloat, u16)
	}
	vol.Texture = tex
}

// SetColorOpacityPreset sets the transfer function from a named config preset.
func (gr *GPUVolumeRenderer) SetColorOpacityPreset(presetName string) {
	cfg := GetConfig()
	if cfg == nil {
		return
	}
	if _, ok := cfg.ColorMaps[presetName]; !ok {
		slog.Warn("Unknown color preset", "name", presetName)
		return
	}
	gr.mu.Lock()
	gr.transferFunc = CreateTransferFunctionFromConfig(cfg, presetName, 30000)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetRendered returns the last rendered image.
func (gr *GPUVolumeRenderer) GetRendered() image.Image {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.rendered
}

// ForceRender marks the image dirty.
func (gr *GPUVolumeRenderer) ForceRender() {
	gr.mu.Lock()
	gr.needsRender = true
	gr.mu.Unlock()
}

// Destroy releases WebGL resources.
func (gr *GPUVolumeRenderer) Destroy() {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	if gr.destroyed {
		return
	}
	gl := gr.gl
	for _, vol := range gr.volumes {
		if !vol.Texture.IsUndefined() {
			gl.Call("deleteTexture", vol.Texture)
		}
	}
	for _, v := range []js.Value{gr.transferTex, gr.colorBuffer} {
		if !v.IsUndefined() {
			gl.Call("deleteTexture", v)
		}
	}
	gl.Call("deleteFramebuffer", gr.fbo)
	gl.Call("deleteBuffer", gr.vbo)
	gl.Call("deleteVertexArray", gr.vao)
	gl.Call("deleteProgram", gr.program)
	gr.destroyed = true
}

// SetTransferFunction sets the transfer function type.
func (gr *GPUVolumeRenderer) SetTransferFunction(tfType TransferFunctionType) {
	gr.mu.Lock()
	gr.transferFuncType = tfType
	gr.transferFunc = CreateTransferFunction(tfType)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetAlphaScale sets the global opacity scale.
func (gr *GPUVolumeRenderer) SetAlphaScale(alpha float64) {
	gr.mu.Lock()
	gr.alphaScale = alpha
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetBackgroundColor sets the clear colour.
func (gr *GPUVolumeRenderer) SetBackgroundColor(r, g, b, a float64) {
	gr.mu.Lock()
	gr.bgColor = [3]float32{float32(r), float32(g), float32(b)}
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetMaterialThresholds rebuilds the DEFAULT map with two boundaries.
func (gr *GPUVolumeRenderer) SetMaterialThresholds(t1, t2 int) {
	gr.mu.Lock()
	gr.transferFunc = CreateTransferFunctionDynamic(GetConfig(), "DEFAULT", 30000, t1, t2)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetMaterialBands rebuilds the transfer function from colour bands.
func (gr *GPUVolumeRenderer) SetMaterialBands(bands []ColorBand) {
	gr.mu.Lock()
	gr.transferFunc = CreateTransferFunctionFromBandsWithGradient(GetConfig(), "DEFAULT", bands)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetClippingPlane sets the plane; dot(pos, n) + d < 0 is clipped.
func (gr *GPUVolumeRenderer) SetClippingPlane(nx, ny, nz, d float32, enable bool) {
	gr.mu.Lock()
	gr.clipPlane = [4]float32{nx, ny, nz, d}
	gr.enableClip = enable
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetDensityThreshold sets the minimum normalized density.
func (gr *GPUVolumeRenderer) SetDensityThreshold(threshold float64) {
	gr.mu.Lock()
	gr.densityThreshold = threshold
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetDensityThreshold returns the density threshold.
func (gr *GPUVolumeRenderer) GetDensityThreshold() float64 {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.densityThreshold
}

// SetLighting sets ambient, diffuse and specular intensities.
func (gr *GPUVolumeRenderer) SetLighting(ambient, diffuse, specular float64) {
	gr.mu.Lock()
	gr.ambientIntensity, gr.diffuseIntensity, gr.specularIntensity = ambient, diffuse, specular
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetDepthCue sets how much samples darken with ray depth (0 = off).
func (gr *GPUVolumeRenderer) SetDepthCue(v float64) {
	gr.mu.Lock()
	gr.depthCue = v
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetLighting returns ambient, diffuse and specular intensities.
func (gr *GPUVolumeRenderer) GetLighting() (ambient, diffuse, specular float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.ambientIntensity, gr.diffuseIntensity, gr.specularIntensity
}

// SetVolumeAlpha sets one volume's alpha scale.
func (gr *GPUVolumeRenderer) SetVolumeAlpha(index int, alpha float64) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	if index >= 0 && index < len(gr.volumes) {
		gr.volumes[index].AlphaScale = alpha
		gr.needsRender = true
	}
}

// SetScaleY sets the Y extent of the volume box relative to X.
func (gr *GPUVolumeRenderer) SetScaleY(scale float64) {
	gr.mu.Lock()
	gr.scaleY = scale
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetStepSize sets the ray step in box units (X is 1.0).
func (gr *GPUVolumeRenderer) SetStepSize(step float64) {
	gr.mu.Lock()
	gr.stepSize = step
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetStepVoxels sets the ray step as a fraction of the smallest voxel, as roxel
// does: the step shrinks as the camera backs off, so a thin shell is still
// sampled from far away.
func (gr *GPUVolumeRenderer) SetStepVoxels(n float64) {
	gr.mu.Lock()
	gr.stepVoxels = n
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetRenderSize asks for a new render target size, applied before the next frame.
func (gr *GPUVolumeRenderer) SetRenderSize(width, height int) {
	width, height = clampRenderSize(width), clampRenderSize(height)
	gr.mu.Lock()
	if width != gr.wantWidth || height != gr.wantHeight {
		gr.wantWidth, gr.wantHeight = width, height
		gr.needsRender = true
	}
	gr.mu.Unlock()
}

// RenderSize returns the size of the last rendered image.
func (gr *GPUVolumeRenderer) RenderSize() (int, int) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.renderWidth, gr.renderHeight
}

// SetScaleZ sets the Z extent of the volume box.
func (gr *GPUVolumeRenderer) SetScaleZ(scale float64) {
	gr.mu.Lock()
	gr.scaleZ = scale
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetPan sets the screen-space pan offset.
func (gr *GPUVolumeRenderer) SetPan(px, py float64) {
	gr.mu.Lock()
	gr.panX, gr.panY = px, py
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetPan returns the pan offset.
func (gr *GPUVolumeRenderer) GetPan() (float64, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.panX, gr.panY
}

// AdjustPan adds to the pan offset.
func (gr *GPUVolumeRenderer) AdjustPan(dx, dy float64) {
	gr.mu.Lock()
	gr.panX += dx
	gr.panY += dy
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetRotation sets the orbit angles.
func (gr *GPUVolumeRenderer) SetRotation(rotX, rotY float64) {
	gr.mu.Lock()
	gr.rotationX, gr.rotationY = rotX, rotY
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetZoom sets the camera distance.
func (gr *GPUVolumeRenderer) SetZoom(zoom float64) {
	gr.mu.Lock()
	gr.zoom = zoom
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetRotation returns the orbit angles.
func (gr *GPUVolumeRenderer) GetRotation() (float64, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.rotationX, gr.rotationY
}

// GetZoom returns the camera distance.
func (gr *GPUVolumeRenderer) GetZoom() float64 {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.zoom
}

// GetAlphaScale returns the global opacity scale.
func (gr *GPUVolumeRenderer) GetAlphaScale() float64 {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.alphaScale
}

// ClearVolumes removes all volumes and their textures.
func (gr *GPUVolumeRenderer) ClearVolumes() {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	for _, vol := range gr.volumes {
		if !vol.Texture.IsUndefined() {
			gr.gl.Call("deleteTexture", vol.Texture)
		}
	}
	gr.volumes = nil
	gr.panX, gr.panY = 0, 0
	gr.needsRender = true
}

// SetWindowLevel sets the window level on every volume.
func (gr *GPUVolumeRenderer) SetWindowLevel(level float64) {
	gr.mu.Lock()
	for _, vol := range gr.volumes {
		vol.WindowLevel = level
	}
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetWindowWidth sets the window width on every volume.
func (gr *GPUVolumeRenderer) SetWindowWidth(width float64) {
	gr.mu.Lock()
	for _, vol := range gr.volumes {
		vol.WindowWidth = width
	}
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetRescaleIntercept sets the rescale intercept on every volume.
func (gr *GPUVolumeRenderer) SetRescaleIntercept(intercept float64) {
	gr.mu.Lock()
	for _, vol := range gr.volumes {
		vol.RescaleIntercept = intercept
	}
	gr.needsRender = true
	gr.mu.Unlock()
}

// Params returns a snapshot of render parameters.
func (gr *GPUVolumeRenderer) Params() RenderParams {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	p := RenderParams{
		AlphaScale:        gr.alphaScale,
		ScaleY:            gr.scaleY,
		ScaleZ:            gr.scaleZ,
		StepSize:          gr.stepSize,
		DensityThreshold:  gr.densityThreshold,
		AmbientIntensity:  gr.ambientIntensity,
		DiffuseIntensity:  gr.diffuseIntensity,
		SpecularIntensity: gr.specularIntensity,
	}
	if len(gr.volumes) > 0 {
		p.WindowLevel = gr.volumes[0].WindowLevel
		p.WindowWidth = gr.volumes[0].WindowWidth
		p.RescaleIntercept = gr.volumes[0].RescaleIntercept
	}
	return p
}

// SetParams updates render parameters in a single lock.
func (gr *GPUVolumeRenderer) SetParams(p RenderParams) {
	gr.mu.Lock()
	gr.alphaScale = p.AlphaScale
	if p.ScaleY > 0 {
		gr.scaleY = p.ScaleY
	}
	gr.scaleZ = p.ScaleZ
	gr.stepSize = p.StepSize
	gr.densityThreshold = p.DensityThreshold
	gr.ambientIntensity = p.AmbientIntensity
	gr.diffuseIntensity = p.DiffuseIntensity
	gr.specularIntensity = p.SpecularIntensity
	for _, vol := range gr.volumes {
		vol.WindowLevel = p.WindowLevel
		vol.WindowWidth = p.WindowWidth
		vol.RescaleIntercept = p.RescaleIntercept
		vol.AlphaScale = p.AlphaScale
	}
	gr.needsRender = true
	gr.mu.Unlock()
}

// AddVolume adds a volume from frame data.
func (gr *GPUVolumeRenderer) AddVolume(pd *PixelData, imageRows, imageCols int, pixelRep int, tint color.RGBA, wl, ww, intercept, alphaScale float64) error {
	if pd == nil || len(pd.Frames) == 0 || imageRows <= 0 || imageCols <= 0 {
		return nil
	}
	dimX, dimY, dimZ := imageCols, imageRows, len(pd.Frames)
	data := make([]uint16, dimX*dimY*dimZ)
	signed := pixelRep == 1
	for z, f := range pd.Frames {
		off := z * dimX * dimY
		if pd.IsEncapsulated {
			for i, val := range decodeJPEGFrame(f.CompressedData, dimX, dimY, signed) {
				if i < dimX*dimY && val > 0 {
					data[off+i] = uint16(val)
				}
			}
		} else if len(f.Data) > 0 {
			copy(data[off:], f.Data)
		}
	}
	gr.mu.Lock()
	gr.volumes = append(gr.volumes, &ActiveVolume{
		Data: data, DimX: dimX, DimY: dimY, DimZ: dimZ, Color: tint,
		WindowLevel: wl, WindowWidth: ww, RescaleIntercept: intercept, AlphaScale: alphaScale,
	})
	gr.needsRender = true
	gr.mu.Unlock()
	return nil
}

// AddVolumeFromData adds raw volume data (copied).
func (gr *GPUVolumeRenderer) AddVolumeFromData(data []uint16, dimX, dimY, dimZ int, tint color.RGBA, wl, ww, intercept, alphaScale float64) {
	vol := &ActiveVolume{
		Data: append([]uint16(nil), data...), DimX: dimX, DimY: dimY, DimZ: dimZ, Color: tint,
		WindowLevel: wl, WindowWidth: ww, RescaleIntercept: intercept, AlphaScale: alphaScale,
	}
	gr.mu.Lock()
	gr.volumes = append(gr.volumes, vol)
	gr.needsRender = true
	gr.mu.Unlock()
}

// Render ray casts all volumes when something changed and returns the image.
func (gr *GPUVolumeRenderer) Render() (result *image.RGBA) {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in WebGL2 render", "err", r)
			result = nil
		}
	}()

	gr.mu.Lock()
	if gr.destroyed {
		gr.mu.Unlock()
		return nil
	}
	if !gr.needsRender {
		r := gr.rendered
		gr.mu.Unlock()
		return r
	}
	gr.needsRender = false
	needsTF := gr.needsTransferUpload
	gr.needsTransferUpload = false
	rotX, rotY, zoom := gr.rotationX, gr.rotationY, gr.zoom
	scaleY, scaleZ := float32(gr.scaleY), float32(gr.scaleZ)
	stepSize, stepVoxels := gr.stepSize, gr.stepVoxels
	resize := gr.wantWidth != gr.renderWidth || gr.wantHeight != gr.renderHeight
	if resize {
		gr.renderWidth, gr.renderHeight = gr.wantWidth, gr.wantHeight
		gr.rendered = image.NewRGBA(image.Rect(0, 0, gr.renderWidth, gr.renderHeight))
	}
	panX, panY := float32(gr.panX), float32(gr.panY)
	alpha := float32(gr.alphaScale)
	density := float32(gr.densityThreshold)
	amb, dif, spec := float32(gr.ambientIntensity), float32(gr.diffuseIntensity), float32(gr.specularIntensity)
	depthCue := float32(gr.depthCue)
	clip, enableClip := gr.clipPlane, gr.enableClip
	bg := gr.bgColor
	vols := append([]*ActiveVolume(nil), gr.volumes...)
	gr.mu.Unlock()

	gl := gr.gl
	w, h := gr.renderWidth, gr.renderHeight
	if resize {
		gr.resizeTarget(w, h)
	}
	if needsTF {
		gr.uploadTransferFunction()
	}
	for _, vol := range vols {
		if vol.Texture.IsUndefined() {
			gr.uploadVolume(vol)
		}
	}

	gl.Call("bindFramebuffer", glFramebuffer, gr.fbo)
	gl.Call("viewport", 0, 0, w, h)
	gl.Call("clearColor", bg[0], bg[1], bg[2], 1.0)
	gl.Call("clear", glColorBufferBit)
	gl.Call("useProgram", gr.program)

	// Orbit camera; identical math to the desktop renderer.
	sinAz, cosAz := float32(math.Sin(rotX)), float32(math.Cos(rotX))
	sinEl, cosEl := float32(math.Sin(rotY)), float32(math.Cos(rotY))
	cx, cy, cz := float32(0.5)+panX, scaleY*0.5+panY, scaleZ*0.5
	camX := sinAz*cosEl*float32(zoom) + cx
	camY := sinEl*float32(zoom) + cy
	camZ := cosAz*cosEl*float32(zoom) + cz
	fx, fy, fz := cx-camX, cy-camY, cz-camZ
	fl := float32(math.Sqrt(float64(fx*fx + fy*fy + fz*fz)))
	fx, fy, fz = fx/fl, fy/fl, fz/fl
	rx, ry, rz := cosAz, float32(0), -sinAz
	ux, uy, uz := fy*rz-fz*ry, fz*rx-fx*rz, fx*ry-fy*rx
	if ul := float32(math.Sqrt(float64(ux*ux + uy*uy + uz*uz))); ul > 0.001 {
		ux, uy, uz = ux/ul, uy/ul, uz/ul
	}

	u := gr.uniforms
	gl.Call("uniform3f", u["camPos"], camX, camY, camZ)
	gl.Call("uniform3f", u["camForward"], fx, fy, fz)
	gl.Call("uniform3f", u["camRight"], rx, ry, rz)
	gl.Call("uniform3f", u["camUp"], ux, uy, uz)
	gl.Call("uniform1f", u["fov"], 0.8)
	gl.Call("uniform1f", u["aspectRatio"], float32(w)/float32(h))
	var first *ActiveVolume
	if len(vols) > 0 {
		first = vols[0]
	}
	gl.Call("uniform1f", u["stepSize"], rayStep(stepSize, stepVoxels, first, float64(scaleY), float64(scaleZ), zoom))
	gl.Call("uniform1f", u["scaleY"], scaleY)
	gl.Call("uniform1f", u["scaleZ"], scaleZ)
	gl.Call("uniform1f", u["ambientIntensity"], amb)
	gl.Call("uniform1f", u["diffuseIntensity"], dif)
	gl.Call("uniform1f", u["specularIntensity"], spec)
	gl.Call("uniform1f", u["depthCue"], depthCue)
	gl.Call("uniform4f", u["clipPlane"], clip[0], clip[1], clip[2], clip[3])
	clipFlag := 0
	if enableClip {
		clipFlag = 1
	}
	gl.Call("uniform1i", u["enableClip"], clipFlag)

	gl.Call("activeTexture", glTexture1)
	gl.Call("bindTexture", glTexture2D, gr.transferTex)
	gl.Call("uniform1i", u["transferTex"], 1)

	gl.Call("enable", glBlend)
	gl.Call("blendFunc", glOne, glOneMinusSrcAlpha) // premultiplied, as on desktop
	gl.Call("bindVertexArray", gr.vao)
	for _, vol := range vols {
		if vol.Texture.IsUndefined() {
			continue
		}
		wl, ww := float32(vol.WindowLevel), float32(vol.WindowWidth)
		gl.Call("uniform1f", u["windowMin"], wl-ww/2)
		gl.Call("uniform1f", u["windowRange"], ww)
		gl.Call("uniform1f", u["alphaScale"], alpha*float32(vol.AlphaScale))
		gl.Call("uniform1f", u["rescaleIntercept"], float32(vol.RescaleIntercept))
		gl.Call("uniform1f", u["densityThreshold"], density)
		gl.Call("uniform4f", u["tintColor"],
			float32(vol.Color.R)/255, float32(vol.Color.G)/255, float32(vol.Color.B)/255, float32(vol.Color.A)/255)
		gl.Call("activeTexture", glTexture0)
		gl.Call("bindTexture", glTexture3D, vol.Texture)
		gl.Call("uniform1i", u["volumeTex"], 0)
		gl.Call("drawArrays", glTriangles, 0, 6)
	}
	gl.Call("disable", glBlend)

	gl.Call("readPixels", 0, 0, w, h, glRGBA, glUnsignedByte, gr.readBuf)
	js.CopyBytesToGo(gr.pixels, gr.readBuf)

	gr.mu.Lock()
	defer gr.mu.Unlock()
	stride := w * 4
	for y := 0; y < h; y++ {
		src := gr.pixels[y*stride : (y+1)*stride] // see the desktop renderer: no vertical flip
		dst := gr.rendered.Pix[y*gr.rendered.Stride : y*gr.rendered.Stride+stride]
		copy(dst, src)
		for i := 3; i < stride; i += 4 {
			dst[i] = 255
		}
	}
	return gr.rendered
}

// AddFinding stores a finding box. The WebGL2 back end does not draw it yet.
func (gr *GPUVolumeRenderer) AddFinding(name string, classID int, bbox BoundingBox3D, c color.RGBA, refDimX, refDimY, refDimZ int) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.findings = append(gr.findings, &FindingBox{Name: name, ClassID: classID, BBox: bbox, Color: c,
		RefDimX: refDimX, RefDimY: refDimY, RefDimZ: refDimZ})
	gr.needsRender = true
}

// ClearFindings removes all finding boxes.
func (gr *GPUVolumeRenderer) ClearFindings() {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.findings = nil
	gr.needsRender = true
}

// SetFindingMode is a no-op, as on desktop.
func (gr *GPUVolumeRenderer) SetFindingMode(mode int) {}
