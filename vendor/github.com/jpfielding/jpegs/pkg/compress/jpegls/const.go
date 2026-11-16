package jpegls

import "errors"

// Markers.
const (
	MarkerSOI   = 0xFFD8 // Start of Image
	MarkerEOI   = 0xFFD9 // End of Image
	MarkerSOS   = 0xFFDA // Start of Scan
	MarkerDNL   = 0xFFDC
	MarkerDRI   = 0xFFDD // Define Restart Interval
	MarkerLSE   = 0xFFF8 // JPEG-LS preset parameters
	MarkerSOF55 = 0xFFF7 // Start of Frame (JPEG-LS)
)

const (
	// DefaultReset is the default statistics reset threshold (T.87 A.2.1 RESET).
	DefaultReset = 64

	// minReset is the smallest permitted LSE RESET (T.87 C.2.4.1.1).
	minReset = 3
)

// Sentinel errors; every error returned by this package wraps one of these,
// ErrImageTooLarge (see limits.go), or an error from the underlying
// io.Reader / io.Writer.
var (
	// ErrInvalidData reports a malformed or truncated stream, or an invalid
	// encode parameter.
	ErrInvalidData = errors.New("jpegls: invalid data")
	// ErrUnsupported reports a well-formed stream (or image type) outside the
	// supported subset of T.87.
	ErrUnsupported = errors.New("jpegls: unsupported")
)

// FrameHeader carries the SOF55 parameters.
type FrameHeader struct {
	Precision    int
	Height       int
	Width        int
	Components   int
	ComponentIDs []byte
}

// ScanHeader carries the SOS parameters of a single-component scan.
type ScanHeader struct {
	ComponentID byte
	Near        int // Near-lossless bound (0 = lossless)
}

// Presets carries LSE ID=1 coding parameters (T.87 C.2.4.1.1). A zero field
// selects the spec default.
type Presets struct {
	MaxVal, T1, T2, T3, Reset int
	// Seen records that an LSE ID=1 segment was present.
	Seen bool
}
