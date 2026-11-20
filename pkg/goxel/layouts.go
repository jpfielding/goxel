package goxel

import (
	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// Layouts with explicit gaps. Fyne's own boxes and borders pad with the theme
// padding, which leaves gaps between panels and rows that roxel does not
// have; roxel's panels meet edge to edge and its rows sit 6-8 px apart.

// stackLayout places children top to bottom at the full width, each at its
// own minimum height, gap apart.
type stackLayout struct{ gap float32 }

func (l stackLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	y := float32(0)
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		h := o.MinSize().Height
		o.Move(fyne.NewPos(0, y))
		o.Resize(fyne.NewSize(size.Width, h))
		y += h + l.gap
	}
}

func (l stackLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	n := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		m.Width = max(m.Width, ms.Width)
		m.Height += ms.Height
		n++
	}
	if n > 1 {
		m.Height += l.gap * float32(n-1)
	}
	return m
}

// vstack stacks objects vertically, gap apart.
func vstack(gap float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(stackLayout{gap}, objs...)
}

// rowLayout places children left to right at their own minimum widths, gap
// apart, each centred on the row's height.
type rowLayout struct{ gap float32 }

func (l rowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	x := float32(0)
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		o.Move(fyne.NewPos(x, (size.Height-ms.Height)/2))
		o.Resize(ms)
		x += ms.Width + l.gap
	}
}

func (l rowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	n := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		m.Width += ms.Width
		m.Height = max(m.Height, ms.Height)
		n++
	}
	if n > 1 {
		m.Width += l.gap * float32(n-1)
	}
	return m
}

// hrow lays objects out in a row, each at its own width, centred vertically.
func hrow(gap float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(rowLayout{gap}, objs...)
}

// flexRowLayout is a row where one child (flex) takes whatever width the
// others leave; the others keep their minimum widths. Children fill the
// row's height.
type flexRowLayout struct {
	gap  float32
	flex int
}

func (l flexRowLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	fixed := float32(0)
	n := 0
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		n++
		if i != l.flex {
			fixed += o.MinSize().Width
		}
	}
	room := max(size.Width-fixed-l.gap*float32(max(n-1, 0)), 0)
	x := float32(0)
	for i, o := range objs {
		if !o.Visible() {
			continue
		}
		w := o.MinSize().Width
		if i == l.flex {
			w = room
		}
		o.Move(fyne.NewPos(x, 0))
		o.Resize(fyne.NewSize(w, size.Height))
		x += w + l.gap
	}
}

func (l flexRowLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	n := 0
	for _, o := range objs {
		if !o.Visible() {
			continue
		}
		ms := o.MinSize()
		m.Width += ms.Width
		m.Height = max(m.Height, ms.Height)
		n++
	}
	if n > 1 {
		m.Width += l.gap * float32(n-1)
	}
	return m
}

// flexRow is a row in which objs[flex] stretches.
func flexRow(gap float32, flex int, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(flexRowLayout{gap, flex}, objs...)
}

// edgeLayout is a border layout without padding: objects are [top, bottom,
// left, right, centre] with nil for an absent edge.
type edgeLayout struct{}

func (edgeLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	top, bottom, left, right, centre := objs[0], objs[1], objs[2], objs[3], objs[4]
	var topH, bottomH, leftW, rightW float32
	if top != nil && top.Visible() {
		topH = top.MinSize().Height
		top.Move(fyne.NewPos(0, 0))
		top.Resize(fyne.NewSize(size.Width, topH))
	}
	if bottom != nil && bottom.Visible() {
		bottomH = bottom.MinSize().Height
		bottom.Move(fyne.NewPos(0, size.Height-bottomH))
		bottom.Resize(fyne.NewSize(size.Width, bottomH))
	}
	midH := max(size.Height-topH-bottomH, 0)
	if left != nil && left.Visible() {
		leftW = left.MinSize().Width
		left.Move(fyne.NewPos(0, topH))
		left.Resize(fyne.NewSize(leftW, midH))
	}
	if right != nil && right.Visible() {
		rightW = right.MinSize().Width
		right.Move(fyne.NewPos(size.Width-rightW, topH))
		right.Resize(fyne.NewSize(rightW, midH))
	}
	if centre != nil && centre.Visible() {
		centre.Move(fyne.NewPos(leftW, topH))
		centre.Resize(fyne.NewSize(max(size.Width-leftW-rightW, 0), midH))
	}
}

func (edgeLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	top, bottom, left, right, centre := objs[0], objs[1], objs[2], objs[3], objs[4]
	size := func(o fyne.CanvasObject) fyne.Size {
		if o == nil || !o.Visible() {
			return fyne.Size{}
		}
		return o.MinSize()
	}
	t, b, l, r, c := size(top), size(bottom), size(left), size(right), size(centre)
	w := max(c.Width+l.Width+r.Width, t.Width, b.Width)
	h := max(c.Height, l.Height, r.Height) + t.Height + b.Height
	return fyne.NewSize(w, h)
}

// border puts centre in the middle and the others on the edges, no padding.
// Any may be nil.
func border(top, bottom, left, right, centre fyne.CanvasObject) *fyne.Container {
	// The layout is given the slots by position; absent ones are nil. A
	// container drops nils from its objects, so keep the slots in a wrapper.
	slots := []fyne.CanvasObject{top, bottom, left, right, centre}
	c := container.New(&slotLayout{slots: slots})
	for _, o := range slots {
		if o != nil {
			c.Objects = append(c.Objects, o)
		}
	}
	return c
}

// slotLayout is edgeLayout with the slots remembered, so nil edges stay nil.
type slotLayout struct{ slots []fyne.CanvasObject }

func (l *slotLayout) Layout(_ []fyne.CanvasObject, size fyne.Size) {
	edgeLayout{}.Layout(l.slots, size)
}

func (l *slotLayout) MinSize(_ []fyne.CanvasObject) fyne.Size {
	return edgeLayout{}.MinSize(l.slots)
}

// columnsLayout divides the width into n equal columns, gap apart.
type columnsLayout struct{ gap float32 }

func (l columnsLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	n := float32(len(objs))
	if n == 0 {
		return
	}
	w := (size.Width - l.gap*(n-1)) / n
	for i, o := range objs {
		o.Move(fyne.NewPos(float32(i)*(w+l.gap), 0))
		o.Resize(fyne.NewSize(max(w, 0), size.Height))
	}
}

func (l columnsLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		ms := o.MinSize()
		m.Width = max(m.Width, ms.Width)
		m.Height = max(m.Height, ms.Height)
	}
	n := float32(len(objs))
	m.Width = m.Width*n + l.gap*max(n-1, 0)
	return m
}

// columns places objects in n equal-width columns, gap apart.
func columns(gap float32, objs ...fyne.CanvasObject) *fyne.Container {
	return container.New(columnsLayout{gap}, objs...)
}

// leftCenterLayout puts its child at the left, at its own size, centred
// vertically: a label in a fixed-height row.
type leftCenterLayout struct{}

func (leftCenterLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		ms := o.MinSize()
		o.Move(fyne.NewPos(0, (size.Height-ms.Height)/2))
		o.Resize(ms)
	}
}

func (leftCenterLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	var m fyne.Size
	for _, o := range objs {
		m = m.Max(o.MinSize())
	}
	return m
}

// rightCenterLayout is leftCenterLayout against the right edge.
type rightCenterLayout struct{}

func (rightCenterLayout) Layout(objs []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objs {
		ms := o.MinSize()
		o.Move(fyne.NewPos(size.Width-ms.Width, (size.Height-ms.Height)/2))
		o.Resize(ms)
	}
}

func (rightCenterLayout) MinSize(objs []fyne.CanvasObject) fyne.Size {
	return leftCenterLayout{}.MinSize(objs)
}

func leftCentre(o fyne.CanvasObject) *fyne.Container  { return container.New(leftCenterLayout{}, o) }
func rightCentre(o fyne.CanvasObject) *fyne.Container { return container.New(rightCenterLayout{}, o) }

// natural keeps a control at its own width on desktop, where roxel sizes a
// segmented strip to its labels; on touch the control stretches across its row.
func natural(o fyne.CanvasObject) fyne.CanvasObject {
	if uiTouch {
		return o
	}
	return hrow(0, o)
}
