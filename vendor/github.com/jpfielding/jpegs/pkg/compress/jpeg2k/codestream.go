package jpeg2k

import (
	"encoding/binary"
	"fmt"
)

// Codestream markers (ITU-T T.800 Table A.2).
const (
	MarkerSOC = 0xFF4F // start of codestream
	MarkerSIZ = 0xFF51 // image and tile size
	MarkerCOD = 0xFF52 // coding style default
	MarkerCOC = 0xFF53 // coding style component
	MarkerTLM = 0xFF55 // tile-part lengths
	MarkerPLM = 0xFF57 // packet length, main header
	MarkerPLT = 0xFF58 // packet length, tile-part header
	MarkerQCD = 0xFF5C // quantization default
	MarkerQCC = 0xFF5D // quantization component
	MarkerRGN = 0xFF5E // region of interest
	MarkerPOC = 0xFF5F // progression order change
	MarkerPPM = 0xFF60 // packed packet headers, main header
	MarkerPPT = 0xFF61 // packed packet headers, tile-part header
	MarkerCRG = 0xFF63 // component registration
	MarkerCOM = 0xFF64 // comment
	MarkerSOT = 0xFF90 // start of tile-part
	MarkerSOD = 0xFF93 // start of data
	MarkerEOC = 0xFFD9 // end of codestream

	// MarkerSODLegacy is the (non-T.800) SOD code written by this package
	// before v0.3.0 and by dicos.rs 1.0.0. Only legacy streams may use it.
	MarkerSODLegacy = 0xFFD3
)

// COD Scod flags.
const (
	scodPrecincts = 0x01
	scodSOP       = 0x02
	scodEPH       = 0x04
)

// Profile constants.
const (
	bitDepth  = 16                  // unsigned 16-bit samples (RI)
	guardBits = 2                   // G (T.800 E.1)
	dcShift   = 1 << (bitDepth - 1) // DC level shift for unsigned samples
)

// qcdEpsilon is εb = RI + log2(gain_b) for reversible coding (T.800 E-10).
func qcdEpsilon(kind bandKind) int {
	_, _, gain := bandOffsets(kind)
	return bitDepth + gain
}

// qcdEpsilons lists SPqcd exponents in T.800 order: LL, then HL, LH, HH
// for n = NL..1.
func qcdEpsilons(levels int) []uint8 {
	v := []uint8{uint8(qcdEpsilon(bandLL))}
	for range levels {
		v = append(v, uint8(qcdEpsilon(bandHL)), uint8(qcdEpsilon(bandLH)), uint8(qcdEpsilon(bandHH)))
	}
	return v
}

// qcdIndex locates band kind at decomposition level in the QCD list.
func qcdIndex(kind bandKind, level, levels int) int {
	if kind == bandLL {
		return 0
	}
	return int(kind) + 3*(levels-level) // HL=1, LH=2, HH=3
}

// sizComponent is one SIZ component entry.
type sizComponent struct {
	precision int // bit depth, 1..38
	signed    bool
	dx, dy    uint8 // XRsiz, YRsiz
}

// header is everything recovered from one bounds-checked walk of a
// codestream. It is structurally valid but not yet checked against the
// supported profile (see validateStandard and legacy.go).
type header struct {
	xsiz, ysiz, xosiz, yosiz     uint32
	xtsiz, ytsiz, xtosiz, ytosiz uint32
	comps                        []sizComponent

	scod, progression  uint8
	layers             uint16
	mct, levels        uint8
	cbw, cbh           uint8 // stored exponents (actual - 2)
	cbStyle, transform uint8

	guard uint8
	exps  []uint8 // SPqcd exponents εb

	sodLegacy bool // SOD was 0xFFD3
	shortPsot bool // Psot = 12 + len (the pre-T.800 Go encoder)
	payload   []byte
}

func (h *header) width() int  { return int(h.xsiz - h.xosiz) }
func (h *header) height() int { return int(h.ysiz - h.yosiz) }

// cursor is a bounds-checked big-endian reader over a byte slice.
type cursor struct {
	data []byte
	pos  int
}

func errTruncated(what string) error {
	return fmt.Errorf("%w: codestream truncated in %s", ErrInvalidData, what)
}

func (c *cursor) u8(what string) (uint8, error) {
	if c.pos+1 > len(c.data) {
		return 0, errTruncated(what)
	}
	c.pos++
	return c.data[c.pos-1], nil
}

func (c *cursor) u16(what string) (uint16, error) {
	if c.pos+2 > len(c.data) {
		return 0, errTruncated(what)
	}
	c.pos += 2
	return binary.BigEndian.Uint16(c.data[c.pos-2:]), nil
}

func (c *cursor) u32(what string) (uint32, error) {
	if c.pos+4 > len(c.data) {
		return 0, errTruncated(what)
	}
	c.pos += 4
	return binary.BigEndian.Uint32(c.data[c.pos-4:]), nil
}

// segment reads a marker segment length and returns a cursor over exactly
// its content, advancing c past it.
func (c *cursor) segment(name string) (*cursor, error) {
	n, err := c.u16(name + " length")
	if err != nil {
		return nil, err
	}
	if n < 2 {
		return nil, fmt.Errorf("%w: %s segment length %d < 2", ErrInvalidData, name, n)
	}
	end := c.pos + int(n) - 2
	if end > len(c.data) {
		return nil, errTruncated(name)
	}
	seg := &cursor{data: c.data[c.pos:end]}
	c.pos = end
	return seg, nil
}

// done errors if the segment has unread bytes.
func (c *cursor) done(name string) error {
	if c.pos != len(c.data) {
		return fmt.Errorf("%w: %s segment has %d unexpected trailing bytes", ErrInvalidData, name, len(c.data)-c.pos)
	}
	return nil
}

func (h *header) readSIZ(c *cursor) error {
	s, err := c.segment("SIZ")
	if err != nil {
		return err
	}
	if _, err = s.u16("SIZ"); err != nil { // Rsiz
		return err
	}
	for _, f := range []*uint32{&h.xsiz, &h.ysiz, &h.xosiz, &h.yosiz, &h.xtsiz, &h.ytsiz, &h.xtosiz, &h.ytosiz} {
		if *f, err = s.u32("SIZ"); err != nil {
			return err
		}
	}
	n, err := s.u16("SIZ")
	if err != nil {
		return err
	}
	if n == 0 || n > 16384 {
		return fmt.Errorf("%w: SIZ Csiz %d", ErrInvalidData, n)
	}
	if len(s.data)+2 != 38+3*int(n) {
		return fmt.Errorf("%w: SIZ length %d for %d components", ErrInvalidData, len(s.data)+2, n)
	}
	h.comps = make([]sizComponent, n)
	for i := range h.comps {
		ssiz, _ := s.u8("SIZ")
		dx, _ := s.u8("SIZ")
		dy, _ := s.u8("SIZ")
		if ssiz&0x7F > 37 || dx == 0 || dy == 0 {
			return fmt.Errorf("%w: SIZ component %d (Ssiz 0x%02X, sub-sampling %dx%d)", ErrInvalidData, i, ssiz, dx, dy)
		}
		h.comps[i] = sizComponent{precision: int(ssiz&0x7F) + 1, signed: ssiz&0x80 != 0, dx: dx, dy: dy}
	}
	switch {
	case h.xsiz <= h.xosiz || h.ysiz <= h.yosiz:
		return fmt.Errorf("%w: SIZ image area is empty (%d-%d x %d-%d)", ErrInvalidData, h.xsiz, h.xosiz, h.ysiz, h.yosiz)
	case h.xtsiz == 0 || h.ytsiz == 0:
		return fmt.Errorf("%w: SIZ tile size has a zero dimension", ErrInvalidData)
	}
	return nil
}

func (h *header) readCOD(c *cursor) error {
	s, err := c.segment("COD")
	if err != nil {
		return err
	}
	if len(s.data) < 10 {
		return errTruncated("COD")
	}
	h.scod, _ = s.u8("COD")
	h.progression, _ = s.u8("COD")
	h.layers, _ = s.u16("COD")
	h.mct, _ = s.u8("COD")
	h.levels, _ = s.u8("COD")
	h.cbw, _ = s.u8("COD")
	h.cbh, _ = s.u8("COD")
	h.cbStyle, _ = s.u8("COD")
	h.transform, _ = s.u8("COD")
	switch {
	case h.progression > 4:
		return fmt.Errorf("%w: unknown progression order %d", ErrInvalidData, h.progression)
	case h.transform > 1:
		return fmt.Errorf("%w: unknown transform %d", ErrInvalidData, h.transform)
	case h.levels > 32:
		return fmt.Errorf("%w: %d decomposition levels exceed 32", ErrInvalidData, h.levels)
	case h.cbw > 8 || h.cbh > 8 || h.cbw+h.cbh > 8:
		return fmt.Errorf("%w: illegal code-block exponents (stored %d,%d)", ErrInvalidData, h.cbw, h.cbh)
	}
	// User precinct sizes, one byte per resolution, follow when signalled.
	want := 10
	if h.scod&scodPrecincts != 0 {
		want += int(h.levels) + 1
	}
	if len(s.data) != want {
		return fmt.Errorf("%w: COD length %d, want %d", ErrInvalidData, len(s.data)+2, want+2)
	}
	return nil
}

func (h *header) readQCD(c *cursor) error {
	s, err := c.segment("QCD")
	if err != nil {
		return err
	}
	sqcd, err := s.u8("QCD")
	if err != nil {
		return err
	}
	h.guard = sqcd >> 5
	if style := sqcd & 0x1F; style != 0 {
		return fmt.Errorf("%w: only reversible (no quantization) QCD, style %d", ErrUnsupported, style)
	}
	h.exps = make([]uint8, 0, len(s.data)-1)
	for s.pos < len(s.data) {
		b, _ := s.u8("QCD")
		h.exps = append(h.exps, b>>3)
	}
	return nil
}

func unsupportedMarker(name string) error {
	return fmt.Errorf("%w: %s marker", ErrUnsupported, name)
}

// parseCodestream walks SOC, the main header, one SOT/SOD tile-part and EOC.
func parseCodestream(data []byte) (*header, error) {
	c := &cursor{data: data}
	soc, err := c.u16("SOC")
	if err != nil {
		return nil, err
	}
	if soc != MarkerSOC {
		return nil, fmt.Errorf("%w: missing SOC marker", ErrInvalidData)
	}
	h := &header{}
	var haveSIZ, haveCOD, haveQCD bool
	once := func(seen *bool, name string) error {
		if *seen {
			return fmt.Errorf("%w: duplicate %s marker", ErrInvalidData, name)
		}
		*seen = true
		return nil
	}
	sotOff := -1
	for sotOff < 0 {
		off := c.pos
		m, err := c.u16("main header")
		if err != nil {
			return nil, err
		}
		switch m {
		case MarkerSIZ:
			if err = once(&haveSIZ, "SIZ"); err == nil {
				err = h.readSIZ(c)
			}
		case MarkerCOD:
			if err = once(&haveCOD, "COD"); err == nil {
				err = h.readCOD(c)
			}
		case MarkerQCD:
			if err = once(&haveQCD, "QCD"); err == nil {
				err = h.readQCD(c)
			}
		case MarkerSOT:
			sotOff = off
		case MarkerCOC:
			err = unsupportedMarker("COC")
		case MarkerQCC:
			err = unsupportedMarker("QCC")
		case MarkerRGN:
			err = unsupportedMarker("RGN")
		case MarkerPOC:
			err = unsupportedMarker("POC")
		case MarkerPPM:
			err = unsupportedMarker("PPM")
		case MarkerPLM:
			err = unsupportedMarker("PLM")
		case MarkerCRG, MarkerCOM, MarkerTLM:
			_, err = c.segment("informational")
		default:
			err = fmt.Errorf("%w: unexpected marker 0x%04X in main header", ErrInvalidData, m)
		}
		if err != nil {
			return nil, err
		}
	}
	switch {
	case !haveSIZ:
		return nil, fmt.Errorf("%w: missing SIZ marker", ErrInvalidData)
	case !haveCOD:
		return nil, fmt.Errorf("%w: missing COD marker", ErrInvalidData)
	case !haveQCD:
		return nil, fmt.Errorf("%w: missing QCD marker", ErrInvalidData)
	}

	sot, err := c.segment("SOT")
	if err != nil {
		return nil, err
	}
	if len(sot.data) != 8 {
		return nil, fmt.Errorf("%w: SOT length %d", ErrInvalidData, len(sot.data)+2)
	}
	isot, _ := sot.u16("SOT")
	psot, _ := sot.u32("SOT")
	tpsot, _ := sot.u8("SOT")
	tnsot, _ := sot.u8("SOT")
	switch {
	case isot != 0:
		return nil, fmt.Errorf("%w: tile index %d (single tile only)", ErrUnsupported, isot)
	case tpsot != 0:
		return nil, fmt.Errorf("%w: tile-part index %d (single tile-part only)", ErrUnsupported, tpsot)
	case tnsot > 1:
		return nil, fmt.Errorf("%w: %d tile-parts (single tile-part only)", ErrUnsupported, tnsot)
	}

	for start := -1; start < 0; {
		m, err := c.u16("tile-part header")
		if err != nil {
			return nil, err
		}
		switch m {
		case MarkerSOD, MarkerSODLegacy:
			h.sodLegacy = m == MarkerSODLegacy
			start = c.pos
		case MarkerCRG, MarkerCOM:
			_, err = c.segment("informational")
		case MarkerPPT:
			err = unsupportedMarker("PPT")
		case MarkerPLT:
			err = unsupportedMarker("PLT")
		default:
			err = fmt.Errorf("%w: expected SOD after SOT, got 0x%04X", ErrInvalidData, m)
		}
		if err != nil {
			return nil, err
		}
	}
	start := c.pos

	// The tile-part (Psot bytes from the SOT marker, T.800 A.4.2, or to EOC
	// when Psot is 0) must be followed by EOC and nothing else.
	endsEOC := func(at int) bool {
		return at >= start && len(data)-at == 2 && binary.BigEndian.Uint16(data[at:]) == MarkerEOC
	}
	end := len(data) - 2
	switch {
	case psot == 0:
		if !endsEOC(end) {
			return nil, fmt.Errorf("%w: Psot is 0 but the codestream does not end with EOC", ErrInvalidData)
		}
	case uint64(sotOff)+uint64(psot) > uint64(len(data)):
		return nil, fmt.Errorf("%w: Psot %d exceeds the codestream", ErrInvalidData, psot)
	case endsEOC(sotOff + int(psot)):
	case h.sodLegacy && endsEOC(sotOff+int(psot)+2):
		h.shortPsot = true
	default:
		return nil, fmt.Errorf("%w: tile-part is not followed by EOC (Psot %d, trailing data?)", ErrInvalidData, psot)
	}
	h.payload = data[start:end]
	return h, nil
}

// validateStandard applies the supported T.800 profile and returns Mb per
// band. Everything legal but outside the profile is ErrUnsupported.
func (h *header) validateStandard() (func(bandKind, int) int, error) {
	unsupported := func(format string, a ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrUnsupported}, a...)...)
	}
	invalid := func(format string, a ...any) error {
		return fmt.Errorf("%w: "+format, append([]any{ErrInvalidData}, a...)...)
	}
	switch {
	case h.sodLegacy || h.shortPsot:
		return nil, invalid("non-T.800 SOD marker or tile-part length (legacy stream?)")
	case len(h.comps) != 1:
		return nil, unsupported("%d components (single component only)", len(h.comps))
	case h.comps[0].signed || h.comps[0].precision != bitDepth:
		return nil, unsupported("%d-bit signed=%v samples (unsigned 16-bit only)", h.comps[0].precision, h.comps[0].signed)
	case h.comps[0].dx != 1 || h.comps[0].dy != 1:
		return nil, unsupported("component sub-sampling %dx%d", h.comps[0].dx, h.comps[0].dy)
	case h.xosiz != 0 || h.yosiz != 0 || h.xtosiz != 0 || h.ytosiz != 0:
		return nil, unsupported("non-zero grid origins")
	case h.xtsiz < h.xsiz || h.ytsiz < h.ysiz:
		return nil, unsupported("multi-tile images")
	case h.scod&(scodPrecincts|scodSOP|scodEPH) != 0:
		return nil, unsupported("COD precinct/SOP/EPH flags 0x%02X", h.scod)
	case h.progression != 0:
		return nil, unsupported("progression order %d (LRCP only)", h.progression)
	case h.layers != 1:
		return nil, unsupported("%d quality layers (one only)", h.layers)
	case h.mct != 0:
		return nil, unsupported("multiple-component transform")
	case h.transform != 1:
		return nil, unsupported("irreversible 9/7 transform")
	case h.cbStyle != 0:
		return nil, unsupported("code-block style 0x%02X", h.cbStyle)
	}
	levels := int(h.levels)
	nBands := 3*levels + 1
	if allZero(h.exps) {
		return nil, invalid("all-zero QCD exponents (legacy stream?)")
	}
	if len(h.exps) < nBands {
		return nil, invalid("QCD has %d exponents, need %d for %d levels", len(h.exps), nBands, levels)
	}
	// Mb = εb + G - 1 must be >= 0 (εb = G = 0 would underflow) and < 32 so
	// tier-1 shifts and int32 magnitudes stay in range.
	mbs := make([]int, nBands)
	for i, e := range h.exps[:nBands] {
		mb := int(e) + int(h.guard) - 1
		switch {
		case mb < 0:
			return nil, invalid("QCD band %d has εb = G = 0", i)
		case mb >= 32:
			return nil, unsupported("QCD band %d Mb %d (εb %d, G %d) exceeds 31 bit-planes", i, mb, e, h.guard)
		}
		mbs[i] = mb
	}
	return func(kind bandKind, level int) int { return mbs[qcdIndex(kind, level, levels)] }, nil
}

func allZero(b []uint8) bool {
	for _, v := range b {
		if v != 0 {
			return false
		}
	}
	return true
}

// appendCodestream writes SOC SIZ COD QCD SOT SOD <tile> EOC for one
// unsigned 16-bit component.
func appendCodestream(out []byte, w, h, levels, xcb, ycb int, tile []byte) ([]byte, error) {
	be16 := binary.BigEndian.AppendUint16
	be32 := binary.BigEndian.AppendUint32
	out = be16(out, MarkerSOC)

	out = be16(out, MarkerSIZ)
	out = be16(out, 38+3)
	out = be16(out, 0) // Rsiz
	for _, v := range []uint32{uint32(w), uint32(h), 0, 0, uint32(w), uint32(h), 0, 0} {
		out = be32(out, v)
	}
	out = be16(out, 1)
	out = append(out, bitDepth-1, 1, 1)

	out = be16(out, MarkerCOD)
	out = be16(out, 12)
	out = append(out, 0, 0) // Scod, LRCP
	out = be16(out, 1)      // layers
	out = append(out, 0, uint8(levels), uint8(xcb-2), uint8(ycb-2), 0, 1)

	exps := qcdEpsilons(levels)
	out = be16(out, MarkerQCD)
	out = be16(out, uint16(3+len(exps)))
	out = append(out, guardBits<<5)
	for _, e := range exps {
		out = append(out, e<<3)
	}

	// Psot spans the SOT segment (12), SOD (2) and the tile data.
	psot := uint64(14) + uint64(len(tile))
	if psot > 0xFFFFFFFF {
		return nil, fmt.Errorf("%w: tile-part of %d bytes exceeds Psot", ErrUnsupported, len(tile))
	}
	out = be16(out, MarkerSOT)
	out = be16(out, 10)
	out = be16(out, 0)
	out = be32(out, uint32(psot))
	out = append(out, 0, 1)
	out = be16(out, MarkerSOD)
	out = append(out, tile...)
	return be16(out, MarkerEOC), nil
}
