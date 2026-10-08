package agent

import (
	"math"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceRequestedScopeDisclosureUsesQueryReceipt(t *testing.T) {
	start, end := 0.0, 1.0
	profile := &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end, SourceQuote: "0..1"}
	for _, tc := range []struct {
		name               string
		ref                types.ObservationSourceRef
		wantStart, wantEnd float64
	}{
		{"zero_start", types.ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: 1}, 0, 1},
		{"wider_query", types.ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: 2}, 0, 2},
		{"line_selector", types.ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: 1, QueryLineRangeKnown: true, QueryLineStart: 4}, 0, 0},
		{"unknown_query", types.ObservationSourceRef{QueryScopeID: "query"}, 0, 0},
		{"point_query", types.ObservationSourceRef{QueryWindowKnown: true, QueryWindowStartTs: 1, QueryWindowEndTs: 1}, 0, 0},
		{"nonfinite_query", types.ObservationSourceRef{QueryWindowKnown: true, QueryWindowEndTs: math.NaN()}, 0, 0},
		{"legacy_single_window", types.ObservationSourceRef{}, 0.25, 0.75},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := types.ObservationRecord{Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", SourceRef: tc.ref,
				Span: types.ObservationSpan{StartTs: 0, EndTs: 1}, RichNotes: []string{"selected_window=0.25..0.75"}}
			for _, lang := range []string{"zh", "en"} {
				want := types.ResolveTraceQueryWindowScope(profile, tc.wantStart, tc.wantEnd).Format(lang)
				if got := traceQueryObservationRequestedScopeNote(record, profile, lang); got != want {
					t.Fatalf("%s: got %q want %q", lang, got, want)
				}
			}
			record.Producer = "read_file"
			if got := traceQueryObservationRequestedScopeNote(record, profile, "en"); got != "" {
				t.Fatalf("source record acquired query disclosure: %q", got)
			}
		})
	}
}
