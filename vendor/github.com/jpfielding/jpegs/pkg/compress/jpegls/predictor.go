package jpegls

// PredictMED implements the LOCO-I median edge detection predictor.
// Ra: left, Rb: above, Rc: above-left.
func PredictMED(Ra, Rb, Rc int) int {
	if Rc >= max(Ra, Rb) {
		return min(Ra, Rb)
	}
	if Rc <= min(Ra, Rb) {
		return max(Ra, Rb)
	}
	return Ra + Rb - Rc
}

// clip clamps val to [lo, hi].
func clip(val, lo, hi int) int {
	if val < lo {
		return lo
	}
	if val > hi {
		return hi
	}
	return val
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
