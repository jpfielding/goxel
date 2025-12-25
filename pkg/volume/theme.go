// Package volume provides 3D volume rendering utilities.
package volume

import "image/color"

// Theme defines the UI appearance for the application.
// For 3D rendering colors and parameters, use RenderConfig in config.go.
type Theme struct {
	// UI Colors
	Background      color.RGBA
	AccentPrimary   color.RGBA
	AccentSecondary color.RGBA
	TextPrimary     color.RGBA
	TextSecondary   color.RGBA
	Border          color.RGBA
}

// DefaultTheme provides a professional dark theme.
var DefaultTheme = Theme{
	Background:      color.RGBA{R: 30, G: 30, B: 35, A: 255},
	AccentPrimary:   color.RGBA{R: 51, G: 122, B: 183, A: 255}, // #337ab7
	AccentSecondary: color.RGBA{R: 92, G: 184, B: 92, A: 255},  // Green
	TextPrimary:     color.RGBA{R: 240, G: 240, B: 245, A: 255},
	TextSecondary:   color.RGBA{R: 160, G: 160, B: 170, A: 255},
	Border:          color.RGBA{R: 60, G: 60, B: 70, A: 255},
}

// LightTheme provides a light-colored theme.
var LightTheme = Theme{
	Background:      color.RGBA{R: 255, G: 255, B: 255, A: 255},
	AccentPrimary:   color.RGBA{R: 51, G: 122, B: 183, A: 255}, // #337ab7
	AccentSecondary: color.RGBA{R: 92, G: 184, B: 92, A: 255},
	TextPrimary:     color.RGBA{R: 51, G: 51, B: 51, A: 255},    // #333
	TextSecondary:   color.RGBA{R: 119, G: 119, B: 119, A: 255}, // #777
	Border:          color.RGBA{R: 221, G: 221, B: 221, A: 255}, // #ddd
}

// MVSTheme provides an MVS-style light gray theme.
var MVSTheme = Theme{
	Background:      color.RGBA{R: 237, G: 237, B: 237, A: 255},
	AccentPrimary:   color.RGBA{R: 51, G: 122, B: 183, A: 255},
	AccentSecondary: color.RGBA{R: 92, G: 184, B: 92, A: 255},
	TextPrimary:     color.RGBA{R: 51, G: 51, B: 51, A: 255},
	TextSecondary:   color.RGBA{R: 119, G: 119, B: 119, A: 255},
	Border:          color.RGBA{R: 200, G: 200, B: 200, A: 255},
}

// CurrentTheme is the active UI theme.
var CurrentTheme = MVSTheme

// SetTheme updates the current UI theme.
func SetTheme(t Theme) {
	CurrentTheme = t
}

// GetTheme returns the current UI theme.
func GetTheme() Theme {
	return CurrentTheme
}
