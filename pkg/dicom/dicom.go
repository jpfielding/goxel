// Package dicom provides a native Go implementation for reading and writing DICOM files.
//
// This package provides:
//   - Low-level DICOM parsing and writing
//   - High-level IOD (CT, DX, MR) access
//   - JPEG-LS and JPEG Lossless compression support
//
// Basic usage:
//
//	// Read a DICOM file
//	ds, err := dicom.ReadFile("/path/to/file.dcm")
//	if err != nil {
//		log.Fatal(err)
//	}
//
//	// Access pixel data
//	pd, err := ds.GetPixelData()
//
//	// Determine modality
//	if dicom.IsCT(ds) {
//		// Process CT data
//	}
package dicom

import (
	"bytes"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
	"github.com/jpfielding/goxel/pkg/dicom/transfer"
)

// Re-export commonly used types from subpackages
type (
	// TransferSyntax represents a DICOM transfer syntax
	TransferSyntax = transfer.Syntax
)

// Transfer syntax constants
const (
	ExplicitVRLittleEndian = transfer.ExplicitVRLittleEndian
	ImplicitVRLittleEndian = transfer.ImplicitVRLittleEndian
	JPEGLSLossless         = transfer.JPEGLSLossless
	JPEGLosslessFirstOrder = transfer.JPEGLosslessFirstOrder
)

// SOP Class UIDs for common modalities
const (
	CTImageStorageUID       = "1.2.840.10008.5.1.4.1.1.2"
	MRImageStorageUID       = "1.2.840.10008.5.1.4.1.1.4"
	DXImageStorageUID       = "1.2.840.10008.5.1.4.1.1.1.1"
	EnhancedCTStorageUID    = "1.2.840.10008.5.1.4.1.1.2.1"
	EnhancedMRStorageUID    = "1.2.840.10008.5.1.4.1.1.4.1"
	SecondaryCaptureUID     = "1.2.840.10008.5.1.4.1.1.7"
	UltrasoundImageUID      = "1.2.840.10008.5.1.4.1.1.6.1"
	NuclearMedicineImageUID = "1.2.840.10008.5.1.4.1.1.20"
)

// ReadFile reads a DICOM file from disk
func ReadFile(path string) (*Dataset, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("opening file: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("reading file: %w", err)
	}

	return Parse(bytes.NewReader(data))
}

// ReadBuffer reads a DICOM file from a byte slice
func ReadBuffer(data []byte) (*Dataset, error) {
	return Parse(bytes.NewReader(data))
}

// GetExtension returns the standard DICOM file extension
func GetExtension() string {
	return ".dcm"
}

// IsCT returns true if the dataset is a CT image
func IsCT(ds *Dataset) bool {
	return checkSOPClass(ds, CTImageStorageUID, EnhancedCTStorageUID)
}

// IsMR returns true if the dataset is an MR image
func IsMR(ds *Dataset) bool {
	return checkSOPClass(ds, MRImageStorageUID, EnhancedMRStorageUID)
}

// IsDX returns true if the dataset is a DX image
func IsDX(ds *Dataset) bool {
	return checkSOPClass(ds, DXImageStorageUID)
}

// GetModality returns the modality string from the dataset
func GetModality(ds *Dataset) string {
	return ds.Modality()
}

// GetTransferSyntax returns the transfer syntax from the dataset
func GetTransferSyntax(ds *Dataset) TransferSyntax {
	return ds.TransferSyntax()
}

// IsEncapsulated returns true if the pixel data is encapsulated (compressed)
func IsEncapsulated(ds *Dataset) bool {
	return ds.IsEncapsulated()
}

// GetRows returns the number of rows in the image
func GetRows(ds *Dataset) int {
	return ds.Rows()
}

// GetColumns returns the number of columns in the image
func GetColumns(ds *Dataset) int {
	return ds.Columns()
}

// GetNumberOfFrames returns the number of frames in the image
func GetNumberOfFrames(ds *Dataset) int {
	return ds.NumberOfFrames()
}

// GetBitsAllocated returns the bits allocated per sample
func GetBitsAllocated(ds *Dataset) int {
	return ds.BitsAllocated()
}

// GetPixelRepresentation returns 0 for unsigned, 1 for signed
func GetPixelRepresentation(ds *Dataset) int {
	return ds.PixelRepresentation()
}

// GetInstanceNumber returns the instance number (0020,0013)
func GetInstanceNumber(ds *Dataset) int {
	if elem, ok := ds.FindElement(tag.InstanceNumber.Group, tag.InstanceNumber.Element); ok {
		if v, ok := elem.GetInt(); ok {
			return v
		}
		if s, ok := elem.GetString(); ok {
			var n int
			fmt.Sscanf(strings.TrimSpace(s), "%d", &n)
			return n
		}
	}
	return 0
}

// GetSliceLocation returns the slice location (0020,1041)
func GetSliceLocation(ds *Dataset) float64 {
	if elem, ok := ds.FindElement(tag.SliceLocation.Group, tag.SliceLocation.Element); ok {
		if s, ok := elem.GetString(); ok {
			var loc float64
			fmt.Sscanf(strings.TrimSpace(s), "%f", &loc)
			return loc
		}
	}
	return 0
}

// GetSeriesDescription returns the series description (0008,103E)
func GetSeriesDescription(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.SeriesDescription.Group, tag.SeriesDescription.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetPatientName returns the patient name (0010,0010)
func GetPatientName(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.PatientName.Group, tag.PatientName.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetPatientID returns the patient ID (0010,0020)
func GetPatientID(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.PatientID.Group, tag.PatientID.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetStudyDate returns the study date (0008,0020)
func GetStudyDate(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.StudyDate.Group, tag.StudyDate.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetStudyDescription returns the study description (0008,1030)
func GetStudyDescription(ds *Dataset) string {
	if elem, ok := ds.FindElement(tag.StudyDescription.Group, tag.StudyDescription.Element); ok {
		if s, ok := elem.GetString(); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

// GetWindowCenter returns the window center (0028,1050)
func GetWindowCenter(ds *Dataset) float64 {
	if elem, ok := ds.FindElement(tag.WindowCenter.Group, tag.WindowCenter.Element); ok {
		if s, ok := elem.GetString(); ok {
			var wc float64
			// May have multiple values separated by backslash
			parts := strings.Split(s, "\\")
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &wc)
			return wc
		}
	}
	return 0
}

// GetWindowWidth returns the window width (0028,1051)
func GetWindowWidth(ds *Dataset) float64 {
	if elem, ok := ds.FindElement(tag.WindowWidth.Group, tag.WindowWidth.Element); ok {
		if s, ok := elem.GetString(); ok {
			var ww float64
			// May have multiple values separated by backslash
			parts := strings.Split(s, "\\")
			fmt.Sscanf(strings.TrimSpace(parts[0]), "%f", &ww)
			return ww
		}
	}
	return 0
}

// GetPixelData extracts and returns pixel data from the dataset
func (ds *Dataset) GetPixelData() (*PixelData, error) {
	elem, ok := ds.FindElement(tag.PixelData.Group, tag.PixelData.Element)
	if !ok {
		return nil, fmt.Errorf("no pixel data element found")
	}

	// Case 1: Already converted to *PixelData (encapsulated)
	if pd, ok := elem.GetPixelData(); ok {
		return pd, nil
	}

	// Case 2: Uncompressed data
	var u16Raw []uint16
	var byteRaw []byte

	switch v := elem.Value.(type) {
	case []byte:
		byteRaw = v
	case []uint16:
		u16Raw = v
	default:
		return nil, fmt.Errorf("pixel data element has unexpected type: %T", elem.Value)
	}

	// Get dimensions for conversion
	rows := GetRows(ds)
	cols := GetColumns(ds)
	numFrames := GetNumberOfFrames(ds)
	bitsAllocated := GetBitsAllocated(ds)

	slog.Debug("Converting uncompressed pixel data",
		slog.Int("rows", rows),
		slog.Int("cols", cols),
		slog.Int("numFrames", numFrames),
		slog.Int("bitsAllocated", bitsAllocated),
		slog.String("type", fmt.Sprintf("%T", elem.Value)))

	if rows == 0 || cols == 0 {
		return nil, fmt.Errorf("invalid dimensions for pixel data conversion: %dx%d", rows, cols)
	}

	pd := &PixelData{
		IsEncapsulated: false,
		Frames:         make([]Frame, numFrames),
	}

	bytesPerPixel := (bitsAllocated + 7) / 8
	pixelsPerFrame := rows * cols
	frameSizeInBytes := pixelsPerFrame * bytesPerPixel

	slog.Debug("Calculated frame metrics",
		slog.Int("bytesPerPixel", bytesPerPixel),
		slog.Int("frameSizeInBytes", frameSizeInBytes),
		slog.Int("pixelsPerFrame", pixelsPerFrame))

	for i := 0; i < numFrames; i++ {
		u16Data := make([]uint16, pixelsPerFrame)

		if len(u16Raw) > 0 {
			start := i * pixelsPerFrame
			end := start + pixelsPerFrame
			if end > len(u16Raw) {
				return nil, fmt.Errorf("pixel data truncated: expected %d pixels for %d frames, got %d", numFrames*pixelsPerFrame, numFrames, len(u16Raw))
			}
			copy(u16Data, u16Raw[start:end])
		} else if len(byteRaw) > 0 {
			start := i * frameSizeInBytes
			end := start + frameSizeInBytes
			if end > len(byteRaw) {
				return nil, fmt.Errorf("pixel data truncated: expected %d bytes for %d frames, got %d", numFrames*frameSizeInBytes, numFrames, len(byteRaw))
			}

			frameData := byteRaw[start:end]
			if bytesPerPixel == 2 {
				for j := 0; j < pixelsPerFrame; j++ {
					if j*2+1 < len(frameData) {
						u16Data[j] = uint16(frameData[j*2]) | (uint16(frameData[j*2+1]) << 8)
					}
				}
			} else {
				for j := 0; j < pixelsPerFrame; j++ {
					if j < len(frameData) {
						u16Data[j] = uint16(frameData[j])
					}
				}
			}
		}

		pd.Frames[i] = Frame{
			Data: u16Data,
		}
	}

	return pd, nil
}

// GetRescale returns the rescale intercept and slope from the dataset.
// If Rescale Intercept is missing, defaults to 0.
func GetRescale(ds *Dataset) (intercept, slope float64) {
	intercept, slope = 0, 1 // Default values

	var foundIntercept bool
	if elem, ok := ds.FindElement(tag.RescaleIntercept.Group, tag.RescaleIntercept.Element); ok {
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f", &intercept)
			foundIntercept = true
		}
	}
	// If tag is absent, check for implicit intercept via heuristic
	// CT images typically have specific defaults or are signed.
	// Some non-compliant files might be marked as Unsigned (0)
	// but contain values offset by +32768. In this case, we need -32768 intercept.
	if !foundIntercept && IsCT(ds) {
		pixelRep := GetPixelRepresentation(ds) // 0=unsigned, 1=signed
		if pixelRep == 0 {
			// Heuristic: Unsigned CT likely implies shifted values
			intercept = -32768.0
		}
	}

	if elem, ok := ds.FindElement(tag.RescaleSlope.Group, tag.RescaleSlope.Element); ok {
		if s, ok := elem.GetString(); ok {
			fmt.Sscanf(s, "%f", &slope)
		}
	}

	return
}

// Helper function to check SOP Class UID
func checkSOPClass(ds *Dataset, uids ...string) bool {
	if elem, ok := ds.FindElement(tag.SOPClassUID.Group, tag.SOPClassUID.Element); ok {
		if s, ok := elem.GetString(); ok {
			s = strings.TrimSpace(s)
			for _, uid := range uids {
				if s == uid {
					return true
				}
			}
		}
	}
	return false
}
