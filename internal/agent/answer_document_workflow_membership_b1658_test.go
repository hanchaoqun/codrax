package agent

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/stageauthority"
	"github.com/hanchaoqun/codrax/internal/types"
)

func b1658WorkflowPromptContext(t *testing.T, lang string, refs bool) *types.AgentContext {
	t.Helper()
	repo, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	source := "internal/types/stage_binding.go"
	raw, err := os.ReadFile(filepath.Join(repo, source))
	if err != nil {
		t.Fatal(err)
	}
	fact := types.AnswerAggregateFact{Kind: types.AnswerAggregateMemberSet,
		Role: types.AnswerAggregateRolePrincipalAnswer, Label: "a model proposal with exact declaration sources", Value: "7", Provenance: "model_emitted",
		Members: []string{"StageLogTriage", "StagePerfTriage", "StageMultiRepoFocus", "StageAnalyze", "StageExplore", "StageExtract", "StageFinalize"},
	}
	var evidence []types.EvidenceItem
	for i, member := range fact.Members {
		lineNumber := 0
		for j, line := range strings.Split(string(raw), "\n") {
			if strings.Contains(line, "Stage:") && strings.Contains(line, member+",") {
				if lineNumber != 0 {
					t.Fatalf("ambiguous declaration fixture for %s", member)
				}
				lineNumber = j + 1
			}
		}
		if lineNumber == 0 {
			t.Fatalf("missing real declaration %s", member)
		}
		evidence = append(evidence, types.EvidenceItem{
			ID: fmt.Sprintf("stage-declaration-%d", i), Kind: types.EvidenceDirect,
			Source: source, LineStart: lineNumber, Scope: types.ScopeLine,
			Subject: member, AnchorSymbol: member, AnchorKind: types.AnchorDefinition,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "The source declares this stage; membership is a separate claim.",
		})
		fact.MemberNotes = append(fact.MemberNotes, "model explanation for "+member)
		if refs {
			fact.SupportRefs = append(fact.SupportRefs, fmt.Sprintf("%s @ %s:%d", member, source, lineNumber))
		}
	}
	mu := types.NewMutableState("Explain the read workflow from analyze to finalizer and give a stage input/output table")
	mu.AppendEvidence(evidence)
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{fact})
	mu.RetainInvestigationAggregateFacts()
	return &types.AgentContext{Mode: types.ModeRead, RepoRoot: repo, Mutable: mu, EvidenceItems: evidence,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, Language: lang,
			PredicateAxis: types.AxisFlow, AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
			Predicates:             types.SemanticPredicates{HasPerMemberTable: true, IsCrossComponent: true},
			CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "from analyze to finalizer"},
			DiagramHint: &types.DiagramHint{Kind: types.DiagramSequence, Required: true,
				Participants: []types.DiagramParticipantHint{
					{Identity: "analyze", Role: types.DiagramParticipantIncidentRequired},
					{Identity: "finalizer", Role: types.DiagramParticipantIncidentRequired},
				}},
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{Index: 1, Required: true, Role: types.RequestedAnswerDimensionStageWorkflow, Label: "stage inputs and outputs"}}},
		}},
	}
}

func TestB1658ActualPromptKeepsSourceButNotMandatoryWorkflowRoster(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		for _, refs := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/refs=%t", lang, refs), func(t *testing.T) {
				ctx := b1658WorkflowPromptContext(t, lang, refs)
				before, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
				prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
				if strings.Contains(prompt, "### a model proposal with exact declaration sources (7 row(s))") ||
					strings.Contains(prompt, "The typed principal lane contains 7 answer-grade member(s)") {
					t.Error("actual prompt promoted seven independently located declarations into mandatory workflow members")
				}
				if !strings.Contains(prompt, "principal_contract=`not_authorized`") {
					t.Error("actual aggregate prompt does not disclose the missing membership authority")
				}
				if !strings.Contains(prompt, "fact_authority=`workflow_membership_unproven`") ||
					!strings.Contains(prompt, "Source coordinates and grounded facts, when present, retain their own citation and claim scope") ||
					!strings.Contains(prompt, "This adds no call-edge, artifact-transfer, or full-roster obligation") {
					t.Error("workflow advisory conflates valid source locations with unproved membership or adds a new relation obligation")
				}
				for _, want := range []string{
					"canonical_read_main_sequence=`analyze -> explore -> extract -> finalize`",
					"cannot widen the active membership", "language model authors only the request classification",
					"StageLogTriage", "StagePerfTriage", "StageMultiRepoFocus", "StageAnalyze", "StageExplore", "StageExtract", "StageFinalize",
					"internal/types/stage_binding.go:", "model explanation for StageMultiRepoFocus",
				} {
					if !strings.Contains(prompt, want) {
						t.Errorf("actual prompt lost source/support or checkout-verified stage authority %q", want)
					}
				}
				support := types.BuildAnswerSupportPlanForAgentContext(ctx)
				if support == nil || len(support.Lanes) == 0 {
					t.Fatal("source-support fallback was erased rather than demoted")
				}
				if (support.Family == types.QFEnumeration && support.PrincipalMemberCoverage != types.PrincipalMemberCoveragePolicyEnrichmentOnly) || len(types.PrincipalSupportMemberObligations(support)) != 0 {
					t.Errorf("support fallback reintroduced mandatory members after row authority was declined: family=%s policy=%s obligations=%d lanes=%+v", support.Family, support.PrincipalMemberCoverage, len(types.PrincipalSupportMemberObligations(support)), support.Lanes)
				}
				after, _ := json.Marshal(ctx.Mutable.StableInvestigationAggregateFacts())
				if string(before) != string(after) {
					t.Fatal("prompt compilation changed model aggregate members, notes, or locations")
				}
			})
		}
	}
}

func TestB1658ActualEnumerationSupportFallbackDoesNotMintWorkflowMembers(t *testing.T) {
	ctx := b1658WorkflowPromptContext(t, "en", true)
	ctx.AnalysisIR.RequestModel.Predicates.IsCategoryEnumeration = true
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	support := types.BuildAnswerSupportPlanForAgentContext(ctx)
	if support == nil || support.Family != types.QFEnumeration || len(support.Lanes) == 0 {
		t.Fatalf("fixture must reach the actual enumeration fallback, got %+v", support)
	}
	if support.PrincipalMemberCoverage != types.PrincipalMemberCoveragePolicyEnrichmentOnly || len(types.PrincipalSupportMemberObligations(support)) != 0 {
		t.Fatalf("enumeration fallback re-minted workflow membership: policy=%s obligations=%d", support.PrincipalMemberCoverage, len(types.PrincipalSupportMemberObligations(support)))
	}
	if strings.Contains(prompt, "The typed principal lane contains 7 answer-grade member(s)") ||
		!strings.Contains(prompt, "Grounded workflow supporting evidence") ||
		!strings.Contains(prompt, "this lane supplies no additional mandatory members") ||
		!strings.Contains(prompt, "StageMultiRepoFocus") {
		t.Fatal("actual fallback prompt erased support or kept a conflicting mandatory member slate")
	}
}

func TestB1658ActualPromptKeepsVerifiedSubspanAndRelationBoundary(t *testing.T) {
	for _, lang := range []string{"zh-CN", "en"} {
		t.Run(lang, func(t *testing.T) {
			ctx := b1658WorkflowPromptContext(t, lang, true)
			// A required all-stage dimension selects the whole lane. Without it,
			// the existing typed endpoint selection must keep its narrower span.
			ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions = nil
			ctx.AnalysisIR.RequestModel.DiagramHint.Participants[0].Identity = "explore"
			authority, ok := stageauthority.LoadReadMode(ctx.RepoRoot)
			if !ok {
				t.Fatal("actual checkout stage provider unavailable")
			}
			selection := stageauthority.SelectRequiredReadModeWorkflow(ctx.AnalysisIR.RequestModel, ctx.EvidenceItems, authority)
			if len(selection.Main) != 3 || selection.Main[0].StageValue != "explore" || len(selection.Precedence) != 2 {
				t.Fatalf("provider fixture does not select the requested contiguous subspan: %+v", selection)
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "canonical_read_main_sequence=`explore -> extract -> finalize`") ||
				!strings.Contains(prompt, "They do not prove `call`, `data_flow`, artifact transfer") {
				t.Fatal("actual prompt lost the verified subspan or upgraded precedence into call/data-flow authority")
			}
		})
	}
}
