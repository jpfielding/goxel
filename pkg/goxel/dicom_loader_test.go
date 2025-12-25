package goxel

import (
	"path/filepath"
	"testing"

	"github.com/jpfielding/goxel/pkg/testdata"
)

func writeEmbeddedDICOM(t *testing.T, dir, name string) string {
	t.Helper()

	path := filepath.Join(dir, name)
	err := testdata.WriteSampleCT(path)
	if err != nil {
		t.Fatalf("failed to write embedded DICOM fixture: %v", err)
	}

	return path
}

func TestLoadDICOM(t *testing.T) {
	tmpDir := t.TempDir()
	path := writeEmbeddedDICOM(t, tmpDir, "sample_ct.dcm")

	scan, err := LoadDICOM(path)
	if err != nil {
		t.Fatalf("LoadDICOM failed: %v", err)
	}

	if len(scan.Volumes) == 0 {
		t.Fatal("expected at least one volume")
	}

	primary := scan.PrimaryVolume()
	if primary == nil {
		t.Fatal("expected a primary volume")
	}

	if primary.Width <= 0 || primary.Height <= 0 || primary.Depth <= 0 {
		t.Fatalf("invalid volume dimensions: %dx%dx%d", primary.Width, primary.Height, primary.Depth)
	}
	if len(primary.Data) == 0 {
		t.Fatal("expected non-empty voxel data")
	}
}

func TestLoadDICOMDir(t *testing.T) {
	tmpDir := t.TempDir()
	_ = writeEmbeddedDICOM(t, tmpDir, "sample_ct.dcm")

	scan, err := LoadDICOMDir(tmpDir)
	if err != nil {
		t.Fatalf("LoadDICOMDir failed: %v", err)
	}

	if len(scan.Volumes) == 0 {
		t.Fatal("expected at least one volume")
	}

	primary := scan.PrimaryVolume()
	if primary == nil {
		t.Fatal("expected a primary volume")
	}

	if primary.Width <= 0 || primary.Height <= 0 || primary.Depth <= 0 {
		t.Fatalf("invalid volume dimensions: %dx%dx%d", primary.Width, primary.Height, primary.Depth)
	}
	if len(primary.Data) == 0 {
		t.Fatal("expected non-empty voxel data")
	}
}
