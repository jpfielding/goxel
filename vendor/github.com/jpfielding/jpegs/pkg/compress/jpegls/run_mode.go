package jpegls

import (
	"image"
	"image/color"
	"image/draw"
)

// decodeRun decodes a Run Mode segment.
func (d *Decoder) decodeRun(Ra, Rb int, img draw.Image, currLine []int, x *int, y int) error {
	// Ra is the value of the "flat" pixels.
	// Rb is needed for run interruption sign.

	// Access Context J State
	// Note: J is array [32]. RunIndex is tracked?
	// Spec uses 'RUN_index'.

	width := d.params.Width
	// Range = MaxVal + 1
	// maxVal := d.context.MaxVal

	for {
		// Read R (Run length encoded) or bit?
		// "Read bit. If 1, read Run Length."
		b, err := d.br.ReadBit()
		if err != nil {
			return err
		}

		if b == 1 {
			// Read r bits where r = J[RunIndex]
			j := d.context.J[d.context.RunIndex]
			// rBits, err := d.br.ReadBits(j) // REMOVED: b==1 implies full 2^J run without extra bits

			// Run Length = (1 << j)
			runLength := (1 << j)

			// Clamp to remaining pixels in line
			remainingPixels := width - *x
			if runLength > remainingPixels {
				runLength = remainingPixels
			}

			// Fill run
			for i := 0; i < runLength; i++ {
				currLine[*x] = Ra
				d.setPixel(img, *x, y, Ra)
				*x++
			}

			// Update RunIndex
			if d.context.RunIndex < 31 {
				d.context.RunIndex++
			}

			// Continue loop? Yes.
		} else {
			// b == 0. End of Run.
			// Read remaining run length r (J bits)
			j := d.context.J[d.context.RunIndex]
			var rBits uint32
			if j > 0 {
				var err error
				rBits, err = d.br.ReadBits(j)
				if err != nil {
					return err
				}
			}
			runLength := int(rBits)

			// Clamp run length to remaining pixels in line
			remainingPixels := width - *x
			if runLength > remainingPixels {
				runLength = remainingPixels
			}

			// Fill remaining run
			for i := 0; i < runLength; i++ {
				currLine[*x] = Ra
				d.setPixel(img, *x, y, Ra)
				*x++
			}

			// Check if we hit EOL?
			if *x >= width {
				// EOL reached exactly at end of run?
				return nil
			}

			// Read Interruption Sample.
			// ...

			// Decrement RunIndex Standard: "Decrements RunIndex".
			if d.context.RunIndex > 0 {
				d.context.RunIndex--
			}

			// Decode Interruption Sample
			// Placeholder: For now, just setting as black/error or try to decode?
			// Since we don't have full context logic here, let's try to assume small error?
			// The Encoder wrote a Golomb code for mapped error.
			// We MUST consume it.

			// Q=365 if a=b, else 366
			Q := 365
			if Ra != Rb {
				Q = 366
			}
			k := d.context.ComputeK(Q)

			mappedErr, err := d.br.ReadGolomb(k)
			if err != nil {
				return err
			}

			// Inverse Map Error
			var ErrVal int
			if mappedErr%2 == 0 {
				ErrVal = int(mappedErr / 2)
			} else {
				ErrVal = int(-(mappedErr + 1) / 2)
			}

			// Update Stats
			d.context.UpdateStats(Q, ErrVal)

			// Reconstruct Ix
			// ISO A.4.2: Px is Ra if a=b, else Rb.
			// Here Ra is 'a' and Rb is 'b'.
			Px := Ra
			sign := 1
			if Ra != Rb {
				Px = Rb
				if Ra > Rb {
					sign = -1
				}
			}

			Ix := Px + sign*ErrVal

			// Modulo reduction normalization
			maxVal := d.context.MaxVal
			rangeVal := maxVal + 1
			if Ix < -rangeVal/2 {
				Ix += rangeVal // Wait, normalization on ErrVal or Ix?
				// ErrVal was encoded after normalization.
				// Correct: It = (It + range/2) % range - range/2 ...
				// No, just apply ErrVal.
				// Encoder:
				// if ErrVal < -range/2 { ErrVal += range }
			}
			// Let's just clip to valid range [0, MaxVal] for now as simplified decoder.
			if Ix < 0 {
				Ix += rangeVal
			}
			if Ix > maxVal {
				Ix -= rangeVal
			}
			if Ix < 0 {
				Ix = 0
			}
			if Ix > maxVal {
				Ix = maxVal
			}

			currLine[*x] = Ix
			d.setPixel(img, *x, y, Ix)
			*x++
			return nil
		}
	}
}

// setPixel helper
func (d *Decoder) setPixel(img draw.Image, x, y, val int) {
	if grayImg, ok := img.(*image.Gray); ok {
		grayImg.SetGray(x, y, color.Gray{Y: uint8(val)})
	} else if gray16Img, ok := img.(*image.Gray16); ok {
		gray16Img.SetGray16(x, y, color.Gray16{Y: uint16(val)})
	}
}

// encodeRun encodes a Run Mode segment.
func (e *Encoder) encodeRun(img image.Image, currLine []int, x *int, y int, Ra, Rb int) error {
	width := e.params.Width

	// 1. Determine Run Length
	runLength := 0
	// While pixels match Ra
	for *x < width {
		val := currLine[*x]
		if val != Ra {
			break
		}
		runLength++
		*x++
	}

	// 2. Encode Run
	// While runLength >= (1 << J[RunIndex] )
	for {
		j := e.context.J[e.context.RunIndex]
		limit := (1 << j)

		if runLength >= limit {
			// Write 1 bit (Run continues)
			if err := e.bw.WriteBit(1); err != nil {
				return err
			}

			// Decrement run
			runLength -= limit
			if e.context.RunIndex < 31 {
				e.context.RunIndex++
			}
		} else {
			// Run ends (runLength < limit)
			// Write 0 bit (Run terminated)
			if err := e.bw.WriteBit(0); err != nil {
				return err
			}

			// Run Terminated by Value or EOL.
			// Encode runLength using J[RunIndex] bits.
			if err := e.bw.WriteBits(uint32(runLength), j); err != nil {
				return err
			}

			// Decrement Index
			if e.context.RunIndex > 0 {
				e.context.RunIndex--
			}

			// Check EOL - If run ended at line boundary, we are done
			if *x == width {
				return nil
			}

			// Encode Interruption Sample (Ix)
			Ix := currLine[*x]
			Px := Ra
			sign := 1
			if Ra != Rb {
				Px = Rb
				if Ra > Rb {
					sign = -1
				}
			}
			ErrVal := (Ix - Px)
			if sign == -1 {
				ErrVal = -ErrVal
			}

			// Modulo Reduction
			maxVal := e.context.MaxVal
			rangeVal := maxVal + 1
			if ErrVal < -rangeVal/2 {
				ErrVal += rangeVal
			}
			if ErrVal > rangeVal/2 {
				ErrVal -= rangeVal
			}

			// Context for Interruption
			Q := 365
			if Ra != Rb {
				Q = 366
			}

			// Map Error
			var MappedErrVal uint32
			if ErrVal >= 0 {
				MappedErrVal = uint32(2 * ErrVal)
			} else {
				MappedErrVal = uint32(-2*ErrVal - 1)
			}

			// Write Golomb
			k := e.context.ComputeK(Q)
			if err := e.bw.WriteGolomb(k, MappedErrVal); err != nil {
				return err
			}

			// Update Stats (AFTER matching decoder order)
			e.context.UpdateStats(Q, ErrVal)

			// Advance x to consume the interruption sample so outer loop proceeds
			*x++

			return nil
		}
	}
}
