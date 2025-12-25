package goxel

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// ProSightTheme implements a dark professional theme matching the ProSight workstation
type ProSightTheme struct{}

var _ fyne.Theme = (*ProSightTheme)(nil)

// Color definitions
var (
	// Background colors
	colorBackground      = color.NRGBA{R: 26, G: 26, B: 46, A: 255} // #1a1a2e
	colorBackgroundLight = color.NRGBA{R: 38, G: 38, B: 66, A: 255} // #262642

	// Accent colors
	colorPrimary = color.NRGBA{R: 124, G: 58, B: 237, A: 255} // #7c3aed purple

	// Text colors
	colorTextLight = color.NRGBA{R: 224, G: 224, B: 224, A: 255} // #e0e0e0

	// Button colors
	colorDanger  = color.NRGBA{R: 239, G: 68, B: 68, A: 255}  // #ef4444 red
	colorWarning = color.NRGBA{R: 236, G: 72, B: 153, A: 255} // #ec4899 pink
	colorSuccess = color.NRGBA{R: 34, G: 197, B: 94, A: 255}  // #22c55e green

	// Borders
	colorBorder   = color.NRGBA{R: 75, G: 75, B: 100, A: 255}
	colorDisabled = color.NRGBA{R: 100, G: 100, B: 120, A: 255}
)

func (t *ProSightTheme) Color(name fyne.ThemeColorName, variant fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colorBackground
	case theme.ColorNameButton:
		return colorBackgroundLight
	case theme.ColorNameDisabledButton, theme.ColorNameDisabled:
		return colorDisabled
	case theme.ColorNameError:
		return colorDanger
	case theme.ColorNameFocus, theme.ColorNamePrimary, theme.ColorNameSelection, theme.ColorNamePressed:
		return colorPrimary
	case theme.ColorNameForeground:
		return colorTextLight
	case theme.ColorNameHover, theme.ColorNameInputBackground, theme.ColorNameMenuBackground:
		return colorBackgroundLight
	case theme.ColorNameInputBorder, theme.ColorNameScrollBar, theme.ColorNameSeparator:
		return colorBorder
	case theme.ColorNameOverlayBackground:
		return colorBackground
	case theme.ColorNamePlaceHolder:
		return colorDisabled
	case theme.ColorNameSuccess:
		return colorSuccess
	case theme.ColorNameWarning:
		return colorWarning
	case theme.ColorNameShadow:
		return color.NRGBA{A: 100}
	}
	return theme.DefaultTheme().Color(name, variant)
}

func (t *ProSightTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *ProSightTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *ProSightTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNamePadding:
		return 6
	case theme.SizeNameInnerPadding:
		return 4
	case theme.SizeNameText:
		return 13
	case theme.SizeNameHeadingText:
		return 20
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 4
	}
	return theme.DefaultTheme().Size(name)
}
