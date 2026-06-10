package types

import (
	"strings"
	"testing"
)

func TestWriteExplorationHandoffFromTurnAProjectsBoundedPlanningContext(t *testing.T) {
	req := WriteExplorationRequest{
		BatchID:              "batch-1",
		Goal:                 "thread retry budget through planner",
		CandidatePaths:       []string{"internal/agent/planner.go"},
		ExplorationQuestions: []string{"confirm retry cap source"},
	}
	turnA := TurnAArtifacts{
		ReadFiles: []string{"internal/orchestrator/phase_scheduler.go"},
		EvidenceItems: []EvidenceItem{
			{
				ID:              "ev1",
				Kind:            EvidenceMechanism,
				Subject:         "planner retry cap",
				Predicate:       "is derived from",
				Object:          "AgentSettings",
				Source:          "internal/agent/planner.go",
				LineStart:       105,
				LineEnd:         120,
				AnchorSymbol:    "BuildInitialInstruction",
				Summary:         "planner captures per-dispatch caps",
				GroundingStatus: GroundingRecovered,
			},
			{
				ID:              "ev2",
				Kind:            EvidenceConflict,
				Subject:         "phase reset",
				Predicate:       "may discard",
				Object:          "planning context",
				Source:          "internal/orchestrator/phase_scheduler.go",
				LineStart:       566,
				Summary:         "phase reset clears per-phase state",
				GroundingStatus: GroundingRecovered,
			},
		},
		FlowFindings: []FlowFindingDigest{{
			Path:              []string{"write_analyzer", "planner", "emit_change_plan"},
			UnsupportedReason: "verification path still unknown",
		}},
		AcceptedAggregateFacts: []AnswerAggregateFact{{
			Label:       "required verification",
			Value:       "go test ./internal/agent",
			SupportRefs: []string{"planner_test @ internal/agent/planner_task_framing_test.go:1"},
		}},
		ValidationBoundaryNotes: []string{"final answer must not use handoff as citations"},
		AcceptedClosureReason:   "source exploration found planner hook points",
	}

	got := WriteExplorationHandoffFromTurnA(req, turnA)
	for _, want := range []string{
		"internal/orchestrator/phase_scheduler.go",
		"internal/agent/planner.go",
	} {
		if !containsWriteExplorationString(got.TargetFiles, want) {
			t.Fatalf("target files missing %q: %+v", want, got.TargetFiles)
		}
	}
	if !containsWriteExplorationString(got.RelevantSymbols, "BuildInitialInstruction") {
		t.Fatalf("relevant symbols missing anchor: %+v", got.RelevantSymbols)
	}
	if !containsWriteExplorationSubstring(got.ExistingPatterns, "planner retry cap") {
		t.Fatalf("mechanism evidence not projected into patterns: %+v", got.ExistingPatterns)
	}
	if !containsWriteExplorationSubstring(got.RiskNotes, "phase reset") {
		t.Fatalf("conflict evidence not projected into risks: %+v", got.RiskNotes)
	}
	if !containsWriteExplorationSubstring(got.Unknowns, "verification path still unknown") ||
		!containsWriteExplorationSubstring(got.Unknowns, "confirm retry cap source") {
		t.Fatalf("unknowns missing flow/request context: %+v", got.Unknowns)
	}
	if !containsWriteExplorationSubstring(got.Invariants, "final answer must not use handoff as citations") ||
		!containsWriteExplorationSubstring(got.Invariants, "source exploration found planner hook points") ||
		!containsWriteExplorationSubstring(got.Invariants, "planner_test") {
		t.Fatalf("invariants missing boundary/support context: %+v", got.Invariants)
	}
	if len(got.EvidenceRefs) != 2 {
		t.Fatalf("evidence refs not preserved: %+v", got.EvidenceRefs)
	}
	if got.Confidence == "" {
		t.Fatalf("confidence should be inferred")
	}
}

func TestMutableStateWriteExplorationDefensiveCopy(t *testing.T) {
	mu := NewMutableState("request")
	req := &WriteExplorationRequest{
		BatchID:        "batch-1",
		CandidatePaths: []string{"a.go"},
	}
	mu.SetWriteExplorationRequest(req)
	req.CandidatePaths[0] = "mutated.go"
	gotReq := mu.WriteExplorationRequest()
	gotReq.CandidatePaths[0] = "caller-mutated.go"
	gotReq2 := mu.WriteExplorationRequest()
	if gotReq2.CandidatePaths[0] != "a.go" {
		t.Fatalf("request mutation leaked through state: %+v", gotReq2)
	}

	handoff := &WriteExplorationHandoff{
		BatchID:     "batch-1",
		TargetFiles: []string{"planner.go"},
		EvidenceRefs: []WriteExplorationEvidenceRef{{
			Source:    "planner.go",
			LineStart: 10,
		}},
	}
	mu.SetWriteExplorationHandoff(handoff)
	handoff.TargetFiles[0] = "mutated.go"
	gotHandoff := mu.WriteExplorationHandoff()
	gotHandoff.EvidenceRefs[0].Source = "caller-mutated.go"
	gotHandoff2 := mu.WriteExplorationHandoff()
	if gotHandoff2.TargetFiles[0] != "planner.go" || gotHandoff2.EvidenceRefs[0].Source != "planner.go" {
		t.Fatalf("handoff mutation leaked through state: %+v", gotHandoff2)
	}

	pack := &WriteContextPack{
		Items: []WriteContextItem{{
			Priority: WriteContextP0,
			Kind:     "constraint",
			Text:     "preserve read mode",
		}},
	}
	mu.SetWriteContextPack(pack)
	pack.Items[0].Text = "mutated"
	gotPack := mu.WriteContextPack()
	gotPack.Items[0].Text = "caller-mutated"
	gotPack2 := mu.WriteContextPack()
	if gotPack2.Items[0].Text != "preserve read mode" {
		t.Fatalf("context pack mutation leaked through state: %+v", gotPack2)
	}

	run := &WriteWorkflowRun{
		RunID: "wf-1",
		Batches: []WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: WriteWorkflowBatchNeedsExploration,
		}},
	}
	mu.SetWriteWorkflowRun(run)
	run.Batches[0].Status = WriteWorkflowBatchBlocked
	gotRun := mu.WriteWorkflowRun()
	gotRun.Batches[0].Status = WriteWorkflowBatchComplete
	gotRun2 := mu.WriteWorkflowRun()
	if gotRun2.Batches[0].Status != WriteWorkflowBatchNeedsExploration {
		t.Fatalf("workflow run mutation leaked through state: %+v", gotRun2)
	}

	raw := []byte(`{"action":"finish"}`)
	mu.SetWriteWorkflowDecisionJSON(raw)
	raw[0] = 'X'
	gotRaw := mu.WriteWorkflowDecisionJSON()
	gotRaw[0] = 'Y'
	gotRaw2 := mu.WriteWorkflowDecisionJSON()
	if string(gotRaw2) != `{"action":"finish"}` {
		t.Fatalf("workflow decision JSON mutation leaked through state: %s", gotRaw2)
	}
}

func TestNormalizeWriteExplorationHandoffBoundsListsAndRefs(t *testing.T) {
	handoff := WriteExplorationHandoff{Confidence: "HIGH"}
	for i := 0; i < writeExplorationMaxListItems+5; i++ {
		handoff.TargetFiles = append(handoff.TargetFiles, strings.Repeat("x", writeExplorationMaxTextLen+20)+string(rune('a'+(i%5)))+string(rune('0'+(i%10))))
	}
	for i := 0; i < writeExplorationMaxEvidenceRefs+5; i++ {
		handoff.EvidenceRefs = append(handoff.EvidenceRefs, WriteExplorationEvidenceRef{
			Source:    "f.go",
			LineStart: -1,
			LineEnd:   -2,
			Summary:   strings.Repeat("s", writeExplorationMaxTextLen+10),
		})
	}
	got := NormalizeWriteExplorationHandoff(handoff)
	if len(got.TargetFiles) > writeExplorationMaxListItems {
		t.Fatalf("target files not capped: %d", len(got.TargetFiles))
	}
	if len(got.EvidenceRefs) != writeExplorationMaxEvidenceRefs {
		t.Fatalf("evidence refs not capped: %d", len(got.EvidenceRefs))
	}
	if got.EvidenceRefs[0].LineStart != 0 || got.EvidenceRefs[0].LineEnd != 0 {
		t.Fatalf("negative lines not normalized: %+v", got.EvidenceRefs[0])
	}
	if len(got.EvidenceRefs[0].Summary) > writeExplorationMaxTextLen+3 || !strings.HasSuffix(got.EvidenceRefs[0].Summary, "...") {
		t.Fatalf("summary not trimmed: %d", len(got.EvidenceRefs[0].Summary))
	}
	if got.Confidence != "high" {
		t.Fatalf("confidence not normalized: %q", got.Confidence)
	}
}

func containsWriteExplorationString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func containsWriteExplorationSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
