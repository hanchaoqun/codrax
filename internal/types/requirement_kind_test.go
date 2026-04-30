package types

import "testing"

func TestIsNegativeEvidencePredicate_NormalizesSeparatorVariants(t *testing.T) {
	for _, predicate := range []string{
		"absent from",
		"absent_from",
		"absent-from",
		"confirms_absent_from",
	} {
		if !IsNegativeEvidencePredicate(predicate) {
			t.Fatalf("IsNegativeEvidencePredicate(%q)=false, want true", predicate)
		}
	}
}
