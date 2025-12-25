package goxel

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/widget"
)

// Interactive2DView wraps the 2D image and handles ROI annotation
type Interactive2DView struct {
	widget.BaseWidget
	image     *canvas.Image
	overlay   *canvas.Rectangle // The box being drawn
	isDrawing bool
	startPos  fyne.Position
	endPos    fyne.Position

	OnAnnotationAdded func(x, y, w, h float32)
	viewer            *Viewer // Reference to check annotation mode
}

func NewInteractive2DView(img *canvas.Image, v *Viewer) *Interactive2DView {
	iv := &Interactive2DView{
		image:   img,
		overlay: canvas.NewRectangle(color.RGBA{255, 0, 0, 100}),
		viewer:  v,
	}
	iv.overlay.Hide()
	iv.overlay.StrokeWidth = 2
	iv.overlay.StrokeColor = color.RGBA{255, 0, 0, 255}
	iv.ExtendBaseWidget(iv)
	return iv
}

func (iv *Interactive2DView) CreateRenderer() fyne.WidgetRenderer {
	iv.image.FillMode = canvas.ImageFillStretch
	return &interactive2DRenderer{iv: iv}
}

func (iv *Interactive2DView) Dragged(e *fyne.DragEvent) {
	if !iv.viewer.isAnnotating {
		return
	}
	iv.endPos = e.PointEvent.Position
	iv.Refresh()
}

func (iv *Interactive2DView) DragEnd() {
	if !iv.viewer.isAnnotating || !iv.isDrawing {
		return
	}
	iv.isDrawing = false
	iv.viewer.isAnnotating = false // Exit mode after one annotation

	// Normalize coordinates
	x1 := float32(iv.startPos.X)
	y1 := float32(iv.startPos.Y)
	x2 := float32(iv.endPos.X)
	y2 := float32(iv.endPos.Y)

	if x1 > x2 {
		x1, x2 = x2, x1
	}
	if y1 > y2 {
		y1, y2 = y2, y1
	}

	w := x2 - x1
	h := y2 - y1

	if w > 5 && h > 5 && iv.OnAnnotationAdded != nil {
		iv.OnAnnotationAdded(x1, y1, w, h)
	}

	iv.overlay.Hide()
	iv.Refresh()
}

func (iv *Interactive2DView) MouseDown(e *desktop.MouseEvent) {
	if !iv.viewer.isAnnotating {
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

func (iv *Interactive2DView) MouseUp(e *desktop.MouseEvent) {
	// Handled by DragEnd usually, but if no drag occurred?
}

type interactive2DRenderer struct {
	iv *Interactive2DView
}

func (r *interactive2DRenderer) Layout(size fyne.Size) {
	// Aspect Fit, Top Aligned
	// We need the intrinsic aspect ratio.
	// canvas.Image doesn't expose it easily unless we check the underlying Image?
	// We can get aspect from r.iv.viewer.dicox.
	// Or try to rely on image bounds.

	aspect := float32(1.0)
	if r.iv.viewer.dicox != nil && r.iv.viewer.dicox.Width() > 0 {
		// DICOX aspect depends on orientation.
		// Width/Height/Depth changes.
		// Best source is the image itself?
		// BaseWidget.Size() is current size.
		// Let's use 1.0 default if unknown.

		// Actually, canvas.ImageFromImage sets the Resource.
		// We can check r.iv.image.Image.Bounds().
	}

	// Better approach:
	// Let's calculate aspect from the dicox orientation.
	// This requires access to dicox state which might race?
	// Layout is called on main thread, safe.

	imgW := float32(100)
	imgH := float32(100)

	// Try to get actual size
	if r.iv.viewer != nil && r.iv.viewer.dicox != nil {
		orient := r.iv.viewer.dicox.Orientation()
		d := r.iv.viewer.dicox
		switch orient {
		case Axial:
			imgW = float32(d.Width())
			imgH = float32(d.Height())
		case Coronal:
			imgW = float32(d.Width())
			imgH = float32(d.Depth())
		case Sagittal:
			imgW = float32(d.Depth())  // Y?
			imgH = float32(d.Height()) // Z?
			// Actually Sagittal is usually Depth x Height
			imgW = float32(d.Height())
			imgH = float32(d.Depth())
		}
		// Fallback simple names: Columns, Rows implies XY
	} else if r.iv.image.Image != nil {
		b := r.iv.image.Image.Bounds()
		imgW = float32(b.Dx())
		imgH = float32(b.Dy())
	}

	if imgW == 0 || imgH == 0 {
		imgW, imgH = 1, 1
	}

	aspect = imgW / imgH

	viewW := size.Width
	viewH := size.Height

	// Scale to fit width
	renderW := viewW
	renderH := viewW / aspect

	// If height too big, fit height
	if renderH > viewH {
		renderH = viewH
		renderW = viewH * aspect
	}

	// Position Top-Left (0,0)
	r.iv.image.Move(fyne.NewPos(0, 0))
	r.iv.image.Resize(fyne.NewSize(renderW, renderH))

	// Update overlay logic to clip?
	if r.iv.isDrawing {
		x1 := float32(r.iv.startPos.X)
		y1 := float32(r.iv.startPos.Y)
		x2 := float32(r.iv.endPos.X)
		y2 := float32(r.iv.endPos.Y)

		if x1 > x2 {
			x1, x2 = x2, x1
		}
		if y1 > y2 {
			y1, y2 = y2, y1
		}

		r.iv.overlay.Move(fyne.NewPos(x1, y1))
		r.iv.overlay.Resize(fyne.NewSize(x2-x1, y2-y1))
	}
}

func (r *interactive2DRenderer) MinSize() fyne.Size {
	return r.iv.image.MinSize()
}

func (r *interactive2DRenderer) Refresh() {
	r.iv.image.Refresh()
	canvas.Refresh(r.iv.overlay)
}

func (r *interactive2DRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.iv.image, r.iv.overlay}
}

func (r *interactive2DRenderer) Destroy() {}
