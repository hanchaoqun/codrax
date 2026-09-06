package tool

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

// B1562: actual scheduler/frequency events, rather than hand-minted candidate
// rows, feed the engine -> observation -> projection -> full emit -> patch.
// The original R5d mixed-state witness supplies the two independent components.
func TestRootCausePriorityMechanismSurvivesRealQueryEmitAndPatch(t *testing.T) {
	path := filepath.Join(t.TempDir(), "mixed-dependency.systrace")
	trace := `
      <idle>-0 (-----) [007] .... 4.900000: cpu_frequency: state=800000 cpu_id=7
      <idle>-0 (-----) [001] .... 4.900000: cpu_frequency: state=2000000 cpu_id=1
        app-100 (100) [001] .... 4.990000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
        dep-200 (100) [002] .... 5.000000: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=R ==> next_comm=idle/2 next_pid=0 next_prio=120
        dep-200 (100) [007] .... 5.010000: sched_switch: prev_comm=idle/7 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=dep next_pid=200 next_prio=20
        dep-200 (100) [007] .... 5.019000: sched_wakeup: comm=app pid=100 prio=52 target_cpu=001
        dep-200 (100) [007] .... 5.019500: sched_switch: prev_comm=dep prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/7 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.020000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`
	if err := os.WriteFile(path, []byte(trace), 0600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	q := tracequery.Query{View: "root_cause_rank", PID: 100, TimeStart: 5, TimeEnd: 5.020, MaxDepth: 4, MinDurationMs: .05, TraceFlavorHint: tracequery.TraceFlavorHarmonyHitrace}
	result := tracequery.Run(idx, q)
	records := traceQueryTypedObservations(result, path, "trace-query-result-abcd0123.json", "raw", "", time.Unix(1, 0))
	ledger := types.ObservationLedger{Records: records}
	set := types.CompileTraceCausalProjectionSet(ledger)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	var selected *types.TraceFindingCandidateV1
	for i := range contract.Candidates {
		if contract.Candidates[i].Decision.Token.Token == "priority_inversion_candidate" {
			selected = &contract.Candidates[i]
			break
		}
	}
	if selected == nil {
		raw, _ := json.Marshal(contract.Candidates)
		t.Fatalf("actual mixed dependency failed to produce the expected ranked candidate: %s", raw)
	}
	contract.RootCauseReportEnabled = true
	mutable := types.NewMutableState("trace root causes")
	mutable.SetTraceFindingContract(contract)
	ctx := &types.BusContext{Mutable: mutable}
	description := "依赖线程先等待 CPU 再运行，建议核实优先级和算力供给。"
	selection := map[string]any{"schema_version": 2, "root_causes": []map[string]any{{"candidate_id": selected.Decision.CandidateID,
		"description": description, "mechanism_qualifier": "invented_confirmation", "impact_breakdown": map[string]any{"runnable_seconds": 99}}}}
	raw, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "模型自己的业务解释"}}, "trace_root_causes": selection})
	emitted, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !emitted.Success {
		t.Fatalf("emit failed: %v %+v", err, emitted)
	}
	report := mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 1 {
		t.Fatalf("model selection was lost: %+v", report)
	}
	cause := report.RootCauses[0]
	if cause.MechanismQualifier != types.TraceMechanismLowerPriorityDependencyCandidate || cause.ImpactBreakdown == nil ||
		cause.ImpactBreakdown.RunnableSeconds <= 0 || cause.ImpactBreakdown.RunningDeficitSeconds <= 0 ||
		cause.ImpactBreakdown.CapabilitySource != "default_table" || cause.CausalQualifier != types.TraceCausalQualifierNotApplicable ||
		math.Abs(cause.ImpactBreakdown.RunnableSeconds+cause.ImpactBreakdown.RunningDeficitSeconds-*cause.ImpactSeconds) > 1.001e-6 ||
		cause.Description != description || !strings.Contains(cause.Summary, "反转候选") || strings.Contains(strings.Join(cause.Evidence, " "), "修向=锁") {
		t.Fatalf("candidate ceiling/composite amount drifted on the public face: %+v", cause)
	}
	patch, _ := json.Marshal(map[string]any{"unchanged_block_ids": []string{"summary"}, "replace_trace_root_causes": selection})
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, patch)
	if err != nil || !patched.Success || !reflect.DeepEqual(report, mutable.TraceRootCauseReport()) {
		t.Fatalf("patch lost the source-bound facts: %v %+v", err, patched)
	}
	if mutable.AnswerDocumentV2().Blocks[0].Text != "模型自己的业务解释" {
		t.Fatal("binding must preserve the model's answer")
	}
}
