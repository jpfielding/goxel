// Package jpegli implements a pure Go JPEG Lossless (ITU-T T.81 Annex H,
// Process 14) codec for single-component images. This handles DICOM Transfer
// Syntaxes 1.2.840.10008.1.2.4.57 and .70 (JPEG Lossless, selection value 1).
package jpegli

import (
	"fmt"
	"image"
	"io"
	"log/slog"
)

// JPEG markers
const (
	MarkerSOI  = 0xFFD8 // Start of Image
	MarkerEOI  = 0xFFD9 // End of Image
	MarkerSOF3 = 0xFFC3 // Lossless (Huffman)
	MarkerDHT  = 0xFFC4 // Define Huffman Table
	MarkerSOS  = 0xFFDA // Start of Scan
	MarkerDQT  = 0xFFDB // Define Quantization Table (not used in lossless)
	MarkerDRI  = 0xFFDD // Define Restart Interval
	MarkerAPP0 = 0xFFE0 // JFIF APP0
	MarkerCOM  = 0xFFFE // Comment
)

const (
	markerRST0 = 0xFFD0
	markerRST7 = 0xFFD7
	markerDNL  = 0xFFDC
	markerTEM  = 0xFF01
)

// profile selects the entropy-coded scan format a decode accepts.
type profile int

const (
	// profileAuto decodes as T.81 and, when that fails on a stream the
	// pre-conformance encoder could have written, retries the frozen legacy
	// format (see legacy.go).
	profileAuto profile = iota
	// profileT81 accepts only ITU-T T.81 conformant streams.
	profileT81
	// profileLegacy accepts only the frozen pre-conformance format.
	profileLegacy
)

// Decoder holds the state of one JPEG Lossless decode.
type Decoder struct {
	data []byte
	pos  int // next unread byte of data

	// Frame parameters
	precision  int // P, bits per sample (2..16)
	height     int
	width      int
	components int
	compInfo   []componentInfo

	// Huffman tables indexed by DC table ID; lossless uses only DC tables.
	dcTables [4]*huffmanTable

	// Scan parameters
	predictor  int // Ss, 1..7
	pointTrans int // Al = Pt, point transform

	// Restart interval Ri in samples (one sample per MCU); 0 = none.
	restartInterval int

	// legacyMark records that the T.81 scan met SSSS=16 and the next 16
	// bits were 0x7FFF, as in every legacy SSSS=16 stream (see decodeAuto).
	legacyMark bool
}

type componentInfo struct {
	id         int
	hSampling  int
	vSampling  int
	tableIndex int
}

type huffmanTable struct {
	bits   [17]int    // BITS: number of codes of each length
	values []byte     // HUFFVAL: symbol values
	codes  []uint16   // Computed: Huffman codes
	sizes  []int      // Computed: code sizes
	lookup [256]int16 // size<<8 | value for codes of up to 8 bits, else -1

	// Canonical decode tables (T.81 F.2.2.3), indexed by code length.
	mincode [17]int32
	maxcode [17]int32 // -1 when there are no codes of that length
	valptr  [17]int
}

// Decode reads a JPEG Lossless image from r and returns an image.Image:
// *image.Gray for precision <= 8, otherwise *image.Gray16. Samples are
// returned at full precision (shifted left by the point transform).
//
// Supported: one component, precision 2..16, predictors 1..7, any point
// transform, any valid DC Huffman table, restart intervals that are a
// multiple of the row width. The stream must contain every sample and end
// with EOI; anything less is an error (ErrTruncated), never a partial image.
//
// 16-bit streams written by this package before it became T.81 conformant
// are still readable: a stream whose SSSS=16 codes carry the legacy appended
// bits is decoded with the frozen legacy decoder (see decodeAuto, legacy.go).
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("jpegli: reading stream: %w", err)
	}
	return decode(data, profileAuto)
}

func decode(data []byte, p profile) (image.Image, error) {
	d := &Decoder{data: data}
	if err := d.readHeaders(); err != nil {
		return nil, err
	}
	ht := d.dcTables[d.compInfo[0].tableIndex]

	var img image.Image
	var pix []byte
	wide := d.precision > 8
	rect := image.Rect(0, 0, d.width, d.height)
	if wide {
		m := image.NewGray16(rect)
		img, pix = m, m.Pix
	} else {
		m := image.NewGray(rect)
		img, pix = m, m.Pix
	}

	var err error
	switch p {
	case profileT81:
		_, err = d.decodeScan(ht, pix, wide)
	case profileLegacy:
		err = d.decodeLegacyScan(ht, pix)
	default:
		err = d.decodeAuto(ht, pix, wide)
	}
	if err != nil {
		return nil, err
	}
	return img, nil
}

// decodeAuto decodes a scan of unknown vintage. The legacy format differs
// from T.81 only after an SSSS=16 code, where it appends 16 bits that are
// always 0x7FFF. Until the first SSSS=16 both decoders read the same bits,
// so a legacy stream is one whose T.81 decode meets SSSS=16 followed by
// 0x7FFF (d.legacyMark) and whose headers match the legacy encoder
// (legacyEligible). Only then is the legacy decode tried, and it is
// preferred whenever it succeeds (every SSSS=16 followed by 0x7FFF, 1-bit
// padding, EOI), even if the T.81 decode succeeded too: a legacy stream
// misread as T.81 desyncs but can still end cleanly by chance, while a T.81
// stream that passes all the legacy checks is far rarer. TestLegacyDetection
// measures both rates. Otherwise the T.81 result or error stands.
func (d *Decoder) decodeAuto(ht *huffmanTable, pix []byte, wide bool) error {
	scanStart := d.pos
	_, err := d.decodeScan(ht, pix, wide)
	if !d.legacyMark || !d.legacyEligible(ht) {
		return err
	}
	d.pos = scanStart
	if lerr := d.decodeLegacyScan(ht, pix); lerr == nil {
		slog.Debug("jpegli: decoded legacy SSSS=16 stream")
		return nil
	}
	if err == nil {
		// Legacy failed after overwriting pix: decode the T.81 result again.
		d.pos = scanStart
		_, err = d.decodeScan(ht, pix, wide)
	}
	return err
}

// readHeaders parses SOI and the marker segments up to and including SOS,
// leaving d.pos at the first byte of entropy-coded data.
func (d *Decoder) readHeaders() error {
	m, err := d.readMarker()
	if err != nil {
		return err
	}
	if m != MarkerSOI {
		return fmt.Errorf("%w: expected SOI marker, got 0x%04X", ErrInvalidData, m)
	}
	for {
		m, err := d.readMarker()
		if err != nil {
			return err
		}
		switch {
		case m == MarkerSOF3:
			err = d.readSOF()
		case m == MarkerDHT:
			err = d.readDHT()
		case m == MarkerDRI:
			err = d.readDRI()
		case m == MarkerSOS:
			return d.readSOS()
		case m == MarkerEOI:
			return fmt.Errorf("%w: EOI before scan data", ErrInvalidData)
		case m >= 0xFFC0 && m <= 0xFFCF:
			return fmt.Errorf("%w: SOF marker 0x%04X (only SOF3 lossless Huffman)", ErrUnsupported, m)
		case m >= markerRST0 && m <= MarkerSOI, m == markerTEM:
			return fmt.Errorf("%w: unexpected marker 0x%04X before scan", ErrInvalidData, m)
		default: // APPn, COM, DQT and other segments carry nothing lossless needs
			_, err = d.readSegment(fmt.Sprintf("marker 0x%04X", m))
		}
		if err != nil {
			return err
		}
	}
}

func (d *Decoder) truncated(context string) error {
	return &TruncatedError{Offset: min(d.pos, len(d.data)), Context: context}
}

// readMarker reads a marker, skipping 0xFF fill bytes (T.81 B.1.1.2).
func (d *Decoder) readMarker() (int, error) {
	if d.pos >= len(d.data) {
		return 0, d.truncated("marker")
	}
	if b := d.data[d.pos]; b != 0xFF {
		return 0, fmt.Errorf("%w: expected marker at offset %d, got 0x%02X", ErrInvalidData, d.pos, b)
	}
	for d.pos < len(d.data) && d.data[d.pos] == 0xFF {
		d.pos++
	}
	if d.pos >= len(d.data) {
		return 0, d.truncated("marker")
	}
	m := 0xFF00 | int(d.data[d.pos])
	d.pos++
	return m, nil
}

// readSegment reads a marker segment's 2-byte length and returns its payload
// (length excluding the length field itself). Lengths below 2 are rejected.
func (d *Decoder) readSegment(name string) ([]byte, error) {
	if len(d.data)-d.pos < 2 {
		return nil, d.truncated(name + " length")
	}
	length := int(d.data[d.pos])<<8 | int(d.data[d.pos+1])
	if length < 2 {
		return nil, fmt.Errorf("%w: %s segment length %d < 2", ErrInvalidData, name, length)
	}
	if len(d.data)-d.pos < length {
		return nil, d.truncated(name + " payload")
	}
	payload := d.data[d.pos+2 : d.pos+length]
	d.pos += length
	return payload, nil
}

func (d *Decoder) readSOF() error {
	data, err := d.readSegment("SOF3")
	if err != nil {
		return err
	}
	if d.compInfo != nil {
		return fmt.Errorf("%w: second SOF3 segment", ErrInvalidData)
	}
	if len(data) < 6 {
		return fmt.Errorf("%w: SOF3 segment too short (%d bytes)", ErrInvalidData, len(data))
	}

	d.precision = int(data[0])
	d.height = int(data[1])<<8 | int(data[2])
	d.width = int(data[3])<<8 | int(data[4])
	d.components = int(data[5])
	// T.81 B.2.2: X > 0; Y = 0 defers the height to a DNL marker, which this
	// codec does not support.
	if d.width == 0 || d.height == 0 {
		return fmt.Errorf("%w: dimensions %dx%d (width and height must be > 0)", ErrInvalidParameter, d.width, d.height)
	}
	if d.precision < 2 || d.precision > 16 {
		return fmt.Errorf("%w: precision %d (allowed 2..16)", ErrInvalidParameter, d.precision)
	}
	if d.components != 1 {
		return fmt.Errorf("%w: %d components (only single-component frames)", ErrUnsupported, d.components)
	}
	if len(data) < 6+d.components*3 {
		return fmt.Errorf("%w: SOF3 segment length %d too short for %d components", ErrInvalidData, len(data), d.components)
	}
	bytesPerSample := 1
	if d.precision > 8 {
		bytesPerSample = 2
	}
	if err := checkDecodedSize(d.width, d.height, d.components, bytesPerSample); err != nil {
		return err
	}

	d.compInfo = make([]componentInfo, d.components)
	for i := range d.compInfo {
		offset := 6 + i*3
		d.compInfo[i] = componentInfo{
			id:        int(data[offset]),
			hSampling: int(data[offset+1]) >> 4,
			vSampling: int(data[offset+1]) & 0x0F,
			// data[offset+2] is Tq (quantization table), unused in lossless;
			// the Huffman table selector comes from the SOS header.
		}
	}

	slog.Debug("jpegli: SOF3 parsed",
		slog.Int("precision", d.precision),
		slog.Int("width", d.width),
		slog.Int("height", d.height),
		slog.Int("components", d.components))
	return nil
}

func (d *Decoder) readDHT() error {
	data, err := d.readSegment("DHT")
	if err != nil {
		return err
	}

	offset := 0
	for offset < len(data) {
		tableInfo := data[offset]
		tableClass := int(tableInfo >> 4) // 0 = DC, 1 = AC
		tableID := int(tableInfo & 0x0F)
		offset++

		if tableClass > 1 {
			return fmt.Errorf("%w: DHT table class %d", ErrInvalidData, tableClass)
		}
		if tableID > 3 {
			return fmt.Errorf("%w: DHT table ID %d", ErrInvalidData, tableID)
		}
		if len(data)-offset < 16 {
			return fmt.Errorf("%w: DHT BITS truncated (%d bytes left, need 16)", ErrInvalidData, len(data)-offset)
		}

		var counts [17]int
		var totalCodes int
		for i := 1; i <= 16; i++ {
			counts[i] = int(data[offset+i-1])
			totalCodes += counts[i]
		}
		offset += 16

		if totalCodes > 256 {
			return fmt.Errorf("%w: DHT declares %d symbols (max 256)", ErrInvalidData, totalCodes)
		}
		if len(data)-offset < totalCodes {
			return fmt.Errorf("%w: DHT declares %d symbols but only %d bytes remain", ErrInvalidData, totalCodes, len(data)-offset)
		}
		vals := data[offset : offset+totalCodes]
		offset += totalCodes

		if tableClass != 0 {
			// Lossless JPEG only uses DC tables; AC definitions are skipped.
			continue
		}

		if totalCodes == 0 {
			return fmt.Errorf("%w: DHT table defines no codes", ErrInvalidData)
		}
		if !bitsValid(&counts) {
			return fmt.Errorf("%w: DHT code lengths are oversubscribed", ErrInvalidData)
		}
		for _, v := range vals {
			if v > 16 {
				return fmt.Errorf("%w: DHT symbol %d exceeds max difference category 16", ErrInvalidData, v)
			}
		}

		ht := &huffmanTable{bits: counts}
		ht.values = make([]byte, totalCodes)
		copy(ht.values, vals)
		generateHuffmanCodes(ht)

		slog.Debug("jpegli: DHT parsed",
			slog.Int("tableID", tableID),
			slog.Int("totalCodes", totalCodes))

		d.dcTables[tableID] = ht
	}
	return nil
}

// bitsValid reports whether the code length counts (1..16) form a
// non-oversubscribed prefix code (Kraft inequality, T.81 Annex C).
func bitsValid(counts *[17]int) bool {
	code := 0
	for l := 1; l <= 16; l++ {
		code += counts[l]
		if code > 1<<l {
			return false
		}
		code <<= 1
	}
	return true
}

// generateHuffmanCodes derives the canonical codes (T.81 C.2), the 8-bit
// fast lookup and the F.2.2.3 decode tables from ht.bits and ht.values.
func generateHuffmanCodes(ht *huffmanTable) {
	var totalCodes int
	for i := 1; i <= 16; i++ {
		totalCodes += ht.bits[i]
	}

	ht.codes = make([]uint16, totalCodes)
	ht.sizes = make([]int, totalCodes)

	k := 0
	for i := 1; i <= 16; i++ {
		for j := 0; j < ht.bits[i]; j++ {
			ht.sizes[k] = i
			k++
		}
	}

	code := uint16(0)
	si := ht.sizes[0]
	for k := 0; k < totalCodes; k++ {
		for ht.sizes[k] > si {
			code <<= 1
			si++
		}
		ht.codes[k] = code
		code++
	}

	k = 0
	for l := 1; l <= 16; l++ {
		if ht.bits[l] == 0 {
			ht.maxcode[l] = -1
			continue
		}
		ht.valptr[l] = k
		ht.mincode[l] = int32(ht.codes[k])
		k += ht.bits[l]
		ht.maxcode[l] = int32(ht.codes[k-1])
	}

	for i := range ht.lookup {
		ht.lookup[i] = -1
	}
	for k := 0; k < totalCodes; k++ {
		size := ht.sizes[k]
		if size <= 8 {
			code := int(ht.codes[k]) << (8 - size)
			for i := 0; i < 1<<(8-size); i++ {
				ht.lookup[code+i] = int16(size)<<8 | int16(ht.values[k])
			}
		}
	}
}

func (d *Decoder) readDRI() error {
	data, err := d.readSegment("DRI")
	if err != nil {
		return err
	}
	if len(data) != 2 {
		return fmt.Errorf("%w: DRI payload is %d bytes, want 2", ErrInvalidData, len(data))
	}
	d.restartInterval = int(data[0])<<8 | int(data[1])
	return nil
}

func (d *Decoder) readSOS() error {
	if len(d.compInfo) == 0 {
		return fmt.Errorf("%w: SOS before SOF3", ErrInvalidData)
	}
	data, err := d.readSegment("SOS")
	if err != nil {
		return err
	}
	if len(data) < 1 {
		return fmt.Errorf("%w: SOS segment empty", ErrInvalidData)
	}

	ns := int(data[0])
	if ns < 1 || ns > len(d.compInfo) {
		return fmt.Errorf("%w: SOS component count %d (frame has %d)", ErrInvalidData, ns, len(d.compInfo))
	}
	if len(data) != 1+2*ns+3 {
		return fmt.Errorf("%w: SOS payload is %d bytes, want %d for %d components", ErrInvalidData, len(data), 1+2*ns+3, ns)
	}

	offset := 1
	for i := 0; i < ns; i++ {
		selector := int(data[offset])
		td := int(data[offset+1] >> 4)
		offset += 2
		if td > 3 {
			return fmt.Errorf("%w: SOS DC table selector %d (allowed 0..3)", ErrInvalidParameter, td)
		}
		found := false
		for j := range d.compInfo {
			if d.compInfo[j].id == selector {
				d.compInfo[j].tableIndex = td
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("%w: SOS component selector %d matches no frame component", ErrInvalidData, selector)
		}
	}
	if d.dcTables[d.compInfo[0].tableIndex] == nil {
		return fmt.Errorf("%w: SOS selects undefined DC Huffman table %d", ErrInvalidData, d.compInfo[0].tableIndex)
	}

	// T.81 Table B.3, lossless: Ss = predictor 1..7, Se = 0, Ah = 0,
	// Al = point transform < P.
	ss, se, ah, al := int(data[offset]), int(data[offset+1]), int(data[offset+2]>>4), int(data[offset+2]&0x0F)
	switch {
	case ss < 1 || ss > 7:
		return fmt.Errorf("%w: predictor (Ss) %d (allowed 1..7)", ErrInvalidParameter, ss)
	case se != 0:
		return fmt.Errorf("%w: Se %d (must be 0 for lossless)", ErrInvalidParameter, se)
	case ah != 0:
		return fmt.Errorf("%w: Ah %d (must be 0 for lossless)", ErrInvalidParameter, ah)
	case al >= d.precision:
		return fmt.Errorf("%w: point transform %d >= precision %d", ErrInvalidParameter, al, d.precision)
	}
	d.predictor, d.pointTrans = ss, al

	// T.81 H.1.1: one sample per MCU, and Ri must be a multiple of the
	// samples per line, so restarts fall on row boundaries.
	if d.restartInterval%d.width != 0 {
		return fmt.Errorf("%w: restart interval %d is not a multiple of width %d", ErrInvalidData, d.restartInterval, d.width)
	}

	slog.Debug("jpegli: SOS parsed",
		slog.Int("predictor", d.predictor),
		slog.Int("pointTrans", d.pointTrans),
		slog.Int("numComponents", ns))
	return nil
}

// readTrailer parses the markers after the scan, starting at d.pos (the
// marker that ended the entropy-coded data). APPn and COM segments may
// precede EOI; anything after EOI is ignored (DICOM pads fragments).
func (d *Decoder) readTrailer() error {
	for {
		if d.pos >= len(d.data) {
			return d.truncated("EOI")
		}
		m, err := d.readMarker()
		if err != nil {
			return err
		}
		switch {
		case m == MarkerEOI:
			return nil
		case m >= markerRST0 && m <= markerRST7:
			return fmt.Errorf("%w: restart marker 0x%04X after the last sample", ErrInvalidData, m)
		case m == MarkerSOS, m == markerDNL:
			return fmt.Errorf("%w: marker 0x%04X after the scan (single scan only)", ErrUnsupported, m)
		case m >= 0xFFE0 && m <= 0xFFEF, m == MarkerCOM:
			if _, err := d.readSegment(fmt.Sprintf("marker 0x%04X", m)); err != nil {
				return err
			}
		default:
			return fmt.Errorf("%w: marker 0x%04X between scan and EOI", ErrInvalidData, m)
		}
	}
}
