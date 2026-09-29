package criterion

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRuntimeObservationCountRequiresMeasuredMetaSignals(t *testing.T) {
	b := &types.PerfBundle{Meta: types.PerfMeta{Signals: []string{"io-block"}}}
	if got := runtimeArtifactObservationCount(nil, b); got != 0 {
		t.Fatalf("model-only label counted as evidence: %d", got)
	}
	b.Meta.Authority = types.PerfObservationAuthorityDeterministicValidator
	if got := runtimeArtifactObservationCount(nil, b); got != 1 {
		t.Fatalf("verified label lost: %d", got)
	}
}
