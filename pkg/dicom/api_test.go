package dicom

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================================
// CT Image API Documentation Tests
// ============================================================================

// TestCTImage_BasicWorkflow demonstrates creating, configuring, and writing
// a CT image. This is the standard workflow for medical CT scans.
func TestCTImage_BasicWorkflow(t *testing.T) {
	// Create a new CT image with default values
	ct := NewCTImage()
	require.NotNil(t, ct)

	// Configure patient/study information
	ct.Patient.SetPatientName("Test", "Patient", "", "", "")
	ct.Patient.PatientID = "PAT-001"
	ct.Study.StudyDescription = "CT Scan"

	// Set image dimensions and pixel data
	rows, cols := 256, 256
	pixelData := make([]uint16, rows*cols)
	for i := range pixelData {
		pixelData[i] = uint16(i % 65536)
	}
	ct.Rows = rows
	ct.Columns = cols

	// Build the dataset
	dataset, err := ct.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)

	// Verify key elements are present
	sopClass, _ := dataset.FindElement(0x0008, 0x0016)
	assert.NotNil(t, sopClass, "SOP Class UID should be present")

	sopInstance, _ := dataset.FindElement(0x0008, 0x0018)
	assert.NotNil(t, sopInstance, "SOP Instance UID should be present")
}

// TestCTImage_WithCompression demonstrates enabling compression codecs.
func TestCTImage_WithCompression(t *testing.T) {
	ct := NewCTImage()
	ct.Rows = 128
	ct.Columns = 128

	// Enable JPEG-LS compression (lossless)
	ct.Codec = CodecJPEGLS

	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Transfer Syntax is set in file meta
	ts, exists := dataset.FindElement(0x0002, 0x0010)
	require.True(t, exists, "Transfer Syntax UID should be present")
	// Verify compression is configured (actual encoding happens when writing pixel data)
	assert.NotNil(t, ts.Value)
}

// TestCTImage_VOILUTPresets demonstrates window/level configuration.
func TestCTImage_VOILUTPresets(t *testing.T) {
	ct := NewCTImage()

	// CT images come with default presets for soft tissue, bone, lung, brain
	require.NotNil(t, ct.VOILUT, "VOI LUT module should be initialized")
	assert.GreaterOrEqual(t, len(ct.VOILUT.Windows), 1, "Should have at least one window preset")

	// Add custom window
	ct.VOILUT.AddWindow(100, 500, "CUSTOM")

	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Window Center should be present
	wc, exists := dataset.FindElement(0x0028, 0x1050)
	assert.True(t, exists, "Window Center should be present")
	assert.NotNil(t, wc)
}

// ============================================================================
// DX Image API Documentation Tests
// ============================================================================

// TestDXImage_BasicWorkflow demonstrates creating a DX (X-ray) image.
func TestDXImage_BasicWorkflow(t *testing.T) {
	dx := NewDXImage()
	require.NotNil(t, dx)

	// Configure for digital X-ray
	dx.Patient.PatientID = "DX-001"
	dx.Rows = 1024
	dx.Columns = 768

	// Set detector parameters
	require.NotNil(t, dx.Detector, "Detector module should be initialized")
	dx.Detector.DetectorType = "SCINTILLATOR"
	dx.Detector.FieldOfViewShape = "RECTANGLE"

	// Set acquisition parameters
	require.NotNil(t, dx.Acquisition, "Acquisition module should be initialized")
	dx.Acquisition.KVP = 140
	dx.Acquisition.XRayTubeCurrent = 200

	dataset, err := dx.GetDataset()
	require.NoError(t, err)
	require.NotNil(t, dataset)
}

// ============================================================================
// IOD Validation API Documentation Tests
// ============================================================================

// TestValidation_CTImage demonstrates validating a CT dataset.
func TestValidation_CTImage(t *testing.T) {
	ct := NewCTImage()
	ct.Rows = 256
	ct.Columns = 256

	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Validate against CT requirements
	result := ValidateCT(dataset)

	// Check validation result
	t.Logf("Valid: %v, Errors: %d, Warnings: %d",
		result.IsValid(), len(result.Errors), len(result.Warnings))

	// Log any errors for debugging
	for _, e := range result.Errors {
		t.Logf("Error: %s", e.Error())
	}
}

// ============================================================================
// Read/Write Roundtrip Tests
// ============================================================================

// TestCTImage_WriteAndRead demonstrates writing to file and reading back.
func TestCTImage_WriteAndRead(t *testing.T) {
	// Create temp directory
	tmpDir := t.TempDir()
	filePath := filepath.Join(tmpDir, "test.dcs")

	// Create and write CT image
	ct := NewCTImage()
	ct.Patient.PatientID = "ROUNDTRIP-001"
	ct.Rows = 64
	ct.Columns = 64

	// Create minimal pixel data
	pixelData := make([]uint16, 64*64)
	for i := range pixelData {
		pixelData[i] = uint16(i)
	}

	// Build dataset with pixel data
	dataset, err := ct.GetDataset()
	require.NoError(t, err)

	// Write to file
	f, err := os.Create(filePath)
	require.NoError(t, err)
	_, err = Write(f, dataset)
	f.Close()
	require.NoError(t, err)

	// Read back
	readDataset, err := ReadFile(filePath)
	require.NoError(t, err)
	require.NotNil(t, readDataset)

	// Verify patient ID matches
	patientID, exists := readDataset.FindElement(0x0010, 0x0020)
	require.True(t, exists, "Patient ID should be present")
	assert.Contains(t, patientID.Value, "ROUNDTRIP-001")
}

