package jpegls

// Markers
const (
	MarkerSOI   = 0xFFD8 // Start of Image
	MarkerEOI   = 0xFFD9 // End of Image
	MarkerSOS   = 0xFFDA // Start of Scan
	MarkerDNL   = 0xFFDC
	MarkerDRI   = 0xFFDD // Define Restart Interval
	MarkerLSE   = 0xFFF8 // JPEG-LS Extension (Parameters)
	MarkerSOF55 = 0xFFF7 // Start of Frame (JPEG-LS)
)

// Parameters
type FrameHeader struct {
	Precision  int
	Height     int
	Width      int
	Components int
}

type ScanHeader struct {
	Components int
	Near       int // Near-lossless parameter (0 = lossless)
	ILV        int // Interleave mode
	Al         int // Point transform
	Ah         int
}
