package volume

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"log/slog"
	"math"
	"runtime"
	"strings"
	"sync"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/glfw/v3.3/glfw"
)

// extractPixelsFromImage extracts grayscale pixel values from an image.Image
func extractPixelsFromImage(img image.Image) []int {
	bounds := img.Bounds()
	width, height := bounds.Dx(), bounds.Dy()
	pixels := make([]int, width*height)

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			r, _, _, _ := img.At(x+bounds.Min.X, y+bounds.Min.Y).RGBA()
			// RGBA returns 16-bit values (0-65535), scale to int
			pixels[y*width+x] = int(r)
		}
	}
	return pixels
}

// decodeJPEGFrame decodes a standard JPEG compressed frame.
// Note: DICOM-specific formats (JPEG-LS, JPEG 2000, RLE) are handled by the dicom library.
func decodeJPEGFrame(data []byte, width, height int, signed bool) []int {
	// Standard JPEG decoding
	if img, err := jpeg.Decode(bytes.NewReader(data)); err == nil {
		return extractPixelsFromImage(img)
	}
	return nil
}

const vertexShaderSource = `
#version 330 core
layout (location = 0) in vec2 aPos;
out vec2 TexCoord;

void main() {
    gl_Position = vec4(aPos, 0.0, 1.0);
    TexCoord = (aPos + 1.0) * 0.5;
}
` + "\x00"

const fragmentShaderSource = `
#version 330 core
in vec2 TexCoord;
out vec4 FragColor;

uniform sampler3D volumeTex;
uniform sampler1D transferTex;
uniform vec3 camPos;
uniform vec3 camForward;
uniform vec3 camRight;
uniform vec3 camUp;
uniform float fov;
uniform float aspectRatio;
uniform float windowMin;
uniform float windowRange;
uniform float alphaScale;
uniform float stepSize;
uniform float scaleZ;
uniform float rescaleIntercept;
uniform vec4 tintColor;
uniform float densityThreshold;
uniform float ambientIntensity;
uniform float diffuseIntensity;
uniform float specularIntensity;
uniform vec4 clipPlane;
uniform bool enableClip;

// Ray-box intersection for box [0,1]x[0,1]x[0,scaleZ]
vec2 intersectBox(vec3 origin, vec3 dir) {
    vec3 boxSize = vec3(1.0, 1.0, scaleZ);
    vec3 invDir = 1.0 / dir;
    vec3 t1 = (vec3(0.0) - origin) * invDir;
    vec3 t2 = (boxSize - origin) * invDir;
    vec3 tMin = min(t1, t2);
    vec3 tMax = max(t1, t2);
    float tNear = max(max(tMin.x, tMin.y), tMin.z);
    float tFar = min(min(tMax.x, tMax.y), tMax.z);
    return vec2(tNear, tFar);
}

void main() {
    vec2 uv = TexCoord * 2.0 - 1.0;
    vec3 rayDir = normalize(camForward + camRight * uv.x * fov * aspectRatio + camUp * uv.y * fov);

    // Intersect with volume bounding box
    vec2 tHit = intersectBox(camPos, rayDir);
    float tNear = max(tHit.x, 0.0);
    float tFar = tHit.y;

    if (tFar < tNear) {
        FragColor = vec4(0.0);
        return;
    }

    vec4 accum = vec4(0.0);
    float t = tNear;
    vec3 boxSize = vec3(1.0, 1.0, scaleZ);

    // Light directions for better depth perception
    vec3 lightDir1 = normalize(vec3(0.5, 0.7, 1.0));   // Main light (front-top-right)
    vec3 lightDir2 = normalize(vec3(-0.3, 0.2, 0.8));  // Fill light (front-top-left)

    for (int i = 0; i < 4096; i++) {
        if (t >= tFar || accum.a > 0.98) break;

        vec3 pos = camPos + rayDir * t;

        // Sample normalized coordinate
        vec3 texPos = pos / boxSize;

        // Clipping plane check (in texture space 0..1)
        if (enableClip) {
             if (dot(texPos, clipPlane.xyz) + clipPlane.w < 0.0) {
                 t += stepSize;
                 continue;
             }
        }

        // Single texture read: density (R) + pre-computed gradient (GBA)
        vec4 volSample = texture(volumeTex, texPos);
        float density = volSample.r;

        // Window/level normalization
        float normalized = clamp((density * 65535.0 + rescaleIntercept - windowMin) / windowRange, 0.0, 1.0);

        if (normalized > densityThreshold) {
            vec4 sampleColor = texture(transferTex, normalized);

            // Alpha threshold skip: avoid lighting for nearly-transparent samples
            if (sampleColor.a < 0.001) {
                t += stepSize;
                continue;
            }

            // Apply tint
            sampleColor.rgb *= tintColor.rgb;

            // Decode pre-computed gradient from GBA channels
            vec3 gradient = volSample.gba * 2.0 - 1.0;
            float gradLen = length(gradient);

            // Edge enhancement: boost opacity at surfaces (high gradient)
            float edgeFactor = smoothstep(0.001, 0.02, gradLen);

            // Only compute full Phong lighting when gradient is significant
            if (gradLen > 0.01) {
                vec3 normal = normalize(gradient);

                // Two-light Phong lighting for better depth
                float NdotL1 = max(dot(normal, lightDir1), 0.0);
                float NdotL2 = max(dot(normal, lightDir2), 0.0);

                // Specular highlight (Blinn-Phong)
                vec3 viewDir = -rayDir;
                vec3 halfVec1 = normalize(lightDir1 + viewDir);
                vec3 halfVec2 = normalize(lightDir2 + viewDir);
                float spec1 = pow(max(dot(normal, halfVec1), 0.0), 32.0);
                float spec2 = pow(max(dot(normal, halfVec2), 0.0), 32.0);

                // Combined lighting
                float ambient = ambientIntensity;
                float diffuse = diffuseIntensity * (NdotL1 * 0.7 + NdotL2 * 0.3);
                float specular = specularIntensity * (spec1 * 0.7 + spec2 * 0.3);

                float lightFactor = ambient + diffuse;
                sampleColor.rgb *= clamp(lightFactor, 0.4, 1.4);
                sampleColor.rgb += vec3(specular * 0.3); // Subtle specular highlight

                // Edge enhancement: increase opacity at boundaries
                sampleColor.a *= (1.0 + edgeFactor * 0.5);
            } else {
                // Flat region: ambient-only lighting
                sampleColor.rgb *= ambientIntensity;
            }

            // Smoother opacity curve (cube root for softer falloff, then square for surfaces)
            float alphaAdjust = pow(sampleColor.a, 1.5);
            float sampleA = alphaAdjust * alphaScale;

            if (sampleA > 0.0005) {
                float oneMinusAccA = 1.0 - accum.a;
                accum.rgb += oneMinusAccA * sampleA * sampleColor.rgb;
                accum.a += oneMinusAccA * sampleA;
            }
        }

        t += stepSize;
    }

    FragColor = accum;
}
` + "\x00"

// ActiveVolume represents a single volume loaded into the renderer
type ActiveVolume struct {
	Data             []uint16
	DimX, DimY, DimZ int
	Color            color.RGBA // Tint color (RGB)
	Texture          uint32     // GPU texture ID

	// Per-volume rendering parameters
	WindowLevel      float64
	WindowWidth      float64
	RescaleIntercept float64
	AlphaScale       float64
}

// FindingBox represents a wireframe bounding box
type FindingBox struct {
	Name    string
	ClassID int
	BBox    BoundingBox3D
	Color   color.RGBA
	// Reference volume dimensions for bbox normalization
	RefDimX, RefDimY, RefDimZ int
}

// GPUVolumeRenderer performs GPU-accelerated volume ray casting
type GPUVolumeRenderer struct {
	// Volume data
	volumes  []*ActiveVolume
	findings []*FindingBox

	// Line rendering (for findings)
	lineProgram uint32
	lineVAO     uint32
	lineVBO     uint32
	lineMVPLoc  int32

	// Gizmo (Orientation Widget)
	gizmoProgram uint32
	gizmoVAO     uint32
	gizmoVBO     uint32

	// Camera controls
	rotationX, rotationY float64
	zoom                 float64

	// Global Rendering parameters (used as defaults or overrides)
	alphaScale        float64
	scaleZ            float64
	stepSize          float64
	densityThreshold  float64 // Clips voxels below this normalized density (0-1)
	ambientIntensity  float64 // Ambient lighting coefficient (0-1)
	diffuseIntensity  float64 // Diffuse lighting coefficient (0-1)
	specularIntensity float64 // Specular lighting coefficient (0-1)

	// Clipping Plane
	clipPlane  [4]float32
	enableClip bool

	// Pan offset for rotation pivot (screen-space, applied via right/up vectors)
	panX, panY float64

	// Render dimensions
	renderWidth, renderHeight int
	bgColor                   [3]float32

	// OpenGL resources
	initialized bool
	window      *glfw.Window
	program     uint32
	vao, vbo    uint32
	transferTex uint32
	fbo         uint32
	colorBuffer uint32
	uniforms    volumeUniforms

	// Transfer function
	transferFunc     []color.RGBA
	transferFuncType TransferFunctionType

	// Output
	rendered *image.RGBA

	// Synchronization
	mu                  sync.RWMutex
	needsRender         bool
	needsTransferUpload bool // Set when transfer function changes
	destroyed           bool // Guard for post-destroy commands

	// Worker thread
	commandChan chan func()
}

type volumeUniforms struct {
	camPos            int32
	camForward        int32
	camRight          int32
	camUp             int32
	fov               int32
	aspectRatio       int32
	stepSize          int32
	scaleZ            int32
	ambientIntensity  int32
	diffuseIntensity  int32
	specularIntensity int32
	clipPlane         int32
	enableClip        int32
	transferTex       int32
	windowMin         int32
	windowRange       int32
	alphaScale        int32
	rescaleIntercept  int32
	densityThreshold  int32
	tintColor         int32
	volumeTex         int32
}

const lineVertexShaderSource = `
#version 330 core
layout (location = 0) in vec3 aPos;
layout (location = 1) in vec4 aColor;
uniform mat4 mvp;
out vec4 vertexColor;
void main() {
    gl_Position = mvp * vec4(aPos, 1.0);
    vertexColor = aColor;
}
` + "\x00"

const lineFragmentShaderSource = `
#version 330 core
in vec4 vertexColor;
out vec4 FragColor;
void main() {
    FragColor = vertexColor;
}
` + "\x00"

// NewGPUVolumeRenderer creates a new GPU-accelerated volume renderer
// IMPORTANT: This must be called from the main thread on macOS
func NewGPUVolumeRenderer(width, height int) (*GPUVolumeRenderer, error) {
	gr := &GPUVolumeRenderer{
		rotationX:         0.3,
		rotationY:         0.5,
		zoom:              1.0,
		alphaScale:        0.08, // Slightly higher for better material visibility
		scaleZ:            2.0,
		stepSize:          0.001, // Default ray step size
		densityThreshold:  0.03,  // Lower threshold for more detail
		ambientIntensity:  0.55,  // Reduced ambient for better depth contrast
		diffuseIntensity:  0.65,  // Higher diffuse for surface definition
		specularIntensity: 0.4,   // Moderate specular for subtle highlights
		renderWidth:       width,
		renderHeight:      height,
		needsRender:       true,
		rendered:          image.NewRGBA(image.Rect(0, 0, width, height)),
		commandChan:       make(chan func(), 10), // Buffer for commands
	}

	// Initialize background color from config
	cfg := GetConfig()
	gr.bgColor = [3]float32{
		float32(cfg.BackgroundColor[0]) / 255.0,
		float32(cfg.BackgroundColor[1]) / 255.0,
		float32(cfg.BackgroundColor[2]) / 255.0,
	}

	gr.createTransferFunction()

	// 1. Create Window on Main Thread (Required by macOS)
	if err := gr.initWindow(); err != nil {
		return nil, err
	}

	// 2. Start Worker Thread for GL Context and Rendering
	// We need a way to know if initialization failed on the worker
	initErr := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		// Claim context functionality
		gr.window.MakeContextCurrent()

		// Safe Initialization
		var err error
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic during GL init: %v", r)
				}
			}()
			if initErr := gr.initGLResources(); initErr != nil {
				err = initErr
			}
		}()

		if err != nil {
			initErr <- err
			return
		}
		initErr <- nil

		// Command Loop
		for cmd := range gr.commandChan {
			// Execute command with panic protection
			func() {
				defer func() {
					if r := recover(); r != nil {
						slog.Error("Recovered from panic in GPU worker", "err", r)
					}
				}()
				cmd()
			}()
		}
	}()

	// Wait for worker initialization
	if err := <-initErr; err != nil {
		gr.window.Destroy()
		return nil, err
	}

	return gr, nil
}

// initWindow creates the hidden GLFW window (Main Thread)
func (gr *GPUVolumeRenderer) initWindow() error {
	// Configure for OpenGL 3.3 core
	glfw.WindowHint(glfw.ContextVersionMajor, 3)
	glfw.WindowHint(glfw.ContextVersionMinor, 3)
	glfw.WindowHint(glfw.OpenGLProfile, glfw.OpenGLCoreProfile)
	glfw.WindowHint(glfw.OpenGLForwardCompatible, glfw.True)
	glfw.WindowHint(glfw.Visible, glfw.False) // Hidden window for offscreen rendering

	// Create hidden window
	window, err := glfw.CreateWindow(gr.renderWidth, gr.renderHeight, "GPU Volume Renderer", nil, nil)
	if err != nil {
		glfw.DefaultWindowHints()
		return fmt.Errorf("failed to create GLFW window: %w", err)
	}
	gr.window = window

	// Reset hints
	glfw.DefaultWindowHints()
	return nil
}

// initGLResources initializes OpenGL context and resources (Worker Thread)
func (gr *GPUVolumeRenderer) initGLResources() error {
	// Initialize OpenGL
	if err := gl.Init(); err != nil {
		return fmt.Errorf("failed to initialize OpenGL: %w", err)
	}

	// Compile shaders
	vertexShader, err := compileShader(vertexShaderSource, gl.VERTEX_SHADER)
	if err != nil {
		return fmt.Errorf("vertex shader: %w", err)
	}

	fragmentShader, err := compileShader(fragmentShaderSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return fmt.Errorf("fragment shader: %w", err)
	}

	// Link program
	gr.program = gl.CreateProgram()
	gl.AttachShader(gr.program, vertexShader)
	gl.AttachShader(gr.program, fragmentShader)
	gl.LinkProgram(gr.program)

	// Initialize Gizmo
	if err := gr.initGizmo(); err != nil {
		return fmt.Errorf("gizmo init: %w", err)
	}

	var status int32
	gl.GetProgramiv(gr.program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(gr.program, gl.INFO_LOG_LENGTH, &logLength)
		logMsg := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(gr.program, logLength, nil, gl.Str(logMsg))
		return fmt.Errorf("failed to link program: %s", logMsg)
	}

	gl.DeleteShader(vertexShader)
	gl.DeleteShader(fragmentShader)

	// Create fullscreen quad
	vertices := []float32{
		-1.0, -1.0,
		1.0, -1.0,
		-1.0, 1.0,
		1.0, -1.0,
		1.0, 1.0,
		-1.0, 1.0,
	}

	gl.GenVertexArrays(1, &gr.vao)
	gl.GenBuffers(1, &gr.vbo)

	gl.BindVertexArray(gr.vao)
	gl.BindBuffer(gl.ARRAY_BUFFER, gr.vbo)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	gl.VertexAttribPointer(0, 2, gl.FLOAT, false, 2*4, nil)
	gl.EnableVertexAttribArray(0)

	// Create framebuffer for offscreen rendering
	gl.GenFramebuffers(1, &gr.fbo)
	gl.BindFramebuffer(gl.FRAMEBUFFER, gr.fbo)

	// Create color buffer texture
	gl.GenTextures(1, &gr.colorBuffer)
	gl.BindTexture(gl.TEXTURE_2D, gr.colorBuffer)
	gl.TexImage2D(gl.TEXTURE_2D, 0, gl.RGBA8, int32(gr.renderWidth), int32(gr.renderHeight), 0, gl.RGBA, gl.UNSIGNED_BYTE, nil)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_2D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.FramebufferTexture2D(gl.FRAMEBUFFER, gl.COLOR_ATTACHMENT0, gl.TEXTURE_2D, gr.colorBuffer, 0)

	if gl.CheckFramebufferStatus(gl.FRAMEBUFFER) != gl.FRAMEBUFFER_COMPLETE {
		return fmt.Errorf("framebuffer is not complete")
	}

	// Create transfer function texture
	gr.uploadTransferFunction()

	gr.cacheUniforms()

	gr.initialized = true
	return nil
}

func (gr *GPUVolumeRenderer) cacheUniforms() {
	gr.uniforms = volumeUniforms{
		camPos:            gl.GetUniformLocation(gr.program, gl.Str("camPos\x00")),
		camForward:        gl.GetUniformLocation(gr.program, gl.Str("camForward\x00")),
		camRight:          gl.GetUniformLocation(gr.program, gl.Str("camRight\x00")),
		camUp:             gl.GetUniformLocation(gr.program, gl.Str("camUp\x00")),
		fov:               gl.GetUniformLocation(gr.program, gl.Str("fov\x00")),
		aspectRatio:       gl.GetUniformLocation(gr.program, gl.Str("aspectRatio\x00")),
		stepSize:          gl.GetUniformLocation(gr.program, gl.Str("stepSize\x00")),
		scaleZ:            gl.GetUniformLocation(gr.program, gl.Str("scaleZ\x00")),
		ambientIntensity:  gl.GetUniformLocation(gr.program, gl.Str("ambientIntensity\x00")),
		diffuseIntensity:  gl.GetUniformLocation(gr.program, gl.Str("diffuseIntensity\x00")),
		specularIntensity: gl.GetUniformLocation(gr.program, gl.Str("specularIntensity\x00")),
		clipPlane:         gl.GetUniformLocation(gr.program, gl.Str("clipPlane\x00")),
		enableClip:        gl.GetUniformLocation(gr.program, gl.Str("enableClip\x00")),
		transferTex:       gl.GetUniformLocation(gr.program, gl.Str("transferTex\x00")),
		windowMin:         gl.GetUniformLocation(gr.program, gl.Str("windowMin\x00")),
		windowRange:       gl.GetUniformLocation(gr.program, gl.Str("windowRange\x00")),
		alphaScale:        gl.GetUniformLocation(gr.program, gl.Str("alphaScale\x00")),
		rescaleIntercept:  gl.GetUniformLocation(gr.program, gl.Str("rescaleIntercept\x00")),
		densityThreshold:  gl.GetUniformLocation(gr.program, gl.Str("densityThreshold\x00")),
		tintColor:         gl.GetUniformLocation(gr.program, gl.Str("tintColor\x00")),
		volumeTex:         gl.GetUniformLocation(gr.program, gl.Str("volumeTex\x00")),
	}
}

func compileShader(source string, shaderType uint32) (uint32, error) {
	shader := gl.CreateShader(shaderType)

	csources, free := gl.Strs(source)
	gl.ShaderSource(shader, 1, csources, nil)
	free()
	gl.CompileShader(shader)

	var status int32
	gl.GetShaderiv(shader, gl.COMPILE_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetShaderiv(shader, gl.INFO_LOG_LENGTH, &logLength)
		logMsg := strings.Repeat("\x00", int(logLength+1))
		gl.GetShaderInfoLog(shader, logLength, nil, gl.Str(logMsg))
		return 0, fmt.Errorf("failed to compile shader: %s", logMsg)
	}

	return shader, nil
}

func (gr *GPUVolumeRenderer) createTransferFunction() {
	cfg := GetConfig()
	bands := DefaultColorBands()
	gr.transferFunc = CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", bands)

	// Set initial density threshold from Air band
	if len(bands) > 0 && bands[0].IsTransparent {
		maxDensity := 30000.0
		if len(bands) > 0 {
			maxDensity = float64(bands[len(bands)-1].Threshold)
		}
		gr.densityThreshold = float64(bands[0].Threshold) / maxDensity
	}
}

// SetColorOpacityPreset sets the transfer function from a named preset in the config
// Valid presets: "DEFAULT", "FINDING", "MONOCHROME", "LAPTOP_REMOVAL"
func (gr *GPUVolumeRenderer) SetColorOpacityPreset(presetName string) {
	cfg := GetConfig()
	if cfg == nil {
		return
	}
	// Check if preset exists in config
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

// GetRendered returns the last rendered image (thread-safe)
func (gr *GPUVolumeRenderer) GetRendered() image.Image {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.rendered
}

// ForceRender forces a re-render
func (gr *GPUVolumeRenderer) ForceRender() {
	gr.mu.Lock()
	gr.needsRender = true
	gr.mu.Unlock()
}

// Destroy cleans up resources
func (gr *GPUVolumeRenderer) Destroy() {
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Recovered from panic in GPU Destroy", "err", r)
		}
	}()

	// Send destroy command to worker
	done := make(chan struct{})
	gr.commandChan <- func() {
		gr.destroyGL()
		close(done)
	}
	<-done
	close(gr.commandChan)

	// Destroy window on Main Thread (Required by macOS)
	// Context must be detached by worker first (in destroyGL)
	if gr.window != nil {
		gr.window.Destroy()
	}
}

func (gr *GPUVolumeRenderer) destroyGL() {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if !gr.initialized {
		return
	}
	gr.window.MakeContextCurrent()

	for _, vol := range gr.volumes {
		if vol.Texture != 0 {
			gl.DeleteTextures(1, &vol.Texture)
		}
	}
	if gr.transferTex != 0 {
		gl.DeleteTextures(1, &gr.transferTex)
	}
	if gr.colorBuffer != 0 {
		gl.DeleteTextures(1, &gr.colorBuffer)
	}
	if gr.fbo != 0 {
		gl.DeleteFramebuffers(1, &gr.fbo)
	}
	if gr.vbo != 0 {
		gl.DeleteBuffers(1, &gr.vbo)
	}
	if gr.vao != 0 {
		gl.DeleteVertexArrays(1, &gr.vao)
	}
	gr.initialized = false
	gr.destroyed = true
	// Detach context on worker thread so main thread can destroy window
	glfw.DetachCurrentContext()
}

// SetTransferFunction sets the transfer function type
func (gr *GPUVolumeRenderer) SetTransferFunction(tfType TransferFunctionType) {
	gr.mu.Lock()
	gr.transferFuncType = tfType
	gr.transferFunc = CreateTransferFunction(tfType)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetAlphaScale sets the opacity scale for ALL volumes (Global Override or Default)
func (gr *GPUVolumeRenderer) SetAlphaScale(alpha float64) {
	gr.mu.Lock()
	gr.alphaScale = alpha
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetBackgroundColor sets the background color
func (gr *GPUVolumeRenderer) SetBackgroundColor(r, g, b, a float64) {
	gr.mu.Lock()
	gr.bgColor = [3]float32{float32(r), float32(g), float32(b)}
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetMaterialThresholds updates the transfer function boundaries for Orange/Green/Blue materials.
// t1: Boundary between Orange and Green
// t2: Boundary between Green and Blue
func (gr *GPUVolumeRenderer) SetMaterialThresholds(t1, t2 int) {
	gr.mu.Lock()
	cfg := GetConfig()
	// Max density 30000 is standard
	gr.transferFunc = CreateTransferFunctionDynamic(cfg, "DEFAULT", 30000, t1, t2)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetMaterialBands updates the transfer function using variable color bands.
// This uses gradient remapping so moving slider handles stretches/compresses
// the color gradients to fit the new thresholds.
func (gr *GPUVolumeRenderer) SetMaterialBands(bands []ColorBand) {
	gr.mu.Lock()
	cfg := GetConfig()
	gr.transferFunc = CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", bands)
	gr.needsTransferUpload = true
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetClippingPlane updates the clipping plane equation (dot(pos, plane.xyz) + plane.w < 0 is clipped)
func (gr *GPUVolumeRenderer) SetClippingPlane(nx, ny, nz, d float32, enable bool) {
	gr.mu.Lock()
	gr.clipPlane = [4]float32{nx, ny, nz, d}
	gr.enableClip = enable
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetDensityThreshold sets the minimum density threshold (0-1 normalized)
// Voxels below this threshold are clipped (invisible)
func (gr *GPUVolumeRenderer) SetDensityThreshold(threshold float64) {
	gr.mu.Lock()
	gr.densityThreshold = threshold
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetDensityThreshold returns the current density threshold
func (gr *GPUVolumeRenderer) GetDensityThreshold() float64 {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.densityThreshold
}

// SetLighting sets the lighting parameters (ambient, diffuse, specular)
func (gr *GPUVolumeRenderer) SetLighting(ambient, diffuse, specular float64) {
	gr.mu.Lock()
	gr.ambientIntensity = ambient
	gr.diffuseIntensity = diffuse
	gr.specularIntensity = specular
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetLighting returns the current lighting parameters
func (gr *GPUVolumeRenderer) GetLighting() (ambient, diffuse, specular float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.ambientIntensity, gr.diffuseIntensity, gr.specularIntensity
}

// SetVolumeAlpha sets the alpha scale for a specific volume index
func (gr *GPUVolumeRenderer) SetVolumeAlpha(index int, alpha float64) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	if index >= 0 && index < len(gr.volumes) {
		gr.volumes[index].AlphaScale = alpha
		gr.needsRender = true
	}
}

// SetScaleZ sets the Z-axis scaling factor for ALL volumes
func (gr *GPUVolumeRenderer) SetScaleZ(scale float64) {
	gr.mu.Lock()
	gr.scaleZ = scale
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetPan sets the screen-space pan offset
func (gr *GPUVolumeRenderer) SetPan(px, py float64) {
	gr.mu.Lock()
	gr.panX = px
	gr.panY = py
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetPan returns the current pan offset
func (gr *GPUVolumeRenderer) GetPan() (float64, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.panX, gr.panY
}

// AdjustPan adjusts the pan offset by delta values
func (gr *GPUVolumeRenderer) AdjustPan(dx, dy float64) {
	gr.mu.Lock()
	gr.panX += dx
	gr.panY += dy
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetRotation sets camera rotation
func (gr *GPUVolumeRenderer) SetRotation(rotX, rotY float64) {
	gr.mu.Lock()
	gr.rotationX = rotX
	gr.rotationY = rotY
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetZoom sets camera zoom
func (gr *GPUVolumeRenderer) SetZoom(zoom float64) {
	gr.mu.Lock()
	gr.zoom = zoom
	gr.needsRender = true
	gr.mu.Unlock()
}

// GetRotation returns current rotation
func (gr *GPUVolumeRenderer) GetRotation() (float64, float64) {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.rotationX, gr.rotationY
}

// GetZoom returns current zoom
func (gr *GPUVolumeRenderer) GetZoom() float64 {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.zoom
}

// GetAlphaScale returns current alpha scale
func (gr *GPUVolumeRenderer) GetAlphaScale() float64 {
	gr.mu.RLock()
	defer gr.mu.RUnlock()
	return gr.alphaScale
}

// ClearVolumes clears all loaded volumes
func (gr *GPUVolumeRenderer) ClearVolumes() {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	// Cleanup textures
	gr.window.MakeContextCurrent()
	for _, vol := range gr.volumes {
		if vol.Texture != 0 {
			gl.DeleteTextures(1, &vol.Texture)
		}
	}
	gr.volumes = nil
	gr.panX = 0
	gr.panY = 0
	gr.needsRender = true
}

// ... SetWindowLevel ...
// SetWindowLevel sets the window level for ALL active volumes (Global Override)
func (gr *GPUVolumeRenderer) SetWindowLevel(level float64) {
	gr.mu.Lock()
	for _, vol := range gr.volumes {
		vol.WindowLevel = level
	}
	gr.needsRender = true
	gr.mu.Unlock()
}

// SetWindowWidth sets the window width for ALL active volumes (Global Override)
func (gr *GPUVolumeRenderer) SetWindowWidth(width float64) {
	gr.mu.Lock()
	for _, vol := range gr.volumes {
		vol.WindowWidth = width
	}
	gr.needsRender = true
	gr.mu.Unlock()
}

// ... SetRescaleIntercept ...
// SetRescaleIntercept sets the rescale intercept offset for ALL active volumes (Global Override)
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
	return RenderParams{
		WindowLevel:       gr.defaultWindowLevel(),
		WindowWidth:       gr.defaultWindowWidth(),
		AlphaScale:        gr.alphaScale,
		ScaleZ:            gr.scaleZ,
		RescaleIntercept:  gr.defaultRescaleIntercept(),
		StepSize:          gr.stepSize,
		DensityThreshold:  gr.densityThreshold,
		AmbientIntensity:  gr.ambientIntensity,
		DiffuseIntensity:  gr.diffuseIntensity,
		SpecularIntensity: gr.specularIntensity,
	}
}

// SetParams updates render parameters in a single lock.
func (gr *GPUVolumeRenderer) SetParams(p RenderParams) {
	gr.mu.Lock()
	gr.alphaScale = p.AlphaScale
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

func (gr *GPUVolumeRenderer) defaultWindowLevel() float64 {
	if len(gr.volumes) > 0 {
		return gr.volumes[0].WindowLevel
	}
	return 0
}

func (gr *GPUVolumeRenderer) defaultWindowWidth() float64 {
	if len(gr.volumes) > 0 {
		return gr.volumes[0].WindowWidth
	}
	return 0
}

func (gr *GPUVolumeRenderer) defaultRescaleIntercept() float64 {
	if len(gr.volumes) > 0 {
		return gr.volumes[0].RescaleIntercept
	}
	return 0
}

// ... AddVolume ...

// AddVolume adds a volume to the renderer with specific rendering parameters
func (gr *GPUVolumeRenderer) AddVolume(pd *PixelData, imageRows, imageCols int, pixelRep int, tint color.RGBA, wl, ww, intercept, alphaScale float64) error {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	if pd == nil || len(pd.Frames) == 0 {
		return nil
	}

	// Get dimensions
	if imageRows <= 0 || imageCols <= 0 {
		return nil
	}
	dimX, dimY := imageCols, imageRows
	dimZ := len(pd.Frames)

	// Allocate volume data
	totalSize := dimX * dimY * dimZ
	data := make([]uint16, totalSize)
	signed := pixelRep == 1

	for z, f := range pd.Frames {
		sliceOffset := z * dimX * dimY

		if pd.IsEncapsulated {
			// Encapsulated (JPEG/RLE) frame
			decodedData := decodeJPEGFrame(f.CompressedData, dimX, dimY, signed)

			// Copy decoded data to volume
			for i, val := range decodedData {
				if i < dimX*dimY {
					uval := uint16(val)
					if val < 0 {
						uval = 0
					}
					data[sliceOffset+i] = uval
				}
			}
		} else {
			// Native flat data
			if len(f.Data) > 0 {
				copy(data[sliceOffset:], f.Data)
			}
		}
	}

	var minVal, maxVal uint16 = 65535, 0
	for _, v := range data {
		if v < minVal {
			minVal = v
		}
		if v > maxVal {
			maxVal = v
		}
	}
	if slog.Default().Enabled(context.Background(), slog.LevelDebug) {
		var sum float64
		for _, v := range data {
			sum += float64(v)
		}
		avg := sum / float64(len(data))
		slog.Debug("GPU AddVolume", "dim", fmt.Sprintf("%dx%dx%d", dimX, dimY, dimZ), "range", fmt.Sprintf("%d-%d", minVal, maxVal), "avg", avg, "wl", wl, "ww", ww)
	}

	gr.volumes = append(gr.volumes, &ActiveVolume{
		Data:             data,
		DimX:             dimX,
		DimY:             dimY,
		DimZ:             dimZ,
		Color:            tint,
		Texture:          0, // Will upload in Render
		WindowLevel:      wl,
		WindowWidth:      ww,
		RescaleIntercept: intercept,
		AlphaScale:       alphaScale,
	})

	gr.needsRender = true
	return nil
}

// AddVolumeFromData adds raw volume data
func (gr *GPUVolumeRenderer) AddVolumeFromData(data []uint16, dimX, dimY, dimZ int, tint color.RGBA, wl, ww, intercept, alphaScale float64) {
	gr.mu.Lock()
	defer gr.mu.Unlock()

	vol := &ActiveVolume{
		Data:             make([]uint16, len(data)),
		DimX:             dimX,
		DimY:             dimY,
		DimZ:             dimZ,
		Color:            tint,
		Texture:          0,
		WindowLevel:      wl,
		WindowWidth:      ww,
		RescaleIntercept: intercept,
		AlphaScale:       alphaScale,
	}
	copy(vol.Data, data)

	gr.volumes = append(gr.volumes, vol)
	gr.needsRender = true
}

// uploadVolume uploads a single volume to GPU with pre-computed gradients.
// Packs density + central-difference gradients into RGBA16 texture:
// R = density, G = gradX, B = gradY, A = gradZ (encoded as (grad + 65535) / 2)
func (gr *GPUVolumeRenderer) uploadVolume(vol *ActiveVolume) {
	if len(vol.Data) == 0 {
		return
	}

	dimX, dimY, dimZ := vol.DimX, vol.DimY, vol.DimZ
	total := dimX * dimY * dimZ
	sliceSize := dimX * dimY

	rgba := make([]uint16, total*4)

	for z := 0; z < dimZ; z++ {
		for y := 0; y < dimY; y++ {
			for x := 0; x < dimX; x++ {
				idx := z*sliceSize + y*dimX + x
				outIdx := idx * 4

				// R = density (raw uint16)
				rgba[outIdx] = vol.Data[idx]

				// Central-difference gradients (zero at volume boundaries)
				var dx, dy, dz float64
				if x > 0 && x < dimX-1 {
					dx = float64(vol.Data[idx+1]) - float64(vol.Data[idx-1])
				}
				if y > 0 && y < dimY-1 {
					dy = float64(vol.Data[idx+dimX]) - float64(vol.Data[idx-dimX])
				}
				if z > 0 && z < dimZ-1 {
					dz = float64(vol.Data[idx+sliceSize]) - float64(vol.Data[idx-sliceSize])
				}

				// Encode gradients: map [-65535, +65535] to [0, 65535]
				rgba[outIdx+1] = uint16(math.Max(0, math.Min(65535, (dx+65535.0)/2.0)))
				rgba[outIdx+2] = uint16(math.Max(0, math.Min(65535, (dy+65535.0)/2.0)))
				rgba[outIdx+3] = uint16(math.Max(0, math.Min(65535, (dz+65535.0)/2.0)))
			}
		}
	}

	if vol.Texture != 0 {
		gl.DeleteTextures(1, &vol.Texture)
	}

	gl.GenTextures(1, &vol.Texture)
	gl.BindTexture(gl.TEXTURE_3D, vol.Texture)
	gl.TexImage3D(gl.TEXTURE_3D, 0, gl.RGBA16,
		int32(dimX), int32(dimY), int32(dimZ),
		0, gl.RGBA, gl.UNSIGNED_SHORT, gl.Ptr(rgba))

	gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_WRAP_T, gl.CLAMP_TO_EDGE)
	gl.TexParameteri(gl.TEXTURE_3D, gl.TEXTURE_WRAP_R, gl.CLAMP_TO_EDGE)
}

// uploadTransferFunction uploads the transfer function to GPU texture
func (gr *GPUVolumeRenderer) uploadTransferFunction() {
	// Convert to float RGBA for GPU (using high-res transfer function)
	size := len(gr.transferFunc)
	data := make([]float32, size*4)
	for i, c := range gr.transferFunc {
		data[i*4+0] = float32(c.R) / 255.0
		data[i*4+1] = float32(c.G) / 255.0
		data[i*4+2] = float32(c.B) / 255.0
		data[i*4+3] = float32(c.A) / 255.0
	}

	if gr.transferTex == 0 {
		gl.GenTextures(1, &gr.transferTex)
	}

	gl.BindTexture(gl.TEXTURE_1D, gr.transferTex)
	gl.TexImage1D(gl.TEXTURE_1D, 0, gl.RGBA32F, int32(size), 0, gl.RGBA, gl.FLOAT, gl.Ptr(data))
	gl.TexParameteri(gl.TEXTURE_1D, gl.TEXTURE_MIN_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_1D, gl.TEXTURE_MAG_FILTER, gl.LINEAR)
	gl.TexParameteri(gl.TEXTURE_1D, gl.TEXTURE_WRAP_S, gl.CLAMP_TO_EDGE)
}

// Render performs GPU-accelerated ray casting (Thread-Safe Dispatch)
func (gr *GPUVolumeRenderer) Render() (result *image.RGBA) {
	resultChan := make(chan *image.RGBA, 1)

	// Protect against send on closed channel
	defer func() {
		if r := recover(); r != nil {
			slog.Error("Possible shutdown race in GPU Render dispatch", "err", r)
			result = nil
		}
	}()

	gr.commandChan <- func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("Recovered from panic in GPU Render Internal", "err", r)
				resultChan <- nil
			}
		}()
		resultChan <- gr.renderInternal()
	}
	return <-resultChan
}

// renderInternal performs the actual GL calls (Must run on Worker Thread)
func (gr *GPUVolumeRenderer) renderInternal() *image.RGBA {
	gr.mu.Lock()
	if gr.destroyed {
		gr.mu.Unlock()
		return nil
	}
	if !gr.needsRender || !gr.initialized {
		result := gr.rendered
		gr.mu.Unlock()
		return result
	}

	// Copy parameters
	rotX, rotY := gr.rotationX, gr.rotationY
	zoom := gr.zoom
	alpha := float32(gr.alphaScale)
	scaleZ := float32(gr.scaleZ)
	stepSize := gr.stepSize
	width, height := gr.renderWidth, gr.renderHeight
	panX, panY := float32(gr.panX), float32(gr.panY)

	needsTransferUpload := gr.needsTransferUpload
	gr.needsRender = false
	gr.needsTransferUpload = false

	// Copy volume list for rendering
	// Note: We access vol.Texture which is GL resource, but we only read it or write it on GL thread.
	// The volumes slice structure changes are protected by mutex, but we need to hold it while iterating?
	// Yes, but we need to unlock to avoid blocking for too long.
	// Best pattern: copy the slice of pointers. The ActiveVolume content (Data) is constant once added.
	volumesToRender := make([]*ActiveVolume, len(gr.volumes))
	copy(volumesToRender, gr.volumes)
	gr.mu.Unlock()

	// GL Context is already current on this thread

	// Upload transfer function
	if needsTransferUpload && gr.transferTex != 0 {
		gl.DeleteTextures(1, &gr.transferTex)
		gr.transferTex = 0
	}
	if gr.transferTex == 0 {
		gr.uploadTransferFunction()
	}

	// Upload volumes
	for _, vol := range volumesToRender {
		if vol.Texture == 0 {
			gr.uploadVolume(vol)
		}
	}

	// Bind framebuffer
	gl.BindFramebuffer(gl.FRAMEBUFFER, gr.fbo)
	gl.Viewport(0, 0, int32(width), int32(height))
	// Use background color from config (MVS uses light gray 237,237,237)
	// Use background color (dynamic)
	bgR, bgG, bgB := gr.bgColor[0], gr.bgColor[1], gr.bgColor[2]
	gl.ClearColor(bgR, bgG, bgB, 1.0)
	gl.Clear(gl.COLOR_BUFFER_BIT)

	// Use shader
	gl.UseProgram(gr.program)

	// Set Camera Uniforms (common for all volumes)
	sinAz, cosAz := float32(math.Sin(rotX)), float32(math.Cos(rotX))
	sinEl, cosEl := float32(math.Sin(rotY)), float32(math.Cos(rotY))

	// Geometric center as rotation pivot, offset by user pan
	centerX, centerY, centerZ := float32(0.5)+panX, float32(0.5)+panY, scaleZ*0.5
	camX := sinAz*cosEl*float32(zoom) + centerX
	camY := sinEl*float32(zoom) + centerY
	camZ := cosAz*cosEl*float32(zoom) + centerZ

	fwdX, fwdY, fwdZ := centerX-camX, centerY-camY, centerZ-camZ
	fwdLen := float32(math.Sqrt(float64(fwdX*fwdX + fwdY*fwdY + fwdZ*fwdZ)))
	fwdX, fwdY, fwdZ = fwdX/fwdLen, fwdY/fwdLen, fwdZ/fwdLen

	// Calculate MVP for lines (Model is Identity scaled by boxSize?)
	// Actually our vertex coordinates are 0..1 normalized.
	// But our camera setup is a bit custom in the shader (ray casting).
	// We need to replicate the camera transform for standard geometry.
	// Camera is orbiting around 0.5,0.5,0.5

	// Create view matrix
	// Eye: camX, camY, camZ
	// Center: 0.5, 0.5, 0.5 * scaleZ
	// Up: 0, 1, 0 (simplified)

	// Since we don't have a matrix library handy, we construct a basic LookAt + Perspective
	// For now, let's skip complex matrix math and trust that we can add it later if needed.
	// Wait, we need it to draw lines!
	// Okay, we'll try to reuse the camera params.

	// ... (Existing ray casting code) ...

	// Draw findings moved to end to prevent state pollution

	// Right = cross(up=(0,1,0), forward)? No, use rotation math
	rightX, rightY, rightZ := cosAz, float32(0), -sinAz

	// Up = cross(forward, right)
	upX := fwdY*rightZ - fwdZ*rightY
	upY := fwdZ*rightX - fwdX*rightZ
	upZ := fwdX*rightY - fwdY*rightX
	upLen := float32(math.Sqrt(float64(upX*upX + upY*upY + upZ*upZ)))
	if upLen > 0.001 {
		upX, upY, upZ = upX/upLen, upY/upLen, upZ/upLen
	}

	gl.Uniform3f(gr.uniforms.camPos, camX, camY, camZ)
	gl.Uniform3f(gr.uniforms.camForward, fwdX, fwdY, fwdZ)
	gl.Uniform3f(gr.uniforms.camRight, rightX, rightY, rightZ)
	gl.Uniform3f(gr.uniforms.camUp, upX, upY, upZ)
	gl.Uniform1f(gr.uniforms.fov, 0.8)
	gl.Uniform1f(gr.uniforms.aspectRatio, float32(width)/float32(height))
	// Adaptive step size: scale by zoom for better interactivity when zoomed out,
	// finer quality when zoomed in. At zoom=1.0, step equals the configured base.
	clampedZoom := math.Max(0.5, math.Min(5.0, zoom))
	adaptiveStep := stepSize * clampedZoom
	gl.Uniform1f(gr.uniforms.stepSize, float32(adaptiveStep))
	gl.Uniform1f(gr.uniforms.scaleZ, scaleZ)
	gl.Uniform1f(gr.uniforms.ambientIntensity, float32(gr.ambientIntensity))
	gl.Uniform1f(gr.uniforms.diffuseIntensity, float32(gr.diffuseIntensity))
	gl.Uniform1f(gr.uniforms.specularIntensity, float32(gr.specularIntensity))

	// Clipping
	gl.Uniform4f(gr.uniforms.clipPlane, gr.clipPlane[0], gr.clipPlane[1], gr.clipPlane[2], gr.clipPlane[3])
	if gr.enableClip {
		gl.Uniform1i(gr.uniforms.enableClip, 1)
	} else {
		gl.Uniform1i(gr.uniforms.enableClip, 0)
	}

	// Set Transfer Function Texture
	gl.ActiveTexture(gl.TEXTURE1)
	gl.BindTexture(gl.TEXTURE_1D, gr.transferTex)
	gl.Uniform1i(gr.uniforms.transferTex, 1)

	// Enable Additive Blending for multi-volume
	gl.Enable(gl.BLEND)
	gl.BlendFunc(gl.SRC_ALPHA, gl.ONE_MINUS_SRC_ALPHA)

	// Render each volume
	gl.BindVertexArray(gr.vao)

	for _, vol := range volumesToRender {
		// Set Volume-specific Uniforms
		wl, ww := float32(vol.WindowLevel), float32(vol.WindowWidth)
		intercept := float32(vol.RescaleIntercept)
		volAlpha := alpha * float32(vol.AlphaScale)

		gl.Uniform1f(gr.uniforms.windowMin, wl-ww/2)
		gl.Uniform1f(gr.uniforms.windowRange, ww)
		gl.Uniform1f(gr.uniforms.alphaScale, volAlpha)
		gl.Uniform1f(gr.uniforms.rescaleIntercept, intercept)
		gl.Uniform1f(gr.uniforms.densityThreshold, float32(gr.densityThreshold))

		// Set Tint Color
		gl.Uniform4f(gr.uniforms.tintColor,
			float32(vol.Color.R)/255.0, float32(vol.Color.G)/255.0, float32(vol.Color.B)/255.0, float32(vol.Color.A)/255.0)

		// Bind Volume Texture
		gl.ActiveTexture(gl.TEXTURE0)
		gl.BindTexture(gl.TEXTURE_3D, vol.Texture)
		gl.Uniform1i(gr.uniforms.volumeTex, 0)

		gl.DrawArrays(gl.TRIANGLES, 0, 6)
	}

	gl.Disable(gl.BLEND)

	// Draw findings on top
	if len(gr.findings) > 0 {
		// Construct simplified MVP
		// Projection
		// Align FOV with Shader Uniform. Shader uses fov=0.8.
		// In shader: rayDir = ... + up * uv.y * fov.
		// uv.y is -1..1. So max tan(angle) is fov.
		// So tangent of half-fov is 0.8.
		// Perspective Matrix f = 1.0 / tan(half_fov).
		shaderFov := float32(0.8)
		aspect := float32(width) / float32(height)
		near := float32(0.1)
		far := float32(100.0)

		// Perspective Matrix
		f := 1.0 / shaderFov
		proj := [16]float32{
			f / aspect, 0, 0, 0,
			0, f, 0, 0,
			0, 0, (far + near) / (near - far), -1,
			0, 0, (2 * far * near) / (near - far), 0,
		}

		// View Matrix (LookAt)
		// eye = camPos, center = center, up = up
		// We need to compute these properly.
		// Using existing camX, camY, camZ from shader logic
		eye := []float32{camX, camY, camZ}
		center := []float32{centerX, centerY, centerZ}
		up := []float32{0, 1, 0}

		// Forward
		fwd := []float32{center[0] - eye[0], center[1] - eye[1], center[2] - eye[2]}
		l := float32(math.Sqrt(float64(fwd[0]*fwd[0] + fwd[1]*fwd[1] + fwd[2]*fwd[2])))
		fwd[0] /= l
		fwd[1] /= l
		fwd[2] /= l

		// Right (Match Shader Logic: Rotation Math)
		right := []float32{cosAz, 0, -sinAz}

		// Up (Match Shader Logic: Cross(Fwd, Right))
		// Note: Shader uses Fwd x Right. Previous MVP used Right x Fwd (Inverted).
		// up = cross(fwd, right)
		up[0] = fwd[1]*right[2] - fwd[2]*right[1]
		up[1] = fwd[2]*right[0] - fwd[0]*right[2]
		up[2] = fwd[0]*right[1] - fwd[1]*right[0]
		upLen := float32(math.Sqrt(float64(up[0]*up[0] + up[1]*up[1] + up[2]*up[2])))
		if upLen > 0.001 {
			up[0], up[1], up[2] = up[0]/upLen, up[1]/upLen, up[2]/upLen
		}

		view := [16]float32{
			right[0], up[0], -fwd[0], 0,
			right[1], up[1], -fwd[1], 0,
			right[2], up[2], -fwd[2], 0,
			0, 0, 0, 1,
		}
		// Translate view by -eye
		// (Simplified matrix multiplication for translation)
		tx := -(right[0]*eye[0] + right[1]*eye[1] + right[2]*eye[2])
		ty := -(up[0]*eye[0] + up[1]*eye[1] + up[2]*eye[2])
		tz := -(-fwd[0]*eye[0] - fwd[1]*eye[1] - fwd[2]*eye[2])
		view[12] = tx
		view[13] = ty
		view[14] = tz

		// MVP = Proj * View
		var mvp [16]float32
		// Mat4 mul: R = A * B
		// R[c][r] = sum(A[k][r] * B[c][k])
		// A = proj, B = view
		// proj and view are flat arrays [16]float32 (col-major)
		// Index(col, row) = col*4 + row
		for c := 0; c < 4; c++ {
			for r := 0; r < 4; r++ {
				sum := float32(0.0)
				for k := 0; k < 4; k++ {
					// proj (A) row r, col k -> A[k*4 + r]
					// view (B) row k, col c -> B[c*4 + k]
					sum += proj[k*4+r] * view[c*4+k]
				}
				mvp[c*4+r] = sum
			}
		}

		gr.drawFindings(mvp)
	}

	// Draw Orientation Gizmo
	gr.drawGizmo()

	// Read pixels back
	pixels := make([]byte, width*height*4)
	gl.ReadPixels(0, 0, int32(width), int32(height), gl.RGBA, gl.UNSIGNED_BYTE, gl.Ptr(pixels))

	// Convert to image.RGBA (flip Y)
	gr.mu.Lock()
	for y := 0; y < height; y++ {
		srcY := height - 1 - y
		for x := 0; x < width; x++ {
			srcIdx := (srcY*width + x) * 4
			gr.rendered.SetRGBA(x, y, color.RGBA{
				R: pixels[srcIdx],
				G: pixels[srcIdx+1],
				B: pixels[srcIdx+2],
				A: 255, // Full alpha for UI display
			})
		}
	}
	gr.mu.Unlock()

	gr.mu.RLock()
	result := gr.rendered
	gr.mu.RUnlock()
	return result
}

// Helper func init moved to end
func init() {
	slog.Info("GPU volume renderer module loaded")
}

// AddFinding adds a finding bounding box to the display
// refDimX/Y/Z are the volume dimensions the bbox coordinates are relative to
func (gr *GPUVolumeRenderer) AddFinding(name string, classID int, bbox BoundingBox3D, c color.RGBA, refDimX, refDimY, refDimZ int) {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.findings = append(gr.findings, &FindingBox{
		Name:    name,
		ClassID: classID,
		BBox:    bbox,
		Color:   c,
		RefDimX: refDimX,
		RefDimY: refDimY,
		RefDimZ: refDimZ,
	})
	gr.needsRender = true
}

// ClearFindings clears all findings
func (gr *GPUVolumeRenderer) ClearFindings() {
	gr.mu.Lock()
	defer gr.mu.Unlock()
	gr.findings = nil
	gr.needsRender = true
}

// SetFindingMode sets the finding display mode
func (gr *GPUVolumeRenderer) SetFindingMode(mode int) {
	// No-op for now in GPU renderer, explicit AddFinding controls visibility
}

// drawFindings renders wireframe boxes for findings
func (gr *GPUVolumeRenderer) drawFindings(mvp [16]float32) {
	if len(gr.findings) == 0 {
		return
	}

	// Initialize line shader if needed
	if gr.lineProgram == 0 {
		var err error
		gr.lineProgram, err = gr.compileShaderWrapper(lineVertexShaderSource, lineFragmentShaderSource)
		if err != nil {
			slog.Error("Failed to compile line shader", "err", err)
			return
		}

		gl.GenVertexArrays(1, &gr.lineVAO)
		gl.GenBuffers(1, &gr.lineVBO)
		gr.lineMVPLoc = gl.GetUniformLocation(gr.lineProgram, gl.Str("mvp\x00"))
	}

	gl.UseProgram(gr.lineProgram)
	if gr.lineMVPLoc == -1 {
		slog.Error("MVP uniform not found in line shader")
	}
	gl.UniformMatrix4fv(gr.lineMVPLoc, 1, false, &mvp[0])

	if len(gr.findings) > 0 {
		// Log 4x4 matrix row-by-row for readability
		slog.Debug("Drawing Findings MVP",
			"Row0", fmt.Sprintf("%.2f %.2f %.2f %.2f", mvp[0], mvp[4], mvp[8], mvp[12]),
			"Row1", fmt.Sprintf("%.2f %.2f %.2f %.2f", mvp[1], mvp[5], mvp[9], mvp[13]),
			"Row2", fmt.Sprintf("%.2f %.2f %.2f %.2f", mvp[2], mvp[6], mvp[10], mvp[14]),
			"Row3", fmt.Sprintf("%.2f %.2f %.2f %.2f", mvp[3], mvp[7], mvp[11], mvp[15]))
	}

	// Enable depth test to draw correctly in 3D
	gl.Enable(gl.DEPTH_TEST)
	// Optionally clear depth buffer if volume drew to it?
	// Volume renderer usually draws to a quad and doesn't write meaningful depth.
	// So we might want to disable depth test or just draw on top?
	// Let's try drawing on top (disable depth test or clear depth)
	gl.Disable(gl.DEPTH_TEST)

	gl.BindVertexArray(gr.lineVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gr.lineVBO)

	// Build vertices for all boxes (Pos + Color)
	// 7 floats per vertex: X, Y, Z, R, G, B, A
	var vertices []float32

	for _, t := range gr.findings {
		// Use RefDim if provided (from caller), else fallback to first volume
		refDimX, refDimY, refDimZ := t.RefDimX, t.RefDimY, t.RefDimZ
		if refDimX <= 0 || refDimY <= 0 || refDimZ <= 0 {
			// Fallback to first volume dims if ref not provided
			if len(gr.volumes) == 0 {
				continue
			}
			vol := gr.volumes[0]
			refDimX, refDimY, refDimZ = vol.DimX, vol.DimY, vol.DimZ
		}
		if refDimX <= 0 || refDimY <= 0 || refDimZ <= 0 {
			continue
		}

		// Normalize bbox (0..1)
		minX := float32(t.BBox.X) / float32(refDimX)
		minY := float32(t.BBox.Y) / float32(refDimY)
		minZ := float32(t.BBox.Z) / float32(refDimZ) * float32(gr.scaleZ)
		maxX := float32(t.BBox.X+t.BBox.Width) / float32(refDimX)
		maxY := float32(t.BBox.Y+t.BBox.Height) / float32(refDimY)
		maxZ := float32(t.BBox.Z+t.BBox.Depth) / float32(refDimZ) * float32(gr.scaleZ)

		// Debug Log
		slog.Debug("Finding BBox Norm", "name", t.Name, "refDim", []int{refDimX, refDimY, refDimZ}, "minX", minX, "minY", minY, "minZ", minZ)

		// Colors
		r, g, b, a := float32(t.Color.R)/255.0, float32(t.Color.G)/255.0, float32(t.Color.B)/255.0, float32(t.Color.A)/255.0

		// Helper to append vertex
		addVert := func(x, y, z float32) {
			vertices = append(vertices, x, y, z, r, g, b, a)
		}

		// Lines (12 lines * 2 vertices)
		// Bottom Face
		addVert(minX, minY, minZ)
		addVert(maxX, minY, minZ)
		addVert(maxX, minY, minZ)
		addVert(maxX, maxY, minZ)
		addVert(maxX, maxY, minZ)
		addVert(minX, maxY, minZ)
		addVert(minX, maxY, minZ)
		addVert(minX, minY, minZ)

		// Top Face
		addVert(minX, minY, maxZ)
		addVert(maxX, minY, maxZ)
		addVert(maxX, minY, maxZ)
		addVert(maxX, maxY, maxZ)
		addVert(maxX, maxY, maxZ)
		addVert(minX, maxY, maxZ)
		addVert(minX, maxY, maxZ)
		addVert(minX, minY, maxZ)

		// Vertical Lines
		addVert(minX, minY, minZ)
		addVert(minX, minY, maxZ)
		addVert(maxX, minY, minZ)
		addVert(maxX, minY, maxZ)
		addVert(maxX, maxY, minZ)
		addVert(maxX, maxY, maxZ)
		addVert(minX, maxY, minZ)
		addVert(minX, maxY, maxZ)
	}

	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.DYNAMIC_DRAW)

	// Set layout
	// Attribute 0: Position (3 floats)
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, 7*4, gl.PtrOffset(0))

	// Attribute 1: Color (4 floats)
	gl.EnableVertexAttribArray(1)
	gl.VertexAttribPointer(1, 4, gl.FLOAT, false, 7*4, gl.PtrOffset(3*4))

	gl.LineWidth(2.0)
	gl.DrawArrays(gl.LINES, 0, int32(len(vertices)/7))

	gl.DisableVertexAttribArray(0)
	gl.DisableVertexAttribArray(1)
}

// compileShaderWrapper is a helper since we can't add methods to existing initGL easily
func (gr *GPUVolumeRenderer) compileShaderWrapper(vSource, fSource string) (uint32, error) {
	vShader, err := compileShader(vSource, gl.VERTEX_SHADER)
	if err != nil {
		return 0, err
	}
	fShader, err := compileShader(fSource, gl.FRAGMENT_SHADER)
	if err != nil {
		return 0, err
	}

	program := gl.CreateProgram()
	gl.AttachShader(program, vShader)
	gl.AttachShader(program, fShader)
	gl.LinkProgram(program)

	var status int32
	gl.GetProgramiv(program, gl.LINK_STATUS, &status)
	if status == gl.FALSE {
		var logLength int32
		gl.GetProgramiv(program, gl.INFO_LOG_LENGTH, &logLength)
		log := strings.Repeat("\x00", int(logLength+1))
		gl.GetProgramInfoLog(program, logLength, nil, gl.Str(log))
		return 0, fmt.Errorf("failed to link program: %v", log)
	}

	gl.DeleteShader(vShader)
	gl.DeleteShader(fShader)
	return program, nil
}
