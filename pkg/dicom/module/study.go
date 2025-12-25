package module

import (
	"time"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// GeneralStudyModule represents the DICOM General Study Module
// Stratovan: SDICOM::GeneralStudyModule
type GeneralStudyModule struct {
	StudyInstanceUID string
	StudyDate        Date
	StudyTime        Time
	StudyID          string
	AccessionNumber  string
	StudyDescription string
}

func NewGeneralStudyModule() GeneralStudyModule {
	t := time.Now()
	return GeneralStudyModule{
		StudyDate: NewDate(t),
		StudyTime: NewTime(t),
	}
}

func (m *GeneralStudyModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.StudyInstanceUID, Value: m.StudyInstanceUID},
		{Tag: tag.StudyDate, Value: m.StudyDate.String()},
		{Tag: tag.StudyTime, Value: m.StudyTime.String()},
		{Tag: tag.StudyID, Value: m.StudyID},
		{Tag: tag.AccessionNumber, Value: m.AccessionNumber},
		{Tag: tag.StudyDescription, Value: m.StudyDescription},
	}
}
