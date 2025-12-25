package module

import "github.com/jpfielding/goxel/pkg/dicom/tag"

// GeneralEquipmentModule represents the DICOM General Equipment Module
// Stratovan: SDICOM::GeneralEquipmentModule
type GeneralEquipmentModule struct {
	Manufacturer      string
	InstitutionName   string
	StationName       string
	ManufacturerModel string
	DeviceSerial      string
	SoftwareVersions  string
}

func (m *GeneralEquipmentModule) ToTags() []IODElement {
	return []IODElement{
		{Tag: tag.Manufacturer, Value: m.Manufacturer},
		{Tag: tag.InstitutionName, Value: m.InstitutionName},
		{Tag: tag.StationName, Value: m.StationName},
		{Tag: tag.ManufacturerModelName, Value: m.ManufacturerModel},
		{Tag: tag.DeviceSerialNumber, Value: m.DeviceSerial},
		{Tag: tag.SoftwareVersions, Value: m.SoftwareVersions},
	}
}
