// Package volume provides 3D volume rendering utilities.
package volume

import (
	"encoding/json"
	"image/color"
	"os"
	"sort"
)

// RenderConfig holds volume rendering configuration loaded from JSON.
// Based on Mercury Viewing Station ctre XML format.
type RenderConfig struct {
	Name             string                        `json:"name"`
	BackgroundColor  [3]int                        `json:"backgroundColor"` // RGB
	RaySamplingStep  float64                       `json:"raySamplingStep"`
	GlobalBrightness float64                       `json:"globalBrightness"`
	GlobalGamma      float64                       `json:"globalGamma"`
	AmbientIntensity float64                       `json:"ambientIntensity"`
	DiffuseIntensity float64                       `json:"diffuseIntensity"`
	ColorMaps        map[string][]ColorEntry       `json:"colorMaps"`
	OpacityMaps      map[string][]OpacityEntry     `json:"opacityMaps"`
	NormalizedMaps   map[string]NormalizedColorMap `json:"normalizedMaps"`
}

// NormalizedColorEntry maps a 0.0-1.0 progress value to an RGB color.
type NormalizedColorEntry struct {
	Progress float64
	R, G, B  int
}

type NormalizedColorMap struct {
	Orange []NormalizedColorEntry
	Green  []NormalizedColorEntry
	Blue   []NormalizedColorEntry
}

// ColorEntry maps a density value to an RGB color.
type ColorEntry struct {
	Density int `json:"density"`
	R       int `json:"r"`
	G       int `json:"g"`
	B       int `json:"b"`
}

// OpacityEntry maps a density value to an alpha (opacity) value.
type OpacityEntry struct {
	Density int     `json:"density"`
	Alpha   float64 `json:"alpha"`
}

// ColorBand represents a single material band with a name, color, and density threshold.
// Bands are ordered by threshold (ascending). The first band starts at 0.
type ColorBand struct {
	Name          string     // Display name (e.g., "Air", "Organic", "Mixed", "Metal")
	Color         color.RGBA // Band color
	Threshold     int        // Upper density threshold for this band (next band starts here)
	IsTransparent bool       // If true, this band represents clipped/transparent voxels
	Alpha         float64    // Per-band opacity multiplier (0.0-2.0, default 1.0 means use MVS baseline)
}

// DefaultColorBands returns the default 5-band configuration for UI material threshold controls.
// These thresholds are tuned for raw CT density data distribution (0-30000 range).
// Note: ColorMaps use MVS trimat_tc_trans.xml values for smooth color gradients,
// but UI band thresholds need higher values to match actual data distribution.
// Air (transparent) + 4 material colors with Alpha=1.0 (use MVS baseline opacity).
func DefaultColorBands() []ColorBand {
	return []ColorBand{
		{Name: "Air", Color: color.RGBA{250, 200, 110, 128}, Threshold: 8000, IsTransparent: true, Alpha: 1.0},
		{Name: "Organic", Color: color.RGBA{230, 150, 50, 255}, Threshold: 15000, Alpha: 1.0},
		{Name: "Inorganic", Color: color.RGBA{80, 200, 40, 255}, Threshold: 20000, Alpha: 1.0},
		{Name: "Metal", Color: color.RGBA{15, 165, 200, 255}, Threshold: 25000, Alpha: 1.0},
		{Name: "Dense", Color: color.RGBA{40, 48, 180, 255}, Threshold: 30000, Alpha: 1.0},
	}
}

// GetColorBandsFromConfig returns color bands for the slider UI.
// Uses DefaultColorBands() which provides explicit control over band colors and thresholds.
func GetColorBandsFromConfig(cfg *RenderConfig, mapName string) []ColorBand {
	return DefaultColorBands()
}

// LoadConfig loads a RenderConfig from a JSON file.
func LoadConfig(path string) (*RenderConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg RenderConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// DefaultConfig returns the embedded MVS default configuration.
// Based on trimat_tc_trans.xml from Mercury Viewing Station.
func DefaultConfig() *RenderConfig {
	return &RenderConfig{
		Name:             "MVS TRIMAT Default",
		BackgroundColor:  [3]int{237, 237, 237},
		RaySamplingStep:  0.5,
		GlobalBrightness: 0.0,
		GlobalGamma:      0.95,
		AmbientIntensity: 1.0,
		DiffuseIntensity: 0.1,
		ColorMaps: map[string][]ColorEntry{
			"DEFAULT": {
				// Orange organics (low density)
				{Density: 0, R: 250, G: 200, B: 110},
				{Density: 200, R: 235, G: 175, B: 85},
				{Density: 500, R: 230, G: 150, B: 50},
				{Density: 1000, R: 220, G: 140, B: 25},
				{Density: 1500, R: 205, G: 120, B: 5},
				// Green inorganics (medium density)
				{Density: 2200, R: 80, G: 200, B: 40},
				{Density: 4500, R: 5, G: 140, B: 60},
				{Density: 5000, R: 5, G: 155, B: 70},
				{Density: 5500, R: 5, G: 125, B: 100},
				{Density: 7000, R: 5, G: 125, B: 125},
				// Blue metals (high density)
				{Density: 7500, R: 15, G: 165, B: 200},
				{Density: 8000, R: 90, G: 170, B: 225},
				{Density: 8500, R: 140, G: 150, B: 235},
				{Density: 9500, R: 140, G: 145, B: 204},
				{Density: 10100, R: 135, G: 145, B: 245},
				{Density: 15000, R: 95, G: 110, B: 225},
				{Density: 17000, R: 50, G: 60, B: 170},
				{Density: 30000, R: 40, G: 48, B: 180},
			},
			// FINDING - Red throughout
			"FINDING": {
				{Density: 0, R: 0, G: 0, B: 0},
				{Density: 700, R: 188, G: 25, B: 30},
				{Density: 1200, R: 188, G: 25, B: 30},
				{Density: 1800, R: 188, G: 25, B: 30},
				{Density: 30000, R: 188, G: 25, B: 30},
			},
			// MONOCHROME (from tc_opac.ts) - Grayscale
			"MONOCHROME": {
				{Density: 0, R: 0, G: 0, B: 0},
				{Density: 600, R: 170, G: 170, B: 170},
				{Density: 900, R: 150, G: 150, B: 150},
				{Density: 1700, R: 125, G: 125, B: 125},
				{Density: 2500, R: 100, G: 100, B: 100},
				{Density: 4200, R: 64, G: 64, B: 64},
				{Density: 30000, R: 40, G: 40, B: 40},
			},
			// LAPTOP_REMOVAL (from tc_opac.ts) - Gray/Blue for laptop subtraction view
			"LAPTOP_REMOVAL": {
				{Density: 0, R: 0, G: 0, B: 0},
				{Density: 700, R: 64, G: 64, B: 64},
				{Density: 1200, R: 128, G: 128, B: 128},
				{Density: 1700, R: 192, G: 192, B: 192},
				{Density: 2701, R: 2, G: 40, B: 225},
				{Density: 30000, R: 20, G: 20, B: 40},
			},
		},
		OpacityMaps: map[string][]OpacityEntry{
			// DEFAULT opacity map from trimat_tc_trans.xml (lines 202-221)
			"DEFAULT": {
				{Density: 0, Alpha: 0.0},
				{Density: 499, Alpha: 0.0},
				{Density: 500, Alpha: 0.005},
				{Density: 800, Alpha: 0.008},
				{Density: 1449, Alpha: 0.008},
				{Density: 1450, Alpha: 0.01},
				{Density: 1451, Alpha: 0.01},
				{Density: 3000, Alpha: 0.01},
				{Density: 4500, Alpha: 0.01},
				{Density: 6500, Alpha: 0.02},
				{Density: 6600, Alpha: 0.03},
				{Density: 7000, Alpha: 0.05},
				{Density: 9000, Alpha: 0.05},
				{Density: 9500, Alpha: 0.05},
				{Density: 10100, Alpha: 0.08},
				{Density: 15000, Alpha: 0.08},
				{Density: 30000, Alpha: 0.3},
				{Density: 35000, Alpha: 0.4},
			},
			// FINDING - Higher base opacity
			"FINDING": {
				{Density: 0, Alpha: 0.0},
				{Density: 700, Alpha: 0.03},
				{Density: 1000, Alpha: 0.1},
				{Density: 1800, Alpha: 0.12},
				{Density: 2000, Alpha: 0.15},
				{Density: 30000, Alpha: 0.2},
			},
			// MONOCHROME (from tc_opac.ts)
			"MONOCHROME": {
				{Density: 0, Alpha: 0.0},
				{Density: 599, Alpha: 0.0},
				{Density: 600, Alpha: 0.03},
				{Density: 899, Alpha: 0.03},
				{Density: 900, Alpha: 0.1},
				{Density: 1699, Alpha: 0.1},
				{Density: 1700, Alpha: 0.15},
				{Density: 2499, Alpha: 0.15},
				{Density: 2500, Alpha: 0.4},
				{Density: 4200, Alpha: 0.4},
				{Density: 30000, Alpha: 0.5},
			},
			// LAPTOP_REMOVAL (from tc_opac.ts) - Zero opacity for subtraction
			"LAPTOP_REMOVAL": {
				{Density: 0, Alpha: 0.0},
				{Density: 700, Alpha: 0.0},
				{Density: 1200, Alpha: 0.0},
				{Density: 1700, Alpha: 0.0},
				{Density: 2000, Alpha: 0.0},
				{Density: 3000, Alpha: 0.0},
				{Density: 30000, Alpha: 0.0},
			},
		},
		NormalizedMaps: map[string]NormalizedColorMap{
			"DEFAULT": {
				Orange: []NormalizedColorEntry{
					{0.0, 250, 200, 110},
					{0.1, 235, 175, 85},
					{0.25, 230, 150, 50},
					{0.5, 220, 140, 25},
					{0.75, 205, 120, 5},
					{1.0, 205, 120, 5},
				},
				Green: []NormalizedColorEntry{
					{0.0, 80, 200, 40},
					{0.4, 5, 140, 60},
					{0.5, 5, 155, 70},
					{0.6, 5, 125, 100},
					{1.0, 5, 125, 125},
				},
				Blue: []NormalizedColorEntry{
					{0.0, 15, 165, 200}, // Light blue start
					{0.1, 90, 170, 225},
					{0.2, 140, 150, 235},
					{0.4, 140, 145, 204},
					{0.5, 135, 145, 245},
					{0.7, 95, 110, 225},
					{0.8, 50, 60, 170},
					{1.0, 40, 48, 180},
				},
			},
		},
	}
}

// InterpolateColor returns the interpolated RGB color for a given density.
func (cfg *RenderConfig) InterpolateColor(mapName string, density int) (r, g, b int) {
	entries, ok := cfg.ColorMaps[mapName]
	if !ok || len(entries) == 0 {
		return 128, 128, 128 // Default gray
	}

	// Sort by density (should already be sorted)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Density < entries[j].Density
	})

	// Find the two entries to interpolate between
	if density <= entries[0].Density {
		return entries[0].R, entries[0].G, entries[0].B
	}
	if density >= entries[len(entries)-1].Density {
		e := entries[len(entries)-1]
		return e.R, e.G, e.B
	}

	// Linear interpolation
	for i := 0; i < len(entries)-1; i++ {
		if density >= entries[i].Density && density < entries[i+1].Density {
			e1, e2 := entries[i], entries[i+1]
			t := float64(density-e1.Density) / float64(e2.Density-e1.Density)
			r = int(float64(e1.R) + t*float64(e2.R-e1.R))
			g = int(float64(e1.G) + t*float64(e2.G-e1.G))
			b = int(float64(e1.B) + t*float64(e2.B-e1.B))
			return r, g, b
		}
	}

	return 128, 128, 128
}

// InterpolateOpacity returns the interpolated alpha for a given density.
func (cfg *RenderConfig) InterpolateOpacity(mapName string, density int) float64 {
	entries, ok := cfg.OpacityMaps[mapName]
	if !ok || len(entries) == 0 {
		return 0.0
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Density < entries[j].Density
	})

	if density <= entries[0].Density {
		return entries[0].Alpha
	}
	if density >= entries[len(entries)-1].Density {
		return entries[len(entries)-1].Alpha
	}

	for i := 0; i < len(entries)-1; i++ {
		if density >= entries[i].Density && density < entries[i+1].Density {
			e1, e2 := entries[i], entries[i+1]
			t := float64(density-e1.Density) / float64(e2.Density-e1.Density)
			return e1.Alpha + t*(e2.Alpha-e1.Alpha)
		}
	}

	return 0.0
}

// CreateTransferFunctionFromConfig generates a high-resolution RGBA transfer function
// by sampling the color and opacity maps at normalized density values.
// maxDensity specifies the density value that maps to normalized 1.0.
func CreateTransferFunctionFromConfig(cfg *RenderConfig, mapName string, maxDensity int) []color.RGBA {
	tf := make([]color.RGBA, TransferFunctionSize)

	for i := 0; i < TransferFunctionSize; i++ {
		// Map 0-1023 to 0-maxDensity
		density := int(float64(i) / float64(TransferFunctionSize-1) * float64(maxDensity))

		r, g, b := cfg.InterpolateColor(mapName, density)
		alpha := cfg.InterpolateOpacity(mapName, density)

		// Clamp values
		if r < 0 {
			r = 0
		} else if r > 255 {
			r = 255
		}
		if g < 0 {
			g = 0
		} else if g > 255 {
			g = 255
		}
		if b < 0 {
			b = 0
		} else if b > 255 {
			b = 255
		}
		if alpha < 0 {
			alpha = 0
		} else if alpha > 1 {
			alpha = 1
		}

		tf[i] = color.RGBA{
			R: uint8(r),
			G: uint8(g),
			B: uint8(b),
			A: uint8(alpha * 255),
		}
	}

	return tf
}

// CreateTransferFunctionFromConfigWithRanges generates a transfer function with user-defined
// clipping ranges for Low, Mid, and High density (material) bands.
// Ranges are closed intervals [min, max]. Density outside these ranges (but within the material band) is set to 0 opacity.
func CreateTransferFunctionFromConfigWithRanges(cfg *RenderConfig, mapName string, maxDensity int,
	lowMin, lowMax, midMin, midMax, highMin, highMax int) []color.RGBA {

	tf := make([]color.RGBA, TransferFunctionSize)

	// Define material band boundaries (nominal)
	// These define "which slider controls this pixel"
	// Low: 0 - 15000 (Orange/Organic)
	// Mid: 15000 - 20000 (Green/Inorganic)
	// High: 20000+ (Blue/Metal)
	const (
		BandLowEnd = 15000
		BandMidEnd = 20000
	)

	for i := 0; i < TransferFunctionSize; i++ {
		// Map to 0-maxDensity
		density := int(float64(i) / float64(TransferFunctionSize-1) * float64(maxDensity))

		r, g, b := cfg.InterpolateColor(mapName, density)
		alpha := cfg.InterpolateOpacity(mapName, density)

		// Apply Range Clipping
		visible := true

		if density < BandLowEnd {
			// Low Band (Orange)
			if density < lowMin || density > lowMax {
				visible = false
			}
		} else if density < BandMidEnd {
			// Mid Band (Green)
			if density < midMin || density > midMax {
				visible = false
			}
		} else {
			// High Band (Blue)
			if density < highMin || density > highMax {
				visible = false
			}
		}

		if !visible {
			alpha = 0
		}

		// Clamp values
		if r < 0 {
			r = 0
		} else if r > 255 {
			r = 255
		}
		if g < 0 {
			g = 0
		} else if g > 255 {
			g = 255
		}
		if b < 0 {
			b = 0
		} else if b > 255 {
			b = 255
		}
		if alpha < 0 {
			alpha = 0
		} else if alpha > 1 {
			alpha = 1
		}

		tf[i] = color.RGBA{
			R: uint8(r),
			G: uint8(g),
			B: uint8(b),
			A: uint8(alpha * 255),
		}
	}

	return tf
}

// InterpolateNormalizedColor returns RGB for a progress 0.0-1.0 within a specific band entries
func InterpolateNormalizedColor(entries []NormalizedColorEntry, progress float64) (r, g, b int) {
	if len(entries) == 0 {
		return 128, 128, 128
	}
	if progress <= entries[0].Progress {
		return entries[0].R, entries[0].G, entries[0].B
	}
	if progress >= entries[len(entries)-1].Progress {
		e := entries[len(entries)-1]
		return e.R, e.G, e.B
	}

	for i := 0; i < len(entries)-1; i++ {
		if progress >= entries[i].Progress && progress < entries[i+1].Progress {
			e1, e2 := entries[i], entries[i+1]
			t := (progress - e1.Progress) / (e2.Progress - e1.Progress)
			r = int(float64(e1.R) + t*float64(e2.R-e1.R))
			g = int(float64(e1.G) + t*float64(e2.G-e1.G))
			b = int(float64(e1.B) + t*float64(e2.B-e1.B))
			return r, g, b
		}
	}
	return 128, 128, 128
}

// CreateTransferFunctionDynamic generates a TF where Orange/Green/Blue bands are
// mapped to dynamic density ranges [0, t1], [t1, t2], [t2, maxDensity].
func CreateTransferFunctionDynamic(cfg *RenderConfig, mapName string, maxDensity int, t1, t2 int) []color.RGBA {
	tf := make([]color.RGBA, TransferFunctionSize)
	normMap, ok := cfg.NormalizedMaps[mapName]
	if !ok {
		// Fallback to static if normalized map missing
		return CreateTransferFunctionFromConfig(cfg, mapName, maxDensity)
	}

	// Safety clamping
	if t1 < 0 {
		t1 = 0
	}
	if t2 < t1 {
		t2 = t1
	}
	if t2 > maxDensity {
		t2 = maxDensity
	}

	for i := 0; i < TransferFunctionSize; i++ {
		density := int(float64(i) / float64(TransferFunctionSize-1) * float64(maxDensity))

		// Determine color based on dynamic bands
		var r, g, b int
		if density < t1 {
			// Orange Band
			prog := 0.0
			if t1 > 0 {
				prog = float64(density) / float64(t1)
			}
			r, g, b = InterpolateNormalizedColor(normMap.Orange, prog)
		} else if density < t2 {
			// Green Band
			prog := 0.0
			width := t2 - t1
			if width > 0 {
				prog = float64(density-t1) / float64(width)
			}
			r, g, b = InterpolateNormalizedColor(normMap.Green, prog)
		} else {
			// Blue Band
			prog := 0.0
			width := maxDensity - t2
			if width > 0 {
				prog = float64(density-t2) / float64(width)
			}
			r, g, b = InterpolateNormalizedColor(normMap.Blue, prog)
		}

		// Opacity still uses the static logic for now, or we can just linearly ramp up?
		// User said "color slider", didn't explicitly ask for opacity changes.
		// However, usually opacity is tied to density.
		// Using the static opacity map ensures we don't accidentally make air visible.
		// BUT: if we shift "Metal" (Blue) to low density, using static opacity (which might be low for that density)
		// could make the metal invisible. This is tricky.
		// Ideally, we should also map opacity "Material Types" to these bands.
		// Let's assume opacity follows the material type too.
		// Orange = Low Opacity, Green = Med, Blue = High.

		// For robustness, let's keep using the static opacity map based on absolute density
		// because "density" implies physical attenuation.
		// Changing color mapping allows visual segmentation, but changing opacity physics might look weird.
		// Wait, if I say "0-1000 is Orange", and "1000-5000 is Green".
		// If I select t1=20000, then "Orange" stretches to 20000.
		// Density 15000 is dense. It should probably be opaque.
		// If I color it Orange, fine.
		// Let's stick to existing opacity logic (`InterpolateOpacity`) which relies on absolute density.
		alpha := cfg.InterpolateOpacity(mapName, density)

		tf[i] = color.RGBA{
			R: uint8(r), G: uint8(g), B: uint8(b), A: uint8(alpha * 255),
		}
	}
	return tf
}

// CreateTransferFunctionFromBands generates a transfer function using variable color bands.
// Each band's threshold defines its upper bound; the color fills from the previous threshold.
// This supports any number of bands (not just 3).
func CreateTransferFunctionFromBands(cfg *RenderConfig, mapName string, bands []ColorBand) []color.RGBA {
	tf := make([]color.RGBA, TransferFunctionSize)

	if len(bands) == 0 {
		bands = DefaultColorBands()
	}

	maxDensity := bands[len(bands)-1].Threshold
	if maxDensity <= 0 {
		maxDensity = 30000
	}

	for i := 0; i < TransferFunctionSize; i++ {
		density := int(float64(i) / float64(TransferFunctionSize-1) * float64(maxDensity))

		// Find which band this density belongs to
		var bandIdx int
		var bandStart int
		for j, band := range bands {
			if density < band.Threshold || j == len(bands)-1 {
				bandIdx = j
				if j > 0 {
					bandStart = bands[j-1].Threshold
				}
				break
			}
		}

		// Use the band's color
		bandColor := bands[bandIdx].Color

		// Calculate progress within the band for gradient effect
		bandEnd := bands[bandIdx].Threshold
		bandWidth := bandEnd - bandStart
		var progress float64
		if bandWidth > 0 {
			progress = float64(density-bandStart) / float64(bandWidth)
		}

		// Slightly darken at start, brighten at end for depth
		brightness := 0.85 + 0.3*progress
		r := uint8(clampInt(int(float64(bandColor.R)*brightness), 0, 255))
		g := uint8(clampInt(int(float64(bandColor.G)*brightness), 0, 255))
		b := uint8(clampInt(int(float64(bandColor.B)*brightness), 0, 255))

		// Get opacity from config (based on absolute density)
		alpha := cfg.InterpolateOpacity(mapName, density)
		// Apply per-band alpha multiplier if set (default 1.0 = no change)
		if bandIdx < len(bands) && bands[bandIdx].Alpha > 0 {
			alpha *= bands[bandIdx].Alpha
		}

		tf[i] = color.RGBA{R: r, G: g, B: b, A: uint8(alpha * 255)}
	}
	return tf
}

func clampInt(v, min, max int) int {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

// CreateTransferFunctionFromBandsWithGradient generates a transfer function where
// color gradients are stretched/compressed to fit the band thresholds.
// This uses NormalizedMaps for smooth color transitions within each material band.
// Band mapping:
//   - Band 0 (Air): transparent
//   - Band 1 (Organic): Orange gradient (full 0.0-1.0)
//   - Band 2 (Inorganic): Green gradient (full 0.0-1.0)
//   - Band 3 (Metal): Blue gradient first half (0.0-0.5)
//   - Band 4 (Dense): Blue gradient second half (0.5-1.0)
func CreateTransferFunctionFromBandsWithGradient(cfg *RenderConfig, mapName string, bands []ColorBand) []color.RGBA {
	tf := make([]color.RGBA, TransferFunctionSize)

	if len(bands) == 0 {
		bands = DefaultColorBands()
	}

	normMap, ok := cfg.NormalizedMaps[mapName]
	if !ok {
		// Fallback to flat colors if no normalized map
		return CreateTransferFunctionFromBands(cfg, mapName, bands)
	}

	maxDensity := bands[len(bands)-1].Threshold
	if maxDensity <= 0 {
		maxDensity = 30000
	}

	for i := 0; i < TransferFunctionSize; i++ {
		density := int(float64(i) / float64(TransferFunctionSize-1) * float64(maxDensity))

		// Find which band this density belongs to
		var bandIdx int
		var bandStart int
		for j, band := range bands {
			if density < band.Threshold || j == len(bands)-1 {
				bandIdx = j
				if j > 0 {
					bandStart = bands[j-1].Threshold
				}
				break
			}
		}

		// Calculate progress within the band (0.0 to 1.0)
		bandEnd := bands[bandIdx].Threshold
		bandWidth := bandEnd - bandStart
		var progress float64
		if bandWidth > 0 {
			progress = float64(density-bandStart) / float64(bandWidth)
		}

		var r, g, b int

		// Map band index to color gradient
		switch bandIdx {
		case 0: // Air - transparent
			r, g, b = 0, 0, 0
		case 1: // Organic - Orange gradient
			r, g, b = InterpolateNormalizedColor(normMap.Orange, progress)
		case 2: // Inorganic - Green gradient
			r, g, b = InterpolateNormalizedColor(normMap.Green, progress)
		case 3: // Metal - Blue gradient first half (0.0-0.5)
			r, g, b = InterpolateNormalizedColor(normMap.Blue, progress*0.5)
		case 4: // Dense - Blue gradient second half (0.5-1.0)
			r, g, b = InterpolateNormalizedColor(normMap.Blue, 0.5+progress*0.5)
		default:
			// Extra bands: use the band's flat color
			if bandIdx < len(bands) {
				c := bands[bandIdx].Color
				r, g, b = int(c.R), int(c.G), int(c.B)
			}
		}

		// Get opacity - use absolute density for physical correctness
		var alpha float64
		if bandIdx == 0 && bands[0].IsTransparent {
			alpha = 0 // Air band is transparent
		} else {
			alpha = cfg.InterpolateOpacity(mapName, density)
			// Apply per-band alpha multiplier if set (default 1.0 = no change)
			if bandIdx < len(bands) && bands[bandIdx].Alpha > 0 {
				alpha *= bands[bandIdx].Alpha
			}
		}

		tf[i] = color.RGBA{
			R: uint8(clampInt(r, 0, 255)),
			G: uint8(clampInt(g, 0, 255)),
			B: uint8(clampInt(b, 0, 255)),
			A: uint8(alpha * 255),
		}
	}
	return tf
}

// CurrentConfig holds the active render configuration.
var CurrentConfig = DefaultConfig()

// SetConfig updates the active render configuration.
func SetConfig(cfg *RenderConfig) {
	CurrentConfig = cfg
}

// GetConfig returns the active render configuration.
func GetConfig() *RenderConfig {
	return CurrentConfig
}
