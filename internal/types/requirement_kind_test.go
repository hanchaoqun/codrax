package types

import "testing"

func TestIsNegativeEvidencePredicate_NormalizesSeparatorVariants(t *testing.T) {
	for _, predicate := range []string{
		"absent from",
		"absent_from",
		"absent-from",
		"confirms_absent_from",
		"is absent from YAML struct",
		"missing from cli overrides",
		"is omitted from runtime binding",
	} {
		if !IsNegativeEvidencePredicate(predicate) {
			t.Fatalf("IsNegativeEvidencePredicate(%q)=false, want true", predicate)
		}
	}
}
