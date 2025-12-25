package module

import (
	"fmt"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// DXDetectorModule represents the DX Detector Module
// Per DICOM Part 3 Section C.8.11.4 and DICOM extensions
type DXDetectorModule struct {
	// Detector Identification
	DetectorType          string // DIRECT, SCINTILLATOR, STORAGE
	DetectorConfiguration string // SLOT, AREA
	DetectorDescription   string
	DetectorID            string
	DetectorManufacturer  string
	DetectorModel         string

	// Detector Conditions
	DetectorConditionsNominal bool    // YES/NO
	DetectorTemperature       float64 // Celsius

	// Detector Geometry
	DetectorElementPhysicalSize float64 // mm
	DetectorElementSpacing      float64 // mm
	DetectorBinning             float64 // binning factor

	// Field of View
	FieldOfViewShape      string  // RECTANGLE, ROUND, HEXAGONAL
	FieldOfViewDimensions float64 // mm
}

// DXAcquisitionModule represents X-ray acquisition parameters
type DXAcquisitionModule struct {
	// X-Ray Generation
	KVP                 float64 // Peak kilovoltage
	XRayTubeCurrent     float64 // mA
	ExposureTime        float64 // ms
	Exposure            float64 // mAs
	FilterType          string  // Filter material
	AnodeTargetMaterial string  // MOLYBDENUM, RHODIUM, TUNGSTEN
	FocalSpotSize       float64 // mm

	// Geometry
	DistanceSourceToDetector float64 // SID (mm)
	DistanceSourceToPatient  float64 // SOD (mm)

	// Exposure Control
	ExposureControlMode string  // MANUAL, AUTOMATIC
	ExposureStatus      string  // NORMAL, ABORTED
	SensitivityValue    float64 // ISO speed

	// Grid
	Grid string // FIXED, FOCUSED, RECIPROCATING, NONE

	// Dose
	ImageAndFluoroscopyAreaDoseProduct float64 // DAP (dGy*cm2)

	// Patient/Object
	BodyPartThickness float64 // mm
	CompressionForce  float64 // N
}

// NewDXDetectorModule creates a DXDetectorModule with defaults
func NewDXDetectorModule() *DXDetectorModule {
	return &DXDetectorModule{
		DetectorType:              "SCINTILLATOR",
		DetectorConfiguration:     "AREA",
		DetectorConditionsNominal: true,
	}
}

// NewDXAcquisitionModule creates a DXAcquisitionModule with defaults
func NewDXAcquisitionModule() *DXAcquisitionModule {
	return &DXAcquisitionModule{
		KVP:                 120.0, // Typical security X-ray
		ExposureControlMode: "AUTOMATIC",
		ExposureStatus:      "NORMAL",
	}
}

// ToTags converts DXDetectorModule to DICOM tag elements
func (m *DXDetectorModule) ToTags() []IODElement {
	var elements []IODElement

	if m.DetectorType != "" {
		elements = append(elements, IODElement{Tag: tag.DetectorType, Value: m.DetectorType})
	}
	if m.DetectorConfiguration != "" {
		elements = append(elements, IODElement{Tag: tag.DetectorConfiguration, Value: m.DetectorConfiguration})
	}
	if m.DetectorDescription != "" {
		elements = append(elements, IODElement{Tag: tag.DetectorDescription, Value: m.DetectorDescription})
	}
	if m.DetectorID != "" {
		elements = append(elements, IODElement{Tag: tag.DetectorID, Value: m.DetectorID})
	}
	if m.DetectorManufacturer != "" {
		elements = append(elements, IODElement{Tag: tag.DetectorManufacturerName, Value: m.DetectorManufacturer})
	}
	if m.DetectorModel != "" {
		elements = append(elements, IODElement{Tag: tag.DetectorManufacturerModelName, Value: m.DetectorModel})
	}

	// Conditions
	if m.DetectorConditionsNominal {
		elements = append(elements, IODElement{Tag: tag.DetectorConditionsNominalFlag, Value: "YES"})
	} else {
		elements = append(elements, IODElement{Tag: tag.DetectorConditionsNominalFlag, Value: "NO"})
	}
	if m.DetectorTemperature != 0 {
		elements = append(elements, IODElement{Tag: tag.DetectorTemperature, Value: formatDS(m.DetectorTemperature)})
	}

	// Geometry
	if m.DetectorElementPhysicalSize != 0 {
		elements = append(elements, IODElement{Tag: tag.DetectorElementPhysicalSize, Value: formatDS(m.DetectorElementPhysicalSize)})
	}
	if m.DetectorElementSpacing != 0 {
		elements = append(elements, IODElement{Tag: tag.DetectorElementSpacing, Value: formatDS(m.DetectorElementSpacing)})
	}
	if m.DetectorBinning != 0 {
		elements = append(elements, IODElement{Tag: tag.DetectorBinning, Value: formatDS(m.DetectorBinning)})
	}

	// FOV
	if m.FieldOfViewShape != "" {
		elements = append(elements, IODElement{Tag: tag.FieldOfViewShape, Value: m.FieldOfViewShape})
	}
	if m.FieldOfViewDimensions != 0 {
		elements = append(elements, IODElement{Tag: tag.FieldOfViewDimensions, Value: fmt.Sprintf("%d", int(m.FieldOfViewDimensions))})
	}

	return elements
}

// ToTags converts DXAcquisitionModule to DICOM tag elements
func (m *DXAcquisitionModule) ToTags() []IODElement {
	var elements []IODElement

	// X-Ray Generation
	if m.KVP != 0 {
		elements = append(elements, IODElement{Tag: tag.KVP, Value: formatDS(m.KVP)})
	}
	if m.XRayTubeCurrent != 0 {
		elements = append(elements, IODElement{Tag: tag.XRayTubeCurrentInmA, Value: formatDS(m.XRayTubeCurrent)})
	}
	if m.ExposureTime != 0 {
		elements = append(elements, IODElement{Tag: tag.ExposureTimeInms, Value: m.ExposureTime})
	}
	if m.Exposure != 0 {
		elements = append(elements, IODElement{Tag: tag.Exposure, Value: fmt.Sprintf("%d", int(m.Exposure))})
	}
	if m.FilterType != "" {
		elements = append(elements, IODElement{Tag: tag.FilterType, Value: m.FilterType})
	}
	if m.AnodeTargetMaterial != "" {
		elements = append(elements, IODElement{Tag: tag.AnodeTargetMaterial, Value: m.AnodeTargetMaterial})
	}
	if m.FocalSpotSize != 0 {
		elements = append(elements, IODElement{Tag: tag.FocalSpotSize, Value: formatDS(m.FocalSpotSize)})
	}

	// Geometry
	if m.DistanceSourceToDetector != 0 {
		elements = append(elements, IODElement{Tag: tag.DistanceSourceToDetector, Value: formatDS(m.DistanceSourceToDetector)})
	}
	if m.DistanceSourceToPatient != 0 {
		elements = append(elements, IODElement{Tag: tag.DistanceSourceToPatient, Value: formatDS(m.DistanceSourceToPatient)})
	}

	// Exposure Control
	if m.ExposureControlMode != "" {
		elements = append(elements, IODElement{Tag: tag.ExposureControlMode, Value: m.ExposureControlMode})
	}
	if m.ExposureStatus != "" {
		elements = append(elements, IODElement{Tag: tag.ExposureStatus, Value: m.ExposureStatus})
	}
	if m.SensitivityValue != 0 {
		elements = append(elements, IODElement{Tag: tag.SensitivityValue, Value: formatDS(m.SensitivityValue)})
	}

	// Grid
	if m.Grid != "" {
		elements = append(elements, IODElement{Tag: tag.Grid, Value: m.Grid})
	}

	// Dose
	if m.ImageAndFluoroscopyAreaDoseProduct != 0 {
		elements = append(elements, IODElement{Tag: tag.ImageAndFluoroscopyAreaDoseProduct, Value: formatDS(m.ImageAndFluoroscopyAreaDoseProduct)})
	}

	// Patient/Object
	if m.BodyPartThickness != 0 {
		elements = append(elements, IODElement{Tag: tag.BodyPartThickness, Value: formatDS(m.BodyPartThickness)})
	}
	if m.CompressionForce != 0 {
		elements = append(elements, IODElement{Tag: tag.CompressionForce, Value: formatDS(m.CompressionForce)})
	}

	return elements
}
