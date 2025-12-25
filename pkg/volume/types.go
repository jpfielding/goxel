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

// Note: ColorBand is defined in config.go
