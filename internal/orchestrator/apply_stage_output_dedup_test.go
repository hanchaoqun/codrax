package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/types"
)

// This file is a characterization + fix lock for the
// applyStageOutput accumulation bug documented in
// memory/project_applystage_dedup.md.
//
// The issue: applyStageOutput uses plain `append` for every
// StageOutput slice (ToolResults, RepoFacts, EvidenceItems,
// FlowFindings, AnswerChains, AnswerSymbols, MCPResponses). When
// the orchestrator re-dispatches explore on a self-loop, the
// explorer's ParseOutput recomputes its output from the
// accumulated investigationNotes and returns the FULL current
// snapshot — so the second run's output contains everything from
// the first run plus anything new. applyStageOutput then appends
// that full snapshot on top of the first run's already-stored
// items, producing near-2x (and on subsequent runs, near-Nx)
// duplication in the busCtx slices that downstream prompt
// builders render verbatim.
//
// This test drives the bug explicitly by calling applyStageOutput
// twice with StageOutputs that simulate explorer self-loop:
// second run contains first run's items + new items. The
// assertions lock the EXPECTED post-fix behavior — four slices
// that should dedup (EvidenceItems, FlowFindings, AnswerChains,
// AnswerSymbols), and the three history-style slices that SHOULD
// still append (ToolResults, MCPResponses, RepoFacts) per the
// design note in the audit doc.

func TestApplyStageOutput_DedupsAnswerChainsOnSelfLoop(t *testing.T) {

	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}

	// First explore run: two chains produced.
	chain1 := types.AnswerChain{
		Item: types.EvidenceItem{
			Kind:    types.EvidenceDataflowPath,
			Summary: "`RegisterDefaultSubAgents()` binds ONLY NewSubExplorer(deps) → `SubExplorer.Name()` returns \"explorer\"",
		},
		Score:    2.0,
		StrictOK: true,
	}
	chain2 := types.AnswerChain{
		Item: types.EvidenceItem{
			Kind:    types.EvidenceDataflowPath,
			Summary: "`NewSubAgentValidator()` returns &SubAgentValidator{ → `SubAgentValidator.Validate()` assigns st := range proposal.SubTasks {",
		},
		Score:    1.5,
		StrictOK: true,
	}
	first := &agent.StageOutput{
		AnswerChains: []types.AnswerChain{chain1, chain2},
	}
	o.applyStageOutput(first)
	if got := len(o.busCtx.AnswerChains); got != 2 {
		t.Fatalf("after first explore: AnswerChains len = %d, want 2", got)
	}

	// Second explore run (self-loop). ParseOutput recomputes from the
	// cumulative investigationNotes, so the second StageOutput contains
	// BOTH first-run chains PLUS a third new one.
	chain3 := types.AnswerChain{
		Item: types.EvidenceItem{
			Kind:    types.EvidenceDataflowPath,
			Summary: "`NewSubAgentRuntime()` returns &SubAgentRuntime{",
		},
		Score:    1.0,
		StrictOK: true,
	}
	second := &agent.StageOutput{
		AnswerChains: []types.AnswerChain{chain1, chain2, chain3},
	}
	o.applyStageOutput(second)

	if got := len(o.busCtx.AnswerChains); got != 3 {
		t.Errorf("after self-loop: AnswerChains len = %d, want 3 (two dedup'd + one new). "+
			"A len of 5 means plain append with no dedup — the accumulation bug. Got: %v",
			got, o.busCtx.AnswerChains)
	}
}

func TestEvidenceRoundIngestUpdatesProductionClosure(t *testing.T) {
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mu := types.NewMutableState("question")
	o.busCtx = &types.BusContext{
		RepoRoot:      "",
		Mutable:       mu,
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}
	results := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[a.go: showing lines 1-3 of 10 total]\ncode",
			ReadCoverage: &types.ToolReadCoverage{
				Path:       "a.go",
				LineStart:  1,
				LineEnd:    3,
				TotalLines: 10,
			},
		},
		{
			ToolName: "emit_evidence",
			Success:  true,
			Handoff: &types.ToolHandoffCarrier{
				Version:          types.ToolHandoffCarrierVersion,
				ToolName:         "emit_evidence",
				AcceptedEvidence: []types.AcceptedEvidenceRef{{ID: "ev-a", Source: "a.go", LineStart: 1}},
			},
		},
	}

	delta := o.ingestEvidenceRound(results)
	if delta.Empty() || !delta.ReadSet["a.go"] {
		t.Fatalf("ingest delta = %+v, want a.go read coverage", delta)
	}
	if got := mu.EvidenceClosure().ReadSet(); !got["a.go"] || len(got) != 1 {
		t.Fatalf("production closure read set = %+v, want a.go", got)
	}
	if refs := mu.EvidenceClosure().AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-a" {
		t.Fatalf("production accepted evidence refs = %+v, want ev-a", refs)
	}

	o.applyStageOutput(&agent.StageOutput{ToolResults: results})
	if got := len(o.busCtx.ToolResults); got != len(results) {
		t.Fatalf("ToolResults history len = %d, want %d", got, len(results))
	}
	if got := mu.EvidenceClosure().ReadSet(); !got["a.go"] || len(got) != 1 {
		t.Fatalf("applyStageOutput production read set = %+v, want stable a.go", got)
	}
	if refs := mu.EvidenceClosure().AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-a" {
		t.Fatalf("applyStageOutput accepted evidence refs = %+v, want stable ev-a", refs)
	}
}

func TestApplyStageOutput_MergesAdvisoryInvestigationNotesIntoTurnA(t *testing.T) {
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		Mutable:       types.NewMutableState("question"),
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}
	o.busCtx.Mutable.SetTurnAArtifacts(types.TurnAArtifacts{
		InvestigationNotes: []string{"existing advisory"},
		ReadFiles:          []string{"a.go"},
	})

	o.applyStageOutput(&agent.StageOutput{
		InvestigationNotes: []string{
			"sub-agent advisory",
			"sub-agent advisory",
			"  ",
		},
	})

	ta := o.busCtx.Mutable.TurnAArtifacts()
	if ta == nil {
		t.Fatal("TurnAArtifacts missing")
	}
	if got, want := len(ta.InvestigationNotes), 2; got != want {
		t.Fatalf("InvestigationNotes len=%d, want %d: %+v", got, want, ta.InvestigationNotes)
	}
	if ta.InvestigationNotes[0] != "existing advisory" || ta.InvestigationNotes[1] != "sub-agent advisory" {
		t.Fatalf("InvestigationNotes not merged in order: %+v", ta.InvestigationNotes)
	}
	if got := len(ta.ReadFiles); got != 1 || ta.ReadFiles[0] != "a.go" {
		t.Fatalf("existing TurnA fields were not preserved: %+v", ta.ReadFiles)
	}
}

func TestApplyStageOutput_DedupsAnswerSymbolsOnSelfLoop(t *testing.T) {

	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}

	sym1 := types.AnswerSymbol{Name: "explorer", File: "internal/agent/sub_explorer.go", Line: 32}
	sym2 := types.AnswerSymbol{Name: "SubAgentRuntime", File: "internal/agent/subagent_runtime.go", Line: 40}

	o.applyStageOutput(&agent.StageOutput{AnswerSymbols: []types.AnswerSymbol{sym1, sym2}})
	if got := len(o.busCtx.AnswerSymbols); got != 2 {
		t.Fatalf("after first explore: AnswerSymbols len = %d, want 2", got)
	}

	// Self-loop: same sym1 + sym2 re-emitted, plus a new sym3.
	sym3 := types.AnswerSymbol{Name: "SubExplorer", File: "internal/agent/sub_explorer.go", Line: 20}
	o.applyStageOutput(&agent.StageOutput{AnswerSymbols: []types.AnswerSymbol{sym1, sym2, sym3}})

	if got := len(o.busCtx.AnswerSymbols); got != 3 {
		t.Errorf("after self-loop: AnswerSymbols len = %d, want 3 (two dedup'd + one new). "+
			"Got: %+v", got, o.busCtx.AnswerSymbols)
	}
}

func TestApplyStageOutput_DedupsEvidenceItemsByID(t *testing.T) {

	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}

	ev1 := types.EvidenceItem{
		Kind: types.EvidenceConcrete, Predicate: "binds ONLY",
		Subject: "RegisterDefaultSubAgents", Object: "NewSubExplorer(deps)",
		Source: "internal/agent/subagent.go", LineStart: 63,
		Scope: types.ScopeLine,
	}
	ev2 := types.EvidenceItem{
		Kind: types.EvidenceConcrete, Predicate: "returns",
		Subject: "SubExplorer.Name", Object: "\"explorer\"",
		Source: "internal/agent/sub_explorer.go", LineStart: 32,
		Scope: types.ScopeLine,
	}
	// Stamp IDs as ensureStructuredEvidence would.
	ev1.ID = types.StableEvidenceID(ev1)
	ev2.ID = types.StableEvidenceID(ev2)

	o.applyStageOutput(&agent.StageOutput{EvidenceItems: []types.EvidenceItem{ev1, ev2}})

	// Self-loop: same ev1/ev2 + a new ev3 with different ID.
	ev3 := types.EvidenceItem{
		Kind: types.EvidenceRelationship, Predicate: "calls",
		Subject: "Orchestrator", Object: "SubAgentRuntime",
		Source: "internal/orchestrator/orchestrator.go", LineStart: 100,
		Scope: types.ScopeLine,
	}
	ev3.ID = types.StableEvidenceID(ev3)

	o.applyStageOutput(&agent.StageOutput{EvidenceItems: []types.EvidenceItem{ev1, ev2, ev3}})

	if got := len(o.busCtx.EvidenceItems); got != 3 {
		t.Errorf("after self-loop: EvidenceItems len = %d, want 3 (two dedup'd by ID + one new). Got: %+v",
			got, o.busCtx.EvidenceItems)
	}
}

func TestApplyStageOutput_IngestsEvidenceItemsIntoMutableOnce(t *testing.T) {
	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	mut := types.NewMutableState("question")
	o.busCtx = &types.BusContext{
		Mutable:       mut,
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}

	ev1 := types.EvidenceItem{
		Kind: types.EvidenceConcrete, Predicate: "binds",
		Subject: "A", Object: "B",
		Source: "a.go", LineStart: 1,
		Scope: types.ScopeLine,
	}
	ev2 := types.EvidenceItem{
		Kind: types.EvidenceConcrete, Predicate: "calls",
		Subject: "B", Object: "C",
		Source: "b.go", LineStart: 2,
		Scope: types.ScopeLine,
	}
	ev1.ID = types.StableEvidenceID(ev1)
	ev2.ID = types.StableEvidenceID(ev2)

	o.applyStageOutput(&agent.StageOutput{EvidenceItems: []types.EvidenceItem{ev1}})
	o.applyStageOutput(&agent.StageOutput{EvidenceItems: []types.EvidenceItem{ev1, ev2}})
	o.applyStageOutput(&agent.StageOutput{EvidenceItems: []types.EvidenceItem{ev1, ev2}})

	if got := len(o.busCtx.EvidenceItems); got != 2 {
		t.Fatalf("bus evidence len = %d, want 2", got)
	}
	if got := len(mut.EmittedEvidence()); got != 2 {
		t.Fatalf("mutable evidence len = %d, want 2", got)
	}
}

// History-style slices (ToolResults, MCPResponses, RepoFacts)
// should CONTINUE appending even across self-loops. They are
// per-call logs, not deduplicable "truth" sets. This test locks
// that intent so a future "dedup everything" patch doesn't
// accidentally erase useful tool-call history.
func TestApplyStageOutput_KeepsAppendingToolResultsOnSelfLoop(t *testing.T) {

	ar, sr, sar := buildRegistries(nil)
	o := New(types.PipelineSettings{}, ar, sr, sar)
	o.busCtx = &types.BusContext{
		PipelineStage: types.StageExplore,
		ActiveAgent:   types.AgentExplorer,
		TaskState:     types.TaskState{Stage: types.StageExplore},
	}

	tr := types.ToolResult{ToolName: "grep", Success: true, Summary: "internal/agent/subagent.go"}

	o.applyStageOutput(&agent.StageOutput{ToolResults: []types.ToolResult{tr}})
	o.applyStageOutput(&agent.StageOutput{ToolResults: []types.ToolResult{tr}})

	if got := len(o.busCtx.ToolResults); got != 2 {
		t.Errorf("ToolResults should APPEND across self-loops (not dedup): want 2, got %d", got)
	}
}
