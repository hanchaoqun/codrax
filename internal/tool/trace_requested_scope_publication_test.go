package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/types"
)

func requestedScopePublicationLedger() types.ObservationLedger {
	start, end := 2.0, 2.02
	record := func(id, predicate, subject, object, value string, notes ...string) types.ObservationRecord {
		return types.ObservationRecord{
			ID: id, Origin: types.AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query",
			GroundingPolicy: types.ClaimGroundingHard,
			SourceRef:       types.ObservationSourceRef{Kind: types.ObservationSourceRuntimeArtifact, Path: "/trace/scope.systrace", PayloadRef: "trace-query-result-scope.json"},
			ClaimKey:        id, Predicate: predicate, Subject: subject, Object: object, Value: value, Unit: "ms", RichNotes: notes,
		}
	}
	return types.ObservationLedger{
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{RequestedScope: types.RuntimeArtifactScopeExplicitWindow,
			TimeStart: &start, TimeEnd: &end, SourceQuote: "2.000s to 2.020s"},
		AnchorUserEntities: []types.AnchorUserEntity{{Value: "100", TypedLane: true}},
		Records: []types.ObservationRecord{
			record("frame", "frame_target_resolution", "app-100", "explicit_query_target", "", "window_source=query_window", "window=2.000000..2.021000"),
			record("path", "wakeup_chain", "app-100", "worker-400 -> app-100", "", "branch=1", "selected_window=2.000000..2.021000"),
			record("rank", "root_cause_primary", "app-100", "runnable_wait", "0.020", "rank=1", "tier=primary", "chain_relevance=on_chain", "effective_impact_ms=0.020", "selected_window=2.000000..2.021000"),
			record("state", "target_window_states", "app-100", "state_partition", "20.020", "selected_window=2.000000..2.021000", "running=0.000", "runnable=0.020", "sleep=20.000", "d_state=0.000", "io_wait=0.000", "total=20.020"),
		},
	}
}

func TestRequestedTraceScopePublicationKeepsExplorationAndDisclosesItsScope(t *testing.T) {
	ledger := requestedScopePublicationLedger()
	projection := types.CompileTraceCausalProjection(ledger)
	if projection.WindowEndTs != 2.021 || projection.TargetStateAccount == nil || projection.TargetStateAccount.RunnableMS != .020 {
		t.Fatalf("do not relabel or clip the measured account: %+v", projection)
	}
	for _, lang := range []string{"zh", "en"} {
		t.Run(lang, func(t *testing.T) {
			blocks := runtimeTraceCausalProjectionCluster(projection, lang, runtimeTraceProjUserFocus{})
			found := 0
			for _, block := range blocks {
				if block.ID != runtimeTraceCausalProjectionBlockIDBase && block.ID != runtimeTraceCausalProjectionBlockIDBase+runtimeTraceCausalProjectionOccupancySuffix {
					continue
				}
				found++
				word := "补充查询"
				if lang == "en" {
					word = "Supplementary query"
				}
				if !strings.Contains(block.Title+block.Text, word) || !strings.Contains(block.Text, "2.020000") || !strings.Contains(block.Text, "2.021000") {
					t.Errorf("%s does not distinguish requested and measured scopes: %s", block.ID, block.Text)
				}
			}
			if found != 2 {
				t.Fatalf("keep both time axes and the projection: blocks=%+v", blocks)
			}
		})
	}
}

func TestRequestedTraceScopeSidecarCannotCallExplorationTheTargetWindow(t *testing.T) {
	ledger := requestedScopePublicationLedger()
	set := types.CompileTraceCausalProjectionSet(ledger)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil || len(contract.Candidates) == 0 {
		t.Fatalf("keep the evidence roster: %v %+v", err, contract)
	}
	contract.RootCauseReportEnabled = true
	description := "模型选择保留这段调度等待用于后续排查。"
	report, err := tracefinding.BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
		RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: contract.Candidates[0].Decision.CandidateID, Description: description}}}, contract)
	if err != nil || len(report.RootCauses) != 1 {
		t.Fatalf("do not drop model selection: %v %+v", err, report)
	}
	item := report.RootCauses[0]
	if item.WindowScope == nil || !item.WindowScope.IsSupportingExploration() || item.WindowScope.QueryWindowEndTs != 2.021 || item.WindowScope.RequestedWindowEndTs != 2.02 {
		t.Fatalf("programmatic scope is absent or differs from the selected evidence: %+v", item.WindowScope)
	}
	if *item.ImpactSeconds != .00002 || item.Description != description {
		t.Fatalf("do not clip the fact or rewrite the model: %+v", item)
	}
	text := strings.Join(item.Evidence, " ")
	if strings.Contains(text, "在目标窗口内") || !strings.Contains(text, "补充查询") || !strings.Contains(text, "2.020000") {
		t.Fatalf("sidecar silently promoted exploration into requested scope: %s", text)
	}
}

func TestRequestedTraceScopeSurvivesFrozenContractEmitPatchAndClone(t *testing.T) {
	ledger := requestedScopePublicationLedger()
	set := types.CompileTraceCausalProjectionSet(ledger)
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil || len(contract.Candidates) != 1 {
		t.Fatalf("fixture: %v %+v", err, contract)
	}
	contract.RootCauseReportEnabled = true
	mutable := types.NewMutableState("scope preservation")
	mutable.SetTraceFindingContract(contract)
	// Mutating either the producer's contract or a returned snapshot must not
	// mutate a later selector's facts; path and scope have separate pointers.
	contract.Candidates[0].Decision.EvidenceFacts.WindowScope.QueryWindowEndTs = 99
	read := mutable.TraceFindingContract()
	if read.Candidates[0].Decision.EvidenceFacts.WindowScope.QueryWindowEndTs != 2.021 {
		t.Fatal("stored contract aliases its producer")
	}
	read.Candidates[0].Decision.EvidenceFacts.WindowScope.RequestedWindowEndTs = 99
	if mutable.TraceFindingContract().Candidates[0].Decision.EvidenceFacts.WindowScope.RequestedWindowEndTs != 2.02 {
		t.Fatal("stored contract aliases its reader")
	}
	ctx := &types.BusContext{Mutable: mutable}
	modelText := "原有业务判断\n\n```mermaid\nflowchart LR\nA-->B\n```"
	description := "模型明确选择的说明，不由系统替换。"
	raw, _ := json.Marshal(map[string]any{
		"blocks":            []map[string]any{{"id": "summary", "kind": "summary", "text": modelText}},
		"trace_root_causes": map[string]any{"schema_version": 2, "root_causes": []map[string]any{{"candidate_id": contract.Candidates[0].Decision.CandidateID, "description": description}}},
	})
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit: %v %+v", err, result)
	}
	report := mutable.TraceRootCauseReport()
	if report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].WindowScope == nil {
		t.Fatal("bound scope lost on emit")
	}
	// Returned report scope is a defensive copy, too.
	returned := mutable.TraceRootCauseReport()
	returned.RootCauses[0].WindowScope.QueryWindowEndTs = 999
	if !reflect.DeepEqual(report, mutable.TraceRootCauseReport()) {
		t.Fatal("report scope aliases mutable state")
	}
	result, err = (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{"unchanged_block_ids":["summary"]}`))
	if err != nil || !result.Success {
		t.Fatalf("patch: %v %+v", err, result)
	}
	if !reflect.DeepEqual(report, mutable.TraceRootCauseReport()) || mutable.AnswerDocumentV2().Blocks[0].Text != modelText || report.RootCauses[0].Description != description {
		t.Fatal("scope handling changed model content, selection, or bound facts")
	}
}
