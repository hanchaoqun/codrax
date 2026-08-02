package types

import "testing"

func TestTraceCausalProjectionSelfRunnableTwoRulerValid(t *testing.T) {
	valid := TraceCausalProjectionSelfRunnableTwoRuler{
		Subject:    "target-10",
		WallEffsMS: []float64{3.956, 1.193}, WallRanks: []int{4, 13}, WallSubtotalMS: 5.149,
		EdgeEffsMS: []float64{1.648}, EdgeRanks: []int{10}, EdgeSubtotalMS: 1.648,
	}
	if !TraceCausalProjectionSelfRunnableTwoRulerValid(valid) {
		t.Fatal("expected complete two-ruler identity to be valid")
	}

	invalid := valid
	invalid.WallSubtotalMS = 6.797 // illegally combines the edge ruler
	if TraceCausalProjectionSelfRunnableTwoRulerValid(invalid) {
		t.Fatal("cross-ruler total must not satisfy the wall-ruler identity")
	}

	invalid = valid
	invalid.EdgeRanks = nil
	if TraceCausalProjectionSelfRunnableTwoRulerValid(invalid) {
		t.Fatal("non-parallel value/rank roster must fail closed")
	}
}
