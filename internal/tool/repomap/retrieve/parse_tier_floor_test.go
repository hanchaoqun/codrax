package retrieve

import (
	"testing"
)

// TestSetMinParseTierFloor_NegativeClamps pins the input gate.
func TestSetMinParseTierFloor_NegativeClamps(t *testing.T) {
	t.Cleanup(func() { SetMinParseTierFloor(0) })
	SetMinParseTierFloor(-1)
	if minParseTierFloor != 0 {
		t.Errorf("negative arg should clamp to 0; got %d", minParseTierFloor)
	}
}

// TestApplyParseTierFloor_DisabledIsByteIdentical pins back-compat:
// floor=0 (default) returns score unchanged for every tier.
func TestApplyParseTierFloor_DisabledIsByteIdentical(t *testing.T) {
	t.Cleanup(func() { SetMinParseTierFloor(0) })
	SetMinParseTierFloor(0)
	for tier := 0; tier <= 4; tier++ {
		if got := applyParseTierFloor(42.0, tier); got != 42.0 {
			t.Errorf("floor=0 tier=%d should pass through; got %v", tier, got)
		}
	}
}

// TestApplyParseTierFloor_DropsAboveFloor pins the hard-gate.
func TestApplyParseTierFloor_DropsAboveFloor(t *testing.T) {
	t.Cleanup(func() { SetMinParseTierFloor(0) })
	SetMinParseTierFloor(2)
	cases := []struct {
		tier  int
		input float64
		want  float64
	}{
		{0, 42, 42}, // unset → treated as Tier 1
		{1, 42, 42},
		{2, 42, 42}, // at floor (strict-greater-than gate)
		{3, 42, 0},  // above floor → drop
		{4, 42, 0},  // above floor → drop
	}
	for _, c := range cases {
		if got := applyParseTierFloor(c.input, c.tier); got != c.want {
			t.Errorf("tier=%d floor=2: got %v, want %v", c.tier, got, c.want)
		}
	}
}

// TestApplyParseTierFloor_FloorOfFour pins the "drop nothing"
// behaviour at the upper boundary.
func TestApplyParseTierFloor_FloorOfFour(t *testing.T) {
	t.Cleanup(func() { SetMinParseTierFloor(0) })
	SetMinParseTierFloor(4)
	for tier := 0; tier <= 4; tier++ {
		if got := applyParseTierFloor(42.0, tier); got != 42.0 {
			t.Errorf("floor=4 tier=%d should NOT drop; got %v", tier, got)
		}
	}
}
