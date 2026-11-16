//go:build !js

package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/dialog"
)

// openFile shows the native file picker and loads the chosen DICOM file.
func (v *Viewer) openFile() {
	dialog.ShowFileOpen(func(reader fyne.URIReadCloser, err error) {
		if err != nil {
			dialog.ShowError(err, v.win)
			return
		}
		if reader == nil {
			return
		}
		defer reader.Close()
		v.loadPath(reader.URI().Path())
	}, v.win)
}

// openFolder shows the native folder picker and loads every DICOM file in it.
func (v *Viewer) openFolder() {
	dialog.ShowFolderOpen(func(list fyne.ListableURI, err error) {
		if err != nil {
			dialog.ShowError(err, v.win)
			return
		}
		if list == nil {
			return
		}
		v.loadPath(list.Path())
	}, v.win)
}

// canOpenFolder reports whether this platform can open a folder of slices.
const canOpenFolder = true

// LoadSample is unavailable on desktop; the sample lives in the repository.
func (v *Viewer) LoadSample() {}

// hasTouch reports a touch screen; the desktop build assumes a mouse.
func hasTouch() bool { return false }
