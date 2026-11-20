package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// menuPanelRadius and menuPanelMargin are the corner radius of a menu panel or
// tooltip and the space from its edge to its content: egui's pop-up frame,
// whose 12 px margin starts inside its 1 px outline.
const (
	menuPanelRadius = 6
	menuPanelMargin = 12 + 1
)

// menuPopup is a menu panel over the window: one rounded, outlined panel on the
// panel colour, casting egui's pop-up shadow (Fyne's own pop-up adds a square
// frame and padding of its own). A tap anywhere else, or Esc, closes it.
type menuPopup struct {
	widget.BaseWidget
	cv      fyne.Canvas
	content fyne.CanvasObject
	pos     fyne.Position // the panel's top left
	size    fyne.Size     // the panel's size, margins included
	onClose func()
	shown   bool
}

func newMenuPopup(cv fyne.Canvas, content fyne.CanvasObject, onClose func()) *menuPopup {
	m := &menuPopup{cv: cv, content: content, onClose: onClose}
	m.ExtendBaseWidget(m)
	return m
}

// show puts the panel at pos, contentWidth wide inside its margins.
func (m *menuPopup) show(pos fyne.Position, contentWidth float32) {
	m.pos = pos
	m.size = fyne.NewSize(contentWidth+2*menuPanelMargin, m.content.MinSize().Height+2*menuPanelMargin)
	m.shown = true
	m.cv.Overlays().Add(m)
	m.Resize(m.cv.Size())
	m.Refresh()
	m.content.Refresh() // controls draw their state (selected, flat) on a refresh
}

// dismiss closes the panel; it is safe to call more than once.
func (m *menuPopup) dismiss() {
	if !m.shown {
		return
	}
	m.shown = false
	m.cv.Overlays().Remove(m)
	if m.onClose != nil {
		m.onClose()
	}
}

func (m *menuPopup) inside(p fyne.Position) bool {
	return p.X >= m.pos.X && p.Y >= m.pos.Y && p.X <= m.pos.X+m.size.Width && p.Y <= m.pos.Y+m.size.Height
}

func (m *menuPopup) Tapped(e *fyne.PointEvent) {
	if !m.inside(e.Position) {
		m.dismiss()
	}
}

func (m *menuPopup) TappedSecondary(e *fyne.PointEvent) {
	if !m.inside(e.Position) {
		m.dismiss()
	}
}

func (m *menuPopup) CreateRenderer() fyne.WidgetRenderer {
	return &menuPopupRenderer{m: m, panel: rrect(colPanel, colOutline, menuPanelRadius, 1),
		shadow: newPopupShadow(menuPanelRadius)}
}

type menuPopupRenderer struct {
	m      *menuPopup
	shadow *canvas.Raster
	panel  *canvas.Rectangle
}

func (r *menuPopupRenderer) Layout(fyne.Size) {
	m := r.m
	placeShadow(r.shadow, m.pos, m.size)
	r.panel.Move(m.pos)
	r.panel.Resize(m.size)
	m.content.Move(m.pos.Add(fyne.NewPos(menuPanelMargin, menuPanelMargin)))
	m.content.Resize(fyne.NewSize(m.size.Width-2*menuPanelMargin, m.size.Height-2*menuPanelMargin))
}

func (r *menuPopupRenderer) MinSize() fyne.Size { return fyne.NewSize(0, 0) }

func (r *menuPopupRenderer) Refresh() {
	r.Layout(r.m.Size())
	canvas.Refresh(r.panel)
}

func (r *menuPopupRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.shadow, r.panel, r.m.content}
}

func (r *menuPopupRenderer) Destroy() {}

// showMenuBelow shows content in a menu panel directly under anchor, as egui
// opens a menu (above it when there is no room), contentWidth wide. An anchor
// with an "open" state (a combo box; egui's menu buttons have none) is kept in
// it while the panel is up.
func (v *Viewer) showMenuBelow(anchor fyne.CanvasObject, content fyne.CanvasObject, contentWidth float32) {
	v.closeMenu()
	opener, _ := anchor.(interface{ SetOpen(bool) })
	if opener != nil {
		opener.SetOpen(true)
	}
	m := newMenuPopup(v.win.Canvas(), content, func() {
		if opener != nil {
			opener.SetOpen(false)
		}
		v.menu = nil
	})
	v.menu = m
	content.Resize(fyne.NewSize(contentWidth, content.MinSize().Height))
	outer := fyne.NewSize(contentWidth+2*menuPanelMargin, content.MinSize().Height+2*menuPanelMargin)
	pos := fyne.CurrentApp().Driver().AbsolutePositionForObject(anchor)
	cs := v.win.Canvas().Size()
	x := min(pos.X, cs.Width-outer.Width-8)
	y := pos.Y + anchor.Size().Height
	if y+outer.Height > cs.Height {
		y = max(pos.Y-outer.Height, 8)
	}
	m.show(fyne.NewPos(max(x, 8), y), contentWidth)
}

// closeMenu closes the open menu panel, if any.
func (v *Viewer) closeMenu() {
	if v.menu != nil {
		v.menu.dismiss()
	}
}
