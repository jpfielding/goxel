package goxel

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Keyboard focus and hover tooltips for the custom controls. Fyne's stock
// widgets have both; ours are drawn by hand, so they implement fyne.Focusable
// (Tab traversal, Space / Enter, arrow keys) and draw roxel's focus ring: 2 px
// in the accent, just outside the control.

// ringWidth is the thickness of the keyboard focus ring.
const ringWidth = 2

// newRing is a hidden rounded outline in the accent; renderers lay it out
// around their control with ringRect and show it while the control has focus.
func newRing(radius float32) *canvas.Rectangle {
	r := rrect(color.Transparent, colAccent, radius+ringWidth, ringWidth)
	r.Hide()
	return r
}

// ringRect places ring around a control of the given size, outside it.
func ringRect(ring *canvas.Rectangle, size fyne.Size) {
	ring.Move(fyne.NewPos(-ringWidth, -ringWidth))
	ring.Resize(fyne.NewSize(size.Width+2*ringWidth, size.Height+2*ringWidth))
}

// showWhen shows o when on, hides it otherwise.
func showWhen(o fyne.CanvasObject, on bool) {
	if on {
		o.Show()
	} else {
		o.Hide()
	}
}

// keyFallback receives keys a focused control does not use (arrow keys step
// slices, as they do with nothing focused). Set by the Viewer.
var keyFallback func(*fyne.KeyEvent)

// isActivate reports whether a key press activates a focused button-like control.
func isActivate(e *fyne.KeyEvent) bool {
	return e.Name == fyne.KeySpace || e.Name == fyne.KeyReturn || e.Name == fyne.KeyEnter
}

// passKey hands an unused key on: Escape drops focus (a second Escape then
// leaves a focus mode, as in roxel); the rest go to the window's handler.
func passKey(self fyne.CanvasObject, e *fyne.KeyEvent) {
	if e.Name == fyne.KeyEscape {
		unfocus(self)
		return
	}
	if keyFallback != nil {
		keyFallback(e)
	}
}

// focusables lists the keyboard-focusable controls under root in tree order.
func focusables(root fyne.CanvasObject) []fyne.Focusable {
	var out []fyne.Focusable
	var walk func(o fyne.CanvasObject)
	walk = func(o fyne.CanvasObject) {
		if o == nil || !o.Visible() {
			return
		}
		if f, ok := o.(fyne.Focusable); ok {
			out = append(out, f)
		}
		switch c := o.(type) {
		case *fyne.Container:
			for _, child := range c.Objects {
				walk(child)
			}
		case *container.Scroll:
			walk(c.Content)
		}
	}
	walk(root)
	return out
}

// --- Tooltips ----------------------------------------------------------------

// Fyne v2.7 has no tooltips (and a pop-up would swallow the next click), so a
// tip is a plain, non-interactive object on a layer above the window's content.
// It looks like egui's, which roxel uses unchanged: body text on the panel
// colour, a 12 px margin inside a 1 px outline with 6 px corners, and the
// pop-up shadow. It opens 4 px under its control, left edges in line.

const (
	// tipDelay is how long the pointer rests on a control before its tip shows.
	tipDelay = 500 * time.Millisecond
	// tipGap is the space between a control and its tip.
	tipGap = 4
)

// tipLayer holds the one tooltip.
type tipLayer struct {
	*fyne.Container
	shadow *canvas.Raster
	bg     *canvas.Rectangle
	label  *canvas.Text
	tip    *fyne.Container
	timer  *time.Timer
	owner  fyne.CanvasObject
}

// tooltips is the window's tooltip layer.
var tooltips *tipLayer

func newTipLayer() *tipLayer {
	t := &tipLayer{
		shadow: newPopupShadow(menuPanelRadius),
		bg:     rrect(colPanel, colOutline, menuPanelRadius, 1),
		label:  text("", 13.5, colText),
	}
	t.tip = container.NewStack(t.bg, container.New(marginLayout{menuPanelMargin, menuPanelMargin}, t.label))
	t.tip.Hide()
	t.shadow.Hide()
	t.Container = container.NewWithoutLayout(t.shadow, t.tip)
	return t
}

// hover schedules the tip for anchor; hide cancels it.
func (t *tipLayer) hover(anchor fyne.CanvasObject, s string) {
	t.hoverRect(anchor, fyne.Position{}, fyne.Size{}, s)
}

// hoverRect schedules the tip for the part of owner at off of the given size
// (the whole of owner when size is zero).
func (t *tipLayer) hoverRect(owner fyne.CanvasObject, off fyne.Position, size fyne.Size, s string) {
	if t == nil || s == "" {
		return
	}
	t.hide(nil)
	t.owner = owner
	t.timer = time.AfterFunc(tipDelay, func() { fyne.Do(func() { t.show(owner, off, size, s) }) })
}

// hide cancels a pending tip and hides a shown one. With an owner given, it
// acts only if the tip belongs to that control.
func (t *tipLayer) hide(owner fyne.CanvasObject) {
	if t == nil || (owner != nil && t.owner != owner) {
		return
	}
	if t.timer != nil {
		t.timer.Stop()
		t.timer = nil
	}
	t.owner = nil
	if !t.tip.Visible() {
		return
	}
	t.tip.Hide()
	t.shadow.Hide()
	t.Container.Refresh()
}

func (t *tipLayer) show(owner fyne.CanvasObject, off fyne.Position, anchor fyne.Size, s string) {
	if t.owner != owner {
		return
	}
	d := fyne.CurrentApp().Driver()
	if d.CanvasForObject(owner) == nil {
		return
	}
	if anchor.IsZero() {
		anchor = owner.Size()
	}
	setFace(t.label, bodySize())
	t.label.Text = s
	t.label.Refresh()
	size := t.tip.MinSize()
	pos := d.AbsolutePositionForObject(owner).Add(off)
	at := tipPosition(pos, anchor, size, t.Container.Size())
	t.tip.Resize(size)
	t.tip.Move(at)
	placeShadow(t.shadow, at, size)
	t.shadow.Show()
	t.tip.Show()
	// The tip's own objects are unknown to the canvas while they are hidden,
	// so it is the layer that asks for the repaint.
	t.Container.Refresh()
}

// tipPosition is where a tip of the given size goes for an anchor at pos in a
// window of size win: under the anchor with left edges in line, as egui places
// it; right edges in line when that would leave the window, and above the
// anchor when there is no room below.
func tipPosition(pos fyne.Position, anchor, size, win fyne.Size) fyne.Position {
	const edge = 4 // the least space kept between a tip and the window's edge
	x := pos.X
	if x+size.Width > win.Width-edge {
		x = pos.X + anchor.Width - size.Width
	}
	x = max(min(x, win.Width-size.Width-edge), edge)
	y := pos.Y + anchor.Height + tipGap
	if y+size.Height > win.Height-edge {
		y = max(pos.Y-size.Height-tipGap, edge)
	}
	return fyne.NewPos(x, y)
}

// tipper is the hover tip of a control.
type tipper struct{ tip string }

func (t *tipper) hoverIn(o fyne.CanvasObject)  { tooltips.hover(o, t.tip) }
func (t *tipper) hoverOut(o fyne.CanvasObject) { tooltips.hide(o) }

// tipArea shows a tooltip for whatever it wraps: content that is not itself a
// control, such as the open study's card.
type tipArea struct {
	widget.BaseWidget
	content fyne.CanvasObject
	tipper
}

func newTipArea(content fyne.CanvasObject, tip string) *tipArea {
	a := &tipArea{content: content, tipper: tipper{tip}}
	a.ExtendBaseWidget(a)
	return a
}

func (a *tipArea) MouseIn(*desktop.MouseEvent)    { a.hoverIn(a) }
func (a *tipArea) MouseMoved(*desktop.MouseEvent) {}
func (a *tipArea) MouseOut()                      { a.hoverOut(a) }

func (a *tipArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(a.content)
}
