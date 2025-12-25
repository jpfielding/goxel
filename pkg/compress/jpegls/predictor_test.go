package jpegls

import "testing"

func TestPredictMED(t *testing.T) {
	tests := []struct {
		Ra, Rb, Rc int
		Want       int
	}{
		{10, 10, 10, 10},
		{100, 200, 300, 100}, // Rb (200) >= max(100, 300)? No. Max=300. Rc=300 >= 300? YES. Result min(100,200)=100.
		{200, 100, 50, 200},  // Rb (100) <= min(200, 50)? No. Min=50. Rc=50 <= 50? YES. Result max(200,100)=200.
		{10, 30, 20, 20},     // Else. Max=30, Min=10. Rc=20. 10+30-20 = 20.
	}

	for _, tt := range tests {
		if got := PredictMED(tt.Ra, tt.Rb, tt.Rc); got != tt.Want {
			t.Errorf("PredictMED(%d, %d, %d) = %d; want %d", tt.Ra, tt.Rb, tt.Rc, got, tt.Want)
		}
	}
}

func TestContextModel_GetContextIndex(t *testing.T) {
	cm := NewContextModel(255, 0, 64)

	// Zero Gradients
	idx, sign := cm.GetContextIndex(0, 0, 0)
	if idx != 0 {
		t.Errorf("Zero Context: Got %d want 0", idx)
	}
	if sign != 1 {
		t.Errorf("Zero Sign: Got %d want 1", sign)
	}

	// T1=3. D1=5 -> Q1=2.
	// Idx = 2*81 = 162.
	idx, _ = cm.GetContextIndex(5, 0, 0)
	if idx != 162 {
		t.Errorf("Pos Context: Got %d want 162", idx)
	}

	// Negative D1=-5 -> Q1=-2. Flip -> Q1=2.
	idx2, sign2 := cm.GetContextIndex(-5, 0, 0)
	if idx2 != 162 {
		t.Errorf("Neg Context (Index): Got %d want 162", idx2)
	}
	if sign2 != -1 {
		t.Errorf("Neg Context (Sign): Got %d want -1", sign2)
	}
}
