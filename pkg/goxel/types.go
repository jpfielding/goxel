// Package goxel provides the main viewer functionality for DICOM files.
package goxel

import (
	"image/color"
	"strings"

	"github.com/jpfielding/goxel/pkg/volume"
)

// Type aliases for volume package types
type Frame = volume.Frame
type PixelData = volume.PixelData
type BoundingBox3D = volume.BoundingBox3D

// CompositeVolume represents a volume for composite rendering.
type CompositeVolume struct {
	Name       string
	SourcePath string // Path to the source file(s) for this volume
	PixelData  *volume.PixelData
	Rows       int
	Cols       int
	PixelRep   int
	RescaleIntercept float64
	WindowCenter     float64
	WindowWidth      float64
	Enabled          bool
	Color            color.RGBA
	Alpha            float64
	BBox             *volume.BoundingBox3D
	// Voxel spacing for aspect ratio calculation
	VoxelSizeX float64
	VoxelSizeY float64
	VoxelSizeZ float64
}

// ScanCollection is the container for all renderable data from a scan.
type ScanCollection struct {
	// DICOM Patient Module
	PatientID   string
	PatientName string

	// DICOM Study Module
	StudyInstanceUID string
	StudyDate        string
	StudyTime        string
	StudyDescription string

	// DICOM Series Module
	SeriesInstanceUID  string
	SeriesDescription  string
	SeriesNumber       int
	Modality           string

	// DICOM General Equipment Module
	Manufacturer      string
	InstitutionName   string
	ManufacturerModel string

	// DICOM Image Module
	AcquisitionDate string
	AcquisitionTime string

	// Additional Info
	OperatorName string
	SourcePath   string

	// Volumes keyed by name (e.g., "he", "le", "volume")
	Volumes map[string]*VolumeData

	// Findings detected in the scan (regions of interest, annotations, or CAD detections)
	Findings []*FindingData

	// Projections (2D views if available)
	Projections map[string]*ProjectionData

	// Display parameters
	WindowLevel float64
	WindowWidth float64

	// Voxel spacing
	VoxelSpacingX float64
	VoxelSpacingY float64
	VoxelSpacingZ float64

	// ROI offsets
	RoiStartX, RoiStartY, RoiStartZ int
}

// VolumeData represents a single 3D volume.
type VolumeData struct {
	Name       string
	SourcePath string // Path to the source file(s) for this volume

	Width  int
	Height int
	Depth  int

	VoxelSizeX float64
	VoxelSizeY float64
	VoxelSizeZ float64

	Data     []uint16
	PixelRep int

	RescaleIntercept float64
	RescaleSlope     float64
}

// FindingData represents a detected finding region (lesion, nodule, ROI, etc).
type FindingData struct {
	Name     string
	Type     int
	Category string

	Density float64
	Zeff    float64
	Mass    float64

	Overlay *VolumeData
	BBox    FindingBoundingBox

	Visible bool
	Color   color.RGBA
}

// FindingBoundingBox represents a 3D axis-aligned bounding box.
type FindingBoundingBox struct {
	MinX, MinY, MinZ int
	MaxX, MaxY, MaxZ int
}

// ProjectionData represents a 2D projection image.
type ProjectionData struct {
	Name   string
	Angle  string
	Type   string
	Width  int
	Height int
	Data   []uint16
}

// NewScanCollection creates an initialized ScanCollection with default values.
func NewScanCollection() *ScanCollection {
	return &ScanCollection{
		Volumes:     make(map[string]*VolumeData),
		Findings:    make([]*FindingData, 0),
		Projections: make(map[string]*ProjectionData),
		WindowLevel: 500,
		WindowWidth: 2000,
	}
}

// PrimaryVolume returns the best volume for initial display.
func (sc *ScanCollection) PrimaryVolume() *VolumeData {
	if len(sc.Volumes) == 0 {
		return nil
	}

	// CT scan priorities (legacy)
	priorities := []string{
		"he_high_res", "high_res_he",
		"le_high_res", "high_res_le",
		"he_low_res", "low_res_he",
		"le_low_res", "low_res_le",
		"he", "le",
		"volume",
	}

	for _, name := range priorities {
		if vol, ok := sc.Volumes[name]; ok {
			return vol
		}
	}

	// For MRI: prefer 3D high-res volumetric acquisitions
	// Look for volumes with "3D" in the name (these are true volumetric acquisitions)
	var best3D *VolumeData
	var best3DSize int
	for name, vol := range sc.Volumes {
		if strings.Contains(strings.ToUpper(name), "3D") {
			size := vol.Width * vol.Height * vol.Depth
			if size > best3DSize {
				best3DSize = size
				best3D = vol
			}
		}
	}
	if best3D != nil {
		return best3D
	}

	// Fallback: select the largest volume (most voxels)
	var largest *VolumeData
	var largestSize int
	for _, vol := range sc.Volumes {
		size := vol.Width * vol.Height * vol.Depth
		if size > largestSize {
			largestSize = size
			largest = vol
		}
	}
	return largest
}

// CalculateWindowFromData computes window/level from actual data values.
func CalculateWindowFromData(data []uint16) (level, width float64) {
	if len(data) == 0 {
		return 500, 2000
	}

	const maxSamples = 10000
	stride := len(data) / maxSamples
	if stride < 1 {
		stride = 1
	}

	var min, max uint16 = data[0], data[0]
	for i := 0; i < len(data); i += stride {
		if data[i] < min {
			min = data[i]
		}
		if data[i] > max {
			max = data[i]
		}
	}

	width = float64(max - min)
	if width < 1 {
		width = 1
	}
	level = float64(min) + width/2

	return level, width
}
