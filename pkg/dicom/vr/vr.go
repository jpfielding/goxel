// Package vr defines DICOM Value Representations
package vr

// VR represents a DICOM Value Representation
type VR string

// Standard DICOM Value Representations
const (
	AE VR = "AE" // Application Entity (16 bytes max)
	AS VR = "AS" // Age String (4 bytes fixed)
	AT VR = "AT" // Attribute Tag (4 bytes fixed)
	CS VR = "CS" // Code String (16 bytes max)
	DA VR = "DA" // Date (8 bytes fixed)
	DS VR = "DS" // Decimal String (16 bytes max)
	DT VR = "DT" // DateTime (26 bytes max)
	FL VR = "FL" // Floating Point Single (4 bytes fixed)
	FD VR = "FD" // Floating Point Double (8 bytes fixed)
	IS VR = "IS" // Integer String (12 bytes max)
	LO VR = "LO" // Long String (64 bytes max)
	LT VR = "LT" // Long Text (10240 bytes max)
	OB VR = "OB" // Other Byte String
	OD VR = "OD" // Other Double String
	OF VR = "OF" // Other Float String
	OL VR = "OL" // Other Long
	OW VR = "OW" // Other Word String
	PN VR = "PN" // Person Name (64 bytes max per component)
	SH VR = "SH" // Short String (16 bytes max)
	SL VR = "SL" // Signed Long (4 bytes fixed)
	SQ VR = "SQ" // Sequence of Items
	SS VR = "SS" // Signed Short (2 bytes fixed)
	ST VR = "ST" // Short Text (1024 bytes max)
	TM VR = "TM" // Time (16 bytes max)
	UC VR = "UC" // Unlimited Characters
	UI VR = "UI" // Unique Identifier (64 bytes max)
	UL VR = "UL" // Unsigned Long (4 bytes fixed)
	UN VR = "UN" // Unknown
	UR VR = "UR" // Universal Resource Identifier
	US VR = "US" // Unsigned Short (2 bytes fixed)
	UT VR = "UT" // Unlimited Text
)

// IsExplicitLength returns true if the VR uses explicit 2-byte length in explicit VR
// Otherwise uses 4-byte length with 2-byte reserved field
func (v VR) IsExplicitLength() bool {
	switch v {
	case OB, OD, OF, OL, OW, SQ, UC, UN, UR, UT:
		return false // Uses 4-byte length with 2 reserved bytes
	default:
		return true // Uses 2-byte length
	}
}

// IsString returns true if this VR contains string data
func (v VR) IsString() bool {
	switch v {
	case AE, AS, CS, DA, DS, DT, IS, LO, LT, PN, SH, ST, TM, UC, UI, UR, UT:
		return true
	default:
		return false
	}
}

// IsBinary returns true if this VR contains binary data
func (v VR) IsBinary() bool {
	switch v {
	case AT, FL, FD, OB, OD, OF, OL, OW, SL, SS, UL, UN, US:
		return true
	default:
		return false
	}
}

// IsSequence returns true if this is a sequence VR
func (v VR) IsSequence() bool {
	return v == SQ
}

// ValueSize returns the fixed size in bytes for fixed-size VRs, or 0 for variable
func (v VR) ValueSize() int {
	switch v {
	case AT:
		return 4
	case FL:
		return 4
	case FD:
		return 8
	case SL:
		return 4
	case SS:
		return 2
	case UL:
		return 4
	case US:
		return 2
	default:
		return 0 // Variable
	}
}
