package goxel

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// listRow is one selectable row of a list, as roxel draws a finding: a colour
// swatch, the name, and the confidence at the right. Selection is a fill and
// an accent outline as well as a colour. It takes keyboard focus.
type listRow struct {
	widget.BaseWidget
	name       string
	confidence string // "95%", or empty
	swatch     color.Color
	selected   bool
	onTap      func()

	hovered, focused bool
}

func newListRow(name, confidence string, swatch color.Color, selected bool, onTap func()) *listRow {
	r := &listRow{name: name, confidence: confidence, swatch: swatch, selected: selected, onTap: onTap}
	r.ExtendBaseWidget(r)
	return r
}

func (r *listRow) Tapped(*fyne.PointEvent) {
	if r.onTap != nil {
		r.onTap()
	}
}

func (r *listRow) MouseIn(*desktop.MouseEvent)    { r.hovered = true; r.Refresh() }
func (r *listRow) MouseMoved(*desktop.MouseEvent) {}
func (r *listRow) MouseOut()                      { r.hovered = false; r.Refresh() }
func (r *listRow) MouseDown(*desktop.MouseEvent)  { unfocus(r) }
func (r *listRow) MouseUp(*desktop.MouseEvent)    {}
func (r *listRow) Cursor() desktop.Cursor         { return desktop.PointerCursor }

func (r *listRow) FocusGained()   { r.focused = true; r.Refresh() }
func (r *listRow) FocusLost()     { r.focused = false; r.Refresh() }
func (r *listRow) TypedRune(rune) {}
func (r *listRow) TypedKey(e *fyne.KeyEvent) {
	if isActivate(e) {
		r.Tapped(nil)
		return
	}
	passKey(r, e)
}

func (r *listRow) CreateRenderer() fyne.WidgetRenderer {
	return &listRowRenderer{r: r, bg: rrect(color.Transparent, color.Transparent, 5, 1),
		swatch: rrect(r.swatch, color.Transparent, 2, 0), name: canvas.NewText("", colText),
		conf: canvas.NewText("", colMuted), ring: newRing(5)}
}

type listRowRenderer struct {
	r          *listRow
	bg, swatch *canvas.Rectangle
	ring       *canvas.Rectangle
	name, conf *canvas.Text
}

func (lr *listRowRenderer) MinSize() fyne.Size { return fyne.NewSize(80, max(28, controlHeight())) }

func (lr *listRowRenderer) Layout(size fyne.Size) {
	lr.bg.Resize(size)
	ringRect(lr.ring, size)
	lr.swatch.Move(fyne.NewPos(9, (size.Height-10)/2))
	lr.swatch.Resize(fyne.NewSize(10, 10))
	right := size.Width - 8
	lr.conf.Text, lr.conf.TextSize = lr.r.confidence, bodySize()
	if lr.r.confidence != "" {
		ts := fyne.MeasureText(lr.r.confidence, bodySize(), fyne.TextStyle{})
		lr.conf.Move(fyne.NewPos(right-ts.Width, (size.Height-ts.Height)/2))
		lr.conf.Resize(ts)
		right -= ts.Width + 8
	}
	left := float32(28)
	lr.name.TextSize = bodySize()
	lr.name.Text = elideToWidth(lr.r.name, bodySize(), right-left)
	ts := fyne.MeasureText(lr.name.Text, bodySize(), fyne.TextStyle{})
	lr.name.Move(fyne.NewPos(left, (size.Height-ts.Height)/2))
	lr.name.Resize(ts)
}

func (lr *listRowRenderer) Refresh() {
	r := lr.r
	fill, stroke := color.Color(color.Transparent), color.Color(color.Transparent)
	fg := color.NRGBA(colText)
	switch {
	case r.selected:
		fill, stroke, fg = colSelected, colAccent, colHeading
	case r.hovered:
		fill = colHover
	}
	lr.bg.FillColor, lr.bg.StrokeColor = fill, stroke
	lr.bg.StrokeWidth = 1
	if r.selected {
		lr.bg.StrokeWidth = 2 // the selection outline is 2 px, as in roxel
	}
	lr.swatch.FillColor = r.swatch
	lr.name.Color = fg
	lr.conf.Hide()
	if r.confidence != "" {
		lr.conf.Show()
	}
	showWhen(lr.ring, r.focused)
	lr.Layout(r.Size())
	canvas.Refresh(lr.bg)
}

func (lr *listRowRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{lr.bg, lr.swatch, lr.name, lr.conf, lr.ring}
}

func (lr *listRowRenderer) Destroy() {}

// elideToWidth keeps both ends of a long name (the end usually tells two
// apart), cut to fit width, as roxel's elide_middle does.
func elideToWidth(s string, size, width float32) string {
	if textWidth(s, size) <= width {
		return s
	}
	chars := []rune(s)
	build := func(kept int) string {
		head := kept * 2 / 5
		return string(chars[:head]) + "…" + string(chars[len(chars)-(kept-head):])
	}
	lo, hi := 0, len(chars)-1
	best := "…"
	for lo <= hi {
		mid := (lo + hi) / 2
		if c := build(mid); textWidth(c, size) <= width {
			best, lo = c, mid+1
		} else if mid == 0 {
			break
		} else {
			hi = mid - 1
		}
	}
	return best
}

// percent is a confidence as roxel shows it: "95%".
func percent(f float64) string { return fmt.Sprintf("%.0f%%", f*100) }
