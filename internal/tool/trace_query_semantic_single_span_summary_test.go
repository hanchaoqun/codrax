package tool

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

// trace_query_semantic_single_span_summary_test.go — CROWNSEM-1 复核收编
// (batch-one adversarial review, 2026-09-02): the single-member
// trace_semantic_span observation record must speak the same credential rule
// as the family summary and its own typed effective note. B829 had left the
// n=1 record prose at "effective_impact=0.000ms" beside a RichNote lane that
// now carries the priced pre-edge share — one record, two effective values,
// and the prose is what the model reads.
func TestSingleSpanSemanticRecordSummarySpeaksPricedPreEdgeShare(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticTiebaTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), elimSemanticTiebaTrace)
	if err != nil {
		t.Fatal(err)
	}
	// The tieba sentinel: host 61839's VerifyClass span (34579.495841..496126)
	// lies fully before its 34579.496810 wakeup edge toward target 59566.
	q := tracequery.Query{PID: 59566, TimeStart: 34579.490, TimeEnd: 34579.500, View: "root_cause_rank",
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace, Limit: 40}
	result := tracequery.Run(idx, q)
	at := time.Unix(1751600000, 0).UTC()
	obs := traceQueryTypedObservations(result, "fixture", "p-root_cause_rank", "r", "", at)
	found := false
	for _, record := range obs {
		if !strings.Contains(record.ID, "#trace_semantic_span:") || !strings.Contains(record.Summary, "VerifyClass") ||
			!strings.Contains(record.Summary, "chain_relevance=on_chain") {
			continue
		}
		found = true
		if strings.Contains(record.Summary, "effective_impact=0.000ms") || strings.Contains(record.Summary, "raw_pre_edge_occupancy") {
			t.Fatalf("the n=1 record prose still speaks the retired relation-only caliber: %s", record.Summary)
		}
		if !strings.Contains(record.Summary, "pre_edge_share=0.285ms priced on-chain") ||
			!strings.Contains(record.Summary, "effective_impact=0.285ms") ||
			!strings.Contains(record.Summary, "semantic_completion_mechanism=unproven (disclosure only)") {
			t.Fatalf("the n=1 record prose must carry the priced pre-edge share, the credential rule and the mechanism disclosure: %s", record.Summary)
		}
		// The typed note lane and the prose agree on ONE effective value.
		noteEffective := ""
		for _, note := range record.RichNotes {
			if strings.HasPrefix(note, "effective_impact_ms=") {
				noteEffective = strings.TrimPrefix(note, "effective_impact_ms=")
			}
		}
		if noteEffective == "" || !strings.Contains(record.Summary, "effective_impact="+noteEffective+"ms") {
			t.Fatalf("typed note effective (%q) and prose effective must be the same value: %s | notes=%v", noteEffective, record.Summary, record.RichNotes)
		}
	}
	if !found {
		t.Fatalf("sentinel must expose the on-chain VerifyClass single-span record; got %d observations", len(obs))
	}
}
