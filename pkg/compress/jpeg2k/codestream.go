package jpeg2k

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Common errors
var (
	ErrInvalidMarker    = errors.New("invalid marker")
	ErrInvalidSIZ       = errors.New("invalid SIZ marker")
	ErrInvalidCOD       = errors.New("invalid COD marker")
	ErrInvalidQCD       = errors.New("invalid QCD marker")
	ErrInvalidSOT       = errors.New("invalid SOT marker")
	ErrUnsupportedCodec = errors.New("unsupported codec feature")
)

// CodestreamReader reads JPEG 2000 codestream structure
type CodestreamReader struct {
	r   *ByteReader
	SIZ SIZMarker
	COD CODMarker
	QCD QCDMarker
}

// NewCodestreamReader creates a new codestream reader
func NewCodestreamReader(r io.Reader) *CodestreamReader {
	return &CodestreamReader{
		r: NewByteReader(r),
	}
}

// ReadMainHeader reads the main header (SOC through first SOT or SOD)
func (c *CodestreamReader) ReadMainHeader() error {
	// Read SOC marker
	marker, err := c.readMarker()
	if err != nil {
		return fmt.Errorf("reading SOC: %w", err)
	}
	if marker != MarkerSOC {
		return fmt.Errorf("%w: expected SOC (0x%04X), got 0x%04X", ErrInvalidMarker, MarkerSOC, marker)
	}

	// Read marker segments until SOT or SOD
	for {
		marker, err = c.readMarker()
		if err != nil {
			return fmt.Errorf("reading marker: %w", err)
		}

		switch marker {
		case MarkerSIZ:
			if err := c.readSIZ(); err != nil {
				return err
			}
		case MarkerCOD:
			if err := c.readCOD(); err != nil {
				return err
			}
		case MarkerCOC:
			// Skip component-specific coding style (use defaults)
			length, err := c.r.ReadUint16()
			if err != nil {
				return err
			}
			c.r.Skip(int(length) - 2)
		case MarkerQCD:
			if err := c.readQCD(); err != nil {
				return err
			}
		case MarkerQCC:
			// Skip component-specific quantization
			length, err := c.r.ReadUint16()
			if err != nil {
				return err
			}
			c.r.Skip(int(length) - 2)
		case MarkerCOM:
			// Skip comments
			length, err := c.r.ReadUint16()
			if err != nil {
				return err
			}
			c.r.Skip(int(length) - 2)
		case MarkerSOT:
			// End of main header, return for tile processing
			return nil
		case MarkerSOD:
			// Single-tile image with no SOT
			return nil
		default:
			// Skip unknown markers
			length, err := c.r.ReadUint16()
			if err != nil {
				return err
			}
			c.r.Skip(int(length) - 2)
		}
	}
}

// readMarker reads a 2-byte marker
func (c *CodestreamReader) readMarker() (uint16, error) {
	return c.r.ReadUint16()
}

// readSIZ reads the SIZ marker segment
func (c *CodestreamReader) readSIZ() error {
	length, err := c.r.ReadUint16()
	if err != nil {
		return err
	}
	if length < 41 { // Minimum SIZ length
		return ErrInvalidSIZ
	}

	c.SIZ.Rsiz, err = c.r.ReadUint16()
	if err != nil {
		return err
	}

	c.SIZ.XSiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.YSiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.XOsiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.YOsiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.XTsiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.YTsiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.XTOsiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}
	c.SIZ.YTOsiz, err = c.r.ReadUint32()
	if err != nil {
		return err
	}

	numComps, err := c.r.ReadUint16()
	if err != nil {
		return err
	}

	c.SIZ.Components = make([]ComponentInfo, numComps)
	for i := range c.SIZ.Components {
		ssiz, err := c.r.ReadByte()
		if err != nil {
			return err
		}
		c.SIZ.Components[i].Signed = (ssiz & 0x80) != 0
		c.SIZ.Components[i].Precision = int(ssiz&0x7F) + 1

		xrsiz, err := c.r.ReadByte()
		if err != nil {
			return err
		}
		c.SIZ.Components[i].XRsiz = int(xrsiz)

		yrsiz, err := c.r.ReadByte()
		if err != nil {
			return err
		}
		c.SIZ.Components[i].YRsiz = int(yrsiz)
	}

	return nil
}

// readCOD reads the COD marker segment
func (c *CodestreamReader) readCOD() error {
	length, err := c.r.ReadUint16()
	if err != nil {
		return err
	}
	if length < 12 {
		return ErrInvalidCOD
	}

	c.COD.Scod, err = c.r.ReadByte()
	if err != nil {
		return err
	}

	progOrder, err := c.r.ReadByte()
	if err != nil {
		return err
	}
	c.COD.Progression = ProgressionOrder(progOrder)

	c.COD.NumLayers, err = c.r.ReadUint16()
	if err != nil {
		return err
	}

	c.COD.MCT, err = c.r.ReadByte()
	if err != nil {
		return err
	}

	c.COD.DecompLevels, err = c.r.ReadByte()
	if err != nil {
		return err
	}

	cbWidth, err := c.r.ReadByte()
	if err != nil {
		return err
	}
	c.COD.CodeBlockWidthExp = cbWidth

	cbHeight, err := c.r.ReadByte()
	if err != nil {
		return err
	}
	c.COD.CodeBlockHeightExp = cbHeight

	c.COD.CodeBlockStyle, err = c.r.ReadByte()
	if err != nil {
		return err
	}

	transform, err := c.r.ReadByte()
	if err != nil {
		return err
	}
	c.COD.Transform = TransformType(transform)

	// Read precinct sizes if specified
	if c.COD.Scod&CodingStylePrecinctsUser != 0 {
		remaining := int(length) - 12
		c.COD.PrecinctSizes = make([]byte, remaining)
		for i := 0; i < remaining; i++ {
			c.COD.PrecinctSizes[i], err = c.r.ReadByte()
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// readQCD reads the QCD marker segment
func (c *CodestreamReader) readQCD() error {
	length, err := c.r.ReadUint16()
	if err != nil {
		return err
	}
	if length < 4 {
		return ErrInvalidQCD
	}

	sqcd, err := c.r.ReadByte()
	if err != nil {
		return err
	}
	c.QCD.Sqcd = sqcd
	c.QCD.GuardBits = (sqcd >> 5) & 0x07

	// Quantization type from lower 5 bits
	qStyle := sqcd & 0x1F

	remaining := int(length) - 3 // length - sizeof(length) - sizeof(sqcd)

	switch qStyle {
	case 0: // No quantization (reversible)
		// Each entry is 1 byte (exponent only)
		c.QCD.StepSizes = make([]int16, remaining)
		for i := 0; i < remaining; i++ {
			exp, err := c.r.ReadByte()
			if err != nil {
				return err
			}
			c.QCD.StepSizes[i] = int16(exp >> 3) // Exponent in bits 3-7
		}
	case 1: // Scalar derived
		// Single 2-byte entry
		val, err := c.r.ReadUint16()
		if err != nil {
			return err
		}
		c.QCD.StepSizes = []int16{int16(val)}
		c.r.Skip(remaining - 2)
	case 2: // Scalar expounded
		// Each entry is 2 bytes
		numEntries := remaining / 2
		c.QCD.StepSizes = make([]int16, numEntries)
		for i := 0; i < numEntries; i++ {
			val, err := c.r.ReadUint16()
			if err != nil {
				return err
			}
			c.QCD.StepSizes[i] = int16(val)
		}
	default:
		return fmt.Errorf("%w: unsupported quantization style %d", ErrInvalidQCD, qStyle)
	}

	return nil
}

// ReadSOT reads a tile-part header and returns the SOT marker data
func (c *CodestreamReader) ReadSOT() (*SOTMarker, error) {
	length, err := c.r.ReadUint16()
	if err != nil {
		return nil, err
	}
	if length != 10 {
		return nil, ErrInvalidSOT
	}

	sot := &SOTMarker{}
	sot.TileIndex, err = c.r.ReadUint16()
	if err != nil {
		return nil, err
	}
	sot.TilePartLen, err = c.r.ReadUint32()
	if err != nil {
		return nil, err
	}
	sot.TilePartIdx, err = c.r.ReadByte()
	if err != nil {
		return nil, err
	}
	sot.NumTileParts, err = c.r.ReadByte()
	if err != nil {
		return nil, err
	}

	return sot, nil
}

// ReadTilePartHeader reads markers between SOT and SOD
func (c *CodestreamReader) ReadTilePartHeader() error {
	for {
		marker, err := c.readMarker()
		if err != nil {
			return err
		}

		switch marker {
		case MarkerSOD:
			// Start of data - end of tile-part header
			return nil
		case MarkerCOD:
			if err := c.readCOD(); err != nil {
				return err
			}
		case MarkerQCD:
			if err := c.readQCD(); err != nil {
				return err
			}
		default:
			// Skip other markers
			length, err := c.r.ReadUint16()
			if err != nil {
				return err
			}
			c.r.Skip(int(length) - 2)
		}
	}
}

// Reader returns the underlying byte reader for reading tile data
func (c *CodestreamReader) Reader() *ByteReader {
	return c.r
}

// CodestreamWriter writes JPEG 2000 codestream structure
type CodestreamWriter struct {
	w *ByteWriter
}

// NewCodestreamWriter creates a new codestream writer
func NewCodestreamWriter(w io.Writer) *CodestreamWriter {
	return &CodestreamWriter{
		w: NewByteWriter(w),
	}
}

// WriteSOC writes the Start of Codestream marker
func (c *CodestreamWriter) WriteSOC() error {
	return c.w.WriteUint16(MarkerSOC)
}

// WriteSIZ writes the SIZ marker segment
func (c *CodestreamWriter) WriteSIZ(siz *SIZMarker) error {
	if err := c.w.WriteUint16(MarkerSIZ); err != nil {
		return err
	}

	// Length: 38 + 3*numComponents
	length := uint16(38 + 3*len(siz.Components))
	if err := c.w.WriteUint16(length); err != nil {
		return err
	}

	if err := c.w.WriteUint16(siz.Rsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.XSiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.YSiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.XOsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.YOsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.XTsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.YTsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.XTOsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint32(siz.YTOsiz); err != nil {
		return err
	}
	if err := c.w.WriteUint16(uint16(len(siz.Components))); err != nil {
		return err
	}

	for _, comp := range siz.Components {
		ssiz := byte(comp.Precision - 1)
		if comp.Signed {
			ssiz |= 0x80
		}
		if err := c.w.WriteByte(ssiz); err != nil {
			return err
		}
		if err := c.w.WriteByte(byte(comp.XRsiz)); err != nil {
			return err
		}
		if err := c.w.WriteByte(byte(comp.YRsiz)); err != nil {
			return err
		}
	}

	return nil
}

// WriteCOD writes the COD marker segment
func (c *CodestreamWriter) WriteCOD(cod *CODMarker) error {
	if err := c.w.WriteUint16(MarkerCOD); err != nil {
		return err
	}

	// Length: 12 + precinct sizes
	length := uint16(12)
	if cod.Scod&CodingStylePrecinctsUser != 0 {
		length += uint16(len(cod.PrecinctSizes))
	}
	if err := c.w.WriteUint16(length); err != nil {
		return err
	}

	if err := c.w.WriteByte(cod.Scod); err != nil {
		return err
	}
	if err := c.w.WriteByte(byte(cod.Progression)); err != nil {
		return err
	}
	if err := c.w.WriteUint16(cod.NumLayers); err != nil {
		return err
	}
	if err := c.w.WriteByte(cod.MCT); err != nil {
		return err
	}
	if err := c.w.WriteByte(cod.DecompLevels); err != nil {
		return err
	}
	if err := c.w.WriteByte(cod.CodeBlockWidthExp); err != nil {
		return err
	}
	if err := c.w.WriteByte(cod.CodeBlockHeightExp); err != nil {
		return err
	}
	if err := c.w.WriteByte(cod.CodeBlockStyle); err != nil {
		return err
	}
	if err := c.w.WriteByte(byte(cod.Transform)); err != nil {
		return err
	}

	if cod.Scod&CodingStylePrecinctsUser != 0 {
		if err := c.w.WriteBytes(cod.PrecinctSizes); err != nil {
			return err
		}
	}

	return nil
}

// WriteQCD writes the QCD marker segment for reversible (lossless) coding
func (c *CodestreamWriter) WriteQCD(qcd *QCDMarker) error {
	if err := c.w.WriteUint16(MarkerQCD); err != nil {
		return err
	}

	// Length: 3 + step sizes
	length := uint16(3 + len(qcd.StepSizes))
	if err := c.w.WriteUint16(length); err != nil {
		return err
	}

	// Sqcd: guard bits in upper 3 bits, quantization type in lower 5
	sqcd := (qcd.GuardBits << 5) | (qcd.Sqcd & 0x1F)
	if err := c.w.WriteByte(sqcd); err != nil {
		return err
	}

	// For reversible coding, each step size is 1 byte (exponent only)
	for _, step := range qcd.StepSizes {
		exp := byte(step << 3) // Exponent in bits 3-7
		if err := c.w.WriteByte(exp); err != nil {
			return err
		}
	}

	return nil
}

// WriteSOT writes a tile-part header
func (c *CodestreamWriter) WriteSOT(sot *SOTMarker) error {
	if err := c.w.WriteUint16(MarkerSOT); err != nil {
		return err
	}
	if err := c.w.WriteUint16(10); err != nil { // Fixed length
		return err
	}
	if err := c.w.WriteUint16(sot.TileIndex); err != nil {
		return err
	}
	if err := c.w.WriteUint32(sot.TilePartLen); err != nil {
		return err
	}
	if err := c.w.WriteByte(sot.TilePartIdx); err != nil {
		return err
	}
	return c.w.WriteByte(sot.NumTileParts)
}

// WriteSOD writes the Start of Data marker
func (c *CodestreamWriter) WriteSOD() error {
	return c.w.WriteUint16(MarkerSOD)
}

// WriteEOC writes the End of Codestream marker
func (c *CodestreamWriter) WriteEOC() error {
	return c.w.WriteUint16(MarkerEOC)
}

// WriteBytes writes raw bytes
func (c *CodestreamWriter) WriteBytes(data []byte) error {
	return c.w.WriteBytes(data)
}

// Flush flushes the underlying buffer
func (c *CodestreamWriter) Flush() error {
	return c.w.Flush()
}

// Writer returns the underlying byte writer
func (c *CodestreamWriter) Writer() *ByteWriter {
	return c.w
}

// BuildDefaultCOD creates a default COD marker for lossless encoding
func BuildDefaultCOD(decompLevels int, numLayers int, progression ProgressionOrder, useMCT bool) *CODMarker {
	cod := &CODMarker{
		Scod:             0, // No user-defined precincts, no SOP/EPH
		Progression:      progression,
		NumLayers:        uint16(numLayers),
		MCT:              0,
		DecompLevels:     byte(decompLevels),
		CodeBlockWidthExp:  4, // 64x64 code-blocks
		CodeBlockHeightExp: 4,
		CodeBlockStyle:   0,
		Transform:        TransformReversible53, // Lossless
	}
	if useMCT {
		cod.MCT = 1
	}
	return cod
}

// BuildDefaultQCD creates a default QCD marker for lossless encoding
func BuildDefaultQCD(decompLevels int, guardBits int) *QCDMarker {
	// For reversible coding: 3*levels + 1 subbands (LL + 3 subbands per level)
	numSubbands := 3*decompLevels + 1
	qcd := &QCDMarker{
		Sqcd:      0, // Reversible, no quantization
		GuardBits: byte(guardBits),
		StepSizes: make([]int16, numSubbands),
	}
	// All exponents set to 0 for reversible
	return qcd
}

// BuildSIZ creates a SIZ marker from image parameters
func BuildSIZ(width, height int, components []ComponentInfo, tileWidth, tileHeight int) *SIZMarker {
	if tileWidth == 0 {
		tileWidth = width
	}
	if tileHeight == 0 {
		tileHeight = height
	}

	return &SIZMarker{
		Rsiz:       0, // Baseline
		XSiz:       uint32(width),
		YSiz:       uint32(height),
		XOsiz:      0,
		YOsiz:      0,
		XTsiz:      uint32(tileWidth),
		YTsiz:      uint32(tileHeight),
		XTOsiz:     0,
		YTOsiz:     0,
		Components: components,
	}
}

// ParseCodestreamHeader parses a JPEG 2000 codestream and returns header info
func ParseCodestreamHeader(data []byte) (*SIZMarker, *CODMarker, *QCDMarker, error) {
	if len(data) < 4 {
		return nil, nil, nil, errors.New("codestream too short")
	}

	// Check SOC marker
	if binary.BigEndian.Uint16(data[0:2]) != MarkerSOC {
		return nil, nil, nil, ErrInvalidMarker
	}

	var siz SIZMarker
	var cod CODMarker
	var qcd QCDMarker

	pos := 2
	for pos < len(data)-2 {
		marker := binary.BigEndian.Uint16(data[pos : pos+2])
		pos += 2

		if marker == MarkerSOT || marker == MarkerSOD {
			break
		}

		if pos+2 > len(data) {
			break
		}
		length := int(binary.BigEndian.Uint16(data[pos : pos+2]))
		if pos+length > len(data) {
			break
		}

		segment := data[pos : pos+length]
		pos += length

		switch marker {
		case MarkerSIZ:
			if err := parseSIZSegment(segment, &siz); err != nil {
				return nil, nil, nil, err
			}
		case MarkerCOD:
			if err := parseCODSegment(segment, &cod); err != nil {
				return nil, nil, nil, err
			}
		case MarkerQCD:
			if err := parseQCDSegment(segment, &qcd); err != nil {
				return nil, nil, nil, err
			}
		}
	}

	return &siz, &cod, &qcd, nil
}

func parseSIZSegment(data []byte, siz *SIZMarker) error {
	if len(data) < 38 {
		return ErrInvalidSIZ
	}
	siz.Rsiz = binary.BigEndian.Uint16(data[2:4])
	siz.XSiz = binary.BigEndian.Uint32(data[4:8])
	siz.YSiz = binary.BigEndian.Uint32(data[8:12])
	siz.XOsiz = binary.BigEndian.Uint32(data[12:16])
	siz.YOsiz = binary.BigEndian.Uint32(data[16:20])
	siz.XTsiz = binary.BigEndian.Uint32(data[20:24])
	siz.YTsiz = binary.BigEndian.Uint32(data[24:28])
	siz.XTOsiz = binary.BigEndian.Uint32(data[28:32])
	siz.YTOsiz = binary.BigEndian.Uint32(data[32:36])
	numComps := int(binary.BigEndian.Uint16(data[36:38]))

	siz.Components = make([]ComponentInfo, numComps)
	pos := 38
	for i := 0; i < numComps && pos+3 <= len(data); i++ {
		ssiz := data[pos]
		siz.Components[i].Signed = (ssiz & 0x80) != 0
		siz.Components[i].Precision = int(ssiz&0x7F) + 1
		siz.Components[i].XRsiz = int(data[pos+1])
		siz.Components[i].YRsiz = int(data[pos+2])
		pos += 3
	}
	return nil
}

func parseCODSegment(data []byte, cod *CODMarker) error {
	if len(data) < 12 {
		return ErrInvalidCOD
	}
	cod.Scod = data[2]
	cod.Progression = ProgressionOrder(data[3])
	cod.NumLayers = binary.BigEndian.Uint16(data[4:6])
	cod.MCT = data[6]
	cod.DecompLevels = data[7]
	cod.CodeBlockWidthExp = data[8]
	cod.CodeBlockHeightExp = data[9]
	cod.CodeBlockStyle = data[10]
	cod.Transform = TransformType(data[11])

	if cod.Scod&CodingStylePrecinctsUser != 0 && len(data) > 12 {
		cod.PrecinctSizes = make([]byte, len(data)-12)
		copy(cod.PrecinctSizes, data[12:])
	}
	return nil
}

func parseQCDSegment(data []byte, qcd *QCDMarker) error {
	if len(data) < 3 {
		return ErrInvalidQCD
	}
	sqcd := data[2]
	qcd.Sqcd = sqcd & 0x1F
	qcd.GuardBits = (sqcd >> 5) & 0x07

	remaining := len(data) - 3
	if qcd.Sqcd == 0 { // Reversible
		qcd.StepSizes = make([]int16, remaining)
		for i := 0; i < remaining; i++ {
			qcd.StepSizes[i] = int16(data[3+i] >> 3)
		}
	}
	return nil
}
