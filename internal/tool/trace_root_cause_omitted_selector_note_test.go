package tool

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// trace_root_cause_omitted_selector_note_test.go — colleague B1548 residual
// (colleague_merge_audit §40.59): the one selector shape that is neither
// rejected nor inherited — the model NEVER submits `trace_root_causes` while
// the contract offers a non-empty selectable roster — used to be silent on
// the accepted emit, and the customer's default root-cause JSON then
// reported "never selected" although the long answer named candidates.
// Omission stays a legitimate model choice (no typed outcome, no retry
// round): the accepted result carries ONE plain Summary line naming the
// selectable ids and the repair route.

func emitAnswerDocumentForNoteTest(t *testing.T, ctx *types.BusContext, payload map[string]any) types.ToolResult {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := executeAnswerDocumentV2("emit_answer_document", ctx, raw, time.Now())
	if err != nil || !result.Success {
		t.Fatalf("emit failed: result=%+v err=%v", result, err)
	}
	return result
}

func TestEmitAnswerDocumentOmittedSelectorWithSelectableRosterNotesOnce(t *testing.T) {
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	result := emitAnswerDocumentForNoteTest(t, ctx, map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "answer without a selection"}},
	})
	want := "trace_root_causes omitted: the run's default root-cause JSON will report no model selection while 1 selectable candidate_id value(s) remain (candidate-sched)"
	if !strings.Contains(result.Summary, want) {
		t.Fatalf("accepted emit that omits the selector beside a selectable roster must carry the advisory note:\n%s", result.Summary)
	}
	if strings.Count(result.Summary, "trace_root_causes omitted:") != 1 {
		t.Fatalf("the note must ride exactly once:\n%s", result.Summary)
	}
	if len(result.OptionalCarrierOutcomes) != 0 {
		t.Fatalf("omission is a legitimate choice — no typed outcome may be minted: %+v", result.OptionalCarrierOutcomes)
	}
	if mutable.TraceRootCauseReport() != nil || mutable.PendingTraceRootCauseReport() != nil {
		t.Fatalf("the note must not mint or stage a report")
	}
	// The next full re-emit that still omits the selector notes again (one
	// per call — the ledger is per call), still without a typed row.
	again := emitAnswerDocumentForNoteTest(t, ctx, map[string]any{
		"blocks": []map[string]any{{"id": "summary", "kind": "summary", "text": "revised answer without a selection"}},
	})
	if strings.Count(again.Summary, "trace_root_causes omitted:") != 1 || len(again.OptionalCarrierOutcomes) != 0 {
		t.Fatalf("re-emit without a selection must note once and mint nothing:\n%s\n%+v", again.Summary, again.OptionalCarrierOutcomes)
	}
	for _, internal := range []string{"OptionalCarrierOutcome", "reason_code", "valid_model_root_cause_selection_unavailable", "sidecar", ".json"} {
		if strings.Contains(again.Summary, internal) {
			t.Fatalf("the model-facing note leaks an internal identifier %q:\n%s", internal, again.Summary)
		}
	}
}

func TestEmitAnswerDocumentOmittedSelectorNoteNegativeArms(t *testing.T) {
	blocks := []map[string]any{{"id": "summary", "kind": "summary", "text": "answer"}}
	noSelectable := testSelectableTraceRootCauseContract()
	noSelectable.Candidates[0].Decision.Magnitude.Unit = "count"
	disabled := testSelectableTraceRootCauseContract()
	disabled.RootCauseReportEnabled = false
	cases := []struct {
		name    string
		prepare func(m *types.MutableState)
		payload map[string]any
	}{
		{"contract disabled", func(m *types.MutableState) { m.SetTraceFindingContract(disabled) }, map[string]any{"blocks": blocks}},
		{"zero selectable candidates", func(m *types.MutableState) { m.SetTraceFindingContract(noSelectable) }, map[string]any{"blocks": blocks}},
		{"no contract", func(m *types.MutableState) {}, map[string]any{"blocks": blocks}},
		{"staged valid selection inherited", func(m *types.MutableState) {
			m.SetTraceFindingContract(testSelectableTraceRootCauseContract())
			m.SetPendingTraceRootCauseReport(&types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion})
		}, map[string]any{"blocks": blocks}},
		{"explicit empty selection withdraws", func(m *types.MutableState) { m.SetTraceFindingContract(testSelectableTraceRootCauseContract()) }, map[string]any{
			"blocks": blocks,
			"trace_root_causes": map[string]any{
				"schema_version": types.TraceRootCauseReportSchemaVersion,
				"root_causes":    []map[string]any{},
			},
		}},
		{"valid selection submitted", func(m *types.MutableState) { m.SetTraceFindingContract(testSelectableTraceRootCauseContract()) }, map[string]any{
			"blocks": blocks,
			"trace_root_causes": map[string]any{
				"schema_version": types.TraceRootCauseReportSchemaVersion,
				"root_causes":    []map[string]any{{"candidate_id": "candidate-sched"}},
			},
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mutable := types.NewMutableState("analyze trace root cause")
			tc.prepare(mutable)
			ctx := &types.BusContext{Mutable: mutable}
			result := emitAnswerDocumentForNoteTest(t, ctx, tc.payload)
			if strings.Contains(result.Summary, "omitted: the run's default root-cause JSON") {
				t.Fatalf("no advisory note may ride in this shape:\n%s", result.Summary)
			}
		})
	}
	// Accepted report present: a later emit/patch that omits the selector
	// inherits it (§40.29.1 ★17) — no note either.
	mutable := types.NewMutableState("analyze trace root cause")
	mutable.SetTraceFindingContract(testSelectableTraceRootCauseContract())
	ctx := &types.BusContext{Mutable: mutable}
	emitAnswerDocumentForNoteTest(t, ctx, map[string]any{
		"blocks": blocks,
		"trace_root_causes": map[string]any{
			"schema_version": types.TraceRootCauseReportSchemaVersion,
			"root_causes":    []map[string]any{{"candidate_id": "candidate-sched"}},
		},
	})
	result := emitAnswerDocumentForNoteTest(t, ctx, map[string]any{"blocks": blocks})
	if strings.Contains(result.Summary, "omitted: the run's default root-cause JSON") {
		t.Fatalf("an inherited accepted report must not be reported as omitted:\n%s", result.Summary)
	}
}

// The teaching is single-source (schema description == selector context,
// pinned in internal/agent): the omission-with-roster behaviour the note
// implements must be promised there too, so the model is never surprised.
func TestTraceRootCauseSelectorTeachingPromisesTheOmissionNote(t *testing.T) {
	teaching := types.TraceRootCauseSelectorOutcomeTeaching()
	if !strings.Contains(teaching, "Omitting it while selectable candidates exist is accepted as your choice, and the tool result then notes that the report will carry no model selection.") {
		t.Fatalf("selector teaching must promise the omission note: %q", teaching)
	}
}
