package goxel

import (
	"log/slog"
	"os"
	"testing"
)

func TestLoadDICOM(t *testing.T) {
	// Set debug logging
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	path := "/Users/fieldingj/Downloads/FIELDINGJEREMYPAUL/dicom/SER00001/IMG00001.dcm"
	scan, err := LoadDICOM(path)
	if err != nil {
		t.Fatalf("LoadDICOM failed: %v", err)
	}

	t.Logf("Loaded scan: %+v", scan)
	for name, vol := range scan.Volumes {
		t.Logf("Volume %s: %dx%dx%d, data len=%d", name, vol.Width, vol.Height, vol.Depth, len(vol.Data))
	}
}

func TestLoadDICOMDir(t *testing.T) {
	// Set info logging for directory test
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	path := "/Users/fieldingj/Downloads/FIELDINGJEREMYPAUL/dicom"
	scan, err := LoadDICOMDir(path)
	if err != nil {
		t.Fatalf("LoadDICOMDir failed: %v", err)
	}

	t.Logf("Loaded scan from directory")
	t.Logf("Patient: %s (%s)", scan.PatientName, scan.PatientID)
	t.Logf("Study: %s", scan.StudyDescription)
	for name, vol := range scan.Volumes {
		t.Logf("Volume %s: %dx%dx%d, data len=%d", name, vol.Width, vol.Height, vol.Depth, len(vol.Data))
	}
}
