package goxel

import (
	"fmt"
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/jpfielding/goxel/pkg/volume"
)

// MultiRangeSlider is a custom widget with N colored sections and N-1 movable handles.
// Supports variable number of color bands from config.
type MultiRangeSlider struct {
	widget.BaseWidget
	Min, Max float64

	// Bands defines the color segments. Thresholds are the upper bounds.
	// For N bands, there are N-1 movable handles (thresholds between bands).
	Bands []volume.ColorBand

	// OnChanged is called when any handle moves. Returns all current thresholds.
	OnChanged func(thresholds []int)

	// OnOpacityChanged is called when a band's opacity is changed.
	// bandIdx is the index of the band, alpha is the new multiplier (0.0-2.0).
	OnOpacityChanged func(bandIdx int, alpha float64)

	draggedHandle int // 0-indexed handle being dragged, -1 = none
}

// NewMultiRangeSlider creates a slider from color bands.
func NewMultiRangeSlider(min, max float64, bands []volume.ColorBand) *MultiRangeSlider {
	if len(bands) == 0 {
		bands = volume.DefaultColorBands()
	}
	s := &MultiRangeSlider{
		Min:           min,
		Max:           max,
		Bands:         bands,
		draggedHandle: -1,
	}
	s.ExtendBaseWidget(s)
	return s
}

// NewMultiRangeSliderFromConfig creates a slider using bands from the config.
func NewMultiRangeSliderFromConfig(cfg *volume.RenderConfig, mapName string) *MultiRangeSlider {
	bands := volume.GetColorBandsFromConfig(cfg, mapName)
	maxDensity := 30000.0
	if len(bands) > 0 {
		maxDensity = float64(bands[len(bands)-1].Threshold)
	}
	return NewMultiRangeSlider(0, maxDensity, bands)
}

// GetThresholds returns all current threshold values (N-1 values for N bands).
func (s *MultiRangeSlider) GetThresholds() []int {
	if len(s.Bands) <= 1 {
		return nil
	}
	thresholds := make([]int, len(s.Bands)-1)
	for i := 0; i < len(s.Bands)-1; i++ {
		thresholds[i] = s.Bands[i].Threshold
	}
	return thresholds
}

// SetThresholds updates the threshold values.
func (s *MultiRangeSlider) SetThresholds(thresholds []int) {
	for i := 0; i < len(thresholds) && i < len(s.Bands)-1; i++ {
		s.Bands[i].Threshold = thresholds[i]
	}
	s.clampValues()
	s.Refresh()
}

// GetBandAlpha returns the alpha multiplier for a specific band.
func (s *MultiRangeSlider) GetBandAlpha(bandIdx int) float64 {
	if bandIdx < 0 || bandIdx >= len(s.Bands) {
		return 1.0
	}
	if s.Bands[bandIdx].Alpha == 0 {
		return 1.0 // Default to 1.0 if not set
	}
	return s.Bands[bandIdx].Alpha
}

// SetBandAlpha sets the alpha multiplier for a specific band and triggers callback.
func (s *MultiRangeSlider) SetBandAlpha(bandIdx int, alpha float64) {
	if bandIdx < 0 || bandIdx >= len(s.Bands) {
		return
	}
	s.Bands[bandIdx].Alpha = alpha
	if s.OnOpacityChanged != nil {
		s.OnOpacityChanged(bandIdx, alpha)
	}
}

func (s *MultiRangeSlider) CreateRenderer() fyne.WidgetRenderer {
	r := &multiRangeSliderRenderer{
		s:      s,
		tracks: make([]*canvas.Rectangle, len(s.Bands)),
		labels: make([]*canvas.Text, len(s.Bands)),
		titles: make([]*canvas.Text, len(s.Bands)),
	}

	// Create track segments
	for i, band := range s.Bands {
		r.tracks[i] = canvas.NewRectangle(band.Color)
	}

	// Create handles (N-1 for N bands)
	r.handles = make([]*canvas.Circle, len(s.Bands)-1)
	r.handleLabels = make([]*canvas.Text, len(s.Bands)-1)
	for i := range r.handles {
		h := canvas.NewCircle(theme.BackgroundColor())
		h.StrokeColor = theme.ForegroundColor()
		h.StrokeWidth = 2
		r.handles[i] = h

		lbl := canvas.NewText("", theme.ForegroundColor())
		lbl.TextSize = 10
		lbl.Alignment = fyne.TextAlignCenter
		r.handleLabels[i] = lbl
	}

	// Create end dots
	if len(s.Bands) > 0 {
		r.dotStart = canvas.NewCircle(s.Bands[0].Color)
		r.dotEnd = canvas.NewCircle(s.Bands[len(s.Bands)-1].Color)
	} else {
		r.dotStart = canvas.NewCircle(color.Gray{128})
		r.dotEnd = canvas.NewCircle(color.Gray{128})
	}

	// Create titles for each band
	for i, band := range s.Bands {
		t := canvas.NewText(band.Name, theme.ForegroundColor())
		t.TextStyle = fyne.TextStyle{Bold: true}
		t.Alignment = fyne.TextAlignCenter
		t.TextSize = 11
		r.titles[i] = t
	}

	return r
}

// Mouse Interactions

func (s *MultiRangeSlider) Tapped(e *fyne.PointEvent) {
	if len(s.Bands) <= 1 {
		return
	}
	val := s.valueFromPos(e.Position.X, s.Size().Width)

	// Find nearest handle
	nearest := s.findNearestHandle(val)
	if nearest >= 0 {
		s.Bands[nearest].Threshold = int(val)
		s.clampValues()
		s.Refresh()
		s.notifyChange()
	}
}

func (s *MultiRangeSlider) MouseDown(e *desktop.MouseEvent) {
	if len(s.Bands) <= 1 {
		return
	}
	val := s.valueFromPos(e.Position.X, s.Size().Width)
	s.draggedHandle = s.findNearestHandle(val)
	if s.draggedHandle >= 0 {
		s.Bands[s.draggedHandle].Threshold = int(val)
		s.clampValues()
		s.Refresh()
		s.notifyChange()
	}
}

func (s *MultiRangeSlider) MouseUp(e *desktop.MouseEvent) {
	s.draggedHandle = -1
}

func (s *MultiRangeSlider) MouseMoved(e *desktop.MouseEvent) {}

func (s *MultiRangeSlider) DragEnd() {
	s.draggedHandle = -1
}

func (s *MultiRangeSlider) Dragged(e *fyne.DragEvent) {
	if s.draggedHandle < 0 || s.draggedHandle >= len(s.Bands)-1 {
		return
	}

	width := s.Size().Width
	dVal := (float64(e.Dragged.DX) / float64(width)) * (s.Max - s.Min)
	s.Bands[s.draggedHandle].Threshold += int(dVal)

	s.clampValues()
	s.Refresh()
	s.notifyChange()
}

func (s *MultiRangeSlider) findNearestHandle(val float64) int {
	if len(s.Bands) <= 1 {
		return -1
	}

	nearest := 0
	minDist := abs(val - float64(s.Bands[0].Threshold))

	for i := 1; i < len(s.Bands)-1; i++ {
		dist := abs(val - float64(s.Bands[i].Threshold))
		if dist < minDist {
			minDist = dist
			nearest = i
		}
	}
	return nearest
}

func (s *MultiRangeSlider) clampValues() {
	for i := range s.Bands {
		if s.Bands[i].Threshold < int(s.Min) {
			s.Bands[i].Threshold = int(s.Min)
		}
		if s.Bands[i].Threshold > int(s.Max) {
			s.Bands[i].Threshold = int(s.Max)
		}
	}

	// Ensure ordering (each threshold >= previous)
	for i := 1; i < len(s.Bands)-1; i++ {
		if s.Bands[i].Threshold < s.Bands[i-1].Threshold {
			if s.draggedHandle == i {
				s.Bands[i].Threshold = s.Bands[i-1].Threshold
			} else {
				s.Bands[i-1].Threshold = s.Bands[i].Threshold
			}
		}
	}
}

func (s *MultiRangeSlider) notifyChange() {
	if s.OnChanged != nil {
		s.OnChanged(s.GetThresholds())
	}
}

func (s *MultiRangeSlider) valueFromPos(x float32, width float32) float64 {
	if width <= 0 {
		return s.Min
	}
	pct := float64(x / width)
	return s.Min + pct*(s.Max-s.Min)
}

// Renderer

type multiRangeSliderRenderer struct {
	s            *MultiRangeSlider
	tracks       []*canvas.Rectangle
	handles      []*canvas.Circle
	handleLabels []*canvas.Text
	titles       []*canvas.Text
	labels       []*canvas.Text
	dotStart     *canvas.Circle
	dotEnd       *canvas.Circle
}

func (r *multiRangeSliderRenderer) Layout(size fyne.Size) {
	trackHeight := float32(6)
	handleSize := float32(16)
	dotSize := float32(8)

	midY := size.Height - 12
	if midY < 50 {
		midY = 50
	}

	rangeW := r.s.Max - r.s.Min
	if rangeW <= 0 {
		rangeW = 1
	}

	// Calculate x positions for each threshold
	xPositions := make([]float32, len(r.s.Bands))
	for i := range r.s.Bands {
		if i == len(r.s.Bands)-1 {
			xPositions[i] = size.Width
		} else {
			xPositions[i] = float32((float64(r.s.Bands[i].Threshold)-r.s.Min)/rangeW) * size.Width
		}
	}

	// Layout tracks
	prevX := float32(0)
	for i, track := range r.tracks {
		track.FillColor = r.s.Bands[i].Color
		// Use lower opacity and add stroke for transparent bands
		if r.s.Bands[i].IsTransparent {
			c := r.s.Bands[i].Color
			track.FillColor = color.RGBA{c.R, c.G, c.B, 32}    // Very low alpha
			track.StrokeColor = color.RGBA{c.R, c.G, c.B, 180} // Visible stroke
			track.StrokeWidth = 1
		} else {
			track.StrokeColor = color.Transparent
			track.StrokeWidth = 0
		}
		track.Move(fyne.NewPos(prevX, midY-trackHeight/2))
		track.Resize(fyne.NewSize(xPositions[i]-prevX, trackHeight))
		prevX = xPositions[i]
	}

	// End dots
	r.dotStart.Resize(fyne.NewSize(dotSize, dotSize))
	r.dotStart.Move(fyne.NewPos(0, midY-dotSize/2))
	if len(r.s.Bands) > 0 {
		r.dotStart.FillColor = r.s.Bands[0].Color
	}

	r.dotEnd.Resize(fyne.NewSize(dotSize, dotSize))
	r.dotEnd.Move(fyne.NewPos(size.Width-dotSize, midY-dotSize/2))
	if len(r.s.Bands) > 0 {
		r.dotEnd.FillColor = r.s.Bands[len(r.s.Bands)-1].Color
	}

	// Layout handles (N-1 handles between bands)
	for i, handle := range r.handles {
		x := xPositions[i]
		handle.Resize(fyne.NewSize(handleSize, handleSize))
		handle.Move(fyne.NewPos(x-handleSize/2, midY-handleSize/2))
		handle.FillColor = theme.BackgroundColor()

		// Handle label (threshold value)
		r.handleLabels[i].Text = fmt.Sprintf("%d", r.s.Bands[i].Threshold)
		r.handleLabels[i].Move(fyne.NewPos(x-20, midY-handleSize-15))
		r.handleLabels[i].Resize(fyne.NewSize(40, 15))
	}

	// Layout titles (centered in each band segment)
	for i, title := range r.titles {
		var startX, endX float32
		if i == 0 {
			startX = 0
		} else {
			startX = xPositions[i-1]
		}
		endX = xPositions[i]

		centerX := startX + (endX-startX)/2
		title.Move(fyne.NewPos(centerX-30, midY-handleSize-35))
		title.Resize(fyne.NewSize(60, 15))
	}
}

func (r *multiRangeSliderRenderer) MinSize() fyne.Size {
	return fyne.NewSize(200, 60)
}

func (r *multiRangeSliderRenderer) Refresh() {
	// Update colors from bands
	for i, track := range r.tracks {
		if i < len(r.s.Bands) {
			track.FillColor = r.s.Bands[i].Color
		}
	}

	for _, handle := range r.handles {
		handle.FillColor = theme.BackgroundColor()
	}

	r.Layout(r.s.Size())
	canvas.Refresh(r.s)
}

func (r *multiRangeSliderRenderer) Objects() []fyne.CanvasObject {
	objs := make([]fyne.CanvasObject, 0, len(r.tracks)+len(r.handles)*2+len(r.titles)+2)

	for _, track := range r.tracks {
		objs = append(objs, track)
	}
	objs = append(objs, r.dotStart, r.dotEnd)
	for _, handle := range r.handles {
		objs = append(objs, handle)
	}
	for _, lbl := range r.handleLabels {
		objs = append(objs, lbl)
	}
	for _, title := range r.titles {
		objs = append(objs, title)
	}
	return objs
}

func (r *multiRangeSliderRenderer) Destroy() {}
