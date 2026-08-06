package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestWindowStatsSupplyPressurePublishesTypedAggregateContext(t *testing.T) {
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 34579.472865, EndTs: 34579.587805},
		SupplyPressureSummary: &tracequery.SupplyPressureSummary{
			Signal: "cpu_pressure", CPUPressureMs: 604.528,
			RunnableWaitMs: 604.528, WindowMs: 114.940, PressureDensity: 5.259,
			LineStart: 2736, LineEnd: 15144, Summary: "typed supply pressure",
		},
	}
	records := traceQueryTypedWindowStatsObservations(stats, types.ObservationSourceRef{}, "scope", "at")
	var got *types.ObservationRecord
	for i := range records {
		if records[i].ClaimKey == "supply_pressure:cpu_pressure" {
			got = &records[i]
			break
		}
	}
	if got == nil {
		t.Fatalf("missing typed supply-pressure observation: %+v", records)
	}
	if got.GroundingPolicy != types.ClaimGroundingHard || got.Value != "604.528" || got.Unit != "cpu·ms" {
		t.Fatalf("supply aggregate must preserve typed value/unit/grounding: %+v", *got)
	}
	joined := strings.Join(got.RichNotes, "\n")
	for _, want := range []string{
		"type=supply_pressure",
		"subject_kind=aggregate_metric",
		"chain_relevance=background",
		"selected_window=34579.472865..34579.587805",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("typed aggregate context missing %q:\n%s", want, joined)
		}
	}
	if got := traceQueryRankObservationUnit("supply_pressure"); got != "ms" {
		t.Fatalf("bounded rank proxy keeps its compatibility caliber; the independent aggregate carrier owns cpu·ms, got %q", got)
	}
}
