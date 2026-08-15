package tool

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQuerySemanticFamilyObservationSeparatesFullTotalFromOnChainProjection(t *testing.T) {
	worker := tracequery.ThreadRef{Comm: "worker", PID: 200}
	spans := []tracequery.TraceSpanSummary{{
		Thread: worker, Kind: "sync", Name: "VerifyClass A", SemanticClass: "class_verification",
		StartTs: 5.000, EndTs: 5.010, DurationMs: 10, StartLine: 10, EndLine: 11,
		ActualStartTs: 4.999, ActualEndTs: 5.011, ActualDurationMs: 12,
	}, {
		Thread: worker, Kind: "sync", Name: "VerifyClass B", SemanticClass: "class_verification",
		StartTs: 5.005, EndTs: 5.015, DurationMs: 10, StartLine: 12, EndLine: 13,
		ActualStartTs: 5.005, ActualEndTs: 5.016, ActualDurationMs: 11,
	}}
	chain := &tracequery.ChainResult{Nodes: []tracequery.ChainNode{{
		Thread: worker, Window: tracequery.TimeWindow{StartTs: 5.002, EndTs: 5.008},
		Dominant: tracequery.StateRunning,
	}, {
		Thread: worker, Window: tracequery.TimeWindow{StartTs: 5.006, EndTs: 5.012},
		Dominant: tracequery.StateRunnable,
	}}}
	families := tracequery.FoldSemanticSpanFamilies(chain, spans)
	if len(families) != 1 || !families[0].OnChain {
		t.Fatalf("expected one on-chain family: %+v", families)
	}
	fam := families[0]
	record := traceQuerySemanticSpanFamilyObservation(
		fam, chain,
		tracequery.WindowStats{Window: tracequery.TimeWindow{StartTs: 5, EndTs: 5.020}},
		types.ObservationSourceRef{ArtifactID: "runtime_artifact:semantic"},
		"semantic", "2026-07-10T00:00:00Z", 1, 0,
	)
	if record.Value != "15.000" {
		t.Fatalf("Value must retain the complete selected-window union (15ms), got %q", record.Value)
	}
	notes := strings.Join(record.RichNotes, "\n")
	for _, want := range []string{
		"projected_impact=10.000",
		"overlap=10.000",
		"actual_impact_ms=17.000",
		"chain_relevance=on_chain",
		"on_chain_basis=semantic_chain_interval_relation",
		"effective_impact_ms=0.000",
	} {
		if !strings.Contains(notes, want) {
			t.Fatalf("on-chain family observation omitted %q:\n%s", want, notes)
		}
	}
	if !strings.Contains(record.Summary, "complete selected-window union=15.000ms") ||
		!strings.Contains(record.Summary, "raw typed-chain interval overlap=10.000ms") ||
		!strings.Contains(record.Summary, "target wait/completion binding=unproven") ||
		!strings.Contains(record.Summary, "effective_impact=0.000ms") {
		t.Fatalf("summary must distinguish full disclosure from causal participation: %q", record.Summary)
	}
	if record.Span.StartTs != 5.000 || record.Span.EndTs != 5.015 {
		t.Fatalf("observation span remains the complete member envelope: %+v", record.Span)
	}

	offFamilies := tracequery.FoldSemanticSpanFamilies(nil, spans)
	if len(offFamilies) != 1 || offFamilies[0].OnChain {
		t.Fatalf("expected one off-chain control family: %+v", offFamilies)
	}
	off := traceQuerySemanticSpanFamilyObservation(
		offFamilies[0], nil, tracequery.WindowStats{}, types.ObservationSourceRef{},
		"semantic_off", "2026-07-10T00:00:00Z", 1, 1,
	)
	offNotes := strings.Join(off.RichNotes, "\n")
	if strings.Contains(offNotes, "projected_impact=") || strings.Contains(offNotes, "overlap=") {
		t.Fatalf("off-chain family must not acquire a fake projected/overlap field: %v", off.RichNotes)
	}
	if strings.Contains(off.Summary, "on-chain intersection") || off.Value != "15.000" {
		t.Fatalf("off-chain observation behavior must stay unchanged: %+v", off)
	}
}
