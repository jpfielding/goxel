// Package layout provides layout managers for the viewer.
package layout

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// LayoutType defines the layout arrangement.
type LayoutType int

const (
	LayoutSingle     LayoutType = iota // Single view (3D or 2D)
	LayoutSideBySide                   // 3D on left, 2D on right
	LayoutQuad                         // 3 orthogonal 2D + 1 3D
)

// Manager coordinates multiple views in a layout.
type Manager struct {
	layoutType LayoutType
	container  *fyne.Container
	views      []fyne.CanvasObject
}

// NewManager creates a new layout manager.
func NewManager(layoutType LayoutType) *Manager {
	return &Manager{
		layoutType: layoutType,
		container:  container.NewMax(),
		views:      make([]fyne.CanvasObject, 0),
	}
}

// SetViews sets the views to arrange.
func (m *Manager) SetViews(views ...fyne.CanvasObject) {
	m.views = views
	m.arrange()
}

// SetLayoutType changes the layout arrangement.
func (m *Manager) SetLayoutType(layoutType LayoutType) {
	m.layoutType = layoutType
	m.arrange()
}

// Container returns the layout container.
func (m *Manager) Container() *fyne.Container {
	return m.container
}

// arrange updates the container based on current layout type.
func (m *Manager) arrange() {
	m.container.Objects = nil

	switch m.layoutType {
	case LayoutSingle:
		if len(m.views) > 0 {
			m.container.Objects = []fyne.CanvasObject{m.views[0]}
		}

	case LayoutSideBySide:
		// Split horizontally: view[0] on left, view[1] on right
		if len(m.views) >= 2 {
			split := container.NewHSplit(m.views[0], m.views[1])
			split.SetOffset(0.6) // 60% for 3D
			m.container.Objects = []fyne.CanvasObject{split}
		} else if len(m.views) == 1 {
			m.container.Objects = []fyne.CanvasObject{m.views[0]}
		}

	case LayoutQuad:
		// 2x2 grid: [top-left, top-right, bottom-left, bottom-right]
		if len(m.views) >= 4 {
			topRow := container.NewHSplit(m.views[0], m.views[1])
			bottomRow := container.NewHSplit(m.views[2], m.views[3])
			grid := container.NewVSplit(topRow, bottomRow)
			m.container.Objects = []fyne.CanvasObject{grid}
		} else if len(m.views) >= 2 {
			split := container.NewHSplit(m.views[0], m.views[1])
			m.container.Objects = []fyne.CanvasObject{split}
		} else if len(m.views) == 1 {
			m.container.Objects = []fyne.CanvasObject{m.views[0]}
		}
	}

	m.container.Refresh()
}
