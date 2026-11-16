package jpegls

import (
	"errors"
	"fmt"
)

// Legacy decoder - FROZEN.
//
// Before this package implemented ITU-T T.87, its encoder wrote a private
// variant of JPEG-LS: T.81-style byte stuffing (0xFF 0x00), regular mode
// only (no run mode), an uncapped Golomb-Rice code, fixed A[Q] = 4
// initialization and a non-standard bias update. Files in that format exist
// in the wild (DICOM / DICOS objects written by earlier releases), so this
// file reproduces the decoder for it bit-for-bit. Nothing here is shared
// with the conformant coder; do not "fix" it.

var errLegacyMarker = errors.New("marker encountered in scan")

// legacyBitReader reads bits with 0xFF 0x00 byte stuffing.
type legacyBitReader struct {
	data  []byte
	pos   int
	bits  uint64
	nBits int
}

func (br *legacyBitReader) fill(n int) error {
	for br.nBits < n {
		if br.pos >= len(br.data) {
			return errEndOfData
		}
		b := br.data[br.pos]
		br.pos++
		if b == 0xFF {
			if br.pos >= len(br.data) {
				return errEndOfData
			}
			if br.data[br.pos] != 0x00 {
				br.pos--
				return errLegacyMarker
			}
			br.pos++
		}
		br.bits = br.bits<<8 | uint64(b)
		br.nBits += 8
	}
	return nil
}

func (br *legacyBitReader) readBits(n int) (int, error) {
	if n == 0 {
		return 0, nil
	}
	if err := br.fill(n); err != nil {
		return 0, err
	}
	br.nBits -= n
	return int(br.bits>>br.nBits) & (1<<n - 1), nil
}

// readGolomb reads an uncapped Golomb-Rice code with parameter k.
func (br *legacyBitReader) readGolomb(k int) (int, error) {
	q := 0
	for {
		bit, err := br.readBits(1)
		if err != nil {
			return 0, err
		}
		if bit == 1 {
			break
		}
		q++
		if q > 65536 {
			return 0, errors.New("golomb quotient overflow")
		}
	}
	r, err := br.readBits(k)
	if err != nil {
		return 0, err
	}
	return q<<k | r, nil
}

// legacyContext is the legacy context model (regular contexts only).
type legacyContext struct {
	t1, t2, t3 int
	a, b, c, n [365]int
}

func newLegacyContext(maxVal, near int) *legacyContext {
	factor := (min(maxVal, 4095) + 128) / 256
	cm := &legacyContext{}
	cm.t1 = clip(factor+2+3*near, near+1, maxVal)
	cm.t2 = clip(factor*4+3+5*near, cm.t1, maxVal)
	cm.t3 = clip(factor*17+4+7*near, cm.t2, maxVal)
	for i := range cm.a {
		cm.a[i] = 4
		cm.n[i] = 1
	}
	return cm
}

func (cm *legacyContext) quantize(d int) int {
	switch {
	case d <= -cm.t3:
		return -4
	case d <= -cm.t2:
		return -3
	case d <= -cm.t1:
		return -2
	case d < 0:
		return -1
	case d == 0:
		return 0
	case d < cm.t1:
		return 1
	case d < cm.t2:
		return 2
	case d < cm.t3:
		return 3
	}
	return 4
}

func (cm *legacyContext) contextIndex(d1, d2, d3 int) (int, int) {
	q1, q2, q3 := cm.quantize(d1), cm.quantize(d2), cm.quantize(d3)
	if q1 < 0 || (q1 == 0 && q2 < 0) || (q1 == 0 && q2 == 0 && q3 < 0) {
		return -(q1*81 + q2*9 + q3), -1
	}
	return q1*81 + q2*9 + q3, 1
}

func (cm *legacyContext) update(q, errVal int) {
	cm.b[q] += errVal
	cm.a[q] += abs(errVal)
	if cm.n[q] == DefaultReset {
		cm.a[q] >>= 1
		cm.b[q] >>= 1
		cm.n[q] >>= 1
	}
	cm.n[q]++

	// Two-step bias update, then clamp (not the T.87 single step).
	for range 2 {
		switch {
		case cm.b[q] <= -cm.n[q]:
			cm.b[q] += cm.n[q]
			cm.c[q]--
		case cm.b[q] > 0:
			cm.b[q] -= cm.n[q]
			cm.c[q]++
		}
	}
	cm.c[q] = clip(cm.c[q], minC, maxC)
}

// legacyDecodeScan decodes a legacy entropy-coded segment. It returns the
// samples and the number of bytes consumed (the offset of the trailing
// marker).
func legacyDecodeScan(data []byte, width, height, maxVal, near int) ([]uint16, int, error) {
	out := sampleBuffer(width, height)
	br := &legacyBitReader{data: data}
	cm := newLegacyContext(maxVal, near)
	rng := maxVal + 1
	cur := make([]int, width)
	prev := make([]int, width)

	for y := range height {
		for x := range width {
			var ra, rb, rc, rd int
			if y > 0 {
				rb = prev[x]
				rc = prev[max(x-1, 0)]
				rd = prev[min(x+1, width-1)]
				ra = prev[0]
			}
			if x > 0 {
				ra = cur[x-1]
			}

			q, sign := cm.contextIndex(rd-rb, rb-rc, rc-ra)
			px := clip(PredictMED(ra, rb, rc)+sign*cm.c[q], 0, maxVal)

			mapped, err := br.readGolomb(golombK(cm.n[q], cm.a[q]))
			if err != nil {
				return nil, 0, fmt.Errorf("sample (%d,%d): %w", x, y, err)
			}
			errVal := mapped >> 1
			if mapped&1 == 1 {
				errVal = -(mapped + 1) >> 1
			}
			// The legacy encoder reduced errors modulo RANGE; a larger value
			// means the stream is corrupt or not in this format.
			if abs(errVal) > rng/2 {
				return nil, 0, fmt.Errorf("sample (%d,%d): error value out of range", x, y)
			}
			cm.update(q, errVal)

			rx := px + sign*errVal
			if rx < 0 {
				rx += rng
			}
			if rx > maxVal {
				rx -= rng
			}
			cur[x] = clip(rx, 0, maxVal)
			out = append(out, uint16(cur[x]))
		}
		prev, cur = cur, prev
	}
	return out, br.pos, nil
}
