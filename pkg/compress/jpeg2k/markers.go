// Package jpeg2k implements JPEG 2000 (Part-1) encoding and decoding.
// This implementation supports lossless compression using the 5/3 reversible
// discrete wavelet transform as specified in ITU-T Rec. T.800 | ISO/IEC 15444-1.
package jpeg2k

// JPEG 2000 Marker codes (ITU-T T.800 Table A.1)
const (
	// Delimiting markers
	MarkerSOC = 0xFF4F // Start of codestream
	MarkerSOT = 0xFF90 // Start of tile-part
	MarkerSOD = 0xFFD3 // Start of data
	MarkerEOC = 0xFFD9 // End of codestream

	// Fixed information markers
	MarkerSIZ = 0xFF51 // Image and tile size

	// Functional markers
	MarkerCOD = 0xFF52 // Coding style default
	MarkerCOC = 0xFF53 // Coding style component
	MarkerRGN = 0xFF5E // Region of interest
	MarkerQCD = 0xFF5C // Quantization default
	MarkerQCC = 0xFF5D // Quantization component
	MarkerPOC = 0xFF5F // Progression order change

	// Pointer markers
	MarkerTLM = 0xFF55 // Tile-part lengths
	MarkerPLM = 0xFF57 // Packet length, main header
	MarkerPLT = 0xFF58 // Packet length, tile-part header
	MarkerPPM = 0xFF60 // Packed packet headers, main header
	MarkerPPT = 0xFF61 // Packed packet headers, tile-part header

	// In-bitstream markers
	MarkerSOP = 0xFF91 // Start of packet
	MarkerEPH = 0xFF92 // End of packet header

	// Informational markers
	MarkerCRG = 0xFF63 // Component registration
	MarkerCOM = 0xFF64 // Comment
)

// ProgressionOrder defines the progression order for JPEG 2000 codestream
type ProgressionOrder byte

const (
	ProgressionLRCP ProgressionOrder = 0 // Layer-Resolution-Component-Position
	ProgressionRLCP ProgressionOrder = 1 // Resolution-Layer-Component-Position
	ProgressionRPCL ProgressionOrder = 2 // Resolution-Position-Component-Layer
	ProgressionPCRL ProgressionOrder = 3 // Position-Component-Resolution-Layer
	ProgressionCPRL ProgressionOrder = 4 // Component-Position-Resolution-Layer
)

// String returns the progression order name
func (p ProgressionOrder) String() string {
	switch p {
	case ProgressionLRCP:
		return "LRCP"
	case ProgressionRLCP:
		return "RLCP"
	case ProgressionRPCL:
		return "RPCL"
	case ProgressionPCRL:
		return "PCRL"
	case ProgressionCPRL:
		return "CPRL"
	default:
		return "Unknown"
	}
}

// CodingStyle flags (ITU-T T.800 Table A.13)
const (
	CodingStylePrecinctsUser   = 0x01 // Custom precinct sizes
	CodingStyleSOPMarker       = 0x02 // SOP marker segments used
	CodingStyleEPHMarker       = 0x04 // EPH marker segments used
	CodingStyleVariablePrecincts = 0x01 // User defined precinct sizes
)

// CodeBlockStyle flags (ITU-T T.800 Table A.15)
const (
	CodeBlockSelectiveBypass    = 0x01 // Selective arithmetic coding bypass
	CodeBlockResetContext       = 0x02 // Reset context on coding pass boundary
	CodeBlockTermOnPass         = 0x04 // Termination on each coding pass
	CodeBlockVerticalCausal     = 0x08 // Vertically causal context
	CodeBlockPredictableTermination = 0x10 // Predictable termination
	CodeBlockSegmentationSymbols   = 0x20 // Segmentation symbols used
)

// TransformType identifies the wavelet transform type
type TransformType byte

const (
	TransformIrreversible97 TransformType = 0 // 9/7 irreversible (lossy)
	TransformReversible53   TransformType = 1 // 5/3 reversible (lossless)
)

// ComponentInfo holds component-specific information from SIZ marker
type ComponentInfo struct {
	Precision  int  // Bit depth (1-38)
	Signed     bool // True if signed samples
	XRsiz      int  // Horizontal sample separation
	YRsiz      int  // Vertical sample separation
}

// SIZMarker holds image and tile size parameters (ITU-T T.800 A.5.1)
type SIZMarker struct {
	Rsiz       uint16          // Capabilities required
	XSiz       uint32          // Reference grid width
	YSiz       uint32          // Reference grid height
	XOsiz      uint32          // Horizontal offset
	YOsiz      uint32          // Vertical offset
	XTsiz      uint32          // Tile width
	YTsiz      uint32          // Tile height
	XTOsiz     uint32          // Tile horizontal offset
	YTOsiz     uint32          // Tile vertical offset
	Components []ComponentInfo // Per-component info
}

// NumXTiles returns the number of tiles horizontally
func (s *SIZMarker) NumXTiles() int {
	return int((s.XSiz - s.XTOsiz + s.XTsiz - 1) / s.XTsiz)
}

// NumYTiles returns the number of tiles vertically
func (s *SIZMarker) NumYTiles() int {
	return int((s.YSiz - s.YTOsiz + s.YTsiz - 1) / s.YTsiz)
}

// NumTiles returns the total number of tiles
func (s *SIZMarker) NumTiles() int {
	return s.NumXTiles() * s.NumYTiles()
}

// CODMarker holds coding style default parameters (ITU-T T.800 A.6.1)
type CODMarker struct {
	Scod             byte             // Coding style
	Progression      ProgressionOrder // Progression order
	NumLayers        uint16           // Number of quality layers
	MCT              byte             // Multiple component transform (0=none, 1=RCT/ICT)
	DecompLevels     byte             // Number of decomposition levels
	CodeBlockWidthExp  byte           // Code-block width exponent (add 2)
	CodeBlockHeightExp byte           // Code-block height exponent (add 2)
	CodeBlockStyle   byte             // Code-block style flags
	Transform        TransformType    // Wavelet transform type
	PrecinctSizes    []byte           // Precinct sizes (if Scod & 0x01)
}

// CodeBlockWidth returns the actual code-block width
func (c *CODMarker) CodeBlockWidth() int {
	return 1 << (c.CodeBlockWidthExp + 2)
}

// CodeBlockHeight returns the actual code-block height
func (c *CODMarker) CodeBlockHeight() int {
	return 1 << (c.CodeBlockHeightExp + 2)
}

// QCDMarker holds quantization default parameters (ITU-T T.800 A.6.4)
type QCDMarker struct {
	Sqcd        byte    // Quantization style
	GuardBits   byte    // Number of guard bits
	StepSizes   []int16 // Quantization step sizes (for reversible, these are exponents)
}

// SOTMarker holds tile-part header parameters (ITU-T T.800 A.4.2)
type SOTMarker struct {
	TileIndex  uint16 // Tile index
	TilePartLen uint32 // Length of tile-part
	TilePartIdx byte   // Tile-part index
	NumTileParts byte  // Number of tile-parts (0 = not specified)
}

// COMMarker holds comment data (ITU-T T.800 A.9.2)
type COMMarker struct {
	Registration uint16 // Registration value (0=binary, 1=Latin-1)
	Data         []byte // Comment data
}

// Subband identifies a subband in the DWT decomposition
type Subband int

const (
	SubbandLL Subband = 0 // Low-Low (approximation)
	SubbandHL Subband = 1 // High-Low (horizontal detail)
	SubbandLH Subband = 2 // Low-High (vertical detail)
	SubbandHH Subband = 3 // High-High (diagonal detail)
)

// String returns the subband name
func (s Subband) String() string {
	switch s {
	case SubbandLL:
		return "LL"
	case SubbandHL:
		return "HL"
	case SubbandLH:
		return "LH"
	case SubbandHH:
		return "HH"
	default:
		return "Unknown"
	}
}

// ResolutionLevel represents a resolution level in the wavelet decomposition
type ResolutionLevel struct {
	Level      int       // Resolution level (0 = lowest)
	Width      int       // Width at this resolution
	Height     int       // Height at this resolution
	Subbands   []Subband // Subbands at this level
}

// CodeBlock represents a code-block within a subband
type CodeBlock struct {
	X0, Y0     int    // Top-left corner in subband coordinates
	X1, Y1     int    // Bottom-right corner (exclusive)
	Data       []int  // Coefficient data
	Passes     int    // Number of coding passes included
	NumZBP     int    // Number of zero bit-planes
	Length     int    // Coded data length
	CodedData  []byte // Encoded data
}

// Precinct represents a precinct (spatial partition of packets)
type Precinct struct {
	X0, Y0     int          // Top-left corner
	X1, Y1     int          // Bottom-right corner
	CodeBlocks []*CodeBlock // Code-blocks in this precinct
}

// Packet represents a packet (atomic unit of progression)
type Packet struct {
	Layer      int        // Quality layer index
	Resolution int        // Resolution level
	Component  int        // Component index
	Precinct   int        // Precinct index
	Empty      bool       // True if packet contains no data
	Header     []byte     // Packet header
	Data       []byte     // Packet body (coded code-block data)
}
