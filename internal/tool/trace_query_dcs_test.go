package tool

// trace_query_dcs_test.go — DCS 修复批 tool-half pins (ledger
// real_trace_campaign_20260705.md §23/§23.1, 2026-07-08): the E5 producer
// half (typed source-window identity + E4 actual_* dual basis on semantic
// span observations), the E6 background_rank note双门 emission scope, and the
// LLM-facing description's deterministic_optimization semantics.

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// E5 producer half: every semantic span observation carries its typed SOURCE
// query window (selected_window, same key+format as the CWD-2 rank identity),
// and a window-clipped span additionally publishes its physical extent on the
// registered actual_* dual-basis keys. Unclipped spans stay actual-free
// (absence = not clipped).
func TestDCSSemanticObservationCarriesSourceWindowAndActualExtent(t *testing.T) {
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 8144.358, EndTs: 8145.242},
		TraceSpans: []tracequery.TraceSpanSummary{
			{
				Thread: tracequery.ThreadRef{Comm: "jit00", PID: 900},
				Name:   "JIT compiling method00", SemanticClass: "jit_compile",
				StartTs: 8144.972, EndTs: 8145.056, DurationMs: 83.893,
				StartLine: 769, EndLine: 770,
			},
			{
				Thread: tracequery.ThreadRef{Comm: "jit01", PID: 901},
				Name:   "JIT compiling method01", SemanticClass: "jit_compile",
				StartTs: 8145.100, EndTs: 8145.242, DurationMs: 142.0,
				ActualStartTs: 8145.100, ActualEndTs: 8145.300, ActualDurationMs: 200.0,
				StartLine: 780, EndLine: 781,
			},
		},
	}
	records := traceQueryTypedSemanticTraceSpanObservations(tracequery.Result{}, stats,
		types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "a.systrace", ArtifactKind: "trace"},
		"scope", time.Unix(1751600000, 0).UTC().Format(time.RFC3339))
	if len(records) != 2 {
		t.Fatalf("expected two semantic observations, got %d", len(records))
	}
	first := strings.Join(records[0].RichNotes, "\n")
	if !strings.Contains(first, "selected_window=8144.358000..8145.242000") {
		t.Fatalf("semantic observation must carry its typed source query window: %v", records[0].RichNotes)
	}
	if strings.Contains(first, "actual_impact_ms=") || strings.Contains(first, "actual_window=") {
		t.Fatalf("an unclipped span must not fake an actual extent: %v", records[0].RichNotes)
	}
	second := strings.Join(records[1].RichNotes, "\n")
	if !strings.Contains(second, "actual_impact_ms=200.000") ||
		!strings.Contains(second, "actual_window=8145.100000..8145.300000") {
		t.Fatalf("a window-clipped span must publish its physical extent on the actual_* keys: %v", records[1].RichNotes)
	}
}

// E6 double-gate note scope: the typed background_rank note rides semantic
// compile span rank rows ONLY — a non-semantic background row never carries
// it, and an on-chain semantic row (BackgroundRank=0) never carries it.
func TestDCSBackgroundRankNoteEmissionScope(t *testing.T) {
	result := tracequery.Result{RootCauseRank: &tracequery.RootCauseRankResult{
		Window: tracequery.TimeWindow{StartTs: 1.0, EndTs: 1.101},
		Items: []tracequery.RootCauseRankItem{
			{
				Rank: 1, Tier: "primary", Type: "runnable_wait", BackgroundRank: 1,
				Thread: tracequery.ThreadRef{Comm: "bg", PID: 300}, ImpactMs: 90,
				Summary: "bg runnable",
			},
			{
				Rank: 2, Tier: "tertiary", Type: "jit_compile", BackgroundRank: 2,
				Thread: tracequery.ThreadRef{Comm: "jit00", PID: 900}, ImpactMs: 83.893,
				SpanName: "JIT compiling method00", SemanticClass: "jit_compile",
				Summary: "jit span",
			},
			{
				Rank: 3, Tier: tracequery.RootCauseTierDeterministicOptimization, Type: "class_verification",
				Thread: tracequery.ThreadRef{Comm: "worker", PID: 200}, ImpactMs: 5.6,
				ChainRelevance: "on_chain", Causality: "on_wakeup_chain",
				SpanName: "VerifyClass com.example.Foo", SemanticClass: "class_verification",
				Summary: "verify span",
			},
		},
	}}
	records := traceQueryTypedObservations(result, "a.systrace", "payload", "raw", "", time.Unix(1751600000, 0).UTC())
	notesFor := func(substr string) string {
		for _, record := range records {
			if strings.Contains(record.Summary, substr) {
				return strings.Join(record.RichNotes, "\n")
			}
		}
		t.Fatalf("no observation for %q", substr)
		return ""
	}
	if notes := notesFor("jit span"); !strings.Contains(notes, "background_rank=2") {
		t.Fatalf("non-chain semantic rank row must publish its typed board position: %v", notes)
	}
	if notes := notesFor("bg runnable"); strings.Contains(notes, "background_rank=") {
		t.Fatalf("non-semantic rows never carry the background_rank note: %v", notes)
	}
	onChain := notesFor("verify span")
	if strings.Contains(onChain, "background_rank=") {
		t.Fatalf("on-chain semantic rows are not on the background board: %v", onChain)
	}
	if !strings.Contains(onChain, "tier=deterministic_optimization") {
		t.Fatalf("the on-chain semantic row must publish its tier note: %v", onChain)
	}
}

// LLM-facing description pin: the semantic span-work semantics speak the new
// tier/board tokens and the co-primary sentence no longer lists semantic
// span-work (E1 不与选举竞争 — prompt face half).
func TestDCSTraceQueryDescriptionSpeaksOptimizationTier(t *testing.T) {
	description := (&TraceQuery{}).Description()
	for _, want := range []string{
		"tier=deterministic_optimization",
		"background_rank",
		"never as the root cause itself",
		"semantic span-work rows never join that election",
	} {
		if !strings.Contains(description, want) {
			t.Fatalf("description missing %q", want)
		}
	}
	if strings.Contains(description, "running/compute-supply, semantic span-work, low-frequency") {
		t.Fatalf("the co-primary sentence must no longer list semantic span-work")
	}
}
