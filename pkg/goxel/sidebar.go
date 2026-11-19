package goxel

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
)

// sidebar is the desktop left column: Study, Details, Volumes, Findings and
// the DICOM metadata, collapsed, at the bottom.
func (v *Viewer) sidebar() fyne.CanvasObject {
	items := []fyne.CanvasObject{v.studySection(), rule(), v.detailsSection()}
	if len(v.volumeKeys()) > 1 {
		items = append(items, rule(), v.volumesSection())
	}
	items = append(items, rule(), v.findingsSection(), rule(), v.metadataSection())
	body := container.NewVScroll(inset(16, 16, vstack(8, items...)))
	return border(nil, nil, nil, hairline(), panel(colPanel, 0, body))
}

// studySection: the "Open now" card, open actions and load status.
func (v *Viewer) studySection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Study", "")}
	if v.loaded() {
		label := text("Open now", 12, colAccent)
		name := text(elideMiddle(v.st.loadedName, 30), bodySize(), colHeading)
		edge := rrect(colAccent, color.Transparent, 3, 0)
		edge.SetMinSize(fyne.NewSize(4, 1))
		body := container.New(marginLayout{14, 8}, vstack(2, leftCentre(label), leftCentre(name)))
		items = append(items, container.NewStack(rrect(colOpenCard, color.Transparent, 6, 0),
			border(nil, nil, edge, nil, nil), body))
	} else {
		items = append(items, note("No study open"))
	}
	items = append(items, v.openButtons(false), v.loadStatus())
	return vstack(8, items...)
}

// detailsSection: voxels, physical extent, modality and volume count.
func (v *Viewer) detailsSection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Details", "")}
	p := v.primary
	if p == nil {
		return vstack(8, append(items, note("—"))...)
	}
	ex, ey, ez := float64(p.Width)*spacingOr1(p.VoxelSizeX), float64(p.Height)*spacingOr1(p.VoxelSizeY),
		float64(p.Depth)*spacingOr1(p.VoxelSizeZ)
	modality := v.scan.Modality
	if modality == "" {
		modality = "—"
	}
	rows := vstack(2,
		keyValue("Voxels", fmt.Sprintf("%d × %d × %d", p.Width, p.Height, p.Depth)),
		keyValue("Extent", fmt.Sprintf("%.0f × %.0f × %.0f mm", ex, ey, ez)),
		keyValue("Modality", modality),
		keyValue("Volumes", fmt.Sprintf("%d", len(v.volumeKeys()))),
	)
	return vstack(8, append(items, rows)...)
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
		alpha := newSliderRow(fraction("Opacity", 0, 1), vol.Alpha,
			func(x float64) {
				vol.Alpha = x
				v.debounced("layers", 80*time.Millisecond, v.setup3DView)
			}, nil)
		items = append(items, check, alpha.obj)
	}
	return vstack(6, items...)
}

// findingsSection: count and rows, or roxel's empty sentence.
func (v *Viewer) findingsSection() fyne.CanvasObject {
	n := v.findingCount()
	count := ""
	if n > 0 {
		count = fmt.Sprintf("%d reported", n)
	}
	head := sectionTitle("Findings", count)
	if n == 0 {
		return vstack(8, head, bodyNote("No findings were loaded with this study."))
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
		items = append(items, flexRow(8, 1, container.NewCenter(swatch), check))
	}
	return vstack(6, items...)
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
		return vstack(0)
	}
	title := "DICOM metadata"
	head := newTapArea(hrow(8, chevron(v.st.metadataOpen), text(title, 14.5, colHeading)), func() {
		v.st.metadataOpen = !v.st.metadataOpen
		v.rebuild()
	})
	if !v.st.metadataOpen {
		return minHeight(24, head)
	}
	rows := vstack(2)
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
		rows.Add(keyValue(kv[0], elideMiddle(val, 18)))
	}
	return vstack(8, minHeight(24, head), rows)
}
