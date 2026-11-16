package goxel

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/widget"
)

// sidebar is the desktop left column: Study, Details, Volumes, Findings and
// the DICOM metadata, collapsed, at the bottom.
func (v *Viewer) sidebar() fyne.CanvasObject {
	items := []fyne.CanvasObject{v.studySection(), hairline(), v.detailsSection()}
	if len(v.volumeKeys()) > 1 {
		items = append(items, hairline(), v.volumesSection())
	}
	items = append(items, hairline(), v.findingsSection(), hairline(), v.metadataSection())
	body := container.NewVScroll(inset(12, 10, container.NewVBox(items...)))
	edge := filled(colHairline)
	edge.SetMinSize(fyne.NewSize(1, 1))
	return container.NewStack(filled(colPanel), container.NewBorder(nil, nil, nil, edge, body))
}

// studySection: the "Open now" card, open actions and load status.
func (v *Viewer) studySection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Study", "")}
	if v.loaded() {
		label := text("Open now", 12, colMuted, false)
		name := widget.NewLabel(elideMiddle(v.st.loadedName, 30))
		name.Truncation = fyne.TextTruncateEllipsis
		edge := filled(colAccent)
		edge.SetMinSize(fyne.NewSize(4, 1))
		card := container.NewStack(filled(colOpenCard),
			container.NewBorder(nil, nil, edge, nil, inset(8, 4, container.NewVBox(label, name))))
		items = append(items, card)
	} else {
		items = append(items, note("No study open"))
	}
	items = append(items, v.openButtons(false), v.loadStatus())
	return container.NewVBox(items...)
}

// detailsSection: voxels, physical extent, modality and volume count.
func (v *Viewer) detailsSection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Details", "")}
	p := v.primary
	if p == nil {
		return container.NewVBox(append(items, note("—"))...)
	}
	ex, ey, ez := float64(p.Width)*spacingOr1(p.VoxelSizeX), float64(p.Height)*spacingOr1(p.VoxelSizeY),
		float64(p.Depth)*spacingOr1(p.VoxelSizeZ)
	modality := v.scan.Modality
	if modality == "" {
		modality = "—"
	}
	items = append(items,
		keyValue("Voxels", fmt.Sprintf("%d × %d × %d", p.Width, p.Height, p.Depth)),
		keyValue("Extent (mm)", fmt.Sprintf("%.0f × %.0f × %.0f", ex, ey, ez)),
		keyValue("Modality", modality),
		keyValue("Volumes", fmt.Sprintf("%d", len(v.volumeKeys()))),
	)
	return container.NewVBox(items...)
}

// volumesSection: one row per 3D volume — visibility and opacity.
func (v *Viewer) volumesSection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Volumes", "")}
	for _, k := range v.volumeKeys() {
		vol := v.compositeVolumes[k]
		check := newCheck(elideMiddle(k, 24), vol.Enabled, func(b bool) {
			vol.Enabled = b
			v.setup3DView()
		})
		alpha := newSliderRow(v.st.layout.compact(), "Opacity", 0, 1, 0.05,
			func(x float64) string { return fmt.Sprintf("%.2f", x) }, vol.Alpha,
			func(x float64) {
				vol.Alpha = x
				v.debounced("layers", 80*time.Millisecond, v.setup3DView)
			}, nil)
		items = append(items, check, alpha.obj)
	}
	return container.NewVBox(items...)
}

// findingsSection: count and rows, or roxel's empty sentence.
func (v *Viewer) findingsSection() fyne.CanvasObject {
	n := v.findingCount()
	head := container.NewBorder(nil, nil, sectionTitle("Findings", ""), nil,
		container.NewHBox(layout.NewSpacer(), container.NewCenter(text(fmt.Sprintf("%d reported", n), 12, colMuted, false))))
	if n == 0 {
		return container.NewVBox(head, note("No findings were loaded with this study."))
	}
	items := []fyne.CanvasObject{head}
	var keys []string
	for _, k := range v.sortedKeys() {
		if strings.HasPrefix(k, "finding_") {
			keys = append(keys, k)
		}
	}
	allOn := true
	for _, k := range keys {
		allOn = allOn && v.compositeVolumes[k].Enabled
	}
	show := newCheck("Show finding boxes", allOn, func(b bool) {
		for _, k := range keys {
			v.compositeVolumes[k].Enabled = b
		}
		v.setup3DView()
		v.rebuild()
	})
	items = append(items, show)
	for _, k := range keys {
		vol := v.compositeVolumes[k]
		swatch := filled(vol.Color)
		swatch.SetMinSize(fyne.NewSize(12, 12))
		check := newCheck(strings.TrimPrefix(k, "finding_"), vol.Enabled, func(b bool) {
			vol.Enabled = b
			v.setup3DView()
		})
		items = append(items, container.NewBorder(nil, nil, container.NewCenter(swatch), nil, check))
	}
	return container.NewVBox(items...)
}

func (v *Viewer) sortedKeys() []string {
	keys := make([]string, 0, len(v.compositeVolumes))
	for k := range v.compositeVolumes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// metadataSection keeps goxel's DICOM fields, collapsed by default.
func (v *Viewer) metadataSection() fyne.CanvasObject {
	s := v.scan
	if s == nil {
		return layout.NewSpacer()
	}
	form := container.NewVBox()
	for _, kv := range [][2]string{
		{"Patient ID", s.PatientID}, {"Patient name", s.PatientName},
		{"Study date", s.StudyDate}, {"Study time", s.StudyTime},
		{"Study", s.StudyDescription}, {"Series", s.SeriesDescription},
		{"Institution", s.InstitutionName}, {"Manufacturer", s.Manufacturer},
		{"Operator", s.OperatorName},
	} {
		val := kv[1]
		if val == "" {
			val = "—"
		}
		form.Add(keyValue(kv[0], val))
	}
	acc := widget.NewAccordion(widget.NewAccordionItem("DICOM metadata", form))
	return acc
}
