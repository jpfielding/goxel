package dicom

import (
	"path/filepath"
	"testing"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
	"github.com/jpfielding/goxel/pkg/testdata"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRescaleIntercept verifies that DICOM CT files have correct
// rescale intercept values for mapping stored pixel values to Hounsfield units.
func TestRescaleIntercept(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "sample_ct.dcm")
	require.NoError(t, testdata.WriteSampleCT(testPath))

	ds, err := ReadFile(testPath)
	require.NoError(t, err)

	// Verify Bits Stored
	bitsStored := 0
	if el, ok := ds.FindElement(tag.BitsStored.Group, tag.BitsStored.Element); ok {
		if val, ok := el.GetInt(); ok {
			bitsStored = val
		}
	}
	assert.Equal(t, 16, bitsStored, "BitsStored should be 16")

	// Verify Rescale Intercept is present and negative (CT data stored as unsigned)
	intercept, slope := GetRescale(ds)
	t.Logf("Rescale: intercept=%.1f, slope=%.1f", intercept, slope)
	assert.Less(t, intercept, 0.0, "Intercept should be negative for CT data")

	// Verify pixel data can be extracted
	pd, err := ds.GetPixelData()
	require.NoError(t, err)
	assert.NotEmpty(t, pd.Frames, "Should have frames")

	// Verify first frame has pixel data
	frame := pd.Frames[0]
	if pd.IsEncapsulated {
		assert.NotEmpty(t, frame.CompressedData, "Encapsulated frame should have compressed data")
	} else {
		assert.NotEmpty(t, frame.Data, "Native frame should have pixel data")

		// Apply rescale intercept and verify Hounsfield units range
		// Air = -1000 HU, Water = 0 HU, Bone = 1000+ HU
		// With intercept applied, we should see values in CT range
		rawMin, rawMax := 65535, 0
		for i := 0; i < len(frame.Data); i += 10 {
			val := int(frame.Data[i])
			if val < rawMin {
				rawMin = val
			}
			if val > rawMax {
				rawMax = val
			}
		}
		t.Logf("Raw pixel range: %d-%d", rawMin, rawMax)

		huMin := float64(rawMin)*slope + intercept
		huMax := float64(rawMax)*slope + intercept
		t.Logf("Hounsfield range: %.0f - %.0f", huMin, huMax)

		assert.Less(t, huMin, 0.0, "HU min should include air (negative)")
	}
}

// TestMultiFrameVolume verifies that the DICOM CT file loads as a multi-frame volume.
func TestMultiFrameVolume(t *testing.T) {
	tmpDir := t.TempDir()
	testPath := filepath.Join(tmpDir, "sample_ct.dcm")
	require.NoError(t, testdata.WriteSampleCT(testPath))

	ds, err := ReadFile(testPath)
	require.NoError(t, err)

	// Verify dimensions
	rows, cols := 0, 0
	if el, ok := ds.FindElement(tag.Rows.Group, tag.Rows.Element); ok {
		if val, ok := el.GetInt(); ok {
			rows = val
		}
	}
	if el, ok := ds.FindElement(tag.Columns.Group, tag.Columns.Element); ok {
		if val, ok := el.GetInt(); ok {
			cols = val
		}
	}
	assert.Greater(t, rows, 0, "Rows should be positive")
	assert.Greater(t, cols, 0, "Columns should be positive")
	t.Logf("Image dimensions: %dx%d", cols, rows)

	// Verify number of frames
	pd, err := ds.GetPixelData()
	require.NoError(t, err)
	assert.Greater(t, len(pd.Frames), 1, "Should have multiple frames (slices)")
	t.Logf("Number of frames: %d", len(pd.Frames))
}
