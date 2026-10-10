package criterion

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestLogObservationFactsCountRequiresExcerpt(t *testing.T) {
	for _, evidence := range []string{"", "  ", "business=refresh status=failed"} {
		bundle := &types.LogBundle{Observations: []types.LogObservation{{
			Kind: types.LogObservationRuntimeEvent, Summary: "model interpretation",
			Evidence: evidence, LineStart: 1234, LineEnd: 9999, Diagnostic: true, Confidence: 1,
		}}}
		want := 0
		if evidence == "business=refresh status=failed" {
			want = 1
		}
		if count := runtimeArtifactObservationCount(bundle, nil); count != want {
			t.Fatalf("evidence=%q count=%d want=%d", evidence, count, want)
		}
	}
	stack := &types.LogBundle{Errors: []types.LogError{{Frames: []types.LogFrame{{Raw: "at Catalog.load(Catalog.java:7)"}}}}}
	if count := runtimeArtifactObservationCount(stack, nil); count != 1 {
		t.Fatalf("unrelated stack support was dropped: %d", count)
	}
}
