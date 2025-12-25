// Package volume provides volume rendering functionality.
package volume

import "image/color"

// Frame represents a single 2D image frame (slice) in a volume.
type Frame struct {
	Data           []uint16 // Raw pixel data (uncompressed)
	CompressedData []byte   // Compressed pixel data (JPEG, etc.)
}

// PixelData contains all frames of a volume dataset.
type PixelData struct {
	Frames         []Frame
	IsEncapsulated bool // True if frames are JPEG/RLE compressed
}

// BoundingBox3D represents a 3D bounding box with position and size.
type BoundingBox3D struct {
	X, Y, Z              int // Position (origin corner)
	Width, Height, Depth int // Size
}

// FindingInfo stores finding metadata including bounding box for visualization.
type FindingInfo struct {
	Name    string
	Type    int
	BBox    BoundingBox3D
	Color   color.RGBA
	Visible bool
}

// RenderParams captures runtime rendering controls shared across CPU/GPU paths.
type RenderParams struct {
	WindowLevel       float64
	WindowWidth       float64
	AlphaScale        float64
	ScaleZ            float64
	RescaleIntercept  float64
	StepSize          float64
	DensityThreshold  float64
	AmbientIntensity  float64
	DiffuseIntensity  float64
	SpecularIntensity float64
}

// Note: ColorBand is defined in config.go
