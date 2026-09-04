package criterion

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInCriterionLiterals is internal/analysis/criterion's
// marker in the glossarylint renderer roster (§40.52). Result.Detail is
// forwarded as contract Violation.Detail; the first red run listed
// "AnchorKind=call" and "typed AnswerDocumentV2".
func TestNoInternalTermsInCriterionLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}
