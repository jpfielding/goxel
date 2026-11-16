package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// responsiveLayout fills the window with the current tree and, when the
// window crosses a layout breakpoint, schedules a rebuild. It never mutates
// the tree inside Layout.
type responsiveLayout struct {
	v *Viewer
}

func (l *responsiveLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
	v := l.v
	class := layoutFromSize(size.Width, size.Height)
	narrow := class == layoutDesktop && shelfIsNarrow(size.Width)
	if (class != v.st.layout || narrow != v.narrowShelf) && !v.rebuildPending {
		v.rebuildPending = true
		go fyne.Do(func() {
			v.rebuildPending = false
			v.st.layout = class
			v.narrowShelf = narrow
			v.rebuild()
			v.volumeRenderer.FitView()
		})
	}
}

// MinSize is small on purpose: every layout must fit a phone.
func (l *responsiveLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(320, 320)
}

// shelfIsNarrow: below 720 px of 3D column the shelf drops to two columns
// and lighting moves into a menu button.
func shelfIsNarrow(window float32) bool {
	return window-sidebarWidth-slicePanelWidth(window) < 720
}

// desktopLayout places [sidebar, slice panel, centre]: the sidebar at its
// fixed width on the left, the slice panel at slicePanelWidth on the right.
type desktopLayout struct{}

func (desktopLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	if len(objs) != 3 {
		return
	}
	right := slicePanelWidth(size.Width)
	objs[0].Move(fyne.NewPos(0, 0))
	objs[0].Resize(fyne.NewSize(sidebarWidth, size.Height))
	objs[1].Move(fyne.NewPos(size.Width-right, 0))
	objs[1].Resize(fyne.NewSize(right, size.Height))
	objs[2].Move(fyne.NewPos(sidebarWidth, 0))
	objs[2].Resize(fyne.NewSize(max(size.Width-sidebarWidth-right, 0), size.Height))
}

func (desktopLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	return fyne.NewSize(sidebarWidth+336+200, h)
}

// desktopRoot builds the desktop layout from state.
func (v *Viewer) desktopRoot() fyne.CanvasObject {
	bar := v.appBar()
	if !v.loaded() {
		return container.NewBorder(bar, nil, nil, nil, v.emptyState())
	}
	status := v.statusBar()
	if v.st.focusSlice {
		return container.NewBorder(bar, status, nil, nil, v.sliceFocusView())
	}
	centre := container.NewBorder(v.volumeHeader(), v.shelf(), nil, nil, v.volumeRenderer)
	if v.st.focusVolume {
		return container.NewBorder(bar, status, nil, nil, centre)
	}
	body := container.New(desktopLayout{}, v.sidebar(), v.slicePanel(), centre)
	return container.NewBorder(bar, status, nil, nil, body)
}
