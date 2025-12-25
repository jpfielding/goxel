package jpeg2k

// EBCOT implements the Embedded Block Coding with Optimal Truncation
// as specified in ITU-T T.800 Annex D.
//
// This is a simplified implementation focusing on lossless encoding.

// CodeBlockEncoder encodes a code-block using EBCOT Tier-1
type CodeBlockEncoder struct {
	mqEnc    *MQEncoder
	contexts []MQState
	width    int
	height   int
	data     []int // Coefficient data
	sigma    []byte // Significance state
	sign     []byte // Sign bits
}

// NewCodeBlockEncoder creates a new code-block encoder
func NewCodeBlockEncoder(width, height int) *CodeBlockEncoder {
	return &CodeBlockEncoder{
		mqEnc:    NewMQEncoder(),
		contexts: SetupDefaultContexts(),
		width:    width,
		height:   height,
		sigma:    make([]byte, (width+2)*(height+2)), // With border
		sign:     make([]byte, width*height),
	}
}

// Encode encodes the coefficients and returns coded data
func (e *CodeBlockEncoder) Encode(data []int) ([]byte, int, int) {
	e.data = data

	// Find maximum bit-plane
	maxVal := 0
	for _, v := range data {
		if v < 0 {
			v = -v
		}
		if v > maxVal {
			maxVal = v
		}
	}

	if maxVal == 0 {
		// All zeros - empty code-block
		return nil, 0, 0
	}

	// Calculate number of bit-planes
	numBitPlanes := 0
	for maxVal > 0 {
		numBitPlanes++
		maxVal >>= 1
	}

	// Encode bit-planes from MSB to LSB
	passes := 0
	for bp := numBitPlanes - 1; bp >= 0; bp-- {
		// Significance propagation pass
		e.sigPropPass(bp)
		passes++

		// Magnitude refinement pass (not for first bit-plane)
		if bp < numBitPlanes-1 {
			e.magRefPass(bp)
			passes++
		}

		// Cleanup pass
		e.cleanupPass(bp)
		passes++
	}

	e.mqEnc.Flush()
	return e.mqEnc.Bytes(), passes, numBitPlanes
}

// sigPropPass performs the significance propagation pass
func (e *CodeBlockEncoder) sigPropPass(bitPlane int) {
	stride := e.width + 2
	mask := 1 << bitPlane

	for y := 0; y < e.height; y++ {
		for x := 0; x < e.width; x++ {
			idx := (y+1)*stride + (x + 1)
			if e.sigma[idx] != 0 {
				continue // Already significant
			}

			// Check if any neighbor is significant
			if !e.hasSignificantNeighbor(x, y) {
				continue
			}

			// Encode significance
			val := e.data[y*e.width+x]
			if val < 0 {
				val = -val
			}
			sig := 0
			if (val & mask) != 0 {
				sig = 1
			}

			ctx := e.getZCContext(x, y)
			e.mqEnc.Encode(sig, &e.contexts[ctx])

			if sig == 1 {
				e.sigma[idx] = 1
				// Encode sign
				signBit := 0
				if e.data[y*e.width+x] < 0 {
					signBit = 1
				}
				e.sign[y*e.width+x] = byte(signBit)
				signCtx := e.getSCContext(x, y)
				e.mqEnc.Encode(signBit, &e.contexts[signCtx])
			}
		}
	}
}

// magRefPass performs the magnitude refinement pass
func (e *CodeBlockEncoder) magRefPass(bitPlane int) {
	stride := e.width + 2
	mask := 1 << bitPlane

	for y := 0; y < e.height; y++ {
		for x := 0; x < e.width; x++ {
			idx := (y+1)*stride + (x + 1)
			if e.sigma[idx] == 0 {
				continue
			}

			val := e.data[y*e.width+x]
			if val < 0 {
				val = -val
			}
			bit := 0
			if (val & mask) != 0 {
				bit = 1
			}

			e.mqEnc.Encode(bit, &e.contexts[CtxMagRef])
		}
	}
}

// cleanupPass performs the cleanup pass
func (e *CodeBlockEncoder) cleanupPass(bitPlane int) {
	stride := e.width + 2
	mask := 1 << bitPlane

	for y := 0; y < e.height; y++ {
		for x := 0; x < e.width; x++ {
			idx := (y+1)*stride + (x + 1)
			if e.sigma[idx] != 0 {
				continue
			}

			if e.hasSignificantNeighbor(x, y) {
				continue // Already handled in sig prop
			}

			val := e.data[y*e.width+x]
			if val < 0 {
				val = -val
			}
			sig := 0
			if (val & mask) != 0 {
				sig = 1
			}

			e.mqEnc.Encode(sig, &e.contexts[CtxRunLength])

			if sig == 1 {
				e.sigma[idx] = 1
				signBit := 0
				if e.data[y*e.width+x] < 0 {
					signBit = 1
				}
				e.sign[y*e.width+x] = byte(signBit)
				e.mqEnc.Encode(signBit, &e.contexts[CtxUniform])
			}
		}
	}
}

func (e *CodeBlockEncoder) hasSignificantNeighbor(x, y int) bool {
	stride := e.width + 2
	idx := (y+1)*stride + (x + 1)

	// Check 8 neighbors
	return e.sigma[idx-stride-1] != 0 || e.sigma[idx-stride] != 0 || e.sigma[idx-stride+1] != 0 ||
		e.sigma[idx-1] != 0 || e.sigma[idx+1] != 0 ||
		e.sigma[idx+stride-1] != 0 || e.sigma[idx+stride] != 0 || e.sigma[idx+stride+1] != 0
}

func (e *CodeBlockEncoder) getZCContext(x, y int) int {
	// Simplified zero coding context based on neighbor significance
	stride := e.width + 2
	idx := (y+1)*stride + (x + 1)

	count := 0
	if e.sigma[idx-1] != 0 {
		count++
	}
	if e.sigma[idx+1] != 0 {
		count++
	}
	if e.sigma[idx-stride] != 0 {
		count++
	}
	if e.sigma[idx+stride] != 0 {
		count++
	}

	if count > 4 {
		count = 4
	}
	return count // Context 0-4
}

func (e *CodeBlockEncoder) getSCContext(x, y int) int {
	// Simplified sign coding context
	return CtxSignStart
}

// Reset resets the encoder for a new code-block
func (e *CodeBlockEncoder) Reset() {
	e.mqEnc.Reset()
	for i := range e.sigma {
		e.sigma[i] = 0
	}
	for i := range e.sign {
		e.sign[i] = 0
	}
}

// CodeBlockDecoder decodes a code-block using EBCOT Tier-1
type CodeBlockDecoder struct {
	mqDec    *MQDecoder
	contexts []MQState
	width    int
	height   int
	sigma    []byte
}

// NewCodeBlockDecoder creates a new code-block decoder
func NewCodeBlockDecoder(data []byte, width, height int) *CodeBlockDecoder {
	return &CodeBlockDecoder{
		mqDec:    NewMQDecoder(data),
		contexts: SetupDefaultContexts(),
		width:    width,
		height:   height,
		sigma:    make([]byte, (width+2)*(height+2)),
	}
}

// Decode decodes the code-block data
func (d *CodeBlockDecoder) Decode(numBitPlanes, numPasses int) []int {
	coeffs := make([]int, d.width*d.height)
	signs := make([]int, d.width*d.height)

	passIdx := 0
	for bp := numBitPlanes - 1; bp >= 0; bp-- {
		mask := 1 << bp

		// Significance propagation pass
		if passIdx < numPasses {
			d.decodeSigPropPass(coeffs, signs, mask)
			passIdx++
		}

		// Magnitude refinement pass
		if bp < numBitPlanes-1 && passIdx < numPasses {
			d.decodeMagRefPass(coeffs, mask)
			passIdx++
		}

		// Cleanup pass
		if passIdx < numPasses {
			d.decodeCleanupPass(coeffs, signs, mask)
			passIdx++
		}
	}

	// Apply signs
	for i, c := range coeffs {
		if signs[i] != 0 {
			coeffs[i] = -c
		}
	}

	return coeffs
}

func (d *CodeBlockDecoder) decodeSigPropPass(coeffs, signs []int, mask int) {
	stride := d.width + 2

	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			idx := (y+1)*stride + (x + 1)
			if d.sigma[idx] != 0 {
				continue
			}

			if !d.hasSignificantNeighbor(x, y) {
				continue
			}

			ctx := d.getZCContext(x, y)
			sig := d.mqDec.Decode(&d.contexts[ctx])

			if sig == 1 {
				d.sigma[idx] = 1
				coeffs[y*d.width+x] |= mask
				signBit := d.mqDec.Decode(&d.contexts[CtxSignStart])
				signs[y*d.width+x] = signBit
			}
		}
	}
}

func (d *CodeBlockDecoder) decodeMagRefPass(coeffs []int, mask int) {
	stride := d.width + 2

	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			idx := (y+1)*stride + (x + 1)
			if d.sigma[idx] == 0 {
				continue
			}

			bit := d.mqDec.Decode(&d.contexts[CtxMagRef])
			if bit == 1 {
				coeffs[y*d.width+x] |= mask
			}
		}
	}
}

func (d *CodeBlockDecoder) decodeCleanupPass(coeffs, signs []int, mask int) {
	stride := d.width + 2

	for y := 0; y < d.height; y++ {
		for x := 0; x < d.width; x++ {
			idx := (y+1)*stride + (x + 1)
			if d.sigma[idx] != 0 {
				continue
			}

			if d.hasSignificantNeighbor(x, y) {
				continue
			}

			sig := d.mqDec.Decode(&d.contexts[CtxRunLength])

			if sig == 1 {
				d.sigma[idx] = 1
				coeffs[y*d.width+x] |= mask
				signBit := d.mqDec.Decode(&d.contexts[CtxUniform])
				signs[y*d.width+x] = signBit
			}
		}
	}
}

func (d *CodeBlockDecoder) hasSignificantNeighbor(x, y int) bool {
	stride := d.width + 2
	idx := (y+1)*stride + (x + 1)

	return d.sigma[idx-stride-1] != 0 || d.sigma[idx-stride] != 0 || d.sigma[idx-stride+1] != 0 ||
		d.sigma[idx-1] != 0 || d.sigma[idx+1] != 0 ||
		d.sigma[idx+stride-1] != 0 || d.sigma[idx+stride] != 0 || d.sigma[idx+stride+1] != 0
}

func (d *CodeBlockDecoder) getZCContext(x, y int) int {
	stride := d.width + 2
	idx := (y+1)*stride + (x + 1)

	count := 0
	if d.sigma[idx-1] != 0 {
		count++
	}
	if d.sigma[idx+1] != 0 {
		count++
	}
	if d.sigma[idx-stride] != 0 {
		count++
	}
	if d.sigma[idx+stride] != 0 {
		count++
	}

	if count > 4 {
		count = 4
	}
	return count
}
