// Package tag defines standard DICOM tags
package tag

// Tag represents a DICOM tag with Group and Element
type Tag struct {
	Group   uint16
	Element uint16
}

// Common comparison and creation functions

// New creates a new Tag
func New(group, element uint16) Tag {
	return Tag{Group: group, Element: element}
}

// Equals compares two tags
func (t Tag) Equals(other Tag) bool {
	return t.Group == other.Group && t.Element == other.Element
}

// IsPrivate returns true if this is a private tag (odd group number)
func (t Tag) IsPrivate() bool {
	return t.Group%2 == 1
}

// IsGroup0002 returns true if this tag is in the File Meta Information group
func (t Tag) IsGroup0002() bool {
	return t.Group == 0x0002
}

// Standard DICOM Tags - File Meta Information (Group 0002)
var (
	FileMetaInformationGroupLength = Tag{0x0002, 0x0000}
	FileMetaInformationVersion     = Tag{0x0002, 0x0001}
	MediaStorageSOPClassUID        = Tag{0x0002, 0x0002}
	MediaStorageSOPInstanceUID     = Tag{0x0002, 0x0003}
	TransferSyntaxUID              = Tag{0x0002, 0x0010}
	ImplementationClassUID         = Tag{0x0002, 0x0012}
	ImplementationVersionName      = Tag{0x0002, 0x0013}
	SpecificCharacterSet           = Tag{0x0008, 0x0005}
)

// Patient Module (Group 0010)
var (
	PatientName      = Tag{0x0010, 0x0010}
	PatientID        = Tag{0x0010, 0x0020}
	PatientBirthDate = Tag{0x0010, 0x0030}
	PatientSex       = Tag{0x0010, 0x0040}
	PatientAge       = Tag{0x0010, 0x1010}
	PatientComments  = Tag{0x0010, 0x4000}
)

// General Study Module (Group 0008, 0020)
var (
	StudyDate        = Tag{0x0008, 0x0020}
	StudyTime        = Tag{0x0008, 0x0030}
	AccessionNumber  = Tag{0x0008, 0x0050}
	StudyDescription = Tag{0x0008, 0x1030}
	StudyInstanceUID = Tag{0x0020, 0x000D}
	StudyID          = Tag{0x0020, 0x0010}
)

// General Series Module
var (
	Modality               = Tag{0x0008, 0x0060}
	SeriesInstanceUID      = Tag{0x0020, 0x000E}
	SeriesNumber           = Tag{0x0020, 0x0011}
	InstanceNumber         = Tag{0x0020, 0x0013}
	SeriesDescription      = Tag{0x0008, 0x103E}
	SeriesDate             = Tag{0x0008, 0x0021}
	SeriesTime             = Tag{0x0008, 0x0031}
	PresentationIntentType = Tag{0x0008, 0x0068}
)

// General Equipment Module
var (
	Manufacturer          = Tag{0x0008, 0x0070}
	InstitutionName       = Tag{0x0008, 0x0080}
	StationName           = Tag{0x0008, 0x1010}
	ManufacturerModelName = Tag{0x0008, 0x1090}
	DeviceSerialNumber    = Tag{0x0018, 0x1000}
	SoftwareVersions      = Tag{0x0018, 0x1020}
)

// X-Ray Acquisition Parameters
var (
	KVP           = Tag{0x0018, 0x0060} // Peak kilo voltage output of X-ray generator
	ImageComments = Tag{0x0020, 0x4000} // User-defined comments about image
)

// SOP Common Module
var (
	SOPClassUID          = Tag{0x0008, 0x0016}
	SOPInstanceUID       = Tag{0x0008, 0x0018}
	InstanceCreationDate = Tag{0x0008, 0x0012}
	InstanceCreationTime = Tag{0x0008, 0x0013}
)

// Reference Tags
var (
	ReferencedSOPClassUID    = Tag{0x0008, 0x1150} // UI - Referenced SOP Class
	ReferencedSOPInstanceUID = Tag{0x0008, 0x1155} // UI - Referenced SOP Instance
	ReferencedSeriesSequence = Tag{0x0008, 0x1115} // SQ - Referenced series
	ReferencedImageSequence  = Tag{0x0008, 0x1140} // SQ - Referenced images
)

// Frame of Reference Module
var (
	FrameOfReferenceUID        = Tag{0x0020, 0x0052}
	PositionReferenceIndicator = Tag{0x0020, 0x1040}
)

// Image Pixel Module (Group 0028)
var (
	SamplesPerPixel           = Tag{0x0028, 0x0002}
	PhotometricInterpretation = Tag{0x0028, 0x0004}
	Rows                      = Tag{0x0028, 0x0010}
	Columns                   = Tag{0x0028, 0x0011}
	BitsAllocated             = Tag{0x0028, 0x0100}
	BitsStored                = Tag{0x0028, 0x0101}
	HighBit                   = Tag{0x0028, 0x0102}
	PixelRepresentation       = Tag{0x0028, 0x0103}
	PixelData                 = Tag{0x7FE0, 0x0010}
	NumberOfFrames            = Tag{0x0028, 0x0008}
)

// CT Image Module
var (
	ImageType                    = Tag{0x0008, 0x0008}
	RescaleIntercept             = Tag{0x0028, 0x1052}
	RescaleSlope                 = Tag{0x0028, 0x1053}
	RescaleType                  = Tag{0x0028, 0x1054}
	WindowCenter                 = Tag{0x0028, 0x1050}
	WindowWidth                  = Tag{0x0028, 0x1051}
	WindowCenterWidthExplanation = Tag{0x0028, 0x1055} // LO - Window explanation
	VOILUTFunction               = Tag{0x0028, 0x1056} // CS - LINEAR, SIGMOID, LINEAR_EXACT
)

// Image Position/Orientation
var (
	ImagePositionPatient    = Tag{0x0020, 0x0032}
	ImageOrientationPatient = Tag{0x0020, 0x0037}
	SliceThickness          = Tag{0x0018, 0x0050}
	SpacingBetweenSlices    = Tag{0x0018, 0x0088}
	PixelSpacing            = Tag{0x0028, 0x0030}
	SliceLocation           = Tag{0x0020, 0x1041}
)

// Content Date/Time
var (
	ContentDate = Tag{0x0008, 0x0023}
	ContentTime = Tag{0x0008, 0x0033}
)

// Sequence delimiters
var (
	Item                     = Tag{0xFFFE, 0xE000}
	ItemDelimitationItem     = Tag{0xFFFE, 0xE00D}
	SequenceDelimitationItem = Tag{0xFFFE, 0xE0DD}
)

// DX Detector Module Tags (Group 0018)
var (
	DetectorType                  = Tag{0x0018, 0x7004} // CS - DIRECT, SCINTILLATOR, STORAGE
	DetectorConfiguration         = Tag{0x0018, 0x7005} // CS - SLOT, AREA
	DetectorDescription           = Tag{0x0018, 0x7006} // LT - Detector description
	DetectorID                    = Tag{0x0018, 0x700A} // SH - Detector identifier
	DetectorManufacturerName      = Tag{0x0018, 0x702A} // LO - Detector manufacturer
	DetectorManufacturerModelName = Tag{0x0018, 0x702B} // LO - Detector model
	DetectorActiveTime            = Tag{0x0018, 0x7014} // DS - Active exposure time (ms)
	DetectorActivationOffset      = Tag{0x0018, 0x7016} // DS - Offset from exposure start (ms)
	DetectorConditionsNominalFlag = Tag{0x0018, 0x7000} // CS - YES or NO
	DetectorTemperature           = Tag{0x0018, 0x7001} // DS - Temperature (deg C)
	DetectorElementPhysicalSize   = Tag{0x0018, 0x7020} // DS - size (mm)
	DetectorElementSpacing        = Tag{0x0018, 0x7022} // DS - spacing (mm)
	DetectorActiveDimensions      = Tag{0x0018, 0x7026} // US - active rows/cols
	DetectorBinning               = Tag{0x0018, 0x701A} // DS - binning factor
	FieldOfViewShape              = Tag{0x0018, 0x1147} // CS - RECTANGLE, ROUND, HEXAGONAL
	FieldOfViewDimensions         = Tag{0x0018, 0x1149} // IS - FOV dimensions (mm)
)

// DX X-Ray Acquisition Tags (Group 0018)
var (
	XRayTubeCurrentInmA                = Tag{0x0018, 0x8151} // DS - Tube current (mA)
	ExposureTimeInms                   = Tag{0x0018, 0x9328} // FD - Exposure time (ms)
	DistanceSourceToDetector           = Tag{0x0018, 0x1110} // DS - SID (mm)
	DistanceSourceToPatient            = Tag{0x0018, 0x1111} // DS - SOD (mm)
	EstimatedDoseSaving                = Tag{0x0018, 0x9324} // FD - Dose saving %
	ExposureControlMode                = Tag{0x0018, 0x7060} // CS - MANUAL, AUTOMATIC
	ExposureControlModeDescription     = Tag{0x0018, 0x7062} // LT - Mode description
	ExposureStatus                     = Tag{0x0018, 0x7064} // CS - NORMAL, ABORTED
	PhototimerSetting                  = Tag{0x0018, 0x7065} // DS - Phototimer setting
	SensitivityValue                   = Tag{0x0018, 0x6000} // DS - Sensitivity (ISO)
	AnodeTargetMaterial                = Tag{0x0018, 0x1191} // CS - MOLYBDENUM, RHODIUM, TUNGSTEN
	BodyPartThickness                  = Tag{0x0018, 0x11A0} // DS - Thickness (mm)
	CompressionForce                   = Tag{0x0018, 0x11A2} // DS - Compression force (N)
	Grid                               = Tag{0x0018, 0x1166} // CS - FIXED, FOCUSED, RECIPROCATING, NONE
	FocalSpotSize                      = Tag{0x0018, 0x1190} // DS - Focal spot (mm)
	ImageAndFluoroscopyAreaDoseProduct = Tag{0x0018, 0x115E} // DS - DAP (dGy*cm2)
)

// Extended Image Pixel Module (Group 0028)
var (
	PlanarConfiguration        = Tag{0x0028, 0x0006} // US - 0=color-by-pixel, 1=color-by-plane
	SmallestImagePixelValue    = Tag{0x0028, 0x0106} // US/SS - Min pixel value
	LargestImagePixelValue     = Tag{0x0028, 0x0107} // US/SS - Max pixel value
	PixelPaddingValue          = Tag{0x0028, 0x0120} // US/SS - Padding value
	PixelPaddingRangeLimit     = Tag{0x0028, 0x0121} // US/SS - Padding range limit
	LossyImageCompression      = Tag{0x0028, 0x2110} // CS - 00=lossless, 01=lossy
	LossyImageCompressionRatio = Tag{0x0028, 0x2112} // DS - Compression ratio
	LUTDescriptor              = Tag{0x0028, 0x3002} // US - LUT descriptor
	LUTData                    = Tag{0x0028, 0x3006} // US/OW - LUT data
	VOILUTSequence             = Tag{0x0028, 0x3010} // SQ - VOI LUT sequence
	ModalityLUTSequence        = Tag{0x0028, 0x3000} // SQ - Modality LUT sequence
	RedPaletteColorLUTData     = Tag{0x0028, 0x1201} // OW - Red palette
	GreenPaletteColorLUTData   = Tag{0x0028, 0x1202} // OW - Green palette
	BluePaletteColorLUTData    = Tag{0x0028, 0x1203} // OW - Blue palette
)

// CT Acquisition Parameters (Group 0018)
var (
	ScanOptions            = Tag{0x0018, 0x0022} // CS - Scan options
	DataCollectionDiameter = Tag{0x0018, 0x0090} // DS - Reconstruction diameter (mm)
	ReconstructionDiameter = Tag{0x0018, 0x1100} // DS - Reconstruction diameter (mm)
	ConvolutionKernel      = Tag{0x0018, 0x1210} // SH - Convolution kernel
	ExposureTime           = Tag{0x0018, 0x1150} // IS - Exposure time (ms)
	XRayTubeCurrent        = Tag{0x0018, 0x1151} // IS - Tube current (mA)
	Exposure               = Tag{0x0018, 0x1152} // IS - Exposure (mAs)
	ExposureInmAs          = Tag{0x0018, 0x1153} // IS - Exposure in mAs
	FilterType             = Tag{0x0018, 0x1160} // SH - Filter type
	GeneratorPower         = Tag{0x0018, 0x1170} // IS - Generator power (kW)
	FocalSpots             = Tag{0x0018, 0x1190} // DS - Focal spot size (mm)
	TableHeight            = Tag{0x0018, 0x1130} // DS - Table height (mm)
	RotationDirection      = Tag{0x0018, 0x1140} // CS - CW or CC (rotation direction)
	GantryDetectorTilt     = Tag{0x0018, 0x1120} // DS - Gantry/Detector tilt (degrees)
	TableSpeed             = Tag{0x0018, 0x9309} // FD - Table speed (mm/s)
	TableFeedPerRotation   = Tag{0x0018, 0x9310} // FD - Table feed per rotation (mm)
	SpiralPitchFactor      = Tag{0x0018, 0x9311} // FD - Spiral pitch factor
	SingleCollimationWidth = Tag{0x0018, 0x9306} // FD - Single collimation width (mm)
	TotalCollimationWidth  = Tag{0x0018, 0x9307} // FD - Total collimation width (mm)
	DateOfLastCalibration  = Tag{0x0018, 0x1200} // DA - Calibration date
	TimeOfLastCalibration  = Tag{0x0018, 0x1201} // TM - Calibration time
	AcquisitionType        = Tag{0x0018, 0x9302} // CS - SPIRAL, CONSTANT_ANGLE, etc
	TubeAngle              = Tag{0x0018, 0x9303} // FD - Tube angle (degrees)
)

// LookupName returns a human-readable name for common tags
func (t Tag) LookupName() string {
	switch t {
	case PatientName:
		return "PatientName"
	case PatientID:
		return "PatientID"
	case Rows:
		return "Rows"
	case Columns:
		return "Columns"
	case BitsAllocated:
		return "BitsAllocated"
	case PixelData:
		return "PixelData"
	case TransferSyntaxUID:
		return "TransferSyntaxUID"
	case SOPClassUID:
		return "SOPClassUID"
	case Modality:
		return "Modality"
	case NumberOfFrames:
		return "NumberOfFrames"
	default:
		return ""
	}
}
