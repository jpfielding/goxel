package goxel

import (
	"math"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/widget"
)

// spinner is egui's, which roxel shows beside "Loading …": a white arc, 3 px
// thick, in a square as tall as a control. It turns once a second while its
// length swings between nothing and two thirds of the circle.
type spinner struct {
	widget.BaseWidget
	// active reports whether the spinner is still wanted; the animation stops
	// itself once it is not.
	active func() bool
}

const (
	spinnerStroke = 3
	spinnerInset  = 2 // egui's: the arc's radius is half the square, less this
)

func newSpinner(active func() bool) *spinner {
	s := &spinner{active: active}
	s.ExtendBaseWidget(s)
	return s
}

func (s *spinner) CreateRenderer() fyne.WidgetRenderer {
	r := &spinnerRenderer{s: s, arc: canvas.NewArc(0, 0, 0, colSpinner), start: time.Now()}
	r.anim = fyne.NewAnimation(time.Second, func(float32) { r.tick() })
	r.anim.RepeatCount = fyne.AnimationRepeatForever
	r.tick()
	r.anim.Start()
	return r
}

type spinnerRenderer struct {
	s     *spinner
	arc   *canvas.Arc
	anim  *fyne.Animation
	start time.Time
}

// spinnerArc is the arc at t seconds, as egui draws it: from one turn a second
// to up to 240 degrees further round, or back. Angles are Fyne's: degrees
// clockwise from the top.
func spinnerArc(t float64) (from, to float32) {
	start := math.Mod(t*360, 360) + 90 // egui measures from the right
	sweep := 240 * math.Sin(t)
	return float32(math.Min(start, start+sweep)), float32(math.Max(start, start+sweep))
}

func (r *spinnerRenderer) tick() {
	if r.s.active != nil && !r.s.active() {
		r.anim.Stop()
		return
	}
	r.arc.StartAngle, r.arc.EndAngle = spinnerArc(time.Since(r.start).Seconds())
	canvas.Refresh(r.arc)
}

func (r *spinnerRenderer) MinSize() fyne.Size {
	return fyne.NewSize(controlHeight(), controlHeight())
}

func (r *spinnerRenderer) Layout(size fyne.Size) {
	side := min(size.Width, size.Height)
	// The stroke is centred on egui's radius.
	outer := side/2 - spinnerInset + spinnerStroke/2.0
	r.arc.CutoutRatio = (outer - spinnerStroke) / outer
	r.arc.Resize(fyne.NewSize(2*outer, 2*outer))
	r.arc.Move(fyne.NewPos(size.Width/2-outer, size.Height/2-outer))
}

func (r *spinnerRenderer) Refresh()                     { canvas.Refresh(r.arc) }
func (r *spinnerRenderer) Objects() []fyne.CanvasObject { return []fyne.CanvasObject{r.arc} }
func (r *spinnerRenderer) Destroy()                     { r.anim.Stop() }
