package tool

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// optional_carrier_outcome_test.go — V2-3 (§40.19): "not rejecting the main
// transaction" must never become "not telling". Every optional carrier the
// tool ignores reaches the SUCCESS result as a typed OptionalCarrierOutcome
// plus one Summary line carrying the precise reason and the repair route.

func rootCauseOutcomeFixture(t *testing.T) (*types.MutableState, *types.BusContext) {
	t.Helper()
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	return mutable, &types.BusContext{Mutable: mutable}
}

func TestEmitAnswerDocumentInvalidRootCauseSelectorIsDisclosed(t *testing.T) {
	mutable, ctx := rootCauseOutcomeFixture(t)
	raw, _ := json.Marshal(map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "useful model answer survives"}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{{"candidate_id": "invented-candidate"}},
		},
	})
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("optional sidecar must not own answer eligibility: result=%+v err=%v", result, err)
	}
	if report := mutable.TraceRootCauseReport(); report != nil {
		t.Fatalf("invalid sidecar must not be persisted: %#v", report)
	}
	if len(result.OptionalCarrierOutcomes) != 1 {
		t.Fatalf("exactly one typed outcome expected: %+v", result.OptionalCarrierOutcomes)
	}
	outcome := result.OptionalCarrierOutcomes[0]
	if outcome.Carrier != "trace_root_causes" || outcome.Status != types.OptionalCarrierStatusIgnored || !strings.Contains(outcome.Reason, `"invented-candidate"`) {
		t.Fatalf("outcome must name the carrier, the ignored status and the offending id: %+v", outcome)
	}
	lines := strings.Split(result.Summary, "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[0], "emit_answer_document accepted:") {
		t.Fatalf("line one must stay the accepted counts line: %q", result.Summary)
	}
	last := lines[len(lines)-1]
	for _, want := range []string{"[optional_carrier_ignored: carrier=trace_root_causes", "invented-candidate", "replace_trace_root_causes", "candidate-sched"} {
		if !strings.Contains(last, want) {
			t.Fatalf("the disclosure line must carry %q for the model's next patch: %q", want, result.Summary)
		}
	}
	if strings.Contains(last, "citations=") {
		t.Fatalf("the disclosure line must never spell the accepted-count token: %q", last)
	}
}

func TestEmitAnswerDocumentPatchBadReplacementRetainsPreviousAndSaysSo(t *testing.T) {
	mutable, ctx := rootCauseOutcomeFixture(t)
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, json.RawMessage(`{
		"blocks":[{"id":"summary","kind":"summary","text":"model conclusion"}],
		"trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}
	}`), time.Now())
	if err != nil || !result.Success || len(result.OptionalCarrierOutcomes) != 0 {
		t.Fatalf("a valid selector carries no outcome: %+v %v", result, err)
	}
	accepted := mutable.TraceRootCauseReport()
	if accepted == nil || len(accepted.RootCauses) != 1 {
		t.Fatalf("fixture: accepted selection missing: %+v", accepted)
	}
	patched, err := (&EmitAnswerDocumentPatch{}).Execute(ctx, json.RawMessage(`{
		"unchanged_block_ids":["summary"],
		"replace_trace_root_causes":{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"},{"candidate_id":"candidate-sched"}]}
	}`))
	if err != nil || !patched.Success {
		t.Fatalf("optional sidecar cannot own patch eligibility: %+v %v", patched, err)
	}
	if !reflect.DeepEqual(mutable.TraceRootCauseReport(), accepted) {
		t.Fatalf("a bad replacement must retain the previously accepted report: %+v", mutable.TraceRootCauseReport())
	}
	if len(patched.OptionalCarrierOutcomes) != 1 {
		t.Fatalf("exactly one typed outcome expected: %+v", patched.OptionalCarrierOutcomes)
	}
	outcome := patched.OptionalCarrierOutcomes[0]
	if outcome.Carrier != "replace_trace_root_causes" || outcome.Status != types.OptionalCarrierStatusRetainedPrevious || !strings.Contains(outcome.Reason, "duplicates an earlier selection") {
		t.Fatalf("outcome must say the previous selection was retained and why: %+v", outcome)
	}
	if !strings.Contains(patched.Summary, "[optional_carrier_retained_previous: carrier=replace_trace_root_causes") {
		t.Fatalf("disclosure line missing on the patch summary: %q", patched.Summary)
	}
}

func TestEmitAnswerDocumentDroppedDescriptionIsATypedPartDrop(t *testing.T) {
	mutable, ctx := rootCauseOutcomeFixture(t)
	raw, _ := json.Marshal(map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "answer"}},
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{{"candidate_id": "candidate-sched", "description": "见 .codrax/blob/x/attached_trace.txt:2892"}},
		},
	})
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: result=%+v err=%v", result, err)
	}
	if report := mutable.TraceRootCauseReport(); report == nil || len(report.RootCauses) != 1 || report.RootCauses[0].Description != "" {
		t.Fatalf("the typed selection is kept and only the description dropped: %#v", report)
	}
	if len(result.OptionalCarrierOutcomes) != 1 {
		t.Fatalf("exactly one typed outcome expected: %+v", result.OptionalCarrierOutcomes)
	}
	outcome := result.OptionalCarrierOutcomes[0]
	if outcome.Carrier != "trace_root_causes" || outcome.Status != types.OptionalCarrierStatusPartDropped ||
		!strings.Contains(outcome.Reason, "description for root_causes[0] dropped") || !strings.Contains(outcome.Reason, "internal references") {
		t.Fatalf("part-drop outcome must carry the binder's precise reason: %+v", outcome)
	}
	if !strings.Contains(result.Summary, "[optional_carrier_part_dropped: carrier=trace_root_causes") {
		t.Fatalf("disclosure line missing: %q", result.Summary)
	}
}

// A structurally rejected document commit also learns that its selector was
// bad — the outcome minted at resolve time rides the rejection result through
// the call's ledger finalize, so the re-emit can fix both at once — while a
// VALID selector on the same rejected commit is staged (§40.31.1 ★16) and
// never published; a rejected submission is marked on the state for the
// customer reason_code (§40.44 residual a). EVOLUTION RECORD (§40.44 E2):
// formerly pinned the commit tail attaching rows itself; the attach now
// lives in the ONE choke point (optionalCarrierLedger.finalize) and the
// commit tail owns state only.
func TestCommitTraceRootCauseSelectionRejectedCommitDisclosesAndStages(t *testing.T) {
	mutable, ctx := rootCauseOutcomeFixture(t)
	rejected := func() types.ToolResult {
		return types.ToolResult{ToolName: "emit_answer_document", Summary: "emit_answer_document rejected: blocks must be non-empty", Success: false}
	}
	carriers := newOptionalCarrierLedger("emit_answer_document")
	invalid := resolveTraceRootCauseSelectionForEmit(ctx, carriers,
		json.RawMessage(`{"schema_version":2,"root_causes":[{"candidate_id":"invented-candidate"}]}`), false)
	res := rejected()
	commitTraceRootCauseSelection(ctx, res, nil, invalid)
	res = carriers.finalize(res)
	if len(res.OptionalCarrierOutcomes) != 1 || res.OptionalCarrierOutcomes[0].Status != types.OptionalCarrierStatusIgnored ||
		!strings.Contains(res.Summary, "[optional_carrier_ignored: carrier=trace_root_causes") {
		t.Fatalf("the rejected result must still carry the selector outcome: %+v %q", res.OptionalCarrierOutcomes, res.Summary)
	}
	if mutable.TraceRootCauseReport() != nil || mutable.PendingTraceRootCauseReport() != nil {
		t.Fatal("an invalid selector must be neither published nor staged")
	}
	if !mutable.TraceRootCauseSelectorRejected() {
		t.Fatal("a rejected submission must be marked for the customer reason_code")
	}
	carriers = newOptionalCarrierLedger("emit_answer_document")
	valid := resolveTraceRootCauseSelectionForEmit(ctx, carriers,
		json.RawMessage(`{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`), false)
	res = rejected()
	commitTraceRootCauseSelection(ctx, res, nil, valid)
	res = carriers.finalize(res)
	if len(res.OptionalCarrierOutcomes) != 0 {
		t.Fatalf("a valid selector carries no outcome: %+v", res.OptionalCarrierOutcomes)
	}
	if mutable.TraceRootCauseReport() != nil {
		t.Fatal("a rejected commit must not publish the report")
	}
	if staged := mutable.PendingTraceRootCauseReport(); staged == nil || len(staged.RootCauses) != 1 {
		t.Fatalf("a validly bound selector on a rejected commit must be staged (§40.31.1 ★16): %+v", staged)
	}
	if mutable.TraceRootCauseSelectorRejected() {
		t.Fatal("a later valid submission clears the rejected mark")
	}
}

// §40.44 E2(a): the patch executor's pre-persist rejects (relation-repair
// scope, hard fix hints) return before any persist site; the selector
// resolved just before them must still be disclosed on that rejection AND a
// validly bound selector must be staged, because the rejection tells the
// model to "submit only new corrections" — a retry that never resubmits the
// selector. Fixture reaches the hard-hint reject (staged_for_retry) exactly
// like TestEmitAnswerDocumentPatch_RejectedPatchStagesCandidateWithoutAdvancingAcceptedOrFullRejectedBase.
func TestEmitAnswerDocumentPatchPrePersistRejectDisclosesAndStagesTheSelector(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selector string
		wantRow  bool
		wantPend bool
	}{
		{name: "invalid selector is disclosed on the reject", selector: `{"schema_version":2,"root_causes":[{"candidate_id":"invented-candidate"}]}`, wantRow: true},
		{name: "valid selector is staged on the reject", selector: `{"schema_version":2,"root_causes":[{"candidate_id":"candidate-sched"}]}`, wantPend: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			mut := types.NewMutableState("transactional rejected patch")
			mut.SetTraceFindingContract(testSelectableTraceRootCauseContract())
			mut.SetLastRejectedAnswerDocumentV2(&types.AnswerDocumentV2{
				DocumentModel: "v2",
				Citations:     []types.Citation{{File: "x.go", Line: 10}},
				Blocks: []types.AnswerBlock{
					{ID: "s1", Kind: types.BlockSummary, SurfaceRole: types.SurfacePrincipal, Text: "stable rejected summary", FacetIDs: []string{string(types.FacetCurrentCodePath)}},
					{ID: "path", Kind: types.BlockOrderedList, SurfaceRole: types.SurfacePrincipal, FacetIDs: []string{string(types.FacetCurrentCodePath)},
						ClaimUses: []types.RenderedClaimUse{{ClaimForm: types.ClaimCallEdge}},
						Items:     []types.AnswerBlockItem{{ID: "hop", Label: "stable hop", CitationRef: 0}}},
				},
			})
			bus := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: types.IntentTrace, PredicateAxis: types.AxisCall, AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)}}}}
			res, err := (&EmitAnswerDocumentPatch{}).Execute(bus, json.RawMessage(`{
				"unchanged_block_ids":["s1"],
				"replace_blocks":[{"id":"path","kind":"ordered_list","surface_role":"principal","facet_ids":["current_code_path"],
					"claim_uses":[{"claim_form":"call_edge"}],"items":[{"id":"hop","label":"changed hidden hop","citation_ref":0}]}],
				"replace_trace_root_causes":`+tc.selector+`
			}`))
			if err != nil || res.Success || res.Repair == nil || res.Repair.Metadata[types.ToolRepairMetaAnswerDocumentPatchOutcome] != types.AnswerDocumentPatchOutcomeStagedForRetry {
				t.Fatalf("fixture must reach the pre-persist hard-hint reject: err=%v res=%+v", err, res)
			}
			if tc.wantRow {
				if len(res.OptionalCarrierOutcomes) != 1 || res.OptionalCarrierOutcomes[0].Carrier != "replace_trace_root_causes" ||
					!strings.Contains(res.Summary, "[optional_carrier_ignored: carrier=replace_trace_root_causes") ||
					!strings.Contains(res.Summary, "invented-candidate") {
					t.Fatalf("the pre-persist reject must disclose the ignored selector: %+v %q", res.OptionalCarrierOutcomes, res.Summary)
				}
				if !strings.HasPrefix(res.Summary, "Patch transaction state:") {
					t.Fatalf("the transaction-state prefix must stay line one: %q", res.Summary)
				}
				if !mut.TraceRootCauseSelectorRejected() {
					t.Fatal("rejected submission must be marked for the customer reason_code")
				}
			} else if len(res.OptionalCarrierOutcomes) != 0 {
				t.Fatalf("a valid selector carries no outcome: %+v", res.OptionalCarrierOutcomes)
			}
			if got := mut.PendingTraceRootCauseReport() != nil; got != tc.wantPend {
				t.Fatalf("staged selector present=%t want %t", got, tc.wantPend)
			}
			if mut.TraceRootCauseReport() != nil {
				t.Fatal("a rejected patch must not publish the report")
			}
		})
	}
}

// §40.44 E0: emit_investigation_complete's pre-complete DOWNGRADE returns are
// Success=true results the model acts on; an ignored waiver must be disclosed
// on that very turn (typed row + its own Summary line, since the downgrade
// prose does not word it), not first on the terminal accepted result.
func TestEmitInvestigationCompleteDowngradeTurnDisclosesTheIgnoredWaiver(t *testing.T) {
	bus := perMemberTableBus(true)
	res, err := (&EmitInvestigationComplete{}).Execute(bus, json.RawMessage(`{
		"reason":"pipeline stages traced end to end",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_log","rationale":"   "},
		"principal_span_waiver":{"reason":"bogus_span_reason","rationale":"x"}
	}`))
	if err != nil || !res.Success || bus.Mutable.IsInvestigationComplete() || res.Repair == nil || res.Repair.Code != "principal_member_set_handoff" {
		t.Fatalf("fixture must be the per-member-table downgrade turn: err=%v res=%+v", err, res)
	}
	if len(res.OptionalCarrierOutcomes) != 1 || res.OptionalCarrierOutcomes[0].Carrier != "evidence_floor_waiver" ||
		res.OptionalCarrierOutcomes[0].Status != types.OptionalCarrierStatusIgnored {
		// The principal_span_waiver is only read after this downgrade point
		// (pre-existing order); the evidence_floor_waiver was ignored BEFORE
		// it and must be on this result.
		t.Fatalf("the downgrade turn must carry the ignored waiver as a typed row: %+v", res.OptionalCarrierOutcomes)
	}
	if !strings.Contains(res.Summary, "[optional_carrier_ignored: carrier=evidence_floor_waiver") ||
		!strings.Contains(res.Summary, "rationale is missing") || !strings.Contains(res.Summary, "member_set") {
		t.Fatalf("the downgrade summary must keep its own text and add the disclosure line: %q", res.Summary)
	}
	if bus.Mutable.EvidenceFloorWaiver() != nil {
		t.Fatal("ignored waiver must not be stored")
	}
}

// The registry's one runtime-detectable violation: a mint after the result
// left the choke point cannot reach it — a test failure here, a WARN in
// production. A nil ledger fails loud too.
func TestOptionalCarrierLedgerMintAfterFinalizeFailsLoud(t *testing.T) {
	carriers := newOptionalCarrierLedger("emit_answer_document")
	_ = carriers.finalize(types.ToolResult{Success: true})
	defer func() {
		if recover() == nil {
			t.Fatal("a mint after finalize must fail under go test")
		}
	}()
	carriers.ignored("trace_root_causes", "late")
}

func TestOptionalCarrierLedgerNilFailsLoud(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("minting without a ledger must fail loud")
		}
	}()
	var carriers *optionalCarrierLedger
	carriers.ignored("trace_root_causes", "no ledger")
}

// A selector on a request that offers no selection is ignored with the
// "omit" route — never the roster hint, and never a whole-answer reject.
func TestEmitAnswerDocumentSelectorWithoutContractIsIgnoredWithOmitHint(t *testing.T) {
	mutable := types.NewMutableState("plain code question")
	ctx := &types.BusContext{Mutable: mutable}
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, json.RawMessage(`{
		"blocks":[{"id":"summary","kind":"summary","text":"answer"}],
		"trace_root_causes":{"schema_version":2,"root_causes":[]}
	}`), time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: %+v %v", result, err)
	}
	if len(result.OptionalCarrierOutcomes) != 1 {
		t.Fatalf("exactly one typed outcome expected: %+v", result.OptionalCarrierOutcomes)
	}
	outcome := result.OptionalCarrierOutcomes[0]
	if outcome.Status != types.OptionalCarrierStatusIgnored || !strings.Contains(outcome.Reason, "not enabled") || !strings.Contains(outcome.Hint, "omit the field") {
		t.Fatalf("unexpected outcome: %+v", outcome)
	}
}

// A malformed selector object is worded with the fixed taught-shape sentence
// — the Go decoder's type names never reach the model.
func TestEmitAnswerDocumentMalformedSelectorIsWordedAsTheTaughtShape(t *testing.T) {
	_, ctx := rootCauseOutcomeFixture(t)
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, json.RawMessage(`{
		"blocks":[{"id":"summary","kind":"summary","text":"answer"}],
		"trace_root_causes":{"schema_version":"two","root_causes":[{"candidate_id":"candidate-sched"}]}
	}`), time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: %+v %v", result, err)
	}
	if len(result.OptionalCarrierOutcomes) != 1 {
		t.Fatalf("exactly one typed outcome expected: %+v", result.OptionalCarrierOutcomes)
	}
	reason := result.OptionalCarrierOutcomes[0].Reason
	if !strings.Contains(reason, "not the taught object") || strings.Contains(reason, "TraceRootCause") || strings.Contains(reason, "Go ") {
		t.Fatalf("decode failures must use the fixed model-facing wording: %q", reason)
	}
}

func TestEmitWriteAnalysisOverriddenEchoIsDisclosedOnTheResult(t *testing.T) {
	tool := &EmitWriteAnalysis{}
	bus := &types.BusContext{Mutable: types.NewMutableState("collapse each consecutive run to one token"), Mode: types.ModePlan}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"raw_request":"collapse an even run to two tokens",
		"task":{"kind":"bugfix","scope":"micro","summary":"repair run collapse"},
		"risk":{"affects_public_api":false,"changes_persistence":false,"changes_build_system":false,"overall":"low"}
	}`))
	if err != nil || !res.Success {
		t.Fatalf("Execute failed: err=%v summary=%s", err, res.Summary)
	}
	if len(res.OptionalCarrierOutcomes) != 1 || res.OptionalCarrierOutcomes[0].Carrier != "raw_request" || res.OptionalCarrierOutcomes[0].Status != types.OptionalCarrierStatusIgnored {
		t.Fatalf("the overridden echo must be a typed outcome on the accepted result: %+v", res.OptionalCarrierOutcomes)
	}
	if !strings.Contains(res.Summary, "[optional_carrier_ignored: carrier=raw_request") {
		t.Fatalf("disclosure line missing: %q", res.Summary)
	}
	// An echo that matches the system authority is not a carrier failure.
	bus = &types.BusContext{Mutable: types.NewMutableState("collapse each consecutive run to one token"), Mode: types.ModePlan}
	res, err = tool.Execute(bus, json.RawMessage(`{
		"raw_request":"collapse each consecutive run to one token",
		"task":{"kind":"bugfix","scope":"micro","summary":"repair run collapse"},
		"risk":{"affects_public_api":false,"changes_persistence":false,"changes_build_system":false,"overall":"low"}
	}`))
	if err != nil || !res.Success || len(res.OptionalCarrierOutcomes) != 0 {
		t.Fatalf("a matching echo must carry no outcome: %+v %v", res, err)
	}
}

func TestOptionalCarrierLedgerFinalizeKeepsExistingProseAndAddsTypedRows(t *testing.T) {
	carriers := newOptionalCarrierLedger("emit_investigation_complete")
	carriers.ignored("evidence_floor_waiver", "ignored evidence_floor_waiver because reason is missing; normal grounding gates still apply")
	carriers.mint(types.OptionalCarrierOutcome{Carrier: "", Reason: "no carrier — dropped"})
	carriers.mint(types.OptionalCarrierOutcome{Carrier: "x", Status: "bogus", Reason: "fresh reason"})
	res := carriers.finalize(types.ToolResult{Summary: "tool accepted | ignored evidence_floor_waiver because reason is missing; normal grounding gates still apply"})
	if len(res.OptionalCarrierOutcomes) != 2 || res.OptionalCarrierOutcomes[1].Status != types.OptionalCarrierStatusIgnored {
		t.Fatalf("rows: %+v", res.OptionalCarrierOutcomes)
	}
	if strings.Count(res.Summary, "reason is missing") != 1 || !strings.HasSuffix(res.Summary, "\n[optional_carrier_ignored: carrier=x reason=fresh reason]") {
		t.Fatalf("prose already on the summary must not be duplicated; a new reason gets its own line: %q", res.Summary)
	}
	// Idempotent against a row already on the result (a finalize never
	// duplicates what an earlier finalize of the same rows attached).
	again := carriers.finalize(res)
	if len(again.OptionalCarrierOutcomes) != 2 || again.Summary != res.Summary {
		t.Fatalf("finalize must be idempotent: %+v %q", again.OptionalCarrierOutcomes, again.Summary)
	}
}
