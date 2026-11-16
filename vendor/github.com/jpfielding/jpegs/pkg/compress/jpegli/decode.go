// Package jpegli implements a pure Go JPEG Lossless (ITU-T T.81 Annex H) decoder.
// This handles DICOM Transfer Syntax 1.2.840.10008.1.2.4.70 (JPEG Lossless, First-Order Prediction).
package jpegli

import (
	"errors"
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

// Decoder decodes JPEG Lossless images.
type Decoder struct {
	r io.Reader

	// Frame parameters
	precision  int // bits per sample (typically 8, 12, or 16)
	height     int
	width      int
	components int

	// Component info
	compInfo []componentInfo

	// Huffman tables (indexed by table class and ID)
	// Class 0 = DC tables (used for lossless)
	dcTables [4]*huffmanTable

	// Scan parameters
	predictor  int // 1-7
	pointTrans int // point transform (right shift)

	// Restart interval
	restartInterval int
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
	lookup [256]int16 // Fast lookup for 8-bit codes
}

// Decode reads a JPEG Lossless image from r and returns an image.Image.
func Decode(r io.Reader) (image.Image, error) {
	d := &Decoder{r: r}
	return d.decode()
}

func (d *Decoder) decode() (image.Image, error) {
	// Read SOI marker
	if err := d.expectMarker(MarkerSOI); err != nil {
		return nil, fmt.Errorf("expected SOI: %w", err)
	}

	// Read markers until SOS
	for {
		marker, err := d.readMarker()
		if err != nil {
			return nil, err
		}

		switch marker {
		case MarkerSOF3:
			if err := d.readSOF(); err != nil {
				return nil, err
			}
		case MarkerDHT:
			if err := d.readDHT(); err != nil {
				return nil, err
			}
		case MarkerSOS:
			return d.decodeScan()
		case MarkerDRI:
			if err := d.readDRI(); err != nil {
				return nil, err
			}
		case MarkerAPP0, MarkerCOM:
			// Skip APP and COM markers
			if err := d.skipMarkerData(); err != nil {
				return nil, err
			}
		case MarkerEOI:
			return nil, errors.New("unexpected EOI before scan data")
		default:
			// Skip unknown markers
			if marker >= 0xFFE0 && marker <= 0xFFEF { // APP markers
				if err := d.skipMarkerData(); err != nil {
					return nil, err
				}
			} else if marker >= 0xFFC0 && marker <= 0xFFCF { // SOF markers
				return nil, fmt.Errorf("unsupported SOF marker: 0x%04X", marker)
			} else {
				if err := d.skipMarkerData(); err != nil {
					return nil, err
				}
			}
		}
	}
}

func (d *Decoder) expectMarker(expected int) error {
	var buf [2]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return err
	}
	marker := int(buf[0])<<8 | int(buf[1])
	if marker != expected {
		return fmt.Errorf("expected marker 0x%04X, got 0x%04X", expected, marker)
	}
	return nil
}

func (d *Decoder) readMarker() (int, error) {
	var buf [2]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return 0, err
	}
	if buf[0] != 0xFF {
		return 0, fmt.Errorf("expected marker, got 0x%02X", buf[0])
	}
	// Skip fill bytes
	for buf[1] == 0xFF {
		if _, err := io.ReadFull(d.r, buf[1:]); err != nil {
			return 0, err
		}
	}
	return int(buf[0])<<8 | int(buf[1]), nil
}

// readSegment reads a marker segment's 2-byte length and returns its payload
// (length excluding the length field itself). Lengths below 2 are rejected.
func (d *Decoder) readSegment(name string) ([]byte, error) {
	var lenBuf [2]byte
	if _, err := io.ReadFull(d.r, lenBuf[:]); err != nil {
		return nil, fmt.Errorf("jpegli: reading %s length: %w", name, err)
	}
	length := int(lenBuf[0])<<8 | int(lenBuf[1])
	if length < 2 {
		return nil, fmt.Errorf("jpegli: %s segment length %d < 2", name, length)
	}
	data := make([]byte, length-2)
	if _, err := io.ReadFull(d.r, data); err != nil {
		return nil, fmt.Errorf("jpegli: reading %s payload: %w", name, err)
	}
	return data, nil
}

func (d *Decoder) skipMarkerData() error {
	var lenBuf [2]byte
	if _, err := io.ReadFull(d.r, lenBuf[:]); err != nil {
		return err
	}
	length := int(lenBuf[0])<<8 | int(lenBuf[1])
	if length < 2 {
		return fmt.Errorf("jpegli: marker segment length %d < 2", length)
	}
	if length > 2 {
		if _, err := io.CopyN(io.Discard, d.r, int64(length-2)); err != nil {
			return fmt.Errorf("jpegli: skipping marker segment: %w", err)
		}
	}
	return nil
}

func (d *Decoder) readSOF() error {
	data, err := d.readSegment("SOF")
	if err != nil {
		return err
	}
	if len(data) < 6 {
		return fmt.Errorf("jpegli: SOF segment too short (%d bytes)", len(data))
	}

	d.precision = int(data[0])
	d.height = int(data[1])<<8 | int(data[2])
	d.width = int(data[3])<<8 | int(data[4])
	d.components = int(data[5])
	if d.precision < 2 || d.precision > 16 {
		return fmt.Errorf("jpegli: invalid precision %d", d.precision)
	}
	if len(data) < 6+d.components*3 {
		return fmt.Errorf("jpegli: SOF segment length %d too short for %d components", len(data), d.components)
	}
	bytesPerSample := 1
	if d.precision > 8 {
		bytesPerSample = 2
	}
	if err := checkDecodedSize(d.width, d.height, d.components, bytesPerSample); err != nil {
		return err
	}

	d.compInfo = make([]componentInfo, d.components)
	for i := 0; i < d.components; i++ {
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
			return fmt.Errorf("jpegli: DHT invalid table class %d", tableClass)
		}
		if tableID > 3 {
			return fmt.Errorf("jpegli: DHT invalid table ID %d", tableID)
		}
		if len(data)-offset < 16 {
			return fmt.Errorf("jpegli: DHT truncated BITS (%d bytes left, need 16)", len(data)-offset)
		}

		var counts [17]int
		var totalCodes int
		for i := 1; i <= 16; i++ {
			counts[i] = int(data[offset+i-1])
			totalCodes += counts[i]
		}
		offset += 16

		if totalCodes > 256 {
			return fmt.Errorf("jpegli: DHT declares %d symbols (max 256)", totalCodes)
		}
		if len(data)-offset < totalCodes {
			return fmt.Errorf("jpegli: DHT declares %d symbols but only %d bytes remain", totalCodes, len(data)-offset)
		}
		vals := data[offset : offset+totalCodes]
		offset += totalCodes

		if tableClass != 0 {
			// Lossless JPEG only uses DC tables; AC definitions are skipped.
			continue
		}

		if totalCodes == 0 {
			return errors.New("jpegli: DHT table defines no codes")
		}
		if !bitsValid(&counts) {
			return errors.New("jpegli: DHT code lengths are oversubscribed")
		}
		for _, v := range vals {
			if v > 16 {
				return fmt.Errorf("jpegli: DHT symbol %d exceeds max difference category 16", v)
			}
		}

		ht := &huffmanTable{bits: counts}
		ht.values = make([]byte, totalCodes)
		copy(ht.values, vals)
		d.generateHuffmanCodes(ht)

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

func (d *Decoder) generateHuffmanCodes(ht *huffmanTable) {
	// Count total symbols
	var totalCodes int
	for i := 1; i <= 16; i++ {
		totalCodes += ht.bits[i]
	}

	ht.codes = make([]uint16, totalCodes)
	ht.sizes = make([]int, totalCodes)

	// Generate sizes
	k := 0
	for i := 1; i <= 16; i++ {
		for j := 0; j < ht.bits[i]; j++ {
			ht.sizes[k] = i
			k++
		}
	}

	// Generate codes
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

	// Build lookup table for fast 8-bit decoding
	for i := range ht.lookup {
		ht.lookup[i] = -1 // Invalid
	}
	for k := 0; k < totalCodes; k++ {
		size := ht.sizes[k]
		if size <= 8 {
			// Extend code to 8 bits
			code := ht.codes[k] << (8 - size)
			count := 1 << (8 - size)
			for i := 0; i < count; i++ {
				// Pack size in high byte, value in low byte
				ht.lookup[int(code)+i] = int16(size)<<8 | int16(ht.values[k])
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
		return fmt.Errorf("jpegli: DRI payload is %d bytes, want 2", len(data))
	}
	d.restartInterval = int(data[0])<<8 | int(data[1])
	return nil
}

func (d *Decoder) readSOS() error {
	if len(d.compInfo) == 0 {
		return errors.New("jpegli: SOS before SOF")
	}
	data, err := d.readSegment("SOS")
	if err != nil {
		return err
	}
	if len(data) < 1 {
		return errors.New("jpegli: SOS segment empty")
	}

	ns := int(data[0])
	if ns < 1 || ns > len(d.compInfo) {
		return fmt.Errorf("jpegli: SOS component count %d invalid (frame has %d)", ns, len(d.compInfo))
	}
	if len(data) != 1+2*ns+3 {
		return fmt.Errorf("jpegli: SOS payload is %d bytes, want %d for %d components", len(data), 1+2*ns+3, ns)
	}

	// Component selectors and table mappings
	offset := 1
	for i := 0; i < ns; i++ {
		selector := int(data[offset])
		td := int(data[offset+1] >> 4)
		offset += 2
		if td > 3 {
			return fmt.Errorf("jpegli: SOS DC table selector %d out of range", td)
		}

		// Find component by selector ID
		found := false
		for j := range d.compInfo {
			if d.compInfo[j].id == selector {
				d.compInfo[j].tableIndex = td
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("jpegli: SOS component selector %d matches no frame component", selector)
		}
	}

	// Ss = predictor selection in lossless
	d.predictor = int(data[offset])
	if d.predictor > 7 {
		return fmt.Errorf("jpegli: SOS invalid predictor %d", d.predictor)
	}
	offset += 2 // skip Se (always 0 for lossless)

	// Al = point transform
	d.pointTrans = int(data[offset]) & 0x0F
	if d.pointTrans >= d.precision {
		return fmt.Errorf("jpegli: SOS point transform %d >= precision %d", d.pointTrans, d.precision)
	}

	slog.Debug("jpegli: SOS parsed",
		slog.Int("predictor", d.predictor),
		slog.Int("pointTrans", d.pointTrans),
		slog.Int("numComponents", ns))

	return nil
}
