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
// These tests pin: (1) invalid optional waiver payloads are ignored
// and never honored, (2) valid typed waivers still store on the
// round-trip, (3) clear+set remains a precise hard conflict, (4)
// gate behaviour observed via the wider completion path. Hard-gate
// consumer integration lives in
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

func TestEmitInvestigationComplete_WaiverIgnoresInvalidReason(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"investigation done",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"bogus_value","rationale":"x"}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("invalid optional waiver should be ignored, not reject; Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored evidence_floor_waiver.reason") {
		t.Errorf("summary must audit ignored optional waiver: %q", res.Summary)
	}
	if mut.EvidenceFloorWaiver() != nil {
		t.Errorf("ignored waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_WaiverIgnoresBlankRationale(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_log","rationale":"   "}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("blank optional waiver rationale should be ignored, not reject; Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored evidence_floor_waiver=external_only_log because rationale is missing") {
		t.Errorf("summary must audit ignored optional waiver: %q", res.Summary)
	}
	if mut.EvidenceFloorWaiver() != nil {
		t.Errorf("ignored waiver must NOT be stored")
	}
}

func TestEmitInvestigationComplete_WaiverIgnoresMissingReason(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"done",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"rationale":"log is external"}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("missing optional waiver reason should be ignored, not reject; Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "ignored evidence_floor_waiver because reason is missing") {
		t.Errorf("summary must audit ignored optional waiver: %q", res.Summary)
	}
	if mut.EvidenceFloorWaiver() != nil {
		t.Errorf("ignored waiver must NOT be stored")
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
	stable := mut.StableEvidenceFloorWaiver()
	if !stable.IsActive() || stable.Reason != types.EvidenceFloorWaiverExternalLog {
		t.Fatalf("successful completion must retain stable waiver, got %+v", stable)
	}
}

// TestEmitInvestigationComplete_WaiverAcceptedOnZeroCurrentSourceRepo pins the
// §1.6 regression from the customer trace_repl.log (2026-07-02): the model
// declared evidence_floor_waiver=external_only_trace correctly, but the
// admissibility gate silently dropped it because a current-status contract
// requirement kept the current-source lane "required" — in a checkout that
// contained no current source at all. With the deterministic census the typed
// escape lane must work again: the waiver stores and completion lands.
func TestEmitInvestigationComplete_WaiverAcceptedOnZeroCurrentSourceRepo(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("分析东湖Trace:record_trace_20260605224432@3279-299954687.sys.ftrace里面这一帧，不分析代码")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentRootCause},
			AnswerContract: types.AnswerContract{
				CitationReq:             types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
			},
		},
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "record_trace_20260605224432@3279-299954687.sys.ftrace",
				Carrier: "request_path",
			}},
			RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, ArtifactFiles: 1},
		}),
	}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"UI thread blocked on lock contention inside the doFrame window; trace is the only answer source",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_trace","rationale":"repository contains only the ftrace artifact, no source code"}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w := mut.EvidenceFloorWaiver(); !w.IsActive() || w.Reason != types.EvidenceFloorWaiverExternalTrace {
		t.Fatalf("waiver must be stored on a zero-current-source repo, got %+v (Summary=%q)", w, res.Summary)
	}
	if !res.Success || strings.Contains(res.Summary, "ignored evidence_floor_waiver") {
		t.Fatalf("waiver must not be silently dropped on a zero-current-source repo: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_WaiverAdmittedViaTypedArtifactIdentity pins
// the carrier half of the same customer failure: the analyzer cleanly
// extracted the .sys.ftrace filename into typed entities but emitted no
// external-observation policy, and every policy-gated carrier read false — so
// the declared waiver was dropped even though a runtime artifact was plainly
// in play. The typed artifact-path identity (policy-independent) must be
// enough to admit the waiver on this relaxation surface.
func TestEmitInvestigationComplete_WaiverAdmittedViaTypedArtifactIdentity(t *testing.T) {
	prev := CurrentGroundingPolicy()
	SetGroundingPolicy(GroundingPolicy{GroundingFloor: 0, Tier1Floor: 0})
	t.Cleanup(func() { SetGroundingPolicy(prev) })

	mut := types.NewMutableState("分析 trace 丢帧,不分析代码")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				AnalyzerHints: types.AnalyzerHints{
					Entities: []string{"record_trace_20260605224432@3279-299954687.sys.ftrace", "Choreographer#doFrame 94410"},
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 2},
			},
		},
	}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"trace-only diagnosis complete from runtime observations",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_trace","rationale":"only evidence source is the referenced ftrace artifact"}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w := mut.EvidenceFloorWaiver(); !w.IsActive() {
		t.Fatalf("typed artifact identity must admit the waiver without an analyzer policy, got %+v (Summary=%q)", w, res.Summary)
	}
	if strings.Contains(res.Summary, "ignored evidence_floor_waiver") {
		t.Fatalf("waiver must not be dropped when a typed artifact identity is present: %s", res.Summary)
	}
}

// TestEmitInvestigationComplete_WaiverRejectionNamesBlockingRequirement pins
// the actionability half of the same fix: when the waiver IS legitimately
// inadmissible (source files exist and the request demands change impact
// against the current checkout), the ignored-note must name the blocking
// requirement instead of a bare generic line the model can only retry blind.
func TestEmitInvestigationComplete_WaiverRejectionNamesBlockingRequirement(t *testing.T) {
	mut := types.NewMutableState("mixed trace + source question")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:              types.IntentRootCause,
				ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true, Target: "FrameScheduler"},
			},
		},
		RuntimeArtifactPreflight: types.NormalizeRuntimeArtifactPreflightProfile(types.RuntimeArtifactPreflightProfile{
			SourceNavigationOptional: true,
			Artifacts: []types.RuntimeArtifactPreflightArtifact{{
				Kind:    "trace",
				Source:  "record_trace_20260605224432@3279-299954687.sys.ftrace",
				Carrier: "request_path",
			}},
			RepoSourceCensus: types.RuntimeArtifactRepoSourceCensus{Completed: true, SourceFiles: 5, ArtifactFiles: 1},
		}),
	}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"trace analysis finished",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{"reason":"external_only_trace","rationale":"trace rows are external"}
	}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if w := mut.EvidenceFloorWaiver(); w.IsActive() {
		t.Fatalf("waiver must stay rejected when a real current-source requirement holds, got %+v", w)
	}
	if !strings.Contains(res.Summary, "change impact against the current checkout") {
		t.Fatalf("rejection must name the blocking requirement, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_AcceptedCompletionClearsBypassedPendingReads(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.EvidenceClosure().AddPendingRead(types.PendingRead{
		File:      "internal/agent/analyzer.go",
		Origin:    "pre_complete.support_tail",
		Rationale: "support-only stale forced-read",
		Stage:     "explore",
	})
	params := `{
		"reason":"log content is sufficient; repo grounding would be misleading",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_log",
			"rationale":"log frames reference services not in this repo"
		}
	}`
	res := runEIC(t, mut, params)
	if !res.Success {
		t.Fatalf("waiver-backed accepted completion should succeed, got Summary=%q", res.Summary)
	}
	if got := mut.EvidenceClosure().PendingReads(); len(got) != 0 {
		t.Fatalf("accepted completion must clear stale pending forced-reads, got %+v", got)
	}
}

func TestEmitInvestigationComplete_IgnoresWaiverForPureVCSHistory(t *testing.T) {
	mut := types.NewMutableState("q")
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			RawRequest: "最近 10 次提交都做了什么",
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	tool := &EmitInvestigationComplete{}
	res, err := tool.Execute(bus, json.RawMessage(`{
		"reason":"git_log already provides the requested commit history summary",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_trace",
			"rationale":"git history is outside source file citations"
		}
	}`))
	if err != nil {
		t.Fatalf("Execute unexpected error: %v", err)
	}
	if !res.Success {
		t.Fatalf("pure VCS waiver misuse should be ignored without forcing a retry: %s", res.Summary)
	}
	if mut.EvidenceFloorWaiver() != nil || mut.StableEvidenceFloorWaiver() != nil {
		t.Fatalf("pure VCS history must not retain runtime-artifact waiver: current=%+v stable=%+v",
			mut.EvidenceFloorWaiver(), mut.StableEvidenceFloorWaiver())
	}
	if !strings.Contains(res.Summary, "ignored evidence_floor_waiver") ||
		!strings.Contains(res.Summary, "pure VCS history") {
		t.Fatalf("summary should disclose ignored VCS waiver misuse, got: %s", res.Summary)
	}
}

func TestEmitInvestigationComplete_WaiverNotStableWhenLaterGateRejects(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"positive answer but bad absence field",
		"confidence":"high",
		"result_kind":"resolved",
		"absence_justification":"nothing exists",
		"evidence_floor_waiver":{
			"reason":"external_only_log",
			"rationale":"log frames reference services not in this repo"
		}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("contradictory absence field should reject, got Summary=%q", res.Summary)
	}
	if !mut.EvidenceFloorWaiver().IsActive() {
		t.Fatalf("current waiver should remain available for retry")
	}
	if got := mut.StableEvidenceFloorWaiver(); got != nil {
		t.Fatalf("rejected completion must not retain stable waiver, got %+v", got)
	}
}

func TestEmitInvestigationComplete_ClearWaiver(t *testing.T) {
	mut := types.NewMutableState("q")
	first := `{
		"reason":"log is external",
		"confidence":"high",
		"result_kind":"resolved",
		"evidence_floor_waiver":{
			"reason":"external_only_log",
			"rationale":"log frames reference services not in this repo"
		}
	}`
	if res := runEIC(t, mut, first); !res.Success {
		t.Fatalf("initial waiver completion failed: %s", res.Summary)
	}
	if !mut.StableEvidenceFloorWaiver().IsActive() {
		t.Fatalf("initial waiver was not retained")
	}

	clear := `{
		"reason":"later inspection found repo grounding applies",
		"confidence":"high",
		"result_kind":"resolved",
		"clear_evidence_floor_waiver":true
	}`
	if res := runEIC(t, mut, clear); !res.Success {
		t.Fatalf("clear waiver completion failed: %s", res.Summary)
	}
	if got := mut.EvidenceFloorWaiver(); got != nil {
		t.Fatalf("current waiver should be cleared, got %+v", got)
	}
	if got := mut.StableEvidenceFloorWaiver(); got != nil {
		t.Fatalf("stable waiver should be cleared, got %+v", got)
	}
}

func TestEmitInvestigationComplete_ClearWaiverRejectsWithNewWaiver(t *testing.T) {
	mut := types.NewMutableState("q")
	params := `{
		"reason":"ambiguous",
		"confidence":"high",
		"result_kind":"resolved",
		"clear_evidence_floor_waiver":true,
		"evidence_floor_waiver":{"reason":"external_only_log","rationale":"x"}
	}`
	res := runEIC(t, mut, params)
	if res.Success {
		t.Fatalf("clear + set must reject, got Summary=%q", res.Summary)
	}
	if !strings.Contains(res.Summary, "clear_evidence_floor_waiver") {
		t.Fatalf("rejection should name clear field, got %q", res.Summary)
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
