package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRootCauseValueCaliberSurvivesObservationProjectionEmitAndPatch(t *testing.T) {
	result := tracequery.Result{View: "root_cause_rank", RootCauseRank: &tracequery.RootCauseRankResult{
		Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.1}, Items: []tracequery.RootCauseRankItem{
			{Rank: 1, Tier: "primary", Type: "running", Thread: tracequery.ThreadRef{Comm: "worker", PID: 9},
				ImpactMs: 12, CumulativeImpactMs: 12, RunningMs: 12, EffectiveImpactMs: 3, DominantState: "running",
				SupplyFoldDeficitMs: 3, SupplyFoldIdealMs: 9, SupplyFoldBasis: &tracequery.SupplyFoldBasis{KnownMs: 10, UnknownMs: 2},
				StartTs: 10.01, EndTs: 10.022, LineStart: 10, LineEnd: 20, Source: "window_stats", ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
			{Rank: 2, Tier: "secondary", Type: "d_state_or_io_wait", Thread: tracequery.ThreadRef{Comm: "render", PID: 10},
				ImpactMs: 2, CumulativeImpactMs: 2, DStateMs: 2, EffectiveImpactMs: 2, DStateAllNonIOProven: true,
				StartTs: 10.03, EndTs: 10.032, LineStart: 21, LineEnd: 25, Source: "window_stats", ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
		}}}
	// Exercise production-sized source handles, not only short E1 refs. The
	// supply description and both provenance axes must survive sidecar binding.
	source := "/workspace/.codrax/blob/20260902-005125-000-78634/attached_trace_with_a_long_capture_name.systrace"
	result.SourcePath = source
	records := traceQueryTypedObservations(result, source, "trace-query-result-28bd29e5.json", "raw", "", time.Unix(1, 0))
	ledger := types.ObservationLedger{Records: records}
	set := types.CompileTraceCausalProjectionSet(ledger)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{Applicable: true})
	if err != nil || len(contract.Candidates) != 2 {
		t.Fatalf("candidate compile: %v %+v", err, contract)
	}
	contract.RootCauseReportEnabled = true
	mutable := types.NewMutableState("trace root causes")
	mutable.SetTraceFindingContract(contract)
	ctx := &types.BusContext{Mutable: mutable}
	selections := []map[string]any{}
	for _, candidate := range contract.Candidates {
		selections = append(selections, map[string]any{"candidate_id": candidate.Decision.CandidateID})
	}
	raw, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "model business explanation"}},
		"trace_root_causes": map[string]any{"schema_version": 2, "root_causes": selections}})
	emitted, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !emitted.Success {
		t.Fatalf("emit: %v %+v", err, emitted)
	}
	report := mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 2 || report.RootCauses[0].Category != types.TraceRootCauseComputeSupplyShortage ||
		report.RootCauses[1].Category != types.TraceRootCauseSleepBlocking || *report.RootCauses[0].ImpactSeconds != .003 ||
		!strings.Contains(strings.Join(report.RootCauses[1].Evidence, " "), "非 I/O") {
		t.Fatalf("published wrong caliber: %+v", report)
	}
	// SIDECAR-EVID-1 (§40.32): the public evidence names the seat's typed facts
	// and NEVER the internal provenance handle — the customer cannot open it.
	for _, item := range report.RootCauses {
		joined := strings.Join(item.Evidence, " ")
		if strings.Contains(joined, source) || strings.Contains(joined, "trace_query:") {
			t.Fatalf("sidecar leaked an internal provenance reference: %+v", item)
		}
		if item.ThreadName != "" && !strings.Contains(joined, item.ThreadName) {
			t.Fatalf("sidecar evidence must name the seat's subject: %+v", item)
		}
	}
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{"unchanged_block_ids":["summary"]}`))
	if err != nil || !patched.Success {
		t.Fatalf("patch: %v %+v", err, patched)
	}
	if !reflect.DeepEqual(mutable.TraceRootCauseReport(), report) || mutable.AnswerDocumentV2().Blocks[0].Text != "model business explanation" {
		t.Fatal("patch lost basis or replaced model prose")
	}
}

// V1-4 (§40.26 acceptance pin): two trace files, a same-named worker thread in
// each → two published causes carrying different artifact_label values, both
// surviving the emit → normalize → patch-retention chain.
func TestRootCauseSidecarKeepsArtifactLabelAcrossTwoTraceFiles(t *testing.T) {
	result := func(source, payloadRef string) []types.ObservationRecord {
		r := tracequery.Result{View: "root_cause_rank", SourcePath: source, RootCauseRank: &tracequery.RootCauseRankResult{
			Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.1}, Items: []tracequery.RootCauseRankItem{
				{Rank: 1, Tier: "primary", Type: "runnable_wait", Thread: tracequery.ThreadRef{Comm: "worker", PID: 9},
					ImpactMs: 4, CumulativeImpactMs: 4, EffectiveImpactMs: 4, DominantState: "runnable",
					StartTs: 10.01, EndTs: 10.014, LineStart: 10, LineEnd: 20, Source: "window_stats", ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
			}}}
		return traceQueryTypedObservations(r, source, payloadRef, "raw", "", time.Unix(1, 0))
	}
	records := append(result("/workspace/traces/a.systrace", "trace-query-result-aaaaaa01.json"),
		result("/workspace/traces/b.systrace", "trace-query-result-bbbbbb02.json")...)
	ledger := types.ObservationLedger{Records: records}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 2 {
		t.Fatalf("two trace files must partition into two projections: %+v", set)
	}
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{Applicable: true})
	if err != nil || len(contract.Candidates) != 2 || !contract.MultiArtifact() {
		t.Fatalf("candidate compile: %v %+v", err, contract)
	}
	contract.RootCauseReportEnabled = true
	mutable := types.NewMutableState("trace root causes")
	mutable.SetTraceFindingContract(contract)
	ctx := &types.BusContext{Mutable: mutable}
	selections := []map[string]any{}
	for _, candidate := range contract.Candidates {
		selections = append(selections, map[string]any{"candidate_id": candidate.Decision.CandidateID})
	}
	raw, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "model business explanation"}},
		"trace_root_causes": map[string]any{"schema_version": 2, "root_causes": selections}})
	emitted, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !emitted.Success {
		t.Fatalf("emit: %v %+v", err, emitted)
	}
	report := mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 2 {
		t.Fatalf("both same-named causes must be published: %+v", report)
	}
	labels := map[string]bool{}
	for _, item := range report.RootCauses {
		if !strings.HasPrefix(item.ThreadName, "worker") || item.ArtifactLabel == "" || item.CandidateID != "" {
			t.Fatalf("published cause lost its partition key or leaked its receipt: %+v", item)
		}
		labels[item.ArtifactLabel] = true
	}
	if !labels["a.systrace"] || !labels["b.systrace"] {
		t.Fatalf("artifact labels must be the partitioner's per-trace labels: %v", labels)
	}
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{"unchanged_block_ids":["summary"]}`))
	if err != nil || !patched.Success {
		t.Fatalf("patch: %v %+v", err, patched)
	}
	if !reflect.DeepEqual(mutable.TraceRootCauseReport(), report) {
		t.Fatal("patch retention lost the labelled report")
	}
}
