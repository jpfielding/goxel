package goxel

import "github.com/jpfielding/goxel/pkg/volume"

// layoutClass is the responsive layout, chosen from the window's logical size
// only (never a platform flag), as roxel does.
type layoutClass int

const (
	layoutDesktop layoutClass = iota
	layoutPhonePortrait
	layoutPhoneLandscape
)

const (
	// portraitMaxWidth: narrower than this the three desktop panels cannot coexist.
	portraitMaxWidth = 640
	// landscapeMaxHeight: shorter than this the desktop shelf leaves no room for the volume.
	landscapeMaxHeight = 520
	// sidebarWidth is the fixed width of the desktop study sidebar.
	sidebarWidth = 232
	// centerMinWidth is the narrowest the desktop 3D column may get.
	centerMinWidth = 532
)

func layoutFromSize(w, h float32) layoutClass {
	switch {
	case w < portraitMaxWidth:
		return layoutPhonePortrait
	case h < landscapeMaxHeight:
		return layoutPhoneLandscape
	default:
		return layoutDesktop
	}
}

func (l layoutClass) compact() bool { return l != layoutDesktop }

// slicePanelWidth is the desktop 2D panel width: about a third of the window
// so the slice is large, but never at the cost of the 3D column.
func slicePanelWidth(window float32) float32 {
	most := max(window-sidebarWidth-centerMinWidth, 336)
	return min(max(window*0.36, 336), min(most, 520))
}

// phoneTab is a destination of the compact layouts' tab bar.
type phoneTab int

const (
	tabStudy phoneTab = iota
	tabFindings
	tabSlice
	tabView3D
)

func (t phoneTab) label() string {
	switch t {
	case tabStudy:
		return "Study"
	case tabFindings:
		return "Findings"
	case tabSlice:
		return "2D slice"
	default:
		return "3D view"
	}
}

func (t phoneTab) isView() bool { return t == tabSlice || t == tabView3D }

// volumePage / slicePage are the pages of the compact controls sheets.
type volumePage int

const (
	pageCamera volumePage = iota
	pageSlicePlane
	pageWindowing
	pageDisplay
	pageLighting
)

var volumePageLabels = []string{"Camera", "Slice", "Windowing", "Display", "Lighting"}

type slicePage int

const (
	pageSlice slicePage = iota
	pageImage
	pageVolume
)

var slicePageLabels = []string{"Slice", "Image", "Volume"}

// palettePreset is the 3D colour scheme.
type palettePreset int

const (
	paletteMaterials palettePreset = iota
	paletteThreat
	paletteGrayscale
)

var paletteLabels = []string{"Materials", "Threat", "Grayscale"}

// configName maps the palette onto goxel's volume config presets.
func (p palettePreset) configName() string {
	switch p {
	case paletteThreat:
		return "FINDING"
	case paletteGrayscale:
		return "MONOCHROME"
	default:
		return "DEFAULT"
	}
}

// renderQuality maps onto the GPU ray step and the CPU quality mode.
type renderQuality int

const (
	qualityFast renderQuality = iota
	qualityBalanced
	qualityHigh
)

var qualityLabels = []string{"Fast", "Balanced", "High"}

func (q renderQuality) stepSize() float64 {
	switch q {
	case qualityFast:
		return 0.004
	case qualityBalanced:
		return 0.002
	default:
		return 0.001
	}
}

var orientationLabels = []string{"Axial", "Coronal", "Sagittal"}

// uiState is every piece of UI state as plain data. Controls write to it and
// call an apply function; Viewer.rebuild builds the widget tree from it and
// runs only when the layout class, focus, tab, page or loaded study changes.
type uiState struct {
	layout layoutClass

	focusVolume bool
	focusSlice  bool

	phoneTab        phoneTab
	sheetOpen       bool
	focusAdjustOpen bool
	volumePage      volumePage
	slicePage       slicePage
	bandsOpen       bool
	selectedBand    int
	dragPans        bool

	showSlicePlane bool
	cutaway        bool
	reverseCutaway bool

	preset  palettePreset
	bands   []volume.ColorBand
	quality renderQuality

	windowCenter3D float64
	windowWidth3D  float64
	opacity        float64
	density        float64
	ambient        float64
	diffuse        float64
	specular       float64
	depthCue       float64

	orientation    ViewOrientation
	sliceIndex     int
	composite      bool
	zoom2D         float64 // percent, 50-500
	windowCenter2D float64
	windowWidth2D  float64

	loadedName  string
	loadedPath  string
	loadingFile string
	loadError   string
}

func newUIState() uiState {
	return uiState{
		layout:      layoutDesktop,
		phoneTab:    tabView3D,
		sheetOpen:   true,
		bands:       volume.DefaultColorBands(),
		quality:     qualityHigh,
		opacity:     0.5,
		density:     float64(volume.DefaultColorBands()[0].Threshold) / 30000,
		ambient:     0.55,
		diffuse:     0.65,
		specular:    0.4,
		orientation: Axial,
		zoom2D:      100,
	}
}

// selectTab applies a tap on a compact tab: another tab opens with its
// controls; the active view tab toggles its controls sheet.
func (s *uiState) selectTab(t phoneTab) {
	if t == s.phoneTab {
		if t.isView() {
			s.sheetOpen = !s.sheetOpen
		}
		return
	}
	s.phoneTab = t
	s.sheetOpen = true
}

// exitFocus leaves whichever focus mode is active; it reports whether one was.
func (s *uiState) exitFocus() bool {
	switch {
	case s.focusSlice:
		s.focusSlice = false
	case s.focusVolume:
		s.focusVolume = false
	default:
		return false
	}
	s.focusAdjustOpen = false
	return true
}

// cutawayPlane returns the clip plane (normal, d) in texture space for the
// current 2D orientation and slice: dot(p, n) + d < 0 is hidden.
func cutawayPlane(o ViewOrientation, slice, count int, reverse bool) (nx, ny, nz, d float32) {
	if count <= 0 {
		count = 1
	}
	frac := (float32(slice) + 0.5) / float32(count)
	switch o {
	case Coronal:
		ny = 1
	case Sagittal:
		nx = 1
	default:
		nz = 1
	}
	d = -frac
	if reverse {
		nx, ny, nz, d = -nx, -ny, -nz, frac
	}
	return nx, ny, nz, d
}
