package goxel

import (
	"image/color"
	"log/slog"
	"math"
	"path/filepath"
	"sort"
)

// SetScanData installs a loaded study. It is the single entry point for all
// file and folder loading, and must run on the UI goroutine.
func (v *Viewer) SetScanData(scan *ScanCollection) error {
	slog.Info("SetScanData", slog.String("path", scan.SourcePath),
		slog.Int("volumes", len(scan.Volumes)),
		slog.Int("findings", len(scan.Findings)))

	primary := scan.PrimaryVolume()
	if primary == nil {
		slog.Warn("No volumes found in scan")
		return nil
	}
	v.scan = scan
	v.primary = primary
	if v.st.loadedName == "" {
		v.st.loadedPath = scan.SourcePath
		v.st.loadedName = filepath.Base(scan.SourcePath)
	}

	keys := make([]string, 0, len(scan.Volumes))
	for k := range scan.Volumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	v.compositeVolumes = make(map[string]*CompositeVolume)
	var primaryKey string
	for _, name := range keys {
		vol := scan.Volumes[name]
		if vol == primary {
			primaryKey = name
		}
		wl, ww := CalculateWindowFromData(vol.Data)
		v.compositeVolumes[name] = &CompositeVolume{
			Name:             name,
			SourcePath:       vol.SourcePath,
			PixelData:        framesOf(vol.Data, vol.Width, vol.Height, vol.Depth),
			Rows:             vol.Height,
			Cols:             vol.Width,
			PixelRep:         vol.PixelRep,
			RescaleIntercept: vol.RescaleIntercept,
			WindowCenter:     wl,
			WindowWidth:      ww,
			Enabled:          vol == primary, // only the primary volume shows by default
			Color:            color.RGBA{255, 255, 255, 255},
			Alpha:            0.5,
			VoxelSizeX:       vol.VoxelSizeX,
			VoxelSizeY:       vol.VoxelSizeY,
			VoxelSizeZ:       vol.VoxelSizeZ,
		}
	}

	// Finding overlays become disabled composite entries with a bounding box.
	for _, finding := range scan.Findings {
		if finding.Overlay == nil {
			continue
		}
		ov := finding.Overlay
		name := "finding_" + finding.Name
		v.compositeVolumes[name] = &CompositeVolume{
			Name:      name,
			PixelData: framesOf(ov.Data, ov.Width, ov.Height, ov.Depth),
			Rows:      ov.Height,
			Cols:      ov.Width,
			Alpha:     0.1,
			BBox: &BoundingBox3D{
				X: finding.BBox.MinX, Y: finding.BBox.MinY, Z: finding.BBox.MinZ,
				Width:  finding.BBox.MaxX - finding.BBox.MinX,
				Height: finding.BBox.MaxY - finding.BBox.MinY,
				Depth:  finding.BBox.MaxZ - finding.BBox.MinZ,
			},
		}
	}

	// Projections are 2D only.
	v.projections = make(map[string]*CompositeVolume)
	for name, proj := range scan.Projections {
		wl, ww := CalculateWindowFromData(proj.Data)
		v.projections[name] = &CompositeVolume{
			Name:         name,
			PixelData:    &PixelData{Frames: []Frame{{Data: proj.Data}}},
			Rows:         proj.Height,
			Cols:         proj.Width,
			WindowCenter: wl,
			WindowWidth:  ww,
			Enabled:      true,
			Color:        color.RGBA{255, 255, 255, 255},
		}
	}

	// Display defaults for a new study (roxel's: axial, single slice, fit).
	pc := v.compositeVolumes[primaryKey]
	v.st.windowCenter3D, v.st.windowWidth3D = pc.WindowCenter, pc.WindowWidth
	v.st.windowCenter2D = clampStored(scan.WindowLevel)
	v.st.windowWidth2D = math.Max(1, clampStored(scan.WindowWidth))
	v.st.orientation = Axial
	v.st.composite = false
	v.st.zoom2D = 100
	v.st.cutaway = false

	v.dicox.SetWindowLevel(int(v.st.windowCenter2D))
	v.dicox.SetWindowWidth(int(v.st.windowWidth2D))
	v.dicox.SetCompositeMode(false)
	v.dicox.SetAlphaScale(v.st.opacity)
	v.interactiveView.SetZoom(100)
	v.set2DSource(primaryKey)

	v.volumeRenderer.SetScaleZ(scaleZFor(primary))
	v.setup3DView()
	v.volumeRenderer.FitView()

	slog.Info("Scan data loaded",
		slog.String("primary", primaryKey),
		slog.Float64("windowLevel", scan.WindowLevel),
		slog.Float64("windowWidth", scan.WindowWidth),
		slog.Int("projections", len(v.projections)))
	v.rebuild()
	return nil
}

// framesOf splits a flat volume into per-slice frames without copying.
func framesOf(data []uint16, w, h, d int) *PixelData {
	frames := make([]Frame, d)
	size := w * h
	for z := 0; z < d; z++ {
		start, end := min(z*size, len(data)), min((z+1)*size, len(data))
		frames[z] = Frame{Data: data[start:end]}
	}
	return &PixelData{Frames: frames}
}

// scaleZFor is the box's Z extent relative to X, from the voxel spacing.
func scaleZFor(vol *VolumeData) float64 {
	vx, vz := spacingOr1(vol.VoxelSizeX), spacingOr1(vol.VoxelSizeZ)
	s := float64(vol.Depth) * vz / (float64(vol.Width) * vx)
	return math.Max(0.1, math.Min(10, s))
}

// clampStored limits a window value to the stored (uint16) range used by the 2D view.
func clampStored(x float64) float64 {
	return math.Max(0, math.Min(65535, x))
}
