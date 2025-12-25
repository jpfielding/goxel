package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type labelledAction struct {
	label  string
	icon   fyne.Resource
	tapped func()
}

func (l *labelledAction) ToolbarObject() fyne.CanvasObject {
	b := widget.NewButtonWithIcon(l.label, l.icon, l.tapped)
	b.Importance = widget.LowImportance
	return b
}
