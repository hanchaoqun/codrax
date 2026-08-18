package tool

import (
	"strings"
	"testing"

	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func requestedBranchBehaviorFixture(t *testing.T, language string) (*types.BusContext, []types.EvidenceItem) {
	t.Helper()
	file := "src/tokenizer.py"
	fi := &repotypes.FileInfo{
		RelPath:  file,
		Language: language,
		ControlFlowBranches: []repotypes.ControlFlowBranch{
			{
				Condition: "_HAVE_NATIVE", GuardLine: 20,
				Arm: repotypes.ControlFlowArmConsequence, BodyLineStart: 21, BodyLineEnd: 21,
				Effects: []repotypes.ControlFlowEffect{{
					Kind: repotypes.ControlFlowEffectCall, Expression: "self._tokenize_fast(data)", LineStart: 21, LineEnd: 21,
				}},
				Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "fixture_parser",
			},
			{
				Condition: "_HAVE_NATIVE", GuardLine: 20,
				Arm: repotypes.ControlFlowArmAlternative, BodyLineStart: 22, BodyLineEnd: 22,
				Effects: []repotypes.ControlFlowEffect{{
					Kind: repotypes.ControlFlowEffectCall, Expression: "self._tokenize_slow(data)", LineStart: 22, LineEnd: 22,
				}},
				Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "fixture_parser",
			},
		},
	}
	graph := &repotypes.Graph{
		Files:     []*repotypes.FileInfo{fi},
		FileIndex: map[string]*repotypes.FileInfo{file: fi},
	}
	mut := types.NewMutableState("explain fallback branch behavior")
	mut.SetSearchGraph(graph)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			RequestedAnswerDimensions: &types.RequestedAnswerDimensionProfile{
				IsDimensionedAnswer: true,
				Dimensions: []types.RequestedAnswerDimension{{
					Label: "fallback branch", Role: types.RequestedAnswerDimensionBranchBehavior, Required: true,
				}},
			},
		}},
	}
	evidence := []types.EvidenceItem{
		{
			Kind: types.EvidenceMechanism, AnchorKind: types.AnchorCondition,
			AnchorSymbol: "_HAVE_NATIVE", Source: file, LineStart: 20,
			Scope: types.ScopeLine, Producer: types.EvidenceProducerExplorerEmitEvidence,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall,
			AnchorSymbol: "_tokenize_slow", Subject: "tokenize", Object: "_tokenize_slow",
			Source: file, LineStart: 22, Scope: types.ScopeLine,
			Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind: types.EvidenceMechanism, AnchorKind: types.AnchorAssignment,
			AnchorSymbol: "_HAVE_NATIVE", Subject: "_HAVE_NATIVE", Object: "true",
			Source: file, LineStart: 3, Scope: types.ScopeLine,
			Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
		},
	}
	return ctx, evidence
}

func TestRequestedBranchBehaviorStateDowngradeLanguageNeutral(t *testing.T) {
	executableLanguages := []string{
		repotypes.LangGo, repotypes.LangPython, repotypes.LangJavaScript,
		repotypes.LangTypeScript, repotypes.LangJava, repotypes.LangKotlin,
		repotypes.LangRust, repotypes.LangC, repotypes.LangCpp,
		repotypes.LangRuby, repotypes.LangSwift, repotypes.LangLua,
		repotypes.LangArkTS, repotypes.LangCangjie,
	}
	for _, language := range executableLanguages {
		language := language
		t.Run(language, func(t *testing.T) {
			ctx, evidence := requestedBranchBehaviorFixture(t, language)
			got := requestedBranchBehaviorStateDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence)
			for _, want := range []string{
				"lacks the state provenance", "`_HAVE_NATIVE`", "alternative",
				"self._tokenize_slow(data)", "`src/tokenizer.py:3=true`", "reachability unproven",
			} {
				if !strings.Contains(got, want) {
					t.Fatalf("downgrade=%q, want %q", got, want)
				}
			}
			repairs := ctx.Mutable.EvidenceClosure().ActiveRepairs()
			if len(repairs) != 1 || repairs[0].Kind != types.RepairEmitEvidence ||
				repairs[0].Origin != "pre_complete.requested_branch_behavior_state.20" ||
				len(repairs[0].Keywords) != 1 || repairs[0].Keywords[0] != "_HAVE_NATIVE" {
				t.Fatalf("repairs=%+v, want exact typed guard repair", repairs)
			}
		})
	}
}

func TestRequestedBranchBehaviorStateDowngradeClosesWhenBothStatesAreGrounded(t *testing.T) {
	ctx, evidence := requestedBranchBehaviorFixture(t, repotypes.LangPython)
	evidence = append(evidence, types.EvidenceItem{
		Kind: types.EvidenceMechanism, AnchorKind: types.AnchorAssignment,
		AnchorSymbol: "_HAVE_NATIVE", Subject: "_HAVE_NATIVE", Object: "false",
		Source: "src/tokenizer.py", LineStart: 6, Scope: types.ScopeLine,
		Producer: types.EvidenceProducerExplorerEmitEvidence, GroundingStatus: types.GroundingGrounded,
	})
	if got := requestedBranchBehaviorStateDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("both grounded states remove the sole-opposite-state contradiction; got %q", got)
	}
}

func TestRequestedBranchBehaviorStateDowngradePreCompleteWirePin(t *testing.T) {
	ctx, evidence := requestedBranchBehaviorFixture(t, repotypes.LangPython)
	got := preCompleteContractCheckWithEvidence(ctx, "", evidence)
	if !strings.Contains(got, "lacks the state provenance") ||
		!strings.Contains(got, "self._tokenize_slow(data)") {
		t.Fatalf("pre-complete output=%q, want requested branch-behavior downgrade", got)
	}
}

func TestRequestedBranchBehaviorStateDowngradeCangjieElseLinePair(t *testing.T) {
	ctx, evidence := requestedBranchBehaviorFixture(t, repotypes.LangCangjie)
	graph := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	alternative := &graph.Files[0].ControlFlowBranches[1]
	alternative.GuardLine = 22
	alternative.BodyLineStart = 23
	alternative.BodyLineEnd = 23
	alternative.Effects[0].LineStart = 23
	alternative.Effects[0].LineEnd = 23
	evidence[1].LineStart = 23
	got := requestedBranchBehaviorStateDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence)
	if !strings.Contains(got, "lacks the state provenance") ||
		!strings.Contains(got, "self._tokenize_slow(data)") || !strings.Contains(got, "@ line 23") {
		t.Fatalf("Cangjie parser-owned consequence/alternative pair was lost: %q", got)
	}
}

func TestRequestedBranchBehaviorStateDowngradeRequiresTypedDimension(t *testing.T) {
	ctx, evidence := requestedBranchBehaviorFixture(t, repotypes.LangPython)
	ctx.AnalysisIR.RequestModel.RequestedAnswerDimensions.Dimensions[0].Role = types.RequestedAnswerDimensionFunctionOrPurpose
	if got := requestedBranchBehaviorStateDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("generic function purpose must not activate branch reachability gate: %q", got)
	}
}

func TestRequestedBranchBehaviorStateDowngradeFailsOpenForCompoundGuard(t *testing.T) {
	ctx, evidence := requestedBranchBehaviorFixture(t, repotypes.LangPython)
	graph := ctx.Mutable.SearchGraph().(*repotypes.Graph)
	for i := range graph.Files[0].ControlFlowBranches {
		graph.Files[0].ControlFlowBranches[i].Condition = "_HAVE_NATIVE && enabled"
	}
	if got := requestedBranchBehaviorStateDowngrade(ctx, ctx.Mutable.EvidenceClosure(), evidence); got != "" {
		t.Fatalf("compound guard must fail open instead of guessing boolean reachability: %q", got)
	}
}

func TestRequestedBranchBehaviorEffectIdentityAcrossLanguageCallShapes(t *testing.T) {
	for _, tc := range []struct {
		name, expression, anchor string
	}{
		{name: "qualified member", expression: "self._tokenize_slow(data)", anchor: "_tokenize_slow"},
		{name: "cpp pointer member", expression: "service->fallback(data)", anchor: "fallback"},
		{name: "cpp template", expression: "service.fallback<Result>(data)", anchor: "fallback"},
		{name: "rust turbofish", expression: "service::fallback::<Result>(data)", anchor: "fallback"},
		{name: "c function pointer", expression: "(*fallback)(data)", anchor: "fallback"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			effect := repotypes.ControlFlowEffect{Kind: repotypes.ControlFlowEffectCall, Expression: tc.expression}
			item := types.EvidenceItem{AnchorSymbol: tc.anchor}
			if !requestedBranchBehaviorEffectMatchesEvidence(effect, item) {
				t.Fatalf("effect %q did not preserve exact endpoint %q", tc.expression, tc.anchor)
			}
		})
	}
}
