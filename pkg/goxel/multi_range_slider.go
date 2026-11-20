package goxel

import (
	"image/color"
	"math"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
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

	// Compact enlarges handles and height for touch.
	Compact bool
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
		s:     s,
		track: rrect(colControl, color.Transparent, 3, 0),
	}
	for range s.Bands {
		r.segments = append(r.segments, rrect(color.Transparent, color.Transparent, 2, 0))
		r.names = append(r.names, text("", 12, colText))
	}
	mono := fyne.TextStyle{Monospace: true}
	for i := 0; i < len(s.Bands)-1; i++ {
		h := canvas.NewCircle(colBG)
		h.StrokeColor = bandHandleStroke
		h.StrokeWidth = 1.5
		r.handles = append(r.handles, h)
		l := text("", 11, colText)
		l.TextStyle = mono
		r.values = append(r.values, l)
	}
	return r
}

// bandHandleStroke is the outline of a resting handle: egui's inactive widget
// stroke.
var bandHandleStroke = color.NRGBA{180, 180, 180, 255}

// Handle radius and the strip's height: bigger where a finger does the dragging.
func (s *MultiRangeSlider) handleRadius() float32 {
	if s.Compact {
		return 11
	}
	return 6
}

func (s *MultiRangeSlider) stripHeight() float32 {
	if s.Compact {
		return 96
	}
	return 84
}

// track is the band bar's extent: inset by the handle radius (and 2 px) so a
// handle at either end stays inside the widget; 12 px tall (16 on touch), its
// bottom 26 px (28) above the widget's.
func (s *MultiRangeSlider) trackRect(size fyne.Size) (left, right, top, bottom float32) {
	r := s.handleRadius()
	if s.Compact {
		return r + 2, size.Width - r - 2, size.Height - 44, size.Height - 28
	}
	return r + 2, size.Width - r - 2, size.Height - 38, size.Height - 26
}

// xAt is the x of a value, and valueAt its inverse, over the track.
func (s *MultiRangeSlider) xAt(v float64, size fyne.Size) float32 {
	left, right, _, _ := s.trackRect(size)
	return left + float32((v-s.Min)/s.span())*(right-left)
}

func (s *MultiRangeSlider) valueAt(x float32, size fyne.Size) float64 {
	left, right, _, _ := s.trackRect(size)
	if right <= left {
		return s.Min
	}
	t := math.Max(0, math.Min(1, float64((x-left)/(right-left))))
	return math.Round(s.Min + t*s.span())
}

func (s *MultiRangeSlider) span() float64 {
	if s.Max <= s.Min {
		return 1
	}
	return s.Max - s.Min
}

// Mouse interactions: a press takes the handle nearest the pointer and moves it
// there; dragging moves it with the pointer, between its neighbours.

func (s *MultiRangeSlider) MouseDown(e *desktop.MouseEvent) {
	if len(s.Bands) <= 1 {
		return
	}
	s.draggedHandle = s.nearestHandle(e.Position.X)
	s.moveHandle(e.Position.X)
}

func (s *MultiRangeSlider) MouseUp(*desktop.MouseEvent) { s.release() }

func (s *MultiRangeSlider) DragEnd() { s.release() }

func (s *MultiRangeSlider) Dragged(e *fyne.DragEvent) { s.moveHandle(e.Position.X) }

func (s *MultiRangeSlider) Tapped(*fyne.PointEvent) {}

func (s *MultiRangeSlider) release() {
	if s.draggedHandle >= 0 {
		s.draggedHandle = -1
		s.Refresh()
	}
}

// nearestHandle is the handle whose x is closest to x.
func (s *MultiRangeSlider) nearestHandle(x float32) int {
	size := s.Size()
	nearest, best := -1, float32(math.MaxFloat32)
	for i := 0; i < len(s.Bands)-1; i++ {
		d := x - s.xAt(float64(s.Bands[i].Threshold), size)
		if d < 0 {
			d = -d
		}
		if d < best {
			nearest, best = i, d
		}
	}
	return nearest
}

// moveHandle puts the dragged handle at x, between its neighbours, and says so.
func (s *MultiRangeSlider) moveHandle(x float32) {
	i := s.draggedHandle
	if i < 0 || i >= len(s.Bands)-1 {
		return
	}
	lo, hi := s.Min, s.Max
	if i > 0 {
		lo = float64(s.Bands[i-1].Threshold)
	}
	if i+1 < len(s.Bands)-1 {
		hi = float64(s.Bands[i+1].Threshold)
	}
	v := int(math.Max(lo, math.Min(hi, s.valueAt(x, s.Size()))))
	if s.Bands[i].Threshold == v {
		return
	}
	s.Bands[i].Threshold = v
	s.Refresh()
	s.notifyChange()
}

func (s *MultiRangeSlider) clampValues() {
	for i := range s.Bands {
		s.Bands[i].Threshold = max(int(s.Min), min(int(s.Max), s.Bands[i].Threshold))
	}
	// Each threshold is at least the one before.
	for i := 1; i < len(s.Bands)-1; i++ {
		if s.Bands[i].Threshold < s.Bands[i-1].Threshold {
			s.Bands[i].Threshold = s.Bands[i-1].Threshold
		}
	}
}

func (s *MultiRangeSlider) notifyChange() {
	if s.OnChanged != nil {
		s.OnChanged(s.GetThresholds())
	}
}

// Renderer, after roxel's band_slider.rs.

type multiRangeSliderRenderer struct {
	s        *MultiRangeSlider
	track    *canvas.Rectangle
	segments []*canvas.Rectangle
	names    []*canvas.Text
	handles  []*canvas.Circle
	values   []*canvas.Text
}

// minNameSegment is the narrowest segment that still carries its band's name.
const minNameSegment = 36

func (r *multiRangeSliderRenderer) Layout(size fyne.Size) {
	s := r.s
	rad := s.handleRadius()
	left, right, top, bottom := s.trackRect(size)
	r.track.Move(fyne.NewPos(left, top))
	r.track.Resize(fyne.NewSize(max(right-left, 0), bottom-top))

	for i, seg := range r.segments {
		start, end := s.Min, s.Max
		if i > 0 {
			start = float64(s.Bands[i-1].Threshold)
		}
		if i < len(s.Bands)-1 {
			end = float64(s.Bands[i].Threshold)
		}
		sx, ex := s.xAt(start, size), s.xAt(end, size)
		c := s.Bands[i].Color
		fill := color.NRGBA{c.R, c.G, c.B, 255}
		if s.Bands[i].IsTransparent {
			fill.A = 64 // the colour at a quarter, as roxel fades the transparent band
		}
		seg.FillColor = fill
		name := r.names[i]
		if ex <= sx {
			seg.Hide()
			name.Hide()
			continue
		}
		seg.Show()
		seg.Move(fyne.NewPos(sx, top))
		seg.Resize(fyne.NewSize(ex-sx, bottom-top))
		name.Text = s.Bands[i].Name
		if ex-sx > minNameSegment {
			ts := measure(name.Text, 12)
			name.Move(fyne.NewPos((sx+ex-ts.Width)/2, bottom+rad+1))
			name.Resize(ts)
			name.Show()
		} else {
			name.Hide()
		}
	}

	for i, h := range r.handles {
		x := s.xAt(float64(s.Bands[i].Threshold), size)
		cy := (top + bottom) / 2
		// The outline goes outside the handle's radius, as egui draws it.
		hr := rad + h.StrokeWidth
		h.Move(fyne.NewPos(x-hr, cy-hr))
		h.Resize(fyne.NewSize(2*hr, 2*hr))
		h.StrokeColor = bandHandleStroke
		if s.draggedHandle == i {
			h.StrokeColor = colAccent
		}
		l := r.values[i]
		l.Text = strconv.Itoa(s.Bands[i].Threshold)
		ts := fyne.MeasureText(l.Text, l.TextSize, l.TextStyle)
		l.Move(fyne.NewPos(x-ts.Width/2, top-rad+1-ts.Height))
		l.Resize(ts)
	}
}

func (r *multiRangeSliderRenderer) MinSize() fyne.Size {
	return fyne.NewSize(180, r.s.stripHeight())
}

func (r *multiRangeSliderRenderer) Refresh() {
	r.Layout(r.s.Size())
	for _, o := range r.Objects() {
		canvas.Refresh(o)
	}
}

func (r *multiRangeSliderRenderer) Objects() []fyne.CanvasObject {
	objs := []fyne.CanvasObject{r.track}
	for _, o := range r.segments {
		objs = append(objs, o)
	}
	for _, o := range r.names {
		objs = append(objs, o)
	}
	for _, o := range r.handles {
		objs = append(objs, o)
	}
	for _, o := range r.values {
		objs = append(objs, o)
	}
	return objs
}

func (r *multiRangeSliderRenderer) Destroy() {}
