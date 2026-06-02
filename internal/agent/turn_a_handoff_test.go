package agent

import (
	"fmt"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestBuildTurnAHandoffEvidenceKeepsStrictFirstAndAddsRankedSupport(t *testing.T) {
	strict := types.EvidenceItem{
		Kind:       types.EvidenceRegistration,
		Subject:    "Register",
		Predicate:  "binds",
		Object:     "NewFoo",
		Source:     "reg.go",
		LineStart:  10,
		LineEnd:    10,
		Producer:   "test.fixture",
		Scope:      types.ScopeLine,
		Confidence: 0.9,
	}
	strict.ID = types.StableEvidenceID(strict)
	support := pinnedEvidenceItem("flow.go", "Runtime.Run", 42)
	support.Kind = types.EvidenceMechanism
	support.Predicate = "calls"
	support.Object = "Worker.Execute"
	support.ID = types.StableEvidenceID(support)

	got := buildTurnAHandoffEvidence(
		nil,
		"registration",
		[]types.EvidenceItem{support, strict},
		[]types.AnswerChain{{Item: strict, StrictOK: true}},
	)

	if len(got) != 2 {
		t.Fatalf("handoff evidence = %d, want 2", len(got))
	}
	if got[0].ID != strict.ID {
		t.Fatalf("strict terminal must stay first, got first ID %q want %q", got[0].ID, strict.ID)
	}
	if got[1].ID != support.ID {
		t.Fatalf("ranked support evidence missing after strict terminal, got second ID %q want %q", got[1].ID, support.ID)
	}
}

func TestBuildTurnAHandoffEvidenceDedupesStrictAndRankedByStableID(t *testing.T) {
	strict := pinnedEvidenceItem("reg.go", "Register", 10)

	got := buildTurnAHandoffEvidence(
		nil,
		"registration",
		[]types.EvidenceItem{strict},
		[]types.AnswerChain{
			{Item: strict, StrictOK: true},
			{Item: strict, StrictOK: false},
		},
	)

	if len(got) != 1 {
		t.Fatalf("duplicate strict/ranked evidence should collapse to one item, got %d", len(got))
	}
	if got[0].ID == "" {
		t.Fatal("handoff evidence should carry a stable ID")
	}
}

func TestBuildTurnAHandoffEvidenceCapsRankedSupport(t *testing.T) {
	items := make([]types.EvidenceItem, 0, extractorMaxEvidence+5)
	for i := 0; i < extractorMaxEvidence+5; i++ {
		items = append(items, pinnedEvidenceItem("a.go", fmt.Sprintf("Foo%d", i), i+1))
	}

	got := buildTurnAHandoffEvidence(nil, string(types.ReqMechanism), items, nil)

	if len(got) != extractorMaxEvidence {
		t.Fatalf("handoff evidence = %d, want cap %d", len(got), extractorMaxEvidence)
	}
}

func TestBuildTurnAHandoffEvidenceKeepsNonSeedNoStrictPathEmpty(t *testing.T) {
	items := []types.EvidenceItem{pinnedEvidenceItem("a.go", "Foo", 1)}

	got := buildTurnAHandoffEvidence(nil, "registration", items, nil)

	if len(got) != 0 {
		t.Fatalf("non-seed no-strict path should remain empty, got %d", len(got))
	}
}

func TestMergeTurnAArtifactsWithPriorPreservesSourceInventory(t *testing.T) {
	prior := &types.TurnAArtifacts{
		SourceInventoryObservation: types.SourceInventoryObservation{
			Active: true,
			Sets: []types.SourceInventoryObservationSet{{
				Role:  types.AnswerCandidateRoleFunction,
				Count: 1,
				Members: []types.SourceInventoryObservationMember{{
					Name:       "loadW3Token",
					Key:        "api.ts:10",
					SupportRef: "api.ts:10",
					File:       "api.ts",
					Line:       10,
				}},
			}},
		},
	}
	current := types.TurnAArtifacts{
		UserQuestion: "headers",
		ToolResults:  []types.ToolResult{{ToolName: "grep", Success: true, Summary: "current retry"}},
	}

	got := mergeTurnAArtifactsWithPrior(prior, current)
	if !got.SourceInventoryObservation.IsActive() ||
		len(got.SourceInventoryObservation.Sets) != 1 ||
		got.SourceInventoryObservation.Sets[0].Members[0].Name != "loadW3Token" {
		t.Fatalf("source-inventory handoff should survive closure-only retry windows: %+v", got.SourceInventoryObservation)
	}
}
