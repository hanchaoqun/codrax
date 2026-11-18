package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	promptcontext "github.com/hanchaoqun/codrax/internal/context"
	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

const (
	hmc376OffChainVocabularySpan = "GC: Wait For Completion Alloc"
	hmc376OnChainVocabularySpan  = "H:FullGC::RunPhases;GCReason2"
)

func hmc376SemanticNode(subject, span, relevance string, impact float64) types.TraceCausalProjectionNode {
	return types.TraceCausalProjectionNode{Subject: subject, SpanName: span, SemanticClass: "gc_pause",
		ChainRelevance: relevance, ImpactMS: impact, LineStart: int(impact) + 1, LineEnd: int(impact) + 1}
}

func hmc376Subjects(nodes []types.TraceCausalProjectionNode) string {
	subjects := make([]string, 0, len(nodes))
	for _, node := range nodes {
		subjects = append(subjects, node.Subject)
	}
	return strings.Join(subjects, " ")
}

// The two decision inventories are bounded by value. A longer off-chain row of
// vocabulary-classified work follows every other row, and an on-chain one does
// not take the seat of a row that would have been listed without it.
func TestHMC376DecisionInventoriesKeepPreviouslyListedRows(t *testing.T) {
	var projection types.TraceCausalProjection
	for i := 0; i < 3; i++ {
		projection.SemanticSpans = append(projection.SemanticSpans,
			hmc376SemanticNode(fmt.Sprintf("builtin-%d", i), "VerifyClass com.example.T", "background", float64(i+1)))
	}
	projection.SemanticSpans = append(projection.SemanticSpans,
		hmc376SemanticNode("sibling", hmc376OffChainVocabularySpan, "background", 300),
		hmc376SemanticNode("target", hmc376OnChainVocabularySpan, "on_chain", 200))
	const want = "target builtin-2 builtin-1"
	if got := hmc376Subjects(traceDecisionSemanticOptimizationSpans(projection, 2)); got != want {
		t.Fatalf("semantic inventory %q, want %q", got, want)
	}
	if got := hmc376Subjects(traceDecisionActualOccupancyCandidates(projection, 2)); got != want {
		t.Fatalf("occupancy candidates %q, want %q", got, want)
	}
	// Unbounded, the off-chain row is listed after every other row.
	const all = "target builtin-2 builtin-1 builtin-0 sibling"
	if got := hmc376Subjects(traceDecisionSemanticOptimizationSpans(projection, 0)); got != all {
		t.Fatalf("unbounded inventory %q, want %q", got, all)
	}
}

// Eight families already fill the bounded fact inventory; three longer sibling
// waits of vocabulary-classified work do not take any of its seats.
func TestHMC376BoundedSemanticFactsKeepPreviouslyListedFamilies(t *testing.T) {
	dir := t.TempDir()
	var trace strings.Builder
	trace.WriteString("# tracer: nop\n")
	line := func(format string, args ...any) { fmt.Fprintf(&trace, format+"\n", args...) }
	line("idle-0 (0) [000] .... 10.000000: sched_switch: prev_comm=idle prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=client next_pid=100 next_prio=120")
	line("client-100 (100) [000] .... 10.000500: sched_switch: prev_comm=client prev_pid=100 prev_prio=120 prev_state=S ==> next_comm=worker next_pid=300 next_prio=120")
	for i := 0; i < 3; i++ {
		line("waiter-%d (100) [002] .... 10.0006%02d: tracing_mark_write: B|100|%s", 500+i, i, hmc376OffChainVocabularySpan)
	}
	for i := 0; i < 8; i++ {
		line("loader-%d (100) [001] .... 10.00%d000: tracing_mark_write: B|100|VerifyClass customer.billing.Type%d", 400+i, i+1, i)
		line("loader-%d (100) [001] .... 10.00%d500: tracing_mark_write: E|100", 400+i, i+1)
	}
	line("worker-300 (300) [000] .... 10.012000: sched_wakeup: comm=client pid=100 prio=120 target_cpu=0")
	line("worker-300 (300) [000] .... 10.013000: sched_switch: prev_comm=worker prev_pid=300 prev_prio=120 prev_state=R ==> next_comm=client next_pid=100 next_prio=120")
	for i := 0; i < 3; i++ {
		line("waiter-%d (100) [002] .... 10.0190%02d: tracing_mark_write: E|100", 500+i, i)
	}
	line("client-100 (100) [000] .... 10.020000: sched_switch: prev_comm=client prev_pid=100 prev_prio=120 prev_state=R ==> next_comm=idle next_pid=0 next_prio=120")
	path := filepath.Join(dir, "capture.ftrace")
	if err := os.WriteFile(path, []byte(trace.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	start, end := 10.0, 10.020
	bus := &types.BusContext{RepoRoot: dir, WorkDir: filepath.Join(dir, ".codrax", "blob", "hmc376"),
		AttachedHitrace: trace.String(), AttachedHitraceSource: "generic_ftrace", Mutable: types.NewMutableState("Bounded trace facts"),
		RuntimeArtifactPreflight: types.RuntimeArtifactPreflightProfile{Artifacts: []types.RuntimeArtifactPreflightArtifact{{Kind: "trace", Source: path, Carrier: "attachment"}}},
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RuntimeQuestionProfile: &types.RuntimeQuestionProfile{Scope: types.RuntimeQuestionScopeBoundedFactSet, FactFamilies: []types.RuntimeQuestionFactFamily{types.RuntimeQuestionFactCountOrDuration}},
			RuntimeTargets:         []types.RuntimeTarget{{Kind: types.RuntimeTargetKindThread, PID: 100, Thread: "client-100", Source: "user_explicit"}},
			RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
				TimeStart: &start, TimeEnd: &end, SourceQuote: "10.000000..10.020000"},
		}},
	}
	params := json.RawMessage(`{"source":"attached_trace","view":"root_cause_rank","pid":100,"time_start":10,"time_end":10.020}`)
	result, err := (&tool.TraceQuery{}).Execute(bus, params)
	if err != nil || !result.Success || result.TraceQuerySourceRead.Path() == "" {
		t.Fatalf("the query must publish a physical-source receipt: %v %+v", err, result.Summary)
	}
	bus.Mutable.AppendDispatchToolResult(result)
	ctx := promptcontext.BuildAgentContext(bus, types.AgentFinalizer, types.StageFinalize)
	got := renderAnswerDocTraceDecisionHandoff(ctx)
	if !strings.Contains(got, "Bounded Semantic Span Facts") {
		t.Fatalf("the bounded fact inventory was not rendered:\n%s", got)
	}
	for i := 0; i < 8; i++ {
		if !strings.Contains(got, fmt.Sprintf("customer.billing.Type%d", i)) {
			t.Errorf("family %d lost its seat in the bounded fact inventory", i)
		}
	}
	if strings.Contains(got, hmc376OffChainVocabularySpan) {
		t.Errorf("an off-chain wait took a seat of the bounded fact inventory:\n%s", got)
	}
}
