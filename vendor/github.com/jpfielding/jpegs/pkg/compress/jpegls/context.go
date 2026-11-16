package jpegls

// Context layout: 365 regular contexts plus two run-interruption contexts
// (T.87 A.7.2) at indices 365 (RItype 0) and 366 (RItype 1).
const (
	numContexts = 367
	runCtxBase  = 365

	minC = -128
	maxC = 127

	basicT1 = 3
	basicT2 = 7
	basicT3 = 21
)

// jTable is the run-length order table J (T.87 A.2.1).
var jTable = [32]int{
	0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3,
	4, 4, 5, 5, 6, 6, 7, 7, 8, 9, 10, 11, 12, 13, 14, 15,
}

// ceilLog2 returns the smallest k with 2^k >= x, for x >= 1.
func ceilLog2(x int) int {
	k := 0
	for 1<<k < x {
		k++
	}
	return k
}

// clampThreshold is the T.87 A.2.1 threshold clamp: a value outside
// [lo, hi] collapses to lo (including values above hi).
func clampThreshold(i, lo, hi int) int {
	if i > hi || i < lo {
		return lo
	}
	return i
}

// ContextModel is the T.87 context state shared by the encoder and decoder:
// the A/B/C/N statistics, the gradient quantizer, the Golomb parameter
// derivation and the run-mode statistics.
type ContextModel struct {
	// Quantization thresholds (T.87 A.3).
	T1, T2, T3 int

	MaxVal int // MAXVAL
	Near   int // NEAR (0 = lossless)
	Range  int // RANGE (A.2.1)
	Qbpp   int // bits per mapped error value
	Limit  int // LIMIT: Golomb code length limit
	Reset  int // RESET: statistics halving threshold

	A  [numContexts]int // accumulated error magnitudes
	B  [numContexts]int // bias accumulator
	C  [numContexts]int // prediction correction
	N  [numContexts]int // occurrence counts
	Nn [2]int           // negative-error counters of the run contexts

	RunIndex int // RUNindex
}

// NewContextModel builds a context model with the default thresholds for
// maxVal and near (T.87 A.2.1).
func NewContextModel(maxVal, near, reset int) *ContextModel {
	return NewContextModelPresets(maxVal, near, reset, 0, 0, 0)
}

// NewContextModelPresets builds a context model; a non-zero t1/t2/t3
// overrides the default threshold (LSE ID=1 presets, T.87 C.2.4.1.1).
func NewContextModelPresets(maxVal, near, reset, t1, t2, t3 int) *ContextModel {
	bpp := max(ceilLog2(maxVal+1), 2)
	rng := maxVal + 1
	if near != 0 {
		rng = (maxVal+2*near)/(2*near+1) + 1
	}
	d1, d2, d3 := DefaultThresholds(maxVal, near)
	cm := &ContextModel{
		T1:     d1,
		T2:     d2,
		T3:     d3,
		MaxVal: maxVal,
		Near:   near,
		Range:  rng,
		Qbpp:   ceilLog2(rng),
		Limit:  2 * (bpp + max(bpp, 8)),
		Reset:  reset,
	}
	if t1 != 0 {
		cm.T1 = t1
	}
	if t2 != 0 {
		cm.T2 = t2
	}
	if t3 != 0 {
		cm.T3 = t3
	}
	aInit := max((rng+32)/64, 2)
	for i := range cm.A {
		cm.A[i] = aInit
		cm.N[i] = 1
	}
	return cm
}

// DefaultThresholds computes the default T1/T2/T3 (T.87 A.2.1 / C.2.4.1.1).
func DefaultThresholds(maxVal, near int) (t1, t2, t3 int) {
	if maxVal >= 128 {
		factor := (min(maxVal, 4095) + 128) / 256
		t1 = clampThreshold(factor*(basicT1-2)+2+3*near, near+1, maxVal)
		t2 = clampThreshold(factor*(basicT2-3)+3+5*near, t1, maxVal)
		t3 = clampThreshold(factor*(basicT3-4)+4+7*near, t2, maxVal)
		return t1, t2, t3
	}
	factor := 256 / (maxVal + 1)
	t1 = clampThreshold(max(2, basicT1/factor+3*near), near+1, maxVal)
	t2 = clampThreshold(max(3, basicT2/factor+5*near), t1, maxVal)
	t3 = clampThreshold(max(4, basicT3/factor+7*near), t2, maxVal)
	return t1, t2, t3
}

// QuantizeGradient maps a local gradient to one of 9 regions -4..4 (A.3.3).
func (cm *ContextModel) QuantizeGradient(d int) int {
	switch {
	case d <= -cm.T3:
		return -4
	case d <= -cm.T2:
		return -3
	case d <= -cm.T1:
		return -2
	case d < -cm.Near:
		return -1
	case d <= cm.Near:
		return 0
	case d < cm.T1:
		return 1
	case d < cm.T2:
		return 2
	case d < cm.T3:
		return 3
	}
	return 4
}

// GetContextIndex computes the context index Q (0..364) and the sign (+1 or
// -1) from the three gradients (A.3.4).
func (cm *ContextModel) GetContextIndex(d1, d2, d3 int) (int, int) {
	q1 := cm.QuantizeGradient(d1)
	q2 := cm.QuantizeGradient(d2)
	q3 := cm.QuantizeGradient(d3)
	if q1 < 0 || (q1 == 0 && q2 < 0) || (q1 == 0 && q2 == 0 && q3 < 0) {
		return -(q1*81 + q2*9 + q3), -1
	}
	return q1*81 + q2*9 + q3, 1
}

// golombK returns the smallest k < 31 with n<<k >= a.
func golombK(n, a int) int {
	k := 0
	for k < 31 && n<<k < a {
		k++
	}
	return k
}

// ComputeK returns the Golomb parameter for regular context q (A.5.1).
func (cm *ContextModel) ComputeK(q int) int {
	return golombK(cm.N[q], cm.A[q])
}

// UpdateStats updates A/B/N (A.6.1) and the bias C (A.6.2) of regular
// context q after coding the error errVal.
func (cm *ContextModel) UpdateStats(q, errVal int) {
	cm.B[q] += errVal * (2*cm.Near + 1)
	cm.A[q] += abs(errVal)
	if cm.N[q] == cm.Reset {
		cm.A[q] >>= 1
		if cm.B[q] >= 0 {
			cm.B[q] >>= 1
		} else {
			cm.B[q] = -((1 - cm.B[q]) >> 1)
		}
		cm.N[q] >>= 1
	}
	cm.N[q]++

	switch {
	case cm.B[q] <= -cm.N[q]:
		if cm.C[q] > minC {
			cm.C[q]--
		}
		cm.B[q] += cm.N[q]
		if cm.B[q] <= -cm.N[q] {
			cm.B[q] = -cm.N[q] + 1
		}
	case cm.B[q] > 0:
		if cm.C[q] < maxC {
			cm.C[q]++
		}
		cm.B[q] -= cm.N[q]
		if cm.B[q] > 0 {
			cm.B[q] = 0
		}
	}
}

// ComputeKRun returns the Golomb parameter for a run-interruption sample of
// type riType (0 or 1) (A.7.2).
func (cm *ContextModel) ComputeKRun(riType int) int {
	ctx := runCtxBase + riType
	temp := cm.A[ctx]
	if riType == 1 {
		temp += cm.N[ctx] >> 1
	}
	return golombK(cm.N[ctx], temp)
}

// UpdateRunStats updates the run-interruption statistics (A.7.2).
func (cm *ContextModel) UpdateRunStats(riType int, eMapped uint32, errNegative bool) {
	ctx := runCtxBase + riType
	if errNegative {
		cm.Nn[riType]++
	}
	cm.A[ctx] += (int(eMapped) + 1 - riType) >> 1
	if cm.N[ctx] == cm.Reset {
		cm.A[ctx] >>= 1
		cm.N[ctx] >>= 1
		cm.Nn[riType] >>= 1
	}
	cm.N[ctx]++
}

func (cm *ContextModel) incRunIndex() {
	if cm.RunIndex < 31 {
		cm.RunIndex++
	}
}

func (cm *ContextModel) decRunIndex() {
	if cm.RunIndex > 0 {
		cm.RunIndex--
	}
}
