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

// MRImage represents a DICOM MR Image IOD
// Supports multi-frame MR images
type MRImage struct {
	Patient   *module.PatientModule
	Study     *module.GeneralStudyModule
	Series    *module.GeneralSeriesModule
	Equipment *module.GeneralEquipmentModule
	SOPCommon *module.SOPCommonModule

	// MR-specific modules
	FrameOfReference *module.FrameOfReferenceModule
	ImagePlane       *module.ImagePlaneModule
	MRImageMod       *module.MRImageModule
	VOILUT           *module.VOILUTModule

	ContentDate module.Date
	ContentTime module.Time

	// Image attributes
	SamplesPerPixel   uint16
	PhotometricInterp string
	BitsAllocated     uint16
	BitsStored        uint16
	HighBit           uint16
	PixelRepresent    uint16
	Rows              int
	Columns           int
	NumberOfFrames    int

	// Pixel Data
	PixelData *PixelData

	// Compression settings
	UseCompression   bool
	CompressionCodec string // "jpeg-ls" (default), "jpeg-li", "rle"
}

// NewMRImage creates a new MR Image IOD with default values
func NewMRImage() *MRImage {
	mr := &MRImage{
		Patient:          &module.PatientModule{},
		Study:            &module.GeneralStudyModule{},
		Series:           &module.GeneralSeriesModule{Modality: "MR"},
		Equipment:        &module.GeneralEquipmentModule{},
		SOPCommon:        &module.SOPCommonModule{},
		FrameOfReference: &module.FrameOfReferenceModule{},
		ImagePlane:       module.NewImagePlaneModule(),
		MRImageMod:       module.NewMRImageModule(),
		VOILUT:           &module.VOILUTModule{},
	}

	// Set defaults
	mr.SamplesPerPixel = 1
	mr.PhotometricInterp = "MONOCHROME2"
	mr.BitsAllocated = 16
	mr.BitsStored = 16
	mr.HighBit = 15
	mr.PixelRepresent = 0 // unsigned

	now := time.Now()

	// Generate UIDs
	mr.Study.StudyInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	mr.Series.SeriesInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	mr.SOPCommon.SOPInstanceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")
	mr.SOPCommon.SOPClassUID = "1.2.840.10008.5.1.4.1.1.4" // MR Image Storage
	mr.FrameOfReference.FrameOfReferenceUID = GenerateUID("1.2.826.0.1.3680043.8.498.")

	mr.Study.StudyDate = module.NewDate(now)
	mr.Study.StudyTime = module.NewTime(now)
	mr.Series.SeriesDate = module.NewDate(now)
	mr.Series.SeriesTime = module.NewTime(now)
	mr.SOPCommon.InstanceCreationDate = module.NewDate(now)
	mr.SOPCommon.InstanceCreationTime = module.NewTime(now)

	mr.ContentDate = module.NewDate(now)
	mr.ContentTime = module.NewTime(now)

	return mr
}

// SetPixelData sets multi-frame pixel data
func (mr *MRImage) SetPixelData(rows, cols int, data []uint16) {
	mr.Rows = rows
	mr.Columns = cols

	pixelsPerFrame := rows * cols
	numFrames := len(data) / pixelsPerFrame
	mr.NumberOfFrames = numFrames

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, numFrames),
	}

	for i := 0; i < numFrames; i++ {
		start := i * pixelsPerFrame
		end := start + pixelsPerFrame
		if end > len(data) {
			end = len(data)
		}

		frameData := make([]uint16, end-start)
		copy(frameData, data[start:end])
		pd.Frames[i] = Frame{Data: frameData}
	}
	mr.PixelData = pd
}

// GetDataset builds and returns the DICOM Dataset
func (mr *MRImage) GetDataset() (*Dataset, error) {
	opts := make([]Option, 0, 32)

	// 1. Determine transfer syntax based on compression
	ts := string(transfer.ExplicitVRLittleEndian)
	willCompress := mr.UseCompression || (mr.PixelData != nil && mr.PixelData.IsEncapsulated)
	if willCompress {
		switch mr.CompressionCodec {
		case "jpeg-li":
			ts = "1.2.840.10008.1.2.4.70" // JPEG Lossless Process 14 SV1
		case "rle":
			ts = "1.2.840.10008.1.2.5" // RLE Lossless
		case "jpeg-2000", "jpeg2000":
			ts = "1.2.840.10008.1.2.4.90" // JPEG 2000 Lossless Only
		default:
			ts = string(transfer.JPEGLSLossless)
		}
	}

	// 2. File Meta Information
	opts = append(opts, WithFileMeta(mr.SOPCommon.SOPClassUID, mr.SOPCommon.SOPInstanceUID, ts))

	// 3. Add Modules
	opts = append(opts,
		WithModule(mr.Patient.ToTags()),
		WithModule(mr.Study.ToTags()),
		WithModule(mr.Series.ToTags()),
		WithModule(mr.Equipment.ToTags()),
		WithModule(mr.SOPCommon.ToTags()),
	)

	// 3b. Add MR-specific modules
	if mr.FrameOfReference != nil {
		opts = append(opts, WithModule(mr.FrameOfReference.ToTags()))
	}
	if mr.ImagePlane != nil {
		opts = append(opts, WithModule(mr.ImagePlane.ToTags()))
	}
	if mr.MRImageMod != nil {
		opts = append(opts, WithModule(mr.MRImageMod.ToTags()))
	}
	if mr.VOILUT != nil && len(mr.VOILUT.Windows) > 0 {
		opts = append(opts, WithModule(mr.VOILUT.ToTags()))
	}

	// 4. Content Date/Time
	opts = append(opts,
		WithElement(tag.ContentDate, mr.ContentDate.String()),
		WithElement(tag.ContentTime, mr.ContentTime.String()),
	)

	// 5. Image Attributes
	opts = append(opts,
		WithElement(tag.SamplesPerPixel, mr.SamplesPerPixel),
		WithElement(tag.PhotometricInterpretation, mr.PhotometricInterp),
		WithElement(tag.BitsAllocated, mr.BitsAllocated),
		WithElement(tag.BitsStored, mr.BitsStored),
		WithElement(tag.HighBit, mr.HighBit),
		WithElement(tag.PixelRepresentation, mr.PixelRepresent),
		WithElement(tag.Rows, uint16(mr.Rows)),
		WithElement(tag.Columns, uint16(mr.Columns)),
	)

	// 6. Number of Frames for multi-frame
	if mr.NumberOfFrames > 1 {
		opts = append(opts, WithElement(tag.NumberOfFrames, fmt.Sprintf("%d", mr.NumberOfFrames)))
	}

	// 7. Pixel Data
	if mr.UseCompression && mr.PixelData != nil && !mr.PixelData.IsEncapsulated {
		flatData := mr.PixelData.GetFlatData()
		opts = append(opts, WithPixelData(mr.Rows, mr.Columns, int(mr.BitsAllocated), flatData, true, mr.CompressionCodec))
	} else if mr.PixelData != nil {
		opts = append(opts, WithRawPixelData(mr.PixelData))
	}

	return NewDataset(opts...)
}

// WriteTo writes the MR Image to any io.Writer
func (mr *MRImage) WriteTo(w io.Writer) (int64, error) {
	ds, err := mr.GetDataset()
	if err != nil {
		return 0, err
	}
	return Write(w, ds)
}

// Write writes the MR Image to a file (convenience wrapper)
func (mr *MRImage) Write(path string) (int64, error) {
	slog.Debug("Writing MR DICOM file", "path", path, "frames", mr.NumberOfFrames, "compressed", mr.UseCompression)
	f, err := os.Create(path)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	return mr.WriteTo(f)
}

// CopyMetadataFrom copies patient, study, series metadata from another dataset
func (mr *MRImage) CopyMetadataFrom(ds *Dataset) {
	// Patient
	mr.Patient.PatientID = GetPatientID(ds)
	mr.Patient.PatientName = parsePersonName(GetPatientName(ds))

	// Study
	mr.Study.StudyInstanceUID = getStringValue(ds, tag.StudyInstanceUID)
	mr.Study.StudyDate = parseDateString(GetStudyDate(ds))
	mr.Study.StudyTime = parseTimeString(getStringValue(ds, tag.StudyTime))
	mr.Study.StudyDescription = GetStudyDescription(ds)

	// Series
	mr.Series.SeriesInstanceUID = getStringValue(ds, tag.SeriesInstanceUID)
	mr.Series.Modality = GetModality(ds)
	mr.Series.SeriesNumber = getIntValue(ds, tag.SeriesNumber)
	mr.Series.SeriesDescription = GetSeriesDescription(ds)

	// Equipment
	mr.Equipment.Manufacturer = getStringValue(ds, tag.Manufacturer)
	mr.Equipment.InstitutionName = getStringValue(ds, tag.InstitutionName)
	mr.Equipment.ManufacturerModel = getStringValue(ds, tag.ManufacturerModelName)

	// Frame of Reference
	forUID := getStringValue(ds, tag.FrameOfReferenceUID)
	if forUID != "" {
		mr.FrameOfReference.FrameOfReferenceUID = forUID
	}

	// Image Plane
	pixelSpacingRow, pixelSpacingCol := GetPixelSpacing(ds)
	if pixelSpacingRow > 0 {
		mr.ImagePlane.PixelSpacing = [2]float64{pixelSpacingRow, pixelSpacingCol}
	}
	mr.ImagePlane.SliceThickness = GetSliceThickness(ds)

	orientation := GetImageOrientationPatient(ds)
	if len(orientation) >= 6 {
		copy(mr.ImagePlane.ImageOrientationPatient[:], orientation[:6])
	}

	position := GetImagePositionPatient(ds)
	if len(position) >= 3 {
		copy(mr.ImagePlane.ImagePositionPatient[:], position[:3])
	}

	// MR-specific attributes
	if mr.MRImageMod != nil {
		mr.MRImageMod.ScanningSequence = getStringValue(ds, tag.Tag{Group: 0x0018, Element: 0x0020})
		mr.MRImageMod.SequenceVariant = getStringValue(ds, tag.Tag{Group: 0x0018, Element: 0x0021})
		mr.MRImageMod.ScanOptions = getStringValue(ds, tag.Tag{Group: 0x0018, Element: 0x0022})
		mr.MRImageMod.MRAcquisitionType = getStringValue(ds, tag.Tag{Group: 0x0018, Element: 0x0023})
		mr.MRImageMod.RepetitionTime = getFloatFromDS(ds, tag.Tag{Group: 0x0018, Element: 0x0080})
		mr.MRImageMod.EchoTime = getFloatFromDS(ds, tag.Tag{Group: 0x0018, Element: 0x0081})
		mr.MRImageMod.FlipAngle = getFloatFromDS(ds, tag.Tag{Group: 0x0018, Element: 0x1314})
		mr.MRImageMod.MagneticFieldStrength = getFloatFromDS(ds, tag.Tag{Group: 0x0018, Element: 0x0087})
		mr.MRImageMod.SpacingBetweenSlices = getFloatFromDS(ds, tag.Tag{Group: 0x0018, Element: 0x0088})
	}

	// Window/Level
	wc := GetWindowCenter(ds)
	ww := GetWindowWidth(ds)
	if wc != 0 || ww != 0 {
		mr.VOILUT = &module.VOILUTModule{
			Windows: []module.WindowLevel{{Center: wc, Width: ww}},
		}
	}
}

// Helper to parse person name string into struct
func parsePersonName(name string) module.PersonName {
	// Simple implementation - just use as family name
	return module.PersonName{FamilyName: name}
}

// parseDateString parses a DICOM date string (YYYYMMDD) into module.Date
func parseDateString(s string) module.Date {
	if len(s) < 8 {
		return module.Date{}
	}
	var year, month, day int
	fmt.Sscanf(s, "%04d%02d%02d", &year, &month, &day)
	return module.Date{Year: year, Month: month, Day: day}
}

// parseTimeString parses a DICOM time string into module.Time
func parseTimeString(s string) module.Time {
	if len(s) < 6 {
		return module.Time{}
	}
	var hour, min, sec int
	fmt.Sscanf(s[:6], "%02d%02d%02d", &hour, &min, &sec)
	return module.Time{Hour: hour, Minute: min, Second: sec}
}

// Helper to get string value from dataset
func getStringValue(ds *Dataset, t tag.Tag) string {
	if elem, ok := ds.FindElement(t.Group, t.Element); ok {
		if s, ok := elem.GetString(); ok {
			return s
		}
	}
	return ""
}

// Helper to get int value from dataset
func getIntValue(ds *Dataset, t tag.Tag) int {
	if elem, ok := ds.FindElement(t.Group, t.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
	}
	return 0
}

// getFloatFromDS extracts a float value from a DS (decimal string) element
func getFloatFromDS(ds *Dataset, t tag.Tag) float64 {
	if elem, ok := ds.FindElement(t.Group, t.Element); ok {
		// Try GetFloats first (for numeric types)
		if vals, ok := elem.GetFloats(); ok && len(vals) > 0 {
			return vals[0]
		}
		// Fall back to parsing string
		if s, ok := elem.GetString(); ok {
			var v float64
			fmt.Sscanf(s, "%f", &v)
			return v
		}
	}
	return 0
}
