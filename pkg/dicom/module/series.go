package module

import (
	"fmt"

	"github.com/jpfielding/goxel/pkg/dicom/tag"
)

// GeneralSeriesModule represents the DICOM General Series Module
// Stratovan: SDICOM::GeneralSeriesModule
type GeneralSeriesModule struct {
	Modality          string
	SeriesInstanceUID string
	SeriesNumber      int
	SeriesDate        Date
	SeriesTime        Time
	SeriesDescription string
}

func (m *GeneralSeriesModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.Modality, Value: m.Modality},
		{Tag: tag.SeriesInstanceUID, Value: m.SeriesInstanceUID},
		{Tag: tag.SeriesNumber, Value: fmt.Sprintf("%d", m.SeriesNumber)},
		{Tag: tag.SeriesDate, Value: m.SeriesDate.String()},
		{Tag: tag.SeriesTime, Value: m.SeriesTime.String()},
		{Tag: tag.SeriesDescription, Value: m.SeriesDescription},
	}
}

func (m *GeneralSeriesModule) SetSeriesInstanceUID(uid string) {
	m.SeriesInstanceUID = uid
}
