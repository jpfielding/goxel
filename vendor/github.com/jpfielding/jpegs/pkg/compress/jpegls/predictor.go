package jpegls

// PredictMED implements Median Edge Detection predictor.
// Ra: Left
// Rb: Above
// Rc: Above-Left
func PredictMED(Ra, Rb, Rc int) int {
	if Rc >= max(Ra, Rb) {
		return min(Ra, Rb)
	}
	if Rc <= min(Ra, Rb) {
		return max(Ra, Rb)
	}
	return Ra + Rb - Rc
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Clip clamps value to range [min, max]
func clip(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}

// Abs returns absolute value
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
