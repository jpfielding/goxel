package goxel

import (
	"fmt"
	"image/color"
	"math"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// roxel's controls, drawn by hand: compact 1 px outlined buttons, one framed
// strip for exclusive choices, checkboxes, and a slider with a teal travelled
// rail and an outlined value box. Fyne's stock widgets cannot be styled to
// these shapes.

// uiTouch selects the compact layouts' touch-sized controls. It is set by
// Viewer.rebuild before the tree is built, so every control reads the class
// it is built for.
var uiTouch bool

// touchTarget is the compact layouts' minimum control height.
const touchTarget = 44

// controlHeight is the height of a button or other one-line control.
func controlHeight() float32 {
	if uiTouch {
		return touchTarget
	}
	return 28
}

// rowHeight is the height of a slider or checkbox row.
func rowHeight() float32 {
	if uiTouch {
		return touchTarget
	}
	return 24
}

// bodySize and buttonSize are roxel's body and button type sizes.
func bodySize() float32 {
	if uiTouch {
		return 16
	}
	return 13.5
}

func buttonSize() float32 {
	if uiTouch {
		return 15
	}
	return 13
}

// mix returns a blended with b, t of the way from a to b.
func mix(a, b color.NRGBA, t float32) color.NRGBA {
	l := func(x, y uint8) uint8 { return uint8(float32(x)*(1-t) + float32(y)*t + 0.5) }
	return color.NRGBA{l(a.R, b.R), l(a.G, b.G), l(a.B, b.B), 255}
}

// fade dims a colour halfway to the panel, for a disabled control: egui draws
// whatever is disabled at half opacity.
func fade(c color.NRGBA) color.NRGBA { return mix(c, colPanel, 0.5) }

func textWidth(s string, size float32) float32 {
	return measure(s, size).Width
}

func textHeight(size float32) float32 {
	return measure("Ag", size).Height
}

func rrect(fill, stroke color.Color, radius, width float32) *canvas.Rectangle {
	r := canvas.NewRectangle(fill)
	r.CornerRadius = radius
	r.StrokeColor = stroke
	r.StrokeWidth = width
	return r
}

func line(c color.Color, width float32) *canvas.Line {
	l := canvas.NewLine(c)
	l.StrokeWidth = width
	return l
}

// --- Button ----------------------------------------------------------------

type iconKind int

const (
	iconNone iconKind = iota
	iconEnlarge
	iconReduce
	iconLeft
	iconRight
	iconUp
	iconDown
)

// button is roxel's: a 1 px outlined, regular-weight control, as wide as its
// label needs and 28 px tall (44 px on touch).
type button struct {
	widget.BaseWidget
	label    string
	icon     iconKind
	onTap    func()
	primary  bool // the one dominant action: filled with the accent
	selected bool // a toggle that is on: SELECTED fill, accent outline
	flat     bool // a selectable row: no frame until hovered, filled only when selected
	menu     bool // an item of a menu: its label at the left, never an outline
	minW     float32
	minH     float32
	size     float32 // text size; 0 means the button size
	disabled bool
	tipper

	hovered, pressed, focused bool
}

func newButton(label string, onTap func()) *button {
	b := &button{label: label, onTap: onTap}
	b.ExtendBaseWidget(b)
	return b
}

func newIconButton(icon iconKind, label string, onTap func()) *button {
	b := newButton(label, onTap)
	b.icon = icon
	return b
}

// sized pins a minimum size; zero keeps the control's own.
func (b *button) sized(w, h float32) *button {
	b.minW, b.minH = w, h
	return b
}

func (b *button) textSize(s float32) *button {
	b.size = s
	return b
}

func (b *button) Tapped(*fyne.PointEvent) {
	if !b.disabled && b.onTap != nil {
		b.onTap()
	}
}

func (b *button) MouseIn(*desktop.MouseEvent) {
	b.hovered = true
	b.hoverIn(b)
	b.Refresh()
}
func (b *button) MouseMoved(*desktop.MouseEvent) {}
func (b *button) MouseOut() {
	b.hovered, b.pressed = false, false
	b.hoverOut(b)
	b.Refresh()
}
func (b *button) MouseDown(*desktop.MouseEvent) {
	b.pressed = true
	b.hoverOut(b)
	unfocus(b) // a click is not keyboard focus
	b.Refresh()
}
func (b *button) MouseUp(*desktop.MouseEvent) { b.pressed = false; b.Refresh() }
func (b *button) Cursor() desktop.Cursor      { return desktop.PointerCursor }

func (b *button) FocusGained()   { b.focused = true; b.Refresh() }
func (b *button) FocusLost()     { b.focused = false; b.Refresh() }
func (b *button) TypedRune(rune) {}
func (b *button) TypedKey(e *fyne.KeyEvent) {
	if isActivate(e) {
		b.Tapped(nil)
		return
	}
	passKey(b, e)
}

// withTip sets the tooltip shown after a short hover.
func (b *button) withTip(s string) *button {
	b.tip = s
	return b
}

func (b *button) Enable()        { b.disabled = false; b.Refresh() }
func (b *button) Disable()       { b.disabled = true; b.Refresh() }
func (b *button) Disabled() bool { return b.disabled }

// SetSelected marks a toggle as on.
func (b *button) SetSelected(on bool) {
	if b.selected != on {
		b.selected = on
		b.Refresh()
	}
}

func (b *button) textPx() float32 {
	if b.size > 0 {
		return b.size
	}
	return buttonSize()
}

// iconGap is the space between a bracket icon and its label.
const iconGap = 8

func iconSide() float32 {
	if uiTouch {
		return 14
	}
	return 12
}

func (b *button) CreateRenderer() fyne.WidgetRenderer {
	r := &buttonRenderer{b: b, bg: rrect(colControl, colOutline, 5, 1), text: canvas.NewText("", colText), ring: newRing(5)}
	for i := range r.lines {
		r.lines[i] = line(colText, 1.5)
	}
	return r
}

type buttonRenderer struct {
	b     *button
	bg    *canvas.Rectangle
	ring  *canvas.Rectangle
	text  *canvas.Text
	lines [8]*canvas.Line
}

func (r *buttonRenderer) padX() float32 {
	if uiTouch {
		return 12
	}
	return 10
}

func (r *buttonRenderer) MinSize() fyne.Size {
	b := r.b
	w := textWidth(b.label, b.textPx()) + 2*r.padX()
	if b.menu {
		w = textWidth(b.label, b.textPx()) + 2*menuItemPad
	}
	if b.icon == iconEnlarge || b.icon == iconReduce {
		w += iconSide() + iconGap
	}
	return fyne.NewSize(max(w, b.minW), max(controlHeight(), b.minH))
}

func (r *buttonRenderer) Layout(size fyne.Size) {
	b := r.b
	r.bg.Resize(size)
	ringRect(r.ring, size)
	ts := measure(b.label, b.textPx())
	setFace(r.text, b.textPx())
	r.text.Text = b.label
	cx, cy := size.Width/2, size.Height/2
	switch b.icon {
	case iconEnlarge, iconReduce:
		side := iconSide()
		total := side + iconGap + ts.Width
		left := cx - total/2
		r.layoutBracket(fyne.NewPos(left, cy-side/2), side)
		r.text.Move(fyne.NewPos(left+side+iconGap, cy-ts.Height/2))
	default:
		r.text.Move(fyne.NewPos(cx-ts.Width/2, cy-ts.Height/2))
		if b.menu {
			r.text.Move(fyne.NewPos(menuItemPad, cy-ts.Height/2))
		}
		r.layoutChevron(fyne.NewPos(cx, cy))
	}
	r.text.Resize(ts)
}

// layoutBracket draws the four corner elbows of the enlarge / reduce glyph.
func (r *buttonRenderer) layoutBracket(at fyne.Position, side float32) {
	arm := side * 0.36
	type corner struct{ x, y, dx, dy float32 }
	corners := []corner{
		{at.X, at.Y, 1, 1}, {at.X + side, at.Y, -1, 1},
		{at.X, at.Y + side, 1, -1}, {at.X + side, at.Y + side, -1, -1},
	}
	sign := float32(1)
	if r.b.icon == iconReduce {
		sign = -1
	}
	for i, c := range corners {
		ex, ey := c.x, c.y
		if r.b.icon == iconReduce {
			ex, ey = c.x+c.dx*arm, c.y+c.dy*arm
		}
		r.lines[i*2].Position1, r.lines[i*2].Position2 = fyne.NewPos(ex, ey), fyne.NewPos(ex+sign*c.dx*arm, ey)
		r.lines[i*2+1].Position1, r.lines[i*2+1].Position2 = fyne.NewPos(ex, ey), fyne.NewPos(ex, ey+sign*c.dy*arm)
	}
}

// layoutChevron draws an arrow button's chevron.
func (r *buttonRenderer) layoutChevron(c fyne.Position) {
	var tip, side fyne.Position
	switch r.b.icon {
	case iconLeft:
		tip, side = fyne.NewPos(-5, 0), fyne.NewPos(0, 7)
	case iconRight:
		tip, side = fyne.NewPos(5, 0), fyne.NewPos(0, 7)
	case iconUp:
		tip, side = fyne.NewPos(0, -5), fyne.NewPos(7, 0)
	case iconDown:
		tip, side = fyne.NewPos(0, 5), fyne.NewPos(7, 0)
	default:
		return
	}
	a := fyne.NewPos(c.X-tip.X+side.X, c.Y-tip.Y+side.Y)
	m := fyne.NewPos(c.X+tip.X, c.Y+tip.Y)
	z := fyne.NewPos(c.X-tip.X-side.X, c.Y-tip.Y-side.Y)
	r.lines[0].Position1, r.lines[0].Position2 = a, m
	r.lines[1].Position1, r.lines[1].Position2 = m, z
}

func (r *buttonRenderer) Refresh() {
	b := r.b
	fill, stroke, fg := color.NRGBA(colControl), color.NRGBA(colOutline), color.NRGBA(colText)
	switch {
	case b.disabled:
		fill, stroke, fg = fade(colControl), fade(colOutline), colDisabled
	case b.primary:
		fill, stroke, fg = colAccent, colAccent, colBG
		if b.hovered {
			fill = mix(colAccent, color.NRGBA{255, 255, 255, 255}, 0.15)
		}
	case b.pressed || b.selected:
		fill, stroke = colSelected, colAccent
	case b.hovered:
		fill, stroke = colHover, colThumb
	case b.flat:
		fill, stroke = color.NRGBA{}, color.NRGBA{}
	}
	r.bg.FillColor, r.bg.StrokeColor = fill, stroke
	r.bg.StrokeWidth = 1
	if b.selected && !b.disabled && !b.primary && !b.pressed {
		r.bg.StrokeWidth = 2 // a selected item's outline is 2 px, as in roxel
	}
	if b.menu {
		// egui's menu style: fills only.
		r.bg.StrokeColor, r.bg.StrokeWidth = color.Transparent, 0
	}
	r.text.Color = fg
	showWhen(r.ring, b.focused && !b.disabled)
	for _, l := range r.lines {
		l.StrokeColor = fg
		l.Hide()
	}
	switch b.icon {
	case iconEnlarge, iconReduce:
		for _, l := range r.lines {
			l.Show()
		}
	case iconLeft, iconRight, iconUp, iconDown:
		for _, l := range r.lines[:2] {
			l.StrokeWidth = 2
			l.Show()
		}
	}
	r.Layout(b.Size())
	canvas.Refresh(r.bg)
}

func (r *buttonRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.bg, r.text}
	for _, l := range r.lines {
		objs = append(objs, l)
	}
	return append(objs, r.ring)
}

func (r *buttonRenderer) Destroy() {}

// --- Segmented control -------------------------------------------------------

// segmented is one framed strip for a mutually exclusive choice: segments are
// sized to their labels, unselected ones are transparent, the selected one is
// filled SELECTED with an accent outline. On touch it stretches across its
// row, sharing the spare width equally.
type segmented struct {
	widget.BaseWidget
	labels   []string
	selected int
	onSelect func(int)
	disabled bool
	focused  bool
}

func newSegmented(labels []string, selected int, onSelect func(int)) *segmented {
	s := &segmented{labels: labels, selected: selected, onSelect: onSelect}
	s.ExtendBaseWidget(s)
	return s
}

// Select changes the highlighted segment without calling onSelect.
func (s *segmented) Select(i int) {
	if s.selected != i {
		s.selected = i
		s.Refresh()
	}
}

func segmentedSize() float32 { return buttonSize() }

func (s *segmented) frameHeight() float32 {
	if uiTouch {
		return touchTarget
	}
	return 30 // 24 + 2 margin and 1 stroke on each side
}

// widths returns each segment's width for a strip of the given inner width.
func (s *segmented) widths(inner float32) []float32 {
	size := segmentedSize()
	texts := make([]float32, len(s.labels))
	var sum float32
	for i, l := range s.labels {
		texts[i] = textWidth(l, size)
		sum += texts[i]
	}
	out := make([]float32, len(s.labels))
	n := float32(len(s.labels))
	if uiTouch {
		spare := max((inner-2*(n-1)-sum)/n, 8)
		for i := range out {
			out[i] = float32(math.Floor(float64(texts[i] + spare)))
		}
		return out
	}
	for i := range out {
		out[i] = texts[i] + 18
	}
	return out
}

func (s *segmented) Tapped(e *fyne.PointEvent) {
	if s.disabled {
		return
	}
	x := float32(3)
	for i, w := range s.widths(s.Size().Width - 6) {
		if e.Position.X < x+w+1 || i == len(s.labels)-1 {
			if i != s.selected {
				s.selected = i
				s.Refresh()
				if s.onSelect != nil {
					s.onSelect(i)
				}
			}
			return
		}
		x += w + 2
	}
}

func (s *segmented) Cursor() desktop.Cursor { return desktop.PointerCursor }

// A click is not keyboard focus: it also hands keys back to the window.
func (s *segmented) MouseDown(*desktop.MouseEvent) { unfocus(s) }
func (s *segmented) MouseUp(*desktop.MouseEvent)   {}

func (s *segmented) FocusGained()   { s.focused = true; s.Refresh() }
func (s *segmented) FocusLost()     { s.focused = false; s.Refresh() }
func (s *segmented) TypedRune(rune) {}

// TypedKey: Left and Right move the selection, as in a group of radio buttons.
func (s *segmented) TypedKey(e *fyne.KeyEvent) {
	d := 0
	switch e.Name {
	case fyne.KeyLeft:
		d = -1
	case fyne.KeyRight:
		d = 1
	default:
		passKey(s, e)
		return
	}
	if i := s.selected + d; i >= 0 && i < len(s.labels) && !s.disabled {
		s.selected = i
		s.Refresh()
		if s.onSelect != nil {
			s.onSelect(i)
		}
	}
}

func (s *segmented) Enable()        { s.disabled = false; s.Refresh() }
func (s *segmented) Disable()       { s.disabled = true; s.Refresh() }
func (s *segmented) Disabled() bool { return s.disabled }

func (s *segmented) CreateRenderer() fyne.WidgetRenderer {
	r := &segmentedRenderer{s: s, frame: rrect(colBG, colHairline, 7, 1), ring: newRing(5)}
	for range s.labels {
		r.segs = append(r.segs, rrect(color.Transparent, color.Transparent, 5, 1))
		r.texts = append(r.texts, canvas.NewText("", colMuted))
	}
	return r
}

type segmentedRenderer struct {
	s     *segmented
	ring  *canvas.Rectangle
	frame *canvas.Rectangle
	segs  []*canvas.Rectangle
	texts []*canvas.Text
}

func (r *segmentedRenderer) MinSize() fyne.Size {
	var w float32
	for _, sw := range r.s.widths(0) {
		w += sw
	}
	w += 2*float32(len(r.s.labels)-1) + 6
	return fyne.NewSize(w, r.s.frameHeight())
}

func (r *segmentedRenderer) Layout(size fyne.Size) {
	r.frame.Resize(size)
	x := float32(3)
	h := size.Height - 6
	for i, w := range r.s.widths(size.Width - 6) {
		r.segs[i].Move(fyne.NewPos(x, 3))
		r.segs[i].Resize(fyne.NewSize(w, h))
		ts := measure(r.s.labels[i], segmentedSize())
		setFace(r.texts[i], segmentedSize())
		r.texts[i].Move(fyne.NewPos(x+(w-ts.Width)/2, 3+(h-ts.Height)/2))
		r.texts[i].Resize(ts)
		if i == r.s.selected {
			// The ring goes around the selected segment.
			r.ring.Move(fyne.NewPos(x-ringWidth, 3-ringWidth))
			r.ring.Resize(fyne.NewSize(w+2*ringWidth, h+2*ringWidth))
		}
		x += w + 2
	}
}

func (r *segmentedRenderer) Refresh() {
	for i := range r.segs {
		on := i == r.s.selected
		fill, stroke, fg := color.Color(color.Transparent), color.Color(color.Transparent), color.NRGBA(colMuted)
		if on {
			fill, stroke, fg = colSelected, colAccent, colHeading
		}
		if r.s.disabled {
			stroke, fg = color.Transparent, colDisabled
			if on {
				fill = fade(colSelected)
			}
		}
		r.segs[i].FillColor, r.segs[i].StrokeColor = fill, stroke
		r.texts[i].Text, r.texts[i].Color = r.s.labels[i], fg
	}
	showWhen(r.ring, r.s.focused && !r.s.disabled)
	r.Layout(r.s.Size())
	canvas.Refresh(r.frame)
}

func (r *segmentedRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.frame}
	for i := range r.segs {
		objs = append(objs, r.segs[i], r.texts[i])
	}
	return append(objs, r.ring)
}

func (r *segmentedRenderer) Destroy() {}

// --- Checkbox ----------------------------------------------------------------

// checkbox is a box that is unmistakably empty or ticked: dark with a light
// outline when off, filled with the accent and a dark tick when on.
type checkbox struct {
	widget.BaseWidget
	label     string
	checked   bool
	onChanged func(bool)
	disabled  bool
	hovered   bool
	focused   bool
	tipper
}

func newCheckbox(label string, checked bool, onChanged func(bool)) *checkbox {
	c := &checkbox{label: label, checked: checked, onChanged: onChanged}
	c.ExtendBaseWidget(c)
	return c
}

// newCheck is a checkbox whose toggle calls onChanged.
func newCheck(label string, checked bool, onChanged func(bool)) *checkbox {
	return newCheckbox(label, checked, onChanged)
}

// SetChecked changes the box without calling onChanged.
func (c *checkbox) SetChecked(on bool) {
	if c.checked != on {
		c.checked = on
		c.Refresh()
	}
}

func (c *checkbox) Tapped(*fyne.PointEvent) {
	if c.disabled {
		return
	}
	c.checked = !c.checked
	c.Refresh()
	if c.onChanged != nil {
		c.onChanged(c.checked)
	}
}

func (c *checkbox) MouseIn(*desktop.MouseEvent) {
	c.hovered = true
	c.hoverIn(c)
	c.Refresh()
}
func (c *checkbox) MouseMoved(*desktop.MouseEvent) {}
func (c *checkbox) MouseOut() {
	c.hovered = false
	c.hoverOut(c)
	c.Refresh()
}
func (c *checkbox) MouseDown(*desktop.MouseEvent) {
	c.hoverOut(c)
	unfocus(c) // a click is not keyboard focus
}
func (c *checkbox) MouseUp(*desktop.MouseEvent) {}
func (c *checkbox) Cursor() desktop.Cursor      { return desktop.PointerCursor }

func (c *checkbox) FocusGained()   { c.focused = true; c.Refresh() }
func (c *checkbox) FocusLost()     { c.focused = false; c.Refresh() }
func (c *checkbox) TypedRune(rune) {}
func (c *checkbox) TypedKey(e *fyne.KeyEvent) {
	if isActivate(e) {
		c.Tapped(nil)
		return
	}
	passKey(c, e)
}

// withTip sets the tooltip shown after a short hover.
func (c *checkbox) withTip(s string) *checkbox {
	c.tip = s
	return c
}
func (c *checkbox) Enable()        { c.disabled = false; c.Refresh() }
func (c *checkbox) Disable()       { c.disabled = true; c.Refresh() }
func (c *checkbox) Disabled() bool { return c.disabled }

func (c *checkbox) side() float32 {
	if uiTouch {
		return 22
	}
	return 16
}

func (c *checkbox) gap() float32 {
	switch {
	case c.label == "":
		return 0
	case uiTouch:
		return 12
	}
	return 8
}

func (c *checkbox) CreateRenderer() fyne.WidgetRenderer {
	r := &checkboxRenderer{c: c, box: rrect(colBG, colOutline, 4, 1.5), text: canvas.NewText("", colText), ring: newRing(4)}
	r.tick[0], r.tick[1] = line(colBG, 2), line(colBG, 2)
	return r
}

type checkboxRenderer struct {
	c    *checkbox
	ring *canvas.Rectangle
	box  *canvas.Rectangle
	tick [2]*canvas.Line
	text *canvas.Text
}

func (r *checkboxRenderer) MinSize() fyne.Size {
	c := r.c
	h := max(rowHeight(), textHeight(bodySize()))
	return fyne.NewSize(c.side()+c.gap()+textWidth(c.label, bodySize()), h)
}

func (r *checkboxRenderer) Layout(size fyne.Size) {
	c := r.c
	side := c.side()
	top := (size.Height - side) / 2
	r.box.Move(fyne.NewPos(0, top))
	r.box.Resize(fyne.NewSize(side, side))
	ts := measure(c.label, bodySize())
	setFace(r.text, bodySize())
	r.text.Move(fyne.NewPos(side+c.gap(), (size.Height-ts.Height)/2))
	r.text.Resize(ts)
	ringRect(r.ring, size)
	s := side * 0.26
	cx, cy := side/2, top+side/2
	r.tick[0].Position1, r.tick[0].Position2 = fyne.NewPos(cx-s, cy), fyne.NewPos(cx-s*0.25, cy+s*0.75)
	r.tick[1].Position1, r.tick[1].Position2 = fyne.NewPos(cx-s*0.25, cy+s*0.75), fyne.NewPos(cx+s, cy-s*0.7)
}

func (r *checkboxRenderer) Refresh() {
	c := r.c
	fg, outline, accent := color.NRGBA(colText), color.NRGBA(colOutline), color.NRGBA(colAccent)
	if c.disabled {
		fg, outline, accent = colDisabled, fade(colOutline), fade(colAccent)
	} else if c.hovered {
		outline = colThumb
	}
	if c.checked {
		r.box.FillColor, r.box.StrokeColor, r.box.StrokeWidth = accent, color.Transparent, 0
	} else {
		r.box.FillColor, r.box.StrokeColor, r.box.StrokeWidth = colBG, outline, 1.5
	}
	for _, t := range r.tick {
		if c.checked {
			t.Show()
		} else {
			t.Hide()
		}
	}
	r.text.Text, r.text.Color = c.label, fg
	showWhen(r.ring, c.focused && !c.disabled)
	r.Layout(c.Size())
	canvas.Refresh(r.box)
}

func (r *checkboxRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.box, r.tick[0], r.tick[1], r.text, r.ring}
}

func (r *checkboxRenderer) Destroy() {}

// --- Slider track ------------------------------------------------------------

// track is the rail and thumb of a control row: a light thumb on a mid-tone
// rail, the travelled part in the accent, so position reads at a glance.
type track struct {
	widget.BaseWidget
	lo, hi    float64
	value     float64
	decimals  int
	onChanged func(float64)
	onEnd     func(float64) // a drag ended, the rail was tapped or a key moved it
	disabled  bool
	dragging  bool
	focused   bool

	// What an arrow key does: one step, kept inside [limLo, limHi].
	step         float64
	limLo, limHi float64
}

func newTrack(lo, hi, value float64, decimals int) *track {
	t := &track{lo: lo, hi: hi, value: value, decimals: decimals, step: 1, limLo: lo, limHi: hi}
	t.ExtendBaseWidget(t)
	return t
}

// knobStroke is the width of the dark outline around a slider thumb.
const knobStroke = 1.5

func (t *track) radius() float32 {
	if uiTouch {
		return 12
	}
	return 7
}

func (t *track) snap(v float64) float64 {
	scale := math.Pow(10, float64(t.decimals))
	return math.Round(v*scale) / scale
}

func (t *track) valueAt(x float32) float64 {
	r := t.radius()
	span := t.hi - t.lo
	f := float64((x - r) / max(t.Size().Width-2*r, 1))
	return t.snap(t.lo + math.Max(0, math.Min(1, f))*span)
}

func (t *track) SetValue(v float64) {
	if t.value != v {
		t.value = v
		t.Refresh()
	}
}

// SetRange changes what the rail covers.
func (t *track) SetRange(lo, hi float64) {
	t.lo, t.hi = lo, hi
	t.Refresh()
}

func (t *track) set(v float64) {
	if v == t.value {
		return
	}
	t.value = v
	t.Refresh()
	if t.onChanged != nil {
		t.onChanged(v)
	}
}

func (t *track) Tapped(e *fyne.PointEvent) {
	if t.disabled {
		return
	}
	t.set(t.valueAt(e.Position.X))
	if t.onEnd != nil {
		t.onEnd(t.value)
	}
}

func (t *track) Dragged(e *fyne.DragEvent) {
	if t.disabled {
		return
	}
	t.dragging = true
	t.set(t.valueAt(e.Position.X))
}

func (t *track) DragEnd() {
	if t.dragging {
		t.dragging = false
		t.Refresh()
		if t.onEnd != nil {
			t.onEnd(t.value)
		}
	}
}

func (t *track) Cursor() desktop.Cursor { return desktop.PointerCursor }

// A click is not keyboard focus: it hands keys back to the window (Esc, arrow
// keys stepping slices) and commits a number being typed.
func (t *track) MouseDown(*desktop.MouseEvent) { unfocus(t) }
func (t *track) MouseUp(*desktop.MouseEvent)   {}

func (t *track) FocusGained()   { t.focused = true; t.Refresh() }
func (t *track) FocusLost()     { t.focused = false; t.Refresh() }
func (t *track) TypedRune(rune) {}

// TypedKey: the arrow keys move the thumb one step; each press is final, like
// a stepper press.
func (t *track) TypedKey(e *fyne.KeyEvent) {
	d := 0.0
	switch e.Name {
	case fyne.KeyLeft, fyne.KeyDown:
		d = -t.step
	case fyne.KeyRight, fyne.KeyUp:
		d = t.step
	case fyne.KeyHome:
		d = t.limLo - t.value
	case fyne.KeyEnd:
		d = t.limHi - t.value
	default:
		passKey(t, e)
		return
	}
	if t.disabled {
		return
	}
	v := math.Max(t.limLo, math.Min(t.limHi, t.snap(t.value+d)))
	if v == t.value {
		return
	}
	t.set(v)
	if t.onEnd != nil {
		t.onEnd(v)
	}
}

func (t *track) Enable()        { t.disabled = false; t.Refresh() }
func (t *track) Disable()       { t.disabled = true; t.Refresh() }
func (t *track) Disabled() bool { return t.disabled }

func (t *track) CreateRenderer() fyne.WidgetRenderer {
	return &trackRenderer{
		t:    t,
		rail: rrect(colRail, color.Transparent, 2, 0),
		fill: rrect(colAccent, color.Transparent, 2, 0),
		knob: canvas.NewCircle(colThumb),
		ring: canvas.NewCircle(color.Transparent),
	}
}

type trackRenderer struct {
	t    *track
	rail *canvas.Rectangle
	fill *canvas.Rectangle
	knob *canvas.Circle
	ring *canvas.Circle
}

func (r *trackRenderer) MinSize() fyne.Size { return fyne.NewSize(40, rowHeight()) }

func (r *trackRenderer) Layout(size fyne.Size) {
	t := r.t
	rad := t.radius()
	half := float32(2)
	if uiTouch {
		half = 3
	}
	x0, x1 := rad, size.Width-rad
	y := size.Height / 2
	span := t.hi - t.lo
	f := float32(0)
	if span > 0 {
		f = float32(math.Max(0, math.Min(1, (t.value-t.lo)/span)))
	}
	x := x0 + f*(x1-x0)
	r.rail.CornerRadius, r.fill.CornerRadius = half, half
	r.rail.Move(fyne.NewPos(x0, y-half))
	r.rail.Resize(fyne.NewSize(max(x1-x0, 0), 2*half))
	r.fill.Move(fyne.NewPos(x0, y-half))
	r.fill.Resize(fyne.NewSize(max(x-x0, 0), 2*half))
	grow := float32(0)
	if t.dragging {
		grow = 1
	}
	rr := rad + grow
	// egui draws the thumb's dark outline outside its radius; Fyne strokes
	// inside the circle it is given, so the circle is that much larger.
	kr := rr + knobStroke
	r.knob.Move(fyne.NewPos(x-kr, y-kr))
	r.knob.Resize(fyne.NewSize(2*kr, 2*kr))
	// The focus ring circles the thumb, 3 px out (its outer edge is 4 px out).
	rg := rr + 2 + ringWidth
	r.ring.Move(fyne.NewPos(x-rg, y-rg))
	r.ring.Resize(fyne.NewSize(2*rg, 2*rg))
}

func (r *trackRenderer) Refresh() {
	rail, fill, knob := color.NRGBA(colRail), color.NRGBA(colAccent), color.NRGBA(colThumb)
	if r.t.disabled {
		rail, fill, knob = fade(colRail), fade(colAccent), fade(colThumb)
	}
	r.rail.FillColor, r.fill.FillColor, r.knob.FillColor = rail, fill, knob
	r.knob.StrokeColor, r.knob.StrokeWidth = colBG, knobStroke
	r.ring.StrokeColor, r.ring.StrokeWidth = colAccent, ringWidth
	showWhen(r.ring, r.t.focused && !r.t.disabled)
	r.Layout(r.t.Size())
	canvas.Refresh(r.rail)
}

func (r *trackRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.rail, r.fill, r.knob, r.ring}
}

func (r *trackRenderer) Destroy() {}

// --- Value box -----------------------------------------------------------------

// valueBox is the outlined number beside a slider. Click to type a value,
// drag sideways to change it.
type valueBox struct {
	widget.BaseWidget
	text     string
	speed    float64 // value change per pixel dragged
	parse    func(string) (float64, bool)
	apply    func(v float64, committed bool)
	value    func() float64
	disabled bool

	// plain is a box in a menu panel: no frame until it is hovered or edited,
	// as egui's menu style draws roxel's.
	plain bool

	editing bool
	fresh   bool // the next typed character replaces the text
	buf     string
	hovered bool
	dragged bool
}

func newValueBox(text string, speed float64) *valueBox {
	b := &valueBox{text: text, speed: speed}
	b.ExtendBaseWidget(b)
	return b
}

func (b *valueBox) SetText(s string) {
	if b.text != s {
		b.text = s
		b.Refresh()
	}
}

func (b *valueBox) Tapped(*fyne.PointEvent) {
	if b.disabled {
		return
	}
	if c := fyne.CurrentApp().Driver().CanvasForObject(b); c != nil {
		c.Focus(b)
	}
}

func (b *valueBox) FocusGained() {
	b.editing, b.fresh, b.buf = true, true, b.text
	b.Refresh()
}

func (b *valueBox) FocusLost() {
	if b.editing {
		b.commit()
	}
}

func (b *valueBox) commit() {
	b.editing = false
	if v, ok := b.parse(strings.TrimSpace(b.buf)); ok && b.apply != nil {
		b.apply(v, true)
	}
	b.Refresh()
}

func (b *valueBox) TypedRune(r rune) {
	if !b.editing {
		return
	}
	if b.fresh {
		b.buf, b.fresh = "", false
	}
	b.buf += string(r)
	b.Refresh()
}

func (b *valueBox) TypedKey(e *fyne.KeyEvent) {
	if !b.editing {
		return
	}
	switch e.Name {
	case fyne.KeyBackspace:
		if b.fresh {
			b.buf, b.fresh = "", false
		} else if r := []rune(b.buf); len(r) > 0 {
			b.buf = string(r[:len(r)-1])
		}
		b.Refresh()
	case fyne.KeyReturn, fyne.KeyEnter:
		unfocus(b)
	case fyne.KeyEscape:
		b.editing = false
		unfocus(b)
		b.Refresh()
	}
}

func (b *valueBox) Dragged(e *fyne.DragEvent) {
	if b.disabled || b.value == nil || b.apply == nil {
		return
	}
	b.dragged = true
	b.apply(b.value()+float64(e.Dragged.DX)*b.speed, false)
}

func (b *valueBox) DragEnd() {
	if b.dragged {
		b.dragged = false
		if b.apply != nil && b.value != nil {
			b.apply(b.value(), true)
		}
	}
}

func (b *valueBox) MouseIn(*desktop.MouseEvent)    { b.hovered = true; b.Refresh() }
func (b *valueBox) MouseMoved(*desktop.MouseEvent) {}
func (b *valueBox) MouseOut()                      { b.hovered = false; b.Refresh() }
func (b *valueBox) Cursor() desktop.Cursor         { return desktop.HResizeCursor }
func (b *valueBox) Enable()                        { b.disabled = false; b.Refresh() }
func (b *valueBox) Disable()                       { b.disabled = true; b.Refresh() }
func (b *valueBox) Disabled() bool                 { return b.disabled }

func (b *valueBox) CreateRenderer() fyne.WidgetRenderer {
	r := &valueBoxRenderer{b: b, bg: rrect(colControl, colOutline, 5, 1),
		text: canvas.NewText("", colText), caret: line(colAccent, 1)}
	r.caret.Hide() // shown only while editing
	return r
}

type valueBoxRenderer struct {
	b     *valueBox
	bg    *canvas.Rectangle
	text  *canvas.Text
	caret *canvas.Line
}

func (r *valueBoxRenderer) MinSize() fyne.Size { return fyne.NewSize(56, 24) }

func (r *valueBoxRenderer) Layout(size fyne.Size) {
	b := r.b
	r.bg.Resize(size)
	s := b.text
	if b.editing {
		s = b.buf
	}
	r.text.Text = s
	setFace(r.text, bodySize())
	ts := measure(s, bodySize())
	x := (size.Width - ts.Width) / 2
	if ts.Width > size.Width-8 {
		x = size.Width - 4 - ts.Width // keep the end, where typing happens
	}
	r.text.Move(fyne.NewPos(x, (size.Height-ts.Height)/2))
	r.text.Resize(ts)
	r.caret.Position1 = fyne.NewPos(x+ts.Width+1, 4)
	r.caret.Position2 = fyne.NewPos(x+ts.Width+1, size.Height-4)
}

func (r *valueBoxRenderer) Refresh() {
	b := r.b
	fill, stroke, fg := color.NRGBA(colControl), color.NRGBA(colOutline), color.NRGBA(colText)
	switch {
	case b.disabled:
		fill, stroke, fg = fade(colControl), fade(colOutline), colDisabled
	case b.editing:
		fill, stroke = colBG, colAccent
	case b.hovered:
		fill, stroke = colHover, colThumb
	}
	if b.plain && !b.editing {
		stroke = color.NRGBA{}
		if !b.hovered || b.disabled {
			fill = color.NRGBA{}
		}
	}
	r.bg.FillColor, r.bg.StrokeColor = fill, stroke
	r.text.Color = fg
	if b.editing {
		r.caret.Show()
	} else {
		r.caret.Hide()
	}
	r.Layout(b.Size())
	canvas.Refresh(r.bg)
}

func (r *valueBoxRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.text, r.caret}
}

func (r *valueBoxRenderer) Destroy() {}

// --- Slider row ------------------------------------------------------------------

// sliderSpec is what one control row edits.
type sliderSpec struct {
	label    string
	lo, hi   float64 // the range the track covers
	limLo    float64 // the range the number box and steppers accept
	limHi    float64
	step     float64 // one press of a stepper
	decimals int
	offset   float64 // added for display (1 for a one-based slice number)
	unit     string
	total    int // shown as "value / total" on touch when positive
}

func fraction(label string, lo, hi float64) sliderSpec {
	return sliderSpec{label: label, lo: lo, hi: hi, limLo: lo, limHi: hi, step: 0.05, decimals: 2}
}

func whole(label string, lo, hi, step float64) sliderSpec {
	return sliderSpec{label: label, lo: lo, hi: hi, limLo: lo, limHi: hi, step: step}
}

func (s sliderSpec) limit(lo, hi float64) sliderSpec { s.limLo, s.limHi = lo, hi; return s }
func (s sliderSpec) withUnit(u string) sliderSpec    { s.unit = u; return s }

func (s sliderSpec) display(v float64) string {
	return fmt.Sprintf("%.*f%s", s.decimals, v+s.offset, s.unit)
}

// sliderRow is roxel's one control row. Desktop: label column, track, number
// box. Touch: label over value, minus, track, plus, at full-size targets.
type sliderRow struct {
	obj    fyne.CanvasObject
	spec   sliderSpec
	track  *track
	box    *valueBox
	minus  *button
	plus   *button
	value  *canvas.Text // touch readout
	onEnd  func(float64)
	change func(float64)
}

// SetValue moves the track and readout without calling the callbacks.
func (r *sliderRow) SetValue(v float64) {
	r.track.SetValue(v)
	r.show(v)
}

func (r *sliderRow) show(v float64) {
	if r.box != nil {
		r.box.SetText(r.spec.display(v))
	}
	if r.value != nil {
		s := r.spec.display(v)
		if r.spec.total > 0 {
			s = fmt.Sprintf("%s / %d", s, r.spec.total)
		}
		r.value.Text = s
		r.value.Refresh()
	}
	r.updateSteppers()
}

func (r *sliderRow) updateSteppers() {
	if r.minus == nil {
		return
	}
	off := r.track.Disabled()
	r.minus.disabled = off || r.track.value <= r.spec.limLo
	r.plus.disabled = off || r.track.value >= r.spec.limHi
	r.minus.Refresh()
	r.plus.Refresh()
}

// SetRange changes what the track covers and the total shown beside the value.
func (r *sliderRow) SetRange(lo, hi float64, total int) {
	r.spec.lo, r.spec.hi, r.spec.limLo, r.spec.limHi, r.spec.total = lo, hi, lo, hi, total
	r.track.limLo, r.track.limHi = lo, hi
	r.track.SetRange(lo, hi)
	r.show(r.track.value)
}

// SetEnabled enables or disables the whole row.
func (r *sliderRow) SetEnabled(on bool) {
	if on {
		r.track.Enable()
		if r.box != nil {
			r.box.Enable()
		}
	} else {
		r.track.Disable()
		if r.box != nil {
			r.box.Disable()
		}
	}
	r.updateSteppers()
}

// newSliderRow builds a row. onChanged runs on every move; onCommitted when a
// drag ends, a stepper is pressed or a number is typed (nil means onChanged
// only).
func newSliderRow(spec sliderSpec, value float64, onChanged, onCommitted func(float64)) *sliderRow {
	value = math.Max(spec.limLo, math.Min(spec.limHi, value))
	r := &sliderRow{spec: spec, change: onChanged, onEnd: onCommitted}
	r.track = newTrack(spec.lo, spec.hi, value, spec.decimals)
	r.track.step, r.track.limLo, r.track.limHi = spec.step, spec.limLo, spec.limHi
	r.track.onChanged = func(v float64) {
		r.show(v)
		if onChanged != nil {
			onChanged(v)
		}
	}
	r.track.onEnd = func(v float64) {
		r.show(v)
		if onCommitted != nil {
			onCommitted(v)
		}
	}
	set := func(v float64, committed bool) {
		v = math.Max(r.spec.limLo, math.Min(r.spec.limHi, r.track.snap(v)))
		r.track.SetValue(v)
		r.show(v)
		if onChanged != nil {
			onChanged(v)
		}
		if committed && onCommitted != nil {
			onCommitted(v)
		}
	}
	if !uiTouch {
		speed := 1.0
		if spec.decimals > 0 {
			speed = 0.01
		}
		r.box = newValueBox(spec.display(value), speed)
		r.box.value = func() float64 { return r.track.value }
		r.box.parse = func(s string) (float64, bool) {
			s = strings.TrimSpace(strings.TrimSuffix(s, r.spec.unit))
			v, err := strconv.ParseFloat(s, 64)
			return v - r.spec.offset, err == nil
		}
		r.box.apply = set
		r.obj = flexRow(8, 1, fixedWidth(64, rowLabel(spec.label)), r.track, r.box)
		return r
	}
	step := func(d float64) {
		set(r.track.value+d, true)
	}
	r.minus = newButton("−", func() { step(-r.spec.step) }).textSize(22).sized(touchTarget, touchTarget)
	r.plus = newButton("+", func() { step(r.spec.step) }).textSize(22).sized(touchTarget, touchTarget)
	label := canvas.NewText(spec.label, colMuted)
	setFace(label, 13)
	r.value = canvas.NewText("", colText)
	setFace(r.value, 15)
	cell := container.New(touchLabelLayout{label: label, value: r.value}, label, r.value)
	r.obj = flexRow(10, 2, cell, r.minus, r.track, r.plus)
	r.show(value)
	return r
}

// touchLabelLayout stacks a muted label over a value in a touch row's first cell.
type touchLabelLayout struct{ label, value *canvas.Text }

func (l touchLabelLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	ls := l.label.MinSize()
	vs := l.value.MinSize()
	l.label.Move(fyne.NewPos(0, 3))
	l.label.Resize(ls)
	l.value.Move(fyne.NewPos(0, size.Height-3-vs.Height))
	l.value.Resize(vs)
}

func (l touchLabelLayout) MinSize([]fyne.CanvasObject) fyne.Size {
	return fyne.NewSize(78, touchTarget)
}
