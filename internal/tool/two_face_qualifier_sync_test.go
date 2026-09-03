package tool

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// two_face_qualifier_sync_test.go — SIDECAR-Q1 acceptance pin ③ (§40.28 ②
// 「头行限定注与 sidecar 限定字段同源一致性: 同一 fixture 两面必须同真值」),
// delivered after the batch-one adversarial review + the eval witness
// (trace_query_frame_semantic_span_optimization, 2026-09-02): the Markdown
// crown face and the .root-causes.json contract are built from ONE ledger
// input, ONE index builder and ONE evidence set per seat, so they cannot
// disagree. The fixture is the eval case's own trace (no frame markers at
// all): the model's root_cause_rank result does not evaluate frames (empty
// status), the frame_root_cause_bundle result evaluates them as absent —
// whichever result supplies the seat's canonical evidence ID, both faces
// must say frame_unproven; without an evaluator, both stay proven; a
// zero-row exploratory bundle probe (the T3-1 class) never taints.

const twoFaceSemanticEvalTrace = `# tracer: nop
#           TASK-PID   CPU#    TIMESTAMP  FUNCTION
        app-100   (  100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200   (  200) [002] .... 5.000400: tracing_mark_write: B|200|VerifyClass com.example.Foo
     worker-200   (  200) [002] .... 5.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200   (  200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
     worker-200   (  200) [002] .... 5.005400: tracing_mark_write: E|200
     worker-200   (  200) [002] .... 5.005600: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100   (  100) [001] .... 5.005800: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100   (  100) [001] .... 5.007000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=R ==> next_comm=idle/1 next_pid=0 next_prio=120
`

type twoFaceResultSpec struct {
	view       string
	start, end float64
	pid        int // 0 ⇒ the fixture target (app-100)
}

func twoFaceResults(t *testing.T, idx *tracequery.Index, specs []twoFaceResultSpec) []types.ToolResult {
	t.Helper()
	at := time.Unix(1751600000, 0).UTC()
	var results []types.ToolResult
	for _, spec := range specs {
		pid := spec.pid
		if pid == 0 {
			pid = 100
		}
		q := tracequery.Query{PID: pid, TimeStart: spec.start, TimeEnd: spec.end, View: spec.view}
		result := tracequery.Run(idx, q)
		results = append(results, types.ToolResult{ToolName: "trace_query", Success: true,
			Observations:           traceQueryTypedObservations(result, "fixture", "p-"+spec.view, "r", "", at),
			TraceEvidenceAuthority: traceQueryEvidenceAuthority(result)})
	}
	return results
}

// twoFaceVerdicts returns (crown headline frame-unproven?, sidecar qualifier
// of the crowned seat) built from ONE ledger input.
func twoFaceVerdicts(t *testing.T, results []types.ToolResult) (bool, string, string) {
	return twoFaceVerdictsGated(t, results, true)
}

// twoFaceVerdictsGated builds both faces from ONE ledger input whose typed
// RequestModel carries the analyzer's frame decision (QUALGATE-1).
func twoFaceVerdictsGated(t *testing.T, results []types.ToolResult, frameQuestion bool) (bool, string, string) {
	t.Helper()
	input := types.ObservationLedgerInput{ToolResults: results,
		RequestModel: &types.RequestModel{RuntimeQuestionProfile: &types.RuntimeQuestionProfile{
			Scope: types.RuntimeQuestionScopeCausalDiagnosis, FrameCausalityRequested: frameQuestion}}}
	index := tracefinding.BuildSeatFrameCausalityAuthority(input)
	ledger := types.CompileObservationLedger(input)
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("fixture must compile one projection, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	crownUnproven := runtimeTraceProjectionLeadFrameCausalityUnproven(projection, model, runtimeTraceProjectionSeatAuthorityIndex(index))
	lead, _ := runtimeTraceProjLeadSelect(projection, model)
	if lead == nil {
		t.Fatalf("fixture must elect a crown seat")
	}
	contract, err := tracefinding.CompileCandidateContract(ledger, set, index)
	if err != nil {
		t.Fatal(err)
	}
	// The tree lead may be keyed by the seat's semantic-channel record while
	// the contract candidate is keyed by its rank-row record (one physical
	// seat, two producer records) — match the seat by its thread subject
	// (unique per seat in this two-thread fixture).
	for _, candidate := range contract.Candidates {
		if candidate.Decision.SubjectName == lead.Subject {
			return crownUnproven, candidate.Decision.CausalQualifier, lead.Subject
		}
	}
	t.Fatalf("the crowned seat %q (%s) must be a sidecar candidate: %+v", lead.Subject, lead.EvidenceID, contract.Candidates)
	return false, "", ""
}

func TestTwoFacesShareOneSeatLevelFrameQualifier(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "frame_semantic_span.ftrace")
	if err := os.WriteFile(path, []byte(twoFaceSemanticEvalTrace), 0o644); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	rank := twoFaceResultSpec{view: "root_cause_rank", start: 5.0, end: 5.007}
	bundle := twoFaceResultSpec{view: "frame_root_cause_bundle", start: 5.0, end: 5.007}
	// A frame bundle for a thread that has no events at all: zero typed rows
	// (the T3-1 exploratory probe class) — the fixture proves it below.
	emptyBundle := twoFaceResultSpec{view: "frame_root_cause_bundle", start: 5.0, end: 5.007, pid: 999}
	if probe := twoFaceResults(t, idx, []twoFaceResultSpec{emptyBundle}); probe[0].TraceEvidenceAuthority == nil ||
		probe[0].TraceEvidenceAuthority.TypedCausalRowCount != 0 {
		t.Fatalf("fixture self-check: the exploratory probe must carry zero typed rows: %+v", probe[0].TraceEvidenceAuthority)
	}

	// The two result orders swap which result supplies the seat's canonical
	// evidence ID — the verdict must not depend on it.
	for name, specs := range map[string][]twoFaceResultSpec{
		"rank_then_bundle": {rank, bundle},
		"bundle_then_rank": {bundle, rank},
	} {
		crown, sidecar, subject := twoFaceVerdicts(t, twoFaceResults(t, idx, specs))
		if !crown || sidecar != types.TraceCausalQualifierFrameUnproven {
			t.Fatalf("%s: frame evidence is absent in this run — both faces must say frame_unproven for %q (crown=%v sidecar=%q)", name, subject, crown, sidecar)
		}
	}
	// No frame evaluator in the run ⇒ no qualifier claim on either face
	// (the historical non-frame-question shape).
	crown, sidecar, subject := twoFaceVerdicts(t, twoFaceResults(t, idx, []twoFaceResultSpec{rank}))
	if crown || sidecar != types.TraceCausalQualifierProven {
		t.Fatalf("without a frame evaluator both faces stay proven for %q (crown=%v sidecar=%q)", subject, crown, sidecar)
	}
	// A zero-row exploratory bundle probe never taints (T3-1 §7.3).
	crown, sidecar, subject = twoFaceVerdicts(t, twoFaceResults(t, idx, []twoFaceResultSpec{rank, emptyBundle}))
	if crown || sidecar != types.TraceCausalQualifierProven {
		t.Fatalf("a zero-row probe must not taint the seat %q (crown=%v sidecar=%q)", subject, crown, sidecar)
	}
	// QUALGATE-1 (§40.30 V-QUAL-1 plan A): not a frame question ⇒ even with
	// the absent-frame evaluator present, neither face makes a frame claim —
	// no headline qualifier, sidecar not_applicable (never proven).
	crown, sidecar, subject = twoFaceVerdictsGated(t, twoFaceResults(t, idx, []twoFaceResultSpec{rank, bundle}), false)
	if crown || sidecar != types.TraceCausalQualifierNotApplicable {
		t.Fatalf("gate closed: both faces must make no frame claim for %q (crown=%v sidecar=%q)", subject, crown, sidecar)
	}
}
