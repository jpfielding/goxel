package dicom

import (
	"fmt"
	"io"
	"os"
	"time"

	"github.com/jpfielding/goxel/pkg/dicom/module"
	"github.com/jpfielding/goxel/pkg/dicom/tag"
	"github.com/jpfielding/goxel/pkg/dicom/transfer"
)

// DXImage represents a DICOM Digital X-Ray Image IOD
type DXImage struct {
	// Modules
	Patient     module.PatientModule
	Study       module.GeneralStudyModule
	Series      module.GeneralSeriesModule // Specializes to DXSeries
	Equipment   module.GeneralEquipmentModule
	SOPCommon   module.SOPCommonModule
	VOILUT      *module.VOILUTModule        // Window/level presets
	Detector    *module.DXDetectorModule    // Detector parameters
	Acquisition *module.DXAcquisitionModule // X-ray acquisition parameters

	// Image Attributes
	InstanceNumber    int
	ContentDate       module.Date
	ContentTime       module.Time
	ImageType         string // ORIGINAL\PRIMARY
	SamplesPerPixel   int
	PhotometricInterp string // MONOCHROME2
	Rows              int
	Columns           int
	BitsAllocated     int
	BitsStored        int
	HighBit           int
	PixelRepresent    int // 0 unsigned, 1 signed

	// Windowing (legacy - prefer VOILUT module)
	WindowCenter float64
	WindowWidth  float64

	// DX Specifics
	PresentationIntentType string // PRESENTATION or PROCESSING

	// Pixel Data
	PixelData []uint16
	Codec     Codec // nil = uncompressed

	// Additional Tags (Generic support for tags not explicitly defined)
	AdditionalTags map[tag.Tag]interface{}
}

// NewDXImage creates a new DX Image with default values
func NewDXImage() *DXImage {
	t := time.Now()
	return &DXImage{
		SamplesPerPixel:        1,
		PhotometricInterp:      "MONOCHROME2",
		BitsAllocated:          16,
		BitsStored:             16,
		HighBit:                15,
		PixelRepresent:         0,
		PresentationIntentType: "PRESENTATION",
		WindowCenter:           32768, // Center for 16-bit
		WindowWidth:            65535, // Full width
		ContentDate:            module.NewDate(t),
		ContentTime:            module.NewTime(t),
		Study:                  module.NewGeneralStudyModule(),
		SOPCommon:              module.NewSOPCommonModule(),
		VOILUT:                 module.NewVOILUTModuleForDX(),
		Detector:               module.NewDXDetectorModule(),
		Acquisition:            module.NewDXAcquisitionModule(),
		AdditionalTags:         make(map[tag.Tag]interface{}),
	}
}

// SetPixelData sets the raw pixel data
func (dx *DXImage) SetPixelData(rows, cols int, data []uint16) {
	dx.Rows = rows
	dx.Columns = cols
	dx.PixelData = data
}

// GetDataset builds and returns the DICOM Dataset
func (dx *DXImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	// 1. File Meta Information
	tsUID := string(transfer.ExplicitVRLittleEndian)
	if dx.Codec != nil {
		tsUID = dx.Codec.TransferSyntaxUID()
	}

	sopInstanceUID := dx.SOPCommon.SOPInstanceUID
	if sopInstanceUID == "" {
		sopInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
		dx.SOPCommon.SOPInstanceUID = sopInstanceUID
	}
	dx.SOPCommon.SOPClassUID = DXImageStorageUID
	if dx.Study.StudyInstanceUID == "" {
		dx.Study.StudyInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	}

	// DX Storage
	opts = append(opts, WithFileMeta(DXImageStorageUID, sopInstanceUID, tsUID))

	// 2. Modules
	opts = append(opts,
		WithModule(dx.Patient.ToTags()),
		WithModule(dx.Study.ToTags()),
		WithModule(dx.Series.ToTags()),
		WithModule(dx.Equipment.ToTags()),
		WithModule(dx.SOPCommon.ToTags()),
	)
	if dx.Detector != nil {
		opts = append(opts, WithModule(dx.Detector.ToTags()))
	}
	if dx.Acquisition != nil {
		opts = append(opts, WithModule(dx.Acquisition.ToTags()))
	}

	// 3. Image Pixel Module & Common
	opts = append(opts,
		WithElement(tag.Rows, dx.Rows),
		WithElement(tag.Columns, dx.Columns),
		WithElement(tag.BitsAllocated, dx.BitsAllocated),
		WithElement(tag.BitsStored, dx.BitsStored),
		WithElement(tag.HighBit, dx.HighBit),
		WithElement(tag.PixelRepresentation, dx.PixelRepresent),
		WithElement(tag.SamplesPerPixel, dx.SamplesPerPixel),
		WithElement(tag.PhotometricInterpretation, dx.PhotometricInterp),
		WithElement(tag.ImageType, dx.ImageType),
		WithElement(tag.ContentDate, dx.ContentDate.String()),
		WithElement(tag.ContentTime, dx.ContentTime.String()),
		WithElement(tag.InstanceNumber, fmt.Sprintf("%d", dx.InstanceNumber)),
		WithElement(tag.PresentationIntentType, dx.PresentationIntentType),
		WithElement(tag.WindowCenter, fmt.Sprintf("%v", dx.WindowCenter)),
		WithElement(tag.WindowWidth, fmt.Sprintf("%v", dx.WindowWidth)),
	)

	// Additional Tags
	for t, v := range dx.AdditionalTags {
		opts = append(opts, WithElement(t, v))
	}

	// 4. Pixel Data
	opts = append(opts, WithPixelData(dx.Rows, dx.Columns, dx.BitsAllocated, dx.PixelData, dx.Codec))

	return NewDataset(opts...)
}

// WriteTo writes the DX Image to any io.Writer
func (dx *DXImage) WriteTo(w io.Writer) (int64, error) {
	dataset, err := dx.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, dataset)
}

// Write saves the DX Image to a DICOM file (convenience wrapper)
func (dx *DXImage) Write(path string) (int64, error) {
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return dx.WriteTo(f)
}
