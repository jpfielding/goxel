package dicom

import (
	"bytes"
	"testing"

	"github.com/jpfielding/goxel/pkg/compress/jpegli"
	"github.com/jpfielding/goxel/pkg/dicom/tag"
	"github.com/stretchr/testify/assert"
)

// TestSignedPixelOffset verifies that signed 16-bit JPEG data
// is correctly handled by applying/expecting the DICOM standard offset.
//
// Per DICOM PS3.5 8.2.1:
// Signed data stored in JPEG Lossless has +32768 added before encoding.
// Decoders must subtract 32768 to restore original values.
//
// This test uses example DICOS files which follow this standard,
// although some may incorrectly tag the pixel representation as Unsigned (0).
func TestSignedPixelOffset(t *testing.T) {
	// 1. Load DICOS file (known to be signed CT data, encapsulated)
	testPath := "testdata/example.dcs"
	ds, err := ReadFile(testPath)
	if err != nil {
		t.Skipf("Test file not found: %v", err)
	}

	// 2. Verify Bits Stored
	bitsStored := 0
	if el, ok := ds.FindElement(tag.BitsStored.Group, tag.BitsStored.Element); ok {
		if val, ok := el.GetInt(); ok {
			bitsStored = val
		}
	}
	assert.Equal(t, 16, bitsStored, "BitsStored should be 16")

	// Verify Heuristic Intercept
	// Since PixelRep=0 but it is CT, our heuristic provides the missing -32768
	intercept, _ := GetRescale(ds)
	assert.Equal(t, -32768.0, intercept, "Heuristic should return -32768 intercept")

	// 3. Extract Raw JPEG Data (from first frame)
	pd, err := ds.GetPixelData()
	assert.NoError(t, err)
	assert.True(t, pd.IsEncapsulated, "Data should be encapsulated")
	assert.NotEmpty(t, pd.Frames, "Should have frames")

	frameData := pd.Frames[0].CompressedData

	// 4. Decode RAW JPEG (without offset correction)
	img, err := jpegli.Decode(bytes.NewReader(frameData))
	assert.NoError(t, err)

	// Check raw values - expecting them to be offset by ~32768
	// Real data range is near 0, so encoded values should be near 32768.
	bounds := img.Bounds()
	rawMin, rawMax := 65535, 0

	// Sample a subset for speed
	for y := 0; y < bounds.Dy(); y += 10 {
		for x := 0; x < bounds.Dx(); x += 10 {
			r, _, _, _ := img.At(x, y).RGBA()
			val := int(r)
			if val < rawMin {
				rawMin = val
			}
			if val > rawMax {
				rawMax = val
			}
		}
	}

	t.Logf("Raw JPEG Values: Min=%d, Max=%d", rawMin, rawMax)

	// Assert raw values are in the offset range
	assert.Greater(t, rawMin, 30000, "Raw encoded values should have +32768 offset")

	// 5. Verify Corrected Range
	// When we apply the heuristic intercept, we should get proper CT values
	correctedMin := float64(rawMin) + intercept
	correctedMax := float64(rawMax) + intercept

	t.Logf("Corrected Values: Min=%.0f, Max=%.0f", correctedMin, correctedMax)

	assert.Less(t, correctedMin, 1000.0, "Corrected values should be in normal CT range (near 0)")
	assert.Greater(t, correctedMin, -1000.0, "Corrected values should be in normal CT range (near 0)")
}
