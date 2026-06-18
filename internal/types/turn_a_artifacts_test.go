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
	"strings"
	"testing"
)

func TestTurnAArtifacts_RoundtripPreservesAllFields(t *testing.T) {
	m := NewMutableState("")
	original := TurnAArtifacts{
		UserQuestion:       "what does Foo return?",
		InvestigationNotes: []string{"iter 1 narrative", "iter 2 narrative"},
		ValidationBoundaryNotes: []string{
			"criterion=answer_set_bounded; downstream=disclose boundary",
		},
		ReadFiles: []string{"a.go", "b.go"},
		ToolResults: []ToolResult{
			{ToolName: "grep", Summary: "a.go:1", Success: true},
		},
		MCPResponses: []MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call",
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			Success:     true,
			Observations: []MCPTypedObservation{{
				Summary:   "line 7",
				LineStart: 7,
			}},
		}},
		EvidenceItems: []EvidenceItem{
			{ID: "ev1", Kind: EvidenceDirect, Source: "a.go", LineStart: 5},
		},
		FlowFindings: []FlowFindingDigest{
			{ID: "ff1", Path: []string{"src", "sink"}, Confidence: 0.7},
		},
		AcceptedAggregateFacts: []AnswerAggregateFact{
			{Kind: AnswerAggregateTotalCount, Label: "matches", Value: "3", Members: []string{"a.go:5"}},
		},
		SourceInventoryAdvisory: SourceInventoryAdvisory{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     true,
			Scopes:       []string{"src"},
			Provenance:   []string{"answer_role_profile", "repomap_graph"},
			Sets: []SourceInventoryAdvisorySet{{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Candidates: []SourceInventoryAdvisoryCandidate{{
					Member:     "Run",
					SupportRef: "Run: src/run.py:7",
					File:       "src/run.py",
					Line:       7,
					Language:   "python",
				}},
			}},
		},
		SourceInventoryObservation: SourceInventoryObservation{
			Active:       true,
			AdvisoryOnly: true,
			Complete:     true,
			Scopes:       []string{"src"},
			Provenance:   []string{"repomap_graph"},
			Lens:         []string{"members", "count"},
			Sets: []SourceInventoryObservationSet{{
				Role:     AnswerCandidateRoleFunction,
				Complete: true,
				Count:    1,
				Members: []SourceInventoryObservationMember{{
					Name:     "Run",
					File:     "src/run.py",
					Line:     7,
					Language: "python",
				}},
			}},
		},
		SourceLocalization: &SourceLocalizationReview{
			Status:      SourceLocalizationObserved,
			Source:      "read_turn_a",
			ReasonCodes: []string{"read_turn_a_source_observed"},
			SourcePaths: []string{"a.go", "b.go"},
			EvidenceRefs: []WriteExplorationEvidenceRef{{
				ID:        "ev1",
				Source:    "a.go",
				LineStart: 5,
			}},
		},
		RuntimeObservationOnlyCompletion: true,
		TerminalEvidenceCount:            3,
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
	if len(got.ValidationBoundaryNotes) != 1 || got.ValidationBoundaryNotes[0] != original.ValidationBoundaryNotes[0] {
		t.Errorf("ValidationBoundaryNotes not preserved: %+v", got.ValidationBoundaryNotes)
	}
	if len(got.ReadFiles) != 2 || got.ReadFiles[0] != "a.go" {
		t.Errorf("ReadFiles not preserved: %+v", got.ReadFiles)
	}
	if len(got.ToolResults) != 1 || got.ToolResults[0].ToolName != "grep" {
		t.Errorf("ToolResults not preserved: %+v", got.ToolResults)
	}
	if len(got.MCPResponses) != 1 || got.MCPResponses[0].ResourceURI != "mcp://fixture/trace/sleep-wakeup" {
		t.Errorf("MCPResponses not preserved: %+v", got.MCPResponses)
	}
	if len(got.EvidenceItems) != 1 || got.EvidenceItems[0].ID != "ev1" {
		t.Errorf("EvidenceItems not preserved: %+v", got.EvidenceItems)
	}
	if len(got.FlowFindings) != 1 || got.FlowFindings[0].ID != "ff1" {
		t.Errorf("FlowFindings not preserved: %+v", got.FlowFindings)
	}
	if len(got.AcceptedAggregateFacts) != 1 || got.AcceptedAggregateFacts[0].Value != "3" {
		t.Errorf("AcceptedAggregateFacts not preserved: %+v", got.AcceptedAggregateFacts)
	}
	if !got.SourceInventoryAdvisory.IsActive() ||
		len(got.SourceInventoryAdvisory.Sets) != 1 ||
		got.SourceInventoryAdvisory.Sets[0].Candidates[0].Language != "python" {
		t.Errorf("SourceInventoryAdvisory not preserved: %+v", got.SourceInventoryAdvisory)
	}
	if !got.SourceInventoryObservation.IsActive() ||
		len(got.SourceInventoryObservation.Sets) != 1 ||
		got.SourceInventoryObservation.Sets[0].Count != 1 ||
		got.SourceInventoryObservation.Sets[0].Members[0].Language != "python" {
		t.Errorf("SourceInventoryObservation not preserved: %+v", got.SourceInventoryObservation)
	}
	if got.SourceLocalization == nil ||
		got.SourceLocalization.Status != SourceLocalizationObserved ||
		len(got.SourceLocalization.SourcePaths) != 2 ||
		got.SourceLocalization.EvidenceRefs[0].Source != "a.go" {
		t.Errorf("SourceLocalization not preserved: %+v", got.SourceLocalization)
	}
	if !got.RuntimeObservationOnlyCompletion {
		t.Error("RuntimeObservationOnlyCompletion not preserved")
	}
	if got.TerminalEvidenceCount != 3 {
		t.Errorf("TerminalEvidenceCount: got %d, want 3", got.TerminalEvidenceCount)
	}
}

func TestTurnAArtifacts_TerminalEvidenceCount_ZeroRoundtrip(t *testing.T) {
	// Zero-value TerminalEvidenceCount must round-trip as 0 — it is
	// the "no β constraint" sentinel that Phase 9's validator reads
	// as "baseline collapses to len(MustInclude) alone". A non-zero
	// default would silently activate the β baseline on legacy runs.
	m := NewMutableState("")
	m.SetTurnAArtifacts(TurnAArtifacts{UserQuestion: "q"})
	got := m.TurnAArtifacts()
	if got == nil {
		t.Fatal("expected snapshot, got nil")
	}
	if got.TerminalEvidenceCount != 0 {
		t.Errorf("TerminalEvidenceCount default: got %d, want 0", got.TerminalEvidenceCount)
	}
}

func TestTurnAArtifacts_NilBeforeSet(t *testing.T) {
	m := NewMutableState("")
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
	m := NewMutableState("")
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
	m := NewMutableState("")
	m.SetTurnAArtifacts(TurnAArtifacts{
		ReadFiles:               []string{"a.go", "b.go"},
		ValidationBoundaryNotes: []string{"boundary 1"},
		MCPResponses: []MCPResponse{{
			ServerName:  "fixture",
			Method:      "tools/call",
			ResourceURI: "mcp://fixture/trace/sleep-wakeup",
			Success:     true,
		}},
		AcceptedAggregateFacts: []AnswerAggregateFact{{
			Kind:    AnswerAggregateTotalCount,
			Label:   "matches",
			Value:   "2",
			Members: []string{"a.go:1"},
		}},
		SourceInventoryAdvisory: SourceInventoryAdvisory{
			Active: true,
			Scopes: []string{"src"},
			Sets: []SourceInventoryAdvisorySet{{
				Role: AnswerCandidateRoleFunction,
				Candidates: []SourceInventoryAdvisoryCandidate{{
					Member: "Run",
					File:   "src/run.py",
					Line:   7,
				}},
			}},
		},
		SourceInventoryObservation: SourceInventoryObservation{
			Active: true,
			Scopes: []string{"src"},
			Sets: []SourceInventoryObservationSet{{
				Role:  AnswerCandidateRoleFunction,
				Count: 1,
				Members: []SourceInventoryObservationMember{{
					Name: "Run",
					File: "src/run.py",
					Line: 7,
				}},
			}},
		},
		SourceLocalization: &SourceLocalizationReview{
			Status:      SourceLocalizationObserved,
			SourcePaths: []string{"src/run.py"},
			EvidenceRefs: []WriteExplorationEvidenceRef{{
				ID:     "ev-run",
				Source: "src/run.py",
			}},
		},
	})
	first := m.TurnAArtifacts()
	first.ReadFiles[0] = "MUTATED.go"
	first.ReadFiles = append(first.ReadFiles, "c.go")
	first.ValidationBoundaryNotes[0] = "mutated boundary"
	first.MCPResponses[0].ResourceURI = "mcp://mutated"
	first.AcceptedAggregateFacts[0].Members[0] = "mutated.go:1"
	first.SourceInventoryAdvisory.Scopes[0] = "mutated"
	first.SourceInventoryAdvisory.Sets[0].Candidates[0].Member = "Mutated"
	first.SourceInventoryObservation.Scopes[0] = "mutated"
	first.SourceInventoryObservation.Sets[0].Members[0].Name = "Mutated"
	first.SourceLocalization.SourcePaths[0] = "mutated.py"
	first.SourceLocalization.EvidenceRefs[0].Source = "mutated.py"

	second := m.TurnAArtifacts()
	if second.ReadFiles[0] != "a.go" {
		t.Errorf("read-side mutation leaked back: %q", second.ReadFiles[0])
	}
	if len(second.ReadFiles) != 2 {
		t.Errorf("read-side append leaked back: len=%d", len(second.ReadFiles))
	}
	if second.ValidationBoundaryNotes[0] != "boundary 1" {
		t.Errorf("boundary-note mutation leaked back: %+v", second.ValidationBoundaryNotes)
	}
	if second.MCPResponses[0].ResourceURI != "mcp://fixture/trace/sleep-wakeup" {
		t.Errorf("mcp response mutation leaked back: %+v", second.MCPResponses)
	}
	if second.AcceptedAggregateFacts[0].Members[0] != "a.go:1" {
		t.Errorf("aggregate fact mutation leaked back: %+v", second.AcceptedAggregateFacts)
	}
	if second.SourceInventoryAdvisory.Scopes[0] != "src" ||
		second.SourceInventoryAdvisory.Sets[0].Candidates[0].Member != "Run" {
		t.Errorf("source-inventory advisory mutation leaked back: %+v", second.SourceInventoryAdvisory)
	}
	if second.SourceInventoryObservation.Scopes[0] != "src" ||
		second.SourceInventoryObservation.Sets[0].Members[0].Name != "Run" {
		t.Errorf("source-inventory observation mutation leaked back: %+v", second.SourceInventoryObservation)
	}
	if second.SourceLocalization == nil ||
		second.SourceLocalization.SourcePaths[0] != "src/run.py" ||
		second.SourceLocalization.EvidenceRefs[0].Source != "src/run.py" {
		t.Errorf("source-localization mutation leaked back: %+v", second.SourceLocalization)
	}
}

func TestTurnAArtifacts_Reset(t *testing.T) {
	m := NewMutableState("")
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

func TestTurnAArtifacts_ExploreForkMergeKeepsSiblingDeltas(t *testing.T) {
	parent := NewMutableState("")
	parent.SetTurnAArtifacts(TurnAArtifacts{
		UserQuestion:            "q",
		InvestigationNotes:      []string{"base-note"},
		ValidationBoundaryNotes: []string{"base-boundary"},
		ReadFiles:               []string{"base.go"},
		ToolResults:             []ToolResult{{ToolName: "grep", Summary: "base", Success: true}},
		MCPResponses:            []MCPResponse{{ServerName: "fixture", Summary: "base", Success: true}},
		SourceLocalization:      &SourceLocalizationReview{Status: SourceLocalizationObserved, SourcePaths: []string{"base.go"}},
		FlowFindings:            []FlowFindingDigest{{Path: []string{"base"}, Confidence: 0.5}},
		TerminalEvidenceCount:   1,
	})

	fork1 := parent.ForkForExploreDispatch()
	fork2 := parent.ForkForExploreDispatch()

	ta1 := fork1.TurnAArtifacts()
	ta1.InvestigationNotes = append(ta1.InvestigationNotes, "fork1-note")
	ta1.ValidationBoundaryNotes = append(ta1.ValidationBoundaryNotes, "fork1-boundary")
	ta1.ReadFiles = append(ta1.ReadFiles, "fork1.go")
	ta1.ToolResults = append(ta1.ToolResults, ToolResult{ToolName: "read_file", Summary: "fork1", Success: true})
	ta1.MCPResponses = append(ta1.MCPResponses, MCPResponse{ServerName: "fixture", Summary: "fork1", Success: true})
	ta1.SourceLocalization = MergeSourceLocalizationReviews(ta1.SourceLocalization, &SourceLocalizationReview{Status: SourceLocalizationObserved, SourcePaths: []string{"fork1.go"}})
	ta1.FlowFindings = append(ta1.FlowFindings, FlowFindingDigest{Path: []string{"fork1"}, Confidence: 0.7})
	ta1.TerminalEvidenceCount = 2
	fork1.SetTurnAArtifacts(*ta1)

	ta2 := fork2.TurnAArtifacts()
	ta2.InvestigationNotes = append(ta2.InvestigationNotes, "fork2-note")
	ta2.ValidationBoundaryNotes = append(ta2.ValidationBoundaryNotes, "fork2-boundary")
	ta2.ReadFiles = append(ta2.ReadFiles, "fork2.go")
	ta2.ToolResults = append(ta2.ToolResults, ToolResult{ToolName: "read_file", Summary: "fork2", Success: true})
	ta2.MCPResponses = append(ta2.MCPResponses, MCPResponse{ServerName: "fixture", Summary: "fork2", Success: true})
	ta2.SourceLocalization = MergeSourceLocalizationReviews(ta2.SourceLocalization, &SourceLocalizationReview{Status: SourceLocalizationObserved, SourcePaths: []string{"fork2.go"}})
	ta2.FlowFindings = append(ta2.FlowFindings, FlowFindingDigest{Path: []string{"fork2"}, Confidence: 0.8})
	ta2.TerminalEvidenceCount = 3
	fork2.SetTurnAArtifacts(*ta2)

	parent.MergeExploreFork(fork1)
	parent.MergeExploreFork(fork2)

	got := parent.TurnAArtifacts()
	if got == nil {
		t.Fatal("expected merged TurnAArtifacts")
	}
	if len(got.InvestigationNotes) != 3 {
		t.Fatalf("notes = %+v, want base + two fork deltas", got.InvestigationNotes)
	}
	if len(got.ValidationBoundaryNotes) != 3 {
		t.Fatalf("boundary notes = %+v, want base + two fork deltas", got.ValidationBoundaryNotes)
	}
	if len(got.ToolResults) != 3 {
		t.Fatalf("tool results = %+v, want base + two fork deltas", got.ToolResults)
	}
	if len(got.MCPResponses) != 3 {
		t.Fatalf("mcp responses = %+v, want base + two fork deltas", got.MCPResponses)
	}
	if len(got.FlowFindings) != 3 {
		t.Fatalf("flow findings = %+v, want base + two fork deltas", got.FlowFindings)
	}
	if got.TerminalEvidenceCount != 3 {
		t.Fatalf("terminal evidence count = %d, want max 3", got.TerminalEvidenceCount)
	}
	for _, want := range []string{"base.go", "fork1.go", "fork2.go"} {
		var found bool
		for _, file := range got.ReadFiles {
			if file == want {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("read files %+v missing %s", got.ReadFiles, want)
		}
	}
	for _, want := range []string{"base.go", "fork1.go", "fork2.go"} {
		var found bool
		if got.SourceLocalization != nil {
			for _, file := range got.SourceLocalization.SourcePaths {
				if file == want {
					found = true
					break
				}
			}
		}
		if !found {
			t.Fatalf("source localization %+v missing %s", got.SourceLocalization, want)
		}
	}
}

func TestTurnAArtifacts_ExploreForkMergePreservesSupersededClosureReasonAsNote(t *testing.T) {
	parent := NewMutableState("q")
	parent.SetTurnAArtifacts(TurnAArtifacts{
		InvestigationNotes:    []string{"base-note"},
		AcceptedClosureReason: "first closure carried rich VCS and current-source synthesis",
		AcceptedResultKind:    "resolved",
	})
	fork := parent.ForkForExploreDispatch()
	ta := fork.TurnAArtifacts()
	ta.InvestigationNotes = append(ta.InvestigationNotes, "fork-note")
	ta.AcceptedClosureReason = "second closure verified the implementation path"
	ta.AcceptedResultKind = "resolved"
	fork.SetTurnAArtifacts(*ta)

	parent.MergeExploreFork(fork)
	got := parent.TurnAArtifacts()
	if got == nil {
		t.Fatal("expected merged TurnAArtifacts")
	}
	if got.AcceptedClosureReason != ta.AcceptedClosureReason {
		t.Fatalf("current closure remains authoritative, got %q", got.AcceptedClosureReason)
	}
	joined := strings.Join(got.InvestigationNotes, "\n")
	if !strings.Contains(joined, "first closure carried rich VCS") ||
		!strings.Contains(joined, "preserved advisory, not a citation") {
		t.Fatalf("superseded closure reason should survive as advisory note, got %+v", got.InvestigationNotes)
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
	m := NewMutableState("")
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

func TestHypothesisVerdictBuffer_LastWriteWinsByID(t *testing.T) {
	m := NewMutableState("")
	m.AppendEmittedHypothesisVerdicts([]HypothesisVerdict{
		{HypothesisID: "H1", Status: HypInconclusive, Rationale: "early"},
		{HypothesisID: "H2", Status: HypConfirmed, Rationale: "stable"},
	})
	m.AppendEmittedHypothesisVerdicts([]HypothesisVerdict{
		{HypothesisID: "H1", Status: HypRejected, Rationale: "final", Citation: "a.go:10"},
	})

	got := m.EmittedHypothesisVerdicts()
	if len(got) != 2 {
		t.Fatalf("verdict buffer should dedupe by ID, got %d items: %+v", len(got), got)
	}
	if got[0].HypothesisID != "H1" || got[0].Status != HypRejected || got[0].Citation != "a.go:10" {
		t.Errorf("H1 should be overwritten in place by the last verdict, got %+v", got[0])
	}
	if got[1].HypothesisID != "H2" || got[1].Status != HypConfirmed {
		t.Errorf("H2 should remain unchanged, got %+v", got[1])
	}
}

// TestCachedLabelSupportTokens_Memoises confirms #10: the helper
// computes once per Set / Reset of turnAArtifacts and reuses the
// cached map on subsequent calls. Build closure invocation counts
// pin the memoisation contract.
func TestCachedLabelSupportTokens_Memoises(t *testing.T) {
	m := NewMutableState("test")
	calls := 0
	build := func(items []EvidenceItem) map[string]struct{} {
		calls++
		out := make(map[string]struct{}, len(items))
		for _, it := range items {
			if it.AnchorSymbol != "" {
				out[it.AnchorSymbol] = struct{}{}
			}
		}
		return out
	}

	// 1. No turn-A snapshot yet → returns nil, no build.
	if got := m.CachedLabelSupportTokens(build); got != nil {
		t.Fatalf("expected nil before SetTurnAArtifacts; got %v", got)
	}
	if calls != 0 {
		t.Fatalf("build should not be called when snapshot is nil; calls=%d", calls)
	}

	// 2. Seed snapshot. First call computes; second call hits cache.
	m.SetTurnAArtifacts(TurnAArtifacts{
		EvidenceItems: []EvidenceItem{{AnchorSymbol: "Foo"}, {AnchorSymbol: "Bar"}},
	})
	first := m.CachedLabelSupportTokens(build)
	if calls != 1 {
		t.Fatalf("first call must invoke build; calls=%d", calls)
	}
	if _, ok := first["Foo"]; !ok {
		t.Fatalf("expected Foo in support pool; got %v", first)
	}
	second := m.CachedLabelSupportTokens(build)
	if calls != 1 {
		t.Fatalf("second call must hit cache (no build invocation); calls=%d", calls)
	}
	// Pointer-equal map → same shared instance.
	if &first == &second {
		// addresses always differ for local map vars; instead check by ptr-of-len reflection.
	}

	// 3. Reset invalidates the cache.
	m.ResetTurnAArtifacts()
	if got := m.CachedLabelSupportTokens(build); got != nil {
		t.Fatalf("expected nil after Reset; got %v", got)
	}

	// 4. New SetTurnAArtifacts → cache rebuilt, fresh content.
	m.SetTurnAArtifacts(TurnAArtifacts{
		EvidenceItems: []EvidenceItem{{AnchorSymbol: "Baz"}},
	})
	got := m.CachedLabelSupportTokens(build)
	if calls != 2 {
		t.Fatalf("after Set with new items, build must run again; calls=%d", calls)
	}
	if _, ok := got["Baz"]; !ok {
		t.Fatalf("expected Baz after re-Set; got %v", got)
	}
	if _, ok := got["Foo"]; ok {
		t.Fatalf("stale Foo from previous snapshot must not survive; got %v", got)
	}

	// 5. nil build → nil result, no panic.
	if got := m.CachedLabelSupportTokens(nil); got != nil {
		t.Fatalf("nil build must yield nil result; got %v", got)
	}
}
