package dicos

// DICOSTag represents a DICOS data element tag.
type DICOSTag struct {
	Group   uint16
	Element uint16
}

// Map of DICOS-specific tags
var DicosTags = map[DICOSTag]string{
	{0x001F, 0x1001}: "ThreatDetectionReport",
	{0x001F, 0x1002}: "ThreatImageProjectionImage",
	{0x001F, 0x1003}: "ThreatDescription",
	{0x001F, 0x1004}: "Owner",
	{0x001F, 0x1005}: "ObjectOfInspection",
	{0x001F, 0x1006}: "Itinerary",
	{0x001F, 0x1007}: "AutomatedThreatRecognitionAlgorithm",
	{0x001F, 0x1008}: "ImageModalityType",
	{0x001F, 0x1009}: "DICOSVersion",
}

// LookupTagName returns the name of a DICOS tag.
func LookupTagName(group, element uint16) string {
	tag := DICOSTag{Group: group, Element: element}
	if name, ok := DicosTags[tag]; ok {
		return name
	}
	return ""
}
