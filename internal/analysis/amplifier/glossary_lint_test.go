package amplifier

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill/glossarylint"
)

// TestNoInternalTermsInAmplifierLiterals is internal/analysis/amplifier's
// marker in the glossarylint renderer roster (§40.52). Amplification
// Reason rows render into the analyzer retry directive (previously
// rescued only at runtime by the agent-side vocabulary sanitizer); the
// first red run listed "AnalyzerHints.Entities has …" twice.
func TestNoInternalTermsInAmplifierLiterals(t *testing.T) {
	glossarylint.RunPackageScan(t, ".", glossarylint.Policy{})
}
