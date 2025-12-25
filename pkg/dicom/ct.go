package dicom

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/jpfielding/goxel/pkg/dicom/module"
	"github.com/jpfielding/goxel/pkg/dicom/tag"
	"github.com/jpfielding/goxel/pkg/dicom/transfer"
)

// CTImage represents a DICOM CT Image IOD
// modeled after SDICOM::CT::CTImage
type CTImage struct {
	Patient   *module.PatientModule
	Study     *module.GeneralStudyModule
	Series    *module.GeneralSeriesModule
	Equipment *module.GeneralEquipmentModule
	SOPCommon *module.SOPCommonModule

	// New NEMA-compliant modules
	FrameOfReference *module.FrameOfReferenceModule
	ImagePlane       *module.ImagePlaneModule
	CTImageMod       *module.CTImageModule // Renamed to avoid conflict
	VOILUT           *module.VOILUTModule  // Window/level presets

	ContentDate module.Date
	ContentTime module.Time

	// Legacy: custom CT items (deprecated, use CTImageMod)
	Image     *CTImageModule
	PixelData *PixelData

	// Convenience fields (mapped to tags in Write)
	SamplesPerPixel   uint16
	PhotometricInterp string
	BitsAllocated     uint16
	BitsStored        uint16
	HighBit           uint16
	PixelRepresent    uint16
	Rows              int
	Columns           int
	RescaleIntercept interface{} // float64 or string (DS)
	RescaleSlope     interface{} // float64 or string (DS)
	RescaleType      string
	Codec            Codec // nil = uncompressed
}

// CTImageModule is a legacy simple container for CT Image module attributes
// Prefer using module.CTImageModule for new code
type CTImageModule struct {
	KV map[tag.Tag]interface{}
}

func NewCTImage() *CTImage {
	ct := &CTImage{
		Patient:          &module.PatientModule{},
		Study:            &module.GeneralStudyModule{},
		Series:           &module.GeneralSeriesModule{},
		Equipment:        &module.GeneralEquipmentModule{},
		SOPCommon:        &module.SOPCommonModule{},
		FrameOfReference: &module.FrameOfReferenceModule{},
		ImagePlane:       module.NewImagePlaneModule(),
		CTImageMod:       module.NewCTImageModule(),
		VOILUT:           module.NewVOILUTModuleForCT(),                     // CT presets
		Image:            &CTImageModule{KV: make(map[tag.Tag]interface{})}, // Legacy
	}

	// Set defaults
	ct.SamplesPerPixel = 1
	ct.PhotometricInterp = "MONOCHROME2"
	ct.BitsAllocated = 16
	ct.BitsStored = 16
	ct.HighBit = 15
	ct.PixelRepresent = 0
	ct.RescaleIntercept = 0.0
	ct.RescaleSlope = 1.0
	ct.RescaleType = "HU"

	now := time.Now()

	// Generate UIDs
	ct.Study.StudyInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	ct.Series.SeriesInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	ct.SOPCommon.SOPInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	ct.SOPCommon.SOPClassUID = "1.2.840.10008.5.1.4.1.1.2" // CT Image Storage

	ct.Study.StudyDate = module.NewDate(now)
	ct.Study.StudyTime = module.NewTime(now)
	ct.Series.SeriesDate = module.NewDate(now)
	ct.Series.SeriesTime = module.NewTime(now)
	ct.SOPCommon.InstanceCreationDate = module.NewDate(now)
	ct.SOPCommon.InstanceCreationTime = module.NewTime(now)

	ct.ContentDate = module.NewDate(now)
	ct.ContentTime = module.NewTime(now)

	// Set default transfer syntax to Explicit VR Little Endian
	// User can change this by setting pixel data with compression
	return ct
}

// GetDataset builds and returns the DICOM Dataset
func (ct *CTImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	// 1. Determine transfer syntax based on compression
	ts := string(transfer.ExplicitVRLittleEndian)
	if ct.Codec != nil {
		ts = ct.Codec.TransferSyntaxUID()
	} else if ct.PixelData != nil && ct.PixelData.IsEncapsulated {
		// Already encapsulated data - try to detect from existing transfer syntax
		ts = string(transfer.JPEGLSLossless)
	}

	// 2. File Meta Information
	opts = append(opts, WithFileMeta(ct.SOPCommon.SOPClassUID, ct.SOPCommon.SOPInstanceUID, ts))

	// 3. Add Modules
	opts = append(opts,
		WithModule(ct.Patient.ToTags()),
		WithModule(ct.Study.ToTags()),
		WithModule(ct.Series.ToTags()),
		WithModule(ct.Equipment.ToTags()),
		WithModule(ct.SOPCommon.ToTags()),
	)

	// 3b. Add optional NEMA-compliant modules
	if ct.FrameOfReference != nil {
		opts = append(opts, WithModule(ct.FrameOfReference.ToTags()))
	}
	if ct.ImagePlane != nil {
		opts = append(opts, WithModule(ct.ImagePlane.ToTags()))
	}
	if ct.CTImageMod != nil {
		opts = append(opts, WithModule(ct.CTImageMod.ToTags()))
	}
	if ct.VOILUT != nil {
		opts = append(opts, WithModule(ct.VOILUT.ToTags()))
	}

	// 4. Content Date/Time
	opts = append(opts,
		WithElement(tag.ContentDate, ct.ContentDate.String()),
		WithElement(tag.ContentTime, ct.ContentTime.String()),
	)

	// 5. Image Attributes
	opts = append(opts,
		WithElement(tag.SamplesPerPixel, ct.SamplesPerPixel),
		WithElement(tag.PhotometricInterpretation, ct.PhotometricInterp),
		WithElement(tag.BitsAllocated, ct.BitsAllocated),
		WithElement(tag.BitsStored, ct.BitsStored),
		WithElement(tag.HighBit, ct.HighBit),
		WithElement(tag.PixelRepresentation, ct.PixelRepresent),
		WithElement(tag.Rows, uint16(ct.Rows)),
		WithElement(tag.Columns, uint16(ct.Columns)),
		WithElement(tag.RescaleIntercept, ct.RescaleIntercept),
		WithElement(tag.RescaleSlope, ct.RescaleSlope),
		WithElement(tag.RescaleType, ct.RescaleType),
	)

	// 6. Legacy image KV pairs
	for t, v := range ct.Image.KV {
		if t == tag.Rows || t == tag.Columns {
			continue
		}
		opts = append(opts, WithElement(t, v))
	}

	// 7. Pixel Data
	if ct.Codec != nil && ct.PixelData != nil && !ct.PixelData.IsEncapsulated {
		flatData := ct.PixelData.GetFlatData()
		opts = append(opts, WithPixelData(ct.Rows, ct.Columns, int(ct.BitsAllocated), flatData, ct.Codec))
	} else if ct.PixelData != nil {
		opts = append(opts, WithRawPixelData(ct.PixelData))
	}

	return NewDataset(opts...)
}

// WriteTo writes the CT Image to any io.Writer
func (ct *CTImage) WriteTo(w io.Writer) (int64, error) {
	ds, err := ct.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, ds)
}

// Write writes the CT Image to a file (convenience wrapper)
func (ct *CTImage) Write(path string) (int64, error) {
	slog.Debug("Writing DICOM file", "path", path, "sop_instance_uid", ct.SOPCommon.SOPInstanceUID, "compressed", ct.PixelData != nil && ct.PixelData.IsEncapsulated)
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return ct.WriteTo(f)
}

// SetPixelData sets native pixel data
func (ct *CTImage) SetPixelData(rows, cols int, data []uint16) {
	// Update image module tags
	ct.Image.KV[tag.Rows] = uint16(rows)
	ct.Image.KV[tag.Columns] = uint16(cols)
	ct.Image.KV[tag.SamplesPerPixel] = uint16(1)
	ct.Image.KV[tag.PhotometricInterpretation] = "MONOCHROME2"
	ct.Image.KV[tag.BitsAllocated] = uint16(16)
	ct.Image.KV[tag.BitsStored] = uint16(16)
	ct.Image.KV[tag.HighBit] = uint16(15)
	ct.Image.KV[tag.PixelRepresentation] = uint16(0)

	// Create PixelData struct
	// For native, we create one frame with all data?
	// Or multiple frames if 3D?
	// Assuming single frame or multi-frame flattened.
	// If data length > rows*cols, it's multi-frame.
	pixelsPerFrame := rows * cols
	numFrames := len(data) / pixelsPerFrame
	ct.Image.KV[tag.NumberOfFrames] = fmt.Sprintf("%d", numFrames) // IS VR

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, numFrames),
	}

	for i := range numFrames {
		start := i * pixelsPerFrame
		end := start + pixelsPerFrame
		if end > len(data) {
			end = len(data)
		}

		frameData := make([]uint16, end-start)
		copy(frameData, data[start:end])
		pd.Frames[i] = Frame{Data: frameData}
	}
	ct.PixelData = pd
}
