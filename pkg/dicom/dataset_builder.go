package dicom

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"log/slog"

	"github.com/jpfielding/goxel/pkg/dicom/module"
	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// Option configures a Dataset during construction
type Option func(*Dataset) error

// NewDataset creates a Dataset with the given options
func NewDataset(opts ...Option) (*Dataset, error) {
	ds := &Dataset{Elements: make(map[Tag]*Element)}
	for _, opt := range opts {
		if err := opt(ds); err != nil {
			return nil, err
		}
	}
	return ds, nil
}

// WithElement adds a single element to the dataset
func WithElement(t tag.Tag, value interface{}) Option {
	return func(ds *Dataset) error {
		internalTag := Tag{Group: t.Group, Element: t.Element}
		vr := GetVR(t)
		ds.Elements[internalTag] = &Element{
			Tag:   internalTag,
			VR:    vr,
			Value: value,
		}
		return nil
	}
}

// WithSequence adds a sequence element to the dataset
func WithSequence(t tag.Tag, items ...*Dataset) Option {
	return func(ds *Dataset) error {
		internalTag := Tag{Group: t.Group, Element: t.Element}
		ds.Elements[internalTag] = &Element{
			Tag:   internalTag,
			VR:    "SQ",
			Value: items,
		}
		return nil
	}
}

// WithFileMeta adds standard file meta information elements
func WithFileMeta(sopClassUID, sopInstanceUID, transferSyntax string) Option {
	return func(ds *Dataset) error {
		opts := []Option{
			WithElement(tag.MediaStorageSOPClassUID, sopClassUID),
			WithElement(tag.MediaStorageSOPInstanceUID, sopInstanceUID),
			WithElement(tag.TransferSyntaxUID, transferSyntax),
			WithElement(tag.ImplementationClassUID, "1.2.826.0.1.3680043.8.498.1"),
			WithElement(tag.ImplementationVersionName, "GO_DICOM"),
		}
		for _, opt := range opts {
			if err := opt(ds); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithModule adds all elements from a module's ToTags() result
func WithModule(tags []module.IODElement) Option {
	return func(ds *Dataset) error {
		for _, el := range tags {
			if err := WithElement(el.Tag, el.Value)(ds); err != nil {
				return err
			}
		}
		return nil
	}
}

// WithPixelData adds pixel data, either native or compressed.
// If codec is nil, data is stored uncompressed. Otherwise, it is compressed using the provided codec.
func WithPixelData(rows, cols, bitsAllocated int, data []uint16, codec Codec) Option {
	return func(ds *Dataset) error {
		if len(data) == 0 {
			return nil
		}

		pixelsPerFrame := rows * cols
		numFrames := len(data) / pixelsPerFrame

		pd := &PixelData{
			IsEncapsulated: codec != nil,
			Frames:         make([]Frame, numFrames),
		}

		if codec != nil {
			offsets := make([]uint32, numFrames)
			currentOffset := uint32(0)

			for i := 0; i < numFrames; i++ {
				offsets[i] = currentOffset
				start := i * pixelsPerFrame
				end := start + pixelsPerFrame
				sliceData := data[start:end]

				var buf bytes.Buffer
				var img image.Image

				if bitsAllocated > 8 {
					img16 := image.NewGray16(image.Rect(0, 0, cols, rows))

					if i == 0 && len(sliceData) > 10 {
						slog.Debug("ENCODE Frame 0", "first_pixels_subset", sliceData[:10])
					}

					for j, val := range sliceData {
						x := j % cols
						y := j / cols
						img16.SetGray16(x, y, color.Gray16{Y: val})
					}
					img = img16
				} else {
					img8 := image.NewGray(image.Rect(0, 0, cols, rows))
					for j, val := range sliceData {
						x := j % cols
						y := j / cols
						img8.SetGray(x, y, color.Gray{Y: uint8(val)})
					}
					img = img8
				}

				if err := codec.Encode(&buf, img); err != nil {
					return fmt.Errorf("%s encode error: %w", codec.Name(), err)
				}

				compressedData := buf.Bytes()
				if len(compressedData)%2 != 0 {
					compressedData = append(compressedData, 0x00)
				}

				pd.Frames[i] = Frame{
					CompressedData: compressedData,
				}

				frameSize := uint32(len(compressedData)) + 8
				currentOffset += frameSize
			}
			pd.Offsets = offsets

			t := Tag{Group: 0x7FE0, Element: 0x0010}
			ds.Elements[t] = &Element{
				Tag:   t,
				VR:    "OB",
				Value: pd,
			}
		} else {
			for i := 0; i < numFrames; i++ {
				start := i * pixelsPerFrame
				end := start + pixelsPerFrame

				fData := make([]uint16, len(data[start:end]))
				copy(fData, data[start:end])

				pd.Frames[i] = Frame{
					Data: fData,
				}
			}

			vr := "OB"
			if bitsAllocated > 8 {
				vr = "OW"
			}

			t := Tag{Group: 0x7FE0, Element: 0x0010}
			ds.Elements[t] = &Element{
				Tag:   t,
				VR:    vr,
				Value: pd,
			}
		}
		return nil
	}
}

// WithRawPixelData adds pre-constructed PixelData to the dataset
func WithRawPixelData(pd *PixelData) Option {
	return func(ds *Dataset) error {
		if pd == nil {
			return nil
		}
		vr := "OB"
		if !pd.IsEncapsulated && len(pd.Frames) > 0 && len(pd.Frames[0].Data) > 0 {
			vr = "OW"
		}
		t := Tag{Group: 0x7FE0, Element: 0x0010}
		ds.Elements[t] = &Element{
			Tag:   t,
			VR:    vr,
			Value: pd,
		}
		return nil
	}
}

// GetVR returns the Value Representation (VR) for a standard tag
func GetVR(t tag.Tag) string {
	if t.Group == 0x0002 {
		if t.Element == 0x0000 {
			return "UL"
		}
		if t.Element == 0x0001 {
			return "OB"
		}
		if t == tag.TransferSyntaxUID {
			return "UI"
		}
		return "UI"
	}

	switch t {
	case tag.PatientName:
		return "PN"
	case tag.PatientID:
		return "LO"
	case tag.PatientBirthDate:
		return "DA"
	case tag.PatientSex:
		return "CS"

	case tag.StudyDate:
		return "DA"
	case tag.StudyTime:
		return "TM"
	case tag.AccessionNumber:
		return "SH"
	case tag.StudyDescription:
		return "LO"
	case tag.StudyInstanceUID:
		return "UI"
	case tag.StudyID:
		return "SH"

	case tag.Modality:
		return "CS"
	case tag.SeriesInstanceUID:
		return "UI"
	case tag.SeriesNumber:
		return "IS"
	case tag.SeriesDescription:
		return "LO"

	case tag.SamplesPerPixel:
		return "US"
	case tag.PhotometricInterpretation:
		return "CS"
	case tag.Rows:
		return "US"
	case tag.Columns:
		return "US"
	case tag.BitsAllocated:
		return "US"
	case tag.BitsStored:
		return "US"
	case tag.HighBit:
		return "US"
	case tag.PixelRepresentation:
		return "US"
	case tag.NumberOfFrames:
		return "IS"

	case tag.RescaleIntercept:
		return "DS"
	case tag.RescaleSlope:
		return "DS"
	case tag.RescaleType:
		return "LO"
	case tag.WindowCenter:
		return "DS"
	case tag.WindowWidth:
		return "DS"

	case tag.PixelSpacing:
		return "DS"
	case tag.SliceThickness:
		return "DS"
	case tag.SpacingBetweenSlices:
		return "DS"
	case tag.ImagePositionPatient:
		return "DS"
	case tag.ImageOrientationPatient:
		return "DS"
	case tag.SliceLocation:
		return "DS"

	case tag.ContentDate:
		return "DA"
	case tag.ContentTime:
		return "TM"
	case tag.InstanceNumber:
		return "IS"
	case tag.ImageType:
		return "CS"

	case tag.SOPClassUID:
		return "UI"
	case tag.SOPInstanceUID:
		return "UI"

	case tag.PixelData:
		return "OW"
	}

	return "UN"
}
