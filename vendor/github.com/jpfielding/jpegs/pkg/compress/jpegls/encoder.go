package jpegls

import (
	"encoding/binary"
	"fmt"
	"image"
	"io"
	"math/bits"
)

// Options for encoding.
type Options struct {
	// Near is the near-lossless error bound (0 = lossless). Every decoded
	// sample differs from the source by at most Near. Bounded by
	// min(255, MAXVAL/2).
	Near int
	// Precision is the sample precision in bits (2..16) written to the frame
	// header. Zero derives it from the image: 8 for *image.Gray; for
	// *image.Gray16 the bits needed by the largest sample, at least 9 so the
	// stream decodes back to *image.Gray16.
	Precision int
}

// Encode writes img to w as an ITU-T T.87 conformant JPEG-LS stream
// (single component, ILV=0). Supported image types are *image.Gray and
// *image.Gray16. A nil opts encodes lossless with a derived precision.
func Encode(w io.Writer, img image.Image, opts *Options) error {
	if opts == nil {
		opts = &Options{}
	}
	var samples []uint16
	var width, height, minPrecision int
	switch m := img.(type) {
	case *image.Gray:
		width, height = m.Rect.Dx(), m.Rect.Dy()
		if err := checkDimensions(width, height); err != nil {
			return err
		}
		samples = make([]uint16, width*height)
		for y := range height {
			row := m.Pix[y*m.Stride : y*m.Stride+width]
			for x, v := range row {
				samples[y*width+x] = uint16(v)
			}
		}
		minPrecision = 8
	case *image.Gray16:
		width, height = m.Rect.Dx(), m.Rect.Dy()
		if err := checkDimensions(width, height); err != nil {
			return err
		}
		samples = make([]uint16, width*height)
		for y := range height {
			row := m.Pix[y*m.Stride : y*m.Stride+2*width]
			for x := range width {
				samples[y*width+x] = binary.BigEndian.Uint16(row[2*x:])
			}
		}
		minPrecision = 9
	default:
		return fmt.Errorf("%w: image type %T (only *image.Gray and *image.Gray16)", ErrUnsupported, img)
	}

	out, err := encodeSamples(samples, width, height, opts.Near, opts.Precision, minPrecision)
	if err != nil {
		return err
	}
	_, err = w.Write(out)
	return err
}

func checkDimensions(width, height int) error {
	if width < 1 || width > 65535 || height < 1 || height > 65535 {
		return fmt.Errorf("%w: image dimensions %dx%d outside 1..65535", ErrInvalidData, width, height)
	}
	return nil
}

// encodeSamples encodes a row-major single-component sample buffer. A zero
// precision derives it from the largest sample, but no lower than
// minPrecision.
func encodeSamples(samples []uint16, width, height, near, precision, minPrecision int) ([]byte, error) {
	var maxSample uint16
	for _, v := range samples {
		maxSample = max(maxSample, v)
	}
	switch {
	case precision == 0:
		precision = max(bits.Len16(maxSample), minPrecision)
	case precision < 2 || precision > 16:
		return nil, fmt.Errorf("%w: precision %d outside 2..16", ErrInvalidData, precision)
	}
	maxVal := 1<<precision - 1
	if int(maxSample) > maxVal {
		return nil, fmt.Errorf("%w: sample %d does not fit precision %d", ErrInvalidData, maxSample, precision)
	}
	if near < 0 || near > min(255, maxVal/2) {
		return nil, fmt.Errorf("%w: near %d outside 0..min(255, MAXVAL/2)", ErrInvalidData, near)
	}

	buf := make([]byte, 0, 32+len(samples))
	buf = binary.BigEndian.AppendUint16(buf, MarkerSOI)
	buf = appendSOF(buf, precision, height, width)
	buf = appendSOS(buf, 1, near)
	bw := NewBitWriter(buf)
	encodeScan(bw, samples, width, height, NewContextModel(maxVal, near, DefaultReset))
	return binary.BigEndian.AppendUint16(bw.Bytes(), MarkerEOI), nil
}

// appendSOF appends a single-component SOF55 segment.
func appendSOF(buf []byte, precision, height, width int) []byte {
	buf = binary.BigEndian.AppendUint16(buf, MarkerSOF55)
	buf = binary.BigEndian.AppendUint16(buf, 11) // Lf = 8 + 3*Nf
	buf = append(buf, byte(precision))
	buf = binary.BigEndian.AppendUint16(buf, uint16(height))
	buf = binary.BigEndian.AppendUint16(buf, uint16(width))
	// Nf = 1; component ID 1, 1:1 sampling, Tq 0.
	return append(buf, 1, 1, 0x11, 0)
}

// appendSOS appends a single-component, non-interleaved SOS segment.
func appendSOS(buf []byte, componentID byte, near int) []byte {
	buf = binary.BigEndian.AppendUint16(buf, MarkerSOS)
	buf = binary.BigEndian.AppendUint16(buf, 8)
	// Ns = 1, Cs, mapping table 0, NEAR, ILV 0, point transform 0.
	return append(buf, 1, componentID, 0, byte(near), 0, 0)
}

// encodeScan writes the entropy-coded segment of one component
// (T.87 A.4 - A.7) and flushes the bit writer.
func encodeScan(bw *BitWriter, samples []uint16, width, height int, cm *ContextModel) {
	near, maxVal, rng := cm.Near, cm.MaxVal, cm.Range
	prev := make([]int, width+2)
	cur := make([]int, width+2)

	for y := range height {
		// Edge rules (A.2.1): Ra(0,y) = Rb, Rd(width-1,y) = Rb.
		cur[0] = prev[1]
		prev[width+1] = prev[width]
		src := samples[y*width : (y+1)*width]

		for x := 0; x < width; {
			ra, rb, rc, rd := cur[x], prev[x+1], prev[x], prev[x+2]
			d1, d2, d3 := rd-rb, rb-rc, rc-ra

			if abs(d1) <= near && abs(d2) <= near && abs(d3) <= near {
				x = encodeRun(bw, cm, src, cur, prev, x)
				continue
			}

			q, sign := cm.GetContextIndex(d1, d2, d3)
			k := cm.ComputeK(q)
			px := clip(PredictMED(ra, rb, rc)+sign*cm.C[q], 0, maxVal)
			errQ := moduloRange(quantizeError((int(src[x])-px)*sign, near), rng)

			correction := 0
			if k == 0 && near == 0 && 2*cm.B[q] <= -cm.N[q] {
				correction = -1
			}
			bw.WriteLimitedGolomb(k, mapError(errQ, correction), cm.Limit, cm.Qbpp)
			cm.UpdateStats(q, errQ)

			cur[x+1] = fixReconstructed(px+sign*errQ*(2*near+1), near, rng, maxVal)
			x++
		}
		prev, cur = cur, prev
	}
	bw.Flush()
}
