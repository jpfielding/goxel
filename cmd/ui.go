package main

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/jpfielding/goxel/pkg/dicos"
	"github.com/suyashkumar/dicom"
	"github.com/suyashkumar/dicom/pkg/frame"
	"github.com/suyashkumar/dicom/pkg/tag"
)

var (
	presetNames = []string{
		"Abdomen",
		"Bone",
		"Brain",
		"Lungs",
		"Mediastinum",
	}

	presetValues = map[string]struct{ level, width int }{
		"Abdomen":     {40, 400},
		"Bone":        {400, 1800},
		"Brain":       {40, 80},
		"Lungs":       {600, 1500},
		"Mediastinum": {50, 350},
	}
)

type viewer struct {
	dicom                  *DICOMImage
	frames                 []*frame.Frame
	currentFrame           int
	image                  *canvas.Image
	study, name, id, frame *widget.Label
	level, width           *widget.Entry

	win fyne.Window
}

func (v *viewer) loadDir(dir fyne.ListableURI) {
	var (
		data   dicom.Dataset
		frames []*frame.Frame
	)

	files, _ := dir.List()
	for i, file := range files {
		r, _ := storage.Reader(file)
		d, err := dicom.Parse(r, fileLength(file.Path()), nil)
		if i == 0 {
			if err != nil {
				slog.Error("first file in dir was not DICOM", slog.Any("error", err))
				return
			}
			data = d
		}
		if err != nil {
			slog.Error("could not open dicom file "+file.Name()+" in folder", slog.Any("error", err))
			continue
		}

		t, err := d.FindElementByTag(tag.PixelData)
		if err == nil {
			frames = append(frames, t.Value.GetValue().(dicom.PixelDataInfo).Frames...)
		}
		_ = r.Close()
	}

	t, err := data.FindElementByTag(tag.PixelData)
	if err == nil {
		info := t.Value.GetValue().(dicom.PixelDataInfo)
		info.Frames = frames
		v, _ := dicom.NewValue(info)
		t.Value = v
	}

	v.loadImage(data)
}

func (v *viewer) loadFile(r io.ReadCloser, length int64) {
	data, err := dicom.Parse(r, length, nil)
	if err != nil {
		dialog.ShowError(err, v.win)
		return
	}
	defer r.Close()
	v.loadImage(data)
}

func (v *viewer) loadImage(data dicom.Dataset) {
	for _, elem := range data.Elements {
		switch elem.Tag {
		case tag.PixelData:
			v.frames = elem.Value.GetValue().(dicom.PixelDataInfo).Frames

			if len(v.frames) == 0 {
				panic("No images found")
			}
			v.setFrame(0)
		case tag.PatientName:
			v.name.SetText(fmt.Sprintf("%v", elem.Value))
		case tag.PatientID:
			v.id.SetText(fmt.Sprintf("%v", elem.Value))
		case tag.StudyDescription:
			v.study.SetText(fmt.Sprintf("%v", elem.Value))
		case tag.WindowCenter:
			str := fmt.Sprintf("%v", elem.Value.GetValue().([]string)[0])
			l, _ := strconv.Atoi(str)
			v.dicom.SetWindowLevel(int16(l))
			v.level.SetText(str)
		case tag.WindowWidth:
			str := fmt.Sprintf("%v", elem.Value.GetValue().([]string)[0])
			l, _ := strconv.Atoi(str)
			v.dicom.SetWindowWidth(int16(l))
			v.width.SetText(str)
		default:
			t := elem.Tag
			key := t.String()
			if ltn := dicos.LookupTagName(t.Group, t.Element); ltn != "" {
				key = ltn
			}
			switch elem.Value.ValueType() {
			case dicom.Strings:
			// Bytes represents an underlying value of []byte
			case dicom.Bytes:
			// Ints represents an underlying value of []int
			case dicom.Ints:
				// PixelData represents an underlying value of PixelDataInfo
			case dicom.PixelData:
				// SequenceItem represents an underlying value of []*Element
			case dicom.SequenceItem:
				// Sequences represents an underlying value of []SequenceItem
			case dicom.Sequences:
				// Floats represents an underlying value of []float64
			case dicom.Floats:
			default:
				slog.Info("tag", slog.Any(key, elem.Value.String()))
			}
		}
	}
}

func (v *viewer) loadKeys() {
	v.win.Canvas().SetOnTypedKey(func(key *fyne.KeyEvent) {
		switch key.Name {
		case fyne.KeyUp:
			v.nextFrame()
		case fyne.KeyDown:
			v.previousFrame()
		case fyne.KeyF:
			v.fullScreen()
		}
	})
}

func (v *viewer) nextFrame() {
	v.setFrame(v.currentFrame + 1)
}

func (v *viewer) previousFrame() {
	v.setFrame(v.currentFrame - 1)
}

func (v *viewer) setFrame(id int) {
	count := len(v.frames)
	if id > count-1 {
		id = 0
	} else if id < 0 {
		id = count - 1
	}
	v.currentFrame = id

	v.dicom.SetFrame(&v.frames[id].NativeData)
	canvas.Refresh(v.image)
	v.frame.SetText(fmt.Sprintf("%d/%d", id+1, count))
}

func fileLength(path string) int64 {
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return 0
	}

	return info.Size()
}

func (v *viewer) fullScreen() {
	v.win.SetFullScreen(!v.win.FullScreen())
}

func (v *viewer) openFile() {
	d := dialog.NewFileOpen(func(f fyne.URIReadCloser, err error) {
		if f == nil || err != nil {
			return
		}

		v.loadFile(f, fileLength(f.URI().Path())) // TODO work with library upstream to not do this
	}, v.win)
	d.SetFilter(storage.NewExtensionFileFilter([]string{".dcm", ".dcs"}))
	wd, _ := os.Getwd()
	here, _ := storage.ListerForURI(storage.NewFileURI(wd))
	d.SetLocation(here)
	d.Show()
}

func (v *viewer) openFolder() {
	d := dialog.NewFolderOpen(func(f fyne.ListableURI, err error) {
		if f == nil || err != nil {
			return
		}

		v.loadDir(f)
	}, v.win)
	wd, _ := os.Getwd()
	here, _ := storage.ListerForURI(storage.NewFileURI(wd))
	d.SetLocation(here)
	d.Show()
}
func (v *viewer) setupForm(dicomImg *DICOMImage, img *canvas.Image) fyne.CanvasObject {
	values := widget.NewForm()

	v.id = widget.NewLabel("anon")
	values.Append("ID", v.id)
	v.name = widget.NewLabel("anon")
	values.Append("Name", v.name)
	v.study = widget.NewLabel("ANON")
	values.Append("Study", v.study)

	v.level = widget.NewEntry()
	v.level.SetText(fmt.Sprintf("%d", dicomImg.WindowLevel()))
	v.level.OnChanged = func(val string) {
		l, _ := strconv.Atoi(val)
		dicomImg.SetWindowLevel(int16(l))

		canvas.Refresh(img)
	}

	v.width = widget.NewEntry()
	v.width.SetText(fmt.Sprintf("%d", dicomImg.WindowWidth()))
	v.width.OnChanged = func(val string) {
		w, _ := strconv.Atoi(val)
		dicomImg.SetWindowWidth(int16(w))

		canvas.Refresh(img)
	}

	presets := widget.NewSelect(presetNames, func(name string) {
		val := presetValues[name]
		v.level.SetText(strconv.Itoa(val.level))
		v.width.SetText(strconv.Itoa(val.width))
	})
	return container.NewVBox(values, widget.NewCard("Window", "", widget.NewForm(
		widget.NewFormItem("Level", v.level),
		widget.NewFormItem("Width", v.width),
		widget.NewFormItem("Preset", presets))))
}

func (v *viewer) setupNavigation() []fyne.CanvasObject {
	next := widget.NewButtonWithIcon("", theme.MoveUpIcon(), func() {
		v.nextFrame()
	})
	prev := widget.NewButtonWithIcon("", theme.MoveDownIcon(), func() {
		v.previousFrame()
	})
	full := widget.NewButtonWithIcon("Full Screen", theme.ViewFullScreenIcon(), func() {
		v.fullScreen()
	})

	v.frame = widget.NewLabel("1/1")
	return []fyne.CanvasObject{
		container.NewGridWithColumns(1, next, container.NewCenter(
			widget.NewForm(&widget.FormItem{Text: "Slice", Widget: v.frame})),
			prev),
		layout.NewSpacer(),
		full,
	}
}

func makeUI(a fyne.App) *viewer {
	win := a.NewWindow("DICOM Viewer")
	dicomImg := NewDICOMImage(nil, 40, 380)

	img := canvas.NewImageFromImage(dicomImg)
	img.FillMode = canvas.ImageFillContain

	view := &viewer{dicom: dicomImg, image: img, win: win}
	form := view.setupForm(dicomImg, img)
	items := []fyne.CanvasObject{view.makeToolbar(), form}
	items = append(items, view.setupNavigation()...)
	bar := container.NewVBox(items...)

	win.SetContent(container.NewBorder(nil, nil, bar, nil, img))
	win.Resize(fyne.NewSize(600, 400))

	return view
}
