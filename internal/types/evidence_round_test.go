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

func TestEvidenceClosureIngestReducerInputNodeArtifactDelta(t *testing.T) {
	c := NewEvidenceClosure("")
	delta := c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class: EvidenceReducerInputNodeArtifactDelta,
		NodeArtifacts: []NodeArtifactRecord{
			{
				ProducerNodeID: "explore",
				ProducerStage:  StageExplore,
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-node"},
			},
			{
				ProducerNodeID: "explore",
				ProducerStage:  StageExplore,
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-node"},
			},
			{
				ProducerNodeID: "",
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "drop"},
			},
		},
	}, "")
	if delta.Empty() || len(delta.NodeArtifacts) != 1 {
		t.Fatalf("node artifact delta = %+v, want one normalized artifact", delta)
	}
	if got := c.NodeArtifactRecords(); len(got) != 1 || got[0].ProducerNodeID != "explore" {
		t.Fatalf("closure node artifacts = %+v", got)
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

func TestEvidenceClosureIngestReducerInputReadRunSnapshotSeed(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{"old.go": true})
	c.SetNodeExecStatus("explore", NodeExecFailed)
	c.SetNodeExecAttempts(map[string]int{"explore": 3})
	observation := evidenceRoundTestSourceInventoryObservation("Resume")
	decision := ProgressDecision{
		ShouldReplan: true,
		ReasonCode:   ProgressReasonContinue,
		Delta: ProgressDelta{
			Kind:          ProgressDeltaDowngradeBlocker,
			DowngradeLane: DowngradeLaneContractChain,
			BlockerKey:    17,
			Consecutive:   2,
		},
	}

	delta := c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class: EvidenceReducerInputReadRunSnapshotSeed,
		NodeStatuses: map[string]NodeExecStatus{
			"explore": NodeExecDone,
			"extract": NodeExecPending,
		},
		NodeAttempts: map[string]int{
			"explore": 2,
			"extract": 1,
		},
		NodeArtifacts: []NodeArtifactRecord{{
			ProducerNodeID: "explore",
			ProducerStage:  StageExplore,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-resume"},
		}},
		ReadSet: map[string]bool{"fresh.go": true},
		ReadRanges: map[string][]LineRange{
			"fresh.go": {{Start: 3, End: 8}},
		},
		FileTotalLines:             map[string]int{"fresh.go": 20},
		ReplaceReadSet:             true,
		ReplaceReadRanges:          true,
		ReplaceFileTotalLines:      true,
		AcceptedEvidence:           []AcceptedEvidenceRef{{ID: "ev-resume", Source: "fresh.go", LineStart: 4}},
		SourceInventoryObservation: observation,
		ProgressDecision:           decision,
		HasProgressDecision:        true,
	}, "")

	if delta.Empty() || len(delta.NodeStatuses) != 2 || len(delta.NodeAttempts) != 2 || len(delta.NodeArtifacts) != 1 || !delta.HasProgressDecision {
		t.Fatalf("snapshot seed delta = %+v, want node/progress carriers", delta)
	}
	if got := c.ReadSet(); len(got) != 1 || !got["fresh.go"] {
		t.Fatalf("snapshot seed should replace read set: %+v", got)
	}
	if got := c.NodeExecStatus("explore"); got != NodeExecDone {
		t.Fatalf("explore status = %s, want done", got)
	}
	if got := c.NodeExecStatus("extract"); got != NodeExecPending {
		t.Fatalf("extract status = %s, want pending", got)
	}
	if got := c.NodeExecAttempt("explore"); got != 2 {
		t.Fatalf("explore attempts = %d, want 2", got)
	}
	if got := c.NodeExecAttempt("extract"); got != 1 {
		t.Fatalf("extract attempts = %d, want 1", got)
	}
	if got := c.NodeArtifactRecords(); len(got) != 1 || got[0].ProducerNodeID != "explore" {
		t.Fatalf("node artifacts = %+v", got)
	}
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-resume" {
		t.Fatalf("accepted evidence refs = %+v", refs)
	}
	if got := c.LatestProgressDecision(); !got.ShouldReplan || got.ReasonCode != ProgressReasonContinue || got.Delta.BlockerKey != 17 {
		t.Fatalf("latest progress decision = %+v", got)
	}
	if got := c.SourceInventoryObservation(); !got.IsActive() || got.Sets[0].Members[0].Name != "Resume" {
		t.Fatalf("source inventory not ingested: %+v", got)
	}

	c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class: EvidenceReducerInputReadRunSnapshotSeed,
		NodeArtifacts: []NodeArtifactRecord{{
			ProducerNodeID: "explore",
			ProducerStage:  StageExplore,
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-resume"},
		}},
		AcceptedEvidence: []AcceptedEvidenceRef{{ID: "ev-resume", Source: "fresh.go", LineStart: 4}},
	}, "")
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 1 {
		t.Fatalf("repeated snapshot seed must not duplicate accepted refs: %+v", refs)
	}
	if got := c.NodeArtifactRecords(); len(got) != 1 {
		t.Fatalf("repeated snapshot seed must not duplicate node artifacts: %+v", got)
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

func TestEvidenceReducerOwnershipBehaviorReadRunSnapshotSeedConflicts(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{"old.go": true})
	c.SetReadRanges(map[string][]LineRange{"old.go": {{Start: 1, End: 2}}})
	c.RecordFileTotalLines("old.go", 200)
	c.SetNodeExecStatus("explore", NodeExecFailed)
	c.SetNodeExecStatus("untouched", NodeExecDone)
	c.SetNodeExecAttempts(map[string]int{"old": 7, "explore": 3})
	c.SetLatestProgressDecision(ProgressDecision{
		ShouldReplan: true,
		ReasonCode:   ProgressReasonContinue,
		Delta:        ProgressDelta{Kind: ProgressDeltaDowngradeBlocker, BlockerKey: 11},
	})
	c.AppendNodeArtifactRecords([]NodeArtifactRecord{{
		ProducerNodeID: "old",
		ProducerStage:  StageExplore,
		Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-old"},
	}})

	c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class: EvidenceReducerInputReadRunSnapshotSeed,
		NodeStatuses: map[string]NodeExecStatus{
			"explore": NodeExecDone,
		},
		NodeAttempts: map[string]int{"explore": 1},
		NodeArtifacts: []NodeArtifactRecord{
			{
				ProducerNodeID: "explore",
				ProducerStage:  StageExplore,
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-fresh"},
			},
			{
				ProducerNodeID: "explore",
				ProducerStage:  StageExplore,
				Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "ev-fresh"},
			},
		},
		ReadSet:               map[string]bool{"fresh.go": true},
		ReadRanges:            map[string][]LineRange{"fresh.go": {{Start: 3, End: 5}}},
		FileTotalLines:        map[string]int{"fresh.go": 40},
		ReplaceReadSet:        true,
		ReplaceReadRanges:     true,
		ReplaceFileTotalLines: true,
		ProgressDecision: ProgressDecision{
			ShouldReplan: false,
			ReasonCode:   ProgressReasonConverged,
			Delta:        ProgressDelta{Kind: ProgressDeltaDowngradeBlocker, BlockerKey: 99},
		},
		HasProgressDecision: true,
	}, "")

	if got := c.ReadSet(); len(got) != 1 || !got["fresh.go"] {
		t.Fatalf("snapshot seed should replace read set: %+v", got)
	}
	if got := c.ReadRanges("old.go"); len(got) != 0 {
		t.Fatalf("snapshot seed should replace read ranges, old.go=%+v", got)
	}
	if got := c.FileTotalLines("old.go"); got != 0 {
		t.Fatalf("snapshot seed should replace file totals, old.go total=%d", got)
	}
	if got := c.NodeExecStatus("explore"); got != NodeExecDone {
		t.Fatalf("node status latest-authority failed, got %s", got)
	}
	if got := c.NodeExecStatus("untouched"); got != NodeExecDone {
		t.Fatalf("snapshot seed must not erase statuses outside its typed payload, got %s", got)
	}
	if attempts := c.NodeExecAttempts(); len(attempts) != 1 || attempts["explore"] != 1 {
		t.Fatalf("snapshot seed should replace node attempts map: %+v", attempts)
	}
	if got := c.LatestProgressDecision(); got.ReasonCode != ProgressReasonConverged || got.Delta.BlockerKey != 99 || got.ShouldReplan {
		t.Fatalf("progress decision latest-authority failed: %+v", got)
	}
	if artifacts := c.NodeArtifactRecords(); len(artifacts) != 2 || !nodeArtifactRecordKeysContain(artifacts, "produced\x00evidence_item\x00ev-old\x00old") || !nodeArtifactRecordKeysContain(artifacts, "produced\x00evidence_item\x00ev-fresh\x00explore") {
		t.Fatalf("node artifacts should use normalized identity dedup without dropping existing refs: %+v", artifacts)
	}
}

func TestEvidenceReducerOwnershipBehaviorForkClosureDeltaStableAndIdempotent(t *testing.T) {
	forkA := evidenceRoundOwnershipFork("a.go", "ev-a", "MemberA", "node-a", 1)
	forkB := evidenceRoundOwnershipFork("b.go", "ev-b", "MemberB", "node-b", 3)

	parentAB := NewEvidenceClosure("")
	parentAB.IngestEvidenceReducerInput(EvidenceReducerInput{Class: EvidenceReducerInputForkClosureDelta, ForkClosure: forkA}, "")
	parentAB.IngestEvidenceReducerInput(EvidenceReducerInput{Class: EvidenceReducerInputForkClosureDelta, ForkClosure: forkA}, "")
	parentAB.IngestEvidenceReducerInput(EvidenceReducerInput{Class: EvidenceReducerInputForkClosureDelta, ForkClosure: forkB}, "")

	parentBA := NewEvidenceClosure("")
	parentBA.IngestEvidenceReducerInput(EvidenceReducerInput{Class: EvidenceReducerInputForkClosureDelta, ForkClosure: forkB}, "")
	parentBA.IngestEvidenceReducerInput(EvidenceReducerInput{Class: EvidenceReducerInputForkClosureDelta, ForkClosure: forkA}, "")

	assertReadSetKeys(t, parentAB.ReadSet(), "a.go", "b.go")
	assertReadSetKeys(t, parentBA.ReadSet(), "a.go", "b.go")
	assertAcceptedEvidenceKeys(t, parentAB.AcceptedEvidenceRefs(), "ev-a:a.go:1:0", "ev-b:b.go:1:0")
	assertAcceptedEvidenceKeys(t, parentBA.AcceptedEvidenceRefs(), "ev-a:a.go:1:0", "ev-b:b.go:1:0")
	assertNodeArtifactIDs(t, parentAB.NodeArtifactRecords(), "ev-a", "ev-b")
	assertNodeArtifactIDs(t, parentBA.NodeArtifactRecords(), "ev-a", "ev-b")
	if got := parentAB.NodeExecAttempt("shared"); got != 3 {
		t.Fatalf("fork attempts should merge by max, got %d", got)
	}
	if got := parentBA.NodeExecAttempt("shared"); got != 3 {
		t.Fatalf("fork attempts should be order-independent max, got %d", got)
	}
	assertSourceInventoryMemberNames(t, parentAB.SourceInventoryObservation(), "MemberA", "MemberB")
	assertSourceInventoryMemberNames(t, parentBA.SourceInventoryObservation(), "MemberA", "MemberB")
}

func TestEvidenceReducerOwnershipBehaviorNodeStatusLatestAuthority(t *testing.T) {
	c := NewEvidenceClosure("")
	c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class:        EvidenceReducerInputReadRunSnapshotSeed,
		NodeStatuses: map[string]NodeExecStatus{"n1": NodeExecDone},
	}, "")
	c.IngestEvidenceReducerInput(EvidenceReducerInput{
		Class:        EvidenceReducerInputReadRunSnapshotSeed,
		NodeStatuses: map[string]NodeExecStatus{"n1": NodeExecRequeued},
	}, "")
	if got := c.NodeExecStatus("n1"); got != NodeExecRequeued {
		t.Fatalf("node status latest-authority conflict policy = %s, want requeued", got)
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

func evidenceRoundOwnershipFork(file, evidenceID, memberName, producerNodeID string, sharedAttempts int) *EvidenceClosure {
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{file: true})
	c.AddReadRanges(map[string][]LineRange{file: {{Start: 1, End: 3}}})
	c.RecordFileTotalLines(file, 80)
	c.AppendAcceptedEvidenceRefs([]AcceptedEvidenceRef{{ID: evidenceID, Source: file, LineStart: 1}})
	c.RecordSourceInventoryObservation(evidenceRoundTestSourceInventoryObservation(memberName))
	c.SetNodeExecAttempts(map[string]int{"shared": sharedAttempts})
	c.AppendNodeArtifactRecords([]NodeArtifactRecord{{
		ProducerNodeID: producerNodeID,
		ProducerStage:  StageExplore,
		Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: evidenceID, Path: file, LineStart: 1},
	}})
	return c
}

func assertReadSetKeys(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("read set = %+v, want %v", got, want)
	}
	for _, file := range want {
		if !got[file] {
			t.Fatalf("read set = %+v, missing %s", got, file)
		}
	}
}

func assertAcceptedEvidenceKeys(t *testing.T, got []AcceptedEvidenceRef, want ...string) {
	t.Helper()
	keys := map[string]bool{}
	for _, ref := range got {
		keys[ref.ID+":"+ref.Source+":"+itoa(ref.LineStart)+":"+itoa(ref.LineEnd)] = true
	}
	if len(keys) != len(want) {
		t.Fatalf("accepted evidence = %+v, want keys %v", got, want)
	}
	for _, key := range want {
		if !keys[key] {
			t.Fatalf("accepted evidence = %+v, missing key %s", got, key)
		}
	}
}

func assertNodeArtifactIDs(t *testing.T, got []NodeArtifactRecord, want ...string) {
	t.Helper()
	ids := map[string]bool{}
	for _, record := range got {
		ids[record.Artifact.ID] = true
	}
	if len(ids) != len(want) {
		t.Fatalf("node artifacts = %+v, want ids %v", got, want)
	}
	for _, id := range want {
		if !ids[id] {
			t.Fatalf("node artifacts = %+v, missing id %s", got, id)
		}
	}
}

func assertSourceInventoryMemberNames(t *testing.T, got SourceInventoryObservation, want ...string) {
	t.Helper()
	names := map[string]bool{}
	for _, set := range got.Sets {
		for _, member := range set.Members {
			names[member.Name] = true
		}
	}
	if len(names) != len(want) {
		t.Fatalf("source inventory = %+v, want member names %v", got, want)
	}
	for _, name := range want {
		if !names[name] {
			t.Fatalf("source inventory = %+v, missing member %s", got, name)
		}
	}
}

func nodeArtifactRecordKeysContain(records []NodeArtifactRecord, prefix string) bool {
	for _, record := range records {
		key := NodeArtifactRecordKey(record)
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}
	return false
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
