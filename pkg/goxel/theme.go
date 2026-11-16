package goxel

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/theme"
)

// Palette tokens, matching roxel's dark theme.
var (
	colBG       = color.NRGBA{16, 22, 29, 255}
	colPanel    = color.NRGBA{23, 31, 40, 255}
	colText     = color.NRGBA{222, 231, 239, 255}
	colHeading  = color.NRGBA{244, 248, 251, 255}
	colMuted    = color.NRGBA{184, 196, 208, 255}
	colAccent   = color.NRGBA{105, 211, 191, 255}
	colDanger   = color.NRGBA{246, 151, 141, 255}
	colSelected = color.NRGBA{36, 81, 80, 255}
	colOutline  = color.NRGBA{124, 148, 168, 255}
	colRail     = color.NRGBA{104, 126, 146, 255}
	colThumb    = color.NRGBA{236, 242, 247, 255}
	colHairline = color.NRGBA{48, 62, 76, 255}
	colWell     = color.NRGBA{0, 0, 0, 255}
	colControl  = color.NRGBA{38, 52, 66, 255}
	colHover    = color.NRGBA{52, 70, 86, 255}
	colOpenCard = color.NRGBA{27, 47, 52, 255}
)

// goxelTheme is the one Fyne theme: dark only, with larger type and touch
// targets when the layout is compact.
type goxelTheme struct {
	compact bool
}

var _ fyne.Theme = (*goxelTheme)(nil)

// NewTheme returns goxel's theme in its desktop sizes.
func NewTheme() fyne.Theme { return &goxelTheme{} }

func (t *goxelTheme) Color(name fyne.ThemeColorName, _ fyne.ThemeVariant) color.Color {
	switch name {
	case theme.ColorNameBackground:
		return colBG
	case theme.ColorNameOverlayBackground, theme.ColorNameMenuBackground, theme.ColorNameHeaderBackground:
		return colPanel
	case theme.ColorNameForeground:
		return colText
	case theme.ColorNamePlaceHolder, theme.ColorNameDisabled:
		return colMuted
	case theme.ColorNamePrimary, theme.ColorNameFocus, theme.ColorNameHyperlink:
		return colAccent
	case theme.ColorNameForegroundOnPrimary:
		return colBG
	case theme.ColorNameSelection, theme.ColorNamePressed:
		return colSelected
	// Segmented controls mark their selected segment with SuccessImportance.
	case theme.ColorNameSuccess:
		return colSelected
	case theme.ColorNameForegroundOnSuccess:
		return colHeading
	case theme.ColorNameButton, theme.ColorNameInputBackground:
		return colControl
	case theme.ColorNameDisabledButton:
		return colPanel
	case theme.ColorNameHover:
		return colHover
	case theme.ColorNameInputBorder:
		return colOutline
	case theme.ColorNameSeparator:
		return colHairline
	case theme.ColorNameScrollBar:
		return colRail
	case theme.ColorNameError, theme.ColorNameForegroundOnError:
		return colDanger
	case theme.ColorNameShadow:
		return color.NRGBA{0, 0, 0, 90}
	}
	return theme.DefaultTheme().Color(name, theme.VariantDark)
}

func (t *goxelTheme) Font(style fyne.TextStyle) fyne.Resource {
	return theme.DefaultTheme().Font(style)
}

func (t *goxelTheme) Icon(name fyne.ThemeIconName) fyne.Resource {
	return theme.DefaultTheme().Icon(name)
}

func (t *goxelTheme) Size(name fyne.ThemeSizeName) float32 {
	switch name {
	case theme.SizeNameText:
		if t.compact {
			return 16
		}
		return 13.5
	case theme.SizeNameCaptionText:
		if t.compact {
			return 13
		}
		return 12
	case theme.SizeNameSubHeadingText:
		if t.compact {
			return 18
		}
		return 17
	case theme.SizeNameHeadingText:
		return 21
	case theme.SizeNamePadding:
		return 4
	case theme.SizeNameInnerPadding:
		if t.compact {
			return 12
		}
		return 8
	case theme.SizeNameInputRadius, theme.SizeNameSelectionRadius:
		return 5
	case theme.SizeNameSeparatorThickness:
		return 1
	}
	return theme.DefaultTheme().Size(name)
}

// markSVG is goxel's hexagon mark (the same shape roxel uses), in ACCENT.
const markSVG = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 64 64">
<polygon points="32,4 56,18 56,46 32,60 8,46 8,18" fill="none" stroke="#69d3bf" stroke-width="5" stroke-linejoin="round"/>
<polygon points="32,20 42,26 42,38 32,44 22,38 22,26" fill="#69d3bf"/>
</svg>`

var markResource = fyne.NewStaticResource("goxel-mark.svg", []byte(markSVG))
