package goxel

import (
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// touchTarget is the compact layouts' minimum control height.
const touchTarget = 44

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

func text(s string, size float32, c color.Color, bold bool) *canvas.Text {
	t := canvas.NewText(s, c)
	t.TextSize = size
	t.TextStyle.Bold = bold
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

// markImage is the hexagon mark at the given size.
func markImage(size float32) fyne.CanvasObject {
	img := canvas.NewImageFromResource(markResource)
	img.FillMode = canvas.ImageFillContain
	img.SetMinSize(fyne.NewSize(size, size))
	return img
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

// segmented is a row of equal buttons with one selected, like roxel's
// segmented control. The selected segment uses SuccessImportance, which the
// theme paints in SELECTED.
type segmented struct {
	buttons  []*widget.Button
	selected int
	onSelect func(int)
	box      *fyne.Container
}

func newSegmented(labels []string, selected int, onSelect func(int)) *segmented {
	s := &segmented{selected: selected, onSelect: onSelect}
	objs := make([]fyne.CanvasObject, len(labels))
	for i, l := range labels {
		b := widget.NewButton(l, func() {
			s.Select(i)
			if s.onSelect != nil {
				s.onSelect(i)
			}
		})
		s.buttons = append(s.buttons, b)
		objs[i] = b
	}
	s.box = container.NewGridWithColumns(len(labels), objs...)
	s.paint()
	return s
}

// Select changes the highlighted segment without calling onSelect.
func (s *segmented) Select(i int) {
	s.selected = i
	s.paint()
}

func (s *segmented) paint() {
	for i, b := range s.buttons {
		imp := widget.MediumImportance
		if i == s.selected {
			imp = widget.SuccessImportance
		}
		if b.Importance != imp {
			b.Importance = imp
			b.Refresh()
		}
	}
}

// sliderRow is a labelled slider with a value readout; compact layouts add
// -/+ steppers and touch height.
type sliderRow struct {
	obj    fyne.CanvasObject
	slider *widget.Slider
	value  *widget.Label
	format func(float64) string
}

// SetValue moves the slider and readout without calling the callbacks' commit.
func (r *sliderRow) SetValue(v float64) {
	r.slider.Value = v
	r.slider.Refresh()
	r.value.SetText(r.format(v))
}

// newSliderRow builds a row. onChanged runs on every move; onCommitted when a
// drag ends or a stepper is tapped (nil means onChanged only).
func newSliderRow(compact bool, label string, lo, hi, step float64, format func(float64) string,
	value float64, onChanged, onCommitted func(float64)) *sliderRow {
	r := &sliderRow{format: format}
	r.slider = widget.NewSlider(lo, hi)
	r.slider.Step = step
	r.slider.Value = math.Max(lo, math.Min(hi, value))
	r.value = widget.NewLabel(format(r.slider.Value))
	r.value.Alignment = fyne.TextAlignTrailing
	r.slider.OnChanged = func(v float64) {
		r.value.SetText(format(v))
		if onChanged != nil {
			onChanged(v)
		}
	}
	r.slider.OnChangeEnded = func(v float64) {
		// Hand keys back to the window (Esc, arrow-key slice stepping).
		unfocus(r.slider)
		if onCommitted != nil {
			onCommitted(v)
		}
	}
	name := widget.NewLabel(label)
	name.Truncation = fyne.TextTruncateEllipsis
	if !compact {
		r.obj = container.NewBorder(nil, nil, fixedWidth(76, name), fixedWidth(60, r.value), r.slider)
		return r
	}
	stepBy := step
	if stepBy <= 0 {
		stepBy = (hi - lo) / 20
	}
	if (hi-lo)/stepBy > 40 {
		stepBy = (hi - lo) / 40
	}
	nudge := func(d float64) {
		v := math.Max(lo, math.Min(hi, r.slider.Value+d))
		r.slider.SetValue(v) // fires OnChanged
		if onCommitted != nil {
			onCommitted(v)
		}
	}
	minus := widget.NewButton("-", func() { nudge(-stepBy) })
	plus := widget.NewButton("+", func() { nudge(stepBy) })
	r.value.Alignment = fyne.TextAlignLeading
	left := fixedWidth(96, container.NewVBox(layout.NewSpacer(), name, layout.NewSpacer()))
	right := container.NewHBox(fixedWidth(touchTarget, minus), r.value, fixedWidth(touchTarget, plus))
	r.obj = minHeight(touchTarget, container.NewBorder(nil, nil, left, right, r.slider))
	return r
}

// sectionTitle is a group heading with an optional muted note after it.
func sectionTitle(title, note string) fyne.CanvasObject {
	h := text(title, 14, colHeading, true)
	if note == "" {
		return inset(0, 4, h)
	}
	return inset(0, 4, container.NewHBox(h, text(note, 12, colMuted, false)))
}

// keyValue is a muted key and a value, as in roxel's Details section.
func keyValue(key, value string) fyne.CanvasObject {
	v := widget.NewLabel(value)
	v.Alignment = fyne.TextAlignTrailing
	v.Truncation = fyne.TextTruncateEllipsis
	k := text(key, 13, colMuted, false)
	return container.NewBorder(nil, nil, container.NewVBox(layout.NewSpacer(), k, layout.NewSpacer()), nil, v)
}

// note is wrapped muted body text.
func note(s string) *widget.Label {
	l := widget.NewLabel(s)
	l.Wrapping = fyne.TextWrapWord
	l.Importance = widget.LowImportance
	return l
}

func hairline() fyne.CanvasObject {
	r := filled(colHairline)
	r.SetMinSize(fyne.NewSize(1, 1))
	return r
}

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

// newCheck is a checkbox that gives keyboard focus back after each toggle.
func newCheck(label string, checked bool, onChanged func(bool)) *widget.Check {
	var c *widget.Check
	c = widget.NewCheck(label, func(b bool) {
		onChanged(b)
		unfocus(c)
	})
	c.Checked = checked
	return c
}
