package goxel

import (
	"image/color"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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

const tipDelay = 600 * time.Millisecond

// tipLayer holds the one tooltip.
type tipLayer struct {
	*fyne.Container
	bg    *canvas.Rectangle
	label *canvas.Text
	box   *fyne.Container
	timer *time.Timer
	owner fyne.CanvasObject
}

// tooltips is the window's tooltip layer.
var tooltips *tipLayer

func newTipLayer() *tipLayer {
	t := &tipLayer{
		bg:    rrect(colPanel, colOutline, 5, 1),
		label: text("", 12, colText),
	}
	t.box = container.New(marginLayout{8, 4}, t.label)
	tip := container.NewStack(t.bg, t.box)
	tip.Hide()
	t.Container = container.NewWithoutLayout(tip)
	return t
}

func (t *tipLayer) tip() fyne.CanvasObject { return t.Container.Objects[0] }

// hover schedules the tip for anchor; hide cancels it.
func (t *tipLayer) hover(anchor fyne.CanvasObject, s string) {
	if t == nil || s == "" {
		return
	}
	t.hide(nil)
	t.owner = anchor
	t.timer = time.AfterFunc(tipDelay, func() { fyne.Do(func() { t.show(anchor, s) }) })
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
	t.tip().Hide()
}

func (t *tipLayer) show(anchor fyne.CanvasObject, s string) {
	if t.owner != anchor {
		return
	}
	d := fyne.CurrentApp().Driver()
	if d.CanvasForObject(anchor) == nil {
		return
	}
	t.label.Text = s
	t.label.Refresh()
	tip := t.tip()
	size := tip.MinSize()
	pos := d.AbsolutePositionForObject(anchor)
	win := t.Container.Size()
	x := min(max(pos.X+anchor.Size().Width/2-size.Width/2, 4), max(win.Width-size.Width-4, 4))
	y := pos.Y + anchor.Size().Height + 6
	if y+size.Height > win.Height {
		y = max(pos.Y-size.Height-6, 4)
	}
	tip.Resize(size)
	tip.Move(fyne.NewPos(x, y))
	tip.Show()
}

// tipper is the hover tip of a control.
type tipper struct{ tip string }

func (t *tipper) hoverIn(o fyne.CanvasObject)  { tooltips.hover(o, t.tip) }
func (t *tipper) hoverOut(o fyne.CanvasObject) { tooltips.hide(o) }
