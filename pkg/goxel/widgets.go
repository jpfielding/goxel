package goxel

import (
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
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
	size, noteSize := float32(14.5), float32(12)
	if uiTouch {
		size, noteSize = 16, 13
	}
	h := leftCentre(text(title, size, colHeading))
	var row fyne.CanvasObject = h
	if note != "" {
		row = border(nil, nil, nil, rightCentre(text(note, noteSize, colMuted)), h)
	}
	return minHeight(20, row)
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
	h, col, size, keySize := float32(22), float32(68), bodySize(), float32(13)
	if uiTouch {
		h, col, keySize = 28, 84, 14
	}
	return minHeight(h, flexRow(0, 1, fixedWidth(col, leftCentre(text(key, keySize, colMuted))),
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

// tapArea makes any content tappable.
type tapArea struct {
	widget.BaseWidget
	content fyne.CanvasObject
	onTap   func()
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

func (t *tapArea) CreateRenderer() fyne.WidgetRenderer {
	return widget.NewSimpleRenderer(t.content)
}

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
	cur := words[0]
	for _, word := range words[1:] {
		if textWidth(cur+" "+word, size) <= width {
			cur += " " + word
			continue
		}
		lines = append(lines, cur)
		cur = word
	}
	return append(lines, cur)
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
