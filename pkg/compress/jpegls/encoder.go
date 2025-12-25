package jpegls

import (
	"errors"
	"image"
	"io"
)

// Options for encoding
type Options struct {
	Near int // Near-lossless parameter (0 = lossless)
	// T1, T2, T3 thresholds? (Optional)
	// Interleave mode? (ILV)
}

// Encoder encodes JPEG-LS data.
type Encoder struct {
	bw      *BitWriter
	params  FrameHeader
	scan    ScanHeader
	context *ContextModel
}

// Encode writes JPEG-LS data to w for the given image.
func Encode(w io.Writer, img image.Image, opts *Options) error {
	// 1. Validate Image
	b := img.Bounds()
	width := b.Max.X - b.Min.X
	height := b.Max.Y - b.Min.Y
	if width <= 0 || height <= 0 {
		return errors.New("invalid image dimensions")
	}

	// Determine precision and components
	var components int
	var precision int
	var maxVal int

	// Support Gray8 and Gray16 for now
	switch img.(type) {
	case *image.Gray:
		components = 1
		precision = 8
		maxVal = 255
	case *image.Gray16:
		components = 1
		precision = 16 // Or less? Assume 16.
		maxVal = 65535 // (1<<16) - 1
	default:
		return errors.New("unsupported image type (only Gray/Gray16)")
	}

	// Options
	near := 0
	if opts != nil {
		near = opts.Near
	}

	e := &Encoder{
		bw: NewBitWriter(w),
		params: FrameHeader{
			Width:      width,
			Height:     height,
			Components: components,
			Precision:  precision,
		},
		scan: ScanHeader{
			Components: components,
			Near:       near,
			// ILV 0 for single component
		},
	}

	return e.encode(img, maxVal)
}

func (e *Encoder) encode(img image.Image, maxVal int) error {
	// Write SOI
	if err := e.writeMarker(MarkerSOI); err != nil {
		return err
	}

	// Write SOF55
	if err := e.writeSOF(); err != nil {
		return err
	}

	// Write SOS
	if err := e.writeSOS(); err != nil {
		return err
	}

	// Init Context
	e.context = NewContextModel(maxVal, e.scan.Near, 64)

	// Encode Scan
	if err := e.encodeScan(img); err != nil {
		return err
	}

	// Flush Bits
	if err := e.bw.Flush(); err != nil {
		return err
	}

	// Write EOI
	if err := e.writeMarker(MarkerEOI); err != nil {
		return err
	}

	// Final Flush to ensure EOI and all bytes are written to underlying writer
	if err := e.bw.w.Flush(); err != nil {
		return err
	}

	return nil
}

func (e *Encoder) writeMarker(marker int) error {
	if err := e.bw.w.WriteByte(0xFF); err != nil {
		return err
	}
	return e.bw.w.WriteByte(byte(marker & 0xFF))
}

func (e *Encoder) writeSOF() error {
	// Length: 2 + 1 + 2 + 2 + 1 + Nf*3 = 8 + Nf*3
	length := 8 + e.params.Components*3

	if err := e.writeMarker(MarkerSOF55); err != nil {
		return err
	}
	if err := e.writeWord(length); err != nil {
		return err
	}

	if err := e.bw.w.WriteByte(byte(e.params.Precision)); err != nil {
		return err
	}
	if err := e.writeWord(e.params.Height); err != nil {
		return err
	}
	if err := e.writeWord(e.params.Width); err != nil {
		return err
	}
	if err := e.bw.w.WriteByte(byte(e.params.Components)); err != nil {
		return err
	}

	// Components (ID=1, H=1, V=1, Tq=0)
	for i := 0; i < e.params.Components; i++ {
		if err := e.bw.w.WriteByte(byte(i + 1)); err != nil {
			return err
		} // ID
		if err := e.bw.w.WriteByte(0x11); err != nil {
			return err
		} // H=1 V=1
		if err := e.bw.w.WriteByte(0x00); err != nil {
			return err
		} // Tq
	}
	return nil
}

func (e *Encoder) writeSOS() error {
	// Length: 2 + 1 + Ns*2 + 3 = 6 + Ns*2
	length := 6 + e.scan.Components*2

	if err := e.writeMarker(MarkerSOS); err != nil {
		return err
	}
	if err := e.writeWord(length); err != nil {
		return err
	}

	if err := e.bw.w.WriteByte(byte(e.scan.Components)); err != nil {
		return err
	}

	// Components (ID, Mapping table selector)
	for i := 0; i < e.scan.Components; i++ {
		if err := e.bw.w.WriteByte(byte(i + 1)); err != nil {
			return err
		}
		if err := e.bw.w.WriteByte(0x00); err != nil {
			return err
		} // Mapping
	}

	if err := e.bw.w.WriteByte(byte(e.scan.Near)); err != nil {
		return err
	}
	if err := e.bw.w.WriteByte(byte(e.scan.ILV)); err != nil {
		return err
	} // ILV
	if err := e.bw.w.WriteByte(0x00); err != nil {
		return err
	} // Al=0, Ah=0

	return nil
}

func (e *Encoder) writeWord(v int) error {
	if err := e.bw.w.WriteByte(byte(v >> 8)); err != nil {
		return err
	}
	return e.bw.w.WriteByte(byte(v & 0xFF))
}

func (e *Encoder) encodeScan(img image.Image) error {
	w := e.params.Width
	h := e.params.Height

	currLine := make([]int, w)
	prevLine := make([]int, w) // 0s
	maxVal := e.context.MaxVal
	// 	// near := e.scan.Near
	// Must be 0 for now (Lossless)

	// Get Pixel Helper
	getPixel := func(x, y int) int {
		if gray, ok := img.(*image.Gray); ok {
			return int(gray.GrayAt(x+img.Bounds().Min.X, y+img.Bounds().Min.Y).Y)
		}
		if gray16, ok := img.(*image.Gray16); ok {
			return int(gray16.Gray16At(x+img.Bounds().Min.X, y+img.Bounds().Min.Y).Y)
		}
		return 0
	}

	for y := 0; y < h; y++ {
		// Reset RunIndex at start of line (A.2)
		e.context.RunIndex = 0

		// Read Line into currLine
		for x := 0; x < w; x++ {
			currLine[x] = getPixel(x, y)
		}

		for x := 0; x < w; x++ {
			// Neighbors
			var Ra, Rb, Rc, Rd int

			if x > 0 {
				Ra = currLine[x-1]
			} else {
				if y > 0 {
					Ra = prevLine[0]
				} else {
					Ra = 0
				}
			}

			if y > 0 {
				Rb = prevLine[x]
				if x > 0 {
					Rc = prevLine[x-1]
				} else {
					Rc = prevLine[0]
				}
				if x < w-1 {
					Rd = prevLine[x+1]
				} else {
					Rd = Rb
				}
			} else {
				Rb = 0
				Rc = 0
				Rd = 0
			}

			Ix := currLine[x] // Current Pixel

			D1 := Rd - Rb
			D2 := Rb - Rc
			D3 := Rc - Ra

			// Run Mode Check (Forcefully Disabled)
			if false {
				// Run Mode
				// Ra, Rb are needed for sign in standard, but we use simplified Ra-based for now.
				// However, let's pass Rb too for better prediction.
				if err := e.encodeRun(img, currLine, &x, y, Ra, Rb); err != nil {
					return err
				}
				// x is updated (Next pixel index). But loop increments x.
				x--
				continue
			}

			// Regular Mode
			Q, sign := e.context.GetContextIndex(D1, D2, D3)

			Px := PredictMED(Ra, Rb, Rc)
			Px += sign * e.context.C[Q]
			Px = clip(Px, 0, maxVal)

			// Error
			Rx := Ix
			ErrVal := Rx - Px
			if sign == -1 {
				ErrVal = -ErrVal
			}

			// Modulo Reduction
			rangeVal := maxVal + 1
			if ErrVal < -rangeVal/2 {
				ErrVal += rangeVal
			}
			if ErrVal > rangeVal/2 {
				ErrVal -= rangeVal
			}

			// Map Error for Encoding (A.5.3)
			// Signed -> Non-Negative
			var MappedErrVal uint32

			// Mapping Logic:
			// if near == 0:
			// if ErrVal >= 0: Mapped = 2 * ErrVal
			// else: Mapped = -2 * ErrVal - 1

			if ErrVal >= 0 {
				MappedErrVal = uint32(2 * ErrVal)
			} else {
				MappedErrVal = uint32(-2*ErrVal - 1)
			}

			// Write Golomb
			k := e.context.ComputeK(Q)
			// Actually I need to import Fmt.
			// I'll skip import for now and assume fmt is available or I add it.
			// File has "errors", "image", "io". No fmt.
			// I will add import in separate step if needed. Or just blindly assume I can add import.
			if err := e.bw.WriteGolomb(k, MappedErrVal); err != nil {
				return err
			}

			// Update Stats
			e.context.UpdateStats(Q, ErrVal)
		}

		copy(prevLine, currLine)
	}
	return nil
}
