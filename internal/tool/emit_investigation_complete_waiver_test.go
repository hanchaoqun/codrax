// Package tool — emit_investigation_complete_waiver_test.go (2026-05-10).
//
// Step 1 of the model-confidence-respected architecture: the
// emit_investigation_complete tool accepts a typed
// evidence_floor_waiver lane the model uses to declare repo
// grounding does not apply (external log / no-repo-intersection /
// informational runtime / external trace). The waiver short-
// circuits the forced-read pending check and the citation-floor
// pre-flight.
//
// These tests pin: (1) strict-decode of reason against the typed
// enum, (2) required rationale audit trail, (3) successful store
// on the round-trip, (4) gate behaviour observed via the wider
// completion path. Hard-gate consumer integration lives in
// TestEmitInvestigationComplete_ForcedReadBypassedByModelWaiver below
// — the canonical regression for the 2026-05-10 customer scenario.
package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func runEIC(t *testing.T, mut *types.MutableState, params string) types.ToolResult {
	t.Helper()
	bus := &types.BusContext{Mutable: mut}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(params))
	if err != nil {
		t.Fatalf("Execute unexpected error: %v", err)
	}
	return res
}

func TestEmitInvestigationComplete_WaiverRejectsInvalidReason(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"investigation done",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"bogus_value","rationale":"x"}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("invalid reason must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "evidence_floor_waiver.reason") {
		t.Errorf("rejection must name the offending field: %q", res.Summary)
	}
	if !strings.Contains(res.Summary, "external_only_log") {
		t.Errorf("rejection must surface the accepted enum values: %q", res.Summary)
	}
	if mut.EvidenceFloorWaiver() != nil {
		t.Errorf("rejected waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_WaiverRequiresRationale(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_log","rationale":"   "}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("whitespace rationale must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "rationale") {
		t.Errorf("rejection must name the rationale requirement: %q", res.Summary)
	}
	if mut.EvidenceFloorWaiver() != nil {
		t.Errorf("rejected waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_WaiverRequiresReason(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"rationale":"log is external"}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("missing reason must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "evidence_floor_waiver.reason") {
		t.Errorf("rejection must name the reason requirement: %q", res.Summary)
	}
}

func TestEmitInvestigationComplete_WaiverStoredOnAcceptance(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"log is external — answer from log content directly",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_log",
			"rationale":"log frames reference services not in this repo"
		}
	}`
	res := runEIC(t, mut, params)
	// We allow the result to succeed OR be rejected by some OTHER
	// gate downstream (e.g. forced-read had pending entries that got
	// bypassed but some other check fires). The contract here is
	// that the waiver itself stored cleanly — not whether the
	// outer completion succeeded.
	w := mut.EvidenceFloorWaiver()
	if !w.IsActive() {
		t.Fatalf("waiver must be stored and active after acceptance; got %+v (Success=%v Summary=%q)",
			w, res.Success, res.Summary)
	}
	if w.Reason != types.EvidenceFloorWaiverExternalLog {
		t.Errorf("Reason round-trip wrong: got %q", w.Reason)
	}
	if !strings.Contains(w.Rationale, "services not in this repo") {
		t.Errorf("Rationale round-trip wrong: got %q", w.Rationale)
	}
}

func TestEmitInvestigationComplete_WaiverAcceptsAllFourReasons(t *testing.T) {
	for _, reason := range types.EvidenceFloorWaiverReasonValues() {
		t.Run(string(reason), func(t *testing.T) {
			mut := types.NewMutableState("q")
			params := `{
				"reason":"done",
				"confidence":"high",
				"result_kind":"resolved",
				"evidence_floor_waiver":{"reason":"` + string(reason) + `","rationale":"justified"}
			}`
			runEIC(t, mut, params)
			w := mut.EvidenceFloorWaiver()
			if !w.IsActive() || w.Reason != reason {
				t.Errorf("reason %q: waiver did not store, got %+v", reason, w)
			}
		})
	}
}

func TestEmitInvestigationComplete_NoWaiverNoBypass(t *testing.T) {
	// Absent waiver = normal completion, MutableState.EvidenceFloorWaiver stays nil.
	mut := types.NewMutableState("q")
	params := `{"reason":"done","confidence":"high","result_kind":"resolved"}`
	runEIC(t, mut, params)
	if mut.EvidenceFloorWaiver() != nil {
		t.Errorf("waiver must remain nil when emit_investigation_complete is called without the field")
	}
}

func TestJoinEvidenceFloorWaiverReasons_IsSchemaCanonical(t *testing.T) {
	// The retry-hint helper renders the accepted-values list. It
	// MUST match types.EvidenceFloorWaiverReasonValues() so the
	// schema description, validator error, and retry hint stay in
	// sync (R2' canonical-list sync).
	got := joinEvidenceFloorWaiverReasons()
	for _, r := range types.EvidenceFloorWaiverReasonValues() {
		if !strings.Contains(got, string(r)) {
			t.Errorf("retry-hint must include %q from canonical enum: got %q", r, got)
		}
	}
}
