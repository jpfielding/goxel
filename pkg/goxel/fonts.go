package goxel

import (
	_ "embed"

	"fyne.io/fyne/v2"
)

// Ubuntu Light is egui's proportional face, so roxel's type and goxel's set
// at the same sizes read alike. It is distributed under the Ubuntu Font
// Licence 1.0; the licence text is fonts/UFL.txt.
//
//go:embed fonts/Ubuntu-Light.ttf
var ubuntuLightTTF []byte

var ubuntuLight = fyne.NewStaticResource("Ubuntu-Light.ttf", ubuntuLightTTF)
