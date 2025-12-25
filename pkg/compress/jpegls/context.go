package jpegls

// ContextModel maintains the state for context modeling (gradients and bias).
type ContextModel struct {
	// Quantization thresholds
	T1, T2, T3 int

	// Limits
	MaxVal int
	Range  int // 2*Near + 1? Or MAXVAL?

	// Context Stats (Array of 365 or 367 contexts)
	// 365 standard contexts + 2 run contexts?
	// Standard map size is 365.
	// Indices computed from Q1, Q2, Q3.
	// Stat entries:
	A []int // Accumulator of absolute errors?
	B []int // Bias correction
	C []int // Bias correction variable? No, C is Main stats?
	N []int // Occurrences

	// Bias correction
	Reset int // Reset parameter

	// Run Mode Stats
	J        [32]int // Run Mode Index
	RunIndex int     // Current Run Index
}

// NewContextModel initializes context model based on MAXVAL and optional params.
// Default thresholds: T1 = 3, T2 = 7, T3 = 21 (for 8-bit).
// For general MAXVAL: derived.
func NewContextModel(maxVal int, near int, reset int) *ContextModel {
	cm := &ContextModel{
		MaxVal: maxVal,
		Reset:  reset, // Default usually 64
	}

	// T1, T2, T3 Calculation (ISO 14495-1 A.3)
	// BASIC_T1 = 3, etc.
	// If MaxVal >= 128:
	// Factor = (min(MaxVal, 4095) + 128) / 256
	// T1 = Clamp(Factor * (3-2) + 2 + 3*Near, Near+1, MaxVal)
	// Detailed formula required.
	// For 8-bit (MaxVal=255): T1=3, T2=7, T3=21.
	// Simplifying for now (Fixed to 8-bit default if unknown, or implement calc later).
	// Let's implement Factor mechanism.

	factor := (min(maxVal, 4095) + 128) / 256

	cm.T1 = factor*(3-2) + 2 + 3*near  // Default T1=3
	cm.T2 = factor*(7-3) + 3 + 5*near  // Default T2=7
	cm.T3 = factor*(21-4) + 4 + 7*near // Default T3=21

	// Clamp
	cm.T1 = clip(cm.T1, near+1, maxVal)
	cm.T2 = clip(cm.T2, cm.T1, maxVal)
	cm.T3 = clip(cm.T3, cm.T2, maxVal)

	// Allocate stats
	// Size = 365 contexts.
	// Run mode contexts are separate?
	// Total 367? 365 regular + 2 run?
	// Size = 365 contexts + 2 run interruption contexts (365, 366).
	size := 367
	cm.A = make([]int, size)
	cm.B = make([]int, size)
	cm.C = make([]int, size)
	cm.N = make([]int, size)

	// Init stats (A.2)
	for i := 0; i < size; i++ {
		cm.A[i] = 4 // Initial A
		cm.N[i] = 1 // Initial N
		cm.B[i] = 0
		cm.C[i] = 0 // Not strictly standard var name, usually B and C are related to Bias?
		// Standard uses A[Q], B[Q], C[Q], N[Q].
		// "Bias" variable is B[Q] * SIGN?
		// Bias correction value C[Q]? No. Bias is B[Q].
		// C[Q] is "Correction"?
		// Wait, standard uses A, B, C, N variables.
		// A: Sum of absolute errors.
		// B: Sum of errors.
		// C: Prediction Correction?
		// N: Count.
		// A: Sum of absolute errors.
		// B: Sum of errors.
		// C: Prediction Correction?
		// N: Count.
	}

	// Init J Table (ISO 14495-1 Table A.3)
	jValues := []int{0, 0, 0, 0, 1, 1, 1, 1, 2, 2, 2, 2, 3, 3, 3, 3, 4, 4, 5, 5, 6, 6, 7, 7, 8, 9, 10, 11, 12, 13, 14, 15}
	copy(cm.J[:], jValues)

	return cm
}

// QuantizeGradient calculates quantization region Q based on difference D.
func (cm *ContextModel) QuantizeGradient(D int) int {
	if D <= -cm.T3 {
		return -4
	}
	if D <= -cm.T2 {
		return -3
	}
	if D <= -cm.T1 {
		return -2
	}
	if D < 0 {
		return -1
	}
	if D == 0 {
		return 0
	}
	if D < cm.T1 {
		return 1
	}
	if D < cm.T2 {
		return 2
	}
	if D < cm.T3 {
		return 3
	}
	return 4
}

// ComputeK calculates the Golomb-Rice parameter k for context Q.
func (cm *ContextModel) ComputeK(Q int) int {
	n := cm.N[Q]
	if n == 0 {
		return 0
	}
	a := cm.A[Q]

	k := 0
	for (n << k) < a {
		k++
	}
	return k
}

// UpdateStats updates context Q with the prediction error ErrVal.
// ErrVal is the UNMAPPED error (raw difference).
func (cm *ContextModel) UpdateStats(Q int, ErrVal int) {
	// Update B[Q]
	cm.B[Q] += ErrVal
	// Update A[Q]
	cm.A[Q] += abs(ErrVal)

	if cm.N[Q] == cm.Reset {
		cm.A[Q] >>= 1
		cm.B[Q] >>= 1
		cm.N[Q] >>= 1
	}
	cm.N[Q]++
	cm.updateBias(Q)
}

func (cm *ContextModel) updateBias(Q int) {
	if cm.B[Q] <= -cm.N[Q] {
		cm.B[Q] += cm.N[Q]
		cm.C[Q]--
		if cm.B[Q] <= -cm.N[Q] {
			cm.B[Q] += cm.N[Q]
			cm.C[Q]--
		}
	} else if cm.B[Q] > 0 {
		cm.B[Q] -= cm.N[Q]
		cm.C[Q]++
		if cm.B[Q] > 0 {
			cm.B[Q] -= cm.N[Q]
			cm.C[Q]++
		}
	}
	// Clamp C[Q] to -128..127 checking?
	// Standard implies bounds handling.
	if cm.C[Q] < -128 {
		cm.C[Q] = -128
	}
	if cm.C[Q] > 127 {
		cm.C[Q] = 127
	}
}

// GetContextIndex computes Q from gradients D1, D2, D3
func (cm *ContextModel) GetContextIndex(D1, D2, D3 int) (int, int) {
	Q1 := cm.QuantizeGradient(D1)
	Q2 := cm.QuantizeGradient(D2)
	Q3 := cm.QuantizeGradient(D3)

	// Sign mapping - if first non-zero gradient is negative, negate all and set sign=-1
	sign := 1
	if Q1 < 0 || (Q1 == 0 && Q2 < 0) || (Q1 == 0 && Q2 == 0 && Q3 < 0) {
		Q1 = -Q1
		Q2 = -Q2
		Q3 = -Q3
		sign = -1
	}

	// After sign normalization:
	// Q1 is in range [0, 4]
	// Q2 is in range [-4, 4]
	// Q3 is in range [-4, 4]

	// ISO 14495-1 Figure A.5 - Determination of context index Q
	// Q1 in [0,4], Q2 in [-4,4], Q3 in [-4,4]
	// Index = (Q1 * 81) + (Q2 * 9) + Q3
	// After sign normalization, this index is guaranteed to be in [0, 364]
	index := Q1*81 + Q2*9 + Q3

	return index, sign
}
