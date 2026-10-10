package tool

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogObservationFactsLineAnchorRequiresExcerpt(t *testing.T) {
	for _, evidence := range []string{"", "  ", "business=refresh status=failed"} {
		bus := &types.BusContext{Mutable: types.NewMutableState("inspect runtime records")}
		bus.Mutable.SetLogTriage(&types.LogBundle{Observations: []types.LogObservation{{
			Kind: types.LogObservationRuntimeEvent, Summary: "model interpretation",
			Evidence: evidence, LineStart: 1234, LineEnd: 9999, Diagnostic: true, Confidence: 1,
		}}})
		want := evidence == "business=refresh status=failed"
		if got := emitAnalysisRuntimeArtifactHasLineAnchors(bus); got != want {
			t.Fatalf("evidence=%q acquired line-anchor eligibility=%v want=%v", evidence, got, want)
		}
	}
}
