package module

import "github.com/jpfielding/goxel/pkg/dicom/tag"

// PatientModule represents the DICOM Patient Module
type PatientModule struct {
	PatientName      PersonName
	PatientID        string
	PatientBirthDate Date
	PatientSex       string // M, F, O
	PatientAge       string
	PatientComments  string
}

func (m *PatientModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.PatientName, Value: m.PatientName.String()},
		{Tag: tag.PatientID, Value: m.PatientID},
		{Tag: tag.PatientBirthDate, Value: m.PatientBirthDate.String()},
		{Tag: tag.PatientSex, Value: m.PatientSex},
		{Tag: tag.PatientAge, Value: m.PatientAge},
		{Tag: tag.PatientComments, Value: m.PatientComments},
	}
}

// SetPatientName sets the patient's name
func (m *PatientModule) SetPatientName(first, last, middle, prefix, suffix string) {
	m.PatientName = PersonName{
		GivenName:  first,
		FamilyName: last,
		MiddleName: middle,
		Prefix:     prefix,
		Suffix:     suffix,
	}
}
