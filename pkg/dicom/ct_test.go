package dicom_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jpfielding/goxel/pkg/dicom"
	"github.com/jpfielding/goxel/pkg/dicom/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCTImage_Write(t *testing.T) {
	ct := dicom.NewCTImage()
	ct.Patient.SetPatientName("Test", "Person", "", "", "")
	ct.Series.Modality = "CT"
	ct.Series.SeriesDescription = "Test Series"

	// Set dummy pixel data
	rows, cols := 10, 10
	data := make([]uint16, rows*cols)
	for i := range data {
		data[i] = uint16(i)
	}

	ct.SetPixelData(rows, cols, data)
	ct.Codec = nil // uncompressed

	// Use temp dir
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test_ct.dcs")

	_, err := ct.Write(tmpFile)
	require.NoError(t, err, "Failed to write CT Image")

	// Basic check if file exists
	require.FileExists(t, tmpFile, "File was not created")
}

func TestCTImage_WriteCompressed(t *testing.T) {
	ct := dicom.NewCTImage()
	ct.Patient.SetPatientName("Compressed", "Test", "", "", "")

	// Create a pattern that compresses well
	rows, cols := 512, 512
	data := make([]uint16, rows*cols)
	for i := range data {
		data[i] = uint16(i % 512)
	}

	ct.Rows = rows
	ct.Columns = cols
	ct.SetPixelData(rows, cols, data)
	ct.ContentDate = module.NewDate(time.Now())

	// Write uncompressed
	tmpDir := t.TempDir()
	ct.Codec = nil
	uncompressedFile := filepath.Join(tmpDir, "test_ct_uncompressed.dcs")
	_, err := ct.Write(uncompressedFile)
	require.NoError(t, err, "Failed to write uncompressed CT")

	// Write compressed
	ct.Codec = dicom.CodecJPEGLS
	compressedFile := filepath.Join(tmpDir, "test_ct_compressed.dcs")
	_, err = ct.Write(compressedFile)
	require.NoError(t, err, "Failed to write compressed CT")

	uncompStat, err := os.Stat(uncompressedFile)
	require.NoError(t, err, "Failed to stat uncompressed file")
	compStat, err := os.Stat(compressedFile)
	require.NoError(t, err, "Failed to stat compressed file")

	t.Logf("Uncompressed size: %d, Compressed size: %d", uncompStat.Size(), compStat.Size())

	assert.Less(t, compStat.Size(), uncompStat.Size(),
		"Compressed file (%d) should be smaller than uncompressed file (%d)",
		compStat.Size(), uncompStat.Size())

	// Verify Transfer Syntax
	ds, err := dicom.ReadFile(compressedFile)
	require.NoError(t, err, "Failed to read back compressed file")

	syntax := dicom.GetTransferSyntax(ds)
	assert.Equal(t, dicom.JPEGLSLossless, syntax, "Expected JPEG-LS Lossless transfer syntax")
}
