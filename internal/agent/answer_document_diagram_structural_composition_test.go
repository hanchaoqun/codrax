package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestDiagramStructuralCompositionUsesParserStampedOwnershipAcrossLanguages(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "Orchestrator", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
				}},
		}},
		EvidenceItems: []types.EvidenceItem{{
			ID: "typed-field", Kind: types.EvidenceDirect,
			Source: "src/pipeline.ext", LineStart: 12, Scope: types.ScopeLine,
			AnchorKind: types.AnchorDefinition, AnchorSymbol: "busCtx",
			DeclaredBinding: "Orchestrator.busCtx", DeclaredType: "*types.BusContext", DeclaredOwner: "Orchestrator",
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	got := renderAnswerDocDiagramStructuralCompositionHandoff(ctx)
	for _, want := range []string{
		"## Final Diagram Structural Composition Handoff",
		"owner=`Orchestrator`; member=`busCtx`; type=`*types.BusContext`",
		"representation=`visible_owner_group_with_visible_nested_member`",
		"Add no arrow and no `edge_anchors` row for this containment",
		"cannot close the missing inter-participant transfer",
		"model still authors all visible wording, selection, layout, and conclusions",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("language-neutral typed ownership composition missing %q:\n%s", want, got)
		}
	}
}

func TestDiagramStructuralCompositionFailsClosedWithoutTwoExactRequestedParticipants(t *testing.T) {
	base := types.EvidenceItem{
		ID: "typed-field", Kind: types.EvidenceDirect,
		Source: "src/pipeline.ext", LineStart: 12, Scope: types.ScopeLine,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "busCtx",
		DeclaredBinding: "Orchestrator.busCtx", DeclaredType: "*types.BusContext", DeclaredOwner: "Orchestrator",
		GroundingStatus: types.GroundingGrounded,
	}
	for _, participants := range [][]types.DiagramParticipantHint{
		{{Identity: "Orchestrator", Role: types.DiagramParticipantIncidentRequired}},
		{
			{Identity: "UnrelatedOwner", Role: types.DiagramParticipantIncidentRequired},
			{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		},
	} {
		ctx := &types.AgentContext{
			AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
				Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
				DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture, Required: true, Participants: participants},
			}},
			EvidenceItems: []types.EvidenceItem{base},
		}
		if got := renderAnswerDocDiagramStructuralCompositionHandoff(ctx); got != "" {
			t.Fatalf("unmatched ownership declaration must fail closed:\n%s", got)
		}
	}
}

func TestRequiredDiagramRepairPreservesVerifiedNoArrowOwnership(t *testing.T) {
	repo := t.TempDir()
	writeStageBindingFixture(t, repo)
	participants := []types.DiagramParticipantHint{
		{Identity: "Analyzer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Explorer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Extractor", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Finalizer", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "BusContext", Role: types.DiagramParticipantIncidentRequired},
		{Identity: "Mutable", Role: types.DiagramParticipantIncidentRequired},
	}
	ctx := &types.AgentContext{
		Mode: types.ModeRead, RepoRoot: repo,
		Mutable: types.NewMutableState("explain the read pipeline"),
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, PredicateAxis: types.AxisFlow,
			AnalyzerHints: types.AnalyzerHints{
				Kind:     string(types.ReqMechanism),
				Entities: []string{"Analyzer", "Explorer", "Extractor", "Finalizer", "BusContext", "Mutable"},
			},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramFlow, Required: true, Participants: participants},
		}},
	}

	hint, ok := answerDocRequiredDiagramRelationBoundaryPatchHint(ctx, false)
	if !ok {
		t.Fatal("verified stage subset should publish a required-diagram repair")
	}
	for _, want := range []string{
		"unproven boundary",
		"must not be flattened into unrelated peer nodes",
		"exact structural composition lane below is independent of the directed edge boundary",
		"owner=`BusContext`; member=`Mutable`; type=`*MutableState`",
		"directed_relation_authority=`none`",
		"does not close any `unproven` directed-relation boundary",
	} {
		if !strings.Contains(hint, want) {
			t.Fatalf("required repair lost exact no-arrow ownership guidance %q:\n%s", want, hint)
		}
	}
	if strings.Contains(hint, "may remain disconnected") {
		t.Fatalf("repair must not flatten proved structural ownership into disconnected peers:\n%s", hint)
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Final Diagram Structural Composition Handoff") ||
		!strings.Contains(prompt, "owner=`BusContext`; member=`Mutable`; type=`*MutableState`") {
		t.Fatalf("initial finalizer prompt must receive the same structural composition authority:\n%s", prompt)
	}
}
