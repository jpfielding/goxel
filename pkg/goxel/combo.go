package goxel

import (
	"image"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// combo is egui's combo box, which roxel uses for the 2D volume picker: an
// outlined control as tall as a button, the choice at the left and a small
// filled triangle at the right; it opens a menu panel of the choices, as wide
// as itself, directly underneath. Fyne's Select has its own height, chevron
// and pop-up.
//
// In a row it goes in a comboCell, which makes it as wide as roxel's.
type combo struct {
	widget.BaseWidget
	options  []string
	selected string
	onSelect func(string)
	// showMenu opens content in a menu panel under the combo (Viewer.showMenuBelow)
	// and closeMenu closes it.
	showMenu  func(anchor fyne.CanvasObject, content fyne.CanvasObject, contentWidth float32)
	closeMenu func()

	hovered, pressed, open, focused bool
}

// egui's measures for a combo box and its menu.
const (
	comboIcon       = 14  // the square the triangle is drawn in
	comboIconGap    = 4   // between the choice and that square
	comboMinWidth   = 120 // roxel's limits for the control's width
	comboMaxWidth   = 280
	comboMenuHeight = 200 // a longer list scrolls
	menuItemPad     = 2   // a menu item's text from its left edge
	menuItemGap     = 8
)

func newCombo(options []string, selected string, onSelect func(string)) *combo {
	c := &combo{options: options, selected: selected, onSelect: onSelect}
	c.ExtendBaseWidget(c)
	return c
}

func (c *combo) padX() float32 {
	if uiTouch {
		return 12
	}
	return 10
}

// width is the control's width in a row cell of the given width, as roxel sets
// it: the cell less 8, between 120 and 280, and wider only for a choice too
// long for that.
func (c *combo) width(cell float32) float32 {
	w := max(comboMinWidth, min(comboMaxWidth, cell-8))
	need := textWidth(c.selected, buttonSize()) + comboIconGap + comboIcon + 2*c.padX()
	return min(max(w, need), max(cell, comboMinWidth))
}

// comboCellLayout places a combo at the left of its row cell, at its width.
type comboCellLayout struct{}

func (comboCellLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	c := objs[0].(*combo)
	c.Move(fyne.NewPos(0, 0))
	c.Resize(fyne.NewSize(c.width(size.Width), size.Height))
}

func (comboCellLayout) MinSize(objs []fyne.CanvasObject) fyne.Size { return objs[0].MinSize() }

// comboCell is c in the row cell it is sized from.
func comboCell(c *combo) fyne.CanvasObject { return container.New(comboCellLayout{}, c) }

// SetOpen marks the combo whose menu is up.
func (c *combo) SetOpen(on bool) {
	if c.open != on {
		c.open = on
		c.Refresh()
	}
}

func (c *combo) Tapped(*fyne.PointEvent) {
	if c.open {
		c.closeMenu()
		return
	}
	width := c.Size().Width - 2*menuPanelMargin
	items := make([]fyne.CanvasObject, 0, len(c.options))
	for _, o := range c.options {
		item := newButton(o, func() {
			c.closeMenu()
			if o != c.selected && c.onSelect != nil {
				c.onSelect(o)
			}
		})
		item.flat, item.menu, item.selected = true, true, o == c.selected
		items = append(items, item)
		// egui does not wrap a menu's items: a long one widens the panel.
		width = max(width, item.MinSize().Width)
	}
	var content fyne.CanvasObject = vstack(menuItemGap, items...)
	if content.MinSize().Height > comboMenuHeight {
		scroll := container.NewVScroll(content)
		scroll.SetMinSize(fyne.NewSize(1, comboMenuHeight))
		content = scroll
	}
	c.showMenu(c, content, width)
}

func (c *combo) MouseIn(*desktop.MouseEvent)    { c.hovered = true; c.Refresh() }
func (c *combo) MouseMoved(*desktop.MouseEvent) {}
func (c *combo) MouseOut() {
	c.hovered, c.pressed = false, false
	c.Refresh()
}
func (c *combo) MouseDown(*desktop.MouseEvent) {
	c.pressed = true
	unfocus(c) // a click is not keyboard focus
	c.Refresh()
}
func (c *combo) MouseUp(*desktop.MouseEvent) { c.pressed = false; c.Refresh() }
func (c *combo) Cursor() desktop.Cursor      { return desktop.PointerCursor }

func (c *combo) FocusGained()   { c.focused = true; c.Refresh() }
func (c *combo) FocusLost()     { c.focused = false; c.Refresh() }
func (c *combo) TypedRune(rune) {}
func (c *combo) TypedKey(e *fyne.KeyEvent) {
	if isActivate(e) {
		c.Tapped(nil)
		return
	}
	passKey(c, e)
}

func (c *combo) CreateRenderer() fyne.WidgetRenderer {
	r := &comboRenderer{c: c, bg: rrect(colControl, colOutline, 5, 1), text: canvas.NewText("", colText),
		icon: canvas.NewRaster(nil), ring: newRing(5)}
	r.icon.Generator = func(w, h int) image.Image { return triangleImage(w, h, r.iconColor) }
	return r
}

type comboRenderer struct {
	c         *combo
	bg        *canvas.Rectangle
	ring      *canvas.Rectangle
	text      *canvas.Text
	icon      *canvas.Raster
	iconColor color.NRGBA
}

func (r *comboRenderer) MinSize() fyne.Size {
	return fyne.NewSize(comboMinWidth, controlHeight())
}

func (r *comboRenderer) Layout(size fyne.Size) {
	c := r.c
	r.bg.Resize(size)
	ringRect(r.ring, size)
	pad := c.padX()
	// The triangle is 0.7 by 0.45 of its square, at the right of the content.
	iw, ih := float32(comboIcon*0.7), float32(comboIcon*0.45)
	r.icon.Resize(fyne.NewSize(iw, ih))
	r.icon.Move(fyne.NewPos(size.Width-pad-comboIcon/2-iw/2, (size.Height-ih)/2))
	label := elideEnd(c.selected, buttonSize(), size.Width-2*pad-comboIconGap-comboIcon)
	ts := measure(label, buttonSize())
	setFace(r.text, buttonSize())
	r.text.Text = label
	r.text.Move(fyne.NewPos(pad, (size.Height-ts.Height)/2))
	r.text.Resize(ts)
}

func (r *comboRenderer) Refresh() {
	c := r.c
	// egui's inactive, hovered and open widget states.
	fill, stroke, icon := color.NRGBA(colControl), color.NRGBA(colOutline), color.NRGBA{180, 180, 180, 255}
	switch {
	case c.open:
		fill, stroke, icon = colSelected, colAccent, color.NRGBA{210, 210, 210, 255}
	case c.pressed:
		fill, stroke, icon = colSelected, colAccent, color.NRGBA{255, 255, 255, 255}
	case c.hovered:
		fill, stroke, icon = colHover, colThumb, color.NRGBA{240, 240, 240, 255}
	}
	r.bg.FillColor, r.bg.StrokeColor = fill, stroke
	r.text.Color = colText
	r.iconColor = icon
	showWhen(r.ring, c.focused)
	r.Layout(c.Size())
	canvas.Refresh(r.bg)
	canvas.Refresh(r.text)
	canvas.Refresh(r.icon)
}

func (r *comboRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.text, r.icon, r.ring}
}

func (r *comboRenderer) Destroy() {}

// elideEnd cuts s to fit width, ending it with an ellipsis, as egui truncates
// a combo box's choice.
func elideEnd(s string, size, width float32) string {
	if textWidth(s, size) <= width {
		return s
	}
	chars := []rune(s)
	lo, hi := 0, len(chars)-1
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if textWidth(string(chars[:mid])+"…", size) <= width {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	return string(chars[:lo]) + "…"
}

// triangleImage draws egui's combo-box icon into w x h pixels: a filled
// triangle pointing down, its edges anti-aliased.
func triangleImage(w, h int, c color.NRGBA) image.Image {
	img := image.NewNRGBA(image.Rect(0, 0, max(w, 1), max(h, 1)))
	const sub = 4 // samples a side for each pixel
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			in := 0
			for sy := 0; sy < sub; sy++ {
				// At height fy (0 at the top, 1 at the tip) the triangle spans
				// the middle 1-fy of the width.
				fy := (float64(y) + (float64(sy)+0.5)/sub) / float64(h)
				for sx := 0; sx < sub; sx++ {
					fx := (float64(x) + (float64(sx)+0.5)/sub) / float64(w)
					if fx >= fy/2 && fx <= 1-fy/2 {
						in++
					}
				}
			}
			img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(in * int(c.A) / (sub * sub))})
		}
	}
	return img
}
