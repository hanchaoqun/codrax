package agent

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestSamplingQueryBoundaryHMC239DoesNotGrantSubjectAuthority(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	path := filepath.Join(t.TempDir(), "unassigned.perftrace")
	if err := os.WriteFile(path, []byte("worker-11 (10) [000] .... 1.1: perf_sample: pid=10 tid=11 thread_identity_known=false sample_weight=7 event=cycles source=raw\n"), 0600); err != nil {
		t.Fatal(err)
	}
	query, _ := json.Marshal(map[string]any{"path": path, "view": "perf_sampling_tree", "pid": 11, "time_start": 1, "time_end": 2})
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), query)
	if err != nil || !result.Success {
		t.Fatalf("public query: %v %+v", err, result)
	}
	var original types.ObservationRecord
	for _, row := range result.Observations {
		if row.Predicate == tool.TracePerfSamplingTreePredicate {
			original = row
		}
	}
	rm := &ctx.AnalysisIR.RequestModel
	before, _ := json.Marshal(original)
	for _, tc := range []struct {
		name string
		edit func(*types.ObservationRecord)
		keep bool
	}{
		{"bound unknown query", func(*types.ObservationRecord) {}, true},
		{"different source", func(r *types.ObservationRecord) { r.SourceRef.Path += ".other" }, false},
		{"different window", func(r *types.ObservationRecord) { r.SourceRef.QueryWindowEndTs++ }, false},
		{"different selector", func(r *types.ObservationRecord) { r.SourceRef.QueryTargetPID++ }, false},
		{"different span", func(r *types.ObservationRecord) { r.Span.EndTs++ }, false},
		{"principal role", func(r *types.ObservationRecord) { r.Role = types.AnswerAggregateRolePrincipalAnswer }, false},
		{"model producer", func(r *types.ObservationRecord) { r.Producer = "model" }, false},
		{"fake subject", func(r *types.ObservationRecord) { r.Subject = "worker-11" }, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := original
			tc.edit(&r)
			if got := answerDocSamplingQueryBoundaryMatchesTarget(r, rm); got != tc.keep {
				t.Fatalf("boundary keep=%t want=%t", got, tc.keep)
			}
			if _, owned := tool.TraceQueryAggregateSelectionSubject(r); owned {
				t.Fatal("query boundary acquired observed aggregate subject")
			}
		})
	}
	for _, target := range []types.RuntimeTarget{
		{Kind: types.RuntimeTargetKindThread, PID: 12, Thread: "worker", Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindProcess, PID: 11, Source: "user_explicit"},
		{Kind: types.RuntimeTargetKindThread, PID: 11, Source: types.RuntimeTargetSourceExplicitToolCall},
	} {
		other := *rm
		other.RuntimeTargets = []types.RuntimeTarget{target}
		if answerDocSamplingQueryBoundaryMatchesTarget(original, &other) {
			t.Fatalf("unrelated target retained boundary: %+v", target)
		}
	}
	after, _ := json.Marshal(original)
	if string(before) != string(after) {
		t.Fatal("prompt boundary changed original ledger record")
	}
}

func TestSamplingQueryBoundaryHMC239RealEmptyStillVisible(t *testing.T) {
	ctx := samplingSelectionAnalysisContext(t)
	result, err := (&tool.TraceQuery{}).Execute(types.ToolBusContext(ctx, types.AgentExplorer), json.RawMessage(`{"source":"attached_trace","view":"perf_sampling_tree","pid":11,"time_start":4,"time_end":5}`))
	if err != nil || !result.Success {
		t.Fatalf("public empty query: %v %+v", err, result)
	}
	for _, row := range result.Observations {
		if row.Predicate != tool.TracePerfSamplingTreePredicate {
			continue
		}
		p, valid := tool.DecodeTracePerfSamplingTree(row)
		if !valid || p.Status != "available" || p.SampleCount != 0 || !answerDocSamplingQueryBoundaryMatchesTarget(row, &ctx.AnalysisIR.RequestModel) {
			t.Fatalf("real empty outcome lost: %+v", p)
		}
		return
	}
	t.Fatal("no native empty result")
}
