package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
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

// TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersBlockContract
// pins that the dynamic prompt surfaces the V2 block contract
// (Required Answer Blocks section with at least the Summary block
// requirement). B8-T1 part 3 deleted the `## Target answer shape`
// section + resolveAnswerDocShape; the V2 block contract carries
// the equivalent guidance via Family-aware block requirements.
//
// The STATIC contract — tool name, required fields, forbidden fields
// — still lives in answer-document-skill.OutputFormat (rendered by
// context/builder.go as a system section), NOT here. Asserting those
// substrings in the dynamic prompt would resurrect the pre-cleanup
// contradiction.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersBlockContract(t *testing.T) {
	// V2 carrier renders the block contract from QuestionFamily.
	// Iterating Intent/Scenario combinations exercises the same
	// surface the pre-PR5 shape-driven test covered: the contract
	// section must surface the "Required Answer Blocks" header
	// plus a summary block requirement.
	intents := []types.Intent{
		types.IntentExplain,
		types.IntentRootCause,
		types.IntentTrace,
		types.IntentEnumerate,
		types.IntentConfigQuery,
		types.IntentReturnValue,
	}
	for _, intent := range intents {
		t.Run(string(intent), func(t *testing.T) {
			ctx := &types.AgentContext{
				AnalysisIR: &types.AnalysisIR{
					RequestModel:   types.RequestModel{Intent: intent},
					AnswerContract: types.AnswerContract{},
				},
			}
			prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
			if !strings.Contains(prompt, "## Required Answer Blocks") {
				t.Errorf("intent=%s: prompt missing V2 block contract header: %q", intent, prompt)
			}
			if !strings.Contains(prompt, "summary") {
				t.Errorf("intent=%s: prompt missing summary block requirement: %q", intent, prompt)
			}
			// Guard against drift back to the pre-cleanup pattern:
			// the static contract MUST NOT resurface here.
			for _, banned := range []string{"emit_answer_document", "Prohibitions", "Citation pool"} {
				if strings.Contains(prompt, banned) {
					t.Errorf("intent=%s: dynamic prompt leaked static contract substring %q — "+
						"that content belongs in answer-document-skill, not the evaluator", intent, banned)
				}
			}
			// B8-T1 part 3: the legacy "## Target answer shape"
			// section is deleted; pin its absence so it can't drift back.
			if strings.Contains(prompt, "## Target answer shape") {
				t.Errorf("intent=%s: pre-B8 ## Target answer shape section resurfaced: %q", intent, prompt)
			}
		})
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedCandidateRoles(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
				AnswerRoleProfile: &types.AnswerRoleProfile{
					IsRoleBindingRequested: true,
					RequiredCandidateRoles: []types.AnswerCandidateRole{
						types.AnswerCandidateRoleBudgetCap,
					},
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Answer Role Contract") {
		t.Fatalf("prompt missing typed answer-role contract:\n%s", prompt)
	}
	for _, want := range []string{"budget_cap", "items[].candidate_role", "prose-only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in typed answer-role contract:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequiredMechanismAnchors(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				AnalyzerHints: types.AnalyzerHints{
					MentionedEntities: []string{"runTaskGraph"},
				},
			},
			AnswerContract: types.AnswerContract{
				MustIncludeTerms: []types.ContractTerm{{
					Text: "runTaskGraph",
					Kind: types.ContractTermSymbol,
				}},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Mechanism Anchor Contract") {
		t.Fatalf("prompt missing typed mechanism-anchor contract:\n%s", prompt)
	}
	for _, want := range []string{"runTaskGraph", "blocks[].items[].label", "edge_anchors", "prose-only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in typed mechanism-anchor contract:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersErrorGranularityContract(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
				ErrorGranularityProfile: &types.ErrorGranularityProfile{
					IsGranularityQuestion: true,
					RequestedVerdictOptions: []types.ErrorGranularityVerdict{
						types.ErrorGranularityPerItemRejection,
						types.ErrorGranularityWholeBatch,
					},
					Confidence: 0.9,
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Error Granularity Contract") {
		t.Fatalf("prompt missing typed error-granularity contract:\n%s", prompt)
	}
	for _, want := range []string{"error_granularity_verdict", "per_item_rejection", "whole_batch_failure", "not_enough_evidence", "requested verdict options", "prose-only"} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q in typed error-granularity contract:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, `"partial_success", "fail_fast"`) {
		t.Fatalf("prompt should narrow allowed verdicts to requested options plus fallback:\n%s", prompt)
	}
}

// TestRenderAnswerDocFacetCoverage_NilOrEmptyReturnsEmpty pins
// byte-identical behaviour for shapes whose AnswerSurfacePlan
// produces no FacetCoverageContract — Phase 2 must not change pre-P1
// prompt output for those cases.
func TestRenderAnswerDocFacetCoverage_NilOrEmptyReturnsEmpty(t *testing.T) {
	if got := renderAnswerDocFacetCoverage(nil); got != "" {
		t.Errorf("nil ctx should return empty; got %q", got)
	}
	// ctx with no AnalysisIR — answerSurfacePlan returns nil.
	emptyCtx := &types.AgentContext{}
	if got := renderAnswerDocFacetCoverage(emptyCtx); got != "" {
		t.Errorf("ctx without AnalysisIR should return empty; got %q", got)
	}
}

// TestRenderAnswerDocFacetCoverage_EmitsHardSoftOptionalLabels pins
// the rendered surface format. The ConfigPrecedence family fires
// HARD on FacetConfigPrecedenceRole + FacetResolvedLiteralOrSymbol,
// SOFT on FacetUncertaintyBoundary, OPTIONAL on FacetDiagramSpine.
// Phase 1 fallback degrades HARD-without-candidates to SOFT, so we
// also cover that path by NOT supplying surface evidence.
func TestRenderAnswerDocFacetCoverage_EmitsHardSoftOptionalLabels(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentConfigQuery,
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	for _, want := range []string{
		"## Required Answer Facets",
		"**SOFT**", // Phase 1 degrades HARD without evidence
		"Config precedence role",
		"Uncertainty boundary",
		"Optional richness facets:",
		// v3 B5 (2026-05-04): "Diagram spine" / "structural backbone"
		// language replaced with neutral "Diagram facet" — lint
		// catches the old wording in InternalTermsBlocklist.
		"Diagram facet",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered prompt missing %q\n----\n%s", want, got)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_EvidenceCountAnnotation pins
// Phase 5-E2: every facet line ends with `(evidence: N)` where N is
// len(SourceCandidate). The annotation surfaces the gate input so
// the LLM understands when SOFT facets are skipped (N=0).
func TestRenderAnswerDocFacetCoverage_EvidenceCountAnnotation(t *testing.T) {
	// IntentRootCause + LogBundle drives QFRootCauseTrace, which has
	// FacetObservedArtifactFact as HARD. Provide one matching surface
	// item so SourceCandidate is non-empty (count = 1).
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: &types.LogBundle{},
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// Every facet line MUST carry the (evidence: N) marker.
	if !strings.Contains(got, "(evidence:") {
		t.Errorf("missing (evidence: N) annotation; got\n%s", got)
	}
	// Header prose MUST explain the gate semantic.
	if !strings.Contains(got, "(evidence: N)") ||
		!strings.Contains(got, "SOFT facets this count gates") {
		t.Errorf("header prose missing E1 gate explanation; got\n%s", got)
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_RendersStructuredRowsAndFlow(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentExplain},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "cfg-default",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/config/runtime.go",
			LineStart:       42,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    "DefaultLimit",
			Subject:         "DefaultLimit",
			Object:          "16",
			Snippet:         "DefaultLimit: 16,",
			Summary:         "raw thinking note that must not be surfaced",
			SurfaceTerms:    []string{"DefaultLimit"},
			GroundingStatus: types.GroundingGrounded,
		}},
		FlowFindings: []types.FlowFindingDigest{{
			ID:      "flow-1",
			Path:    []string{"loadConfig", "normalize", "compile"},
			Sources: []string{"codrax.yaml"},
			Sinks:   []string{"RuntimeConfig"},
		}},
	}
	got := renderAnswerDocTypedExplorationEnrichment(ctx, false)
	for _, want := range []string{
		"## Typed Exploration Enrichment Facts",
		"model-emitted `EvidenceItems`, deterministic evidence projections, and `FlowFindings`",
		"lane=value_fact",
		"DefaultLimit: 16,",
		"### Flow/source-sink rows",
		"path=loadConfig -> normalize -> compile",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed enrichment prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "raw thinking note") {
		t.Fatalf("typed enrichment must not replay non-load-bearing free-form summary:\n%s", got)
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_ContextDoesNotBecomePrincipal(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:              "nearby-helper",
			Kind:            types.EvidenceRelationship,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/helper.go",
			LineStart:       77,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "helper",
			Subject:         "NearbyHelper",
			Object:          "Target",
			Snippet:         "NearbyHelper()",
			ContextRole:     types.EvidenceContextRoleRelatedContext,
			GroundingStatus: types.GroundingGrounded,
		}},
	}
	got := renderAnswerDocTypedExplorationEnrichment(ctx, true)
	for _, want := range []string{
		"Because typed support lanes are present, those lanes remain the principal boundary",
		"They are not a member slate by themselves",
		"Do not promote an enrichment fact into a new principal ordered-list member",
		"lane=context_enrichment_fact",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed enrichment boundary prompt missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "### Principal Member Obligations") {
		t.Fatalf("enrichment rows must not create principal member obligations:\n%s", got)
	}
}

func TestAnswerDocTypedEnrichmentEvidencePoolBoundsLargeContext(t *testing.T) {
	evidence := make([]types.EvidenceItem, answerDocMaxEnrichmentCandidateFacts+50)
	for i := range evidence {
		evidence[i] = types.EvidenceItem{
			ID:         fmt.Sprintf("ev-%04d", i),
			Kind:       types.EvidenceDirect,
			Source:     "internal/example.go",
			LineStart:  i + 1,
			AnchorKind: types.AnchorAssignment,
			Subject:    fmt.Sprintf("Subject%d", i),
			Snippet:    fmt.Sprintf("Subject%d := value", i),
		}
	}
	ctx := &types.AgentContext{EvidenceItems: evidence}

	got := answerDocTypedEnrichmentEvidencePool(ctx, answerDocMaxEnrichmentCandidateFacts)
	if len(got) != answerDocMaxEnrichmentCandidateFacts {
		t.Fatalf("bounded enrichment pool len=%d, want %d", len(got), answerDocMaxEnrichmentCandidateFacts)
	}
	if got[len(got)-1].ID != fmt.Sprintf("ev-%04d", answerDocMaxEnrichmentCandidateFacts-1) {
		t.Fatalf("bounded enrichment pool should preserve ranked prefix, last=%q", got[len(got)-1].ID)
	}
}

func TestSelectAnswerDocTypedEnrichmentFactsTruncatesLargeSurface(t *testing.T) {
	ctx := &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{Intent: types.IntentExplain}}}
	longSnippet := strings.Repeat("x", answerDocMaxEnrichmentSurfaceBytes+200)
	got := selectAnswerDocTypedEnrichmentFacts(ctx, []types.EvidenceItem{{
		ID:              "large-surface",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/example.go",
		LineStart:       12,
		AnchorKind:      types.AnchorAssignment,
		Subject:         "LargeSurface",
		Snippet:         longSnippet,
		GroundingStatus: types.GroundingGrounded,
	}}, false, nil)
	if len(got) != 1 {
		t.Fatalf("expected one enrichment fact, got %+v", got)
	}
	if len(got[0].surface) > answerDocMaxEnrichmentSurfaceBytes+len("…[truncated]") {
		t.Fatalf("surface was not bounded: len=%d", len(got[0].surface))
	}
	if !strings.Contains(got[0].surface, "…[truncated]") {
		t.Fatalf("surface should carry truncation marker, got %q", got[0].surface)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_DiagnosticSupportScopeFiltersUnrelatedRows(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:    "support-build",
				Source:        "internal/agent/analyzer.go",
				LineStart:     1271,
				AnchorKind:    types.AnchorCall,
				AnchorSymbol:  "buildAnalysisIR",
				Subject:       "buildAnalysisIR",
				SurfaceTerms:  []string{"analysis IR builder"},
				GroundingTier: types.TierLineText,
			}},
		}},
	}, true, extractorValueRankDiagnostic)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentRootCause,
				Scenario: types.ScenarioRootCause,
			},
		},
	}
	evidence := []types.EvidenceItem{
		{
			ID:              "relevant-build",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/analyzer.go",
			LineStart:       1280,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "buildAnalysisIR",
			Subject:         "buildAnalysisIR",
			Object:          "request model",
			Snippet:         "raw := ctx.Mutable.RequestModel()",
			GroundingStatus: types.GroundingGrounded,
		},
		{
			ID:              "unrelated-multigraph",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/tool/repomap/multigraph/build.go",
			LineStart:       42,
			AnchorKind:      types.AnchorCall,
			AnchorSymbol:    "New",
			Subject:         "MultiGraph.New",
			Object:          "graph construction",
			Snippet:         "return MultiGraph.New(nodes)",
			GroundingStatus: types.GroundingGrounded,
		},
	}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, true, supportScope)
	if len(got) != 1 {
		t.Fatalf("diagnostic support scope should keep only linked rows, got %d: %+v", len(got), got)
	}
	if got[0].item.ID != "relevant-build" {
		t.Fatalf("diagnostic support scope kept wrong row: %+v", got[0].item)
	}
}

func TestSelectAnswerDocTypedEnrichmentFacts_NonDiagnosticScopeKeepsContextRows(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-build",
				Source:       "internal/agent/analyzer.go",
				LineStart:    1271,
				AnchorSymbol: "buildAnalysisIR",
				Subject:      "buildAnalysisIR",
			}},
		}},
	}, true, extractorValueRankComparison)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
		},
	}
	evidence := []types.EvidenceItem{{
		ID:              "architecture-context",
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Source:          "internal/tool/repomap/multigraph/build.go",
		LineStart:       42,
		AnchorKind:      types.AnchorCall,
		AnchorSymbol:    "New",
		Subject:         "MultiGraph.New",
		Object:          "graph construction",
		Snippet:         "return MultiGraph.New(nodes)",
		GroundingStatus: types.GroundingGrounded,
	}}

	got := selectAnswerDocTypedEnrichmentFacts(ctx, evidence, true, supportScope)
	if len(got) != 1 || got[0].item.ID != "architecture-context" {
		t.Fatalf("non-diagnostic prompt enrichment should keep context rows, got %+v", got)
	}
}

func TestSelectAnswerDocFlowEnrichmentLines_DiagnosticSupportScopeFiltersUnrelatedFlow(t *testing.T) {
	supportScope := supportLaneScopeFromPlan(&types.AnswerSupportPlan{
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLaneCurrentCodePath,
			Entries: []types.AnswerSupportEntry{{
				EvidenceID:   "support-build",
				Source:       "internal/agent/analyzer.go",
				LineStart:    1271,
				AnchorSymbol: "buildAnalysisIR",
				Subject:      "buildAnalysisIR",
			}},
		}},
	}, true, extractorValueRankDiagnostic)
	findings := []types.FlowFindingDigest{
		{
			ID:          "flow-relevant",
			Path:        []string{"parseAnalyzerPayload", "buildAnalysisIR"},
			EvidenceIDs: []string{"support-build"},
		},
		{
			ID:   "flow-unrelated",
			Path: []string{"MultiGraph.New", "Builder"},
		},
	}

	got := selectAnswerDocFlowEnrichmentLines(findings, 10, supportScope)
	if len(got) != 1 {
		t.Fatalf("diagnostic support scope should keep only linked flow rows, got %d: %+v", len(got), got)
	}
	if !strings.Contains(got[0], "flow-relevant") {
		t.Fatalf("diagnostic support scope kept wrong flow row: %+v", got)
	}
}

func TestRenderAnswerDocTypedExplorationEnrichment_CoversQuestionFamilies(t *testing.T) {
	cases := []struct {
		name string
		rm   types.RequestModel
		item types.EvidenceItem
		want string
	}{
		{
			name: "scalar",
			rm: types.RequestModel{
				Intent:        types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectNumeric},
			},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorReturn, AnchorSymbol: "limit", Subject: "limit", Snippet: "return 16"},
			want: "lane=value_fact",
		},
		{
			name: "config",
			rm: types.RequestModel{
				Intent:        types.IntentConfigQuery,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorInitializer, AnchorSymbol: "llm_stream", Subject: "llm_stream", Snippet: "llm_stream: true"},
			want: "lane=value_fact",
		},
		{
			name: "trace",
			rm:   types.RequestModel{Intent: types.IntentTrace, PredicateAxis: types.AxisCall},
			item: types.EvidenceItem{Kind: types.EvidenceRelationship, AnchorKind: types.AnchorCall, Subject: "Analyze", Object: "Compile", Snippet: "Compile(ir)"},
			want: "lane=chain_or_intermediate_fact",
		},
		{
			name: "diagnostic",
			rm:   types.RequestModel{Intent: types.IntentRootCause, Scenario: types.ScenarioRootCause},
			item: types.EvidenceItem{Kind: types.EvidenceConflict, AnchorKind: types.AnchorCondition, Subject: "guard", Condition: "ctx == nil", Snippet: "if ctx == nil { return }"},
			want: "lane=boundary_or_exclusion_fact",
		},
		{
			name: "comparison",
			rm:   types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioArchitectureExplain, SubTopics: []types.SubTopic{{Summary: "A"}, {Summary: "B"}}},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorImport, Subject: "agent", Object: "internal/types", Snippet: "import \"github.com/hanchaoqun/codrax/internal/types\""},
			want: "lane=chain_or_intermediate_fact",
		},
		{
			name: "enumeration",
			rm:   types.RequestModel{Intent: types.IntentEnumerate, Predicates: types.SemanticPredicates{IsCategoryEnumeration: true}},
			item: types.EvidenceItem{Kind: types.EvidenceDirect, AnchorKind: types.AnchorDefinition, Subject: "ExplorerAgent", Object: "agent implementation", SurfaceTerms: []string{"ExplorerAgent"}},
			want: "lane=context_enrichment_fact",
		},
		{
			name: "generic",
			rm:   types.RequestModel{Intent: types.IntentExplain, Scenario: types.ScenarioGeneric},
			item: types.EvidenceItem{Kind: types.EvidenceAbsent, Scope: types.ScopeNegative, Source: "internal/agent/agent.go", NegativeQuery: &types.NegativeQuery{File: "internal/agent/agent.go", Pattern: "typed handoff"}, NegativeScope: types.NegativeScopeFile, Subject: "typed handoff"},
			want: "lane=boundary_or_exclusion_fact",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			item := tc.item
			item.ID = "ev-" + tc.name
			if item.Scope == "" {
				item.Scope = types.ScopeLine
			}
			if item.Source == "" {
				item.Source = "internal/agent/example.go"
			}
			if item.LineStart == 0 && item.Scope == types.ScopeLine {
				item.LineStart = 10
			}
			if item.GroundingStatus == "" {
				item.GroundingStatus = types.GroundingGrounded
			}
			ctx := &types.AgentContext{
				AnalysisIR:    &types.AnalysisIR{RequestModel: tc.rm, AnswerContract: types.AnswerContract{}},
				EvidenceItems: []types.EvidenceItem{item},
			}
			got := renderAnswerDocTypedExplorationEnrichment(ctx, false)
			if !strings.Contains(got, tc.want) {
				t.Fatalf("%s enrichment missing %q:\n%s", tc.name, tc.want, got)
			}
		})
	}
}

func TestRenderAnswerDocFacetCoverage_UsesCuratedPrincipalEvidenceCount(t *testing.T) {
	modelItem := types.EvidenceItem{
		ID:              "literal-model",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/analyzer.go",
		LineStart:       1903,
		AnchorKind:      types.AnchorAssignment,
		Subject:         "CitationReq.Required",
		Object:          "false",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
	}
	evidence := []types.EvidenceItem{modelItem}
	for i := 0; i < 20; i++ {
		evidence = append(evidence, types.EvidenceItem{
			ID:              fmt.Sprintf("noise-%02d", i),
			Kind:            types.EvidenceConcrete,
			Scope:           types.ScopeLine,
			Source:          "internal/tool/noise.go",
			LineStart:       i + 1,
			AnchorKind:      types.AnchorAssignment,
			AnchorSymbol:    fmt.Sprintf("helper%d", i),
			Producer:        "concrete_values",
			GroundingStatus: types.GroundingGrounded,
		})
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsScalarAnswer: true,
				},
			},
		},
		EvidenceItems: evidence,
	}

	got := renderAnswerDocFacetCoverage(ctx)
	if !strings.Contains(got, `facet_id: "resolved_literal_or_symbol"`) ||
		!strings.Contains(got, "Resolved literal or symbol") ||
		!strings.Contains(got, "(evidence: 1)") {
		t.Fatalf("facet prompt should render curated principal evidence count, got:\n%s", got)
	}
	if strings.Contains(got, "(evidence: 21)") {
		t.Fatalf("facet prompt should not expose broad raw candidate count as answer-grade evidence:\n%s", got)
	}
}

// G6-3 (post_v2_runtime_gap_remediation, 2026-05-04): SOFT facets
// with typed evidence available (IsPromoted=true) carry an
// `(elevated)` marker after the evidence count, signalling to the
// LLM that the SOFT gate is currently behaving as HARD.
func TestRenderAnswerDocFacetCoverage_ElevatedMarkerOnPromotedSoft(t *testing.T) {
	// IntentTrace → QFCallChain template has FacetCurrentCodePath
	// as SOFT requirement, AcceptableForms=[ClaimDefinitionFact].
	// FacetBranchGuard is SOFT with AcceptableForms=[ClaimGuardCondition]
	// — leave it WITHOUT matching evidence so it stays unpromoted.
	// AnchorDefinition projects to ClaimDefinitionFact, which matches
	// FacetCurrentCodePath's AcceptableForms.
	defEvidence := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "DefSym",
		Source:     "x.go",
		LineStart:  10,
		LineEnd:    10,
		AnchorKind: types.AnchorDefinition,
	}
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
		EvidenceItems: []types.EvidenceItem{defEvidence},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// Header prose MUST explain elevation.
	if !strings.Contains(got, "elevated") {
		t.Errorf("header prose missing elevated explanation; got\n%s", got)
	}
	// At least one promoted SOFT line should carry `(elevated)` — the
	// header occurrence is wrapped in backticks; the bare marker on
	// a facet line is the typed signal.
	lines := strings.Split(got, "\n")
	bareMarkerCount := 0
	for _, line := range lines {
		// Skip header lines (they describe the marker via backticks)
		if !strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if strings.Contains(line, " (elevated)") {
			bareMarkerCount++
		}
	}
	if bareMarkerCount < 1 {
		t.Errorf("expected ≥1 facet line with bare `(elevated)`; got %d\n%s", bareMarkerCount, got)
	}
}

// G6-3: facet line WITHOUT typed evidence (SOFT, SourceCandidate=0)
// MUST NOT carry the (elevated) bare marker — IsPromoted=false
// keeps the gate skipped per Phase 5-E1 evidence-sufficient
// invariant.
func TestRenderAnswerDocFacetCoverage_NoElevatedWhenNoEvidence(t *testing.T) {
	// IntentTrace → QFCallChain. Provide NO evidence at all so every
	// SOFT facet has SourceCandidate=0; expect no bare (elevated).
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	lines := strings.Split(got, "\n")
	for _, line := range lines {
		// Header lines (no `-` prefix) include the marker in backticks
		// — fine. Facet lines start with `-` and must NOT carry the
		// bare marker when evidence is absent.
		if !strings.HasPrefix(strings.TrimSpace(line), "-") {
			continue
		}
		if strings.Contains(line, " (elevated)") {
			t.Errorf("no-evidence SOFT facet must NOT carry bare (elevated); got line %q", line)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_NoGoInternalNamesLeak pins the
// glossary-lint contract at the function-output level. None of the
// Phase 1 internal type names may appear in the rendered LLM-facing
// prompt, no matter what facets fire.
func TestRenderAnswerDocFacetCoverage_NoGoInternalNamesLeak(t *testing.T) {
	// Drive every family to maximise facet variety in the rendered
	// output. Use four distinct ctxs and concatenate.
	cases := []*types.AgentContext{
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentRootCause, LogTriage: &types.LogBundle{}}}},
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentConfigQuery}}},
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate}}},
		{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentExplain,
			AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName, Confidence: 0.8}}}},
	}
	var combined strings.Builder
	for _, ctx := range cases {
		combined.WriteString(renderAnswerDocFacetCoverage(ctx))
	}
	got := combined.String()
	for _, banned := range []string{
		"FacetCoverageContract", "FacetCoverage", "FacetRequirement",
		"AnswerFacetKind", "QuestionFamily", "ClaimFormOf",
		"FacetRequiredness", "AcceptableForms", "SourceCandidate",
		"ClaimDefinitionFact", "ClaimCallEdge", "ClaimExternalObservation",
		"QFRootCauseTrace", "QFConfigPrecedence",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("rendered prompt leaked Go-internal token %q\n----\n%s", banned, got)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_OptionalSectionOmittedWhenEmpty
// pins that the "Optional richness facets" sub-heading only renders
// when there's at least one optional facet — keeps prompt lean.
// QFCallChain template (Intent=Trace, no obligation, no log) has
// HARD/SOFT-only entries with zero FacetOptional rows.
func TestRenderAnswerDocFacetCoverage_OptionalSectionOmittedWhenEmpty(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	if got == "" {
		t.Fatal("QFCallChain should produce a non-empty contract")
	}
	if strings.Contains(got, "Optional richness facets:") {
		t.Errorf("QFCallChain template has no optional facets; section must be omitted; got:\n%s", got)
	}
	if !strings.Contains(got, "Principal path edge") {
		t.Errorf("QFCallChain principal-path-edge facet missing; got:\n%s", got)
	}
}

// TestRenderAnswerDocFacetCoverage_BuildInitialInstructionWiring
// pins that BuildInitialInstruction picks up the section in its
// per-dispatch output (covers the orchestration plumb between
// answerSurfacePlan and the prompt builder).
func TestRenderAnswerDocFacetCoverage_BuildInitialInstructionWiring(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 3, SourceQuote: "3 X",
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Required Answer Facets") {
		t.Errorf("BuildInitialInstruction did not surface facet section; got:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Enumeration item") {
		t.Errorf("enumeration_item facet expected for IntentEnumerate + EnumerationBoundary; got:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SingleTopicExplanationLeavesSymbolsEmpty(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentExplain},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Do NOT add an enumeration anchor skeleton block unless the prompt explicitly attached an Anchor skeleton section") {
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
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectConfigKey,
					AllowAbsence: true,
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// Pre-PR5 the prompt embedded the literal "explanation" shape
	// label; with shape retired, the absence-narrative path is
	// indicated by the V2 BlockSummary contract — assert that the
	// contract section is rendered (not by tag) for an absent
	// exact-config-key dispatch.
	if !strings.Contains(prompt, "## Required Answer Blocks") {
		t.Fatalf("absent exact config-key dispatch should still render block contract: %q", prompt)
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline
// checks that when MustInclude is populated and the answer needs an
// enumeration slate, the dynamic prompt renders the grounded
// required-member floor so the finalizer preserves those members in
// V2 blocks instead of inventing a retired completeness field.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_SurfacesCardinalityBaseline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"Alpha", "Beta"},
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
	if !strings.Contains(prompt, "Required-term floor: **2 term(s)**") {
		t.Errorf("required-term floor not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "Alpha (symbol)") || !strings.Contains(prompt, "Beta (symbol)") {
		t.Errorf("required-term labels not surfaced: %q", prompt)
	}
	if !strings.Contains(prompt, "must preserve all 2 grounded term(s)") {
		t.Errorf("required-member preservation guidance not surfaced: %q", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_TypedMustIncludeTerms(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"providers.yaml"},
				MustIncludeTerms: []types.ContractTerm{
					{Text: "emit_evidence", Kind: types.ContractTermToolName},
					{Text: "providers.yaml", Kind: types.ContractTermFileStem},
					{Text: "raw prompt", Kind: types.ContractTermUserPhrase},
				},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"emit_evidence (tool name)",
		"providers.yaml (file stem)",
		"raw prompt (phrase)",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("typed must-include term %q missing from prompt:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "Required-symbol floor") {
		t.Fatalf("typed must-include prompt should not call every term a symbol:\n%s", prompt)
	}
	if strings.Contains(prompt, "providers.yaml (symbol)") {
		t.Fatalf("typed file-stem term should override duplicate legacy symbol lane:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_MustIncludeTermsAreNotPrincipalFloorWithoutSlate(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{
				MustInclude: []string{"SubAgentRuntime", "ProposeSubAgents"},
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Required answer-term coverage:") {
		t.Fatalf("must_include terms should still be surfaced as answer coverage:\n%s", prompt)
	}
	if strings.Contains(prompt, "must preserve all 2 grounded term(s)") {
		t.Fatalf("must_include terms without answer-symbol slate must not become principal item floor:\n%s", prompt)
	}
	if !strings.Contains(prompt, "do not turn a helper, tool, file stem, attribute, or mechanism component into a principal list member") {
		t.Fatalf("prompt should guard against context helper promotion:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCallChainSupportLanesFromStepBackbone(t *testing.T) {
	mut := types.NewMutableState("")
	syms := []types.AnswerSymbol{
		{Name: "RequestModel", File: "internal/agent/analyzer.go", Line: 616, Rationale: "在 buildAnalysisIR 内部获取 LLM 输出的 RequestModel，是后续步骤的输入基础"},
		{Name: "gate.Run", File: "internal/agent/analyzer.go", Line: 1062, Rationale: "执行质量门检查，生成最终 gate 结果"},
	}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessLowerBound)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{},
		},
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"### Current grounded call chain",
		"Allowed block kinds: summary, ordered_list, diagram",
		"`RequestModel`",
		"internal/agent/analyzer.go:616",
		"`gate.Run`",
		"internal/agent/analyzer.go:1062",
		"runtime frames as additional principal hops",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("call-chain support prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Resolved Step Sequence") {
		t.Fatalf("call-chain support lanes should replace legacy step sequence prompt:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCallChainSupportLanesFromEvidence(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceDirect,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       127,
				LineEnd:         130,
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
		"## Typed Answer Support Lanes",
		"### Current grounded call chain",
		"`checkCoverage`",
		"internal/analysis/gate/gate.go:127",
		"equivalent typed anchors: `internal/analysis/gate/gate.go:130`",
		"`checkDAGClosure`",
		"internal/analysis/gate/gate.go:128",
		"`checkBudgetSanity`",
		"internal/analysis/gate/gate.go:129",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("fallback call-chain support prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "## Resolved Step Sequence") {
		t.Fatalf("call-chain support lanes should replace legacy fallback step sequence prompt:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_PrincipalSupportLaneBacktickBoundary(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{
					Kind:       types.SubjectNumeric,
					Confidence: 0.95,
				},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "required-false-1",
				Kind:            types.EvidenceDirect,
				Scope:           types.ScopeLine,
				Source:          "internal/agent/analyzer.go",
				LineStart:       1903,
				AnchorKind:      types.AnchorAssignment,
				AnchorSymbol:    "CitationReq.Required",
				Subject:         "AnswerContract.CitationReq.Required",
				Object:          "false",
				Snippet:         "out.AnswerContract.CitationReq.Required = false",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"inline backticks around code / file / config surfaces only when that exact surface is visible in a lane entry",
		"Names that appear only in `Evidence note`, retry diagnostics, raw tool output, search hints, or nearby context are background",
		"If the user's scalar / count question also asks for concrete members, files, paths, or line numbers",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal support prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrincipalMemberObligations(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
			},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:              "enum-intent",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/types/analysis_ir.go",
			LineStart:       642,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "Intent",
			Subject:         "Intent",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"### Principal Member Obligations",
		"Intent",
		"internal/types/analysis_ir.go:642",
		"evidence_id=enum-intent",
		"fresh `citations[]` pool",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal member obligation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_FileImpactObligationsUseFileCount(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					RequestedOutput: types.ImpactOutputFiles,
					Target:          "CitationReq.Required",
				},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
			},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				ID:              "builder-required",
				Kind:            types.EvidenceConditional,
				Scope:           types.ScopeLine,
				Source:          "internal/context/builder.go",
				LineStart:       1729,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "Required",
				Subject:         "CitationReq.Required",
				Condition:       "if c.CitationReq.Required",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
			{
				ID:              "gate-required",
				Kind:            types.EvidenceConditional,
				Scope:           types.ScopeLine,
				Source:          "internal/analysis/gate/gate.go",
				LineStart:       301,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "Required",
				Subject:         "CitationReq.Required",
				Condition:       "if c.CitationReq.Required && c.CitationReq.MinCitations < 0",
				Producer:        "explorer.emit_evidence",
				GroundingStatus: types.GroundingGrounded,
				GroundingTier:   types.TierLineText,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"This is a change-impact file enumeration",
		"the 2 member(s) below are the file set",
		"Use each file path as the item label",
		"Do not copy numeric file counts from unstructured closure prose",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("file-impact obligation prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersEquivalentPrincipalAnchors(t *testing.T) {
	mut := types.NewMutableState("all implementers")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "LoopController implementers",
		Value:   "1",
		Members: []string{"analyzerEvaluator (internal/agent/analyzer.go:887)"},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"analyzerEvaluator"},
				},
				CompletenessObligation: &types.CompletenessObligation{Required: true, SourceQuote: "all implementers"},
			},
			AnswerContract: types.AnswerContract{
				CitationReq: types.CitationReq{Required: true, Granularity: "file_line", MinCitations: 1},
			},
		},
		Mutable: mut,
		EvidenceItems: []types.EvidenceItem{{
			ID:              "analyzer-definition",
			Kind:            types.EvidenceDirect,
			Scope:           types.ScopeLine,
			Source:          "internal/agent/analyzer.go",
			LineStart:       46,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "analyzerEvaluator",
			Subject:         "analyzerEvaluator",
			Producer:        "explorer.emit_evidence",
			GroundingStatus: types.GroundingGrounded,
			GroundingTier:   types.TierLineText,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"equivalent typed anchors",
		"internal/agent/analyzer.go:887",
		"internal/agent/analyzer.go:46",
		"one of internal/agent/analyzer.go:887, internal/agent/analyzer.go:46",
		"Do not churn `citation_ref`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal equivalent-anchor prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersStructuredAggregateFacts(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateTotalCount,
			Label: "production assignment locations",
			Value: "4",
			Unit:  "locations",
			Members: []string{
				"internal/agent/analyzer.go:1903",
				"internal/orchestrator/contract_check.go:63",
			},
		},
		{
			Kind:  types.AnswerAggregateUniqueCount,
			Label: "unique files",
			Value: "3",
			Unit:  "files",
			Members: []string{
				"internal/agent/analyzer.go",
				"internal/orchestrator/contract_check.go",
				"internal/orchestrator/orchestrator.go",
			},
		},
	})
	mut.SetInvestigationComplete("deterministic count and classification complete")
	mut.SetInvestigationResultKind("resolved")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{
					Kind:       types.SubjectNumeric,
					Confidence: 0.95,
				},
				Predicates: types.SemanticPredicates{
					IsScalarAnswer:  true,
					IsCountQuestion: true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		Mutable: mut,
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Structured Aggregate Facts",
		"emit_investigation_complete.aggregate_facts",
		"kind=`total_count`, label=production assignment locations, value=`4`",
		"kind=`unique_count`, label=unique files, value=`3`",
		"internal/orchestrator/orchestrator.go",
		"create or reuse a matching `citations[]` entry",
		"Do not recompute new aggregate values in finalization",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("aggregate prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrincipalBoundaryForPriorContext(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{
					Kind:       types.SubjectConfigKey,
					Confidence: 0.9,
				},
				ConversationReferenceProfile: &types.ConversationReferenceProfile{
					RequiresPriorContext:  true,
					NeedsRepoVerification: true,
					Ambiguity:             types.ConversationReferenceAmbiguityNone,
					ResolvedSubjects: []types.ResolvedConversationSubject{{
						Surface:          "explore_mid_loop_hint_budget",
						Kind:             types.SubjectConfigKey,
						Source:           types.ConversationReferenceSourcePriorContext,
						Role:             "asked_default_value",
						UseAsExactTarget: true,
					}},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Principal Answer Boundary",
		"Resolved prior-context focus:",
		"not an authorization to add neighboring subjects as answer members",
		"surface `explore_mid_loop_hint_budget`",
		"kind `config_key`",
		"eligible exact target",
		"For role lookups, answer with the single role-bearing literal",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("principal boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_PrincipalBoundaryBindsDiagramsToUserIntent(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "entry/src/main/ets/pages/Index.ets",
				LineStart:       42,
				AnchorKind:      types.AnchorCall,
				Subject:         "Index.ets.render",
				Object:          "NativeBridge.invokeOhSum",
				AnchorSymbol:    "NativeBridge.invokeOhSum",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceRelationship,
				Source:          "src/native/sum.cpp",
				LineStart:       18,
				AnchorKind:      types.AnchorCall,
				Subject:         "NativeBridge.invokeOhSum",
				Object:          "demo.bridge.ohSum",
				AnchorSymbol:    "demo.bridge.ohSum",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceRelationship,
				Source:          "src/bridge/Bridge.cj",
				LineStart:       27,
				AnchorKind:      types.AnchorCall,
				Subject:         "demo.bridge.ohSum",
				Object:          "Bridge.sum",
				AnchorSymbol:    "Bridge.sum",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Principal Answer Boundary",
		"Any diagram must obey the same principal boundary as the prose blocks",
		"For call-chain answers, the principal ordered list and any sequence diagram contain only the requested chain hops",
		"### Current grounded call chain",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("diagram boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SuppressesStepBackboneWhenTypedSupportPlanExists(t *testing.T) {
	mut := types.NewMutableState("")
	syms := []types.AnswerSymbol{
		{Name: "ParseOutput", File: "internal/agent/analyzer.go", Line: 698, Rationale: "stack trace shows all args are nil"},
		{Name: "buildAnalysisIR", File: "internal/agent/analyzer.go", Line: 743, Rationale: "panic must come from deeper nil path"},
	}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessLowerBound)
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors:        []types.LogError{{Type: "panic"}},
					ResolvedFiles: []string{"internal/agent/analyzer.go"},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		LogTriage:                &types.LogBundle{Errors: []types.LogError{{Type: "panic", Frames: []types.LogFrame{{Raw: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)\n\tinternal/agent/analyzer.go:250 +0x1e", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"}}}}, ResolvedFiles: []string{"internal/agent/analyzer.go"}},
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessLowerBound,
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "## Typed Answer Support Lanes") {
		t.Fatalf("typed support lanes missing:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Allowed block kinds: summary, caveat") {
		t.Fatalf("typed support lanes should expose block-kind boundaries:\n%s", prompt)
	}
	if !strings.Contains(prompt, "If a lane does not list `ordered_list`, do not turn its entries into principal hop items") {
		t.Fatalf("typed support instructions should forbid boundary/observation lanes from becoming principal hops:\n%s", prompt)
	}
	if !strings.Contains(prompt, "If the **Nearest grounded mechanism** lane is absent, do NOT invent a likely internal cause") {
		t.Fatalf("typed support instructions should forbid speculative cause promotion when mechanism lane is absent:\n%s", prompt)
	}
	if strings.Contains(prompt, "## Resolved Step Sequence") {
		t.Fatalf("legacy step backbone should be suppressed when typed support lanes exist:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_CurrentStatusDecisionLane(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentRootCause,
				LogTriage: &types.LogBundle{
					Errors:        []types.LogError{{Type: "panic"}},
					ResolvedFiles: []string{"internal/agent/analyzer.go"},
				},
			},
			AnswerContract: types.AnswerContract{
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{
					Required: true,
					AllowedVerdicts: []types.CurrentStatusVerdict{
						types.CurrentStatusStillPresent,
						types.CurrentStatusFixed,
						types.CurrentStatusNotEnoughEvidence,
					},
				},
			},
		},
		LogTriage: &types.LogBundle{
			Errors:        []types.LogError{{Type: "panic", Frames: []types.LogFrame{{Raw: "frame", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"}}}},
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
		},
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "typed `decision` verdict only from the lanes below") {
		t.Fatalf("current-status support instructions should name the decision verdict lane:\n%s", prompt)
	}
	if !strings.Contains(prompt, "current_status_verdict") {
		t.Fatalf("current-status prompt should require typed verdict field:\n%s", prompt)
	}
	if !strings.Contains(prompt, "The typed verdict field is the decision carrier") {
		t.Fatalf("current-status prompt should make the typed verdict the decision carrier:\n%s", prompt)
	}
	if strings.Contains(prompt, "current_status_verdict` set to the canonical status enum. Put only the rationale and evidence boundary in block `text`; do not repeat a second canonical status token there. Attach a one-element `items=[{id:\"d\", citation_ref:N}]` when you need a citation anchor, and attach block-level `claim_uses=") {
		t.Fatalf("current-status checklist should not force decision claim_uses:\n%s", prompt)
	}
	if !strings.Contains(prompt, "`not_enough_evidence`: current cited code cannot decide") {
		t.Fatalf("current-status prompt should define not_enough_evidence narrowly:\n%s", prompt)
	}
	if strings.Contains(prompt, "verdict at the START of block `text`") {
		t.Fatalf("current-status prompt must not require a second prose verdict token:\n%s", prompt)
	}
	if !strings.Contains(prompt, "### Current status verdict synthesis") {
		t.Fatalf("current-status verdict lane missing from support plan:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Allowed block kinds: decision") {
		t.Fatalf("current-status verdict lane should explicitly allow decision blocks:\n%s", prompt)
	}
	if !strings.Contains(prompt, "- **decision** (exactly 1)") {
		t.Fatalf("semantic view should still require summary, path, and decision blocks:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_SuppressesMultiTopicStructureWhenTypedSupportPlanExists(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "panic"}}},
				SubTopics: []types.SubTopic{
					{Summary: "panic 的直接触发点"},
					{Summary: "外层调用链"},
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		LogTriage: &types.LogBundle{Errors: []types.LogError{{Type: "panic", Frames: []types.LogFrame{{Raw: "github.com/hanchaoqun/codrax/internal/agent.buildAnalysisIR(0x0)\n\tinternal/agent/analyzer.go:250 +0x1e", File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"}}}}},
		Mutable:   types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       743,
				AnchorKind:      types.AnchorCall,
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				AnchorSymbol:    "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       978,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "## Answer Structure (multi-topic)") {
		t.Fatalf("multi-topic scaffold should be suppressed when typed support lanes are authoritative:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRequestedEnumerationBoundary(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentTrace,
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 7,
					SourceQuote:   "7 checks",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// Post-shape-retirement: an EnumerationBoundary obligation routes
	// the family to QFEnumeration (symbols-slate principal payload).
	for _, want := range []string{
		"## Requested Set Boundary",
		"`7 checks` (7 item(s))",
		"Keep the principal `ordered_list` block's `items[]` slate to 7 item(s)",
		"do not silently blend them into the principal set",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("requested set boundary prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_AttributeBearingEnumerationGuidance(t *testing.T) {
	mut := types.NewMutableState("q")
	syms := []types.AnswerSymbol{{
		Name:      "aggregator",
		File:      "internal/analysis/aggregator/aggregator.go",
		Line:      1,
		Kind:      types.KindPackage,
		Rationale: "entry function New at internal/analysis/aggregator/aggregator.go:112",
	}}
	mut.SetEmittedAnswerSymbols(syms, types.CompletenessComplete)
	ctx := &types.AgentContext{
		Mutable:                  mut,
		AnswerSymbols:            syms,
		AnswerSymbolCompleteness: types.CompletenessComplete,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				RawRequest: "list all packages and each package entry point",
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
				PredicateAxis: types.AxisDefine,
				AnalyzerHints: types.AnalyzerHints{
					PrimaryEntities: []string{"packages"},
					Entities:        []string{"aggregator", "compiler"},
				},
				EnumerationBoundary: &types.RequestedEnumerationBoundary{
					DeclaredCount: 1,
					SourceQuote:   "all packages",
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all packages",
				},
			},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"attribute-bearing enumeration",
		"separate the principal member set from per-member attributes",
		"Do not reduce the principal item count just because an attribute is missing",
		"Two-axis enumeration label rule",
		"the principal `ordered_list.items[].label` is the enumerated member itself",
		"Do not use an AnswerSymbol name as the item label when that symbol is the per-member attribute",
		"entry function New at internal/analysis/aggregator/aggregator.go:112",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("attribute-bearing finalizer prompt missing %q:\n%s", want, prompt)
		}
	}
}

// TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude
// checks the other branch: when MustInclude is empty, the prompt
// says there is no explicit required-member floor, so the finalizer
// keeps the rendered list aligned with the prior slate / evidence
// rather than inventing a retired completeness field.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_NoFloorWithoutMustInclude(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Intent: types.IntentEnumerate},
			AnswerContract: types.AnswerContract{},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "Required-member floor is empty") {
		t.Errorf("no-floor branch missing: %q", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_StopsOnCanceledContext(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	cancel()
	ctx := &types.AgentContext{
		Ctx: base,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentEnumerate},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "structural emit-time constraints") {
		t.Fatalf("canceled prompt should keep the schema pointer, got:\n%s", prompt)
	}
	if strings.Contains(prompt, "Required Answer Blocks") || strings.Contains(prompt, "Expected principal-item floor") {
		t.Fatalf("canceled prompt should stop before expensive dynamic sections, got:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersDecoratedSymbolHeaderContext(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceDirect,
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "Index",
		Subject:      "Index",
		Producer:     "explorer.emit_evidence",
		SurfaceTerms: []string{"Index.ets", "@Entry"},
	}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
		AnswerSymbols: []types.AnswerSymbol{{
			Name:      "Index",
			File:      "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
			Line:      7,
			Kind:      types.KindStruct,
			Rationale: "@Entry 页面入口",
		}},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"surface_terms",
		"preserve those exact structured terms",
		"Model-Emitted Surface Terms",
		"Index.ets, @Entry",
		"internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("decorated symbol header context missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_EnumerationKeepsMoreThanSixSurfaceTerms(t *testing.T) {
	mut := types.NewMutableState("q")
	var evidence []types.EvidenceItem
	for i := 1; i <= 8; i++ {
		evidence = append(evidence, types.EvidenceItem{
			ID:           fmt.Sprintf("import-%02d", i),
			Kind:         types.EvidenceDirect,
			Scope:        types.ScopeLine,
			Source:       "internal/agent/explorer.go",
			LineStart:    10 + i,
			AnchorKind:   types.AnchorImport,
			AnchorSymbol: fmt.Sprintf("pkg%d", i),
			Producer:     "explorer.emit_evidence",
			SurfaceTerms: []string{fmt.Sprintf("github.com/acme/project/internal/pkg%d", i)},
		})
	}
	mut.AppendEvidence(evidence)
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for i := 1; i <= 8; i++ {
		want := fmt.Sprintf("github.com/acme/project/internal/pkg%d", i)
		if !strings.Contains(prompt, want) {
			t.Fatalf("enumeration surface terms should not be silently capped below complete small sets; missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DropsUnrelatedPathSurfaceTermContext(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.AppendEvidence([]types.EvidenceItem{{
		Kind:         types.EvidenceDirect,
		Source:       "internal/agent/analyzer.go",
		LineStart:    1322,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "analyzerGraphForNormalize",
		Subject:      "buildAnalysisIR",
		Object:       "analyzerGraphForNormalize",
		Producer:     "explorer.emit_evidence",
		SurfaceTerms: []string{"internal/types/enumeration_boundary.go"},
	}})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "internal/types/enumeration_boundary.go") {
		t.Fatalf("irrelevant source-path surface term should not be rendered into finalizer prompt:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersDiagramContractAndSeeds(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentTrace},
			AnswerContract: types.AnswerContract{
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
		"Do not synthesize bare line-number aliases such as `L877`, `Line 42`",
		"## Diagram Seeds",
		"### Grounded Labeling",
		"### Diagram Node Allowlist",
		"`internal/a.go`",
		"`internal/b.go`",
		"### Log Triage",
		"### Flow Findings",
		"### Answer Chains",
		"## First-Pass Diagram Reference",
		"Supporting anchor locations belong OUTSIDE the fence",
		// Log seed is now Mermaid (was bare-fence ASCII "innermost
		// failure:" prose). Assert on the grounded label that
		// survives both the Mermaid wrap and the later RenderMermaidBlocks
		// transformation.
		"innermost:",
		"Do not invent shorthand labels from citation line numbers",
		"split the hop or cite the line that actually names the action",
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
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
					ScopeHint:      types.DiagramScopeOverall,
					Reasons:        []string{"config_lineage"},
				},
			},
		},
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
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
		"## First-Pass Diagram Reference",
		"## Precedence Role Coverage",
		"`code default` [default] → `internal/types/config.go:707`",
		"`config file` [config] → `codrax.yaml.example:20`",
		"`runtime binding` [runtime] → `internal/config/runtime.go:194`",
		"`operator override` [override] → `cmd/root.go:1381`",
		"codrax.yaml.example:20",
		"internal/types/config.go:707",
		"cmd/root.go:1381",
		"prefer the validated role-labeled precedence chain below",
		"compiled role abstraction backed by grounded evidence",
		"highest precedence at the top to lowest precedence at the bottom",
		// Pre-2026-04-30 the prompt named the literal `CLI` as the
		// concrete `override` example, which over-fit the s3a eval
		// case (eval/cases/s3a.case literally asks about "code
		// default / codrax.yaml / CLI"). Generalised to "operator-
		// supplied layer (CLI flag / env override / SDK setter /
		// RPC override — all the same tier)" so the binding rule
		// is structural, not vocabulary-specific.
		"highest-precedence operator-supplied layer",
		"all the same tier",
		"`runtime` is the binding / merge code path",
		"grounded FLOOR",
		"add additional grounded precedence layers when your investigation supports a richer chain",
		"The support locations listed above stay outside the fence",
		// New two-channel rule replaces the old "rename to a bare
		// `CLI` or tier label" prohibition. The validator works
		// off these structural channels (data-driven from the
		// EvidenceDiagramRole enum), not from prompt-side keyword
		// examples.
		"three structural grounding channels",
		"role label EXACTLY as listed",
		"<content> (<role-marker>)",
		"override` / `config` / `runtime` / `default",
		"concrete file path or symbol that appears in `citations[]`",
		"Drop those nodes from the fence and explain the concept in prose",
		"## Submission Checklist",
		"FLOOR you can extend, not a verbatim ceiling",
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

// TestAnswerDocumentEvaluator_BuildInitialInstruction_NoLiteralCLIBlacklist
// is the dedicated guard against re-introducing the s3a over-fit:
// the prompt MUST NOT carry phrases that name `CLI` (or other
// vocabulary tokens from the s3a question) as a forbidden bucket
// label. The structural three-channel rule replaces the old literal
// blacklist, so neither the rejected old phrase nor a new
// equivalent should slip back in.
func TestAnswerDocumentEvaluator_BuildInitialInstruction_NoLiteralCLIBlacklist(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Scenario: types.ScenarioConfigTrace},
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					Minimum:        1,
					PreferredKinds: []types.DiagramKind{types.DiagramFlow},
				},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{Source: "internal/types/config.go", LineStart: 707, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "DefaultExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleDefault},
			{Source: "codrax.yaml.example", LineStart: 20, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorDefinition, AnchorSymbol: "ExploreHeuristics", DiagramRole: types.EvidenceDiagramRoleConfig},
			{Source: "internal/config/runtime.go", LineStart: 194, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "ExploreMidLoopMinIteration", DiagramRole: types.EvidenceDiagramRoleRuntime},
			{Source: "cmd/root.go", LineStart: 1381, GroundingStatus: types.GroundingGrounded, AnchorKind: types.AnchorAssignment, AnchorSymbol: "OverrideLayer", DiagramRole: types.EvidenceDiagramRoleOverride},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	// Specific over-fitted phrases that MUST NOT reappear.
	forbidden := []string{
		"a bare `CLI`",
		"the literal `CLI`",
		"renaming it to `Layer N`",
		"abstract bucket name (for example a generic step number, a bare `CLI`",
	}
	for _, ff := range forbidden {
		if strings.Contains(prompt, ff) {
			t.Errorf("prompt regressed to s3a-overfit phrase %q. Use the structural three-channel rule instead.", ff)
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
		UnverifiedAnalyzerFindings: []types.UnverifiedFinding{{
			Token: "explore_mid_loop_hint_budget",
			Kind:  "symbol",
		}},
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

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersCapabilitySurfaceAuthority(t *testing.T) {
	question := "Explorer stage 之前的 analyzer stage 里是否允许调用 read_file？"
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(question),
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				RawRequest: question,
				Scenario:   types.ScenarioGeneric,
			},
		},
	}
	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Capability Surface Authority",
		"`read_file`",
		"`analysis-skill` skill",
		"`ToolSuggestions`",
		"`buildToolSchemas`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
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
		// G4 (post_v2_runtime_gap_remediation, 2026-05-04): "upstream
		// state" was R4-cleaned out of the LLM-facing hint. Lock the
		// remaining contract phrasing ("the status is already locked")
		// without the pipeline-shape leak.
		"the status is already locked",
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
		"Do NOT restate `explore_mid_loop_hint_budget` in the `summary` block's text",
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
			AnswerContract: types.AnswerContract{},
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
		"`scalar` block with the literal in block `text`",
		"Keep the complete scalar answer inside the answer document blocks",
		"names the subject being measured",
		"## Scalar Lookup Discipline",
		"one named source-code literal",
		"Do not expand into adjacent helpers",
		"Every non-negative `citation_ref` on a scalar payload must point at a real entry in `citations[]`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	for _, banned := range []string{
		"value{literal, citation_ref}",
		"value{key, literal, citation_ref}",
		"boolean{decision, rationale, citation_ref}",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("prompt must not teach retired V1 payload %q:\n%s", banned, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRoleLocateScalarDiscipline(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{},
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
				Type:    "runtime error: invalid memory address or nil pointer dereference",
				Message: "nil pointer dereference while parsing analyzer output",
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
		"the exact structured log error type(s) you must mention literally in `summary` are: `runtime error: invalid memory address or nil pointer dereference`",
		"Because the typed request profile marks this as a diagnostic / root-cause artifact question, preserve these structured log error message(s) verbatim in `summary` or body: `nil pointer dereference while parsing analyzer output`",
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
			AnswerContract: types.AnswerContract{},
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
		},
		LogTriage: &types.LogBundle{
			ResolvedFiles: []string{"internal/agent/analyzer.go"},
			Errors: []types.LogError{{
				Frames: []types.LogFrame{
					{File: "internal/agent/analyzer.go", Line: 250, Func: "buildAnalysisIR"},
				},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       651,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "buildAnalysisIR",
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
			{
				Kind:            types.EvidenceConditional,
				Source:          "internal/agent/analyzer.go",
				LineStart:       861,
				AnchorKind:      types.AnchorCondition,
				AnchorSymbol:    "buildAnalysisIR",
				Condition:       "ctx == nil || ctx.Mutable == nil",
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Log Source Drift",
		"older or shifted build snapshot",
		"Do not claim that the current cited line is the exact crashing line from the log",
		"## Typed Answer Support Lanes",
		"it does NOT by itself prove the runtime artifact actually passed the guard and reached the dereference path",
		"do NOT fill the gap with generic language-runtime guesses such as nil-map write, nil-slice index, field dereference",
		"### Observed artifact facts",
		"current grounded code exposes only a protective guard near the observed site",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
	if strings.Contains(prompt, "### Nearest grounded mechanism") {
		t.Fatalf("guard-only drift prompt should not surface a dedicated nearest-mechanism lane:\n%s", prompt)
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
	mu.SetInvestigationComplete("all three nearby precedence layers were already traced before the window reset")
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
		"## Accepted Closure Status",
		"Treat this closure as a structured exploration handoff",
		"model-authored closure reason: all three nearby precedence layers were already traced before the window reset",
		"Emit `exact_resolution.status=\"absent\"`",
		"do NOT emit a principal scalar block with a synthetic literal",
		"grounded same-scope anchors may appear in `summary` even when they do not carry a validated diagram role",
		"Prefer a summary-led explanation",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q after reset:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersPrimaryAbsenceProofCitationSeeds(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetInvestigationComplete("bounded search already confirmed the exact key is absent")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
				},
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
			},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:      types.EvidenceAbsent,
				Subject:   "explore_mid_loop_hint_budget",
				Predicate: "absent_in",
				Object:    "cmd/root.go",
				Source:    "cmd/root.go",
				Scope:     types.ScopeNegative,
				NegativeQuery: &types.NegativeQuery{
					File:    "cmd/root.go",
					Pattern: "ExploreMidLoopHintBudget|ExploreMidLoop.*Budget",
				},
				NegativeScope:   types.NegativeScopeFile,
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
				DiagramRole:     types.EvidenceDiagramRoleDefault,
				GroundingStatus: types.GroundingGrounded,
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Primary Exact-Proof / Absence-Proof Citation Seeds",
		"`{\"file\":\"cmd/root.go\",\"negative_pattern\":\"ExploreMidLoopHintBudget|ExploreMidLoop.*Budget\",\"scope\":\"negative\"}`",
		"In `exact_resolution.status=\"absent\"` mode, at least one cited seed MUST be an absence-proof object",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_ConfigTraceAbsenceAsksForExplicitMissingLayerWording(t *testing.T) {
	mu := types.NewMutableState("")
	mu.SetInvestigationComplete("bounded search already confirmed the exact key is absent")
	mu.SetAbsenceJustification("no config key named `explore_mid_loop_hint_budget` exists in the repo")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.AgentContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:           types.SubjectConfigKey,
					TargetLabel:          "config key",
					Targets:              []string{"explore_mid_loop_hint_budget"},
					AllowAbsence:         true,
					RelatedContextPolicy: types.ExactContextSameFamilyGrounded,
					RequestedContextRoles: []types.EvidenceDiagramRole{
						types.EvidenceDiagramRoleDefault,
						types.EvidenceDiagramRoleConfig,
						types.EvidenceDiagramRoleOverride,
					},
				},
			},
			RequestModel: types.RequestModel{
				Scenario:      types.ScenarioConfigTrace,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectConfigKey},
				Buckets: []types.QuestionBucket{
					{Label: "code default", Index: 1},
					{Label: "codrax.yaml", Index: 2},
					{Label: "CLI", Index: 3},
				},
			},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"say that layer absence explicitly",
		"`no config-file key matches this target`",
		"`no CLI flag binds this key`",
		"instead of vague placeholders like `N/A` / `不适用`",
		"Populate document-level `missing_requested_roles[]` with the missing requested layers below",
		"## Explicit Missing-Layer Wording",
		"`CLI`, prefer explicit absence wording such as `CLI 层未绑定该键` or `no CLI flag binds this key`",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DoesNotMentionRetiredTopLevelScalarPayloads(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectFunctionName},
			},
		},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFRoleLookup,
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, SurfaceRoleHint: types.SurfacePrincipal},
			{Kind: types.BlockScalar, Required: true, SurfaceRoleHint: types.SurfacePrincipal},
			{Kind: types.BlockDecision, Required: true, SurfaceRoleHint: types.SurfacePrincipal},
		},
	}

	prompt := renderAnswerDocSubmissionChecklist(ctx, view, false)
	for _, forbidden := range []string{"value{", "boolean{", "retired top-level scalar payload", "retired top-level decision payload"} {
		if strings.Contains(prompt, forbidden) {
			t.Fatalf("submission checklist must not mention retired top-level payload %q:\n%s", forbidden, prompt)
		}
	}
	for _, want := range []string{
		"Keep the complete scalar answer inside the answer document blocks",
		"Keep the complete decision answer inside the answer document blocks",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("submission checklist missing positive guidance %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildRetryInstruction_UsesPluralBlockClaimUsesPath(t *testing.T) {
	mut := types.NewMutableState("")
	mut.SetRetryState(&types.RetryState{
		Attempt: 1,
		PrevEmitSummary: types.RetryStateSummary{
			BlockSummaries: []types.RetryBlockSummary{{
				ID:   "b1",
				Kind: types.BlockSummary,
			}},
		},
		ActiveViolations: []types.ScoredViolation{{
			Severity:  types.SeverityHigh,
			Kind:      types.ViolPrincipalClaimUseMissing,
			FieldPath: "blocks[0].claim_uses",
			Detail:    "principal block missing claim use",
			Repair:    "add block-level claim_uses",
			Layer:     "finalize",
		}},
	})
	hint := renderAnswerDocRetryState(&types.AgentContext{Mutable: mut})
	if !strings.Contains(hint, "`blocks[].claim_uses[]`") {
		t.Fatalf("retry instruction must mention blocks[].claim_uses[] path:\n%s", hint)
	}
	if strings.Contains(hint, "`blocks[].claim_use`") {
		t.Fatalf("retry instruction must not mention stale blocks[].claim_use path:\n%s", hint)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_DoesNotHardRequireLogMessagesForNonDiagnosticIntent(t *testing.T) {
	ctx := &types.AgentContext{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type:    "panic",
				Message: "index out of bounds: index=5, size=3",
			}},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if strings.Contains(prompt, "preserve these structured log error message(s) verbatim") {
		t.Fatalf("non-diagnostic intent must not hard-require log message literals:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Structured log error type") {
		t.Fatalf("log artifact should still be available as soft context:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersExternalObservationSeeds(t *testing.T) {
	ctx := &types.AgentContext{
		LogTriage: &types.LogBundle{
			Errors: []types.LogError{{
				Type: "runtime error: invalid memory address or nil pointer dereference",
				Frames: []types.LogFrame{
					{
						File: "internal/agent/analyzer.go",
						Line: 320,
						Func: "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput",
						Raw:  "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput(0x0, 0x0, 0x0)",
					},
				},
			}},
		},
		EvidenceItems: []types.EvidenceItem{
			{
				Kind:            types.EvidenceRelationship,
				Source:          "internal/agent/analyzer.go",
				LineStart:       651,
				AnchorKind:      types.AnchorCall,
				AnchorSymbol:    "buildAnalysisIR",
				Subject:         "ParseOutput",
				Object:          "buildAnalysisIR",
				GroundingStatus: types.GroundingGrounded,
			},
		},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario: types.ScenarioRootCause,
				Intent:   types.IntentRootCause,
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Typed Answer Support Lanes",
		"### Observed artifact facts",
		"structured runtime error type",
		`runtime artifact identifies error head stack frame "github.com/hanchaoqun/codrax/internal/agent.(*analyzerEvaluator).ParseOutput" at observed internal/agent/analyzer.go:320`,
		"internal/agent/analyzer.go:651",
		"Items rendered under the **Observed artifact facts** lane are runtime trace observations",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_BuildInitialInstruction_RendersRuntimeGroundingDisposition(t *testing.T) {
	mut := types.NewMutableState("q")
	// A runtime artifact MUST be attached for the disposition to
	// activate — the disposition's whole vocabulary ("the attached
	// log / trace") presumes an artifact in scope. The 2026-05-16
	// finalize-loop bug fix gates the waiver→disposition projection
	// on this. Without the LogTriage below, the prompt section would
	// (correctly) stay empty.
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "synthetic_frame",
		}}}},
	})
	mut.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    types.EvidenceFloorWaiverNoRepoIntersection,
		Rationale: "synthetic frames represent a different deployed build",
	})
	mut.RetainEvidenceFloorWaiver()
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Scenario:  types.ScenarioRootCause,
				Intent:    types.IntentRootCause,
				LogTriage: mut.LogTriage(),
			},
			AnswerContract: types.AnswerContract{},
		},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, want := range []string{
		"## Runtime Grounding Disposition",
		"`no_repo_intersection`",
		"synthetic frames represent a different deployed build",
		"`citation_ref=-1`",
		"This dispatch is observation-only",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}
}

func TestAnswerDocumentEvaluator_RuntimeObservationOnlySuppressesRepoEnrichment(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{
			Func: "load_config",
		}}}},
	})
	ctx := &types.AgentContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
			},
			AnswerContract: types.AnswerContract{},
		},
		EvidenceItems: []types.EvidenceItem{{
			ID:           "repo-helper",
			Source:       "internal/env/cache/disk_cache.go",
			LineStart:    179,
			Kind:         types.EvidenceConcrete,
			Subject:      "Cache.load",
			AnchorKind:   types.AnchorReturn,
			AnchorSymbol: "Cache.load",
			Origin:       types.ClaimOriginCurrentRepo,
		}},
	}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	for _, banned := range []string{
		"lane=value_fact label=`Cache.load`",
		"internal/env/cache/disk_cache.go:179",
		"`current_code_path\"`. (evidence: 1)",
	} {
		if strings.Contains(prompt, banned) {
			t.Fatalf("observation-only runtime prompt leaked repo enrichment %q:\n%s", banned, prompt)
		}
	}
	if !strings.Contains(prompt, "This dispatch is observation-only") {
		t.Fatalf("prompt missing observation-only boundary:\n%s", prompt)
	}
}

func TestAnswerDocumentEvaluator_LogSourceDriftHonorsRuntimeDisposition(t *testing.T) {
	plan := &types.AnswerSurfacePlan{
		RuntimeGroundingDisposition: &types.RuntimeGroundingDisposition{
			Source:         types.RuntimeGroundingModelDeclared,
			Reason:         types.EvidenceFloorWaiverNoRepoIntersection,
			Rationale:      "different deployed build",
			CitationPolicy: types.RuntimeGroundingCitationRuntimeObservation,
		},
		LogSourceDriftAnchors: []types.LogSourceDriftAnchor{{
			File:         "a.go",
			ObservedLine: 20,
			AnchoredLine: 40,
			Func:         "Run",
		}},
	}
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState("q"),
		AnalysisIR: &types.AnalysisIR{
			RequestModel:   types.RequestModel{Scenario: types.ScenarioRootCause, Intent: types.IntentRootCause},
			AnswerContract: types.AnswerContract{},
		},
	}
	ctx.Mutable.SetEvidenceFloorWaiver(&types.EvidenceFloorWaiver{
		Reason:    plan.RuntimeGroundingDisposition.Reason,
		Rationale: plan.RuntimeGroundingDisposition.Rationale,
	})
	ctx.Mutable.RetainEvidenceFloorWaiver()
	ctx.EvidenceItems = []types.EvidenceItem{{
		Source:          "a.go",
		LineStart:       40,
		AnchorSymbol:    "Run",
		GroundingStatus: types.GroundingGrounded,
		Authority:       types.AuthorityConditional,
		Origin:          types.ClaimOriginLog,
		DriftReason:     types.DriftReasonLineDrift,
	}}
	ctx.LogTriage = &types.LogBundle{Errors: []types.LogError{{Frames: []types.LogFrame{{File: "a.go", Line: 20, Func: "Run", Raw: "a.go:20"}}}}}

	prompt := (&answerDocumentEvaluator{}).BuildInitialInstruction(ctx, nil)
	if !strings.Contains(prompt, "observation-first for citation purposes") {
		t.Fatalf("drift section must honor runtime disposition:\n%s", prompt)
	}
	if strings.Contains(prompt, "Treat the current repo as the authoritative explanation surface") {
		t.Fatalf("drift section must not contradict runtime disposition:\n%s", prompt)
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

// TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing is the
// complement: no doc in Mutable → Observe returns HintRequested.
func TestAnswerDocumentEvaluator_Observe_RetriesWhenDocMissing(t *testing.T) {
	mu := types.NewMutableState("") // empty Mutable
	e := &answerDocumentEvaluator{mu: mu, maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, softStopObs(0))
	if !sig.HintRequested {
		t.Error("doc missing: HintRequested = false, want true")
	}
	if !sig.BypassThrottle {
		t.Fatalf("missing-document correction must bypass throttle so finalizer retries cannot be swallowed, got %+v", sig)
	}
	if !sig.BypassBudget {
		t.Fatalf("missing-document correction must bypass ordinary hint budget so finalizer delivery remains bounded by maxRetries, got %+v", sig)
	}
	if e.retriesUsed != 1 {
		t.Errorf("retriesUsed = %d, want 1", e.retriesUsed)
	}
}

func TestAnswerDocumentEvaluator_MissingDocumentHintDoesNotInventBlankDraft(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, LoopObservation{
		Phase:    PhaseSoftStop,
		Response: llm.Response{Content: "\n\n"},
	})
	if !sig.HintRequested {
		t.Fatalf("blank no-tool finalizer response should request correction")
	}
	if strings.Contains(sig.Hint, "You already drafted the answer") {
		t.Fatalf("blank response must not be described as an existing draft: %q", sig.Hint)
	}
	if !strings.Contains(sig.Hint, "did not contain a usable visible draft") {
		t.Fatalf("blank response hint should name the actual condition: %q", sig.Hint)
	}
}

func TestAnswerDocumentEvaluator_MissingDocumentHintPreservesVisibleDraft(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: types.DefaultAgentSettings().FinalizerMaxCorrectionRetries}
	sig := e.Observe(nil, LoopObservation{
		Phase:    PhaseSoftStop,
		Response: llm.Response{Content: "Here is a rich draft answer."},
	})
	if !sig.HintRequested {
		t.Fatalf("visible no-tool finalizer draft should request structured emit")
	}
	if !strings.Contains(sig.Hint, "You already drafted the answer") {
		t.Fatalf("visible draft should still get preservation guidance: %q", sig.Hint)
	}
	if strings.Contains(sig.Hint, "did not contain a usable visible draft") {
		t.Fatalf("visible draft should not use blank-response guidance: %q", sig.Hint)
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
			Summary:  "summary length 2782 exceeds cap 2500 — shorten the summary",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("summary-cap reject should request a correction hint, got %+v", sig)
	}
	if !sig.BypassThrottle {
		t.Fatalf("summary-cap reject hint should bypass throttle, got %+v", sig)
	}
	if !sig.BypassBudget {
		t.Fatalf("summary-cap reject hint should bypass the ordinary mid-loop budget, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "2500") || !strings.Contains(sig.Hint, "2782") {
		t.Fatalf("targeted summary-cap hint missing cap detail: %q", sig.Hint)
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
			Summary:  "summary length 2782 exceeds cap 2500 — shorten the summary",
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
		"grounded `diagram` block",
		"emit_answer_document",
		"diagram.kind",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("missing-diagram hint missing %q: %q", want, sig.Hint)
		}
	}
}

func TestAnswerDocumentEvaluator_Observe_UnexpectedFinalizerToolBypassesThrottle(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 2,
		Response: llm.Response{ToolCalls: []llm.ToolCall{{
			Name: "read_file",
		}}},
	})
	if !sig.HintRequested {
		t.Fatalf("unexpected finalizer tool should request a protocol correction, got %+v", sig)
	}
	if !sig.BypassThrottle {
		t.Fatalf("unexpected finalizer tool correction must bypass throttle, got %+v", sig)
	}
	if !strings.Contains(sig.Hint, "read_file") || !strings.Contains(sig.Hint, "emit_answer_document") {
		t.Fatalf("unexpected tool hint missing tool/protocol detail: %q", sig.Hint)
	}
}

func TestCompactToolRejectSummaryPrefersStructuredFieldActions(t *testing.T) {
	summary := "The answer document does not yet meet the structural contract for this question.\n\n" +
		"  1. Field: `citations[]`\n" +
		"     Action: preserve / emit a top-level citations[] pool with at least 9 entries so every non-negative citation_ref resolves\n" +
		"     Why: citation refs are zero-based\n" +
		"  2. Field: `blocks[].items[].label/text OR blocks[].text`\n" +
		"     Action: include every model-emitted principal member_set member in the visible answer\n" +
		"  3. Field: `blocks[].text/count claims`\n" +
		"     Action: make every visible count claim match the member_set cardinality; current_citation is INVALID, not a target. Use a candidate_citations entry when present, or change the label to an endpoint actually present at current_citation: block=\"main\" item=\"i6\" label=\"subExplorerEvaluator\" current_citation=internal/agent/explorer.go:30 candidate_citations=[internal/agent/sub_explorer.go:135, internal/agent/sub_explorer.go:139]\n"
	got := compactToolRejectSummary(summary)
	for _, want := range []string{
		"`citations[]`: preserve / emit",
		"`blocks[].items[].label/text OR blocks[].text`: include every model-emitted",
		"`blocks[].text/count claims`: make every visible count claim",
		"candidate_citations=[internal/agent/sub_explorer.go:135, internal/agent/sub_explorer.go:139]",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("compact structured summary missing %q:\n%s", want, got)
		}
	}
	if strings.HasPrefix(got, "The answer document does not yet meet") {
		t.Fatalf("compact summary should not stop at the generic heading: %q", got)
	}
}

func TestAnswerDocumentEvaluator_Observe_MidLoopRejectHintIncludesFieldActions(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary: "The answer document does not yet meet the structural contract for this question.\n\n" +
				"  1. Field: `citations[]`\n" +
				"     Action: preserve / emit a top-level citations[] pool with at least 9 entries\n" +
				"  2. Field: `blocks[].items[].label`\n" +
				"     Action: structured answer anchor label(s) must preserve: explorer\n",
		},
	})
	if !sig.HintRequested {
		t.Fatalf("tool reject should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"`citations[]`: preserve / emit",
		"`blocks[].items[].label`: structured answer anchor",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("reject hint missing compact action %q:\n%s", want, sig.Hint)
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
		// Hint reframed alongside the seed-extension authorisation:
		// the precedence chain is a FLOOR, not a verbatim ceiling.
		// The grounded reference fence is Mermaid and keeps only the
		// role labels inside the fence; supporting file:line anchors
		// stay elsewhere in the prompt so retries do not copy them
		// into node labels.
		"the seeded grounded precedence chain is the FLOOR",
		"```mermaid",
		"flowchart",
		"runtime binding",
		"code default",
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
	// Seed is now a ```mermaid``` flowchart (was ASCII art with
	// "innermost failure:" / "  ->" prose). Assert on the grounded
	// labels themselves — those survive the format change.
	for _, want := range []string{
		"```mermaid",
		"flowchart",
		"internal/agent/analyzer.go:320",
		"buildAnalysisIR",
		"internal/orchestrator/orchestrator.go:101",
		"Run",
		"-->",
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
		"```mermaid",
		"flowchart",
		"config.handlers.explorer",
		"NewExplorer",
		"Register",
		"-->",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("flow retry seed missing %q:\n%s", want, got)
		}
	}
}

func TestRenderRetryDiagramSeedFenceForRepair_SequencePrefersAnswerChainsOverFlowNoise(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramSequence},
				},
			},
		},
		FlowFindings: []types.FlowFindingDigest{{
			Path: []string{
				`finding.Conditions, " AND ", "## Cross-References"`,
				`"Evidence Gaps" prompt scaffold`,
			},
		}},
		AnswerChains: []types.AnswerChain{
			{Item: types.EvidenceItem{Subject: "ClientRequest.Start", Source: "internal/a.go", LineStart: 10, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Subject: "Coordinator.Dispatch", Source: "internal/b.go", LineStart: 20, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Subject: "WorkerRuntime.Run", Source: "internal/c.go", LineStart: 30, GroundingStatus: types.GroundingGrounded}},
			{Item: types.EvidenceItem{Subject: "LeafWorker.Execute", Source: "internal/d.go", LineStart: 40, GroundingStatus: types.GroundingGrounded}},
		},
	}

	got := renderRetryDiagramSeedFenceForRepair(ctx, &types.ToolRepair{})
	for _, want := range []string{
		"```mermaid",
		"sequenceDiagram",
		"ClientRequest.Start",
		"Coordinator.Dispatch",
		"WorkerRuntime.Run",
		"LeafWorker.Execute",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("sequence retry seed missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "Evidence Gaps") || strings.Contains(got, "Cross-References") {
		t.Fatalf("sequence retry seed should not let generic flow-finding prose shadow answer-chain nodes:\n%s", got)
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
		"```mermaid",
		"flowchart",
		"internal/a.go:10",
		"internal/b.go:20",
		"-->",
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
		"runtime binding",
		"code default",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("config-trace repair seed missing %q:\n%s", want, got)
		}
	}
	for _, banned := range []string{
		"internal/config/runtime.go:231",
		"internal/types/config.go:707",
	} {
		if strings.Contains(got, banned) {
			t.Fatalf("config-trace repair seed should keep support locations outside the fence, but found %q:\n%s", banned, got)
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

func TestRenderRetryDiagramSeedFenceForRepair_FallsBackToAllowedCitationsWhenScenarioSeedsDoNotMatch(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			AnswerContract: types.AnswerContract{
				Diagram: &types.DiagramContract{
					Required:       true,
					PreferredKinds: []types.DiagramKind{types.DiagramCallDAG},
				},
			},
		},
		FlowFindings: []types.FlowFindingDigest{
			{Path: []string{"internal/agent/agent.go:1786", "internal/agent/agent.go:1814"}},
		},
	}
	repair := &types.ToolRepair{
		Code: "diagram_codename",
		Metadata: map[string]string{
			"allowed_citations": "internal/agent/analyzer.go:651, internal/agent/analyzer.go:861",
		},
	}
	got := renderRetryDiagramSeedFenceForRepair(ctx, repair)
	for _, want := range []string{
		"internal/agent/analyzer.go:651",
		"internal/agent/analyzer.go:861",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repair seed should fall back to current allowed citations %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "internal/agent/agent.go:1786") {
		t.Fatalf("repair seed should not reuse unrelated scenario seed when it falls outside the allowed citation set:\n%s", got)
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
		"Keep `citations[]` byte-identical while repairing this gate",
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
		"pure answer synthesizer",
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
		// Reworded post-2026-04-30: the hint now uses the explicit
		// "paste the FULL previous payload byte-identical" preserve
		// directive instead of the ambiguous "Only change the named
		// field(s)" wording, which empirically caused models to
		// drop the rest of the payload on retry. Assert on the new
		// preserve language.
		"paste the FULL previous payload byte-identical",
		"Every other field must stay byte-identical",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("generic reject hint missing %q: %q", want, sig.Hint)
		}
	}
}

// TestAnswerDocumentEvaluator_Observe_PayloadRegressionOverridesFieldHint pins
// the catastrophic-regression override: when the tool flags
// `payload_regression` (the LLM's retry collapsed multiple grounded
// fields vs the prior emit), the hint must promote payload
// restoration to the FIRST instruction and demote field correction
// to secondary. Without this, the iter=2→iter=3 collapse trace
// (citations 18→0; steps 16→0; summary 4250→0) would keep firing
// per-field rejections that the model interprets as "emit only this
// field", shrinking the payload further each round.
func TestAnswerDocumentEvaluator_Observe_PayloadRegressionOverridesFieldHint(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 1,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:literal_grounding] steps[2].description not corroborated by citations[5]",
			Repair: &types.ToolRepair{
				Code:   "payload_regression:literal_grounding",
				Fields: []string{"steps[2].description", "steps[2].citation_ref"},
				Hint:   "Fix steps[2].description so cited identifiers overlap.",
				Metadata: map[string]string{
					"payload_regression_summary": "citations 18→0; steps 16→0; summary runes 4250→0",
				},
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("payload-regression should request a hint, got %+v", sig)
	}
	for _, want := range []string{
		"REGRESSED",
		"PASTE THE FULL PRIOR PAYLOAD BACK byte-identical",
		"citations 18→0; steps 16→0; summary runes 4250→0",
		"Restoring the payload comes FIRST",
		"steps[2].description",
		"steps[2].citation_ref",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("payload-regression hint missing %q: %q", want, sig.Hint)
		}
	}
	if !strings.Contains(sig.HintKey, "payload-regression") {
		t.Errorf("payload-regression should drive HintKey reasonKey; got %q", sig.HintKey)
	}
}

// TestComputeAnswerDocAttemptShape locks the attempt-shape capture so
// the regression detector's input is well-defined for tests.
func TestComputeAnswerDocAttemptShape(t *testing.T) {
	// Cannot import emit_answer_document params type from here —
	// asserting via the public detector path is enough. The
	// regression detector is the load-bearing consumer.
	prior := &types.AnswerDocAttemptShape{
		CitationsCount: 18, StepsCount: 16, SymbolsCount: 0, SummaryRunes: 4250,
		HasValue: false, HasBoolean: false, HasExactResolution: true,
	}
	if !shapeIndicatesContent(prior) {
		t.Errorf("prior with substantial payload must register as content-bearing")
	}
}

func shapeIndicatesContent(s *types.AnswerDocAttemptShape) bool {
	if s == nil {
		return false
	}
	return s.CitationsCount+s.StepsCount+s.SymbolsCount+s.SummaryRunes >= 16
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

func TestAnswerDocumentEvaluator_Observe_MidLoopCitationRefRepairUsesStructuredHint(t *testing.T) {
	e := &answerDocumentEvaluator{maxRetries: 2}
	sig := e.Observe(nil, LoopObservation{
		Phase:     PhaseMidLoop,
		Iteration: 0,
		LastToolResult: &types.ToolResult{
			ToolName: "emit_answer_document",
			Success:  false,
			Summary:  "[answer_doc_reject:citation_ref_range] steps[3].citation_ref 6 is out of range (citations pool has 6 entries)",
			Repair: &types.ToolRepair{
				Code:   "citation_ref_range",
				Fields: []string{"steps[3].citation_ref"},
				Hint:   "Re-emit `emit_answer_document` and fix ONLY `steps[3].citation_ref`: citation_ref is zero-based. The current citations[] pool has 6 entries, so valid indices are `0` through `5`, or `-1` for 'no citation'. Keep the existing grounded evidence and renumber only the offending citation_ref fields; do not reopen files.",
			},
		},
	})
	if !sig.HintRequested {
		t.Fatalf("structured citation_ref repair should request a correction hint, got %+v", sig)
	}
	for _, want := range []string{
		"steps[3].citation_ref",
		"zero-based",
		"`0` through `5`",
		"Do not write free-form prose outside the tool call",
	} {
		if !strings.Contains(sig.Hint, want) {
			t.Fatalf("citation_ref repair hint missing %q: %q", want, sig.Hint)
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

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_FailLoudZh(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "原始兜底内容"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Errorf("zh fail-loud warning should be localized: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "未能生成结构化答案") {
		t.Errorf("zh fail-loud warning missing: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "原始兜底内容") {
		t.Errorf("raw content lost: %q", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_RecoversAnswerDocumentJSONContent(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	messages := []llm.Message{{
		Role: "assistant",
		Content: `{
  "blocks": [
    {
      "id": "summary",
      "kind": "summary",
      "surface_role": "principal",
      "text": "## 系统架构概述\n\n可见答案应从 block text 渲染，而不是展示原始 JSON。"
    },
    {
      "id": "diagram",
      "kind": "diagram",
      "diagram": {
        "kind": "sequence",
        "language": "mermaid",
        "body": "sequenceDiagram\nUser->>System: ask"
      }
    }
  ],
  "citations": [
    {"file": "internal/agent/agent.go", "line": 859}
  ]
}`,
	}}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Fatalf("recovered answer should not use raw-content missing-doc banner: %q", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, `"blocks"`) || strings.Contains(out.FinalAnswer, `"citations"`) {
		t.Fatalf("recovered answer leaked raw JSON: %q", out.FinalAnswer)
	}
	doc := ctx.Mutable.AnswerDocumentV2()
	if doc == nil {
		t.Fatal("lossless recovered answer document should be restored to Mutable for contract validation")
	}
	if len(doc.Blocks) != 2 || len(doc.Citations) != 1 {
		t.Fatalf("recovered Mutable document shape = blocks:%d citations:%d, want 2/1", len(doc.Blocks), len(doc.Citations))
	}
	for _, want := range []string{
		"系统架构概述",
		"可见答案应从 block text 渲染",
		"```mermaid",
		"sequenceDiagram",
		"internal/agent/agent.go:859",
		"已从模型写在文本中的 answer_document JSON 恢复本答案",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("recovered answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_RendersRetryStateDraftAndRawContent(t *testing.T) {
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "上一版结构化答案正文",
		}},
		Citations: []types.Citation{{File: "internal/agent/agent.go", Line: 969}},
	}
	prevJSON, err := json.Marshal(prevDoc)
	if err != nil {
		t.Fatalf("marshal prev doc: %v", err)
	}
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
	}
	ctx.Mutable.SetRetryState(&types.RetryState{
		Attempt:      2,
		PrevEmitJSON: prevJSON,
	})
	messages := []llm.Message{
		{Role: "assistant", Content: "【审计员注】patch 仍未通过。"},
	}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, "answer_document emission missing") {
		t.Fatalf("retry-state draft recovery should avoid raw-only missing-doc banner: %q", out.FinalAnswer)
	}
	for _, want := range []string{
		"上一版结构化答案正文",
		"internal/agent/agent.go:969",
		"最终重试未能产出有效的 answer_document",
		"模型最后一轮原文",
		"【审计员注】patch 仍未通过。",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
	}
	if doc := ctx.Mutable.AnswerDocumentV2(); doc != nil {
		t.Fatalf("retry-state recovery should not mark rejected draft as accepted mutable document: %+v", doc)
	}
}

func TestAnswerDocumentEvaluator_ParseOutput_TextRecoveryKeepsRetryStateDiagram(t *testing.T) {
	prevDoc := &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:   "summary",
				Kind: types.BlockSummary,
				Text: "上一版结构化答案正文",
			},
			{
				ID:    "diagram",
				Kind:  types.BlockDiagram,
				Title: "执行图",
				Diagram: &types.AnswerDiagramBlock{
					Kind:     types.DiagramFlow,
					Language: "mermaid",
					Body:     "flowchart TD\n  A[Agent] --> B[Tool]",
				},
			},
		},
	}
	prevJSON, err := json.Marshal(prevDoc)
	if err != nil {
		t.Fatalf("marshal prev doc: %v", err)
	}
	ctx := &types.AgentContext{Mutable: types.NewMutableState("")}
	ctx.Mutable.SetRetryState(&types.RetryState{
		Attempt:      2,
		PrevEmitJSON: prevJSON,
	})
	messages := []llm.Message{{
		Role: "assistant",
		Content: `{
			"blocks": [
				{"id": "summary", "kind": "summary", "text": "文本恢复正文"}
			]
		}`,
	}}
	e := &answerDocumentEvaluator{language: "zh"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	for _, want := range []string{
		"文本恢复正文",
		"执行图",
		"```mermaid",
		"flowchart TD",
	} {
		if !strings.Contains(out.FinalAnswer, want) {
			t.Fatalf("final answer missing %q:\n%s", want, out.FinalAnswer)
		}
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

func TestAnswerDocumentEvaluator_ParseOutput_MissingDoc_SynthesizesAuthorityCaveatFromContextEvidence(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:        "ev1",
			Source:    "internal/agent/analyzer.go",
			LineStart: 981,
			Authority: types.AuthorityHistorical,
		}},
	}
	messages := []llm.Message{
		{Role: "assistant", Content: "raw fallback text"},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out, err := e.ParseOutput(ctx, messages, nil, nil)
	if err != nil {
		t.Fatalf("ParseOutput err: %v", err)
	}
	if strings.Contains(out.FinalAnswer, render.AuthorityCaveatTag()) {
		t.Fatalf("fallback output leaked private authority tag: %q", out.FinalAnswer)
	}
	if strings.Contains(out.FinalAnswer, "Authority:") {
		t.Fatalf("fallback output should hide authority caveat from user surface: %q", out.FinalAnswer)
	}
	if !strings.Contains(out.FinalAnswer, "raw fallback text") {
		t.Fatalf("fallback output lost raw answer body: %q", out.FinalAnswer)
	}
}

func TestAnswerDocumentEvaluator_ParseOutputV2_AuthorityCaveatUsesMergedEvidencePool(t *testing.T) {
	ctx := &types.AgentContext{
		Mutable: types.NewMutableState(""),
		EvidenceItems: []types.EvidenceItem{{
			ID:        "ev1",
			Source:    "internal/agent/analyzer.go",
			LineStart: 981,
			Authority: types.AuthorityHistorical,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "summary body"},
		},
	}
	e := &answerDocumentEvaluator{language: "en"}
	out := &StageOutput{}
	got, err := e.parseOutputV2(ctx, doc, out)
	if err != nil {
		t.Fatalf("parseOutputV2 err: %v", err)
	}
	if strings.Contains(got.FinalAnswer, render.AuthorityCaveatTag()) {
		t.Fatalf("rendered V2 answer leaked private authority tag:\n%s", got.FinalAnswer)
	}
	if strings.Contains(got.FinalAnswer, "Authority:") {
		t.Fatalf("rendered V2 answer should hide authority caveat from user surface:\n%s", got.FinalAnswer)
	}
	if !strings.Contains(got.FinalAnswer, "summary body") {
		t.Fatalf("rendered V2 answer lost summary body:\n%s", got.FinalAnswer)
	}
	foundStructuredCaveat := false
	for _, blk := range doc.Blocks {
		if blk.Kind == types.BlockCaveat && strings.Contains(blk.Text, render.AuthorityCaveatTag()) {
			foundStructuredCaveat = true
			break
		}
	}
	if !foundStructuredCaveat {
		t.Fatalf("authority hedging should remain on the structured doc for downstream checks; doc=%+v", doc.Blocks)
	}
}

// TestAnswerDocumentEvaluator_ParseOutput_CardinalityDowngrade is the
// critical P2.2 test: when Shape == list_of_symbols + Completeness ==
// complete + len(Symbols) < baseline, ParseOutput downgrades to
// lower_bound and appends a caveat. Reuses the same validator as
// extractorEvaluator so this test pins the cross-stage contract.

// TestAnswerDocumentEvaluator_ParseOutput_NoDowngrade — completeness
// complete with enough symbols passes through unchanged.

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
	// reintroduce the prose-writing pathway. The expanded set is
	// {emit_answer_document, emit_answer_document_patch} — patch
	// is the protocol-level retry preservation tool used on retry
	// paths only.
	allowed := map[string]bool{
		"emit_answer_document":       true,
		"emit_answer_document_patch": true,
	}
	for _, name := range sk.ToolSuggestions {
		if !allowed[name] {
			t.Errorf("answer-document-skill ToolSuggestions has unexpected entry %q (allowed: %v)",
				name, allowed)
		}
	}
}

// TestAnswerDocAttachEscalation pins B2-F4's retry escalation
// contract: same-issue retry text must visibly differ between
// attempts so the LLM knows it's being re-prompted on a
// persisted failure rather than reading a fresh issue.
//
//	attempt 1 → no escalation (first time hitting this issue)
//	attempt 2 → "RETRY ... your previous fix did not address it"
//	attempt 3+ → "FINAL RETRY ... fix NOW or the answer ships with violation"
//
// The dedup-key contract (rejectHintsUsed embedded in HintKey) is
// orthogonal — it ensures each retry actually delivers the hint;
// this test ensures the hint TEXT escalates.
func TestAnswerDocAttachEscalation(t *testing.T) {
	hint := "Attach claim_use to block X."
	cases := []struct {
		attempt        int
		mustEqual      string
		mustNotContain []string
		mustContain    []string
	}{
		{
			attempt:        1,
			mustEqual:      hint, // no escalation
			mustNotContain: []string{"RETRY", "FINAL RETRY"},
		},
		{
			attempt:        2,
			mustContain:    []string{"RETRY", "2nd attempt", "previous fix did not address"},
			mustNotContain: []string{"FINAL RETRY"},
		},
		{
			attempt:     3,
			mustContain: []string{"FINAL RETRY", "fix the named field NOW", "ships with the violation"},
		},
		{
			attempt:     5,
			mustContain: []string{"FINAL RETRY", "attempt #5"},
		},
	}
	for _, tc := range cases {
		got := answerDocAttachEscalation(hint, tc.attempt)
		if tc.mustEqual != "" && got != tc.mustEqual {
			t.Errorf("attempt=%d: got %q, want %q", tc.attempt, got, tc.mustEqual)
		}
		for _, want := range tc.mustContain {
			if !strings.Contains(got, want) {
				t.Errorf("attempt=%d: missing %q in %q", tc.attempt, want, got)
			}
		}
		for _, banned := range tc.mustNotContain {
			if strings.Contains(got, banned) {
				t.Errorf("attempt=%d: leaks %q in %q", tc.attempt, banned, got)
			}
		}
	}
}

// ── R7 typed-set 字段值 verbatim 渲染 (post_shape_residual_audit
// 2026-05-04) ─────────────────────────────────────────────────────

// TestRenderAnswerDocBlockRequirement_VerbatimTypedSets pins R7's
// fix: BlockRequirement.FacetIDs / AcceptableClaimForms /
// SurfaceRoleHint must each render as JSON-ready string lists the
// LLM can copy verbatim into block.facet_ids[] / block.claim_uses[].claim_form
// / surface_role.
//
// Pre-R7 the prose only said "covers facet(s): X" without
// instructing the LLM to copy "X" into the emit's typed field, so
// the LLM 100% missed HARD facets in eval verification (4/4 runs).
func TestRenderAnswerDocBlockRequirement_VerbatimTypedSets(t *testing.T) {
	req := types.BlockRequirement{
		Kind:                 types.BlockSection,
		MinCount:             1,
		MaxCount:             1,
		Required:             true,
		Rationale:            "the principal explanation surface",
		FacetIDs:             []string{"current_code_path", "component_relation"},
		AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
		SurfaceRoleHint:      types.SurfacePrincipal,
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, nil, true)
	got := b.String()

	// Verbatim FacetID strings (LLM copies into block.facet_ids[]).
	for _, want := range []string{
		"`block.facet_ids` MUST include",
		`"current_code_path"`,
		`"component_relation"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing verbatim facet_ids token %q in:\n%s", want, got)
		}
	}
	// Verbatim ClaimForm strings.
	for _, want := range []string{
		"`block.claim_uses[]` entry's `claim_form` MUST be one of",
		`"definition_fact"`,
		`"call_edge"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing verbatim claim_form token %q in:\n%s", want, got)
		}
	}
	// Verbatim SurfaceRole.
	if !strings.Contains(got, `"principal"`) {
		t.Errorf("missing verbatim surface_role 'principal' in:\n%s", got)
	}
	if !strings.Contains(got, "`block.surface_role`") {
		t.Errorf("missing surface_role field name in:\n%s", got)
	}
}

func TestRenderAnswerDocBlockRequirement_TypedDecisionCarrierDoesNotRequireClaimUse(t *testing.T) {
	req := types.BlockRequirement{
		Kind:                 types.BlockDecision,
		MinCount:             1,
		MaxCount:             1,
		Required:             true,
		AcceptableClaimForms: []types.ClaimForm{types.ClaimGuardCondition, types.ClaimDefinitionFact},
		SurfaceRoleHint:      types.SurfacePrincipal,
	}
	view := &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, view, true)
	got := b.String()
	if strings.Contains(got, "`block.claim_uses[]` entry's `claim_form` MUST be one of") {
		t.Fatalf("typed decision carrier should not force claim_uses:\n%s", got)
	}
	for _, want := range []string{
		"The active typed decision verdict field is the carrier",
		"`block.claim_uses[]` is optional",
		`"guard_condition"`,
		`"definition_fact"`,
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("typed decision requirement missing %q:\n%s", want, got)
		}
	}
}

// TestRenderAnswerDocBlockRequirement_OmitsTypedSetsWhenEmpty
// confirms zero-value fields are NOT rendered (no empty
// "MUST include []" lines).
func TestRenderAnswerDocBlockRequirement_OmitsTypedSetsWhenEmpty(t *testing.T) {
	req := types.BlockRequirement{
		Kind:     types.BlockSummary,
		MinCount: 1, MaxCount: 1,
		Rationale: "lead-in",
	}
	var b strings.Builder
	renderAnswerDocBlockRequirement(&b, req, nil, true)
	got := b.String()
	for _, banned := range []string{
		"`block.facet_ids` MUST include",
		"`block.claim_uses[]` entry's `claim_form` MUST be one of",
		"`block.surface_role` SHOULD be",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("empty-field requirement should not render %q in:\n%s", banned, got)
		}
	}
}

// TestRenderAnswerDocFacetCoverage_VerbatimFacetIDAndOwnerBlock
// confirms each facet line carries `facet_id: "X"` verbatim AND
// the reverse-lookup "Place on block kind: ..." hint when the
// view's BlockRequirements list which block covers the facet.
func TestRenderAnswerDocFacetCoverage_VerbatimFacetIDAndOwnerBlock(t *testing.T) {
	ctx := &types.AgentContext{
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
			},
		},
	}
	got := renderAnswerDocFacetCoverage(ctx)
	// verbatim facet_id string
	if !strings.Contains(got, `facet_id: "current_code_path"`) {
		t.Errorf("missing verbatim facet_id for current_code_path:\n%s", got)
	}
	// preamble teaching that the value is what to copy into emit
	if !strings.Contains(got, "`block.facet_ids` (or `block.claim_uses[j].facet_id`)") {
		t.Errorf("missing R7 declarative preamble:\n%s", got)
	}
	// reverse-lookup hint (architecture family has BlockRequirement
	// with FacetIDs, so the reverse map is populated)
	if strings.Contains(got, "Place on block kind:") == false {
		t.Errorf("missing R7 reverse-lookup hint:\n%s", got)
	}
}

// TestRenderQuotedList_FormatStability pins the JSON-ready output
// shape (LLM-friendly format).
func TestRenderQuotedList_FormatStability(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{}, ""},
		{[]string{"a"}, `["a"]`},
		{[]string{"a", "b"}, `["a", "b"]`},
		{[]string{"  a  ", "b"}, `["a", "b"]`}, // trim whitespace
		{[]string{"", "a"}, `["a"]`},           // skip empty
	}
	for _, tc := range cases {
		if got := renderQuotedList(tc.in); got != tc.want {
			t.Errorf("renderQuotedList(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// ── R14-c9 retry-state rendering lock tests
// (post_shape_residual_audit.md, 2026-05-04) ──────────────────────

// TestRenderAnswerDocRetryState_NilOrEmpty pins the byte-identity
// invariant: fresh dispatches (no retry state on Mutable) must
// render empty so the prompt is byte-identical to pre-R14.
func TestRenderAnswerDocRetryState_NilOrEmpty(t *testing.T) {
	if got := renderAnswerDocRetryState(nil); got != "" {
		t.Errorf("nil ctx: got %q, want empty", got)
	}
	ctx := &types.AgentContext{}
	if got := renderAnswerDocRetryState(ctx); got != "" {
		t.Errorf("ctx without Mutable: got %q, want empty", got)
	}
	ctx = &types.AgentContext{Mutable: &types.MutableState{}}
	if got := renderAnswerDocRetryState(ctx); got != "" {
		t.Errorf("Mutable without RetryState: got %q, want empty", got)
	}
	mut := &types.MutableState{}
	mut.SetRetryState(&types.RetryState{Attempt: 0})
	ctx = &types.AgentContext{Mutable: mut}
	if got := renderAnswerDocRetryState(ctx); got != "" {
		t.Errorf("Attempt=0 RetryState: got %q, want empty", got)
	}
}

// TestRenderAnswerDocRetryState_FullPayload pins the 4 required
// sections + key invariants:
//   - Hard Rule referenced first
//   - Required Changes groups actionable fixes by repair phase
//   - Active Violations groups by Severity (incl Soft as telemetry)
//   - Previous Emit shows block-level claim_use + facet_ids verbatim
func TestRenderAnswerDocRetryState_FullPayload(t *testing.T) {
	rs := &types.RetryState{
		Attempt: 2,
		PrevEmitSummary: types.RetryStateSummary{
			CitationsCount: 5,
			CitationFiles:  []string{"a.go", "b.go"},
			BlockSummaries: []types.RetryBlockSummary{
				{
					ID: "lifecycle", Kind: types.BlockSection,
					SurfaceRole: types.SurfacePrincipal,
					FacetIDs:    []string{"current_code_path"},
					HasClaimUse: true,
					ClaimForm:   types.ClaimDefinitionFact,
				},
				{
					ID: "items_block", Kind: types.BlockOrderedList,
					HasItems: true, ItemCount: 3,
					ItemsWithCitation: 3,
				},
			},
			HasExactResolution: false,
		},
		ActiveViolations: []types.ScoredViolation{
			{
				Kind:      types.ViolPrincipalClaimUseMissing,
				Detail:    `principal block id="lifecycle" kind=section has no claim_use`,
				Repair:    "emit claim_use on the block",
				Severity:  types.SeverityCritical,
				Layer:     "v2_oracle",
				BlockID:   "lifecycle",
				FieldPath: `blocks[id="lifecycle"].claim_use`,
			},
			{
				Kind:      types.ViolFacetUncovered,
				Detail:    `required facet "diagram_spine" not covered`,
				Repair:    `declare facet_id="diagram_spine"`,
				Severity:  types.SeverityHigh,
				Layer:     "v2_oracle",
				FieldPath: "blocks[*].facet_ids",
			},
			{
				Kind:     types.ViolRichnessRegression,
				Detail:   `optional facet "uncertainty_boundary" not surfaced`,
				Severity: types.SeveritySoft,
				Layer:    "v2_oracle",
			},
		},
	}
	mut := &types.MutableState{}
	mut.SetRetryState(rs)
	ctx := &types.AgentContext{Mutable: mut}
	got := renderAnswerDocRetryState(ctx)

	// 1. Hard Rule
	for _, want := range []string{
		"## Hard Rule (retry attempt 2)",
		"byte-identical to your Previous Emit",
		"Do NOT regenerate from scratch",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Hard Rule missing %q in:\n%s", want, got)
		}
	}

	// 2. Required Changes — phase partitioned; Soft NOT in this section
	cIdx := strings.Index(got, "## Required Changes")
	if cIdx < 0 {
		t.Fatal("Required Changes section missing")
	}
	shapeIdx := strings.Index(got, "**Payload shape**")
	coverageIdx := strings.Index(got, "**Requested topic and coverage**")
	if shapeIdx <= 0 || coverageIdx <= 0 || shapeIdx > coverageIdx {
		t.Errorf("Payload shape fixes must precede requested-topic fixes in Required Changes")
	}
	if !strings.Contains(got, `blocks[id="lifecycle"].claim_use`) {
		t.Errorf("missing typed FieldPath in Required Changes:\n%s", got[cIdx:cIdx+800])
	}
	// Soft not in Required Changes section (between Required and Active sections).
	requiredSection := got[cIdx:strings.Index(got, "## Active Violations")]
	if strings.Contains(requiredSection, "uncertainty_boundary") {
		t.Errorf("Soft violation must not appear in Required Changes:\n%s", requiredSection)
	}

	// 3. Active Violations
	if !strings.Contains(got, "## Active Violations (typed, by severity + layer)") {
		t.Error("Active Violations section header missing")
	}
	// Soft IS shown in Active Violations with telemetry tag
	if !strings.Contains(got, "uncertainty_boundary") {
		t.Error("Soft violation missing in Active Violations")
	}
	if !strings.Contains(got, "*(telemetry only)*") {
		t.Error("Soft violation must carry telemetry-only tag")
	}

	// 4. Previous Emit
	for _, want := range []string{
		"## Previous Emit (preserve every field below byte-identical",
		`Block id="lifecycle" kind=section surface_role="principal"`,
		`facet_ids: ["current_code_path"]`,
		`claim_use: present (claim_form="definition_fact")`,
		`Block id="items_block" kind=ordered_list`,
		"items: 3 total, 3 with citation",
		"Citations: 5 entries (top files: a.go, b.go)",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("Previous Emit missing %q in:\n%s", want, got)
		}
	}
}

// TestRenderRetryRequiredChanges_GroupsByPhaseThenSeverity verifies
// phase partitioning, per-phase Critical -> High -> Medium ordering,
// and Soft exclusion.
func TestRenderRetryRequiredChanges_GroupsByPhaseThenSeverity(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{Kind: types.ViolMustInclude, Severity: types.SeverityMedium, Detail: "consistency med"},
			{Kind: types.ViolBlockCoverageMissing, Severity: types.SeverityMedium, Detail: "shape med"},
			{Kind: types.ViolPrincipalClaimUseMissing, Severity: types.SeverityCritical, Detail: "shape crit"},
			{Kind: types.ViolFacetUncovered, Severity: types.SeverityHigh, Detail: "coverage high"},
			{Kind: types.ViolRichnessGlaringGap, Severity: types.SeverityMedium, Detail: "richness med"},
			{Kind: types.ViolRichnessRegression, Severity: types.SeveritySoft, Detail: "soft"},
		},
	}
	got := renderRetryRequiredChanges(rs)
	shapeIdx := strings.Index(got, "**Payload shape**")
	consistencyIdx := strings.Index(got, "**Grounding and consistency**")
	coverageIdx := strings.Index(got, "**Requested topic and coverage**")
	richnessIdx := strings.Index(got, "**Supported enrichment**")
	if shapeIdx < 0 || consistencyIdx < 0 || coverageIdx < 0 || richnessIdx < 0 {
		t.Fatalf("missing phase sections; got:\n%s", got)
	}
	if !(shapeIdx < consistencyIdx && consistencyIdx < coverageIdx && coverageIdx < richnessIdx) {
		t.Errorf("phase ordering broken: shape=%d consistency=%d coverage=%d richness=%d", shapeIdx, consistencyIdx, coverageIdx, richnessIdx)
	}
	critIdx := strings.Index(got, "shape crit")
	medIdx := strings.Index(got, "shape med")
	if critIdx < 0 || medIdx < 0 || critIdx > medIdx {
		t.Errorf("severity ordering inside phase broken:\n%s", got)
	}
	if strings.Contains(got, "soft") {
		t.Errorf("Soft must not appear in Required Changes:\n%s", got)
	}
}

func TestRenderRetryRequiredChanges_TopicMismatchSuppressesEnrichment(t *testing.T) {
	rs := &types.RetryState{
		ActiveViolations: []types.ScoredViolation{
			{Kind: types.ViolAnswerTopicMismatch, Severity: types.SeverityHigh, Detail: "wrong subject"},
			{Kind: types.ViolAnswerSemanticUnderfilled, Severity: types.SeverityMedium, Detail: "thin explanation"},
			{Kind: types.ViolRichnessGlaringGap, Severity: types.SeverityMedium, Detail: "missing optional detail"},
			{Kind: types.ViolPrincipalClaimUseMissing, Severity: types.SeverityCritical, Detail: "missing claim use"},
		},
	}
	got := renderRetryRequiredChanges(rs)
	topicIdx := strings.Index(got, "wrong subject")
	shapeIdx := strings.Index(got, "missing claim use")
	if topicIdx < 0 {
		t.Fatalf("topic mismatch must stay actionable:\n%s", got)
	}
	if shapeIdx < 0 {
		t.Fatalf("critical structural blocker must stay actionable:\n%s", got)
	}
	if topicIdx > shapeIdx {
		t.Errorf("topic mismatch should lead the repair when present:\n%s", got)
	}
	for _, banned := range []string{"thin explanation", "missing optional detail"} {
		if strings.Contains(got, banned) {
			t.Errorf("topic-mismatch repair must not bundle enrichment %q:\n%s", banned, got)
		}
	}
}
