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

func (d *Decoder) skipMarkerData() error {
	var lenBuf [2]byte
	if _, err := io.ReadFull(d.r, lenBuf[:]); err != nil {
		return err
	}
	length := int(lenBuf[0])<<8 | int(lenBuf[1]) - 2
	if length > 0 {
		_, err := io.CopyN(io.Discard, d.r, int64(length))
		return err
	}
	return nil
}

func (d *Decoder) readSOF() error {
	var lenBuf [2]byte
	if _, err := io.ReadFull(d.r, lenBuf[:]); err != nil {
		return err
	}
	length := int(lenBuf[0])<<8 | int(lenBuf[1]) - 2

	data := make([]byte, length)
	if _, err := io.ReadFull(d.r, data); err != nil {
		return err
	}

	d.precision = int(data[0])
	d.height = int(data[1])<<8 | int(data[2])
	d.width = int(data[3])<<8 | int(data[4])
	d.components = int(data[5])

	d.compInfo = make([]componentInfo, d.components)
	for i := 0; i < d.components; i++ {
		offset := 6 + i*3
		d.compInfo[i] = componentInfo{
			id:         int(data[offset]),
			hSampling:  int(data[offset+1]) >> 4,
			vSampling:  int(data[offset+1]) & 0x0F,
			tableIndex: int(data[offset+2]),
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
	var lenBuf [2]byte
	if _, err := io.ReadFull(d.r, lenBuf[:]); err != nil {
		return err
	}
	length := int(lenBuf[0])<<8 | int(lenBuf[1]) - 2

	data := make([]byte, length)
	if _, err := io.ReadFull(d.r, data); err != nil {
		return err
	}

	offset := 0
	for offset < len(data) {
		tableInfo := data[offset]
		tableClass := int(tableInfo >> 4) // 0 = DC, 1 = AC
		tableID := int(tableInfo & 0x0F)
		offset++

		if tableClass != 0 {
			// Lossless JPEG only uses DC tables
			// Skip AC table definition
			var count int
			for i := 0; i < 16; i++ {
				count += int(data[offset+i])
			}
			offset += 16 + count
			continue
		}

		if tableID >= 4 {
			return fmt.Errorf("invalid Huffman table ID: %d", tableID)
		}

		ht := &huffmanTable{}

		// Read BITS (16 bytes)
		var totalCodes int
		for i := 0; i < 16; i++ {
			ht.bits[i+1] = int(data[offset+i])
			totalCodes += ht.bits[i+1]
		}
		offset += 16

		// Read HUFFVAL
		ht.values = make([]byte, totalCodes)
		copy(ht.values, data[offset:offset+totalCodes])
		offset += totalCodes

		// Generate codes and sizes
		d.generateHuffmanCodes(ht)

		slog.Debug("jpegli: DHT parsed",
			slog.Int("tableID", tableID),
			slog.Int("totalCodes", totalCodes),
			slog.Any("bits", ht.bits[1:17]),
			slog.Any("values", ht.values),
			slog.Any("codes", ht.codes),
			slog.Any("sizes", ht.sizes))

		d.dcTables[tableID] = ht
	}

	return nil
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
	var buf [4]byte
	if _, err := io.ReadFull(d.r, buf[:]); err != nil {
		return err
	}
	// Length is always 4 (2 bytes length + 2 bytes interval)
	d.restartInterval = int(buf[2])<<8 | int(buf[3])
	return nil
}

func (d *Decoder) readSOS() error {
	var lenBuf [2]byte
	if _, err := io.ReadFull(d.r, lenBuf[:]); err != nil {
		return err
	}
	length := int(lenBuf[0])<<8 | int(lenBuf[1]) - 2

	data := make([]byte, length)
	if _, err := io.ReadFull(d.r, data); err != nil {
		return err
	}

	numComponents := int(data[0])
	_ = numComponents // Used for interleaved scans

	// Component selectors and table mappings
	offset := 1
	for i := 0; i < numComponents; i++ {
		selector := int(data[offset])
		tableMapping := int(data[offset+1])
		offset += 2

		// Find component by selector ID
		for j := range d.compInfo {
			if d.compInfo[j].id == selector {
				d.compInfo[j].tableIndex = tableMapping >> 4
				slog.Debug("jpegli: Component mapping",
					slog.Int("id", selector),
					slog.Int("tableIndex", d.compInfo[j].tableIndex))
				break
			}
		}
	}

	// Spectral selection (Ss, Se) - in lossless, Ss = predictor
	d.predictor = int(data[offset])
	offset++
	// Se is always 0 for lossless
	offset++

	// Successive approximation (Ah, Al) - Al = point transform
	d.pointTrans = int(data[offset]) & 0x0F

	slog.Debug("jpegli: SOS parsed",
		slog.Int("predictor", d.predictor),
		slog.Int("pointTrans", d.pointTrans),
		slog.Int("numComponents", numComponents))

	return nil
}
