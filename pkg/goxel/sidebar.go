package goxel

import (
	"fmt"
	"image/color"

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
	return edged(panel(colPanel, 0, body), edgeRight)
}

// studySection: the "Open now" card, open actions and load status.
func (v *Viewer) studySection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Study", "")}
	if v.loaded() {
		label := text("Open now", 12, colAccent)
		// The whole name, wrapped: nothing is cut off, on any screen.
		name := newWrapped(v.st.loadedName, bodySize(), colHeading)
		edge := rrect(colAccent, color.Transparent, 3, 0)
		edge.SetMinSize(fyne.NewSize(4, 1))
		body := container.New(insetsLayout{left: 14, right: 10, top: 8, bottom: 9},
			vstack(2, minHeight(eguiRowHeight(12), leftCentre(label)), name))
		items = append(items, container.NewStack(rrect(colOpenCard, color.Transparent, 6, 0),
			border(nil, nil, edge, nil, nil), body))
	} else {
		items = append(items, note("No study open"))
	}
	items = append(items, v.openButtons(false))
	if st := v.loadStatus(); st != nil {
		items = append(items, st)
	}
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

// volumesSection picks the volume shown in both the 3D view and the 2D slice,
// one at a time, as roxel does.
func (v *Viewer) volumesSection() fyne.CanvasObject {
	items := []fyne.CanvasObject{sectionTitle("Volumes", "")}
	for _, k := range v.volumeKeys() {
		b := newButton(elideMiddle(k, 24), func() {
			if k != v.active3D {
				v.showVolume(k, false)
				v.rebuild()
			}
		})
		b.selected = k == v.active3D
		b.flat = true
		items = append(items, b)
	}
	return vstack(8, items...)
}

// findingsSection: heading with the count, the controls that act on every
// finding, then one selectable row per finding; or roxel's one sentence saying
// nothing was loaded.
func (v *Viewer) findingsSection() fyne.CanvasObject {
	summary, rows := v.findingsSummary(), v.findingsList()
	if rows == nil {
		return summary
	}
	return vstack(8, summary, rows)
}

// findingsSummary is the heading with the count, then either the sentence
// saying no findings were loaded or the controls that act on every finding.
func (v *Viewer) findingsSummary() fyne.CanvasObject {
	n := v.findingCount()
	count := ""
	if n > 0 {
		count = fmt.Sprintf("%d reported", n)
	}
	head := sectionTitle("Findings", count)
	if n == 0 {
		// Says what is missing; never that the scan is clear.
		return vstack(8, head, bodyNote("No findings were loaded with this study."))
	}
	show := newCheck("Show finding boxes", v.st.showFindingBoxes, func(b bool) {
		v.st.showFindingBoxes = b
		v.syncFindings()
	}).withTip("Draw a box around each finding in the 3D view")
	all := newButton("Show all", func() { v.showFindings(func(int) bool { return true }) })
	none := newButton("Hide all", func() { v.showFindings(func(int) bool { return false }) })
	selected := v.selectedFinding()
	only := newButton("Show only the selected one", func() {
		v.showFindings(func(i int) bool { return i == selected })
	})
	if selected < 0 {
		only.Disable() // always present, so the list never jumps
	}
	return vstack(8, head, show, buttonRow(all, none), only)
}

// findingsList is one row per finding: a box to show or hide it, and the row
// itself to select it. It is nil when there are none.
func (v *Viewer) findingsList() fyne.CanvasObject {
	if v.findingCount() == 0 {
		return nil
	}
	selected := v.selectedFinding()
	rows := make([]fyne.CanvasObject, 0, v.findingCount())
	for i, f := range v.scan.Findings {
		check := newCheck("", f.Visible, func(b bool) {
			f.Visible = b
			v.syncFindings()
		}).withTip("Show this finding")
		conf := ""
		if f.Confidence > 0 {
			conf = percent(f.Confidence)
		}
		row := newListRow(f.Name, conf, f.Color, i == selected, func() {
			v.st.selectedFinding = i
			v.rebuild()
		})
		rows = append(rows, minHeight(max(28, controlHeight()), flexRow(8, 1, check, row)))
	}
	return vstack(8, rows...)
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
		return minHeight(28, head)
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
	return vstack(8, minHeight(28, head), rows)
}
