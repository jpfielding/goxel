package module

import (
	"fmt"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// CTImageModule represents the CT Image Module attributes
// Per NEMA IIC 1 v04-2023 Section A.3 (CT Image IOD)
type CTImageModule struct {
	// Required (Type 1)
	ImageType         []string // ORIGINAL\PRIMARY\AXIAL, etc.
	SamplesPerPixel   uint16   // Always 1 for CT
	PhotometricInterp string   // MONOCHROME2

	// Conditionally Required (Type 1C/2)
	RescaleIntercept float64 // Hounsfield offset
	RescaleSlope     float64 // Usually 1.0
	RescaleType      string  // "HU" for Hounsfield Units

	// Optional but recommended (Type 3)
	KVP                    float64 // Peak kilo voltage (kV)
	DataCollectionDiameter float64 // Scan FOV diameter (mm)
	ReconstructionDiameter float64 // Reconstruction diameter (mm)
	GantryDetectorTilt     float64 // Gantry tilt angle (degrees)
	TableHeight            float64 // Table height (mm)
	RotationDirection      string  // "CW" or "CC"
	ExposureTime           int     // Exposure time (ms)
	XRayTubeCurrent        int     // Tube current (mA)
	Exposure               int     // Exposure (mAs)
	FilterType             string  // Filter material
	ConvolutionKernel      string  // Reconstruction kernel
	GeneratorPower         int     // Generator power (kW)
	FocalSpots             float64 // Focal spot size (mm)
	DateOfLastCalibration  Date    // Calibration date
	TimeOfLastCalibration  Time    // Calibration time

	// Spiral/Helical CT parameters
	SpiralPitchFactor      float64 // Pitch factor
	TableSpeed             float64 // Table speed (mm/s)
	TableFeedPerRotation   float64 // Feed per rotation (mm)
	SingleCollimationWidth float64 // Single collimation width (mm)
	TotalCollimationWidth  float64 // Total collimation width (mm)
	AcquisitionType        string  // "SPIRAL", "CONSTANT_ANGLE", "STATIONARY", "FREE"

	// Window/Level for display
	WindowCenter float64
	WindowWidth  float64
}

// NewCTImageModule creates a CTImageModule with default values
func NewCTImageModule() *CTImageModule {
	return &CTImageModule{
		ImageType:         []string{"ORIGINAL", "PRIMARY", "AXIAL"},
		SamplesPerPixel:   1,
		PhotometricInterp: "MONOCHROME2",
		RescaleIntercept:  0.0,
		RescaleSlope:      1.0,
		RescaleType:       "HU",
		RotationDirection: "CW",
	}
}

// ToTags converts the module to DICOM tag elements
func (m *CTImageModule) ToTags() []IODElement {
	elements := []IODElement{
		{Tag: tag.ImageType, Value: formatMultiValue(m.ImageType)},
		{Tag: tag.SamplesPerPixel, Value: m.SamplesPerPixel},
		{Tag: tag.PhotometricInterpretation, Value: m.PhotometricInterp},
		{Tag: tag.RescaleIntercept, Value: formatDS(m.RescaleIntercept)},
		{Tag: tag.RescaleSlope, Value: formatDS(m.RescaleSlope)},
		{Tag: tag.RescaleType, Value: m.RescaleType},
	}

	// Add optional elements if set
	if m.KVP != 0 {
		elements = append(elements, IODElement{Tag: tag.KVP, Value: formatDS(m.KVP)})
	}
	if m.DataCollectionDiameter != 0 {
		elements = append(elements, IODElement{Tag: tag.DataCollectionDiameter, Value: formatDS(m.DataCollectionDiameter)})
	}
	if m.ReconstructionDiameter != 0 {
		elements = append(elements, IODElement{Tag: tag.ReconstructionDiameter, Value: formatDS(m.ReconstructionDiameter)})
	}
	if m.GantryDetectorTilt != 0 {
		elements = append(elements, IODElement{Tag: tag.GantryDetectorTilt, Value: formatDS(m.GantryDetectorTilt)})
	}
	if m.TableHeight != 0 {
		elements = append(elements, IODElement{Tag: tag.TableHeight, Value: formatDS(m.TableHeight)})
	}
	if m.RotationDirection != "" {
		elements = append(elements, IODElement{Tag: tag.RotationDirection, Value: m.RotationDirection})
	}
	if m.ExposureTime != 0 {
		elements = append(elements, IODElement{Tag: tag.ExposureTime, Value: formatIS(m.ExposureTime)})
	}
	if m.XRayTubeCurrent != 0 {
		elements = append(elements, IODElement{Tag: tag.XRayTubeCurrent, Value: formatIS(m.XRayTubeCurrent)})
	}
	if m.Exposure != 0 {
		elements = append(elements, IODElement{Tag: tag.Exposure, Value: formatIS(m.Exposure)})
	}
	if m.FilterType != "" {
		elements = append(elements, IODElement{Tag: tag.FilterType, Value: m.FilterType})
	}
	if m.ConvolutionKernel != "" {
		elements = append(elements, IODElement{Tag: tag.ConvolutionKernel, Value: m.ConvolutionKernel})
	}
	if m.GeneratorPower != 0 {
		elements = append(elements, IODElement{Tag: tag.GeneratorPower, Value: formatIS(m.GeneratorPower)})
	}
	if m.FocalSpots != 0 {
		elements = append(elements, IODElement{Tag: tag.FocalSpots, Value: formatDS(m.FocalSpots)})
	}
	if !m.DateOfLastCalibration.IsZero() {
		elements = append(elements, IODElement{Tag: tag.DateOfLastCalibration, Value: m.DateOfLastCalibration.String()})
	}
	if !m.TimeOfLastCalibration.IsZero() {
		elements = append(elements, IODElement{Tag: tag.TimeOfLastCalibration, Value: m.TimeOfLastCalibration.String()})
	}

	// Spiral CT parameters
	if m.SpiralPitchFactor != 0 {
		elements = append(elements, IODElement{Tag: tag.SpiralPitchFactor, Value: m.SpiralPitchFactor})
	}
	if m.TableSpeed != 0 {
		elements = append(elements, IODElement{Tag: tag.TableSpeed, Value: m.TableSpeed})
	}
	if m.TableFeedPerRotation != 0 {
		elements = append(elements, IODElement{Tag: tag.TableFeedPerRotation, Value: m.TableFeedPerRotation})
	}
	if m.SingleCollimationWidth != 0 {
		elements = append(elements, IODElement{Tag: tag.SingleCollimationWidth, Value: m.SingleCollimationWidth})
	}
	if m.TotalCollimationWidth != 0 {
		elements = append(elements, IODElement{Tag: tag.TotalCollimationWidth, Value: m.TotalCollimationWidth})
	}
	if m.AcquisitionType != "" {
		elements = append(elements, IODElement{Tag: tag.AcquisitionType, Value: m.AcquisitionType})
	}

	// Window/Level
	if m.WindowCenter != 0 || m.WindowWidth != 0 {
		elements = append(elements, IODElement{Tag: tag.WindowCenter, Value: formatDS(m.WindowCenter)})
		elements = append(elements, IODElement{Tag: tag.WindowWidth, Value: formatDS(m.WindowWidth)})
	}

	return elements
}

// Helper functions
func formatDS(v float64) string {
	return fmt.Sprintf("%g", v)
}

func formatIS(v int) string {
	return fmt.Sprintf("%d", v)
}

func formatMultiValue(values []string) string {
	if len(values) == 0 {
		return ""
	}
	result := values[0]
	for i := 1; i < len(values); i++ {
		result += "\\" + values[i]
	}
	return result
}

// IsZero checks if Date is uninitialized
func (d Date) IsZero() bool {
	return d.Year == 0 && d.Month == 0 && d.Day == 0
}

// IsZero checks if Time is uninitialized
func (t Time) IsZero() bool {
	return t.Hour == 0 && t.Minute == 0 && t.Second == 0 && t.Nano == 0
}
