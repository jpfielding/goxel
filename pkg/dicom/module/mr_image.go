package module

import (
	"fmt"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// MRImageModule represents DICOM MR Image Module attributes
type MRImageModule struct {
	// Required
	ScanningSequence    string // SE, IR, GR, EP, RM
	SequenceVariant     string // SK, MTC, SS, TRSS, SP, MP, OSP, NONE
	ScanOptions         string // PER, RG, CG, PPG, FC, PFF, PFP, SP, FS
	MRAcquisitionType   string // 2D, 3D
	RepetitionTime      float64
	EchoTime            float64
	EchoTrainLength     int
	InversionTime       float64
	FlipAngle           float64
	ImagingFrequency    float64
	ImagedNucleus       string // 1H, 31P, etc.
	MagneticFieldStrength float64
	SpacingBetweenSlices float64
	NumberOfPhaseEncodingSteps int
	PercentSampling      float64
	PercentPhaseFieldOfView float64
	PixelBandwidth       float64
	ReceiveCoilName      string
	TransmitCoilName     string
}

func NewMRImageModule() *MRImageModule {
	return &MRImageModule{
		ScanningSequence:  "GR",  // Gradient Recalled
		SequenceVariant:   "SP",  // Spoiled
		MRAcquisitionType: "3D",
		ImagedNucleus:     "1H",
	}
}

// formatMRDS formats a float64 as a DICOM DS (decimal string)
func formatMRDS(v float64) string {
	return fmt.Sprintf("%.6g", v)
}

func (m *MRImageModule) ToTags() []IODElement {
	elements := []IODElement{
		{Tag: tag.Tag{Group: 0x0018, Element: 0x0020}, Value: m.ScanningSequence},       // ScanningSequence
		{Tag: tag.Tag{Group: 0x0018, Element: 0x0021}, Value: m.SequenceVariant},        // SequenceVariant
		{Tag: tag.Tag{Group: 0x0018, Element: 0x0022}, Value: m.ScanOptions},            // ScanOptions
		{Tag: tag.Tag{Group: 0x0018, Element: 0x0023}, Value: m.MRAcquisitionType},      // MRAcquisitionType
	}

	if m.RepetitionTime > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0080}, Value: formatMRDS(m.RepetitionTime)}) // RepetitionTime
	}
	if m.EchoTime > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0081}, Value: formatMRDS(m.EchoTime)}) // EchoTime
	}
	if m.EchoTrainLength > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0091}, Value: fmt.Sprintf("%d", m.EchoTrainLength)}) // EchoTrainLength
	}
	if m.InversionTime > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0082}, Value: formatMRDS(m.InversionTime)}) // InversionTime
	}
	if m.FlipAngle > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x1314}, Value: formatMRDS(m.FlipAngle)}) // FlipAngle
	}
	if m.ImagingFrequency > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0084}, Value: formatMRDS(m.ImagingFrequency)}) // ImagingFrequency
	}
	if m.ImagedNucleus != "" {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0085}, Value: m.ImagedNucleus}) // ImagedNucleus
	}
	if m.MagneticFieldStrength > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0087}, Value: formatMRDS(m.MagneticFieldStrength)}) // MagneticFieldStrength
	}
	if m.SpacingBetweenSlices > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0088}, Value: formatMRDS(m.SpacingBetweenSlices)}) // SpacingBetweenSlices
	}
	if m.PixelBandwidth > 0 {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x0095}, Value: formatMRDS(m.PixelBandwidth)}) // PixelBandwidth
	}
	if m.ReceiveCoilName != "" {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x1250}, Value: m.ReceiveCoilName}) // ReceiveCoilName
	}
	if m.TransmitCoilName != "" {
		elements = append(elements, IODElement{Tag: tag.Tag{Group: 0x0018, Element: 0x1251}, Value: m.TransmitCoilName}) // TransmitCoilName
	}

	return elements
}
