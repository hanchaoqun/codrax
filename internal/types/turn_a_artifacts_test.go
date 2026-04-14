package types

// P2.1 Session 1 Phase 6 — TurnAArtifacts handoff + MarkHypothesis
// carve-out tests.
//
// The handoff struct is the contract surface between Turn A
// (explorer) and Turn B (extractor) — both sides ship in Session 2,
// but Phase 6 pins the shape now so the Session 2 wiring on either
// side cannot silently drop a field. The MarkHypothesis carve-out
// is the D7 deferred item from project_p1_3_deferred_items.md.

import (
	"testing"
)

func TestTurnAArtifacts_RoundtripPreservesAllFields(t *testing.T) {
	m := NewMutableState(TaskList{})
	original := TurnAArtifacts{
		UserQuestion:       "what does Foo return?",
		InvestigationNotes: []string{"iter 1 narrative", "iter 2 narrative"},
		ReadFiles:          []string{"a.go", "b.go"},
		ToolResults: []ToolResult{
			{ToolName: "grep", Summary: "a.go:1", Success: true},
		},
		EvidenceItems: []EvidenceItem{
			{ID: "ev1", Kind: EvidenceDirect, Source: "a.go", LineStart: 5},
		},
		FlowFindings: []FlowFindingDigest{
			{ID: "ff1", Path: []string{"src", "sink"}, Confidence: 0.7},
		},
	}
	m.SetTurnAArtifacts(original)
	got := m.TurnAArtifacts()
	if got == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if got.UserQuestion != original.UserQuestion {
		t.Errorf("UserQuestion: got %q, want %q", got.UserQuestion, original.UserQuestion)
	}
	if len(got.InvestigationNotes) != 2 || got.InvestigationNotes[1] != "iter 2 narrative" {
		t.Errorf("InvestigationNotes not preserved: %+v", got.InvestigationNotes)
	}
	if len(got.ReadFiles) != 2 || got.ReadFiles[0] != "a.go" {
		t.Errorf("ReadFiles not preserved: %+v", got.ReadFiles)
	}
	if len(got.ToolResults) != 1 || got.ToolResults[0].ToolName != "grep" {
		t.Errorf("ToolResults not preserved: %+v", got.ToolResults)
	}
	if len(got.EvidenceItems) != 1 || got.EvidenceItems[0].ID != "ev1" {
		t.Errorf("EvidenceItems not preserved: %+v", got.EvidenceItems)
	}
	if len(got.FlowFindings) != 1 || got.FlowFindings[0].ID != "ff1" {
		t.Errorf("FlowFindings not preserved: %+v", got.FlowFindings)
	}
}

func TestTurnAArtifacts_NilBeforeSet(t *testing.T) {
	m := NewMutableState(TaskList{})
	if got := m.TurnAArtifacts(); got != nil {
		t.Errorf("expected nil before any Set, got %+v", got)
	}
}

func TestTurnAArtifacts_DefensiveCopyOnWrite(t *testing.T) {
	// SetTurnAArtifacts must defensively copy slice headers so a
	// later append on the caller side cannot mutate the buffered
	// snapshot in place. This is the structural defense against a
	// Session-2 explorer ParseOutput that builds the slices once and
	// then keeps appending in subsequent iterations.
	m := NewMutableState(TaskList{})
	notes := []string{"iter 1"}
	m.SetTurnAArtifacts(TurnAArtifacts{InvestigationNotes: notes})

	notes = append(notes, "iter 2 (after handoff)")
	got := m.TurnAArtifacts()
	if len(got.InvestigationNotes) != 1 {
		t.Errorf("post-handoff append leaked into snapshot: %+v", got.InvestigationNotes)
	}
}

func TestTurnAArtifacts_DefensiveCopyOnRead(t *testing.T) {
	// TurnAArtifacts() returns a fresh copy. Mutating the returned
	// pointer must not affect the next read.
	m := NewMutableState(TaskList{})
	m.SetTurnAArtifacts(TurnAArtifacts{
		ReadFiles: []string{"a.go", "b.go"},
	})
	first := m.TurnAArtifacts()
	first.ReadFiles[0] = "MUTATED.go"
	first.ReadFiles = append(first.ReadFiles, "c.go")

	second := m.TurnAArtifacts()
	if second.ReadFiles[0] != "a.go" {
		t.Errorf("read-side mutation leaked back: %q", second.ReadFiles[0])
	}
	if len(second.ReadFiles) != 2 {
		t.Errorf("read-side append leaked back: len=%d", len(second.ReadFiles))
	}
}

func TestTurnAArtifacts_Reset(t *testing.T) {
	m := NewMutableState(TaskList{})
	m.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "first"})
	m.ResetTurnAArtifacts()
	if got := m.TurnAArtifacts(); got != nil {
		t.Errorf("Reset must clear; got %+v", got)
	}
	// Set after reset works.
	m.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "second"})
	if got := m.TurnAArtifacts(); got == nil || got.UserQuestion != "second" {
		t.Errorf("post-reset Set must work; got %+v", got)
	}
}

func TestTurnAArtifacts_NilMutableStateSafe(t *testing.T) {
	var m *MutableState
	// All four operations must be no-ops, not panics.
	m.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "x"})
	if got := m.TurnAArtifacts(); got != nil {
		t.Errorf("nil receiver must return nil")
	}
	m.ResetTurnAArtifacts()
}

// ── MarkHypothesis carve-out ───────────────────────────────────────

func TestMarkHypothesis_UpdatesStatus(t *testing.T) {
	ir := &AnalysisIR{
		HypothesisSet: []Hypothesis{
			{ID: "H1", Statement: "Foo returns true", Status: HypUnknown},
			{ID: "H2", Statement: "Bar registers Baz", Status: HypUnknown},
		},
	}
	if err := ir.MarkHypothesis("H1", HypConfirmed); err != nil {
		t.Fatalf("MarkHypothesis(H1, confirmed): %v", err)
	}
	if ir.HypothesisSet[0].Status != HypConfirmed {
		t.Errorf("H1 status not updated: %q", ir.HypothesisSet[0].Status)
	}
	if ir.HypothesisSet[1].Status != HypUnknown {
		t.Errorf("H2 must remain unchanged: %q", ir.HypothesisSet[1].Status)
	}
}

func TestMarkHypothesis_AllValidStatuses(t *testing.T) {
	for _, st := range []HypothesisStatus{HypUnknown, HypConfirmed, HypRejected, HypInconclusive} {
		t.Run(string(st), func(t *testing.T) {
			ir := &AnalysisIR{
				HypothesisSet: []Hypothesis{{ID: "H1", Status: HypUnknown}},
			}
			if err := ir.MarkHypothesis("H1", st); err != nil {
				t.Errorf("MarkHypothesis(H1, %q): %v", st, err)
			}
			if ir.HypothesisSet[0].Status != st {
				t.Errorf("status not set: got %q, want %q", ir.HypothesisSet[0].Status, st)
			}
		})
	}
}

func TestMarkHypothesis_RejectsUnknownID(t *testing.T) {
	ir := &AnalysisIR{
		HypothesisSet: []Hypothesis{{ID: "H1", Status: HypUnknown}},
	}
	err := ir.MarkHypothesis("H99", HypConfirmed)
	if err == nil {
		t.Fatal("expected error for unknown id")
	}
	if ir.HypothesisSet[0].Status != HypUnknown {
		t.Error("on error, no status must be mutated")
	}
}

func TestMarkHypothesis_RejectsEmptyID(t *testing.T) {
	ir := &AnalysisIR{HypothesisSet: []Hypothesis{{ID: "H1", Status: HypUnknown}}}
	if err := ir.MarkHypothesis("", HypConfirmed); err == nil {
		t.Fatal("expected error for empty id")
	}
}

func TestMarkHypothesis_RejectsUnknownStatus(t *testing.T) {
	ir := &AnalysisIR{HypothesisSet: []Hypothesis{{ID: "H1", Status: HypUnknown}}}
	err := ir.MarkHypothesis("H1", HypothesisStatus("partial"))
	if err == nil {
		t.Fatal("expected error for unknown status")
	}
	if ir.HypothesisSet[0].Status != HypUnknown {
		t.Error("on error, status must not be touched")
	}
}

func TestMarkHypothesis_NilIRSafe(t *testing.T) {
	var ir *AnalysisIR
	if err := ir.MarkHypothesis("H1", HypConfirmed); err == nil {
		t.Fatal("nil IR must return an error, not panic")
	}
}

func TestMarkHypothesis_IsIdempotentByValue(t *testing.T) {
	// Calling twice with the same value is allowed and a no-op
	// observably (same final state, no error).
	ir := &AnalysisIR{HypothesisSet: []Hypothesis{{ID: "H1", Status: HypUnknown}}}
	if err := ir.MarkHypothesis("H1", HypConfirmed); err != nil {
		t.Fatalf("first call: %v", err)
	}
	if err := ir.MarkHypothesis("H1", HypConfirmed); err != nil {
		t.Fatalf("second call: %v", err)
	}
	if ir.HypothesisSet[0].Status != HypConfirmed {
		t.Errorf("final status: %q", ir.HypothesisSet[0].Status)
	}
}

// ── HypothesisVerdict buffer (already shipped in P3) — sanity check
// that the new TurnAArtifacts field does not interfere with the
// emit_hypothesis_verdict pipeline.

func TestHypothesisVerdictBuffer_IndependentFromTurnAArtifacts(t *testing.T) {
	m := NewMutableState(TaskList{})
	m.AppendEmittedHypothesisVerdicts([]HypothesisVerdict{
		{HypothesisID: "H1", Status: HypConfirmed, Citation: "a.go:1"},
	})
	m.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "q"})

	verdicts := m.EmittedHypothesisVerdicts()
	if len(verdicts) != 1 || verdicts[0].HypothesisID != "H1" {
		t.Errorf("verdict buffer leaked: %+v", verdicts)
	}
	artifacts := m.TurnAArtifacts()
	if artifacts == nil || artifacts.UserQuestion != "q" {
		t.Errorf("artifacts buffer leaked: %+v", artifacts)
	}

	m.ResetEmittedHypothesisVerdicts()
	if v := m.EmittedHypothesisVerdicts(); len(v) != 0 {
		t.Error("verdict reset must clear verdicts only")
	}
	if a := m.TurnAArtifacts(); a == nil {
		t.Error("verdict reset must NOT touch turn-a artifacts")
	}
}
