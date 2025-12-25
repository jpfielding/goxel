// Package transfer defines DICOM Transfer Syntaxes
package transfer

// Syntax represents a DICOM Transfer Syntax
type Syntax string

// Standard Transfer Syntaxes
const (
	// Uncompressed
	ImplicitVRLittleEndian    Syntax = "1.2.840.10008.1.2"
	ExplicitVRLittleEndian    Syntax = "1.2.840.10008.1.2.1"
	ExplicitVRLittleEndianExt Syntax = "1.2.840.10008.1.2.1.64" // Extended (>4GB)
	ExplicitVRBigEndian       Syntax = "1.2.840.10008.1.2.2"    // Retired

	// JPEG Lossless
	JPEGLossless           Syntax = "1.2.840.10008.1.2.4.57"
	JPEGLosslessFirstOrder Syntax = "1.2.840.10008.1.2.4.70" // Most common

	// JPEG-LS
	JPEGLSLossless     Syntax = "1.2.840.10008.1.2.4.80"
	JPEGLSNearLossless Syntax = "1.2.840.10008.1.2.4.81"

	// JPEG 2000
	JPEG2000Lossless Syntax = "1.2.840.10008.1.2.4.90"
	JPEG2000         Syntax = "1.2.840.10008.1.2.4.91"

	// JPEG Lossy
	JPEGBaseline Syntax = "1.2.840.10008.1.2.4.50"
	JPEGExtended Syntax = "1.2.840.10008.1.2.4.51"

	// Other
	RLELossless        Syntax = "1.2.840.10008.1.2.5"
	DeflatedExplicitVR Syntax = "1.2.840.10008.1.2.1.99"
)

// IsExplicitVR returns true if this transfer syntax uses explicit VR
func (s Syntax) IsExplicitVR() bool {
	return s != ImplicitVRLittleEndian
}

// IsLittleEndian returns true if this transfer syntax uses little endian byte order
func (s Syntax) IsLittleEndian() bool {
	return s != ExplicitVRBigEndian
}

// IsEncapsulated returns true if pixel data is encapsulated (compressed)
func (s Syntax) IsEncapsulated() bool {
	switch s {
	case ImplicitVRLittleEndian, ExplicitVRLittleEndian, ExplicitVRLittleEndianExt, ExplicitVRBigEndian:
		return false
	default:
		return true
	}
}

// IsJPEGLS returns true if this is a JPEG-LS transfer syntax
func (s Syntax) IsJPEGLS() bool {
	return s == JPEGLSLossless || s == JPEGLSNearLossless
}

// IsJPEGLossless returns true if this is a JPEG Lossless transfer syntax
func (s Syntax) IsJPEGLossless() bool {
	return s == JPEGLossless || s == JPEGLosslessFirstOrder
}

// Name returns a human-readable name for the transfer syntax
func (s Syntax) Name() string {
	switch s {
	case ImplicitVRLittleEndian:
		return "Implicit VR Little Endian"
	case ExplicitVRLittleEndian:
		return "Explicit VR Little Endian"
	case ExplicitVRLittleEndianExt:
		return "Explicit VR Little Endian Extended"
	case ExplicitVRBigEndian:
		return "Explicit VR Big Endian (Retired)"
	case JPEGLossless:
		return "JPEG Lossless (Process 14)"
	case JPEGLosslessFirstOrder:
		return "JPEG Lossless First-Order (Process 14, SV1)"
	case JPEGLSLossless:
		return "JPEG-LS Lossless"
	case JPEGLSNearLossless:
		return "JPEG-LS Near-Lossless"
	case JPEG2000Lossless:
		return "JPEG 2000 Lossless"
	case JPEG2000:
		return "JPEG 2000"
	case JPEGBaseline:
		return "JPEG Baseline (Process 1)"
	case JPEGExtended:
		return "JPEG Extended (Process 2 & 4)"
	case RLELossless:
		return "RLE Lossless"
	case DeflatedExplicitVR:
		return "Deflated Explicit VR Little Endian"
	default:
		return string(s)
	}
}

// FromUID converts a UID string to a Syntax
func FromUID(uid string) Syntax {
	return Syntax(uid)
}
