package module

import (
	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// FrameOfReferenceModule represents the Frame of Reference Module
// Per DICOM Part 3 Section C.7.4.1
type FrameOfReferenceModule struct {
	// Required (Type 1)
	FrameOfReferenceUID string // Unique identifier for spatial frame

	// Optional (Type 2)
	PositionReferenceIndicator string // Anatomical reference point (e.g., "VERTEX", "NA")
}

// NewFrameOfReferenceModule creates a new FrameOfReferenceModule with generated UID
func NewFrameOfReferenceModule(uidPrefix string) *FrameOfReferenceModule {
	return &FrameOfReferenceModule{
		FrameOfReferenceUID:        generateUID(uidPrefix),
		PositionReferenceIndicator: "",
	}
}

// ToTags converts the module to DICOM tag elements
func (m *FrameOfReferenceModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.FrameOfReferenceUID, Value: m.FrameOfReferenceUID},
		{Tag: tag.PositionReferenceIndicator, Value: m.PositionReferenceIndicator},
	}
}

// ImagePlaneModule represents the Image Plane Module
// Per DICOM Part 3 Section C.7.6.2
type ImagePlaneModule struct {
	// Required (Type 1)
	PixelSpacing            [2]float64 // Row\Column spacing (mm)
	ImageOrientationPatient [6]float64 // Direction cosines (row_x, row_y, row_z, col_x, col_y, col_z)
	ImagePositionPatient    [3]float64 // Position of upper-left corner (x, y, z)

	// Conditionally Required (Type 1C)
	SliceThickness       float64 // Slice thickness (mm)
	SpacingBetweenSlices float64 // Spacing between slices (mm)

	// Optional (Type 3)
	SliceLocation float64 // Relative position of slice (mm)
}

// NewImagePlaneModule creates an ImagePlaneModule with default identity orientation
func NewImagePlaneModule() *ImagePlaneModule {
	return &ImagePlaneModule{
		PixelSpacing:            [2]float64{1.0, 1.0},
		ImageOrientationPatient: [6]float64{1, 0, 0, 0, 1, 0}, // Identity: rows along X, cols along Y
		ImagePositionPatient:    [3]float64{0, 0, 0},
		SliceThickness:          1.0,
	}
}

// ToTags converts the module to DICOM tag elements
func (m *ImagePlaneModule) ToTags() []IODElement {
	elements := []IODElement{
		{Tag: tag.PixelSpacing, Value: formatDSPair(m.PixelSpacing[0], m.PixelSpacing[1])},
		{Tag: tag.ImageOrientationPatient, Value: formatDS6(m.ImageOrientationPatient)},
		{Tag: tag.ImagePositionPatient, Value: formatDS3(m.ImagePositionPatient)},
	}

	if m.SliceThickness != 0 {
		elements = append(elements, IODElement{Tag: tag.SliceThickness, Value: formatDS(m.SliceThickness)})
	}
	if m.SpacingBetweenSlices != 0 {
		elements = append(elements, IODElement{Tag: tag.SpacingBetweenSlices, Value: formatDS(m.SpacingBetweenSlices)})
	}
	if m.SliceLocation != 0 {
		elements = append(elements, IODElement{Tag: tag.SliceLocation, Value: formatDS(m.SliceLocation)})
	}

	return elements
}

// Helper formatters for DS multi-value strings
func formatDSPair(a, b float64) string {
	return formatDS(a) + "\\" + formatDS(b)
}

func formatDS3(v [3]float64) string {
	return formatDS(v[0]) + "\\" + formatDS(v[1]) + "\\" + formatDS(v[2])
}

func formatDS6(v [6]float64) string {
	return formatDS(v[0]) + "\\" + formatDS(v[1]) + "\\" + formatDS(v[2]) + "\\" +
		formatDS(v[3]) + "\\" + formatDS(v[4]) + "\\" + formatDS(v[5])
}

// Simple UID generation placeholder - actual impl in dicos package
func generateUID(prefix string) string {
	// Placeholder - caller should use dicos.GenerateUID
	return prefix + "1.2.3.4.5"
}
