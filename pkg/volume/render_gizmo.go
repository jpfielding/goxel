package volume

import (
	"math"

	"github.com/go-gl/gl/v3.3-core/gl"
	"github.com/go-gl/mathgl/mgl32"
)

const gizmoVertexShader = `
#version 330 core
layout (location = 0) in vec3 aPos;
layout (location = 1) in vec4 aColor;
uniform mat4 model;
uniform mat4 view;
uniform mat4 projection;
out vec4 vertexColor;
void main() {
    gl_Position = projection * view * model * vec4(aPos, 1.0);
    vertexColor = aColor;
}
` + "\x00"

const gizmoFragmentShader = `
#version 330 core
in vec4 vertexColor;
out vec4 FragColor;
void main() {
    FragColor = vertexColor;
}
` + "\x00"

// initGizmo initializes the gizmo resources
func (gr *GPUVolumeRenderer) initGizmo() error {
	var err error
	gr.gizmoProgram, err = gr.compileShaderWrapper(gizmoVertexShader, gizmoFragmentShader)
	if err != nil {
		return err
	}

	// Axis lines: Origin -> X (Red), Origin -> Y (Green), Origin -> Z (Blue)
	vertices := []float32{
		// Pos (XYZ)       // Color (RGBA)
		0.0, 0.0, 0.0, 1.0, 0.0, 0.0, 1.0, // Origin (Red start)
		1.0, 0.0, 0.0, 1.0, 0.0, 0.0, 1.0, // X axis (Red end)

		0.0, 0.0, 0.0, 0.0, 1.0, 0.0, 1.0, // Origin (Green start)
		0.0, 1.0, 0.0, 0.0, 1.0, 0.0, 1.0, // Y axis (Green end)

		0.0, 0.0, 0.0, 0.0, 0.0, 1.0, 1.0, // Origin (Blue start)
		0.0, 0.0, 1.0, 0.0, 0.0, 1.0, 1.0, // Z axis (Blue end)
	}

	gl.GenVertexArrays(1, &gr.gizmoVAO)
	gl.GenBuffers(1, &gr.gizmoVBO)

	gl.BindVertexArray(gr.gizmoVAO)
	gl.BindBuffer(gl.ARRAY_BUFFER, gr.gizmoVBO)
	gl.BufferData(gl.ARRAY_BUFFER, len(vertices)*4, gl.Ptr(vertices), gl.STATIC_DRAW)

	stride := int32(7 * 4)
	gl.VertexAttribPointer(0, 3, gl.FLOAT, false, stride, gl.PtrOffset(0))
	gl.EnableVertexAttribArray(0)
	gl.VertexAttribPointer(1, 4, gl.FLOAT, false, stride, gl.PtrOffset(3*4))
	gl.EnableVertexAttribArray(1)

	return nil
}

// drawGizmo renders the orientation widget in the bottom-left corner
func (gr *GPUVolumeRenderer) drawGizmo() {
	if gr.gizmoProgram == 0 {
		return
	}

	gl.UseProgram(gr.gizmoProgram)
	gl.BindVertexArray(gr.gizmoVAO)

	// Disable depth test so it draws on top
	gl.Disable(gl.DEPTH_TEST)

	// Viewport for Gizmo (Bottom-Left, small)
	gizmoSize := int32(100)
	gl.Viewport(0, 0, gizmoSize, gizmoSize)

	// Projection: Orthographic box aligned with cameras
	// We want to rotate the gizmo same as the volume, but keep it centered in its viewport
	projection := mgl32.Ortho(-1.5, 1.5, -1.5, 1.5, 0.1, 10.0)

	// View: Extracted rotation from main camera view
	// Main camera is orbital. We just want the rotation component.
	// We can reconstruct the lookAt matrix without translation

	// Standard orbital camera position
	// camX := float32(gr.zoom * math.Sin(gr.rotationX) * math.Cos(gr.rotationY))
	// camY := float32(gr.zoom * math.Sin(gr.rotationY))
	// camZ := float32(gr.zoom * math.Cos(gr.rotationX) * math.Cos(gr.rotationY))

	// We place camera on unit sphere for gizmo
	zoom := 3.0 // Constant distance
	cx := float32(zoom * math.Sin(gr.rotationX) * math.Cos(gr.rotationY))
	cy := float32(zoom * math.Sin(gr.rotationY))
	cz := float32(zoom * math.Cos(gr.rotationX) * math.Cos(gr.rotationY))

	view := mgl32.LookAtV(mgl32.Vec3{cx, cy, cz}, mgl32.Vec3{0, 0, 0}, mgl32.Vec3{0, 1, 0})
	model := mgl32.Ident4() // Axis aligned at origin

	gl.UniformMatrix4fv(gl.GetUniformLocation(gr.gizmoProgram, gl.Str("projection\x00")), 1, false, &projection[0])
	gl.UniformMatrix4fv(gl.GetUniformLocation(gr.gizmoProgram, gl.Str("view\x00")), 1, false, &view[0])
	gl.UniformMatrix4fv(gl.GetUniformLocation(gr.gizmoProgram, gl.Str("model\x00")), 1, false, &model[0])

	gl.LineWidth(3.0)
	gl.DrawArrays(gl.LINES, 0, 6)
	gl.LineWidth(1.0)

	gl.Enable(gl.DEPTH_TEST)

	// Restore viewport
	gl.Viewport(0, 0, int32(gr.renderWidth), int32(gr.renderHeight))
}
