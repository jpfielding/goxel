package module

import (
	"time"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// SOPCommonModule represents the DICOM SOP Common Module
// Stratovan: SDICOM::SOPCommonModule
type SOPCommonModule struct {
	SOPClassUID          string
	SOPInstanceUID       string
	SpecificCharacterSet string
	InstanceCreationDate Date
	InstanceCreationTime Time
}

func NewSOPCommonModule() SOPCommonModule {
	t := time.Now()
	return SOPCommonModule{
		SpecificCharacterSet: "ISO_IR 100", // Latin 1
		InstanceCreationDate: NewDate(t),
		InstanceCreationTime: NewTime(t),
	}
}

func (m *SOPCommonModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.SOPClassUID, Value: m.SOPClassUID},
		{Tag: tag.SOPInstanceUID, Value: m.SOPInstanceUID},
		{Tag: tag.SpecificCharacterSet, Value: m.SpecificCharacterSet},
		{Tag: tag.InstanceCreationDate, Value: m.InstanceCreationDate.String()},
		{Tag: tag.InstanceCreationTime, Value: m.InstanceCreationTime.String()},
	}
}
