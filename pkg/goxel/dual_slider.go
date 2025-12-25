package goxel

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// DualSlider is a custom widget with two handles for selecting a range
type DualSlider struct {
	widget.BaseWidget
	Min, Max  float64
	Low, High float64
	Color     color.Color
	OnChanged func(low, high float64)

	draggedHandle int // 0: none, 1: low, 2: high
}

// NewDualSlider creates a new range slider
func NewDualSlider(min, max float64, initialLow, initialHigh float64, c color.Color) *DualSlider {
	s := &DualSlider{
		Min:   min,
		Max:   max,
		Low:   initialLow,
		High:  initialHigh,
		Color: c,
	}
	s.ExtendBaseWidget(s)
	return s
}

func (s *DualSlider) CreateRenderer() fyne.WidgetRenderer {
	track := canvas.NewRectangle(theme.DisabledButtonColor())
	active := canvas.NewRectangle(s.Color)
	handleLow := canvas.NewCircle(theme.BackgroundColor())
	handleLow.StrokeColor = theme.ForegroundColor()
	handleLow.StrokeWidth = 2
	handleHigh := canvas.NewCircle(theme.BackgroundColor())
	handleHigh.StrokeColor = theme.ForegroundColor()
	handleHigh.StrokeWidth = 2

	return &dualSliderRenderer{
		s:          s,
		track:      track,
		active:     active,
		handleLow:  handleLow,
		handleHigh: handleHigh,
	}
}

// Mouse interaction
func (s *DualSlider) Tapped(e *fyne.PointEvent) {
	// Move closest handle to tap
	val := s.valueFromPos(e.Position.X, s.Size().Width)

	distLow := abs(val - s.Low)
	distHigh := abs(val - s.High)

	if distLow < distHigh {
		s.Low = val
	} else {
		s.High = val
	}
	s.clampValues()
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(s.Low, s.High)
	}
}

// We rely on Mouseable for handle locking
func (s *DualSlider) MouseDown(e *desktop.MouseEvent) {
	val := s.valueFromPos(e.Position.X, s.Size().Width)
	distLow := abs(val - s.Low)
	distHigh := abs(val - s.High)

	// Threshold for grabbing (in value space, approximated)
	// Just pick closest
	if distLow < distHigh {
		s.draggedHandle = 1
		s.Low = val // Jump to click
	} else {
		s.draggedHandle = 2
		s.High = val // Jump to click
	}
	s.clampValues()
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(s.Low, s.High)
	}
}

func (s *DualSlider) MouseUp(e *desktop.MouseEvent) {
	s.draggedHandle = 0
}

func (s *DualSlider) MouseMoved(e *desktop.MouseEvent) {
	// Visual hover effect if we wanted
}

func (s *DualSlider) DragEnd() {
	s.draggedHandle = 0
}

// Implementation of Draggable
func (s *DualSlider) Dragged(e *fyne.DragEvent) {
	width := s.Size().Width
	// Normalize delta
	dVal := (float64(e.Dragged.DX) / float64(width)) * (s.Max - s.Min)

	switch s.draggedHandle {
	case 1:
		s.Low += dVal
	case 2:
		s.High += dVal
	default:
		return
	}

	s.clampValues()
	s.Refresh()
	if s.OnChanged != nil {
		s.OnChanged(s.Low, s.High)
	}
}

func (s *DualSlider) clampValues() {
	if s.Low < s.Min {
		s.Low = s.Min
	}
	if s.Low > s.Max {
		s.Low = s.Max
	}
	if s.High < s.Min {
		s.High = s.Min
	}
	if s.High > s.Max {
		s.High = s.Max
	}
	if s.Low > s.High {
		if s.draggedHandle == 1 {
			s.Low = s.High
		} else {
			s.High = s.Low
		}
	}
}

func (s *DualSlider) valueFromPos(x float32, width float32) float64 {
	if width <= 0 {
		return s.Min
	}
	pct := float64(x / width)
	return s.Min + pct*(s.Max-s.Min)
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

type dualSliderRenderer struct {
	s          *DualSlider
	track      *canvas.Rectangle
	active     *canvas.Rectangle
	handleLow  *canvas.Circle
	handleHigh *canvas.Circle
}

func (r *dualSliderRenderer) Layout(size fyne.Size) {
	trackHeight := float32(4)
	handleSize := float32(16)

	// Track centered vertically
	r.track.Move(fyne.NewPos(0, (size.Height-trackHeight)/2))
	r.track.Resize(fyne.NewSize(size.Width, trackHeight))

	rangeWidth := r.s.Max - r.s.Min
	if rangeWidth <= 0 {
		rangeWidth = 1
	}

	// Calc positions
	xLow := float32((r.s.Low-r.s.Min)/rangeWidth) * size.Width
	xHigh := float32((r.s.High-r.s.Min)/rangeWidth) * size.Width

	// Active region
	r.active.Move(fyne.NewPos(xLow, (size.Height-trackHeight)/2))
	r.active.Resize(fyne.NewSize(xHigh-xLow, trackHeight))

	// Handles
	r.handleLow.Resize(fyne.NewSize(handleSize, handleSize))
	r.handleLow.Move(fyne.NewPos(xLow-handleSize/2, (size.Height-handleSize)/2))

	r.handleHigh.Resize(fyne.NewSize(handleSize, handleSize))
	r.handleHigh.Move(fyne.NewPos(xHigh-handleSize/2, (size.Height-handleSize)/2))
}

func (r *dualSliderRenderer) MinSize() fyne.Size {
	return fyne.NewSize(100, 24)
}

func (r *dualSliderRenderer) Refresh() {
	// Colors
	r.active.FillColor = r.s.Color
	r.handleLow.FillColor = theme.BackgroundColor()
	r.handleHigh.FillColor = theme.BackgroundColor()

	r.Layout(r.s.Size())
	canvas.Refresh(r.s)
}

func (r *dualSliderRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.track, r.active, r.handleLow, r.handleHigh}
}

func (r *dualSliderRenderer) Destroy() {}
