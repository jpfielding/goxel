package goxel

import (
	"image/color"
	"log/slog"
	"sort"
)

// SetScanData configures the viewer with data from a ScanCollection.
// This is the unified entry point for all file/directory loading.
func (v *Viewer) SetScanData(scan *ScanCollection) error {
	slog.Info("SetScanData", slog.String("path", scan.SourcePath),
		slog.Int("volumes", len(scan.Volumes)),
		slog.Int("findings", len(scan.Findings)))

	// Get primary volume for 2D viewer setup
	primary := scan.PrimaryVolume()
	if primary == nil {
		slog.Warn("No volumes found in scan")
		return nil
	}
	slog.Info("Primary volume selected",
		slog.String("name", primary.Name),
		slog.Int("width", primary.Width),
		slog.Int("height", primary.Height),
		slog.Int("depth", primary.Depth),
		slog.Float64("voxelX", primary.VoxelSizeX),
		slog.Float64("voxelY", primary.VoxelSizeY),
		slog.Float64("voxelZ", primary.VoxelSizeZ))

	// Set viewer metadata
	v.imageRows = primary.Height
	v.imageCols = primary.Width
	v.bitsAllocated = 16
	v.pixelRepresentation = primary.PixelRep

	// Convert primary volume to 3D int slice for DICOXImage
	volumeData := make([][][]int, primary.Depth)
	for z := 0; z < primary.Depth; z++ {
		volumeData[z] = make([][]int, primary.Height)
		sliceOffset := z * primary.Width * primary.Height
		for y := 0; y < primary.Height; y++ {
			volumeData[z][y] = make([]int, primary.Width)
			rowOffset := y * primary.Width
			for x := 0; x < primary.Width; x++ {
				idx := sliceOffset + rowOffset + x
				if idx < len(primary.Data) {
					volumeData[z][y][x] = int(primary.Data[idx])
				}
			}
		}
	}

	// Apply window/level - clamp to int16 range before casting
	wl := scan.WindowLevel
	ww := scan.WindowWidth
	if wl < 0 {
		wl = 0
	} else if wl > 32767 {
		wl = 32767
	}
	if ww < 0 {
		ww = 0
	} else if ww > 32767 {
		ww = 32767
	}

	v.dicox.SetWindowLevel(int(wl))
	v.dicox.SetWindowWidth(int(ww))
	v.dicox.SetVolume(volumeData, primary.Width, primary.Height, primary.Depth)

	// Find first non-empty slice
	firstNonEmptySlice := 0
	for z := 0; z < primary.Depth; z++ {
		sliceOffset := z * primary.Width * primary.Height
		for i := sliceOffset; i < sliceOffset+primary.Width*primary.Height && i < len(primary.Data); i++ {
			if primary.Data[i] != 0 {
				firstNonEmptySlice = z
				break
			}
		}
		if firstNonEmptySlice != 0 {
			break
		}
	}

	// Default to Coronal view with slice 50 (or max if out of range)
	v.dicox.SetOrientation(Coronal)
	initialSlice := 50
	maxSlice := primary.Height - 1 // Coronal scrolls Y dimension
	if initialSlice > maxSlice {
		initialSlice = maxSlice
	}
	if initialSlice < 0 {
		initialSlice = 0
	}
	v.currentFrame = initialSlice
	v.setFrame(v.currentFrame)

	// Sort keys for deterministic color assignment
	keys := make([]string, 0, len(scan.Volumes))
	for k := range scan.Volumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Convert all volumes to CompositeVolume format
	v.compositeVolumes = make(map[string]*CompositeVolume)
	for _, name := range keys {
		vol := scan.Volumes[name]
		frames := make([]Frame, vol.Depth)
		sliceSize := vol.Width * vol.Height
		for z := 0; z < vol.Depth; z++ {
			offset := z * sliceSize
			end := offset + sliceSize
			if end > len(vol.Data) {
				end = len(vol.Data)
			}
			frames[z] = Frame{Data: vol.Data[offset:end]}
		}

		pd := &PixelData{
			IsEncapsulated: false,
			Frames:         frames,
		}

		wl, ww := CalculateWindowFromData(vol.Data)

		// Generate color based on index
		// Use White (255, 255, 255) to allow the Transfer Function (ColorMap)
		// to define the material coloring (Orange/Green/Blue based on density).
		// Tinting overrides the transfer function, which we want to avoid for the main view.
		var c color.RGBA = color.RGBA{255, 255, 255, 255}

		// Defaults
		alpha := 0.5
		enabled := false

		// Enable only the primary volume by default to avoid clutter/overlapping
		if vol == primary {
			enabled = true
		}

		v.compositeVolumes[name] = &CompositeVolume{
			Name:             name,
			SourcePath:       vol.SourcePath,
			PixelData:        pd,
			Rows:             vol.Height,
			Cols:             vol.Width,
			PixelRep:         vol.PixelRep,
			RescaleIntercept: vol.RescaleIntercept,
			WindowCenter:     wl,
			WindowWidth:      ww,
			Enabled:          enabled,
			Color:            c,
			Alpha:            alpha,
			VoxelSizeX:       vol.VoxelSizeX,
			VoxelSizeY:       vol.VoxelSizeY,
			VoxelSizeZ:       vol.VoxelSizeZ,
		}
	}

	// Add finding overlays as composite volumes (disabled by default)
	for _, finding := range scan.Findings {
		if finding.Overlay == nil {
			continue
		}
		ov := finding.Overlay
		findingName := "finding_" + finding.Name

		frames := make([]Frame, ov.Depth)
		sliceSize := ov.Width * ov.Height
		for z := 0; z < ov.Depth; z++ {
			offset := z * sliceSize
			end := offset + sliceSize
			if end > len(ov.Data) {
				end = len(ov.Data)
			}
			frames[z] = Frame{Data: ov.Data[offset:end]}
		}

		pd := &PixelData{
			IsEncapsulated: false,
			Frames:         frames,
		}

		// Map BoundingBox - coordinates are already ROI-adjusted by the loader
		bbox := &BoundingBox3D{
			X:      finding.BBox.MinX,
			Y:      finding.BBox.MinY,
			Z:      finding.BBox.MinZ,
			Width:  finding.BBox.MaxX - finding.BBox.MinX,
			Height: finding.BBox.MaxY - finding.BBox.MinY,
			Depth:  finding.BBox.MaxZ - finding.BBox.MinZ,
		}

		v.compositeVolumes[findingName] = &CompositeVolume{
			Name:      findingName,
			PixelData: pd,
			Rows:      ov.Height,
			Cols:      ov.Width,
			PixelRep:  0,
			Enabled:   false, // Disabled by default
			Alpha:     0.1,   // Half opacity for findings
			BBox:      bbox,
		}
	}

	// Map Projections for 2D viewing (exclude from main composite list to avoid clutter)
	v.projections = make(map[string]*CompositeVolume)
	for name, proj := range scan.Projections {
		frames := make([]Frame, 1)
		frames[0] = Frame{Data: proj.Data}

		pd := &PixelData{
			IsEncapsulated: false,
			Frames:         frames,
		}

		wl, ww := CalculateWindowFromData(proj.Data)

		v.projections[name] = &CompositeVolume{
			Name:         name,
			PixelData:    pd,
			Rows:         proj.Height,
			Cols:         proj.Width,
			PixelRep:     0,
			WindowCenter: wl,
			WindowWidth:  ww,
			Enabled:      true,
			Color:        color.RGBA{255, 255, 255, 255}, // White for 2D
		}
	}

	// Update metadata display with DICOM fields
	if scan.PatientID != "" {
		v.patientID.SetText(scan.PatientID)
	}
	if scan.PatientName != "" {
		v.patientName.SetText(scan.PatientName)
	}
	if scan.StudyDate != "" {
		v.studyDate.SetText(scan.StudyDate)
	}
	if scan.StudyTime != "" {
		v.studyTime.SetText(scan.StudyTime)
	}
	if scan.StudyDescription != "" {
		v.studyDescription.SetText(scan.StudyDescription)
	}
	if scan.SeriesDescription != "" {
		v.seriesDescription.SetText(scan.SeriesDescription)
	}
	if scan.Modality != "" {
		v.modality.SetText(scan.Modality)
	}
	if scan.InstitutionName != "" {
		v.institutionName.SetText(scan.InstitutionName)
	}
	if scan.Manufacturer != "" {
		v.manufacturer.SetText(scan.Manufacturer)
	}
	if scan.OperatorName != "" {
		v.operatorName.SetText(scan.OperatorName)
	}

	slog.Info("Scan data loaded via SetScanData",
		slog.Float64("windowLevel", scan.WindowLevel),
		slog.Float64("windowWidth", scan.WindowWidth),
		slog.Int("projections", len(v.projections)))

	// Setup 3D view
	v.setup3DView()

	// Compute and set correct Z scale based on voxel spacing
	// This ensures the volume isn't stretched in coronal/sagittal views
	if v.volumeRenderer != nil && primary != nil {
		voxelX := primary.VoxelSizeX
		voxelY := primary.VoxelSizeY
		voxelZ := primary.VoxelSizeZ

		// Default to 1.0 if not specified
		if voxelX <= 0 {
			voxelX = 1.0
		}
		if voxelY <= 0 {
			voxelY = 1.0
		}
		if voxelZ <= 0 {
			voxelZ = 1.0
		}

		// Physical dimensions in mm
		physicalX := float64(primary.Width) * voxelX
		physicalY := float64(primary.Height) * voxelY
		physicalZ := float64(primary.Depth) * voxelZ

		// Normalize Z relative to X (shader has X=Y=1.0)
		// scaleZ = physical_Z / physical_X
		scaleZ := physicalZ / physicalX
		if scaleZ < 0.1 {
			scaleZ = 0.1 // Minimum to avoid degenerate volumes
		}
		if scaleZ > 10.0 {
			scaleZ = 10.0 // Maximum to avoid extreme stretching
		}

		slog.Info("Volume aspect ratio computed",
			slog.String("primaryVolume", primary.Name),
			slog.Int("width", primary.Width),
			slog.Int("height", primary.Height),
			slog.Int("depth", primary.Depth),
			slog.Float64("rawVoxelX", primary.VoxelSizeX),
			slog.Float64("rawVoxelY", primary.VoxelSizeY),
			slog.Float64("rawVoxelZ", primary.VoxelSizeZ),
			slog.Float64("usedVoxelX", voxelX),
			slog.Float64("usedVoxelY", voxelY),
			slog.Float64("usedVoxelZ", voxelZ),
			slog.Float64("physicalX_mm", physicalX),
			slog.Float64("physicalY_mm", physicalY),
			slog.Float64("physicalZ_mm", physicalZ),
			slog.Float64("scaleZ", scaleZ))

		v.volumeRenderer.SetScaleZ(scaleZ)
		v.currentScaleZ = scaleZ
		// Update the Z scale slider to reflect computed value
		if v.zScaleSlider != nil {
			v.zScaleSlider.SetValue(scaleZ)
		}
		v.volumeRenderer.Render()
		v.volumeRenderer.Refresh()
	}

	// Refresh layout to populate 2D/3D views
	v.updateLayout()

	return nil
}
