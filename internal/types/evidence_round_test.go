package types

import "testing"

func TestEvidenceClosureIngestRoundReadCoverageAndAcceptedEvidence(t *testing.T) {
	c := NewEvidenceClosure("")
	results := []ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[./a.go: showing lines 2-5 of 10 total]\ncode",
		},
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[forced_read] [b.go: showing all 3 lines (90 bytes); limit=1 expanded]\ncode",
		},
		{
			ToolName: "emit_evidence",
			Success:  true,
			Handoff: &ToolHandoffCarrier{
				Version:          ToolHandoffCarrierVersion,
				ToolName:         "emit_evidence",
				AcceptedEvidence: []AcceptedEvidenceRef{{ID: "ev-1", Source: "a.go", LineStart: 2}},
			},
		},
	}

	delta := c.IngestRound(results, "")
	if delta.Empty() {
		t.Fatal("expected non-empty delta")
	}
	if !delta.ReadSet["a.go"] || !delta.ReadSet["b.go"] {
		t.Fatalf("delta read set = %+v", delta.ReadSet)
	}
	if got := c.ReadRanges("a.go"); len(got) != 1 || got[0].Start != 2 || got[0].End != 5 {
		t.Fatalf("a.go ranges = %+v", got)
	}
	if got := c.FileTotalLines("b.go"); got != 3 {
		t.Fatalf("b.go total = %d, want 3", got)
	}
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-1" {
		t.Fatalf("accepted evidence refs = %+v", refs)
	}
}

func TestEvidenceClosureIngestReducerInputTurnAHandoffSnapshot(t *testing.T) {
	c := NewEvidenceClosure("")
	observation := evidenceRoundTestSourceInventoryObservation("Run")
	input := EvidenceReducerInput{
		Class: EvidenceReducerInputTurnAHandoffSnapshot,
		HandoffCarriers: []ToolHandoffCarrier{{
			Version:  ToolHandoffCarrierVersion,
			ToolName: "emit_evidence",
			AcceptedEvidence: []AcceptedEvidenceRef{{
				ID:        "ev-carrier",
				Source:    "a.go",
				LineStart: 3,
				Subject:   "A",
			}},
		}},
		EvidenceItems: []EvidenceItem{{
			ID:        "ev-item",
			Source:    "b.go",
			LineStart: 7,
			Subject:   "B",
		}},
		SourceInventoryObservation: observation,
	}

	delta := c.IngestEvidenceReducerInput(input, "")
	if delta.Empty() || len(delta.AcceptedEvidence) != 2 || !delta.SourceInventoryObservation.IsActive() {
		t.Fatalf("turn-a reducer delta = %+v, want accepted evidence and source inventory", delta)
	}
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 2 {
		t.Fatalf("accepted evidence refs = %+v, want 2", refs)
	}
	if got := c.SourceInventoryObservation(); !got.IsActive() || len(got.Sets) != 1 || got.Sets[0].Members[0].Name != "Run" {
		t.Fatalf("source inventory observation not ingested: %+v", got)
	}

	c.IngestEvidenceReducerInput(input, "")
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 2 {
		t.Fatalf("repeated turn-a ingest must be idempotent, refs=%+v", refs)
	}
}

func TestEvidenceClosureIngestReducerInputStageEvidenceSnapshot(t *testing.T) {
	c := NewEvidenceClosure("")
	delta := c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class: EvidenceReducerInputStageEvidenceSnapshot,
		EvidenceItems: []EvidenceItem{{
			ID:        "ev-stage",
			Source:    "stage.go",
			LineStart: 4,
			Subject:   "Stage",
		}},
	}, "")
	if delta.Empty() || len(delta.AcceptedEvidence) != 1 || delta.AcceptedEvidence[0].ID != "ev-stage" {
		t.Fatalf("stage evidence reducer delta = %+v", delta)
	}
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-stage" {
		t.Fatalf("stage evidence refs = %+v", refs)
	}
}

func TestEvidenceClosureIngestReducerInputReadCoverageDeltaAdds(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{"old.go": true})
	c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class:   EvidenceReducerInputReadCoverageDelta,
		ReadSet: map[string]bool{"new.go": true},
		ReadRanges: map[string][]LineRange{
			"new.go": {{Start: 3, End: 5}},
		},
		FileTotalLines: map[string]int{"new.go": 9},
	}, "")

	if got := c.ReadSet(); !got["old.go"] || !got["new.go"] {
		t.Fatalf("read coverage delta should add without replacing: %+v", got)
	}
	if ranges := c.ReadRanges("new.go"); len(ranges) != 1 || ranges[0].Start != 3 || ranges[0].End != 5 {
		t.Fatalf("new.go ranges = %+v", ranges)
	}
	if total := c.FileTotalLines("new.go"); total != 9 {
		t.Fatalf("new.go total = %d, want 9", total)
	}
}

func TestEvidenceClosureIngestReducerInputStageCoverageSnapshotReplaces(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{"old.go": true})
	c.SetReadRanges(map[string][]LineRange{"old.go": {{Start: 1, End: 2}}})
	c.SetFileTotalLines(map[string]int{"old.go": 2})

	c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class:   EvidenceReducerInputStageCoverageSnapshot,
		ReadSet: map[string]bool{"fresh.go": true},
		ReadRanges: map[string][]LineRange{
			"fresh.go": {{Start: 10, End: 20}},
		},
		FileTotalLines:        map[string]int{"fresh.go": 30},
		ReplaceReadSet:        true,
		ReplaceReadRanges:     true,
		ReplaceFileTotalLines: true,
	}, "")

	if got := c.ReadSet(); len(got) != 1 || !got["fresh.go"] {
		t.Fatalf("stage coverage snapshot should replace read set: %+v", got)
	}
	if old := c.ReadRanges("old.go"); len(old) != 0 {
		t.Fatalf("old.go ranges should be replaced, got %+v", old)
	}
	if ranges := c.ReadRanges("fresh.go"); len(ranges) != 1 || ranges[0].Start != 10 || ranges[0].End != 20 {
		t.Fatalf("fresh.go ranges = %+v", ranges)
	}
	if total := c.FileTotalLines("fresh.go"); total != 30 {
		t.Fatalf("fresh.go total = %d, want 30", total)
	}
}

func TestEvidenceClosureIngestReducerInputForkClosureDelta(t *testing.T) {
	parent := NewEvidenceClosure("")
	fork := NewEvidenceClosure("")
	fork.SetReadSet(map[string]bool{"fork.go": true})
	fork.AppendAcceptedEvidenceRefs([]AcceptedEvidenceRef{{ID: "ev-fork", Source: "fork.go", LineStart: 1}})
	fork.RecordSourceInventoryObservation(evidenceRoundTestSourceInventoryObservation("Fork"))

	parent.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class:       EvidenceReducerInputForkClosureDelta,
		ForkClosure: fork,
	}, "")

	if got := parent.ReadSet(); len(got) != 1 || !got["fork.go"] {
		t.Fatalf("fork closure read set not merged: %+v", got)
	}
	if refs := parent.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-fork" {
		t.Fatalf("fork accepted refs not merged: %+v", refs)
	}
	if got := parent.SourceInventoryObservation(); !got.IsActive() || got.Sets[0].Members[0].Name != "Fork" {
		t.Fatalf("fork source inventory not merged: %+v", got)
	}
}

func TestMutableSetTurnAArtifactsProjectsClosureThroughReducer(t *testing.T) {
	mut := NewMutableState("q")
	mut.SetTurnAArtifacts(TurnAArtifacts{
		EvidenceItems: []EvidenceItem{{
			ID:        "ev-turn-a",
			Source:    "internal/app.go",
			LineStart: 11,
			Subject:   "App",
		}},
		SourceInventoryObservation: evidenceRoundTestSourceInventoryObservation("App"),
	})

	closure := mut.EvidenceClosure()
	if refs := closure.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-turn-a" {
		t.Fatalf("turn-a accepted evidence was not projected through reducer: %+v", refs)
	}
	if got := closure.SourceInventoryObservation(); !got.IsActive() || got.Sets[0].Members[0].Name != "App" {
		t.Fatalf("turn-a source inventory was not projected through reducer: %+v", got)
	}

	mut.SetTurnAArtifacts(*mut.TurnAArtifacts())
	if refs := closure.AcceptedEvidenceRefs(); len(refs) != 1 {
		t.Fatalf("repeated SetTurnAArtifacts must not duplicate accepted refs: %+v", refs)
	}
}

func TestMutableSourceInventoryObservationSetterProjectsThroughReducer(t *testing.T) {
	mut := NewMutableState("q")
	observation := evidenceRoundTestSourceInventoryObservation("Build")
	mut.SetSourceInventoryObservation(observation)
	mut.SetSourceInventoryObservation(observation)

	got := mut.EvidenceClosure().SourceInventoryObservation()
	if !got.IsActive() || len(got.Sets) != 1 {
		t.Fatalf("source inventory observation not projected: %+v", got)
	}
	if members := got.Sets[0].Members; len(members) != 1 || members[0].Name != "Build" {
		t.Fatalf("source inventory projection not idempotent: %+v", got.Sets[0])
	}
}

func TestEvidenceRoundDeltaMatchesExtractReadCoverage(t *testing.T) {
	results := []ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 1-2 of 5 total]\n"},
		{ToolName: "read_file", Success: false, Summary: "[ignored.go: showing lines 1-2 of 5 total]\n"},
	}
	readSet, readRanges, totals := ExtractReadCoverage(results, "")
	delta := EvidenceRoundDeltaFromToolResults(results, "")
	if len(delta.ReadSet) != len(readSet) || !delta.ReadSet["a.go"] {
		t.Fatalf("delta read set = %+v, want %+v", delta.ReadSet, readSet)
	}
	if len(delta.ReadRanges["a.go"]) != len(readRanges["a.go"]) ||
		delta.ReadRanges["a.go"][0] != readRanges["a.go"][0] {
		t.Fatalf("delta ranges = %+v, want %+v", delta.ReadRanges, readRanges)
	}
	if delta.FileTotalLines["a.go"] != totals["a.go"] {
		t.Fatalf("delta totals = %+v, want %+v", delta.FileTotalLines, totals)
	}
}

func evidenceRoundTestSourceInventoryObservation(name string) SourceInventoryObservation {
	return SourceInventoryObservation{
		Active:   true,
		Complete: true,
		Scopes:   []string{"src"},
		Sets: []SourceInventoryObservationSet{{
			Role:     AnswerCandidateRoleFunction,
			Complete: true,
			Members: []SourceInventoryObservationMember{{
				Name: name,
				Key:  name,
				Role: AnswerCandidateRoleFunction,
				File: "src/app.go",
				Line: 11,
			}},
		}},
	}
}
