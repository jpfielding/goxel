package dicom

import (
	"fmt"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// AttributeType represents DICOM attribute type requirements
type AttributeType int

const (
	// Type1 - Required, must have value
	Type1 AttributeType = 1
	// Type1C - Conditionally required, must have value if present
	Type1C AttributeType = 2
	// Type2 - Required, may be empty
	Type2 AttributeType = 3
	// Type2C - Conditionally required, may be empty if present
	Type2C AttributeType = 4
	// Type3 - Optional
	Type3 AttributeType = 5
)

// ValidationError represents a single validation failure
type ValidationError struct {
	Tag        tag.Tag
	Type       AttributeType
	Message    string
	IsCritical bool // Type 1 and 1C violations are critical
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("(%04X,%04X) %s: %s", e.Tag.Group, e.Tag.Element, e.typeName(), e.Message)
}

func (e ValidationError) typeName() string {
	switch e.Type {
	case Type1:
		return "Type 1"
	case Type1C:
		return "Type 1C"
	case Type2:
		return "Type 2"
	case Type2C:
		return "Type 2C"
	case Type3:
		return "Type 3"
	default:
		return "Unknown"
	}
}

// ValidationResult contains all validation errors for a dataset
type ValidationResult struct {
	Errors   []ValidationError
	Warnings []ValidationError
}

// IsValid returns true if there are no critical errors
func (r ValidationResult) IsValid() bool {
	for _, err := range r.Errors {
		if err.IsCritical {
			return false
		}
	}
	return true
}

// HasErrors returns true if there are any errors
func (r ValidationResult) HasErrors() bool {
	return len(r.Errors) > 0
}

// HasWarnings returns true if there are any warnings
func (r ValidationResult) HasWarnings() bool {
	return len(r.Warnings) > 0
}

// IODRequirement defines a required attribute for an IOD
type IODRequirement struct {
	Tag       tag.Tag
	Type      AttributeType
	Condition func(*Dataset) bool // For Type 1C/2C, returns true if attribute is required
}

// ValidateDataset validates a dataset against a set of requirements
func ValidateDataset(ds *Dataset, requirements []IODRequirement) ValidationResult {
	result := ValidationResult{}

	for _, req := range requirements {
		elem, exists := ds.FindElement(req.Tag.Group, req.Tag.Element)

		switch req.Type {
		case Type1:
			if !exists {
				result.Errors = append(result.Errors, ValidationError{
					Tag:        req.Tag,
					Type:       Type1,
					Message:    "Required attribute missing",
					IsCritical: true,
				})
			} else if isEmpty(elem) {
				result.Errors = append(result.Errors, ValidationError{
					Tag:        req.Tag,
					Type:       Type1,
					Message:    "Required attribute is empty",
					IsCritical: true,
				})
			}

		case Type1C:
			if req.Condition != nil && req.Condition(ds) {
				if !exists {
					result.Errors = append(result.Errors, ValidationError{
						Tag:        req.Tag,
						Type:       Type1C,
						Message:    "Conditionally required attribute missing",
						IsCritical: true,
					})
				} else if isEmpty(elem) {
					result.Errors = append(result.Errors, ValidationError{
						Tag:        req.Tag,
						Type:       Type1C,
						Message:    "Conditionally required attribute is empty",
						IsCritical: true,
					})
				}
			}

		case Type2:
			if !exists {
				result.Warnings = append(result.Warnings, ValidationError{
					Tag:        req.Tag,
					Type:       Type2,
					Message:    "Required attribute missing (may be empty)",
					IsCritical: false,
				})
			}

		case Type2C:
			if req.Condition != nil && req.Condition(ds) && !exists {
				result.Warnings = append(result.Warnings, ValidationError{
					Tag:        req.Tag,
					Type:       Type2C,
					Message:    "Conditionally required attribute missing (may be empty)",
					IsCritical: false,
				})
			}

		case Type3:
			// Optional - no validation needed
		}
	}

	return result
}

// isEmpty checks if an element has no value
func isEmpty(elem *Element) bool {
	if elem == nil {
		return true
	}
	if elem.Value == nil {
		return true
	}
	switch v := elem.Value.(type) {
	case string:
		return v == ""
	case []byte:
		return len(v) == 0
	case []uint16:
		return len(v) == 0
	default:
		return false
	}
}

// Common IOD Requirements

// PatientModuleRequirements defines required attributes for Patient Module
var PatientModuleRequirements = []IODRequirement{
	{Tag: tag.PatientName, Type: Type2},
	{Tag: tag.PatientID, Type: Type2},
}

// GeneralStudyModuleRequirements defines required attributes for General Study Module
var GeneralStudyModuleRequirements = []IODRequirement{
	{Tag: tag.StudyInstanceUID, Type: Type1},
	{Tag: tag.StudyDate, Type: Type2},
	{Tag: tag.StudyTime, Type: Type2},
}

// GeneralSeriesModuleRequirements defines required attributes for General Series Module
var GeneralSeriesModuleRequirements = []IODRequirement{
	{Tag: tag.Modality, Type: Type1},
	{Tag: tag.SeriesInstanceUID, Type: Type1},
}

// ImagePixelModuleRequirements defines required attributes for Image Pixel Module
var ImagePixelModuleRequirements = []IODRequirement{
	{Tag: tag.SamplesPerPixel, Type: Type1},
	{Tag: tag.PhotometricInterpretation, Type: Type1},
	{Tag: tag.Rows, Type: Type1},
	{Tag: tag.Columns, Type: Type1},
	{Tag: tag.BitsAllocated, Type: Type1},
	{Tag: tag.BitsStored, Type: Type1},
	{Tag: tag.HighBit, Type: Type1},
	{Tag: tag.PixelRepresentation, Type: Type1},
	{Tag: tag.PixelData, Type: Type1},
}

// SOPCommonModuleRequirements defines required attributes for SOP Common Module
var SOPCommonModuleRequirements = []IODRequirement{
	{Tag: tag.SOPClassUID, Type: Type1},
	{Tag: tag.SOPInstanceUID, Type: Type1},
}

// CTImageRequirements combines all requirements for CT Image IOD
var CTImageRequirements = append(append(append(append(append(
	PatientModuleRequirements,
	GeneralStudyModuleRequirements...),
	GeneralSeriesModuleRequirements...),
	ImagePixelModuleRequirements...),
	SOPCommonModuleRequirements...),
	// CT-specific
	IODRequirement{Tag: tag.RescaleIntercept, Type: Type1},
	IODRequirement{Tag: tag.RescaleSlope, Type: Type1},
)

// DXImageRequirements combines all requirements for DX Image IOD
var DXImageRequirements = append(append(append(append(
	PatientModuleRequirements,
	GeneralStudyModuleRequirements...),
	GeneralSeriesModuleRequirements...),
	ImagePixelModuleRequirements...),
	SOPCommonModuleRequirements...)

// ValidateCT validates a CT Image dataset
func ValidateCT(ds *Dataset) ValidationResult {
	return ValidateDataset(ds, CTImageRequirements)
}

// ValidateDX validates a DX Image dataset
func ValidateDX(ds *Dataset) ValidationResult {
	return ValidateDataset(ds, DXImageRequirements)
}
