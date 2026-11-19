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
		wl, ww := defaultWindow(vol)
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
			Alpha:            1,
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

	// Display defaults for a new study, as roxel opens one: the coronal plane
	// at its middle slice, the file's own window (or one derived from the
	// data), the slice plane outlined in the 3D view.
	pc := v.compositeVolumes[primaryKey]
	v.st.windowCenter3D, v.st.windowWidth3D = pc.WindowCenter, pc.WindowWidth
	v.st.windowCenter2D, v.st.windowWidth2D = pc.WindowCenter, pc.WindowWidth
	v.st.range3D = windowRangeFor(pc.WindowCenter, pc.WindowWidth)
	v.st.range2D = v.st.range3D
	v.st.orientation = Coronal
	v.st.composite = false
	v.st.zoom2D = 100
	v.st.cutaway = false
	v.st.reverseCutaway = false

	v.dicox.SetRescale(pc.RescaleIntercept)
	v.dicox.SetWindowLevel(int(v.st.windowCenter2D))
	v.dicox.SetWindowWidth(int(v.st.windowWidth2D))
	v.dicox.SetCompositeMode(false)
	v.dicox.SetAlphaScale(v.st.opacity)
	v.interactiveView.SetZoom(100)
	v.set2DSource(primaryKey)

	v.volumeRenderer.SetBoxScale(boxScaleFor(primary))
	v.setup3DView()
	v.volumeRenderer.ResetView()
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

// boxScaleFor is the box's Y and Z extents relative to X, from the voxel
// spacing: the volume is drawn in its true physical proportions.
func boxScaleFor(vol *VolumeData) (y, z float64) {
	vx, vy, vz := spacingOr1(vol.VoxelSizeX), spacingOr1(vol.VoxelSizeY), spacingOr1(vol.VoxelSizeZ)
	x := float64(max(vol.Width, 1)) * vx
	return float64(max(vol.Height, 1)) * vy / x, float64(max(vol.Depth, 1)) * vz / x
}

// defaultWindow is a volume's opening window, in modality units (stored value
// times slope plus intercept). It is the file's own Window Center / Width when
// it has a single usable one; otherwise it spans the 1st to the 99th percentile
// of the data, which ignores outliers.
func defaultWindow(vol *VolumeData) (center, width float64) {
	if vol.WindowFromTags {
		return vol.WindowCenter, vol.WindowWidth
	}
	n := len(vol.Data)
	if n == 0 {
		return 32768, 65536
	}
	slope := vol.RescaleSlope
	if slope == 0 {
		slope = 1
	}
	step := max(n/100_000, 1)
	samples := make([]float64, 0, n/step+1)
	for i := 0; i < n; i += step {
		samples = append(samples, float64(vol.Data[i])*slope+vol.RescaleIntercept)
	}
	sort.Float64s(samples)
	p01, p99 := samples[len(samples)/100], samples[len(samples)*99/100]
	if p99 <= p01 {
		return 32768, 65536
	}
	width = math.Max(p99-p01, 1)
	return p01 + width/2, width
}
