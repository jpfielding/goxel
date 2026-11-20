package goxel

import (
	"image"
	"image/color"
	"math"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Interactive2DView shows the 2D slice centred in a black well. Zoom scales
// the fitted image (Ctrl + scroll or a pinch zooms about the pointer, 50-500 %);
// a drag pans it, or draws an ROI when annotating.
type Interactive2DView struct {
	widget.BaseWidget
	image     *canvas.Image
	well      *canvas.Rectangle
	overlay   *canvas.Rectangle // The box being drawn
	isDrawing bool
	startPos  fyne.Position
	endPos    fyne.Position

	zoom       float32 // 1 = fit
	panX, panY float32

	// TurnToFit lets a slice that is clearly larger on its side be drawn
	// turned a quarter turn (phone focus). turned says whether it is.
	TurnToFit bool
	turned    bool

	OnAnnotationAdded func(x, y, w, h float32)
	viewer            *Viewer // Reference to check annotation mode

	// OnLayout runs after each layout, so the footer's scale bar follows
	// the zoom and the window size.
	OnLayout func()
	scale    float32 // pixels per millimetre of the image as last drawn
}

// pixelsPerMM is the scale of the image as drawn, for the footer's scale bar.
func (iv *Interactive2DView) pixelsPerMM() float32 { return iv.scale }

func NewInteractive2DView(img *canvas.Image, v *Viewer) *Interactive2DView {
	iv := &Interactive2DView{
		image:   img,
		well:    canvas.NewRectangle(colWell),
		overlay: canvas.NewRectangle(color.RGBA{255, 0, 0, 100}),
		viewer:  v,
		zoom:    1,
	}
	iv.overlay.Hide()
	iv.overlay.StrokeWidth = 2
	iv.overlay.StrokeColor = color.RGBA{255, 0, 0, 255}
	iv.ExtendBaseWidget(iv)
	return iv
}

// SetZoom sets the zoom in percent (100 = fit) and recentres when fitting.
func (iv *Interactive2DView) SetZoom(percent float64) {
	iv.zoom = float32(percent / 100)
	if iv.zoom <= 1 {
		iv.panX, iv.panY = 0, 0
	}
	iv.Refresh()
}

// centre puts the image back in the middle of the well.
func (iv *Interactive2DView) centre() {
	iv.panX, iv.panY = 0, 0
	iv.Refresh()
}

// turnGain is how much larger a slice must get on its side before it is turned.
const turnGain = 1.25

// Zoom limits, as fractions of the fitted size.
const (
	minImageZoom = 0.5
	maxImageZoom = 5
)

// Scrolled zooms about the pointer: a touch pinch (a ratio from the page) or
// Ctrl + scroll. A plain scroll moves the zoomed image in the well, as it
// scrolls roxel's.
func (iv *Interactive2DView) Scrolled(e *fyne.ScrollEvent) {
	f := takePinch()
	if f == 0 {
		if !zoomModifierDown() {
			if iv.zoom > 1 {
				iv.panX += e.Scrolled.DX
				iv.panY += e.Scrolled.DY
				iv.Refresh()
			}
			return
		}
		f = math.Exp(math.Max(-0.2, math.Min(0.2, float64(e.Scrolled.DY)*0.01)))
	}
	iv.zoomAbout(float32(f), e.Position)
}

// zoomAbout multiplies the zoom by f, keeping the point under at where it is,
// and tells the Zoom slider.
func (iv *Interactive2DView) zoomAbout(f float32, at fyne.Position) {
	old := iv.zoom
	z := max(minImageZoom, min(maxImageZoom, old*f))
	if z == old {
		return
	}
	size := iv.Size()
	if z <= 1 {
		iv.panX, iv.panY = 0, 0
	} else {
		r := z / old
		px, py := at.X-size.Width/2, at.Y-size.Height/2
		iv.panX = px - (px-iv.panX)*r
		iv.panY = py - (py-iv.panY)*r
	}
	iv.zoom = z
	iv.Refresh()
	if iv.viewer != nil {
		iv.viewer.zoomChanged(float64(z) * 100)
	}
}

// applySource points the image at the slice, turned when it is drawn on its side.
func (iv *Interactive2DView) applySource() {
	if iv.viewer == nil {
		return
	}
	var src image.Image = iv.viewer.dicox
	if iv.turned {
		src = rotatedCW{iv.viewer.dicox}
	}
	iv.image.Image = src
}

// rotatedCW is an image turned a quarter turn clockwise: the source's left
// edge runs along the top.
type rotatedCW struct{ src image.Image }

func (r rotatedCW) ColorModel() color.Model { return r.src.ColorModel() }
func (r rotatedCW) Bounds() image.Rectangle {
	b := r.src.Bounds()
	return image.Rect(0, 0, b.Dy(), b.Dx())
}
func (r rotatedCW) At(x, y int) color.Color {
	b := r.src.Bounds()
	return r.src.At(b.Min.X+y, b.Max.Y-1-x)
}

func (iv *Interactive2DView) CreateRenderer() fyne.WidgetRenderer {
	iv.image.FillMode = canvas.ImageFillStretch
	// Voxels stay square-edged when magnified, as in roxel.
	iv.image.ScaleMode = canvas.ImageScalePixels
	return &interactive2DRenderer{iv: iv}
}

func (iv *Interactive2DView) annotating() bool {
	return iv.viewer != nil && iv.viewer.isAnnotating
}

func (iv *Interactive2DView) Dragged(e *fyne.DragEvent) {
	if !iv.annotating() {
		iv.panX += e.Dragged.DX
		iv.panY += e.Dragged.DY
		iv.Refresh()
		return
	}
	iv.endPos = e.PointEvent.Position
	iv.Refresh()
}

func (iv *Interactive2DView) DragEnd() {
	if !iv.annotating() || !iv.isDrawing {
		return
	}
	iv.isDrawing = false
	iv.viewer.isAnnotating = false // Exit mode after one annotation

	x1, y1 := iv.startPos.X, iv.startPos.Y
	x2, y2 := iv.endPos.X, iv.endPos.Y
	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}
	w, h := x2-x1, y2-y1
	if w > 5 && h > 5 && iv.OnAnnotationAdded != nil {
		iv.OnAnnotationAdded(x1, y1, w, h)
	}
	iv.overlay.Hide()
	iv.Refresh()
}

func (iv *Interactive2DView) MouseDown(e *desktop.MouseEvent) {
	if !iv.annotating() {
		return
	}
	if e.Button == desktop.MouseButtonPrimary {
		iv.isDrawing = true
		iv.startPos = e.Position
		iv.endPos = e.Position
		iv.overlay.Show()
		iv.Refresh()
	}
}

func (iv *Interactive2DView) MouseUp(*desktop.MouseEvent) {}

// imageSize is the slice's physical size in millimetres for the current
// orientation: the image is drawn in true proportions, not in voxels.
func (iv *Interactive2DView) imageSize() (float32, float32) {
	if iv.viewer != nil && iv.viewer.dicox != nil && iv.viewer.loaded() {
		if w, h := iv.viewer.sliceSizeMM(); w > 0 && h > 0 {
			return float32(w), float32(h)
		}
	}
	if iv.image.Image != nil {
		b := iv.image.Image.Bounds()
		if b.Dx() > 0 && b.Dy() > 0 {
			return float32(b.Dx()), float32(b.Dy())
		}
	}
	return 1, 1
}

type interactive2DRenderer struct {
	iv *Interactive2DView
}

func (r *interactive2DRenderer) Layout(size fyne.Size) {
	iv := r.iv
	iv.well.Move(fyne.NewPos(0, 0))
	iv.well.Resize(size)

	imgW, imgH := iv.imageSize()
	// The slice fits inside the well less a 1 px margin, as roxel's does.
	room := fyne.NewSize(max(size.Width-2, 1), max(size.Height-2, 1))
	upright := min(room.Width/imgW, room.Height/imgH)
	onSide := min(room.Width/imgH, room.Height/imgW)
	turned := iv.TurnToFit && onSide >= upright*turnGain
	if turned != iv.turned {
		iv.turned = turned
		iv.panX, iv.panY = 0, 0
		iv.applySource()
		iv.image.Refresh()
	}
	fit := upright
	shownW, shownH := imgW, imgH
	if turned {
		fit, shownW, shownH = onSide, imgH, imgW
	}
	scale := fit * iv.zoom
	iv.scale = scale
	w, h := shownW*scale, shownH*scale
	// The image never leaves the well: pan only as far as it overflows.
	maxX, maxY := max((w-size.Width)/2, 0), max((h-size.Height)/2, 0)
	iv.panX = max(-maxX, min(maxX, iv.panX))
	iv.panY = max(-maxY, min(maxY, iv.panY))
	iv.image.Move(fyne.NewPos((size.Width-w)/2+iv.panX, (size.Height-h)/2+iv.panY))
	iv.image.Resize(fyne.NewSize(w, h))

	if iv.isDrawing {
		x1, y1 := iv.startPos.X, iv.startPos.Y
		x2, y2 := iv.endPos.X, iv.endPos.Y
		if x1 > x2 {
			x1, x2 = x2, x1
		}
		if y1 > y2 {
			y1, y2 = y2, y1
		}
		iv.overlay.Move(fyne.NewPos(x1, y1))
		iv.overlay.Resize(fyne.NewSize(x2-x1, y2-y1))
	}
	if iv.OnLayout != nil {
		iv.OnLayout()
	}
}

func (r *interactive2DRenderer) MinSize() fyne.Size {
	return fyne.NewSize(120, 120)
}

func (r *interactive2DRenderer) Refresh() {
	r.Layout(r.iv.Size())
	r.iv.image.Refresh()
	canvas.Refresh(r.iv.overlay)
}

func (r *interactive2DRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.iv.well, r.iv.image, r.iv.overlay}
}

func (r *interactive2DRenderer) Destroy() {}
