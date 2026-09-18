package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestTraceQueryBusinessSpanKeepsIndependentFactsAndSemanticAuthority(t *testing.T) {
	stats := tracequery.WindowStats{
		Window: tracequery.TimeWindow{StartTs: 5, EndTs: 5.01},
		TraceSpans: []tracequery.TraceSpanSummary{
			{Thread: tracequery.ThreadRef{Comm: "worker", PID: 7}, Name: "LoadDocumentIndex", Kind: "sync", Category: "trace_span", StartTs: 5, EndTs: 5.01, DurationMs: 10, StartLine: 3, EndLine: 8, ActualStartTs: 4.98, ActualEndTs: 5.03, ActualDurationMs: 50},
			{Thread: tracequery.ThreadRef{Comm: "worker", PID: 7}, Name: "VerifyClass Demo", Kind: "sync", SemanticClass: "class_verification", StartTs: 5, EndTs: 5.005, DurationMs: 5, StartLine: 4, EndLine: 6},
		},
	}
	ref := types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/capture/events", ArtifactID: "capture-A", ArtifactKind: "trace"}
	before, _ := json.Marshal(stats)
	rows := traceQueryTypedBusinessSpanObservations(stats, ref, "payload", "now")
	if len(rows) != 1 {
		t.Fatalf("ordinary/known-semantic lanes overlapped: %+v", rows)
	}
	r := rows[0]
	if r.Predicate != types.TraceBusinessSpanPredicate || r.Object != "LoadDocumentIndex" || r.Subject != "worker-7" || r.Value != "10.000" || r.SourceRef != ref || r.Span.StartTs != 5 || r.Span.EndTs != 5.01 || r.Span.LineStart != 3 || r.Span.LineEnd != 8 {
		t.Fatalf("business measurement identity changed: %+v", r)
	}
	for _, note := range []string{"selected_window=5.000000..5.010000", "actual_impact_ms=50.000", "actual_window=4.980000..5.030000", "span_name=LoadDocumentIndex"} {
		if !strings.Contains(strings.Join(r.RichNotes, "\n"), note) {
			t.Errorf("lost exact dual-basis detail %q: %+v", note, r)
		}
	}
	for _, forbidden := range []string{"chain_relevance=", "on_chain_basis=", "effective_impact_ms=", "semantic_class="} {
		if strings.Contains(strings.Join(r.RichNotes, "\n"), forbidden) {
			t.Errorf("ordinary work acquired causal/optimization authority: %s", forbidden)
		}
	}
	if !reflect.DeepEqual(r.SupportRefs, []string{"/capture/events:3-8"}) {
		t.Fatalf("wrong physical line support: %+v", r.SupportRefs)
	}
	semantic := traceQueryTypedSemanticTraceSpanObservations(tracequery.Result{}, stats, ref, "payload", "now")
	base := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: semantic})
	withBusiness := types.CompileTraceCausalProjectionSet(types.ObservationLedger{Records: append(semantic, rows...)})
	if !reflect.DeepEqual(base, withBusiness) {
		t.Fatal("ordinary business publication changed the semantic/causal projection")
	}
	after, _ := json.Marshal(stats)
	if string(before) != string(after) {
		t.Fatal("publication mutated parsed source data")
	}
}
