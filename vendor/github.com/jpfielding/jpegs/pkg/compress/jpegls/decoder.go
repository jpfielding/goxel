package jpegls

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"io"
	"slices"
)

// profile selects the entropy-coded scan format a decode accepts.
type profile int

const (
	// profileAuto decodes as T.87 and, when that fails, retries the frozen
	// legacy format (see legacy.go).
	profileAuto profile = iota
	// profileT87 accepts only ITU-T T.87 conformant streams.
	profileT87
	// profileLegacy accepts only the frozen pre-conformance format.
	profileLegacy
)

// Decode reads a JPEG-LS stream from r.
//
// Conformant ITU-T T.87 streams are supported for single-component frames
// and 3-component planar frames (ILV=0, one scan per component), precision
// 2..16, lossless and near-lossless, with LSE ID=1 presets. A grayscale frame
// decodes to *image.Gray (precision <= 8) or *image.Gray16; a 3-component
// frame to *image.RGBA or *image.RGBA64. Sample values are stored as coded,
// without scaling to the full range of the image type.
//
// Streams written by this package before it became conformant are still
// readable: when a single-component stream fails to decode as T.87, it is
// retried with the legacy format.
func Decode(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("jpegls: reading stream: %w", err)
	}
	return decode(data, profileAuto)
}

func decode(data []byte, p profile) (image.Image, error) {
	d := &Decoder{data: data}
	frame, presets, scan, err := d.readHeaders()
	if err != nil {
		return nil, err
	}
	var samples []uint16
	switch p {
	case profileT87:
		samples, err = d.decodeT87(frame, presets, scan)
	case profileLegacy:
		samples, err = d.decodeLegacy(frame, scan)
	default:
		samples, err = d.decodeAuto(frame, presets, scan)
	}
	if err != nil {
		return nil, err
	}
	return toImage(frame, samples), nil
}

// decodeAuto decodes a stream of unknown vintage. The headers of the legacy
// format are identical to T.87, so the scan data decides: decode as T.87
// and, if that fails, as legacy. Both decodes are strict - the scan must
// yield every sample and be followed directly by EOI - which a stream of the
// other format practically never satisfies for frames of realistic size.
// (For frames of a few dozen 8-bit samples one bit string can be valid in
// both formats; T.87 then wins. See TestLegacyDetection.) If both fail, the
// T.87 error is reported.
func (d *Decoder) decodeAuto(frame FrameHeader, presets Presets, scan ScanHeader) ([]uint16, error) {
	scanStart := d.pos
	samples, err := d.decodeT87(frame, presets, scan)
	// The legacy encoder only wrote single-component frames of 8 or more
	// bits, and never an LSE segment.
	if err == nil || frame.Components != 1 || presets.Seen || frame.Precision < 8 {
		return samples, err
	}
	d.pos = scanStart
	if legacy, lerr := d.decodeLegacy(frame, scan); lerr == nil {
		return legacy, nil
	}
	return nil, err
}

// toImage converts component-planar samples to an image.Image.
func toImage(frame FrameHeader, samples []uint16) image.Image {
	rect := image.Rect(0, 0, frame.Width, frame.Height)
	plane := frame.Width * frame.Height
	switch {
	case frame.Components == 1 && frame.Precision <= 8:
		img := image.NewGray(rect)
		for i, v := range samples {
			img.Pix[i] = byte(v)
		}
		return img
	case frame.Components == 1:
		img := image.NewGray16(rect)
		for i, v := range samples {
			binary.BigEndian.PutUint16(img.Pix[2*i:], v)
		}
		return img
	case frame.Precision <= 8:
		img := image.NewRGBA(rect)
		for i := range plane {
			img.Pix[4*i] = byte(samples[i])
			img.Pix[4*i+1] = byte(samples[plane+i])
			img.Pix[4*i+2] = byte(samples[2*plane+i])
			img.Pix[4*i+3] = 0xFF
		}
		return img
	}
	img := image.NewRGBA64(rect)
	for i := range plane {
		binary.BigEndian.PutUint16(img.Pix[8*i:], samples[i])
		binary.BigEndian.PutUint16(img.Pix[8*i+2:], samples[plane+i])
		binary.BigEndian.PutUint16(img.Pix[8*i+4:], samples[2*plane+i])
		binary.BigEndian.PutUint16(img.Pix[8*i+6:], 0xFFFF)
	}
	return img
}

// Decoder parses the marker segments of a JPEG-LS stream held in memory.
type Decoder struct {
	data []byte
	pos  int
}

func (d *Decoder) readByte() (byte, error) {
	if d.pos >= len(d.data) {
		return 0, fmt.Errorf("%w: unexpected end of data", ErrInvalidData)
	}
	b := d.data[d.pos]
	d.pos++
	return b, nil
}

// take returns the next n bytes of a segment payload.
func (d *Decoder) take(n int) ([]byte, error) {
	if n < 0 || n > len(d.data)-d.pos {
		return nil, fmt.Errorf("%w: unexpected end of data in marker segment", ErrInvalidData)
	}
	b := d.data[d.pos : d.pos+n]
	d.pos += n
	return b, nil
}

// readMarker reads a marker and the payload of its segment. Fill bytes
// (0xFF) before the marker code are skipped (T.81 B.1.1.2).
func (d *Decoder) readMarker() (int, []byte, error) {
	b, err := d.readByte()
	if err != nil {
		return 0, nil, err
	}
	if b != 0xFF {
		return 0, nil, fmt.Errorf("%w: expected marker prefix 0xFF, got 0x%02X", ErrInvalidData, b)
	}
	for b == 0xFF {
		if b, err = d.readByte(); err != nil {
			return 0, nil, err
		}
	}
	marker := 0xFF00 | int(b)
	lb, err := d.take(2)
	if err != nil {
		return 0, nil, err
	}
	// The length field counts its own two bytes.
	payload, err := d.take(max(int(binary.BigEndian.Uint16(lb)), 2) - 2)
	if err != nil {
		return 0, nil, err
	}
	return marker, payload, nil
}

// expectEOI consumes the EOI marker, tolerating 0xFF fill bytes before it.
func (d *Decoder) expectEOI() error {
	p := d.pos
	if p >= len(d.data) || d.data[p] != 0xFF {
		return fmt.Errorf("%w: expected EOI after scan at offset %d", ErrInvalidData, d.pos)
	}
	for p < len(d.data) && d.data[p] == 0xFF {
		p++
	}
	if p >= len(d.data) || d.data[p] != MarkerEOI&0xFF {
		return fmt.Errorf("%w: expected EOI after scan at offset %d", ErrInvalidData, d.pos)
	}
	d.pos = p + 1
	return nil
}

// parseSOF parses a SOF55 payload and rejects a frame whose decoded size
// exceeds MaxDecodedBytes. Both the T.87 and the legacy scan decoders run
// only after this check.
func parseSOF(p []byte) (FrameHeader, error) {
	var f FrameHeader
	if len(p) < 6 {
		return f, fmt.Errorf("%w: SOF55 segment too short (%d payload bytes)", ErrInvalidData, len(p))
	}
	f.Precision = int(p[0])
	f.Height = int(binary.BigEndian.Uint16(p[1:]))
	f.Width = int(binary.BigEndian.Uint16(p[3:]))
	f.Components = int(p[5])
	if f.Precision < 2 || f.Precision > 16 {
		return f, fmt.Errorf("%w: precision %d outside 2..16", ErrInvalidData, f.Precision)
	}
	if f.Width == 0 || f.Height == 0 {
		return f, fmt.Errorf("%w: SOF55 dimensions must be non-zero", ErrInvalidData)
	}
	if f.Components != 1 && f.Components != 3 {
		return f, fmt.Errorf("%w: component count %d (only 1 or 3)", ErrUnsupported, f.Components)
	}
	bytesPerSample := 1
	if f.Precision > 8 {
		bytesPerSample = 2
	}
	if err := checkDecodedSize(f.Width, f.Height, f.Components, bytesPerSample); err != nil {
		return f, err
	}
	if len(p) != 6+3*f.Components {
		return f, fmt.Errorf("%w: SOF55 payload is %d bytes, expected %d", ErrInvalidData, len(p), 6+3*f.Components)
	}
	for c := range f.Components {
		id, sampling, tq := p[6+3*c], p[7+3*c], p[8+3*c]
		if bytes.IndexByte(f.ComponentIDs, id) >= 0 {
			return f, fmt.Errorf("%w: duplicate SOF55 component ID %d", ErrInvalidData, id)
		}
		f.ComponentIDs = append(f.ComponentIDs, id)
		if sampling != 0x11 {
			return f, fmt.Errorf("%w: component sampling 0x%02X (must be 1:1)", ErrUnsupported, sampling)
		}
		if tq != 0 {
			return f, fmt.Errorf("%w: component mapping table %d", ErrUnsupported, tq)
		}
	}
	return f, nil
}

// parseLSE parses an LSE segment; only ID=1 (coding parameters) is supported.
func parseLSE(p []byte, presets *Presets) error {
	if len(p) < 1 {
		return fmt.Errorf("%w: empty LSE segment", ErrInvalidData)
	}
	switch id := p[0]; id {
	case 1:
		if len(p) < 11 {
			return fmt.Errorf("%w: LSE ID=1 segment too short", ErrInvalidData)
		}
		*presets = Presets{
			MaxVal: int(binary.BigEndian.Uint16(p[1:])),
			T1:     int(binary.BigEndian.Uint16(p[3:])),
			T2:     int(binary.BigEndian.Uint16(p[5:])),
			T3:     int(binary.BigEndian.Uint16(p[7:])),
			Reset:  int(binary.BigEndian.Uint16(p[9:])),
			Seen:   true,
		}
		// RESET has no "use the default" encoding here: a value below 3
		// would suppress statistics halving, so it is rejected outright.
		if presets.Reset < minReset {
			return fmt.Errorf("%w: LSE RESET=%d (must be >= %d)", ErrInvalidData, presets.Reset, minReset)
		}
		return nil
	case 2, 3, 4:
		return fmt.Errorf("%w: LSE ID=%d (mapping tables / extended dimensions)", ErrUnsupported, id)
	default:
		return fmt.Errorf("%w: unknown LSE ID=%d", ErrInvalidData, id)
	}
}

// parseSOS parses a scan header. nf is the frame component count, or 0 when
// no SOF55 has been read yet.
func parseSOS(p []byte, nf int) (ScanHeader, error) {
	var s ScanHeader
	if len(p) < 6 {
		return s, fmt.Errorf("%w: SOS segment too short", ErrInvalidData)
	}
	ns := int(p[0])
	if ns == 0 || ns > 4 || (nf != 0 && ns > nf) {
		return s, fmt.Errorf("%w: SOS component count Ns=%d", ErrInvalidData, ns)
	}
	if ns != 1 {
		return s, fmt.Errorf("%w: multi-component scan (Ns=%d)", ErrUnsupported, ns)
	}
	if len(p) != 6 {
		return s, fmt.Errorf("%w: SOS length does not match component count", ErrInvalidData)
	}
	mapping, near, ilv, pointTransform := p[2], p[3], p[4], p[5]
	if mapping != 0 {
		return s, fmt.Errorf("%w: SOS mapping table selector %d", ErrUnsupported, mapping)
	}
	if ilv != 0 {
		return s, fmt.Errorf("%w: interleaved scan (ILV=%d)", ErrUnsupported, ilv)
	}
	if pointTransform != 0 {
		return s, fmt.Errorf("%w: SOS point transform 0x%02X must be 0", ErrInvalidData, pointTransform)
	}
	return ScanHeader{ComponentID: p[1], Near: int(near)}, nil
}

// readHeaders parses SOI and every segment up to and including the first SOS.
func (d *Decoder) readHeaders() (FrameHeader, Presets, ScanHeader, error) {
	var (
		frame   FrameHeader
		presets Presets
		scan    ScanHeader
	)
	fail := func(err error) (FrameHeader, Presets, ScanHeader, error) {
		return frame, presets, scan, err
	}
	if len(d.data) < 2 || binary.BigEndian.Uint16(d.data) != MarkerSOI {
		return fail(fmt.Errorf("%w: missing SOI marker", ErrInvalidData))
	}
	d.pos = 2

	for {
		marker, payload, err := d.readMarker()
		if err != nil {
			return fail(err)
		}
		switch marker {
		case MarkerSOF55:
			if frame, err = parseSOF(payload); err != nil {
				return fail(err)
			}
		case MarkerLSE:
			if err = parseLSE(payload, &presets); err != nil {
				return fail(err)
			}
		case MarkerSOS:
			if scan, err = parseSOS(payload, frame.Components); err != nil {
				return fail(err)
			}
			if frame.Components == 0 {
				return fail(fmt.Errorf("%w: missing SOF55 marker", ErrInvalidData))
			}
			return frame, presets, scan, nil
		case MarkerEOI:
			return fail(fmt.Errorf("%w: unexpected EOI before SOS", ErrInvalidData))
		default:
			// Other segments (including DRI: restart markers inside a scan
			// are rejected by the entropy decoder) are skipped.
		}
	}
}

// validateParams checks MAXVAL / NEAR / threshold ordering (T.87 C.2.4.1.1)
// and returns the effective MAXVAL and RESET.
func validateParams(frame FrameHeader, presets Presets, near int) (maxVal, reset int, err error) {
	sofMaxVal := 1<<frame.Precision - 1
	maxVal = sofMaxVal
	if presets.MaxVal != 0 {
		maxVal = presets.MaxVal
	}
	if maxVal > sofMaxVal {
		return 0, 0, fmt.Errorf("%w: LSE MAXVAL=%d exceeds %d for precision %d", ErrInvalidData, maxVal, sofMaxVal, frame.Precision)
	}
	if near > min(255, maxVal/2) {
		return 0, 0, fmt.Errorf("%w: NEAR=%d outside 0..min(255, MAXVAL/2)", ErrInvalidData, near)
	}
	t1, t2, t3 := DefaultThresholds(maxVal, near)
	if presets.T1 != 0 {
		t1 = presets.T1
	}
	if presets.T2 != 0 {
		t2 = presets.T2
	}
	if presets.T3 != 0 {
		t3 = presets.T3
	}
	if near >= t1 || t1 > t2 || t2 > t3 || t3 > maxVal {
		return 0, 0, fmt.Errorf("%w: thresholds violate NEAR<T1<=T2<=T3<=MAXVAL (NEAR=%d T1=%d T2=%d T3=%d MAXVAL=%d)",
			ErrInvalidData, near, t1, t2, t3, maxVal)
	}
	reset = DefaultReset
	if presets.Seen {
		reset = presets.Reset
	}
	return maxVal, reset, nil
}

// decodeT87 decodes every scan of a conformant stream (one scan per
// component, ILV=0) and the trailing EOI. The first SOS is already parsed.
func (d *Decoder) decodeT87(frame FrameHeader, presets Presets, scan ScanHeader) ([]uint16, error) {
	near := scan.Near
	maxVal, reset, err := validateParams(frame, presets, near)
	if err != nil {
		return nil, err
	}
	// One plane per component, in frame order. Each grows as its scan is
	// decoded, so a truncated or crafted stream cannot make the decoder
	// allocate the full frame up front.
	planes := make([][]uint16, frame.Components)

	for c := range frame.Components {
		if c > 0 {
			marker, payload, err := d.readMarker()
			if err != nil {
				return nil, err
			}
			if marker != MarkerSOS {
				return nil, fmt.Errorf("%w: expected SOS for component %d, got marker 0x%04X", ErrInvalidData, c, marker)
			}
			if scan, err = parseSOS(payload, frame.Components); err != nil {
				return nil, err
			}
			if scan.Near != near {
				return nil, fmt.Errorf("%w: NEAR differs across scans (%d then %d)", ErrInvalidData, near, scan.Near)
			}
		}
		index := bytes.IndexByte(frame.ComponentIDs, scan.ComponentID)
		if index < 0 {
			return nil, fmt.Errorf("%w: unknown SOS component selector %d", ErrInvalidData, scan.ComponentID)
		}
		if planes[index] != nil {
			return nil, fmt.Errorf("%w: duplicate SOS component selector %d", ErrInvalidData, scan.ComponentID)
		}

		cm := NewContextModelPresets(maxVal, near, reset, presets.T1, presets.T2, presets.T3)
		br := NewBitReader(d.data[d.pos:])
		if planes[index], err = decodeScan(br, frame.Width, frame.Height, cm); err != nil {
			return nil, scanError(err, d.pos+br.Pos())
		}
		d.pos += br.MarkerPos()
	}
	if err := d.expectEOI(); err != nil {
		return nil, err
	}
	return slices.Concat(planes...), nil
}

// decodeLegacy decodes the single scan of a legacy stream and its EOI.
func (d *Decoder) decodeLegacy(frame FrameHeader, scan ScanHeader) ([]uint16, error) {
	if frame.Components != 1 {
		return nil, fmt.Errorf("%w: legacy streams are single-component", ErrUnsupported)
	}
	samples, consumed, err := legacyDecodeScan(d.data[d.pos:], frame.Width, frame.Height, 1<<frame.Precision-1, scan.Near)
	if err != nil {
		return nil, fmt.Errorf("%w: legacy scan: %v", ErrInvalidData, err)
	}
	d.pos += consumed
	if err := d.expectEOI(); err != nil {
		return nil, err
	}
	return samples, nil
}

// scanError classifies a bit reader error met before a scan was complete.
func scanError(err error, offset int) error {
	var m markerError
	if errors.As(err, &m) && m >= 0xFFD0 && m <= 0xFFD7 {
		return fmt.Errorf("%w: restart markers inside a scan", ErrUnsupported)
	}
	return fmt.Errorf("%w: entropy-coded scan truncated or corrupt at offset %d: %v", ErrInvalidData, offset, err)
}

// decodeScan decodes the entropy-coded segment of one component
// (T.87 A.4 - A.7). The result grows row by row; see sampleBuffer.
func decodeScan(br *BitReader, width, height int, cm *ContextModel) ([]uint16, error) {
	out := sampleBuffer(width, height)
	near, maxVal, rng := cm.Near, cm.MaxVal, cm.Range
	prev := make([]int, width+2)
	cur := make([]int, width+2)

	for range height {
		cur[0] = prev[1]
		prev[width+1] = prev[width]

		for x := 0; x < width; {
			ra, rb, rc, rd := cur[x], prev[x+1], prev[x], prev[x+2]
			d1, d2, d3 := rd-rb, rb-rc, rc-ra

			if abs(d1) <= near && abs(d2) <= near && abs(d3) <= near {
				next, err := decodeRun(br, cm, cur, prev, x, width)
				if err != nil {
					return nil, err
				}
				x = next
				continue
			}

			q, sign := cm.GetContextIndex(d1, d2, d3)
			k := cm.ComputeK(q)
			px := clip(PredictMED(ra, rb, rc)+sign*cm.C[q], 0, maxVal)

			correction := 0
			if k == 0 && near == 0 && 2*cm.B[q] <= -cm.N[q] {
				correction = -1
			}
			mapped, err := br.ReadLimitedGolomb(k, cm.Limit, cm.Qbpp)
			if err != nil {
				return nil, err
			}
			errQ := unmapError(mapped, correction)
			if !validErrVal(errQ, rng) {
				return nil, errBadCode
			}
			cm.UpdateStats(q, errQ)

			cur[x+1] = fixReconstructed(px+sign*errQ*(2*near+1), near, rng, maxVal)
			x++
		}
		for _, v := range cur[1 : width+1] {
			out = append(out, uint16(v))
		}
		prev, cur = cur, prev
	}
	return out, nil
}

// sampleBuffer returns an empty buffer for the samples of one component.
// Its initial capacity is bounded, and it is grown by append as rows are
// decoded, so memory use follows the data actually present in the stream
// rather than the dimensions the header claims.
func sampleBuffer(width, height int) []uint16 {
	return make([]uint16, 0, min(width*height, 1<<16))
}
