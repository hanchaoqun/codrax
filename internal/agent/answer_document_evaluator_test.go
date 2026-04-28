package agent

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
)

// enableSummaryCapsForTest flips summary-cap master switch on for the
// duration of a test and restores the default (Enabled=false) on
// cleanup. Needed by the shrinkage-salvage cap-trim cases — they
// expect the trimmer to land at SummaryCapFor(shape, itemCount),
// which returns SummaryCapUnlimited when the switch is off.
func enableSummaryCapsForTest(t *testing.T) {
	t.Helper()
	cfg := types.DefaultSummaryCapConfig()
	cfg.Enabled = true
	types.SetSummaryCapConfig(cfg)
	t.Cleanup(func() { types.SetSummaryCapConfig(types.DefaultSummaryCapConfig()) })
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersResolvedShape
// pins that the dynamic prompt surfaces the resolved target shape
// (for operator visibility + diagnostic logs). The STATIC shape
// dispatch table — tool name, required fields, forbidden fields —
// lives in answer-document-skill.OutputFormat and is rendered as a
// system section by context/builder.go, NOT here. Asserting those
// substrings in the dynamic prompt would resurrect the pre-cleanup
// contradiction between the skill's declarative contract and the
// evaluator's baked-in instructions.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersResolvedShape(t *testing.T) {
	shapes := []types.AnswerShape{
		types.ShapeListOfSymbols, types.ShapeStepList, types.ShapeValue,
		types.ShapeConfigValue, types.ShapeBoolean, types.ShapeExplanation,
	}
	for _, shape := range shapes {
		t.Run(string(shape), func(t *testing.T) {
			ctx := &types.AgentContext{
				AnalysisIR: &types.AnalysisIR{
					AnswerContract: types.AnswerContract{RequiredAnswerShape: shape},
				},
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			// The dynamic prompt carries the shape name for operator
			// visibility — this is the one substring the evaluator
			// still owns after the static contract moved to the skill.
			if !strings.Contains(prompt, string(shape)) {
				t.Errorf("shape=%s: dynamic prompt missing resolved shape name: %q", shape, prompt)
			}
			// Guard against drift back to the pre-cleanup pattern:
			// the static contract MUST NOT resurface here.
			for _, banned := range []string{"emit_answer_document", "Prohibitions", "Citation pool"} {
				if strings.Contains(prompt, banned) {
					t.Errorf("shape=%s: dynamic prompt leaked static contract substring %q — "+
						"that content belongs in answer-document-skill, not the evaluator", shape, banned)
				}
			}
		})
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SingleTopicExplanationLeavesSymbolsEmpty(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeExplanation},
		},
		Mutable: types.NewMutableState(""),
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Leave `symbols[]` empty for this single-topic explanation") {
		t.Fatalf("single-topic explanation checklist must forbid anchor skeleton noise:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ResolvesAbsentConfigValueToExplanation(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetInvestigationResultKind("absence")
	mut.SetAbsenceJustification("repo-wide search found no exact key")
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeConfigValue,
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					AllowAbsence: true,
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, string(types.ShapeExplanation)) {
		t.Fatalf("resolved prompt should target explanation for stable absent exact config key: %q", prompt)
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline
// checks that when MustInclude is populated and the resolved shape
// is list_of_symbols, the dynamic prompt renders the γ floor so the
// LLM can compute its completeness claim without re-deriving it from
// the IR.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeListOfSymbols,
				MustInclude:         []string{"Alpha", "Beta"},
			},
		},
		AnswerSymbols: []types.AnswerSymbol{
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Alpha") || !strings.Contains(prompt, "Beta") {
		t.Errorf("prior slate not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "Required-symbol floor: **2 name(s)**") {
		t.Errorf("required-symbol floor not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "fewer than 2 items will be DOWNGRADED") {
		t.Errorf("downgrade warning not surfaced: %q", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersStepBackboneForStepList(t *testing.T) {
	mut := types.NewMutableState("")
	syms := []types.AnswerSymbol{
		{Name: "RequestModel", File: "internal/agent/analyzer.go", Line: 616, Rationale: "在 buildAnalysisIR 内部获取 LLM 输出的 RequestModel，是后续步骤的输入基础"},
		{Name: "gate.Run", File: "internal/agent/analyzer.go", Line: 1062, Rationale: "执行质量门检查，生成最终 gate 结果"},
	}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessLowerBound)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeStepList},
		},
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Step Backbone",
		"ordered lower-bound backbone",
		"`RequestModel` (internal/agent/analyzer.go:616)",
		"`gate.Run` (internal/agent/analyzer.go:1062)",
		"Do not merge one anchor's citation with semantics that only appear in another file / definition",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("step backbone prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersFallbackStepBackboneFromEvidence(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeStepList},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       127,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkCoverage",
				Summary:         "checkCoverage is appended as the first gate check",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       128,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkDAGClosure",
				Summary:         "checkDAGClosure is appended next",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       129,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "checkBudgetSanity",
				Summary:         "checkBudgetSanity is appended after DAG closure",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Step Backbone",
		"`checkCoverage` (internal/analysis/gate/gate.go:127)",
		"`checkDAGClosure` (internal/analysis/gate/gate.go:128)",
		"`checkBudgetSanity` (internal/analysis/gate/gate.go:129)",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("fallback step backbone prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedEnumerationBoundary(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeStepList},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Requested Set Boundary",
		"`7 checks` (7 item(s))",
		"Keep the main ordered `steps[]` sequence to 7 principal item(s).",
		"do not silently blend them into the principal set",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("requested set boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude
// checks the other branch: when MustInclude is empty, the prompt
// says "no floor is enforced" so the LLM picks the claim from its
// own recall confidence.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{RequiredAnswerShape: types.ShapeListOfSymbols},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Required-symbol floor is empty") {
		t.Errorf("no-floor branch missing: %q", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersDiagramContractAndSeeds(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeStepList,
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"axis_call"},
				},
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/a.go", Line: 10, Func: "inner"},
					{File: "internal/b.go", Line: 20, Func: "outer"},
				},
			}},
		},
		FlowFindings: []types.FlowFindingDigest{{
			Path:       []string{"Dispatch", "Handler"},
			Conditions: []string{"kind == call"},
		}},
		AnswerChains: []types.AnswerChain{{
			Item: types.EvidenceItem{
				Summary:   "Dispatch routes to Handler",
				Source:    "internal/a.go",
				LineStart: 10,
			},
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Diagram Contract",
		"Required: yes",
		"Preferred kinds: call_dag",
		"Avoid invented enumeration labels like `Level 1`, `Round 2`, or `Step 3`",
		"## Diagram Seeds",
		"### Grounded Labeling",
		"### Diagram Node Allowlist",
		"`internal/a.go`",
		"`internal/b.go`",
		"### Log Triage",
		"### Flow Findings",
		"### Answer Chains",
		"## First-Pass Diagram Skeleton",
		"innermost failure:",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersConfigTraceDiagramSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeExplanation,
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Subject: "DefaultExploreHeuristics", Summary: "code defaults", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "yaml precedence comment", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleYAML},
			{Source: "internal/config/runtime.go", LineStart: 194, Subject: "ExploreMidLoopMinIteration", Summary: "runtime yaml binding", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
			{Source: "cmd/root.go", LineStart: 1381, Summary: "CLI override applies when non-nil", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "OverrideLayer", DiagramRole: types.EvidenceDiagramRoleOverride},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"### Config Trace Precedence",
		"## First-Pass Diagram Skeleton",
		"## Precedence Role Coverage",
		"`default` → `internal/types/config.go:707`",
		"`config` → `codrax.yaml.example:20`",
		"`runtime` → `internal/config/runtime.go:194`",
		"codrax.yaml.example:20",
		"internal/types/config.go:707",
		"cmd/root.go:1381",
		"use grounded source labels instead of numbered layers",
		"validated `diagram_role_hint` evidence",
		"highest precedence at the top to lowest precedence at the bottom",
		"`override` = highest-precedence operator / CLI layer",
		"`runtime` is the binding / merge code path",
		"do NOT rename or reorder its nodes into abstract numbered placeholders",
		"The safest valid fenced diagram for this dispatch is an exact copy of that chain",
		"Conceptual layer names requested by the user belong in prose headings or bullets",
		"keep that explanation in prose outside the fenced diagram",
		"## Submission Checklist",
		"treat it as the grounded template for first-pass repair-resistant output",
		"every fenced-diagram node must have its own grounded citation",
		"### Diagram Node Allowlist",
		"`codrax.yaml.example`",
		"`internal/config/runtime.go`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DowngradesHardDiagramWithoutGroundedStructure(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramArchitecture, types.DiagramFlow},
				},
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{"missing_key"},
					AllowAbsence: true,
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Source:          "internal/types/config.go",
				LineStart:       707,
				GroundingStatus: types.GroundingGrounded,
				ContextRole:     types.EvidenceContextRoleAbsenceSupport,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
			},
		},
	}
	ctx.Mutable.SetInvestigationResultKind("absence")
	ctx.Mutable.SetAbsenceJustification("missing_key is absent from the current repo state")
	e := &answerDocumentEvaluator{}
	text := e.BuildInitialInstruction(ctx, nil)
	if strings.Contains(text, "## Diagram Contract") {
		t.Fatalf("hard diagram contract should downgrade when grounded structure is incomplete, got: %s", text)
	}
	if !strings.Contains(text, "## Diagram Preference") {
		t.Fatalf("downgraded diagram requirement should still leave a preference note, got: %s", text)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ConfigTraceSeedWarnsWhenOverrideAnchorMissing(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeExplanation,
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Subject: "DefaultExploreHeuristics", Summary: "code defaults", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "yaml precedence comment", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleYAML},
			{Source: "internal/config/runtime.go", LineStart: 194, Subject: "ExploreMidLoopMinIteration", Summary: "runtime yaml binding", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Current grounded evidence does NOT include anchor(s) for these precedence role(s): override") {
		t.Fatalf("prompt missing generic missing-role warning:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ConfigTraceSeedWarnsWhenYAMLAnchorMissing(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeExplanation,
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Subject: "DefaultExploreHeuristics", Summary: "code defaults", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "internal/config/runtime.go", LineStart: 194, Subject: "ExploreMidLoopMinIteration", Summary: "runtime yaml binding", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "same-family background without a validated diagram role", Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics"},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Current grounded evidence does NOT include anchor(s) for these precedence role(s): override, config") {
		t.Fatalf("prompt missing generic missing-role warning:\n%s", prompt)
	}
	if strings.Contains(prompt, "### Diagram Node Allowlist") && strings.Contains(prompt, "`codrax.yaml.example`") {
		t.Fatalf("diagram node allowlist must exclude non-diagram evidence labels when that role is missing:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SanitizesIllustrativeAbsenceJustification(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("searched the repo and found the token only in internal/skill/analysis_contract.go:367 comment examples")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					TargetLabel:  "config key",
					Targets:      []string{"explore_mid_loop_hint_budget"},
					AllowAbsence: true,
				},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
		EvidenceItems: []types.EvidenceItem{{
			Source:          "internal/skill/analysis_contract.go",
			LineStart:       367,
			Subject:         "explore_mid_loop_hint_budget",
			Summary:         "comment example only",
			ContextRole:     types.EvidenceContextRoleIllustrativeOnly,
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	contractStart := strings.Index(prompt, "## Exact Resolution Contract")
	if contractStart < 0 {
		t.Fatalf("prompt missing exact-resolution contract:\n%s", prompt)
	}
	contractEnd := strings.Index(prompt[contractStart+1:], "\n## ")
	contractBody := prompt[contractStart:]
	if contractEnd > 0 {
		contractBody = prompt[contractStart : contractStart+1+contractEnd]
	}
	if strings.Contains(contractBody, "internal/skill/analysis_contract.go:367") {
		t.Fatalf("exact-resolution contract should not echo illustrative-only source details into absence justification:\n%s", contractBody)
	}
	if !strings.Contains(prompt, "doc/test/example/comment-only mentions are illustrative only") {
		t.Fatalf("prompt should carry sanitized absence wording:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersExactResolutionContract(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "RuntimeSettings",
			Predicate:       "binds",
			Object:          "explore_midloop_min_iteration",
			Summary:         "RuntimeSettings exposes the YAML/runtime binding layer.",
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "defines",
			Object:          "ExploreHeuristics defaults",
			Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
			Source:          "internal/types/config.go",
			LineStart:       707,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleDefault,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "ExploreHeuristics",
			Predicate:       "documents",
			Object:          "heuristics config layer",
			Summary:         "codrax.yaml.example documents the three-layer precedence rule.",
			Source:          "codrax.yaml.example",
			LineStart:       25,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "ExploreHeuristics",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			DiagramRole:     types.EvidenceDiagramRoleYAML,
			GroundingStatus: types.GroundingRecovered,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "ExploreBudget",
			Predicate:       "defines",
			Object:          "runtime budget counter",
			Summary:         "ExploreBudget is a runtime counter, not a config lineage anchor.",
			Source:          "internal/types/explore_budget.go",
			LineStart:       40,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:              types.SubjectConfigKey,
					TargetLabel:             "config key",
					Targets:                 []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:            true,
					RequireTargetMention:    true,
					AliasRequiresProof:      true,
					RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
					RelatedContextScopeHint: "same namespace / prefix family",
				},
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:            "config_mapping",
					PrimaryEntities: []string{"explore_mid_loop_hint_budget"},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceMechanism,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
				Source:          "internal/types/config.go",
				LineStart:       520,
				AnchorKind:      types.AnchorDefinition,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Exact Resolution Contract",
		"explore_mid_loop_hint_budget",
		"Absence-only is acceptable",
		"same namespace / prefix family",
		"you MUST set `exact_resolution.context_mode=\"grounded_context_only\"`",
		"Locked exact-resolution output",
		"Do not speculate about hypothetical parser / runtime behavior",
		"renderer will insert the exact-absence lead before `summary`",
		"treat `summary` as the follow-on grounded-context block only",
		"Keep the exact target name in the renderer-generated lead only",
		"Preferred exact_resolution object for this dispatch",
		"{\"status\":\"absent\",\"context_mode\":\"grounded_context_only\"}",
		"Summary surface mode: follow-on grounded context only",
		"does NOT license an invented field inventory",
		"Do not add a separate paragraph about the effect of supplying the absent target",
		"Surface-allowed nearby context is not automatically citation-grade",
		"only create a separate numbered step when that layer has its own grounded repo anchor",
		"repo-wide search result, aggregate absence conclusion, or test-only proof step usually has no single corroborating production line",
		"## Citation-Grade Grounded Context Anchors",
		"## Prose-Only Grounded Context Anchors",
		"Other surface-allowed anchors may still appear in `summary`, but only as uncited prose",
		"## Diagram-Grade Context Anchors",
		"## Related Context Citation Candidates",
		"## Background-Only Anchors",
		"## Exact Resolution Seeds",
		"DefaultExploreHeuristics",
		"codrax.yaml.example:25",
		"internal/types/config.go:520",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if !strings.Contains(prompt, "ExploreBudget") {
		t.Fatalf("background-only same-family anchors should be surfaced explicitly in the background-only section:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_NeutralizesExactResolutionChainSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RelatedContextTerms:  []string{"explore"},
				},
			},
		},
		AnswerChains: []types.AnswerChain{{
			Item: types.EvidenceItem{
				Kind:            types.EvidenceMechanism,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "explains",
				Object:          "nearby precedence baseline",
				Summary:         "This item names explore_mid_loop_hint_budget only in explanatory context; do NOT repair this item.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
		}},
	}

	seed := renderAnswerDocDiagramChainSeed(ctx)
	if strings.Contains(seed, "do NOT repair this item") {
		t.Fatalf("Answer Chains seed leaked operational repair prose:\n%s", seed)
	}
	if strings.Contains(seed, "explore_mid_loop_hint_budget") {
		t.Fatalf("Answer Chains seed leaked repeated exact target prose:\n%s", seed)
	}
	if !strings.Contains(seed, "DefaultExploreHeuristics explains nearby precedence baseline") {
		t.Fatalf("Answer Chains seed lost structural nearby-context claim:\n%s", seed)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersProseOnlyNearbyContextPolicy(t *testing.T) {
	mu := types.NewMutableState("")
	target := "explore_mid_loop_hint_budget"
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.SetExactContextRequiredFiles([]string{"internal/types/config.go"})
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			Subject:         "RuntimeSettings",
			Predicate:       "binds",
			Object:          "explore_midloop_min_iteration",
			Source:          "internal/config/runtime.go",
			LineStart:       231,
			ContextRole:     types.EvidenceContextRoleAbsenceSupport,
			GroundingStatus: types.GroundingGrounded,
		},
		{
			Kind:            types.EvidenceDirect,
			Subject:         "DefaultExploreHeuristics",
			Predicate:       "defines",
			Object:          "ExploreHeuristics defaults",
			Source:          "internal/types/config.go",
			LineStart:       707,
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		},
	})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{target},
					AllowAbsence:         true,
					RequireTargetMention: true,
					AliasRequiresProof:   true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Nearby Context Citation Policy",
		"validated nearby grounded context is prose-only",
		"Do NOT place nearby grounded context anchors into `citations[]` or fenced diagrams",
		"Keep `citations[]` on the primary exact-proof / absence-proof anchors only",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Surface-Allowed Grounded Context Anchors") {
		t.Fatalf("surface-allowed section should be suppressed when nearby context is prose-only:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopExactResolutionLockedRejectUsesMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_resolution] exact-resolution contract violated: the upstream investigation already closed as absence",
			Repair: &types.ToolRepair{
				Code: "exact_resolution",
				Metadata: map[string]string{
					"locked_status":          "absent",
					"preferred_context_mode": "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("locked exact-resolution reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"status is already locked by upstream state",
		"`exact_resolution.status=\"absent\"`",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
		"Do not switch to `exact_match` or `alias_match`",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("locked exact-resolution hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestRenderAnswerDocDiagramGradeExactContextAnchors_UsesAnswerChainPool(t *testing.T) {
	mu := types.NewMutableState("explore_mid_loop_hint_budget 的最终有效值是怎么计算出来的？")
	mu.SetInvestigationResultKind("absence")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Source:          "internal/config/runtime.go",
		LineStart:       32,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "RuntimeSettings",
		ContextRole:     types.EvidenceContextRoleAbsenceSupport,
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					AliasRequiresProof:   true,
					RequireTargetMention: true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
		AnswerChains: []types.AnswerChain{{
			Item: types.EvidenceItem{
				Kind:            types.EvidenceDirect,
				Source:          "internal/types/config.go",
				LineStart:       707,
				AnchorKind:      types.AnchorDefinition,
				AnchorSymbol:    "DefaultExploreHeuristics",
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
		}},
	}
	got := renderAnswerDocDiagramGradeExactContextAnchors(ctx, ctx.AnalysisIR.AnswerContract.ExactResolution)
	if !strings.Contains(got, "internal/types/config.go:707") {
		t.Fatalf("diagram-grade anchors should include answer-chain precedence anchors, got:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopExactContextSurfaceRejectUsesMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, diagramRequired: true}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_context_surface] summary leaked background-only anchors",
			Repair: &types.ToolRepair{
				Code: "exact_context_surface",
				Metadata: map[string]string{
					"repeated_target":   "`explore_mid_loop_hint_budget`",
					"forbidden_anchors": "ExploreBudget, internal/config/runtime.go",
					"allowed_anchors":   "DefaultExploreHeuristics(), codrax.yaml.example",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("exact-context-surface reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"Do NOT restate `explore_mid_loop_hint_budget` in `summary`",
		"renderer already prints the exact-target lead",
		"`ExploreBudget`, `internal/config/runtime.go`",
		"`DefaultExploreHeuristics()`, `codrax.yaml.example`",
		"A grounded diagram is still required for this dispatch",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("exact-context-surface hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersScalarLookupDiscipline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeValue,
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioGeneric,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Submission Checklist",
		"Fill `value.literal` and `value.citation_ref`",
		"Fill a real `summary` that names the subject being measured",
		"## Scalar Lookup Discipline",
		"one named source-code literal",
		"`shape=value` / `shape=config_value` / `shape=boolean` still require a real `summary`",
		"Do not expand into adjacent helpers",
		"every non-negative `citation_ref` must point at a real entry in `citations[]`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRoleLocateScalarDiscipline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeValue,
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioGeneric,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
				AnalyzerHints: types.AnalyzerHints{
					Kind: "return_value",
				},
				PredicateAxis: types.AxisReturn,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"role-locate lookup",
		"Do not promote the clue itself into the exact target lane",
		"answer with the located literal and its file:line first",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersLogTriageAndDiagramChecklist(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeExplanation,
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type: "runtime error: invalid memory address or nil pointer dereference",
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
					{File: "internal/agent/analyzer.go", Line: 320, Func: "ParseOutput"},
				},
			}},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Submission Checklist",
		"name each structured log error type or exception identifier from Log Triage",
		"Every file/path node you keep inside a fenced diagram must also be grounded by `citations[]` or by attached Log Triage frames",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersLogSourceDriftGuidance(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeExplanation,
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				},
			}},
		},
		EvidenceItems: []types.EvidenceItem{{
			Kind:            types.EvidenceDirect,
			Source:          "internal/agent/analyzer.go",
			LineStart:       612,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "buildAnalysisIR",
			GroundingStatus: types.GroundingGrounded,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Log Source Drift",
		"older or shifted build snapshot",
		"Do not claim that the current cited line is the exact crashing line from the log",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestCollectExactResolutionSeeds_FiltersDifferentConfigFamilies(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	ctx := &types.AgentContext{
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultLoopPolicy",
				Predicate:       "defines",
				Object:          "loop policy defaults",
				Summary:         "DefaultLoopPolicy returns loop-level defaults such as MaxMidLoopInjects=6.",
				Source:          "internal/agent/loop_policy.go",
				LineStart:       118,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	seeds := collectExactResolutionSeeds(ctx, contract)
	if len(seeds) == 0 {
		t.Fatal("expected exact-resolution seeds")
	}
	joined := make([]string, 0, len(seeds))
	for _, seed := range seeds {
		joined = append(joined, seed.Text)
	}
	text := strings.Join(joined, "\n")
	if !strings.Contains(text, "DefaultExploreHeuristics") {
		t.Fatalf("expected same-family explore seed, got: %s", text)
	}
	if strings.Contains(text, "DefaultLoopPolicy") {
		t.Fatalf("different config family should not survive exact-resolution seeds, got: %s", text)
	}
}

func TestCollectExactResolutionSeeds_ConfigTraceRequiresDiagramRoleForNearbyContext(t *testing.T) {
	contract := &types.ExactResolutionContract{
		TargetKind:           types.SubjectConfigKey,
		TargetLabel:          "config key",
		Targets:              []string{"explore_mid_loop_hint_budget"},
		AllowAbsence:         true,
		RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
		RelatedContextTerms:  []string{"explore"},
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Subject:         "ExploreBudget",
				Predicate:       "defines",
				Object:          "runtime budget counter",
				Summary:         "ExploreBudget is a runtime counter, not a config lineage anchor.",
				Source:          "internal/types/explore_budget.go",
				LineStart:       40,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "DefaultExploreHeuristics",
				Predicate:       "defines",
				Object:          "ExploreHeuristics defaults",
				Summary:         "DefaultExploreHeuristics defines the code defaults for explorer heuristics.",
				Source:          "internal/types/config.go",
				LineStart:       707,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceDirect,
				Subject:         "RuntimeSettings",
				Predicate:       "binds",
				Object:          "explore_midloop_min_iteration",
				Summary:         "RuntimeSettings binds the YAML override layer.",
				Source:          "internal/config/runtime.go",
				LineStart:       231,
				ContextRole:     types.EvidenceContextRoleRelatedContext,
				DiagramRole:     types.EvidenceDiagramRoleRuntime,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	seeds := collectExactResolutionSeeds(ctx, contract)
	if len(seeds) == 0 {
		t.Fatal("expected exact-resolution seeds")
	}
	var joined []string
	for _, seed := range seeds {
		joined = append(joined, seed.Text)
	}
	text := strings.Join(joined, "\n")
	if strings.Contains(text, "ExploreBudget") {
		t.Fatalf("same-family symbol without validated diagram role should be filtered in config-trace, got: %s", text)
	}
	for _, want := range []string{"DefaultExploreHeuristics", "RuntimeSettings"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected config-lineage seed %q, got: %s", want, text)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_UsesStableAbsenceStateAfterWindowReset(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.ResetInvestigationComplete()
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:              types.SubjectConfigKey,
					TargetLabel:             "config key",
					Targets:                 []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:            true,
					RequireTargetMention:    true,
					AliasRequiresProof:      true,
					RelatedContextPolicy:    types.ExactContextSameFamilyGrounded,
					RelatedContextScopeHint: "same namespace / prefix family",
				},
			},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
				AnalyzerHints: types.AnalyzerHints{
					Kind:         "config_mapping",
					ExactTargets: []string{"explore_mid_loop_hint_budget"},
				},
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"Investigation state: the exact target is currently absent in the repo / branch under inspection.",
		"no config key named `explore_mid_loop_hint_budget` exists in the repo",
		"Emit `exact_resolution.status=\"absent\"`",
		"do NOT force `shape=config_value` with a synthetic literal",
		"grounded same-scope anchors may appear in `summary` even when they do not carry a validated diagram role",
		"Prefer `shape=explanation`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q after reset:\n%s", want, prompt)
		}
	}
}

// TestAnswerDocumentEvaluator_LanguageCapture reads language from
// AgentContext.Language (set by BuildAgentContext from -lang flag).
func TestAnswerDocumentEvaluator_LanguageCapture(t *testing.T) {
	ctx := &types.AgentContext{Language: "zh"}
	e := &answerDocumentEvaluator{}
	e.BuildInitialInstruction(ctx, nil)
	if e.language != "zh" {
		t.Errorf("language = %q, want zh", e.language)
	}

	ctx2 := &types.AgentContext{Language: "en"}
	e2 := &answerDocumentEvaluator{}
	e2.BuildInitialInstruction(ctx2, nil)
	if e2.language != "en" {
		t.Errorf("language = %q, want en", e2.language)
	}

	ctx3 := &types.AgentContext{} // no language set
	e3 := &answerDocumentEvaluator{}
	e3.BuildInitialInstruction(ctx3, nil)
	if e3.language != "en" {
		t.Errorf("default language = %q, want en", e3.language)
	}
}

// softStopObs builds a minimal PhaseSoftStop LoopObservation for the
// Observe tests — all the answer-document evaluator cares about is
// Phase; the rest of the fields can stay zero.
func softStopObs(continuationCount int) LoopObservation {
	return LoopObservation{
		Phase:             PhaseSoftStop,
		Iteration:         0,
		ContinuationsUsed: continuationCount,
	}
}

// TestAnswerDocumentEvaluator_Observe_RetryBounded exercises the
// evaluator-owned retry budget. After maxFinalizerCorrectionRetries
// retries, Observe must stop returning HintRequested so the policy
// accepts the soft-stop. The retries counter is an
// evaluator-internal contract (fail-loud when the LLM stays off-
// contract after N corrections), distinct from LoopPolicy's
// MaxContinuations which applies loop-wide.
func TestAnswerDocumentEvaluator_Observe_RetryBounded(t *testing.T) {
	maxRetries := types.DefaultAgentSettings().FinalizerMaxCorrectionRetries
	e := &answerDocumentEvaluator{maxRetries: maxRetries}
	for i := 0; i < maxRetries; i++ {
		sig := e.Observe(nil, softStopObs(i))
		if !sig.HintRequested {
			t.Errorf("retry %d: HintRequested = false, want true (still within budget)", i)
		}
	}
	sig := e.Observe(nil, softStopObs(maxRetries))
	if sig.HintRequested {
		t.Error("after budget: HintRequested = true, want false")
	}
}

// TestAnswerDocumentEvaluator_Observe_AcceptsWhenDocPresent pins
// the simplify-round bug fix: when the LLM emits a tool call then
// soft-stops with a content-only turn, Observe must see the
// populated AnswerDocument in Mutable and NOT burn a retry.
// Without this guard, every successful emit followed by a
// free-text closer would trigger a correction retry that clobbers
// the first document on the second call.
func TestAnswerDocumentEvaluator_Observe_AcceptsWhenDocPresent(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAnswerDocument(&types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "landed tool call",
	})
	e := &answerDocumentEvaluator{mu: mu}
	sig := e.Observe(nil, softStopObs(0))
	if sig.HintRequested {
		t.Error("doc present in Mutable: HintRequested = true, want false (no retry)")
	}
	if e.retriesUsed != 0 {
		t.Errorf("doc-present path burned a retry: retriesUsed = %d, want 0", e.retriesUsed)
	}
}

// TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing is the
// complement: no doc in Mutable → Observe returns HintRequested.
func TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing(t *testing.T) {
	mu := types.NewMutableState("") // empty Mutable
	e := &answerDocumentEvaluator{mu: mu, maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, softStopObs(0))
	if !sig.HintRequested {
		t.Error("doc missing: HintRequested = false, want true")
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopSummaryCapRejectRequestsTargetedHint(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary length 2782 exceeds cap 2500 for shape=explanation — shorten the summary",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("summary-cap reject should request a correction hint, got %+v", sig)
	}
	if !sig.BypassThrottle {
		t.Fatalf("summary-cap reject hint should bypass throttle, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "2500") || !strings.Contains(sig.Hint, "explanation") {
		t.Fatalf("targeted summary-cap hint missing cap/shape detail: %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "emit_answer_document") {
		t.Fatalf("targeted summary-cap hint must tell the model to re-emit the tool call: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopSummaryCapRejectPreservesRequiredDiagram(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, diagramRequired: true}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary length 2782 exceeds cap 2500 for shape=explanation — shorten the summary",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("summary-cap reject should request a correction hint, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "Preserve the required grounded diagram") {
		t.Fatalf("diagram-required summary-cap hint must preserve the diagram: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopMissingDiagramRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "diagram required for this dispatch (preferred kinds: call_dag); summary must include at least 1 grounded triple-backtick diagram block. This obligation is independent of answer shape.",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("missing-diagram reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"grounded diagram",
		"independent of answer shape",
		"emit_answer_document",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("missing-diagram hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopMissingDiagramRejectIncludesConfigTraceSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				RequiredAnswerShape: types.ShapeExplanation,
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, Summary: "code defaults", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, Subject: "ExploreHeuristics", Summary: "yaml precedence comment", Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleYAML},
			{Source: "internal/config/runtime.go", LineStart: 194, Summary: "runtime yaml binding", Kind: types.EvidenceDirect, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
		},
	}
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(ctx, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "diagram required for this dispatch (preferred kinds: architecture, flow); summary must include at least 1 grounded triple-backtick diagram block(s). This obligation is independent of answer shape.",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("missing-diagram reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"copying the seeded grounded precedence chain verbatim",
		"copy this seeded fenced diagram verbatim",
		"```",
		"internal/config/runtime.go:194",
		"internal/types/config.go:707",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("missing-diagram config-trace hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestRenderRetryDiagramSeedFence_UsesLogSeedForCallDAG(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
				},
			},
		},
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 320, Func: "buildAnalysisIR"},
					{File: "internal/orchestrator/orchestrator.go", Line: 101, Func: "Run"},
				},
			}},
		},
	}
	got := renderRetryDiagramSeedFence(ctx)
	for _, want := range []string{
		"```",
		"innermost failure: internal/agent/analyzer.go:320 in buildAnalysisIR",
		"caller (outermost): internal/orchestrator/orchestrator.go:101 in Run",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("call_dag retry seed missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRetryDiagramSeedFence_UsesFlowFindingSeedForFlow(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		FlowFindings: []types.FlowFindingDigest{{
			Path: []string{"config.handlers.explorer", "NewExplorer", "Register"},
		}},
	}
	got := renderRetryDiagramSeedFence(ctx)
	for _, want := range []string{
		"```",
		"config.handlers.explorer",
		"NewExplorer",
		"Register",
		"  ->",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("flow retry seed missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRetryDiagramSeedFence_UsesAnswerChainSeedForArchitecture(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramArchitecture},
				},
			},
		},
		AnswerChains: []types.AnswerChain{
			{Item: types.EvidenceItem{Source: "internal/a.go", LineStart: 10, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Source: "internal/b.go", LineStart: 20, GroundingStatus: types.GroundingGrounded}},
		},
	}
	got := renderRetryDiagramSeedFence(ctx)
	for _, want := range []string{
		"```",
		"internal/a.go:10",
		"internal/b.go:20",
		"  ->",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("architecture retry seed missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_ConfigTraceRejectKeepsValidatedPrecedenceSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, DiagramRole: types.EvidenceDiagramRoleDefault, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics"},
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration"},
		},
		AnswerChains: []types.AnswerChain{
			{Item: types.EvidenceItem{Source: "cmd/root.go", LineStart: 2036, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Source: "internal/analysis/declarative/classifier.go", LineStart: 66, GroundingStatus: types.GroundingGrounded}},
		},
	}
	repair := &types.ToolRepair{
		Code: "config_trace_context_citation",
		Metadata: map[string]string{
			"allowed_citations": "internal/config/runtime.go:231, internal/types/config.go:707",
		},
	}
	got := renderRetryDiagramSeedFenceForRepair(ctx, repair)
	for _, want := range []string{
		"internal/config/runtime.go:231",
		"internal/types/config.go:707",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config-trace repair seed missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{
		"cmd/root.go:2036",
		"internal/analysis/declarative/classifier.go:66",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("config-trace repair seed should not fall back to unrelated answer chain %q:\n%s", banned, got)
		}
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_ConfigTraceRejectOmitsUnrelatedFallbackWhenNoValidatedPrecedenceSeed(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioConfigTrace,
			},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		},
		AnswerChains: []types.AnswerChain{
			{Item: types.EvidenceItem{Source: "cmd/root.go", LineStart: 2036, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Source: "internal/analysis/declarative/classifier.go", LineStart: 66, GroundingStatus: types.GroundingGrounded}},
		},
	}
	repair := &types.ToolRepair{
		Code: "config_trace_context_citation",
		Metadata: map[string]string{
			"allowed_citations": "internal/config/runtime.go:231",
		},
	}
	if got := renderRetryDiagramSeedFenceForRepair(ctx, repair); got != "" {
		t.Fatalf("expected no repair seed when native precedence chain is incomplete, got:\n%s", got)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopDiagramGroundingRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary fenced code block references file(s) not present in citations[] or attached-log frames: codrax.yaml. ASCII diagrams are structural claims.",
			Repair: &types.ToolRepair{
				Code: "diagram_grounding",
				Hint: "Re-emit `emit_answer_document` with the same grounded answer, but inside fenced diagrams keep file/path node labels to the exact grounded allowlist for this dispatch. If a node has no grounded label, remove it from the fence and explain that relationship in prose instead.",
				Metadata: map[string]string{
					"allowed_labels": "cmd/root.go, internal/config/runtime.go, internal/types/config.go",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("diagram-grounding reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"DIAGRAM-GROUNDING",
		"reuse the exact grounded file / symbol / path labels",
		"Diagram Node Allowlist",
		"`cmd/root.go`, `internal/config/runtime.go`, `internal/types/config.go`",
		"Do NOT normalize one grounded label into a different spelling",
		"Do NOT call `read_file`, `grep`, or any other tool",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("diagram-grounding hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopConfigTraceContextCitationRejectSurfacesAllowedAndForbiddenAnchors(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(&types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
			{Source: "internal/types/config.go", LineStart: 707, DiagramRole: types.EvidenceDiagramRoleDefault, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		},
	}, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:config_trace_context_citation] exact-absent config-trace answers may cite only precedence-capable lineage anchors.",
			Repair: &types.ToolRepair{
				Code: "config_trace_context_citation",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but if `summary` continues to explain nearby precedence / lineage context, keep at least one grounded precedence anchor in `citations[]`.",
				Metadata: map[string]string{
					"allowed_citations": "internal/config/runtime.go:231, internal/types/config.go:707",
					"allowed_anchors":   "DefaultExploreHeuristics, internal/config/runtime.go",
					"forbidden_anchors": "ExploreBudget",
					"drop_citations":    "internal/types/explore_budget.go:40",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("config-trace context-citation reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"Only these grounded file:line anchors may appear in `citations[]` or fenced diagrams",
		"`internal/config/runtime.go:231`, `internal/types/config.go:707`",
		"Visible nearby context may only use this validated anchor set",
		"`DefaultExploreHeuristics`, `internal/config/runtime.go`",
		"Being visible does NOT make every anchor citation-grade",
		"Drop any prose / diagram node whose only support comes from these background-only anchors",
		"`ExploreBudget`",
		"Drop these invalid citation(s) from `citations[]`",
		"`internal/types/explore_budget.go:40`",
		"Choose one valid repair path now",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("config-trace context-citation hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopConfigTraceContextCitationRejectPreservesProseOnlyAnchors(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(&types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/config/runtime.go", LineStart: 231, DiagramRole: types.EvidenceDiagramRoleRuntime, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
			{Source: "internal/types/config.go", LineStart: 707, Kind: types.EvidenceDirect, GroundingStatus: types.GroundingGrounded},
		},
	}, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:config_trace_context_citation] exact-absent config-trace answers may cite only precedence-capable lineage anchors.",
			Repair: &types.ToolRepair{
				Code: "config_trace_context_citation",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but remove this anchor from `citations[]` and from any fenced diagram nodes. You may keep it in `summary` as prose-only grounded nearby context, but if the user-visible answer still explains precedence / lineage, cite at least one validated default/config/runtime/override anchor.",
				Metadata: map[string]string{
					"allowed_citations":            "internal/config/runtime.go:231",
					"allowed_anchors":              "DefaultExploreHeuristics, internal/config/runtime.go",
					"prose_only_anchors":           "DefaultExploreHeuristics",
					"drop_citations":               "internal/types/config.go:707",
					"nearby_context_citation_mode": "prose_only",
					"preferred_context_mode":       "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("config-trace prose-only citation reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"treat the nearby grounded context as prose-only for this dispatch",
		"Only these grounded file:line anchors may appear in `citations[]` or fenced diagrams",
		"`internal/config/runtime.go:231`",
		"Visible nearby context may only use this validated anchor set",
		"`DefaultExploreHeuristics`, `internal/config/runtime.go`",
		"Being visible does NOT make every anchor citation-grade",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
		"may stay on the user-visible answer surface as uncited prose-only grounded context",
		"`DefaultExploreHeuristics`",
		"Drop these invalid citation(s) from `citations[]`",
		"`internal/types/config.go:707`",
		"Choose one valid repair path now",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("config-trace prose-only context-citation hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopConfigTraceContextCitationRejectKeepsFollowOnContextVisible(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	mu.AppendEvidence([]types.EvidenceItem{{
		Kind:            types.EvidenceDirect,
		Subject:         "DefaultExploreHeuristics",
		Predicate:       "defines",
		Object:          "ExploreHeuristics defaults",
		Source:          "internal/types/config.go",
		LineStart:       707,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "DefaultExploreHeuristics",
		ContextRole:     types.EvidenceContextRoleRelatedContext,
		GroundingStatus: types.GroundingGrounded,
	}})
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(&types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
		},
	}, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:config_trace_context_citation] exact-absent config-trace answers may cite only precedence-capable lineage anchors.",
			Repair: &types.ToolRepair{
				Code: "config_trace_context_citation",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but remove this anchor from `citations[]`.",
				Metadata: map[string]string{
					"allowed_anchors":              "DefaultExploreHeuristics",
					"drop_citations":               "internal/types/config.go:707",
					"nearby_context_citation_mode": "prose_only",
					"preferred_context_mode":       "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("follow-on grounded-context citation reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"follow-on grounded-context mode",
		"Keep the nearby grounded context visible as uncited prose-only explanation instead of collapsing to the exact-absence lead alone",
		"Keep the nearby grounded context visible as uncited prose-only explanation",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("follow-on grounded-context hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopFollowOnGroundedContextRejectUsesAllowedAnchors(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:follow_on_grounded_context] exact-absent answers in follow-on grounded-context mode collapsed to the lead only.",
			Repair: &types.ToolRepair{
				Code: "follow_on_grounded_context",
				Hint: "Re-emit `emit_answer_document` with the same exact-absence conclusion, but keep the grounded nearby context visible after the renderer-generated lead.",
				Metadata: map[string]string{
					"allowed_anchors":        "DefaultExploreHeuristics, ResolvedExploreHeuristics",
					"preferred_context_mode": "grounded_context_only",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("follow-on grounded-context reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"`DefaultExploreHeuristics`, `ResolvedExploreHeuristics`",
		"`exact_resolution.context_mode=\"grounded_context_only\"`",
		"Do not collapse the answer to the renderer-generated exact-absence lead alone",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("follow-on grounded-context hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopDiagramCodenameRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2, configTraceDiagram: true}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary introduces codename label(s) not present in any citation's ±3-line window: Level 1, Level 2.",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("diagram-codename reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"CODENAME-GROUNDING",
		"Level 1",
		"Label the diagram directly with grounded files, functions, config keys",
		"Do NOT call `read_file`, `grep`, or any other tool",
		"defaults / config-file load / runtime binding / operator override",
		"move that explanation into prose outside the fenced diagram",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("diagram-codename hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopExactResolutionRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:exact_resolution] exact-resolution contract violated: summary must explicitly name the requested exact config key and lead with its absence before any nearby context.",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("exact-resolution reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"absence-only is acceptable",
		"requested exact target",
		"related context",
		"equivalent, alias, or substitute",
		"Do NOT call `read_file`, `grep`, or any other tool",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("exact-resolution hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopAbsentExactConfigValueShapeRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:absent_exact_config_value_shape] exact absent config-key answers must not use shape=config_value with a synthetic missing literal; use shape=explanation so the answer can lead with the exact absence",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("absent exact config-value reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"shape=explanation",
		"exact_resolution.status=\"absent\"",
		"related context only",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("absent config-value hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopUnexpectedReadToolRequestsSynthesisOnlyRetry(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 3,
		Response: llm.Response{
			ToolCalls: []llm.ToolCall{{ID: "1", Name: "read_file"}},
		},
		LastToolResult: &types.ToolResult{
			ToolName: "read_file",
			Success:  false,
			Summary:  "invalid params",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("unexpected read tool should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"pure synthesizer",
		"Do NOT call `read_file`",
		"emit_answer_document",
		"Diagram Node Allowlist",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("unexpected-tool hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopGenericRejectSurfacesToolError(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "symbols[0].file is required when symbols[0].line is set\nextra detail follows",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("generic reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"symbols[0].file is required when symbols[0].line is set",
		"emit_answer_document",
		"Only change the named field(s)",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("generic reject hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopStructuredRepairHintUsesRepairMetadata(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "validation failed",
			Repair: &types.ToolRepair{
				Code:   "scalar_summary_required",
				Fields: []string{"summary", "value.literal"},
				Hint:   "Re-emit `emit_answer_document` with the same scalar payload, keep the grounded literal and citation unchanged, and expand `summary` so it names the measured subject and how the value was obtained. Do not reopen files or change the answer shape.",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("structured repair should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"summary",
		"value.literal",
		"same scalar payload",
		"Do not reopen files or change the answer shape",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("structured repair hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopValueSummaryRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  `shape=value requires summary to name the subject (file path / symbol / directory / measurement target) AND the methodology (command, chain, lookup) that produced the literal — the bare literal "buildAnalysisIR" alone is not a complete answer`,
		},
	})
	if !sig.HintRequested {
		t.Fatalf("value-summary reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"`shape=value`",
		"missing the required `summary`",
		"keep the grounded `value.literal` / `value.citation_ref`",
		"Do NOT reopen files or change the answer shape",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("value-summary hint missing %q: %q", want, sig.Hint)
		}
	}
}

// TestAnswerDocumentEvaluator_Observe_MidLoopLiteralGroundingRejectSurfacesAction
// pins the session-22 in-dispatch self-correction nudge: when the
// literal-grounding gate rejects a value-shape citation, the
// mid-loop reject hint must surface the single-action fix
// ("citation_ref=-1 + summary caveat") at the TOP of the hint so
// the LLM stops trying more fabrications and reaches for the
// escape. Without this special-case, the generic "fix the exact
// validation error" hint buried the action behind diagnostic
// prose and the LLM burned the full retry budget on fresh
// fabrications before the dispatch exited (observed: 16 min on
// the partial eval case).
func TestAnswerDocumentEvaluator_Observe_MidLoopLiteralGroundingRejectSurfacesAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 3}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  `value.literal "processRequest" is not corroborated by citations[0] (internal/agent/analyzer.go:1): the cited line and ±3-line window contain no identifier overlap with the literal. If this literal originates from the attached log / external source rather than repo code, set citation_ref=-1 and state in summary that the answer is derived from log semantics (no grounded repo source). Otherwise cite a real file:line where the literal appears.`,
		},
	})
	if !sig.HintRequested {
		t.Fatalf("literal-grounding reject should request a correction hint, got %+v", sig)
	}
	// The action must appear BEFORE the diagnostic prose so the LLM
	// acts on it before scrolling past.
	actionIdx := strings.Index(sig.Hint, "citation_ref = -1")
	diagIdx := strings.Index(sig.Hint, "Full tool error")
	if actionIdx < 0 || diagIdx < 0 || actionIdx > diagIdx {
		t.Fatalf("citation_ref=-1 action must appear before diagnostic body "+
			"(action at %d, diagnostic at %d); hint:\n%s",
			actionIdx, diagIdx, sig.Hint)
	}
	if !strings.Contains(sig.Hint, "LITERAL-GROUNDING") {
		t.Errorf("hint should name the gate so operators can trace the signal: %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "external source") {
		t.Errorf("hint should surface the 'external source' escape rationale: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopLiteralGroundingRejectSurfacesStepAction(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 3}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  `steps[0].description "searched the repo and found no production definition" is not corroborated by citations[0] (internal/tool/emit_answer_document_test.go:805): the cited line and ±3-line window contain no identifier overlap with the claim. If this step paraphrases an aggregate absence conclusion rather than one corroborated line, set citation_ref=-1 so the renderer drops the suffix.`,
		},
	})
	if !sig.HintRequested {
		t.Fatalf("step literal-grounding reject should request a correction hint, got %+v", sig)
	}
	actionIdx := strings.Index(sig.Hint, "steps[0].citation_ref = -1")
	diagIdx := strings.Index(sig.Hint, "Full tool error")
	if actionIdx < 0 || diagIdx < 0 || actionIdx > diagIdx {
		t.Fatalf("steps[0].citation_ref=-1 action must appear before diagnostic body (action at %d, diagnostic at %d); hint:\n%s",
			actionIdx, diagIdx, sig.Hint)
	}
	for _, want := range []string{
		"LITERAL-GROUNDING",
		"`steps[0].description`",
		"repo-wide search, aggregate absence, test-only proof",
		"Do NOT try to borrow a nearby file:line",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("step literal-grounding hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopRejectStopsHintingAfterBudget(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 1}
	obs := LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "summary length 2782 exceeds cap 2500 for shape=explanation — shorten the summary",
		},
	}
	if sig := e.Observe(nil, obs); !sig.HintRequested {
		t.Fatalf("first reject should request a correction hint, got %+v", sig)
	}
	if sig := e.Observe(nil, obs); !sig.HintRequested {
		t.Fatalf("tool-level rejects should continue surfacing repair hints beyond the first correction budget round, got %+v", sig)
	}
	for i := 0; i < e.rejectHintBudget()-2; i++ {
		if sig := e.Observe(nil, obs); !sig.HintRequested {
			t.Fatalf("repair hint %d should still be available before the extended budget is exhausted, got %+v", i+3, sig)
		}
	}
	if sig := e.Observe(nil, obs); sig.HintRequested {
		t.Fatalf("after the extended reject-hint budget, evaluator should stay silent, got %+v", sig)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_Happy — a fully-populated
// AnswerDocument in Mutable is rendered into FinalAnswer.
func TestAnswerDocumentEvaluator_ParseOutput_Happy(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	doc := &types.AnswerDocument{
		Shape:     types.ShapeValue,
		Value:     &types.AnswerValue{Literal: "explorer", CitationRef: 0},
		Citations: []types.Citation{{File: "a.go", Line: 42}},
	}
	ctx.Mutable.SetAnswerDocument(doc)

	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if out.FinalAnswer == "" {
		t.Fatal("FinalAnswer empty")
	}
	if !strings.Contains(out.FinalAnswer, "`explorer`") {
		t.Errorf("FinalAnswer missing literal: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "a.go:42") {
		t.Errorf("FinalAnswer missing citation: %q", out.FinalAnswer)
	}
	// Data payload carries the structured doc for debugging.
	var payload struct {
		FinalAnswer    string          `json:"final_answer"`
		AnswerDocument json.RawMessage `json:"answer_document"`
	}
	if err := json.Unmarshal(out.Data, &payload); err != nil {
		t.Fatalf("unmarshal data: %v", err)
	}
	if len(payload.AnswerDocument) == 0 || string(payload.AnswerDocument) == "null" {
		t.Error("Data.answer_document is empty/null")
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoud covers
// the fail-loud path: no document, retries exhausted, ParseOutput
// surfaces a warning banner prefixed to the raw content.
func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoud(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "raw fallback text"},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if !strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Errorf("fail-loud warning missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "raw fallback text") {
		t.Errorf("raw content lost: %q", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_SanitizesFallback(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{
		{
			Role: "assistant",
			Content: "<think>internal reasoning</think>\n\n" +
				"Grounded user-facing answer.\n\n" +
				"<minimax:tool_call>\n" +
				"{\"shape\":\"explanation\"}\n" +
				"</minimax:tool_call>\n",
		},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "<think>") || strings.Contains(out.FinalAnswer, "<minimax:tool_call>") || strings.Contains(out.FinalAnswer, "\"shape\"") {
		t.Fatalf("fail-loud fallback leaked internal scaffolding: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "Grounded user-facing answer.") {
		t.Fatalf("sanitized fallback lost user-facing content: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_CardinalityDowngrade is the
// critical P2.2 test: when Shape == list_of_symbols + Completeness ==
// complete + len(Symbols) < baseline, ParseOutput downgrades to
// lower_bound and appends a caveat. Reuses the same validator as
// extractorEvaluator so this test pins the cross-stage contract.
func TestAnswerDocumentEvaluator_ParseOutput_CardinalityDowngrade(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"Alpha", "Beta", "Gamma", "Delta"}, // baseline = 4
			},
		},
	}
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{ // only 2 — below baseline of 4
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	ctx.Mutable.SetAnswerDocument(doc)

	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, nil, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if out.AnswerSymbolCompleteness != types.CompletenessLowerBound {
		t.Errorf("completeness = %q, want lower_bound (downgrade)", out.AnswerSymbolCompleteness)
	}
	if !strings.Contains(out.FinalAnswer, "confirmed items") {
		t.Errorf("downgraded rendering footer tag missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "downgraded to lower_bound") {
		t.Errorf("downgrade caveat missing: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade — completeness
// complete with enough symbols passes through unchanged.
func TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"Alpha"}, // baseline = 1
			},
		},
	}
	doc := &types.AnswerDocument{
		Shape:               types.ShapeListOfSymbols,
		SymbolsCompleteness: types.CompletenessComplete,
		Symbols: []types.AnswerSymbol{
			{Name: "Alpha", File: "a.go", Line: 10},
			{Name: "Beta", File: "b.go", Line: 20},
		},
	}
	ctx.Mutable.SetAnswerDocument(doc)
	e := &answerDocumentEvaluator{language: "en"}
	out, _ := e.ParseOutput(ctx, nil, nil, nil)
	if out.AnswerSymbolCompleteness != types.CompletenessComplete {
		t.Errorf("completeness = %q, want complete (no downgrade)", out.AnswerSymbolCompleteness)
	}
	// Complete answers have no completeness tag — symbols listed directly.
	if strings.Contains(out.FinalAnswer, "Complete answer") {
		t.Errorf("complete should not have header in body: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "**Alpha**") {
		t.Errorf("symbol missing: %q", out.FinalAnswer)
	}
}

// richDraftProse is a ~1500-char explanation the LLM writes as plain
// prose in its first attempt — the kind of answer the shrinkage
// salvage must preserve when the correction retry emits a compressed
// paraphrase. Kept as a test helper so every shrinkage case shares
// one fixture and the length floor matches the 400-char threshold.
func richDraftProse() string {
	return strings.Repeat(
		"The dispatcher walks the handler chain and delegates to the registered listener. ",
		20) // 20 * ~78 chars = ~1560 chars, well above the 400-char floor
}

// parseStageDoc reads the mutated AnswerDocument back out of the
// StageOutput's JSON Data payload. MutableState.AnswerDocument()
// returns a defensive clone on every call, so the mutations the
// salvage applies inside ParseOutput are only observable through the
// StageOutput it returns.
func parseStageDoc(t *testing.T, out *StageOutput) *types.AnswerDocument {
	t.Helper()
	var payload struct {
		AnswerDocument *types.AnswerDocument `json:"answer_document"`
	}
	if err := json.Unmarshal(out.Data, &payload); err != nil {
		t.Fatalf("unmarshal stage data: %v", err)
	}
	if payload.AnswerDocument == nil {
		t.Fatal("stage data.answer_document is nil")
	}
	return payload.AnswerDocument
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage — the
// positive case. Explanation shape + rich pre-tool-call draft + a
// short compressed summary in the emitted AnswerDocument. ParseOutput
// must overwrite Summary with the prior draft (trimmed to the
// explanation cap) and append a user-visible caveat.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	doc := &types.AnswerDocument{
		Shape:   types.ShapeExplanation,
		Summary: "The dispatcher delegates to the listener.", // ~41 chars, way under 50% of ~1560
	}
	ctx.Mutable.SetAnswerDocument(doc)

	messages := []llm.Message{
		{Role: "user", Content: "explain the dispatch flow"},
		{Role: "assistant", Content: richDraftProse()}, // no ToolCalls — pre-tool-call draft
		{Role: "user", Content: "[correction hint elided]"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}

	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if len(got.Summary) < 400 {
		t.Errorf("summary not salvaged: len=%d, expected prior draft copied in", len(got.Summary))
	}
	if !strings.Contains(got.Summary, "dispatcher") {
		t.Errorf("summary does not contain prior draft content: %q", got.Summary)
	}
	foundCaveat := false
	for _, c := range got.Caveats {
		if strings.Contains(c, "richer prior draft") {
			foundCaveat = true
		}
	}
	if !foundCaveat {
		t.Errorf("salvage caveat missing: %v", got.Caveats)
	}
	if !strings.Contains(out.FinalAnswer, "dispatcher") {
		t.Errorf("FinalAnswer missing salvaged content: %q", out.FinalAnswer)
	}
}

func TestSanitizePriorDraftForSummary_StripsInternalScaffolding(t *testing.T) {
	cleanTail := strings.Repeat("入口函数负责把请求整理成结构化 IR。", 40)
	in := `<think>internal reasoning</think>

Translation: explain the answer

` + "```json\n{\"shape\":\"value\",\"summary\":\"x\",\"citations\":[]}\n```" + `

I need to emit exactly one emit_answer_document tool call.

<minimax:tool_call>
<invoke name="emit_answer_document">
<parameter name="shape">value</parameter>
</invoke>
</minimax:tool_call>

` + cleanTail
	got := sanitizePriorDraftForSummary(in)
	if strings.Contains(got, "<think>") || strings.Contains(got, "emit_answer_document") ||
		strings.Contains(got, "\"shape\":") || strings.Contains(got, "<minimax:tool_call>") {
		t.Fatalf("sanitizePriorDraftForSummary leaked internal scaffolding: %q", got)
	}
	if !strings.Contains(got, "结构化 IR") {
		t.Fatalf("sanitizePriorDraftForSummary dropped user-facing tail: %q", got)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_AllShapes —
// positive cross-shape check. Summary is framing text for every
// shape (the body for explanation; a 1-3-sentence lead-in for
// structured shapes). When the LLM compresses its prior draft well
// below the per-shape floor + ratio, the salvage fires, Summary is
// overwritten, caveat is appended, and the structured payload is
// left untouched.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_AllShapes(t *testing.T) {
	cases := []struct {
		shape       types.AnswerShape
		origSummary string
		seed        func(doc *types.AnswerDocument)
		check       func(t *testing.T, doc *types.AnswerDocument)
	}{
		{
			shape:       types.ShapeStepList,
			origSummary: "short lead-in",
			seed: func(doc *types.AnswerDocument) {
				doc.Steps = []types.AnswerStep{{Index: 1, Description: "d", CitationRef: types.CitationRefUnset}}
			},
			check: func(t *testing.T, doc *types.AnswerDocument) {
				if len(doc.Steps) != 1 || doc.Steps[0].Description != "d" {
					t.Errorf("structured payload mutated by salvage: %+v", doc.Steps)
				}
			},
		},
		{
			shape:       types.ShapeListOfSymbols,
			origSummary: "short lead-in",
			seed: func(doc *types.AnswerDocument) {
				doc.Symbols = []types.AnswerSymbol{{Name: "X", File: "a.go", Line: 1}}
			},
			check: func(t *testing.T, doc *types.AnswerDocument) {
				if len(doc.Symbols) != 1 || doc.Symbols[0].Name != "X" {
					t.Errorf("structured payload mutated by salvage: %+v", doc.Symbols)
				}
			},
		},
		{
			shape:       types.ShapeBoolean,
			origSummary: "short lead-in",
			seed: func(doc *types.AnswerDocument) {
				doc.Boolean = &types.AnswerBoolean{Decision: true, Rationale: "r", CitationRef: types.CitationRefUnset}
			},
			check: func(t *testing.T, doc *types.AnswerDocument) {
				if doc.Boolean == nil || !doc.Boolean.Decision || doc.Boolean.Rationale != "r" {
					t.Errorf("structured payload mutated by salvage: %+v", doc.Boolean)
				}
			},
		},
		{
			shape:       types.ShapeValue,
			origSummary: "short",
			seed: func(doc *types.AnswerDocument) {
				doc.Value = &types.AnswerValue{Literal: "v", CitationRef: types.CitationRefUnset}
			},
			check: func(t *testing.T, doc *types.AnswerDocument) {
				if doc.Value == nil || doc.Value.Literal != "v" {
					t.Errorf("structured payload mutated by salvage: %+v", doc.Value)
				}
			},
		},
		{
			shape:       types.ShapeConfigValue,
			origSummary: "short",
			seed: func(doc *types.AnswerDocument) {
				doc.Value = &types.AnswerValue{Key: "k", Literal: "v", CitationRef: types.CitationRefUnset}
			},
			check: func(t *testing.T, doc *types.AnswerDocument) {
				if doc.Value == nil || doc.Value.Key != "k" || doc.Value.Literal != "v" {
					t.Errorf("structured payload mutated by salvage: %+v", doc.Value)
				}
			},
		},
	}
	for _, c := range cases {
		t.Run(string(c.shape), func(t *testing.T) {
			ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
			doc := &types.AnswerDocument{Shape: c.shape, Summary: c.origSummary}
			c.seed(doc)
			ctx.Mutable.SetAnswerDocument(doc)
			messages := []llm.Message{
				{Role: "assistant", Content: richDraftProse()},
				{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
			}
			e := &answerDocumentEvaluator{language: "en"}
			out, err := e.ParseOutput(ctx, messages, nil, nil)
			if err != nil {
				t.Fatalf("ParseOutput err: %v", err)
			}
			got := parseStageDoc(t, out)
			if got.Summary == c.origSummary {
				t.Errorf("Summary was NOT overwritten — salvage did not fire for shape=%s", c.shape)
			}
			if !strings.Contains(got.Summary, "dispatcher") {
				t.Errorf("Summary missing salvaged content: %q", got.Summary)
			}
			itemCount := len(got.Steps) + len(got.Symbols)
			if cap := types.SummaryCapFor(c.shape, itemCount); len(got.Summary) > cap {
				t.Errorf("Summary exceeds cap: len=%d, cap=%d", len(got.Summary), cap)
			}
			foundCaveat := false
			for _, cv := range got.Caveats {
				if strings.Contains(cv, "richer prior draft") {
					foundCaveat = true
				}
			}
			if !foundCaveat {
				t.Errorf("salvage caveat missing: %v", got.Caveats)
			}
			c.check(t, got)
		})
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_BelowFloorPerShape —
// per-shape floor check. shrinkageThresholdsForShape scales the
// prior-draft length floor off Summary's role in each shape: 1.0×
// for explanation, 0.5× for step_list/list_of_symbols, 3/8× for
// boolean, 0.25× for value/config_value. A prior draft that falls
// just under the scaled floor must NOT trigger salvage, even though
// it would for a shape with a smaller floor.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_BelowFloorPerShape(t *testing.T) {
	cases := []struct {
		shape    types.AnswerShape
		priorLen int // just below shape's scaled floor
		seed     func(doc *types.AnswerDocument)
	}{
		// baseline = 400; step_list / list_of_symbols floor = 200; use 180.
		{
			shape:    types.ShapeStepList,
			priorLen: 180,
			seed: func(doc *types.AnswerDocument) {
				doc.Steps = []types.AnswerStep{{Index: 1, Description: "d", CitationRef: types.CitationRefUnset}}
			},
		},
		// value / config_value floor = 100; use 80.
		{
			shape:    types.ShapeValue,
			priorLen: 80,
			seed: func(doc *types.AnswerDocument) {
				doc.Value = &types.AnswerValue{Literal: "v", CitationRef: types.CitationRefUnset}
			},
		},
	}
	for _, c := range cases {
		t.Run(string(c.shape), func(t *testing.T) {
			ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
			origSummary := "short"
			doc := &types.AnswerDocument{Shape: c.shape, Summary: origSummary}
			c.seed(doc)
			ctx.Mutable.SetAnswerDocument(doc)
			prior := strings.Repeat("x", c.priorLen)
			messages := []llm.Message{
				{Role: "assistant", Content: prior},
				{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
			}
			e := &answerDocumentEvaluator{language: "en"}
			out, err := e.ParseOutput(ctx, messages, nil, nil)
			if err != nil {
				t.Fatalf("ParseOutput err: %v", err)
			}
			got := parseStageDoc(t, out)
			if got.Summary != origSummary {
				t.Errorf("shape=%s: Summary was overwritten despite prior draft below scaled floor (len=%d): %q",
					c.shape, c.priorLen, got.Summary)
			}
		})
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ShortPriorDraft —
// negative length-floor check. When the prior draft is below the
// minimum prose length (400 chars), the salvage must not fire.
// Otherwise a one-line placeholder pre-tool-call content would start
// overwriting every answer.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ShortPriorDraft(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	origSummary := "The dispatcher delegates to the listener."
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: origSummary}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: "short draft, well under 400 chars"},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Errorf("Summary was overwritten despite short prior draft: %q", got.Summary)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_NotShrunk —
// negative ratio check. When the emitted Summary is already at least
// half the length of the prior draft, the salvage must not fire —
// the LLM did NOT compress this time.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_NotShrunk(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	// Emitted summary is 80% of prior draft — within tolerance, no salvage.
	prior := richDraftProse()
	origSummary := prior[:int(float64(len(prior))*0.8)]
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: origSummary}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Errorf("Summary was overwritten despite ratio above threshold: len=%d → %d", len(origSummary), len(got.Summary))
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_SkipsScaffoldingOnlyScalarDraft(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	origSummary := "入口函数是 `buildAnalysisIR`，定义在 `internal/agent/analyzer.go:565`。"
	doc := &types.AnswerDocument{
		Shape:   types.ShapeValue,
		Summary: origSummary,
		Value:   &types.AnswerValue{Literal: "buildAnalysisIR", CitationRef: types.CitationRefUnset},
	}
	ctx.Mutable.SetAnswerDocument(doc)
	prior := "<think>\n" +
		"The user is asking about the exact entry function.\n" +
		"</think>\n\n" +
		"```json\n{\"shape\":\"value\",\"summary\":\"x\",\"citations\":[{\"file\":\"internal/agent/analyzer.go\",\"line\":565}]}\n```\n\n" +
		"I need to emit exactly one emit_answer_document tool call.\n\n" +
		"<minimax:tool_call>\n" +
		"<invoke name=\"emit_answer_document\">\n" +
		"<parameter name=\"shape\">value</parameter>\n" +
		"</invoke>\n" +
		"</minimax:tool_call>\n\n" +
		"值为 `buildAnalysisIR` (`internal/agent/analyzer.go:565`)."
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Fatalf("scalar salvage should skip scaffolding-heavy draft: got %q want %q", got.Summary, origSummary)
	}
	if len(got.Caveats) != 0 {
		t.Fatalf("scalar salvage should not append caveat when sanitized draft is too short: %v", got.Caveats)
	}
	if strings.Contains(out.FinalAnswer, "<think>") || strings.Contains(out.FinalAnswer, "<minimax:tool_call>") {
		t.Fatalf("FinalAnswer leaked scaffolding: %q", out.FinalAnswer)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CapTrim —
// a prior draft exceeding SummaryCapFor(ShapeExplanation, 0) must be
// trimmed, not rejected — the salvage's job is best-effort recovery.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CapTrim(t *testing.T) {
	enableSummaryCapsForTest(t)
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	cap := types.SummaryCapFor(types.ShapeExplanation, 0)
	// Build a draft 1.5x the cap.
	prior := strings.Repeat("a", cap*3/2)
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "x"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if len(got.Summary) != cap {
		t.Errorf("Summary not trimmed to cap: len=%d, cap=%d", len(got.Summary), cap)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CJKRuneBoundary —
// the cap trim must land on a rune boundary so the tail of a CJK
// answer does not end mid-codepoint (which would display as an
// invalid glyph). Build a prior draft of 3-byte CJK characters long
// enough to overshoot the cap, then verify the trimmed summary is
// still valid UTF-8.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CJKRuneBoundary(t *testing.T) {
	enableSummaryCapsForTest(t)
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	// 中 is 3 bytes UTF-8. 1000 copies = 3000 bytes, overshoots the 2500 cap.
	prior := strings.Repeat("中", 1000)
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "短"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: prior},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if !utf8.ValidString(got.Summary) {
		t.Errorf("trimmed summary has invalid UTF-8 tail: len=%d last8=%x",
			len(got.Summary), got.Summary[max(0, len(got.Summary)-8):])
	}
	// Trimmed length should be close to 2500, never exceed it.
	if len(got.Summary) > types.SummaryCapFor(types.ShapeExplanation, 0) {
		t.Errorf("trimmed summary exceeds cap: len=%d", len(got.Summary))
	}
	// At 3 bytes/rune, trimming 2500 bytes should preserve at least
	// 833 runes (2500 / 3 = 833.33).
	if got := utf8.RuneCountInString(got.Summary); got < 830 {
		t.Errorf("trimmed summary lost too many runes: got %d runes, expected ~833", got)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ZhCaveat —
// bilingual caveat check: when language=zh, the appended caveat is
// rendered in Chinese.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_ZhCaveat(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "short"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: richDraftProse()},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	foundZh := false
	for _, c := range got.Caveats {
		if strings.Contains(c, "更丰富的前一轮草稿") {
			foundZh = true
		}
	}
	if !foundZh {
		t.Errorf("zh caveat missing: %v", got.Caveats)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_KillSwitch —
// when AgentSettings.FinalizerPreservePriorProse is explicitly false
// (set via codrax.yaml: agent_finalizer_preserve_prior_prose: false),
// the salvage must NOT fire even under conditions that would normally
// trigger it. This pins the config knob end-to-end: the *bool field
// propagates from YAML → AgentSettings → evaluator struct → runtime
// decision. A silent drift at any hop would re-enable the salvage
// against the operator's wishes.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_KillSwitch(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	origSummary := "short compressed paraphrase"
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: origSummary}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: richDraftProse()},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	disabled := false
	e := &answerDocumentEvaluator{
		language:           "en",
		preservePriorProse: &disabled,
	}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Errorf("kill switch violated: Summary was overwritten %q → %q even though preservePriorProse=&false",
			origSummary, got.Summary)
	}
	if len(got.Caveats) != 0 {
		t.Errorf("kill switch violated: caveat appended despite preservePriorProse=&false: %v", got.Caveats)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_NilMeansDefault —
// when preservePriorProse is nil (test default / evaluator constructed
// without explicit settings), the salvage must treat it as enabled.
// The production call-path always non-nil via ResolvedAgentSettings,
// but test constructors and edge cases pass through nil; the
// evaluator must not interpret that as "disabled".
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_NilMeansDefault(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: "short"}
	ctx.Mutable.SetAnswerDocument(doc)
	messages := []llm.Message{
		{Role: "assistant", Content: richDraftProse()},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	// No explicit preservePriorProse, no explicit thresholds.
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if len(got.Summary) < 400 {
		t.Errorf("nil preservePriorProse should behave as enabled; salvage did not fire. Summary: %q", got.Summary)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CustomThresholds —
// custom shrinkageMinProseLen / shrinkageRatio from AgentSettings must
// override the package-level defaults. Build a scenario that fires
// under the default thresholds (min=400, ratio=0.5) but is BLOCKED
// under stricter custom thresholds (min=2000) — that proves the
// evaluator reads its per-instance field instead of the const.
func TestAnswerDocumentEvaluator_ParseOutput_ShrinkageSalvage_CustomThresholds(t *testing.T) {
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	origSummary := "compressed"
	doc := &types.AnswerDocument{Shape: types.ShapeExplanation, Summary: origSummary}
	ctx.Mutable.SetAnswerDocument(doc)
	// richDraftProse is ~1560 bytes; a min floor of 2000 must block salvage.
	messages := []llm.Message{
		{Role: "assistant", Content: richDraftProse()},
		{Role: "assistant", Content: "", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit_answer_document"}}},
	}
	e := &answerDocumentEvaluator{
		language:             "en",
		shrinkageMinProseLen: 2000, // raised above prior-draft length
		shrinkageRatio:       0.5,
	}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	got := parseStageDoc(t, out)
	if got.Summary != origSummary {
		t.Errorf("custom floor of 2000 should block salvage, but Summary was overwritten: %q", got.Summary)
	}
}

// TestFindLastPreToolCallDraft_IgnoresToolCallTurns — the helper
// must SKIP assistant messages that have tool calls, since those
// represent "tool call fired" turns, not pre-tool-call drafts. The
// target is the draft from BEFORE the emit landed.
func TestFindLastPreToolCallDraft_IgnoresToolCallTurns(t *testing.T) {
	messages := []llm.Message{
		{Role: "user", Content: "q"},
		{Role: "assistant", Content: "pre-tool-call draft"}, // no ToolCalls
		{Role: "user", Content: "hint"},
		{Role: "assistant", Content: "tool-call turn preamble", ToolCalls: []llm.ToolCall{{ID: "1", Name: "emit"}}},
	}
	got := findLastPreToolCallDraft(messages)
	if got != "pre-tool-call draft" {
		t.Errorf("findLastPreToolCallDraft = %q, want pre-tool-call draft", got)
	}
}

// TestAnswerDocumentEvaluator_DetermineMissingPiece — always returns
// MissingNone, matching the legacy finalizer contract.
func TestAnswerDocumentEvaluator_DetermineMissingPiece(t *testing.T) {
	e := &answerDocumentEvaluator{}
	if got := e.DetermineMissingPiece(nil, nil); got != types.MissingNone {
		t.Errorf("DetermineMissingPiece = %q, want MissingNone", got)
	}
}

// TestAnswerDocumentSkill_DeclaresEmitTool pins the P2.2 cleanup
// contract: the declarative answer-document-skill in
// internal/skill/defaults.go MUST declare emit_answer_document in
// its ToolSuggestions. This replaces the pre-cleanup approach of
// patching the two legacy finalize skills' ToolSuggestions at runtime
// in cmd/root.go, which would have left their contradictory
// Answer/Evidence markdown OutputFormat in the prompt.
func TestAnswerDocumentSkill_DeclaresEmitTool(t *testing.T) {
	reg := skill.NewRegistry()
	skill.RegisterDefaults(reg)
	sk, err := reg.Get("answer-document-skill")
	if err != nil {
		t.Fatalf("answer-document-skill not registered by RegisterDefaults: %v", err)
	}
	found := false
	for _, name := range sk.ToolSuggestions {
		if name == "emit_answer_document" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("answer-document-skill.ToolSuggestions missing emit_answer_document: %v",
			sk.ToolSuggestions)
	}
	// Sanity checks: the skill must NOT accidentally declare the
	// legacy finalize skills' tools (todo_write etc.) which would
	// reintroduce the prose-writing pathway.
	if len(sk.ToolSuggestions) != 1 {
		t.Errorf("answer-document-skill should only declare emit_answer_document, got %v",
			sk.ToolSuggestions)
	}
}
