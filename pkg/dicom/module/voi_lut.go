package module

import (
	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// VOILUTModule represents the VOI LUT (Value of Interest Lookup Table) Module
// Per DICOM Part 3 Section C.11.2
// Provides window/level and optional LUT-based transformations for display
type VOILUTModule struct {
	// Linear Window/Level (most common)
	// Multiple windows supported for different viewing presets
	Windows []WindowLevel

	// Optional: LUT-based transformation (VOI LUT Sequence)
	// Used when linear transformation is insufficient
	LUTs []VOILUT

	// VOI LUT Function - how to interpret window values
	// LINEAR (default), SIGMOID, or LINEAR_EXACT
	VOILUTFunction string
}

// WindowLevel represents a single window/level preset
type WindowLevel struct {
	Center      float64 // Window center value
	Width       float64 // Window width value
	Explanation string  // Optional description (e.g., "BONE", "SOFT TISSUE")
}

// VOILUT represents a VOI Lookup Table
type VOILUT struct {
	// LUT Descriptor: [number of entries, first input value, bits per entry]
	Descriptor [3]uint16
	// LUT Data - the actual lookup table values
	Data []uint16
	// Optional explanation
	Explanation string
}

// NewVOILUTModule creates a VOILUTModule with default CT soft tissue window
func NewVOILUTModule() *VOILUTModule {
	return &VOILUTModule{
		Windows: []WindowLevel{
			{Center: 40, Width: 400, Explanation: "SOFT_TISSUE"},
		},
		VOILUTFunction: "LINEAR",
	}
}

// NewVOILUTModuleForCT creates presets for common CT viewing windows
func NewVOILUTModuleForCT() *VOILUTModule {
	return &VOILUTModule{
		Windows: []WindowLevel{
			{Center: 40, Width: 400, Explanation: "SOFT_TISSUE"},
			{Center: 400, Width: 2000, Explanation: "BONE"},
			{Center: -600, Width: 1500, Explanation: "LUNG"},
			{Center: 50, Width: 350, Explanation: "BRAIN"},
		},
		VOILUTFunction: "LINEAR",
	}
}

// NewVOILUTModuleForDX creates presets for X-ray viewing
func NewVOILUTModuleForDX() *VOILUTModule {
	return &VOILUTModule{
		Windows: []WindowLevel{
			{Center: 32768, Width: 65535, Explanation: "DEFAULT"},
		},
		VOILUTFunction: "LINEAR",
	}
}

// AddWindow adds a window/level preset
func (m *VOILUTModule) AddWindow(center, width float64, explanation string) {
	m.Windows = append(m.Windows, WindowLevel{
		Center:      center,
		Width:       width,
		Explanation: explanation,
	})
}

// SetWindow sets a single window (clears any existing windows)
func (m *VOILUTModule) SetWindow(center, width float64) {
	m.Windows = []WindowLevel{{Center: center, Width: width}}
}

// ToTags converts the module to DICOM tag elements
func (m *VOILUTModule) ToTags() []IODElement {
	var elements []IODElement

	if len(m.Windows) > 0 {
		// Build multi-valued strings for Window Center and Width
		centers := ""
		widths := ""
		explanations := ""

		for i, w := range m.Windows {
			if i > 0 {
				centers += "\\"
				widths += "\\"
				explanations += "\\"
			}
			centers += formatDS(w.Center)
			widths += formatDS(w.Width)
			explanations += w.Explanation
		}

		elements = append(elements,
			IODElement{Tag: tag.WindowCenter, Value: centers},
			IODElement{Tag: tag.WindowWidth, Value: widths},
		)

		// Window Center/Width Explanation is optional
		if hasExplanations(m.Windows) {
			elements = append(elements, IODElement{Tag: tag.WindowCenterWidthExplanation, Value: explanations})
		}
	}

	// VOI LUT Function
	if m.VOILUTFunction != "" && m.VOILUTFunction != "LINEAR" {
		elements = append(elements, IODElement{Tag: tag.VOILUTFunction, Value: m.VOILUTFunction})
	}

	// VOI LUT Sequence (if LUTs defined)
	// Note: Sequence handling would require additional builder support
	// For now, we only support linear window/level

	return elements
}

// hasExplanations checks if any window has an explanation
func hasExplanations(windows []WindowLevel) bool {
	for _, w := range windows {
		if w.Explanation != "" {
			return true
		}
	}
	return false
}
