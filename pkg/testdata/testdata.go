package testdata

import (
	"embed"
	"os"
	"path/filepath"
)

//go:embed dicom/sample_ct.dcm
var files embed.FS

// SampleCT returns the embedded DICOM CT scan fixture bytes.
func SampleCT() ([]byte, error) {
	return files.ReadFile("dicom/sample_ct.dcm")
}

// WriteSampleCT writes the embedded CT fixture to dst.
func WriteSampleCT(dst string) error {
	return writeFile("dicom/sample_ct.dcm", dst)
}

func writeFile(embedPath, dst string) error {
	data, err := files.ReadFile(embedPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}
