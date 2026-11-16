package jpegls

// This file holds the T.87 run mode (A.7) and the sample arithmetic shared
// with regular mode, so the encoder and decoder cannot drift.
//
// Line buffers: cur and prev are width+2 element buffers of reconstructed
// samples with a one-slot offset - sample x lives at index x+1. Slot 0 is the
// left border and slot width+1 the right border, realizing the A.2.1 edge
// rules Ra(0,y) = Rb and Rd(width-1,y) = Rb.

// mapError maps a signed error to a non-negative code (A.5.2). correction is
// 0, or -1 when the k == 0 && NEAR == 0 bias flip applies.
func mapError(errVal, correction int) uint32 {
	v := correction ^ errVal
	if v >= 0 {
		return uint32(2 * v)
	}
	return uint32(-2*v - 1)
}

// unmapError inverts mapError.
func unmapError(mapped uint32, correction int) int {
	m := int(mapped)
	return correction ^ (m>>1 ^ -(m & 1))
}

// quantizeError is the near-lossless error quantization (A.4.4).
func quantizeError(e, near int) int {
	if near == 0 {
		return e
	}
	if e >= 0 {
		return (e + near) / (2*near + 1)
	}
	return -((near - e) / (2*near + 1))
}

// moduloRange reduces a (quantized) error into the coding range (A.4.5).
func moduloRange(e, rng int) int {
	if e < 0 {
		e += rng
	}
	if e >= (rng+1)/2 {
		e -= rng
	}
	return e
}

// validErrVal reports whether a decoded error is one a conformant encoder
// can produce (|Errval| <= RANGE/2). Rejecting anything larger also keeps the
// context accumulators bounded on corrupt input.
func validErrVal(e, rng int) bool {
	return abs(e) <= rng/2
}

// fixReconstructed folds a reconstructed value back into [0, maxVal] (A.4.6).
func fixReconstructed(v, near, rng, maxVal int) int {
	if v < -near {
		v += rng * (2*near + 1)
	} else if v > maxVal+near {
		v -= rng * (2*near + 1)
	}
	return clip(v, 0, maxVal)
}

// encodeRunLength emits the run-length code (A.7.1.2).
func encodeRunLength(bw *BitWriter, cm *ContextModel, runLength int, endOfLine bool) {
	for runLength >= 1<<jTable[cm.RunIndex] {
		bw.WriteBit(1)
		runLength -= 1 << jTable[cm.RunIndex]
		cm.incRunIndex()
	}
	if endOfLine {
		// A residual run reaching the end of line costs a single 1 bit and
		// does not advance RUNindex.
		if runLength != 0 {
			bw.WriteBit(1)
		}
		return
	}
	bw.WriteBit(0)
	bw.WriteBits(uint32(runLength), jTable[cm.RunIndex])
}

// decodeRunLength reads a run-length code for a line with remaining samples
// left (A.7.1.2).
func decodeRunLength(br *BitReader, cm *ContextModel, remaining int) (int, error) {
	index := 0
	for {
		bit, err := br.ReadBit()
		if err != nil {
			return 0, err
		}
		if bit == 0 {
			break
		}
		block := 1 << jTable[cm.RunIndex]
		count := min(block, remaining-index)
		index += count
		if count == block {
			cm.incRunIndex()
		}
		if index == remaining {
			return index, nil
		}
	}
	v, err := br.ReadBits(jTable[cm.RunIndex])
	if err != nil {
		return 0, err
	}
	index += int(v)
	if index > remaining {
		return 0, errBadCode
	}
	return index, nil
}

// interruptionParams selects RItype, the prediction and the sign for a
// run-interruption sample (A.7.2).
func interruptionParams(ra, rb, near int) (riType, px, sign int) {
	if abs(ra-rb) <= near {
		return 1, ra, 1
	}
	if rb > ra {
		return 0, rb, 1
	}
	return 0, rb, -1
}

// encodeRunInterruption codes the sample ending a run and returns its
// reconstructed value (A.7.2).
func encodeRunInterruption(bw *BitWriter, cm *ContextModel, ix, ra, rb int) int {
	near := cm.Near
	riType, px, s := interruptionParams(ra, rb, near)
	ctx := runCtxBase + riType

	errQ := moduloRange(quantizeError((ix-px)*s, near), cm.Range)
	k := cm.ComputeKRun(riType)
	nn, n := cm.Nn[riType], cm.N[ctx]
	mapBit := 0
	if (k == 0 && errQ > 0 && 2*nn < n) || (errQ < 0 && 2*nn >= n) || (errQ < 0 && k != 0) {
		mapBit = 1
	}
	eMapped := uint32(2*abs(errQ) - riType - mapBit)

	bw.WriteLimitedGolomb(k, eMapped, cm.Limit-jTable[cm.RunIndex]-1, cm.Qbpp)
	cm.UpdateRunStats(riType, eMapped, errQ < 0)
	return fixReconstructed(px+s*errQ*(2*near+1), near, cm.Range, cm.MaxVal)
}

// decodeRunInterruption decodes the sample ending a run (A.7.2).
func decodeRunInterruption(br *BitReader, cm *ContextModel, ra, rb int) (int, error) {
	near := cm.Near
	riType, px, s := interruptionParams(ra, rb, near)
	ctx := runCtxBase + riType

	k := cm.ComputeKRun(riType)
	eMapped, err := br.ReadLimitedGolomb(k, cm.Limit-jTable[cm.RunIndex]-1, cm.Qbpp)
	if err != nil {
		return 0, err
	}

	// temp = EMErrval + RItype = 2*|Errval| - map: its parity is map and
	// ceil(temp/2) is |Errval|. For a positive error the encoder's map is
	// (k == 0 && 2*Nn < N); the negative case is its complement.
	temp := int(eMapped) + riType
	errQ := (temp + temp&1) >> 1
	mapPositive := 0
	if k == 0 && 2*cm.Nn[riType] < cm.N[ctx] {
		mapPositive = 1
	}
	if temp&1 != mapPositive {
		errQ = -errQ
	}
	if !validErrVal(errQ, cm.Range) {
		return 0, errBadCode
	}
	cm.UpdateRunStats(riType, eMapped, errQ < 0)
	return fixReconstructed(px+s*errQ*(2*near+1), near, cm.Range, cm.MaxVal), nil
}

// encodeRun codes a run starting at sample start and returns the index of
// the next sample to code. src is the source row.
func encodeRun(bw *BitWriter, cm *ContextModel, src []uint16, cur, prev []int, start int) int {
	width := len(src)
	ra := cur[start]

	end := start
	for end < width && abs(int(src[end])-ra) <= cm.Near {
		cur[end+1] = ra
		end++
	}
	encodeRunLength(bw, cm, end-start, end == width)
	if end == width {
		return end
	}
	cur[end+1] = encodeRunInterruption(bw, cm, int(src[end]), ra, prev[end+1])
	cm.decRunIndex()
	return end + 1
}

// decodeRun decodes a run starting at sample start and returns the index of
// the next sample to decode.
func decodeRun(br *BitReader, cm *ContextModel, cur, prev []int, start, width int) (int, error) {
	ra := cur[start]
	runLength, err := decodeRunLength(br, cm, width-start)
	if err != nil {
		return 0, err
	}
	end := start + runLength
	for i := start; i < end; i++ {
		cur[i+1] = ra
	}
	if end == width {
		return end, nil
	}
	recon, err := decodeRunInterruption(br, cm, ra, prev[end+1])
	if err != nil {
		return 0, err
	}
	cur[end+1] = recon
	cm.decRunIndex()
	return end + 1, nil
}
