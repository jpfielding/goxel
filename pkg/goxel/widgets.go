package goxel

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// sizedLayout pins its single child to a fixed width and/or height (0 = the
// child's own minimum on that axis).
type sizedLayout struct{ w, h float32 }

func (l sizedLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (l sizedLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	if l.w > 0 {
		m.Width = l.w
	}
	if l.h > 0 {
		m.Height = max(m.Height, l.h)
	}
	return m
}

func fixedWidth(w float32, o fyne.CanvasObject) *fyne.Container {
	return container.New(sizedLayout{w: w}, o)
}

func minHeight(h float32, o fyne.CanvasObject) *fyne.Container {
	return container.New(sizedLayout{h: h}, o)
}

// shrinkLayout lays its children over the whole area but reports a tiny
// minimum size, so long content (labels, file names) never widens the window.
type shrinkLayout struct{ minW, minH float32 }

func (l shrinkLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(0, 0))
		o.Resize(size)
	}
}

func (l shrinkLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var h float32
	for _, o := range objs {
		h = max(h, o.MinSize().Height)
	}
	if l.minH > 0 {
		h = l.minH
	}
	return fyne.NewSize(l.minW, h)
}

func shrinkable(o fyne.CanvasObject) *fyne.Container {
	return container.New(shrinkLayout{minW: 1}, o)
}

// text is one line of regular-weight text.
func text(s string, size float32, c color.Color) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	return t
}

func filled(c color.Color) *canvas.Rectangle { return canvas.NewRectangle(c) }

// panel puts content on a filled background with a uniform inner margin.
func panel(fill color.Color, margin float32, content fyne.CanvasObject) *fyne.Container {
	return container.NewStack(filled(fill), container.New(marginLayout{margin, margin}, content))
}

// marginLayout insets its children by x and y.
type marginLayout struct{ x, y float32 }

func (l marginLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(l.x, l.y))
		o.Resize(fyne.NewSize(max(size.Width-2*l.x, 0), max(size.Height-2*l.y, 0)))
	}
}

func (l marginLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	return m.Add(fyne.NewSize(2*l.x, 2*l.y))
}

// insetsLayout insets its children by a different amount on each side.
type insetsLayout struct{ left, right, top, bottom float32 }

func (l insetsLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		o.Move(fyne.NewPos(l.left, l.top))
		o.Resize(fyne.NewSize(max(size.Width-l.left-l.right, 0), max(size.Height-l.top-l.bottom, 0)))
	}
}

func (l insetsLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	return m.Add(fyne.NewSize(l.left+l.right, l.top+l.bottom))
}

func inset(x, y float32, o fyne.CanvasObject) *fyne.Container {
	return container.New(marginLayout{x, y}, o)
}

// frame draws a rounded outline around content with the given inner margin.
func frame(stroke color.Color, radius, mx, my float32, content fyne.CanvasObject) *fyne.Container {
	return container.NewStack(rrect(color.Transparent, stroke, radius, 1), inset(mx, my, content))
}

// markLayout draws roxel's mark: a cube outline, a hexagon with three spokes
// from its centre to alternate corners.
type markLayout struct{ size float32 }

func (l markLayout) Layout(objs []fyne.CanvasObject, _ fyne.Size) {
	c := fyne.NewPos(l.size/2, l.size/2)
	a := l.size * 0.40
	pts := [6]fyne.Position{
		{X: c.X, Y: c.Y - a}, {X: c.X + a, Y: c.Y - a/2}, {X: c.X + a, Y: c.Y + a/2},
		{X: c.X, Y: c.Y + a}, {X: c.X - a, Y: c.Y + a/2}, {X: c.X - a, Y: c.Y - a/2},
	}
	for i := 0; i < 6; i++ {
		ln := objs[i].(*canvas.Line)
		ln.Position1, ln.Position2 = pts[i], pts[(i+1)%6]
	}
	for k, i := range []int{1, 3, 5} {
		ln := objs[6+k].(*canvas.Line)
		ln.Position1, ln.Position2 = c, pts[i]
	}
}

func (l markLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(l.size, l.size) }

// markImage is goxel's mark (roxel's cube outline) in the accent, at the given size.
func markImage(size float32) fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 9)
	for i := range objs {
		objs[i] = line(colAccent, 1.5)
	}
	return container.New(markLayout{size}, objs...)
}

// sectionTitle is a group heading with an optional muted note at the right,
// such as the scale its numbers are in.
func sectionTitle(title, note string) fyne.CanvasObject {
	size, noteSize, height := float32(14.5), float32(12), float32(28)
	if uiTouch {
		size, noteSize, height = 16, 13, touchTarget
	}
	h := leftCentre(text(title, size, colHeading))
	var row fyne.CanvasObject = h
	if note != "" {
		row = border(nil, nil, nil, rightCentre(text(note, noteSize, colMuted)), h)
	}
	// A heading is a full row, as in roxel: its text sits mid-row.
	return minHeight(height, row)
}

// pageTitle is the name of a panel or page: its largest text.
func pageTitle(s string) fyne.CanvasObject {
	size := float32(17)
	if uiTouch {
		size = 18
	}
	return leftCentre(text(s, size, colHeading))
}

// rowLabel is a muted row label in the column the control rows use.
func rowLabel(s string) fyne.CanvasObject {
	size := float32(13)
	if uiTouch {
		size = 14
	}
	return leftCentre(text(s, size, colMuted))
}

// keyValue is a muted caption, then its value starting in the same column:
// `Voxels   256 × 256 × 128`.
func keyValue(key, value string) fyne.CanvasObject {
	h, col, gap, size, keySize := float32(22), float32(68), float32(8), bodySize(), float32(13)
	if uiTouch {
		h, col, gap, keySize = 28, 84, 10, 14
	}
	// The value starts a row gap after the label column.
	return minHeight(h, flexRow(gap, 1, fixedWidth(col, leftCentre(text(key, keySize, colMuted))),
		leftCentre(text(value, size, colText))))
}

// note is wrapped muted body text.
func note(s string) *wrappedText { return newWrapped(s, bodySize(), colMuted) }

// bodyNote is wrapped body text in the full text colour.
func bodyNote(s string) *wrappedText { return newWrapped(s, bodySize(), colText) }

func hairline() fyne.CanvasObject {
	r := filled(colHairline)
	r.SetMinSize(fyne.NewSize(1, 1))
	return r
}

// edgeSide is the side of a panel its separator line is on.
type edgeSide int

const (
	edgeTop edgeSide = iota
	edgeBottom
	edgeLeft
	edgeRight
)

// edgeLayout draws a one-pixel line along one side of the area it is given.
type edgeLineLayout struct{ side edgeSide }

func (l edgeLineLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	switch l.side {
	case edgeTop:
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(fyne.NewSize(size.Width, 1))
	case edgeBottom:
		objs[0].Move(fyne.NewPos(0, size.Height-1))
		objs[0].Resize(fyne.NewSize(size.Width, 1))
	case edgeLeft:
		objs[0].Move(fyne.NewPos(0, 0))
		objs[0].Resize(fyne.NewSize(1, size.Height))
	default:
		objs[0].Move(fyne.NewPos(size.Width-1, 0))
		objs[0].Resize(fyne.NewSize(1, size.Height))
	}
}

func (edgeLineLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(1, 1) }

// edged draws a panel's separator line on one of its sides, over the panel
// rather than beside it: as in roxel, the line takes no room of its own.
func edged(content fyne.CanvasObject, side edgeSide) fyne.CanvasObject {
	return container.NewStack(content, container.New(edgeLineLayout{side}, filled(colHairline)))
}

// ruleLayout centres a one-pixel line in a nine-pixel row.
type ruleLayout struct{}

func (ruleLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	objs[0].Move(fyne.NewPos(0, size.Height/2))
	objs[0].Resize(fyne.NewSize(size.Width, 1))
}

func (ruleLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(1, 9) }

// rule is a thin rule between groups in one panel.
func rule() fyne.CanvasObject { return container.New(ruleLayout{}, filled(colHairline)) }

// elideMiddle keeps both ends of a long name: "very_long…name.dcm".
func elideMiddle(s string, maxRunes int) string {
	r := []rune(s)
	if len(r) <= maxRunes || maxRunes < 5 {
		return s
	}
	head := (maxRunes - 1) / 2
	tail := maxRunes - 1 - head
	return string(r[:head]) + "…" + string(r[len(r)-tail:])
}

// unfocus clears keyboard focus from o's canvas, so window-level keys (Esc,
// arrows) keep working after a click on a focusable control.
func unfocus(o fyne.CanvasObject) {
	if c := fyne.CurrentApp().Driver().CanvasForObject(o); c != nil {
		c.Unfocus()
	}
}

// tapArea makes any content tappable; it takes keyboard focus (Space or
// Enter taps) and rings its content while it has it.
type tapArea struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
	focused bool
}

func newTapArea(content fyne.CanvasObject, onTap func()) *tapArea {
	t := &tapArea{content: content, onTap: onTap}
	t.ExtendBaseWidget(t)
	return t
}

func (t *tapArea) Tapped(*fyne.PointEvent) {
	if t.onTap != nil {
		t.onTap()
	}
}

func (t *tapArea) MouseDown(*desktop.MouseEvent) { unfocus(t) }
func (t *tapArea) MouseUp(*desktop.MouseEvent)   {}
func (t *tapArea) Cursor() desktop.Cursor        { return desktop.PointerCursor }

func (t *tapArea) FocusGained()   { t.focused = true; t.Refresh() }
func (t *tapArea) FocusLost()     { t.focused = false; t.Refresh() }
func (t *tapArea) TypedRune(rune) {}
func (t *tapArea) TypedKey(e *fyne.KeyEvent) {
	if isActivate(e) {
		t.Tapped(nil)
		return
	}
	passKey(t, e)
}

func (t *tapArea) CreateRenderer() fyne.WidgetRenderer {
	return &tapAreaRenderer{t: t, ring: newRing(4)}
}

type tapAreaRenderer struct {
	t    *tapArea
	ring *canvas.Rectangle
}

func (r *tapAreaRenderer) MinSize() fyne.Size { return r.t.content.MinSize() }
func (r *tapAreaRenderer) Layout(size fyne.Size) {
	r.t.content.Move(fyne.NewPos(0, 0))
	r.t.content.Resize(size)
	ringRect(r.ring, size)
}
func (r *tapAreaRenderer) Refresh() {
	showWhen(r.ring, r.t.focused)
	r.Layout(r.t.Size())
	canvas.Refresh(r.ring)
}
func (r *tapAreaRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.t.content, r.ring}
}
func (r *tapAreaRenderer) Destroy() {}

// chevronLayout draws a small chevron: down when open, right when closed.
type chevronLayout struct{ open bool }

func (l chevronLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	c := fyne.NewPos(size.Width/2, size.Height/2)
	a, b, z := fyne.NewPos(c.X-3, c.Y-2), fyne.NewPos(c.X, c.Y+2), fyne.NewPos(c.X+3, c.Y-2)
	if !l.open {
		a, b, z = fyne.NewPos(c.X-2, c.Y-3), fyne.NewPos(c.X+2, c.Y), fyne.NewPos(c.X-2, c.Y+3)
	}
	objs[0].(*canvas.Line).Position1, objs[0].(*canvas.Line).Position2 = a, b
	objs[1].(*canvas.Line).Position1, objs[1].(*canvas.Line).Position2 = b, z
}

func (l chevronLayout) MinSize([]fyne.CanvasObject) fyne.Size { return fyne.NewSize(10, 10) }

func chevron(open bool) fyne.CanvasObject {
	return container.New(chevronLayout{open}, line(colText, 1.6), line(colText, 1.6))
}

// wrappedText is regular-weight text that breaks into lines at the width it
// is given; its height follows.
type wrappedText struct {
	widget.BaseWidget
	text   string
	size   float32
	color  color.Color
	centre bool
}

func newWrapped(s string, size float32, c color.Color) *wrappedText {
	w := &wrappedText{text: s, size: size, color: c}
	w.ExtendBaseWidget(w)
	return w
}

func (w *wrappedText) CreateRenderer() fyne.WidgetRenderer {
	return &wrappedRenderer{w: w}
}

type wrappedRenderer struct {
	w     *wrappedText
	lines []*canvas.Text
}

// wrapWords breaks s greedily into lines no wider than width.
func wrapWords(s string, size, width float32) []string {
	words := strings.Fields(s)
	if len(words) == 0 {
		return []string{""}
	}
	var lines []string
	cur := ""
	for _, word := range words {
		if cur != "" && textWidth(cur+" "+word, size) <= width {
			cur += " " + word
			continue
		}
		if cur != "" {
			lines = append(lines, cur)
		}
		// A word wider than the line is broken where it must be.
		for width > 8 && textWidth(word, size) > width {
			cut := breakAt(word, size, width)
			lines = append(lines, word[:cut])
			word = word[cut:]
		}
		cur = word
	}
	return append(lines, cur)
}

// breakAt is the byte length of the longest prefix of s that fits width (at
// least one character).
func breakAt(s string, size, width float32) int {
	cut := 0
	for i, r := range s {
		end := i + len(string(r))
		if textWidth(s[:end], size) > width && cut > 0 {
			break
		}
		cut = end
	}
	return cut
}

func (r *wrappedRenderer) wrap(width float32) []string {
	if width <= 1 {
		return []string{r.w.text}
	}
	return wrapWords(r.w.text, r.w.size, width)
}

func (r *wrappedRenderer) lineHeight() float32 { return textHeight(r.w.size) + 2 }

func (r *wrappedRenderer) MinSize() fyne.Size {
	return fyne.NewSize(40, r.lineHeight()*float32(len(r.wrap(r.w.Size().Width))))
}

func (r *wrappedRenderer) Layout(size fyne.Size) {
	lines := r.wrap(size.Width)
	for len(r.lines) < len(lines) {
		r.lines = append(r.lines, canvas.NewText("", r.w.color))
	}
	lh := r.lineHeight()
	for i, t := range r.lines {
		if i >= len(lines) {
			t.Hide()
			continue
		}
		t.Show()
		t.Text, t.TextSize, t.Color = lines[i], r.w.size, r.w.color
		ts := fyne.MeasureText(lines[i], r.w.size, fyne.TextStyle{})
		x := float32(0)
		if r.w.centre {
			x = (size.Width - ts.Width) / 2
		}
		t.Move(fyne.NewPos(x, float32(i)*lh+(lh-ts.Height)/2))
		t.Resize(ts)
	}
}

func (r *wrappedRenderer) Refresh() {
	r.Layout(r.w.Size())
	for _, t := range r.lines {
		canvas.Refresh(t)
	}
}

func (r *wrappedRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, len(r.lines))
	for i, t := range r.lines {
		objs[i] = t
	}
	return objs
}

func (r *wrappedRenderer) Destroy() {}
