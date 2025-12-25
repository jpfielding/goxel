package goxel

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// LabeledSlider wraps a widget.Slider and adds a floating value label above the handle.
type LabeledSlider struct {
	widget.BaseWidget
	Slider *widget.Slider
	Label  *canvas.Text
}

func NewLabeledSlider(min, max float64) *LabeledSlider {
	ls := &LabeledSlider{}
	ls.ExtendBaseWidget(ls)

	ls.Slider = widget.NewSlider(min, max)
	ls.Label = canvas.NewText("", theme.ForegroundColor())
	ls.Label.TextSize = 10
	ls.Label.Alignment = fyne.TextAlignCenter

	// Hook into OnChanged to update layout
	// Note: User must set OnChanged on the wrapper or we proxy it
	// For now, we update label in Layout.
	// But we need to trigger refresh when slider changes.

	// We wrap the slider's OnChanged
	ls.Slider.OnChanged = func(f float64) {
		ls.Refresh()
	}

	return ls
}

func (ls *LabeledSlider) SetValue(v float64) {
	// Use Slider.SetValue to trigger OnChanged events if registered
	ls.Slider.SetValue(v)
	// Refresh wrapper to update label position
	ls.Refresh()
}

func (ls *LabeledSlider) SetOnChanged(f func(float64)) {
	ls.Slider.OnChanged = func(v float64) {
		ls.Refresh() // Update label position
		if f != nil {
			f(v)
		}
	}
}

func (ls *LabeledSlider) CreateRenderer() fyne.WidgetRenderer {
	return &labeledSliderRenderer{
		ls: ls,
	}
}

type labeledSliderRenderer struct {
	ls *LabeledSlider
}

func (r *labeledSliderRenderer) Layout(size fyne.Size) {
	// Position Slider (fill width, standard height)
	sliderHeight := r.ls.Slider.MinSize().Height
	// We want slider at bottom, label above

	// Fyne sliders usually take whole height but draw centered.
	// To align label exactly above handle, we need to know where the track is.
	// Default theme: track is centered vertically.

	// Let's give the slider the bottom part of the widget.
	// And label the top part.

	labelHeight := float32(20)

	sliderY := labelHeight
	sliderH := size.Height - labelHeight
	if sliderH < sliderHeight {
		sliderH = sliderHeight
	}

	r.ls.Slider.Move(fyne.NewPos(0, sliderY))
	r.ls.Slider.Resize(fyne.NewSize(size.Width, sliderH))

	// Calculate Label Position
	min := r.ls.Slider.Min
	max := r.ls.Slider.Max
	val := r.ls.Slider.Value
	width := size.Width

	rangeW := max - min
	if rangeW <= 0 {
		rangeW = 1
	}

	ratio := (val - min) / rangeW
	// Adjustment for padding? Fyne sliders usually have small padding.
	// Let's assume 0..width for now.

	x := float32(ratio) * width

	r.ls.Label.Text = fmt.Sprintf("%.2f", val)
	if val > 10 { // Integer formatting for large numbers (slice index)
		// Heuristic: if step is 1, use integer?
		if r.ls.Slider.Step == 1 || r.ls.Slider.Max > 100 {
			r.ls.Label.Text = fmt.Sprintf("%.0f", val)
		}
	}

	r.ls.Label.Color = theme.ForegroundColor()
	r.ls.Label.Resize(fyne.NewSize(40, labelHeight))
	// Center label on x, but clamp to bounds
	labelX := x - 20
	if labelX < 0 {
		labelX = 0
	} else if labelX+40 > width {
		labelX = width - 40
	}
	r.ls.Label.Move(fyne.NewPos(labelX, 0))
}

func (r *labeledSliderRenderer) MinSize() fyne.Size {
	s := r.ls.Slider.MinSize()
	return fyne.NewSize(s.Width, s.Height+20)
}

func (r *labeledSliderRenderer) Refresh() {
	r.Layout(r.ls.Size())
	canvas.Refresh(r.ls.Label)
	canvas.Refresh(r.ls.Slider)
}

func (r *labeledSliderRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.ls.Slider, r.ls.Label}
}

func (r *labeledSliderRenderer) Destroy() {}
