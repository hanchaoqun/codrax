package hint

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInHintComposerLiterals is internal/analysis/hint's
// marker in the glossarylint renderer roster (§40.52). The composer's
// Hint fields and Allowed hints render straight into the next
// dispatch's Retry Directive; the first red run listed "ReadSet" and
// "Turn A" in four places.
func TestNoInternalTermsInHintComposerLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}
