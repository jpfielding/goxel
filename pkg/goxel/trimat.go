package goxel

import (
	"image/color"
	"log/slog"
	"strings"
)

// CreateTrimatVolume creates a TRIMAT rescaled volume by blending HE and LE data
// This implements the MVS formula: rescaled = he * (le / Z_ORGANIC)
// where Z_ORGANIC = 550 (typical Zeff for organic materials)
func CreateTrimatVolume(v *Viewer, scan *ScanCollection) {
	const zOrganic = 550.0

	// Find HE and LE volume pairs by resolution
	var heVol, leVol *VolumeData
	var resolution string

	// Prefer low_res for performance
	for name, vol := range scan.Volumes {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "he") && strings.Contains(lower, "low") {
			heVol = vol
			resolution = "low_res"
		}
		if strings.Contains(lower, "le") && strings.Contains(lower, "low") {
			leVol = vol
		}
	}

	// Fallback to high_res if no low_res found
	if heVol == nil || leVol == nil {
		for name, vol := range scan.Volumes {
			lower := strings.ToLower(name)
			if heVol == nil && strings.Contains(lower, "he") {
				heVol = vol
				resolution = "high_res"
			}
			if leVol == nil && strings.Contains(lower, "le") {
				leVol = vol
			}
		}
	}

	// Need both volumes with matching dimensions
	if heVol == nil || leVol == nil {
		slog.Debug("TRIMAT: HE or LE volume not found, skipping rescaled volume creation",
			"hasHE", heVol != nil, "hasLE", leVol != nil)
		return
	}

	if heVol.Width != leVol.Width || heVol.Height != leVol.Height || heVol.Depth != leVol.Depth {
		slog.Warn("TRIMAT: HE and LE volume dimensions don't match",
			"he", [3]int{heVol.Width, heVol.Height, heVol.Depth},
			"le", [3]int{leVol.Width, leVol.Height, leVol.Depth})
		return
	}

	slog.Info("TRIMAT: Creating rescaled volume from HE+LE",
		"resolution", resolution,
		"dims", [3]int{heVol.Width, heVol.Height, heVol.Depth})

	// Create rescaled volume
	rescaledData := make([]uint16, len(heVol.Data))
	for i := 0; i < len(heVol.Data); i++ {
		heVal := float64(heVol.Data[i])
		leVal := float64(leVol.Data[i])

		// MVS thresholds: only rescale if both values are above minimum
		const ctMin = 100.0
		const zeffMin = 100.0

		if heVal > ctMin && leVal > zeffMin {
			// MVS formula: rescaled = ct * (zeff / Z_ORGANIC)
			rescaled := heVal * (leVal / zOrganic)
			// Clamp to uint16 range
			if rescaled > 65535 {
				rescaled = 65535
			}
			rescaledData[i] = uint16(rescaled)
		} else {
			// Below threshold: use raw HE value
			rescaledData[i] = heVol.Data[i]
		}
	}

	// Create frames from rescaled data
	name := "trimat_" + resolution
	frames := make([]Frame, heVol.Depth)
	sliceSize := heVol.Width * heVol.Height
	for z := 0; z < heVol.Depth; z++ {
		offset := z * sliceSize
		end := offset + sliceSize
		if end > len(rescaledData) {
			end = len(rescaledData)
		}
		frames[z] = Frame{Data: rescaledData[offset:end]}
	}

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         frames,
	}

	wl, ww := CalculateWindowFromData(rescaledData)

	// TRIMAT volume gets white tint (let transfer function handle colors)
	v.compositeVolumes[name] = &CompositeVolume{
		Name:             name,
		PixelData:        pd,
		Rows:             heVol.Height,
		Cols:             heVol.Width,
		PixelRep:         0,
		Enabled:          true, // Enable TRIMAT by default
		RescaleIntercept: 0,
		WindowCenter:     wl,
		WindowWidth:      ww,
		Color:            color.RGBA{255, 255, 255, 255}, // White tint
		Alpha:            1.0,
	}

	// Disable individual HE/LE volumes when TRIMAT is enabled
	for volName := range v.compositeVolumes {
		lower := strings.ToLower(volName)
		if strings.Contains(lower, "he_") || strings.Contains(lower, "le_") {
			if cv, ok := v.compositeVolumes[volName]; ok {
				cv.Enabled = false
			}
		}
	}

	slog.Info("TRIMAT: Created rescaled volume", "name", name, "wl", wl, "ww", ww)
}
