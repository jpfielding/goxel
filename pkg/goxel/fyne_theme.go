package goxel

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
	"github.com/jpfielding/goxel/pkg/volume"
)

// customTheme implements fyne.Theme using our centralized volume.Theme configuration
type customTheme struct{}

var _ fyne.Theme = (*customTheme)(nil)

func (d customTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	vTheme := volume.GetTheme()

	switch name {
	case theme.ColorNameBackground, "inputBackground", "menuBackground", "overlay":
		return vTheme.Background
	case theme.ColorNameForeground:
		return vTheme.TextPrimary
	case theme.ColorNamePrimary, theme.ColorNameButton:
		return vTheme.AccentPrimary
	case theme.ColorNameFocus:
		return vTheme.AccentPrimary
	case theme.ColorNameSelection:
		// Transparent selection based on accent
		r, g, b, _ := vTheme.AccentPrimary.RGBA()
		return color.RGBA64{R: uint16(r), G: uint16(g), B: uint16(b), A: 0x3333}
	case theme.ColorNameShadow:
		return color.RGBA{0, 0, 0, 100}
	default:
		return theme.DefaultTheme().Color(name, variant)
	}
}

func (d customTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (d customTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (d customTheme) Size(name fyne.ThemeSizeName) float32 {
	return theme.DefaultTheme().Size(name)
}
