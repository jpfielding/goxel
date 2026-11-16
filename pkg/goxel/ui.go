package goxel

import (
	"fmt"
	"image/color"
	"log/slog"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/jpfielding/goxel/pkg/volume"
)

// Viewer is the goxel window: plain UI state plus the long-lived views (the
// 3D renderer and the 2D slice). Everything else is rebuilt from state.
type Viewer struct {
	app   fyne.App
	win   fyne.Window
	theme *goxelTheme
	st    uiState

	image           *canvas.Image
	dicox           *DICOXImage
	interactiveView *Interactive2DView
	volumeRenderer  *VolumeRenderer

	// content holds the current widget tree; root watches the window size.
	content *fyne.Container
	root    *fyne.Container

	// Loaded study
	scan             *ScanCollection
	primary          *VolumeData
	compositeVolumes map[string]*CompositeVolume
	projections      map[string]*CompositeVolume
	currentVolumeKey string // volume shown in 2D

	isAnnotating   bool
	rebuildPending bool
	narrowShelf    bool

	// syncers re-read state into the live widgets of the current tree, for
	// changes that do not warrant a rebuild (slice stepping, value readouts).
	syncers []func()

	debouncers map[string]*debouncer
}

// NewViewer creates the window, the views and the empty-state UI.
func NewViewer(a fyne.App) *Viewer {
	v := &Viewer{
		app:        a,
		win:        a.NewWindow("goxel"),
		theme:      &goxelTheme{},
		st:         newUIState(),
		debouncers: map[string]*debouncer{},
	}
	a.Settings().SetTheme(v.theme)
	if !fyne.CurrentDevice().IsBrowser() {
		v.win.SetMainMenu(v.makeMenu())
	}

	v.dicox = NewDICOXImage(nil, 0, 0, 40, 400)
	v.image = canvas.NewImageFromImage(v.dicox)
	v.image.FillMode = canvas.ImageFillContain
	v.interactiveView = NewInteractive2DView(v.image, v)
	v.interactiveView.OnAnnotationAdded = func(x, y, w, h float32) {
		slog.Info("Annotation Added", "rect", fmt.Sprintf("%.1f,%.1f %.1fx%.1f", x, y, w, h))
		dialog.ShowInformation("ROI Annotated", "ROI bounding box created.", v.win)
	}

	v.volumeRenderer = NewVolumeRenderer()
	v.volumeRenderer.DragPans = func() bool { return v.st.dragPans }

	v.content = container.NewStack()
	v.root = container.New(&responsiveLayout{v: v}, v.content)
	v.win.SetContent(v.root)
	v.win.Canvas().SetOnTypedKey(v.onKey)
	v.win.Resize(fyne.NewSize(1440, 900))
	v.rebuild()
	return v
}

// makeMenu creates the desktop main menu.
func (v *Viewer) makeMenu() *fyne.MainMenu {
	fileMenu := fyne.NewMenu("File",
		fyne.NewMenuItem("Open File...", v.openFile),
		fyne.NewMenuItem("Open Folder...", v.openFolder),
		fyne.NewMenuItemSeparator(),
		fyne.NewMenuItem("Export Screenshot", v.exportScreenshot),
	)
	viewMenu := fyne.NewMenu("View",
		fyne.NewMenuItem("Full Screen", func() { v.win.SetFullScreen(!v.win.FullScreen()) }),
	)
	return fyne.NewMainMenu(fileMenu, viewMenu)
}

// exportScreenshot is a stub kept from the original menu.
func (v *Viewer) exportScreenshot() {
	dialog.ShowFileSave(func(writer fyne.URIWriteCloser, err error) {
		if err != nil {
			dialog.ShowError(err, v.win)
			return
		}
		if writer == nil {
			return
		}
		defer writer.Close()
		_ = v.win.Canvas().Capture()
		slog.Info("Screenshot saved", "path", writer.URI().Path())
	}, v.win)
}

// Run starts the application event loop.
func (v *Viewer) Run() { v.win.ShowAndRun() }

// LoadKeys is kept for callers; keys are installed by NewViewer.
func (v *Viewer) LoadKeys() {}

// loaded reports whether a study is open.
func (v *Viewer) loaded() bool { return v.scan != nil }

// rebuild replaces the widget tree with one built from state.
func (v *Viewer) rebuild() {
	v.syncers = nil
	// The theme reads its compact flag on every size lookup, and the tree is
	// rebuilt below, so no SetTheme is needed. (SetTheme at runtime also
	// crashes Fyne 2.7's wasm driver while it reloads textures.)
	compact := v.st.layout.compact()
	v.theme.compact = compact
	var c fyne.CanvasObject
	if compact {
		c = v.phoneRoot()
	} else {
		c = v.desktopRoot()
	}
	v.content.Objects = []fyne.CanvasObject{c}
	v.content.Refresh()
}

// sync pushes state into the live widgets after a change made outside them.
func (v *Viewer) sync() {
	for _, f := range v.syncers {
		f()
	}
}

func (v *Viewer) onSync(f func()) { v.syncers = append(v.syncers, f) }

// debounced runs fn on the UI goroutine once calls under key stop for d.
func (v *Viewer) debounced(key string, d time.Duration, fn func()) {
	db, ok := v.debouncers[key]
	if !ok {
		db = newDebouncer(d)
		v.debouncers[key] = db
	}
	db.call(func() { fyne.Do(fn) })
}

// onKey: Esc leaves focus; arrows step the slice.
func (v *Viewer) onKey(e *fyne.KeyEvent) {
	switch e.Name {
	case fyne.KeyEscape:
		if v.win.Canvas().Overlays().Top() != nil {
			return
		}
		if _, ok := v.win.Canvas().Focused().(*widget.Entry); ok {
			return
		}
		if v.st.exitFocus() {
			v.rebuild()
			v.volumeRenderer.FitView()
		}
	case fyne.KeyUp, fyne.KeyRight:
		v.stepSlice(1)
	case fyne.KeyDown, fyne.KeyLeft:
		v.stepSlice(-1)
	}
}

// --- Loading -------------------------------------------------------------

// beginLoad records a load in flight; it returns false if one already is.
func (v *Viewer) beginLoad(name string) bool {
	if v.st.loadingFile != "" {
		return false
	}
	v.st.loadingFile = name
	v.st.loadError = ""
	v.rebuild()
	return true
}

// finishLoad installs a loaded study or shows the error inline.
func (v *Viewer) finishLoad(path string, scan *ScanCollection, err error) {
	v.st.loadingFile = ""
	if err == nil && scan.PrimaryVolume() == nil {
		err = fmt.Errorf("%s has no image volume", filepath.Base(path))
	}
	if err != nil {
		slog.Error("Load failed", "path", path, "error", err)
		v.st.loadError = err.Error()
		v.rebuild()
		return
	}
	v.st.loadedPath = path
	v.st.loadedName = filepath.Base(path)
	if err := v.SetScanData(scan); err != nil {
		v.st.loadError = err.Error()
		v.rebuild()
	}
}

// loadPath loads a file or folder off the UI goroutine.
func (v *Viewer) loadPath(path string) {
	if !v.beginLoad(filepath.Base(path)) {
		return
	}
	go func() {
		scan, err := Load(path)
		fyne.Do(func() { v.finishLoad(path, scan, err) })
	}()
}

// --- Study ---------------------------------------------------------------

// volumeKeys returns the 3D volume names, sorted.
func (v *Viewer) volumeKeys() []string {
	keys := make([]string, 0, len(v.compositeVolumes))
	for k := range v.compositeVolumes {
		if !strings.Contains(strings.ToLower(k), "finding") {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}

// sliceSourceKeys returns everything that can be shown in 2D: volumes, then projections.
func (v *Viewer) sliceSourceKeys() []string {
	keys := v.volumeKeys()
	var proj []string
	for k := range v.projections {
		proj = append(proj, k)
	}
	sort.Strings(proj)
	return append(keys, proj...)
}

func (v *Viewer) findingCount() int {
	if v.scan == nil {
		return 0
	}
	return len(v.scan.Findings)
}

// setup3DView (re)loads every enabled volume and finding into the renderer.
func (v *Viewer) setup3DView() {
	vr := v.volumeRenderer
	vr.ClearVolumes()
	vr.ClearFindings()

	keys := make([]string, 0, len(v.compositeVolumes))
	for k := range v.compositeVolumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	findingPalette := []color.RGBA{
		{255, 0, 0, 255}, {255, 128, 0, 255}, {255, 0, 255, 255}, {0, 255, 255, 255},
		{255, 255, 0, 255}, {128, 0, 255, 255}, {0, 255, 128, 255}, {255, 128, 128, 255},
	}
	var refX, refY, refZ int
	if p := v.primary; p != nil {
		refX, refY, refZ = p.Width, p.Height, p.Depth
	}
	fi := 0
	for _, k := range keys {
		vol := v.compositeVolumes[k]
		if !vol.Enabled {
			continue
		}
		if strings.Contains(strings.ToLower(k), "finding") && vol.BBox != nil {
			if vol.Color.A == 0 {
				vol.Color = findingPalette[fi%len(findingPalette)]
			}
			fi++
			vr.AddFinding(k, 0, *vol.BBox, vol.Color, refX, refY, refZ)
			continue
		}
		_ = vr.AddVolume(vol.PixelData, vol.Rows, vol.Cols, vol.PixelRep, vol.Color,
			v.st.windowCenter3D, v.st.windowWidth3D, vol.RescaleIntercept, vol.Alpha)
	}
	v.applyAll3D()
}

// set2DSource shows a volume or projection in the 2D view.
func (v *Viewer) set2DSource(key string) {
	cv, ok := v.compositeVolumes[key]
	if !ok {
		cv, ok = v.projections[key]
	}
	if !ok || cv.PixelData == nil {
		return
	}
	v.currentVolumeKey = key
	w, h, d := cv.Cols, cv.Rows, len(cv.PixelData.Frames)
	data := make([][][]int, d)
	for z, f := range cv.PixelData.Frames {
		data[z] = make([][]int, h)
		for y := 0; y < h; y++ {
			row := make([]int, w)
			off := y * w
			for x := 0; x < w && off+x < len(f.Data); x++ {
				row[x] = int(f.Data[off+x])
			}
			data[z][y] = row
		}
	}
	v.dicox.SetVolume(data, w, h, d)
	v.dicox.SetOrientation(v.st.orientation)
	v.st.sliceIndex = v.dicox.SliceIndex()
	v.applyCutaway()
	v.sync()
	v.refresh2D()
}

// --- Apply state to the views --------------------------------------------

func (v *Viewer) applyAll3D() {
	vr := v.volumeRenderer
	vr.SetWindowLevel(v.st.windowCenter3D)
	vr.SetWindowWidth(v.st.windowWidth3D)
	vr.SetAlphaScale(v.st.opacity)
	vr.SetDensityThreshold(v.st.density)
	vr.SetLighting(v.st.ambient, v.st.diffuse, v.st.specular)
	vr.SetQuality(int(v.st.quality))
	v.applyPalette()
	v.applyCutaway()
	vr.Redraw()
}

func (v *Viewer) redraw3D() {
	v.debounced("3d", 30*time.Millisecond, v.volumeRenderer.Redraw)
}

func (v *Viewer) apply3DWindow() {
	v.volumeRenderer.SetWindowLevel(v.st.windowCenter3D)
	v.volumeRenderer.SetWindowWidth(v.st.windowWidth3D)
	v.redraw3D()
}

func (v *Viewer) applyOpacity() {
	v.volumeRenderer.SetAlphaScale(v.st.opacity)
	v.dicox.SetAlphaScale(v.st.opacity)
	v.redraw3D()
	if v.st.composite {
		v.debounced("2d", 80*time.Millisecond, v.refresh2D)
	}
}

func (v *Viewer) applyDensity() {
	v.volumeRenderer.SetDensityThreshold(v.st.density)
	v.redraw3D()
}

func (v *Viewer) applyLighting() {
	v.volumeRenderer.SetLighting(v.st.ambient, v.st.diffuse, v.st.specular)
	v.redraw3D()
}

func (v *Viewer) applyQuality() {
	v.volumeRenderer.SetQuality(int(v.st.quality))
	v.redraw3D()
}

// applyPalette sets the 3D transfer function and the 2D composite's colours.
func (v *Viewer) applyPalette() {
	cfg := volume.GetConfig()
	var tf []color.RGBA
	if v.st.preset == paletteMaterials {
		bands := append([]volume.ColorBand(nil), v.st.bands...)
		v.volumeRenderer.SetMaterialBands(bands)
		tf = volume.CreateTransferFunctionFromBandsWithGradient(cfg, "DEFAULT", bands)
	} else {
		name := v.st.preset.configName()
		v.volumeRenderer.SetColorOpacityPreset(name)
		tf = volume.CreateTransferFunctionFromConfig(cfg, name, 30000)
	}
	v.dicox.SetTransferFunction(tf)
	v.redraw3D()
	if v.st.composite {
		v.debounced("2d", 80*time.Millisecond, v.refresh2D)
	}
}

// cutawayAvailable: the cutaway follows a single 2D slice of a 3D volume.
func (v *Viewer) cutawayAvailable() bool {
	return v.loaded() && !v.st.composite && v.compositeVolumes[v.currentVolumeKey] != nil
}

func (v *Viewer) applyCutaway() {
	if !v.st.cutaway || !v.cutawayAvailable() {
		v.volumeRenderer.SetClippingPlane(0, 0, 0, 0, false)
		v.redraw3D()
		return
	}
	nx, ny, nz, d := cutawayPlane(v.st.orientation, v.st.sliceIndex, v.dicox.MaxSlice()+1, v.st.reverseCutaway)
	v.volumeRenderer.SetClippingPlane(nx, ny, nz, d, true)
	v.redraw3D()
}

func (v *Viewer) refresh2D() {
	v.image.Image = v.dicox
	canvas.Refresh(v.image)
	v.interactiveView.Refresh()
}

// setSlice moves the 2D slice (and the cutaway that follows it).
func (v *Viewer) setSlice(i int) {
	i = max(0, min(i, v.dicox.MaxSlice()))
	if i == v.st.sliceIndex && v.dicox.SliceIndex() == i {
		return
	}
	v.st.sliceIndex = i
	v.dicox.SetSliceIndex(i)
	v.sync()
	v.debounced("2d", 16*time.Millisecond, v.refresh2D)
	if v.st.cutaway {
		v.applyCutaway()
	}
}

func (v *Viewer) stepSlice(d int) {
	if !v.loaded() || v.st.composite {
		return
	}
	v.setSlice(v.st.sliceIndex + d)
}

func (v *Viewer) setOrientation(o ViewOrientation) {
	v.st.orientation = o
	v.dicox.SetOrientation(o)
	v.st.sliceIndex = v.dicox.SliceIndex()
	v.interactiveView.SetZoom(v.st.zoom2D)
	v.sync()
	v.refresh2D()
	if v.st.cutaway {
		v.applyCutaway()
	}
}

func (v *Viewer) setComposite(on bool) {
	v.st.composite = on
	v.dicox.SetCompositeMode(on)
	v.refresh2D()
	v.applyCutaway()
}

// sliceCaption is "Axial 12 / 128 · 640 × 440 mm", as under roxel's well.
func (v *Viewer) sliceCaption() string {
	if !v.loaded() {
		return ""
	}
	name := orientationLabels[v.st.orientation]
	where := fmt.Sprintf("%s %d / %d", name, v.st.sliceIndex+1, v.dicox.MaxSlice()+1)
	if v.st.composite {
		where = name + " projection"
	}
	if cv := v.compositeVolumes[v.currentVolumeKey]; cv != nil {
		w, h := v.planeExtentMM(cv)
		return fmt.Sprintf("%s · %.0f × %.0f mm", where, w, h)
	}
	return where
}

func (v *Viewer) planeExtentMM(cv *CompositeVolume) (float64, float64) {
	vx, vy, vz := spacingOr1(cv.VoxelSizeX), spacingOr1(cv.VoxelSizeY), spacingOr1(cv.VoxelSizeZ)
	x, y, z := float64(cv.Cols)*vx, float64(cv.Rows)*vy, float64(len(cv.PixelData.Frames))*vz
	switch v.st.orientation {
	case Coronal:
		return x, z
	case Sagittal:
		return y, z
	default:
		return x, y
	}
}

func spacingOr1(s float64) float64 {
	if s <= 0 {
		return 1
	}
	return s
}
