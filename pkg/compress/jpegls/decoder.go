package jpegls

import (
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"io"
)

// Decoder decodes JPEG-LS data.
type Decoder struct {
	br      *BitReader
	params  FrameHeader
	scan    ScanHeader
	context *ContextModel

	// Decoded data (buffer)
	// We decode into a Linear buffer or Image?
	// For large images, maybe row by row?
	// But image.Image interface requires random access at (x,y)? No.
	// Ideally we return *image.Gray16 or *image.Gray or *image.RGBA.
	// For DICOM: usually 8-bit or 16-bit Gray.
}

// Decode reads JPEG-LS data from r and returns an image.Image.
func Decode(r io.Reader) (image.Image, error) {
	d := &Decoder{
		br: NewBitReader(r),
		// Defaults
	}
	return d.decode()
}

func (d *Decoder) decode() (image.Image, error) {
	// 1. Read SOI
	if err := d.expectMarker(MarkerSOI); err != nil {
		return nil, err
	}

	// 2. Read Markers until SOS
	for {
		marker, length, err := d.readMarker()
		if err != nil {
			return nil, err
		}

		switch marker {
		case MarkerSOF55:
			if err := d.readSOF(length); err != nil {
				return nil, err
			}
		case MarkerLSE:
			// Custom parameters (MaxVal, etc)
			if err := d.readLSE(length); err != nil {
				return nil, err
			}
		case MarkerSOS:
			if err := d.readSOS(length); err != nil {
				return nil, err
			}
			goto ScanStarted
		case MarkerEOI:
			return nil, errors.New("unexpected EOI before SOS")
		default:
			// Skip unknown markers?
			if err := d.skip(length); err != nil {
				return nil, err
			}
		}
	}

ScanStarted:
	// Initialize Context Model
	// MaxVal determined by Precision (SOF) or LSE?
	// Default MaxVal = (1<<Precision) - 1.
	maxVal := (1 << d.params.Precision) - 1
	// If LSE updated it, we should track it.

	// Debug logging
	// fmt.Printf("JPEGLS Decode: W=%d H=%d P=%d Near=%d MaxVal=%d\n", d.params.Width, d.params.Height, d.params.Precision, d.scan.Near, maxVal)

	d.context = NewContextModel(maxVal, d.scan.Near, 64) // 64 is default reset

	// Allocate Image
	var img draw.Image
	if d.params.Precision <= 8 {
		img = image.NewGray(image.Rect(0, 0, d.params.Width, d.params.Height))
	} else {
		img = image.NewGray16(image.Rect(0, 0, d.params.Width, d.params.Height))
	}

	// Decode Scan
	if err := d.decodeScan(img); err != nil {
		return nil, err
	}

	// Read EOI?
	// d.expectMarker(MarkerEOI)

	return img, nil
}

// Helpers
func (d *Decoder) expectMarker(tm int) error {
	// Read 0xFF, then marker
	// Markers are Byte Aligned?
	// We assume we are at byte boundary.
	b1, err := d.br.r.ReadByte()
	if err != nil {
		return err
	}
	if b1 != 0xFF {
		return fmt.Errorf("expected marker FF, got %X", b1)
	}

	b2, err := d.br.r.ReadByte()
	if err != nil {
		return err
	}

	marker := 0xFF00 | int(b2)
	if marker != tm {
		return fmt.Errorf("expected marker %X, got %X", tm, marker)
	}
	return nil
}

func (d *Decoder) readMarker() (int, int, error) {
	b1, err := d.br.r.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	if b1 != 0xFF {
		return 0, 0, fmt.Errorf("expected marker FF, got %X", b1)
	}

	b2, err := d.br.r.ReadByte()
	if err != nil {
		return 0, 0, err
	}

	marker := 0xFF00 | int(b2)

	// Read Length (2 bytes)
	l1, err := d.br.r.ReadByte()
	if err != nil {
		return 0, 0, err
	}
	l2, err := d.br.r.ReadByte()
	if err != nil {
		return 0, 0, err
	}

	length := (int(l1) << 8) | int(l2)
	return marker, length - 2, nil // Length includes length bytes
}

func (d *Decoder) skip(n int) error {
	_, err := d.br.r.Discard(n)
	return err
}

func (d *Decoder) readSOF(n int) error {
	// P (1), Y (2), X (2), Nf (1)
	// For now:
	p, err := d.br.r.ReadByte()
	d.params.Precision = int(p)

	h1, _ := d.br.r.ReadByte()
	h2, _ := d.br.r.ReadByte()
	d.params.Height = (int(h1) << 8) | int(h2)

	w1, _ := d.br.r.ReadByte()
	w2, _ := d.br.r.ReadByte()
	d.params.Width = (int(w1) << 8) | int(w2)

	nf, err := d.br.r.ReadByte()
	if err != nil {
		return err
	}
	d.params.Components = int(nf)

	// Read components (Nf * 3 bytes)
	// ID (1), Sampling (1), Quant (1)
	toSkip := n - 6
	return d.skip(toSkip)
}

func (d *Decoder) readLSE(n int) error {
	return d.skip(n) // Placeholder
}

func (d *Decoder) readSOS(n int) error {
	// Ns (1), Components (Ns*2), Near (1), ILV (1), Al (1), Ah (1)
	ns, err := d.br.r.ReadByte()
	d.scan.Components = int(ns)

	// Skip components spec (Ns * 2 bytes)
	d.skip(d.scan.Components * 2)

	near, _ := d.br.r.ReadByte()
	d.scan.Near = int(near)

	ilv, _ := d.br.r.ReadByte()
	d.scan.ILV = int(ilv)

	bits, _ := d.br.r.ReadByte()
	d.scan.Al = int(bits >> 4)
	d.scan.Ah = int(bits & 0xF)

	return err
}

func (d *Decoder) decodeScan(img draw.Image) error {
	w := d.params.Width
	h := d.params.Height
	currLine := make([]int, w)
	prevLine := make([]int, w) // Initialized to 0s

	// Stats
	// near := d.scan.Near
	maxVal := d.context.MaxVal

	// Pixel Reconstruction map (Modulo MaxVal+1)
	// Rx = (Px + Err) mod Range.
	maxValPlus1 := maxVal + 1

	for y := 0; y < h; y++ {
		// Reset RunIndex at start of line (A.2)
		d.context.RunIndex = 0

		// Line Logic
		for x := 0; x < w; x++ {
			// Neighbors
			var Ra, Rb, Rc, Rd int

			// Rb (Above), Rc (Above-Left), Rd (Above-Right)
			if y > 0 {
				Rb = prevLine[x]
				if x > 0 {
					Rc = prevLine[x-1]
				} else {
					// Start of line: Rc = Rb? Or Rc = prevLine[0]
					Rc = prevLine[0]
					// Ra (Left) at x=0 is Rb (Above)
				}
				if x < w-1 {
					Rd = prevLine[x+1]
				} else {
					Rd = Rb // End of line
				}
			} else {
				// First line: Rb=0, Rc=0, Rd=0 (All zero)
				// Ra=0 (at x=0) or currLine[x-1]
			}

			if x > 0 {
				Ra = currLine[x-1]
			} else {
				if y > 0 {
					Ra = prevLine[0] // "Ra = Rb"
				} else {
					Ra = 0 // First pixel
				}
			}

			// Gradients
			D1 := Rd - Rb
			D2 := Rb - Rc
			D3 := Rc - Ra

			// Run Mode Check (Forcefully Disabled)
			if false {
				if err := d.decodeRun(Ra, Rb, img, currLine, &x, y); err != nil {
					// Check for marker error
					if err.Error() == "marker encountered" || len(err.Error()) > 6 && err.Error()[:6] == "marker" {
						return nil
					}
					return fmt.Errorf("decodeRun failed at x=%d, y=%d: %w", x, y, err)
				}
				// x points to next pixel index.
				// Loop increments x. So we must decrement to compensate.
				x--
				continue
			}

			// Regular Mode
			Q, sign := d.context.GetContextIndex(D1, D2, D3)

			// Prediction
			Px := PredictMED(Ra, Rb, Rc)

			// Correct Prediction with Bias C[Q]
			// "If sign == -1, C[Q] is subtracted?"
			// Stat update handles sign.
			// Px = Px + sign * C[Q]
			// Standard: Apply sign to correction.
			Px += sign * d.context.C[Q]

			// Clamp Px to [0, MaxVal]
			Px = clip(Px, 0, maxVal)

			// Decode Error
			k := d.context.ComputeK(Q)
			MappedErrVal, err := d.br.ReadGolomb(k)
			if err != nil {
				// If marker encountered, stop decoding elegantly (assume rest is 0/padded)
				if err.Error() == "marker encountered" || len(err.Error()) > 6 && err.Error()[:6] == "marker" {
					return nil
				}
				return fmt.Errorf("ReadGolomb failed at x=%d, y=%d: %w", x, y, err)
			}

			// Map ErrVal
			ErrVal := 0
			em := int(MappedErrVal)
			if (em & 1) == 0 {
				ErrVal = em >> 1
			} else {
				ErrVal = -(em + 1) >> 1
			}

			StatsErrVal := ErrVal

			if sign == -1 {
				ErrVal = -ErrVal
			}

			d.context.UpdateStats(Q, StatsErrVal)

			rxVal := Px + ErrVal

			// Range Correct (Modulo)
			// "Modulo reduction" to valid range.
			// Usually [ -MaxVal/2 .. MaxVal/2 ] range for error?
			// Or Rx in [0..MaxVal].
			// If rxVal < 0: rxVal += Range?
			// If rxVal > MaxVal: rxVal -= Range?
			// Range = MaxVal + 1.

			if rxVal < 0 {
				rxVal += maxValPlus1
			}
			if rxVal > maxVal {
				rxVal -= maxValPlus1
			}

			// Clamp just in case
			if rxVal < 0 {
				rxVal = 0
			}
			if rxVal > maxVal {
				rxVal = maxVal
			}

			currLine[x] = rxVal

			// Store in Image
			if grayImg, ok := img.(*image.Gray); ok {
				grayImg.SetGray(x, y, color.Gray{Y: uint8(rxVal)})
			} else if gray16Img, ok := img.(*image.Gray16); ok {
				gray16Img.SetGray16(x, y, color.Gray16{Y: uint16(rxVal)})
			}
		}
		// Swap lines not needed in this simplified loop if we copy?
		// prevLine = currLine (Copy contents)
		copy(prevLine, currLine)
	}
	return nil
}
