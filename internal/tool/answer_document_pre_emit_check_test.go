package tool

import (
	"slices"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === P1 (2026-05-10) — emit-time pre-validation chokepoint ===

// TestRunPreEmitChecks_NilSafe — nil doc / nil view return nil
// (no spurious rejections; the post-emit chain takes over).
func TestRunPreEmitChecks_NilSafe(t *testing.T) {
	if got := runPreEmitChecks(nil, nil, nil); got != nil {
		t.Errorf("nil/nil: want nil, got %v", got)
	}
	if got := runPreEmitChecks(&types.AnswerDocumentV2{}, nil, nil); got != nil {
		t.Errorf("nil view: want nil, got %v", got)
	}
	if got := runPreEmitChecks(nil, &types.AnswerSemanticView{}, nil); got != nil {
		t.Errorf("nil doc: want nil, got %v", got)
	}
}

func TestPreCheckAbsenceScopeBound_RequiresNegativeCitation(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{
			{File: "codrax.yaml", Line: 12},
		},
	}
	if got := preCheckAbsenceScopeBound(doc); len(got) != 1 {
		t.Fatalf("expected absence-scope pre-emit hint, got %+v", got)
	}
	doc.Citations = append(doc.Citations, types.Citation{
		File: "codrax.yaml", Scope: types.ScopeNegative, NegativePattern: "legacy_config_key",
	})
	if got := preCheckAbsenceScopeBound(doc); len(got) != 0 {
		t.Fatalf("negative-scope citation should satisfy absence pre-check, got %+v", got)
	}
}

func TestPreCheckAbsenceScopeBound_SourceInventoryClassUniverseRequiresInventoryProof(t *testing.T) {
	mu := types.NewMutableState("source inventory absence")
	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleThirdParty,
			Count:    1,
			Complete: true,
		}},
	})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{{
			File:            ".",
			Scope:           types.ScopeNegative,
			NegativePattern: "repo-wide no hit",
		}},
	}
	hints := preCheckAbsenceScopeBound(doc, ctx)
	if len(hints) != 1 || !strings.Contains(hints[0].Reason, "thirdparty:1") {
		t.Fatalf("source-class universe should keep source-inventory absence open, got %+v", hints)
	}

	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleThirdParty,
			Count:    1,
			Complete: true,
		}},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
		}},
	})
	if hints := preCheckAbsenceScopeBound(doc, ctx); len(hints) != 0 {
		t.Fatalf("complete empty source-inventory set should close the class-bound absence proof: %+v", hints)
	}

	mu.SetSourceInventoryObservation(types.SourceInventoryObservation{
		Active:       true,
		AdvisoryOnly: true,
		Complete:     true,
		Lens:         []string{"source_class_universe", "count"},
		SourceClasses: []types.SourceInventorySourceClassCount{{
			Role:     types.SourcePathRoleThirdParty,
			Count:    1,
			Complete: true,
		}},
		Execution: &types.SourceInventoryExecutionState{
			Budgeted:                 true,
			CandidateBudgetTruncated: true,
		},
		Sets: []types.SourceInventoryObservationSet{{
			Role:     types.AnswerCandidateRoleFunction,
			Complete: true,
		}},
	})
	hints = preCheckAbsenceScopeBound(doc, ctx)
	if len(hints) != 1 || !strings.Contains(hints[0].Reason, "candidate_budget_truncated") {
		t.Fatalf("budget-truncated zero set must not close source-inventory absence proof: %+v", hints)
	}
}

func TestPreCheckNegativeCitationBoundsRejectsUnboundedAbsenceCitation(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "runtime observation uses citation_ref=-1",
			Items: []types.AnswerBlockItem{{
				ID:          "runtime",
				CitationRef: -1,
			}},
		}},
		Citations: []types.Citation{{
			File:  "/app/src/",
			Scope: types.ScopeNegative,
			Quote: "RuntimeError: config unavailable",
		}},
	}
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil)
	if len(hints) == 0 {
		t.Fatal("expected unbounded negative citation rejection")
	}
	if !strings.Contains(hints[0].ExpectedShape, "negative_pattern") ||
		!strings.Contains(hints[0].Reason, "runtime-observation lane") {
		t.Fatalf("hint should steer external observations away from fake absence citations: %+v", hints[0])
	}
}

func TestPreEmitStructuredMemberBlockCoversFactAcrossBlocks(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{
		{
			ID:   "model_table",
			Kind: types.BlockTable,
			Text: "| 名称 | 位置 |\n|---|---|\n| Eval | eval.go:15 |\n| EvalAll | eval.go:36 |",
		},
		{
			ID:    "system_missing_rows",
			Kind:  types.BlockTable,
			Title: "系统按已验证证据补充成员：公开函数（1）",
			Items: []types.AnswerBlockItem{{
				ID:    "registered",
				Label: "RegisteredKinds",
				Text:  "RegisteredKinds 返回已注册 Kind。",
			}},
		},
	}}
	fact := types.AnswerAggregateFact{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "公开函数",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"Eval", "EvalAll", "RegisteredKinds"},
	}
	if !preEmitStructuredMemberBlockCoversFact(doc, fact) {
		t.Fatalf("member-set coverage should compose across model rows and localized supplement blocks")
	}
}

func TestPreCheckRuntimeObservationRepoContaminationRejectsRepoCitationsOnly(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{Func: "load_config"}}}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic: true,
				},
				ExternalObservationPolicy: &types.ExternalObservationPolicy{
					CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
					ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
					SourceQuotes:      []string{"只分析日志"},
					Confidence:        0.9,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/env/cache/disk_cache.go", Line: 179}},
		Blocks: []types.AnswerBlock{{
			ID:   "c",
			Kind: types.BlockCaveat,
			Text: "外部日志事实使用 artifact observation，不绑定当前仓库引用。",
		}},
	}
	hints := preCheckRuntimeObservationRepoContamination(doc, ctx)
	if len(hints) == 0 {
		t.Fatal("expected repo citation contamination hint")
	}
	if !strings.Contains(hints[0].ExpectedShape, "observation-only external runtime artifact answer") {
		t.Fatalf("unexpected hint: %+v", hints[0])
	}
}

func TestPreCheckRuntimeObservationRepoContaminationAllowsCurrentStatus(t *testing.T) {
	mut := types.NewMutableState("q")
	mut.SetLogTriage(&types.LogBundle{
		Errors: []types.LogError{{Type: "RuntimeError", Frames: []types.LogFrame{{Func: "load_config"}}}},
	})
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				LogTriage: mut.LogTriage(),
				DiagnosticProfile: types.DiagnosticIntentProfile{
					IsDiagnostic:        true,
					CurrentVersionCheck: true,
				},
				CurrentSourceExplanationProfile: &types.CurrentSourceExplanationProfile{
					IsCurrentSourceExplanationRequested: true,
					Modes:                               []types.CurrentSourceExplanationMode{types.CurrentSourceExplanationVerifyCurrentStatus},
					SourceQuotes:                        []string{"current code"},
					Confidence:                          0.95,
				},
			},
			AnswerContract: types.AnswerContract{
				CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{Required: true},
			},
		},
		EvidenceItems: []types.EvidenceItem{{
			Source: "internal/env/cache/disk_cache.go",
			Origin: types.ClaimOriginCurrentRepo,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/env/cache/disk_cache.go", Line: 179}},
		Blocks: []types.AnswerBlock{{
			ID:   "d",
			Kind: types.BlockDecision,
			Text: "not_enough_evidence: current code at `internal/env/cache/disk_cache.go:179` is only a comparison point.",
		}},
	}
	if hints := preCheckRuntimeObservationRepoContamination(doc, ctx); len(hints) != 0 {
		t.Fatalf("current-status diagnostics must allow current-code verification citations, got %+v", hints)
	}
}

func TestRunPreEmitChecksDoesNotKeywordMatchRenderedCarrierTerms(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "c",
		Kind: types.BlockCaveat,
		Text: "这些日志事实来自外部工件，citation_ref=-1。",
	}}}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: "解释这个 traceback",
	}}}
	if hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx); len(hints) != 0 {
		t.Fatalf("pre-emit must not reject by scanning rendered text / RawRequest keywords, got %+v", hints)
	}
}

func TestRunPreEmitChecksAllowsTypedToolNameRowsWithoutRawRequestScan(t *testing.T) {
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "tools",
		Kind: types.BlockTable,
		Items: []types.AnswerBlockItem{{
			ID:            "finalizer-tool",
			Label:         "FinalizerAgent",
			Text:          "`emit_answer_document`",
			CandidateRole: types.AnswerCandidateRoleToolName,
			CitationRef:   -1,
		}},
	}}}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		RawRequest: "FinalizerAgent 写结构化输出的工具名是什么？",
	}}}
	if hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx); len(hints) != 0 {
		t.Fatalf("typed tool-name rows should pass without RawRequest/text keyword gates, got %+v", hints)
	}
}

func TestRunPreEmitChecksDoesNotKeywordMatchMultiRepoAbsenceDisclosure(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Citations: []types.Citation{{
			File:            "repo-go/internal/router.go",
			Line:            42,
			Scope:           types.ScopeNegative,
			NegativePattern: "process_request",
		}},
		Blocks: []types.AnswerBlock{{
			ID:   "s",
			Kind: types.BlockSummary,
			Text: "active repos did not define `process_request`.",
		}},
	}
	ctx := &types.BusContext{PendingSubRepos: []string{"repo-tools-py"}}
	if hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx); len(hints) != 0 {
		t.Fatalf("pre-emit must not scan rendered text for inactive repo names, got %+v", hints)
	}
}

func TestPreEmitBlockCitationRoleForms_TableParticipates(t *testing.T) {
	block := types.AnswerBlock{
		ID:   "table",
		Kind: types.BlockTable,
		ClaimUses: []types.RenderedClaimUse{{
			ClaimForm: types.ClaimImportEdge,
		}},
	}
	got := preEmitBlockCitationRoleForms(block, nil)
	if len(got) != 1 || got[0] != types.ClaimImportEdge {
		t.Fatalf("table blocks should participate in typed citation-role alignment, got %+v", got)
	}
}

func TestNormalizeCitationBackedPrincipalClaimUses_AddsFormsFromCitedEvidence(t *testing.T) {
	mu := &types.MutableState{}
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID:         "ev-def",
			Source:     "internal/demo.go",
			LineStart:  10,
			AnchorKind: types.AnchorDefinition,
		},
		{
			ID:         "ev-call",
			Source:     "internal/demo.go",
			LineStart:  20,
			AnchorKind: types.AnchorCall,
		},
	})
	ctx := &types.BusContext{Mutable: mu}
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{
			Kind: types.BlockOrderedList,
			FacetIDs: []string{
				string(types.FacetEnumerationItem),
			},
			AcceptableClaimForms: []types.ClaimForm{
				types.ClaimDefinitionFact,
				types.ClaimCallEdge,
			},
			SurfaceRoleHint: types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/demo.go", Line: 10},
			{File: "internal/demo.go", Line: 20},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "facts",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{
				{ID: "a", Label: "A", CitationRef: 0},
				{ID: "b", Label: "B", CitationRef: 1},
			},
		}},
	}

	if fixed := normalizeCitationBackedPrincipalClaimUses(doc, view, newPreEmitCheckContext(ctx)); fixed != 1 {
		t.Fatalf("expected one claim_use carrier repair, got %d doc=%+v", fixed, doc.Blocks[0].ClaimUses)
	}
	got := doc.Blocks[0].ClaimUses
	if len(got) != 2 {
		t.Fatalf("expected definition and call claim forms, got %+v", got)
	}
	forms := map[types.ClaimForm]bool{}
	for _, claim := range got {
		forms[claim.ClaimForm] = true
		if claim.FacetID != string(types.FacetEnumerationItem) {
			t.Fatalf("claim should inherit singleton facet id, got %+v", claim)
		}
	}
	for _, want := range []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge} {
		if !forms[want] {
			t.Fatalf("missing claim form %s in %+v", want, got)
		}
	}
}

func TestPreCheckCallChainItemCitationRoleAlignment_SkipsChangeImpactSourcePrincipalRows(t *testing.T) {
	mu := types.NewMutableState("change impact aggregate members")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "affected source members",
		Value: "1",
		Members: []string{
			"internal/agent/analyzer.go:1935",
		},
	}})
	mu.SetInvestigationComplete("aggregate member set emitted")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "bad-edge-context",
		Kind:            types.EvidenceRelationship,
		Scope:           types.ScopeLine,
		Source:          "internal/types/analysis_ir.go",
		LineStart:       1234,
		AnchorKind:      types.AnchorCall,
		Subject:         "ShapeValue",
		Object:          "ShapeScalar",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
	}})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentEnumerate,
				AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqEnumeration)},
				Predicates:    types.SemanticPredicates{IsCategoryEnumeration: true},
				ChangeImpactProfile: &types.ChangeImpactProfile{
					IsChangeImpact:  true,
					Target:          "ShapeValue",
					RequestedOutput: types.ImpactOutputFiles,
					Scope:           types.ImpactScopeProduction,
					Confidence:      0.9,
				},
			},
		},
	}
	view := &types.AnswerSemanticView{
		Family: types.QFEnumeration,
		RequiredBlocks: []types.BlockRequirement{{
			Kind:                 types.BlockOrderedList,
			Required:             true,
			AcceptableClaimForms: []types.ClaimForm{types.ClaimCallEdge},
			SurfaceRoleHint:      types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/analyzer.go", Line: 1935}},
		Blocks: []types.AnswerBlock{{
			ID:          "files",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "analyzer",
				Label:       "internal/agent/analyzer.go",
				Text:        "rename ShapeValue -> ShapeScalar in the comment-only affected site",
				CitationRef: 0,
			}},
		}},
	}

	if hints := preCheckCallChainItemCitationRoleAlignment(doc, view, ctx); len(hints) != 0 {
		t.Fatalf("source-location principal member should not be reinterpreted as an edge role, got %+v", hints)
	}
}

func TestPreCheckAggregateScalarValueCoverage_RequiresModelAuthoredScalarFacts(t *testing.T) {
	mu := types.NewMutableState("scalar aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "retry budget parameter name",
		Value: "MaxRetriesPerStage",
	}, {
		Kind:  types.AnswerAggregateScalar,
		Label: "retry attempt counter field",
		Value: "EmitStageRetryAttempt",
	}})
	mu.SetInvestigationComplete("structured scalar facts accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "Yes. The analyze stage retries through prependEmitRetryDirective and the attempt counter is `EmitStageRetryAttempt`.",
		}},
	}

	hints := preCheckAggregateScalarValueCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("missing scalar aggregate value should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "MaxRetriesPerStage") {
		t.Fatalf("hint should name the omitted model-authored scalar, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape, "EmitStageRetryAttempt") {
		t.Fatalf("visible scalar should not be reported missing, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " The retry budget parameter is `MaxRetriesPerStage`."
	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("all scalar aggregate values are visible; got hints %+v", got)
	}
}

func TestPreCheckAggregateScalarValueCoverage_DoesNotForceNonScalarHistoryMetadata(t *testing.T) {
	mu := types.NewMutableState("最近一次合入的是什么特性？请说明特性内容。")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "最近一次合入特性",
		Value: "Tighten explorer guidance and evidence grounding",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
	}, {
		Kind:  types.AnswerAggregateScalar,
		Label: "merge commit",
		Value: "aa27be48",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("history metadata accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "最近一次合入的特性是 Tighten explorer guidance and evidence grounding，主要强化 explorer 引导和证据 grounding。",
	}}}

	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("non-scalar history metadata should not force a scalar-value repair, got %+v", got)
	}
}

func TestPreCheckAggregateScalarValueCoverage_UsesVCSOriginWhenHistoryPredicateMissing(t *testing.T) {
	mu := types.NewMutableState("最近一次合入的是什么特性？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "latest merge commit",
		Value: "aa27be48",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
		},
	}})
	mu.SetInvestigationComplete("history metadata accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "最近一次合入主要是强化 explorer 引导和证据 grounding；commit id 只是这条结论的来源元数据。",
	}}}

	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("typed VCS metadata should not force a raw scalar into narrative answers, got %+v", got)
	}
}

func TestPreCheckAggregateScalarValueCoverage_StillRequiresExactVCSScalar(t *testing.T) {
	mu := types.NewMutableState("最近一次合入的 commit id 是什么？")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "latest merge commit",
		Value: "aa27be48",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: "vcs_metadata"},
		},
	}})
	mu.SetInvestigationComplete("history scalar accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
				IsScalarAnswer:  true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "最近一次合入是 explorer grounding 相关修改。",
	}}}

	hints := preCheckAggregateScalarValueCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("exact VCS scalar should still require the value, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "aa27be48") {
		t.Fatalf("hint should preserve the requested exact commit id, got %+v", hints[0])
	}
}

func TestPreCheckAggregateScalarValueCoverage_DoesNotHardGateConflictingParallelCount(t *testing.T) {
	mu := types.NewMutableState("conflicting parallel aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateScalar,
			Label: "func 数量",
			Value: "8",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "公开函数",
			Value:   "5",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"Eval", "EvalAll", "IsRegistered", "RegisteredKinds", "SetExternalArtifactFloor"},
		},
	})
	mu.SetInvestigationComplete("structured enumeration facts accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all public symbols",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "公开函数共 5 个：Eval、EvalAll、IsRegistered、RegisteredKinds、SetExternalArtifactFloor。",
	}}}

	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("conflicting stale count should not force finalizer rewrite, got %+v", got)
	}
}

func TestPreCheckAggregateScalarValueCoverage_RequiresPrincipalCommandCount(t *testing.T) {
	mu := types.NewMutableState("principal command count handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateTotalCount,
		Label: "production matches",
		Value: "7",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Unit:  "locations",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "tool", Value: "rg"},
			{Name: "scope", Value: "production"},
		},
	}, {
		Kind:  types.AnswerAggregateTotalCount,
		Label: "candidate prefilter",
		Value: "21",
		Role:  types.AnswerAggregateRoleSupportingCoverage,
		Unit:  "candidates",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "stage", Value: "candidate_pre_filter"},
		},
	}})
	mu.SetInvestigationComplete("structured count facts accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "搜索完成，最终答案只保留了文字结论。",
		}},
	}

	hints := preCheckAggregateScalarValueCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("missing principal count should produce one advisory hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "production matches") ||
		!strings.Contains(hints[0].ExpectedShape, "value=\"7\"") ||
		!strings.Contains(hints[0].ExpectedShape, "scope=\"production\"") {
		t.Fatalf("hint should describe the omitted principal count with dimensions, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape, "candidate prefilter") ||
		strings.Contains(hints[0].ExpectedShape, "21") {
		t.Fatalf("supporting count should not be promoted to visible-value obligation, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " 其中 production 范围内共有 7 locations。"
	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("visible principal count should satisfy aggregate coverage, got %+v", got)
	}
}

func TestPreCheckAggregateScalarValueCoverage_RequiresAbsenceNegativeSearchDimensions(t *testing.T) {
	mu := types.NewMutableState("negative search handoff")
	facts, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeSearch,
		Label: "frameworks/base ressched interface search",
		Value: "0",
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "repo", Value: "frameworks/base"},
			{Name: "pattern", Value: "ResSched|IRemoteBroker|SAMR"},
			{Name: "scope", Value: "frameworks/base"},
			{Name: "searched_at", Value: "current_investigation"},
		},
	}})
	if err != nil {
		t.Fatalf("negative search aggregate should normalize: %v", err)
	}
	mu.SetInvestigationAggregateFacts(facts)
	mu.SetInvestigationComplete("zero-hit search accepted")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "没有发现两个仓库之间的直接接口交互。",
		}},
	}

	hints := preCheckAggregateScalarValueCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("missing negative-search dimensions should produce one advisory hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "negative_search") ||
		!strings.Contains(hints[0].ExpectedShape, "frameworks/base") ||
		!strings.Contains(hints[0].ExpectedShape, "ResSched|IRemoteBroker|SAMR") {
		t.Fatalf("hint should preserve the typed no-hit dimensions, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " 在 frameworks/base 范围内对 ResSched 相关接口搜索结果为 0 matches。"
	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("visible zero-hit value and dimensions should satisfy aggregate coverage, got %+v", got)
	}
}

func TestPreCheckAggregateScalarValueCoverage_MixedNegativeSearchAndCurrentSourceKeepsAbsenceBoundary(t *testing.T) {
	mu := types.NewMutableState("negative plus present source")
	facts, err := types.NormalizeAnswerAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "present current source anchors",
			Value:   "1",
			Role:    types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{"ResSchedService"},
			SupportRefs: []string{
				"ResSchedService: ressched/services/resschedservice/src/res_sched_service.cpp:42",
			},
		},
		{
			Kind:  types.AnswerAggregateNegativeSearch,
			Label: "frameworks/base ressched interface search",
			Value: "0",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "repo", Value: "frameworks/base"},
				{Name: "query", Value: "ResSched"},
				{Name: "scope", Value: "frameworks/base"},
				{Name: "searched_at", Value: "current_investigation"},
			},
		},
	})
	if err != nil {
		t.Fatalf("aggregate facts should normalize: %v", err)
	}
	mu.SetInvestigationAggregateFacts(facts)
	mu.SetInvestigationComplete("present source and bounded absence")
	mu.SetInvestigationResultKind("absence")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
			AnswerContract: types.AnswerContract{
				ExactResolution: &types.ExactResolutionContract{
					TargetKind:   types.SubjectFunctionName,
					TargetLabel:  "interface",
					Targets:      []string{"ResSched"},
					AllowAbsence: true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "在 ressched 中能看到 ResSchedService，但没有发现 frameworks/base 与 ressched 的直接接口交互。",
		}},
	}

	hints := preCheckAggregateScalarValueCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("mixed current-source/negative-search answer should still require the bounded absence detail, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "frameworks/base") ||
		!strings.Contains(hints[0].ExpectedShape, "ResSched") ||
		!strings.Contains(hints[0].ExpectedShape, "result_count") {
		t.Fatalf("hint should preserve the negative-search boundary, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " 对 frameworks/base 范围执行 ResSched 搜索，result_count=0，searched_at=current_investigation。"
	doc.Blocks[0].Text += " 当前源码锚点 1 个：ResSchedService。"
	if got := preCheckAggregateScalarValueCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("visible bounded absence should satisfy mixed-origin coverage, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_RequiresVisibleModelAuthoredMembers(t *testing.T) {
	mu := types.NewMutableState("enum aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "public enum types",
		Value:   "3",
		Members: []string{"Intent", "QuestionFamily", "Scenario @ internal/types/analysis_ir.go:673"},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				AnalyzerHints: types.AnalyzerHints{
					Kind:     string(types.ReqEnumeration),
					Entities: []string{"Intent", "QuestionFamily", "Scenario"},
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all public enum types",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "intent",
				Label: "Intent",
			}, {
				ID:    "scenario",
				Label: "Scenario",
			}},
		}},
	}

	hints := preCheckAggregateMemberSetCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("missing member_set value should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "QuestionFamily") {
		t.Fatalf("hint should name the omitted model-authored member, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape, "Scenario") {
		t.Fatalf("visible display prefix should satisfy member entry, got %+v", hints[0])
	}

	doc.Blocks[0].Items = append(doc.Blocks[0].Items, types.AnswerBlockItem{ID: "family", Label: "QuestionFamily"})
	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("all member_set members are visible; got hints %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_ScalarCountTreatsMembersAsSupportOnly(t *testing.T) {
	mu := types.NewMutableState("scalar count aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind constants",
		Value:   "3",
		Unit:    "constants",
		Members: []string{"KindSymbolPresent", "KindNoCallSites", "KindAnswerSetBounded"},
	}})
	mu.SetInvestigationComplete("structured count basis accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:        types.IntentReturnValue,
				AnswerSubject: types.AnswerSubject{Kind: types.SubjectNumeric, Confidence: 0.95},
				Predicates: types.SemanticPredicates{
					IsCountQuestion: true,
					IsScalarAnswer:  true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "count",
			Kind: types.BlockScalar,
			Text: "3 distinct Criterion Kind constants are declared.",
			Items: []types.AnswerBlockItem{{
				ID:          "v",
				Label:       "3",
				CitationRef: -1,
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("count-only scalar answer should not be forced to render support members, got %+v", got)
	}
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("matching visible count should satisfy cardinality consistency, got %+v", got)
	}

	doc.Blocks[0].Text = "4 distinct Criterion Kind constants are declared."
	doc.Blocks[0].Items[0].Label = "4"
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) == 0 {
		t.Fatal("wrong scalar count must still be checked against the support member_set cardinality")
	}
}

func TestPreCheckSourceInventoryCandidateUniverseCoverage_RequiresCoverageOrCaveat(t *testing.T) {
	ctx := sourceInventoryUniverseTestContext([]string{"alpha", "beta", "gamma"})
	ctx.AnalysisIR = &types.AnalysisIR{
		RequestModel: types.RequestModel{
			Intent:     types.IntentEnumerate,
			Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
			AnalyzerHints: types.AnalyzerHints{
				Kind: string(types.ReqEnumeration),
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all direct scopes",
			},
		},
	}
	ctx.Mutable.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "source scopes",
		Value:   "2",
		Members: []string{"alpha", "beta"},
	}})
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "alpha",
				Label: "alpha",
			}, {
				ID:    "beta",
				Label: "beta",
			}},
		}},
	}
	hints := preCheckSourceInventoryCandidateUniverseCoverage(doc, ctx)
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, "gamma") {
		t.Fatalf("missing exact-universe candidate should produce gamma hint, got %+v", hints)
	}

	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "scope",
		Kind: types.BlockCaveat,
		Text: "gamma was a direct candidate but was intentionally out of the requested principal answer boundary.",
	})
	if got := preCheckSourceInventoryCandidateUniverseCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("explicit caveat should satisfy boundary disclosure, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_PrincipalFactsDoNotDependOnRequestFamily(t *testing.T) {
	mu := types.NewMutableState("relation aggregate handoff routed as architecture")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "agents that can invoke subagents",
		Value:   "1",
		Members: []string{"explorer"},
	}})
	mu.SetInvestigationComplete("structured relation member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: false,
					IsRelationalLookup:    false,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "SubAgentRuntime validates proposals and dispatches registered subagents through the orchestration layer.",
	}}}

	hints := preCheckAggregateMemberSetCoverage(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("model-authored principal member_set must be preserved independent of analyzer family, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "explorer") {
		t.Fatalf("hint should name the omitted qualifying member, got %+v", hints[0])
	}

	doc.Blocks[0].Text = "可以调用 subagent 的是 `explorer`；SubAgentRuntime 只是执行层。"
	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("visible qualifying member should satisfy family-independent coverage, got %+v", got)
	}
}

func TestPreCheckRelationMemberSetAnswerShape_RequiresSingletonCarrierForTypedRelation(t *testing.T) {
	mu := types.NewMutableState("which workers can invoke capability Y")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "workers that can invoke capability Y",
		Value:   "1",
		Members: []string{"WorkerA"},
		Role:    types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("typed relation singleton member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup:    true,
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "WorkerA is the only qualifying member; the runtime dispatcher is support context.",
	}}}
	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("summary text still visibly preserves the member, got aggregate coverage hint %+v", got)
	}
	hints := preCheckRelationMemberSetAnswerShape(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("typed relation singleton must require a structured carrier, got %+v", hints)
	}
	for _, want := range []string{"principal relation member_set", "label=\"workers that can invoke capability Y\" members=1"} {
		if !strings.Contains(hints[0].ExpectedShape, want) {
			t.Fatalf("singleton relation hint missing %q: %+v", want, hints[0])
		}
	}

	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "members",
		Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{
			ID:    "worker-a",
			Label: "WorkerA",
			Text:  "qualifies by the grounded relation evidence",
		}},
	})
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("structured singleton member carrier should satisfy relation shape, got %+v", got)
	}
}

func TestPreCheckRelationMemberSetAnswerShape_AcceptsPrincipalSectionItems(t *testing.T) {
	mu := types.NewMutableState("which packages declare sources")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "package declarations",
		Value:   "2",
		Members: []string{"demo.app", "demo.cart"},
		Role:    types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("package declaration set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "packages",
		Kind:        types.BlockSection,
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		Title:       "package declarations",
		Items: []types.AnswerBlockItem{
			{ID: "app", Label: "demo.app"},
			{ID: "cart", Label: "demo.cart"},
		},
	}}}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("principal section items should satisfy relation member-set shape, got %+v", got)
	}

	doc.Blocks[0].SurfaceRole = ""
	doc.Blocks[0].FacetIDs = nil
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) == 0 {
		t.Fatal("unannotated section items should not satisfy relation member-set shape")
	}
}

func TestPreCheckRelationMemberSetAnswerShape_SkipsMechanismOnlyPlainMemberSet(t *testing.T) {
	mu := types.NewMutableState("explain how workers invoke capability Y")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "mechanism support members",
		Value:   "1",
		Members: []string{"WorkerA"},
		Role:    types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("mechanism support member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Predicates: types.SemanticPredicates{
					IsRelationalLookup: true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "WorkerA participates in the mechanism, but this turn did not carry a typed set-valued relation contract.",
	}}}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("mechanism-only plain member_set should not trigger relation carrier hard gate, got %+v", got)
	}
}

func TestPreCheckRelationMemberSetAnswerShape_RelationSurfaceSingletonRequiresCarrier(t *testing.T) {
	mu := types.NewMutableState("explain relation member surface")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "caller relation",
		Value:   "1",
		Members: []string{"Worker -> Helper"},
		Role:    types.AnswerAggregateRolePrincipalAnswer,
	}})
	mu.SetInvestigationComplete("relation surface singleton member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{Intent: types.IntentExplain},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "Worker reaches Helper through the run path.",
	}}}
	hints := preCheckRelationMemberSetAnswerShape(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("single explicit relation surface should require structured carrier, got %+v", hints)
	}

	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "relation",
		Kind: types.BlockBulletList,
		Items: []types.AnswerBlockItem{{
			ID:    "worker-helper",
			Label: "Worker -> Helper",
		}},
	})
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("structured relation-surface item should satisfy carrier, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsCodeIdentityProjection(t *testing.T) {
	mu := types.NewMutableState("subagent call-chain aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "subagent调用链节点",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"ExplorerAgent (LLM调用propose_sub_agents)",
			"ProposeSubAgents工具.Execute (验证提案结构)",
		},
	}})
	mu.SetInvestigationComplete("principal call-chain member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentTrace,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsRoleLocateLookup: true,
					IsScalarAnswer:     true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "hops",
		Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{
			ID:    "explorer",
			Label: "ExplorerAgent",
			Text:  "LLM调用propose_sub_agents。",
		}, {
			ID:    "tool",
			Label: "ProposeSubAgents.Execute",
			Text:  "验证提案结构。",
		}},
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("code-identity-equivalent member displays should satisfy coverage, got %+v", got)
	}
}

func TestRunPreEmitChecks_ExplicitPrincipalMemberSetHardForScalarCompression(t *testing.T) {
	mu := types.NewMutableState("subagent call-chain aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "subagent调用链节点",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"ExplorerAgent (LLM调用propose_sub_agents)",
			"SubAgentRuntime.Run (编排层入口)",
		},
	}})
	mu.SetInvestigationComplete("principal call-chain member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentTrace,
				Scenario: types.ScenarioArchitectureExplain,
				Predicates: types.SemanticPredicates{
					IsRoleLocateLookup: true,
					IsScalarAnswer:     true,
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "scalar",
		Kind: types.BlockScalar,
		Text: "Orchestrator 是唯一能够调用 SubAgent 的组件。",
	}}}

	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	if len(hints) == 0 {
		t.Fatal("explicit principal member_set must not be accepted as a soft advisory when finalization collapses it to a scalar")
	}
	memberHint := false
	for _, hint := range hints {
		if strings.Contains(hint.ExpectedShape, "ExplorerAgent") &&
			strings.Contains(hint.ExpectedShape, "SubAgentRuntime.Run") {
			memberHint = true
			break
		}
	}
	if !memberHint {
		t.Fatalf("hard hint should preserve omitted principal members, got %+v", hints)
	}
	hard, advisory := splitPreEmitHintsByGate(hints)
	if len(hard) == 0 {
		t.Fatalf("principal member_set coverage drift must force same-turn repair, advisory=%+v hints=%+v", advisory, hints)
	}
}

func TestNormalizeAggregateMemberSetCarriers_DoesNotAuthorComparisonMembers(t *testing.T) {
	mu := types.NewMutableState("comparison aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "codrax 核心防幻觉子系统",
		Value: "2",
		Members: []string{
			"codrax/internal/types/evidence.go: EvidenceKind（11种分类）",
			"codrax/internal/types/violation_registry.go: ViolKindRegistry（单点注册表）",
		},
	}, {
		Kind:  types.AnswerAggregateMemberSet,
		Label: "opencode 防幻觉机制",
		Value: "1",
		Members: []string{
			"opencode/packages/opencode/src/agent/prompt/scout.txt: 文本层面引用要求",
		},
	}})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "ev-evidence-kind",
		Kind:            types.EvidenceDirect,
		Source:          "codrax/internal/types/evidence.go",
		LineStart:       11,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "EvidenceKind",
		GroundingStatus: types.GroundingGrounded,
	}, {
		ID:              "ev-registry",
		Kind:            types.EvidenceDirect,
		Source:          "codrax/internal/types/violation_registry.go",
		LineStart:       1,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "ViolKindRegistry",
		GroundingStatus: types.GroundingGrounded,
	}, {
		ID:              "ev-scout",
		Kind:            types.EvidenceMechanism,
		Source:          "opencode/packages/opencode/src/agent/prompt/scout.txt",
		LineStart:       1,
		AnchorKind:      types.AnchorTextReference,
		AnchorSymbol:    "scout.txt",
		SurfaceTerms:    []string{"文本层面引用要求"},
		GroundingStatus: types.GroundingGrounded,
	}})
	mu.SetInvestigationComplete("structured comparison member sets accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				Buckets: []types.QuestionBucket{
					{Label: "codrax", Index: 1},
					{Label: "opencode", Index: 2},
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "codrax 和 opencode 的防幻觉设计差异很大。",
	}}}
	if hints := preCheckAggregateMemberSetCoverage(doc, ctx); len(hints) == 0 {
		t.Fatal("test setup should miss every aggregate member")
	}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("fixed=%d, want 0; normalizer must not author visible answer members", fixed)
	}
	if len(doc.Blocks) != 1 || len(doc.Blocks[0].Items) != 0 {
		t.Fatalf("normalizer must leave model-authored visible content unchanged: %+v", doc.Blocks)
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("normalizer must not append citations for system-authored members: %+v", doc.Citations)
	}
	if hints := preCheckAggregateMemberSetCoverage(doc, ctx); len(hints) == 0 {
		t.Fatal("coverage checker should remain responsible for missing aggregate members")
	}
}

func TestRunPreEmitChecks_AggregateMemberSetCoverageAdvisoryForNarrative(t *testing.T) {
	mu := types.NewMutableState("narrative aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "supporting mechanisms",
		Value: "2",
		Members: []string{
			"codrax/internal/types/evidence.go: EvidenceKind（11种分类）",
			"codrax/internal/types/violation_registry.go: ViolKindRegistry（单点注册表）",
		},
	}})
	mu.SetInvestigationComplete("member set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Buckets: []types.QuestionBucket{
				{Label: "codrax", Index: 1},
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "这是机制说明，不是穷举清单。",
	}}}
	if hints := preCheckAggregateMemberSetCoverage(doc, ctx); len(hints) == 0 {
		t.Fatal("test setup should have aggregate coverage hints")
	}
	if hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx); len(hints) != 0 {
		t.Fatalf("narrative aggregate coverage should be advisory at emit-time, got %+v", hints)
	}
}

func TestNormalizeAggregateMemberSetCarriers_MaterializesExhaustiveEnumerationRows(t *testing.T) {
	mu := types.NewMutableState("exhaustive aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind constants",
		Value:   "3",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindA", "KindB", "KindC"},
		SupportRefs: []string{
			"KindA: internal/analysis/criterion/grammar.go:29",
			"KindB: internal/analysis/criterion/grammar.go:30",
			"KindC: internal/analysis/criterion/grammar.go:31",
		},
	}})
	mu.SetInvestigationComplete("member set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "完整成员名称",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "KindA、KindB、KindC。",
	}}}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 3 {
		t.Fatalf("fixed=%d, want 3", fixed)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("expected appended carrier block, got %+v", doc.Blocks)
	}
	block := doc.Blocks[1]
	if block.Kind != types.BlockOrderedList || block.SurfaceRole != types.SurfacePrincipal || len(block.Items) != 3 {
		t.Fatalf("unexpected materialized block: %+v", block)
	}
	if block.Items[0].Label != "KindA" || block.Items[0].CitationRef < 0 || len(doc.Citations) != 3 {
		t.Fatalf("items should preserve member labels and cite support_refs: block=%+v citations=%+v", block, doc.Citations)
	}
	if !strings.Contains(block.Title, "系统按已验证证据补充成员") ||
		!strings.Contains(block.Text, "结构化调查清单") {
		t.Fatalf("zh system supplement should be clearly marked and localized: %+v", block)
	}
}

func TestNormalizeAggregateMemberSetCarriers_PreservesRelationDimensionLabel(t *testing.T) {
	mu := types.NewMutableState("list classes and package declarations")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "package declarations",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"demo.app", "demo.cart"},
		SupportRefs: []string{
			"demo.app @ main.cj:6",
			"demo.cart @ cart/Cart.cj:4",
		},
	}})
	mu.SetInvestigationComplete("relation dimension handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
				IsRelationalLookup:    true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "classes",
		Kind:        types.BlockTable,
		Title:       "public classes",
		SurfaceRole: types.SurfacePrincipal,
		FacetIDs:    []string{string(types.FacetEnumerationItem)},
		Text: strings.Join([]string{
			"| symbol | file | package |",
			"|---|---|---|",
			"| App | main.cj:11 | demo.app |",
			"| Cart | cart/Cart.cj:14 | demo.cart |",
		}, "\n"),
	}}}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 2 {
		t.Fatalf("fixed=%d, want 2", fixed)
	}
	if len(doc.Blocks) != 2 {
		t.Fatalf("expected one relation label carrier, got %+v", doc.Blocks)
	}
	block := doc.Blocks[1]
	if block.Title != "package declarations（2）" {
		t.Fatalf("relation label carrier should preserve typed label, got %+v", block)
	}
	if strings.Contains(block.Title+block.Text, "系统按已验证证据补充成员") {
		t.Fatalf("label carrier should not use noisy system supplement copy: %+v", block)
	}
	if len(block.Items) != 2 || block.Items[0].Label != "demo.app" || block.Items[1].Label != "demo.cart" {
		t.Fatalf("label carrier items did not preserve relation members: %+v", block.Items)
	}
}

func TestNormalizeAggregateMemberSetCarriers_LocalizesEnglishSystemSupplement(t *testing.T) {
	mu := types.NewMutableState("exhaustive aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind constants",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindA", "KindB"},
		SupportRefs: []string{
			"KindA: internal/analysis/criterion/grammar.go:29",
			"KindB: internal/analysis/criterion/grammar.go:30",
		},
	}})
	mu.SetInvestigationComplete("member set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "en",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all member names",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "KindA and KindB are present.",
	}}}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 2 {
		t.Fatalf("fixed=%d, want 2", fixed)
	}
	block := doc.Blocks[1]
	if !strings.Contains(block.Title, "System-verified member supplement") ||
		!strings.Contains(block.Text, "accepted structured investigation checklist") {
		t.Fatalf("English system supplement should be clearly marked and localized: %+v", block)
	}
	if strings.Contains(block.Title+block.Text, "系统") || strings.Contains(block.Title+block.Text, "结构化调查") {
		t.Fatalf("English system supplement should not leak zh copy: %+v", block)
	}
}

func TestNormalizePrincipalSupportSurfaceTermSupplement_MaterializesMissingEvidenceLabels(t *testing.T) {
	mu := types.NewMutableState("surface-term handoff")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Producer:     EmitEvidenceProducer,
		Scope:        types.ScopeLine,
		Kind:         types.EvidenceRegistration,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Snippet:      "@Entry\n@Component\nstruct Index {",
		SurfaceTerms: []string{"Index.ets", "@Entry", "@Component"},
	}})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all members",
			},
		}},
	}
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:         "Index @Entry component",
				Location:     "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
				ClaimForm:    types.ClaimDefinitionFact,
				Subject:      "Index",
				AnchorSymbol: "Index",
				SurfaceTerms: []string{"Index.ets", "@Entry", "@Component"},
				Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
				LineStart:    7,
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "entry-list",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Text:        "| 组件名 | 文件路径 | 行号 |\n|---|---|---|\n| Index | internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets | 7 |",
		}},
	}

	if fixed := normalizePrincipalSupportSurfaceTermSupplement(doc, plan, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1; doc=%+v", fixed, doc.Blocks)
	}
	visible := preEmitVisibleAnswerSurface(doc)
	for _, want := range []string{"@Component", "Index.ets", "系统按已验证证据补充可见标签"} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible supplement missing %q:\n%s", want, visible)
		}
	}
	if len(doc.Citations) != 1 ||
		doc.Citations[0].File != "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets" ||
		doc.Citations[0].Line != 7 {
		t.Fatalf("supplement should cite the typed evidence line, got %+v", doc.Citations)
	}
	if fixed := normalizePrincipalSupportSurfaceTermSupplement(doc, plan, ctx); fixed != 0 {
		t.Fatalf("second normalization must be idempotent, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
}

func TestNormalizePrincipalSupportSurfaceTermSupplement_SuppressesWhenSourceInventoryOnlyRequestsNameLocation(t *testing.T) {
	mu := types.NewMutableState("source-inventory name location only")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-index",
		Producer:     EmitEvidenceProducer,
		Scope:        types.ScopeLine,
		Kind:         types.EvidenceRegistration,
		Subject:      "Index",
		AnchorSymbol: "Index",
		Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
		LineStart:    7,
		Snippet:      "@Entry\n@Component\nstruct Index {",
		SurfaceTerms: []string{"@Entry", "@Component"},
	}})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleFunction,
				},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
			},
		}},
	}
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:         "Index @Entry component",
				Location:     "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets:7",
				ClaimForm:    types.ClaimDefinitionFact,
				Subject:      "Index",
				AnchorSymbol: "Index",
				SurfaceTerms: []string{"@Entry", "@Component"},
				Source:       "internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets",
				LineStart:    7,
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "entry-list",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Text:        "| 组件名 | 文件路径 | 行号 |\n|---|---|---|\n| Index | internal/thirdparty/tree-sitter-arkts/corpus/sources/01_entry_component_minimal.ets | 7 |",
		}},
	}

	if fixed := normalizePrincipalSupportSurfaceTermSupplement(doc, plan, ctx); fixed != 0 {
		t.Fatalf("name/location-only source inventory should not add surface-term supplement, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	visible := preEmitVisibleAnswerSurface(doc)
	if strings.Contains(visible, "系统按已验证证据补充可见标签") || strings.Contains(visible, "@Component") {
		t.Fatalf("surface-term supplement should stay internal when only name/location were requested:\n%s", visible)
	}
}

func TestNormalizePrincipalSupportSurfaceTermSupplement_SkipsCoveredSourceInventoryRows(t *testing.T) {
	mu := types.NewMutableState("source-inventory primary row already covered")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-cart-class",
		Producer:     EmitEvidenceProducer,
		Scope:        types.ScopeLine,
		Kind:         types.EvidenceDirect,
		Subject:      "Cart",
		AnchorSymbol: "Cart",
		Source:       "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
		LineStart:    14,
		Snippet:      "public class Cart {",
		SurfaceTerms: []string{"public class"},
	}})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles: []types.AnswerCandidateRole{
					types.AnswerCandidateRoleType,
				},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldValues,
					types.SourceInventoryFieldLocation,
				},
			},
		}},
	}
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:         "Cart public class",
				Location:     "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
				ClaimForm:    types.ClaimDefinitionFact,
				Subject:      "Cart",
				AnchorSymbol: "Cart",
				SurfaceTerms: []string{"Cart", "public class"},
				Source:       "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				LineStart:    14,
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj", Line: 14}},
		Blocks: []types.AnswerBlock{{
			ID:          "cangjie-types",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Text:        "| 成员 | 包 | 位置 |\n|---|---|---|\n| Cart | demo.cart | eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14 |",
			Items: []types.AnswerBlockItem{{
				ID:          "cart",
				Label:       "Cart",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizePrincipalSupportSurfaceTermSupplement(doc, plan, ctx); fixed != 0 {
		t.Fatalf("covered source-inventory primary row should not get a duplicate supplement, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	visible := preEmitVisibleAnswerSurface(doc)
	if strings.Contains(visible, "系统按已验证证据补充可见标签") || strings.Contains(visible, "public class") {
		t.Fatalf("source-inventory supplement should not duplicate an already covered primary row:\n%s", visible)
	}
}

func TestNormalizePrincipalSupportSurfaceTermSupplement_UsesStableDisplayLocation(t *testing.T) {
	mu := types.NewMutableState("surface-term stable display location")
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:           "ev-cart-class",
		Producer:     EmitEvidenceProducer,
		Scope:        types.ScopeLine,
		Kind:         types.EvidenceDirect,
		Subject:      "Cart",
		AnchorSymbol: "Cart",
		Source:       "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
		LineStart:    14,
		Snippet:      "public class Cart {",
		SurfaceTerms: []string{"public class"},
	}})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
		}},
	}
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:                "Cart public class",
				Location:            "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14",
				EquivalentLocations: []string{"eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:30"},
				ClaimForm:           types.ClaimDefinitionFact,
				Subject:             "Cart",
				AnchorSymbol:        "Cart",
				SurfaceTerms:        []string{"Cart", "public class"},
				Source:              "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj",
				LineStart:           14,
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "cangjie-types",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Text:        "| 成员 | 包 | 位置 |\n|---|---|---|\n| Cart | demo.cart | eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14 |",
		}},
	}

	if fixed := normalizePrincipalSupportSurfaceTermSupplement(doc, plan, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1; doc=%+v", fixed, doc.Blocks)
	}
	visible := preEmitVisibleAnswerSurface(doc)
	if !strings.Contains(visible, "eval/fixtures/testdata/cangjie_minimal/cart/Cart.cj:14") {
		t.Fatalf("supplement should render stable support citation location:\n%s", visible)
	}
	if strings.Contains(visible, "one of") || strings.Contains(visible, "Cart.cj:30") {
		t.Fatalf("supplement must not render repair-style equivalent-anchor text:\n%s", visible)
	}
}

func TestNormalizeAggregateMemberSetCarriers_DoesNotOverrideSingletonModelCategoryBlock(t *testing.T) {
	mu := types.NewMutableState("singleton aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "codrax 默认注册的 SubAgent 完整成员名列表",
		Value:   "1",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{`SubExplorer (Name() 返回 "explorer")`},
		SupportRefs: []string{
			"SubExplorer: internal/agent/sub_explorer.go:20",
		},
	}})
	mu.SetInvestigationComplete("member set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "完整成员名",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/agent/sub_explorer.go", Line: 31}},
		Blocks: []types.AnswerBlock{{
			ID:          "enum",
			Kind:        types.BlockOrderedList,
			Title:       "codrax 默认注册的 SubAgent 完整成员名列表",
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "explorer",
				Label:       "explorer",
				Text:        `SubExplorer.Name() 返回 "explorer"。`,
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("fixed=%d, want 0; singleton category block from finalizer should not be overridden by system supplement", fixed)
	}
	if len(doc.Blocks) != 1 {
		t.Fatalf("system supplement should not be appended: %+v", doc.Blocks)
	}
	if doc.Blocks[0].Items[0].Label != "explorer" {
		t.Fatalf("model-authored principal item should remain authoritative: %+v", doc.Blocks[0].Items)
	}
}

func TestNormalizeAggregateMemberSetCarriers_UsesEvidenceSummaryText(t *testing.T) {
	mu := types.NewMutableState("rich aggregate handoff")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "kind_a",
			Kind:            types.EvidenceDirect,
			Subject:         "KindA",
			Summary:         "KindA 表示按布尔条件求值的 criterion 类型。",
			Source:          "internal/analysis/criterion/grammar.go",
			LineStart:       29,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "KindA",
			GroundingStatus: types.GroundingGrounded,
			Scope:           types.ScopeLine,
		},
		{
			ID:              "kind_b",
			Kind:            types.EvidenceDirect,
			Subject:         "KindB",
			Summary:         "KindB 表示按外部 artifact 下限求值的 criterion 类型。",
			Source:          "internal/analysis/criterion/grammar.go",
			LineStart:       30,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "KindB",
			GroundingStatus: types.GroundingGrounded,
			Scope:           types.ScopeLine,
		},
	})
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "Kind constants",
		Value:   "2",
		Role:    types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{"KindA", "KindB"},
		SupportRefs: []string{
			"KindA: internal/analysis/criterion/grammar.go:29",
			"KindB: internal/analysis/criterion/grammar.go:30",
		},
	}})
	mu.SetInvestigationComplete("member set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all public symbols",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "KindA、KindB。",
	}}}

	if fixed := normalizeAggregateMemberSetCarriers(doc, ctx); fixed != 2 {
		t.Fatalf("fixed=%d, want 2", fixed)
	}
	if len(doc.Blocks) != 2 || len(doc.Blocks[1].Items) != 2 {
		t.Fatalf("expected materialized rows, got %+v", doc.Blocks)
	}
	if !strings.Contains(doc.Blocks[1].Items[0].Text, "布尔条件求值") ||
		!strings.Contains(doc.Blocks[1].Items[1].Text, "artifact 下限") {
		t.Fatalf("materialized rows should carry evidence summaries, got %+v", doc.Blocks[1].Items)
	}
}

func TestRunPreEmitChecks_AggregateMemberSetCoverageHardForExhaustiveEnumeration(t *testing.T) {
	mu := types.NewMutableState("exhaustive aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "all handlers",
		Value: "2",
		Members: []string{
			"pkg/a.go: HandleA",
			"pkg/b.go: HandleB",
		},
	}})
	mu.SetInvestigationComplete("member set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentEnumerate,
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			CompletenessObligation: &types.CompletenessObligation{
				Required:    true,
				SourceQuote: "all handlers",
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "HandleA is present.",
	}}}
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	if len(hints) == 0 {
		t.Fatal("exhaustive enumeration member_set coverage should remain hard at emit-time")
	}
	if !strings.Contains(hints[0].ExpectedShape, "HandleB") {
		t.Fatalf("hint should name the missing member, got %+v", hints)
	}
}

func TestRunPreEmitChecks_AggregateMemberSetCoverageHardForHistoryList(t *testing.T) {
	mu := types.NewMutableState("recent commit impact rollup")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "最近10次提交",
		Value: "2",
		Members: []string{
			"abc1234 (docs: plan contract)",
			"def5678 (agent: preserve VCS summaries)",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "vcs_metadata"}},
	}})
	mu.SetInvestigationComplete("VCS member_set handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "最近提交主要围绕 prompt ledger 和渲染稳定性展开。",
	}}}
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	if len(hints) == 0 {
		t.Fatal("principal VCS history member_set must not be silently collapsed to a summary")
	}
	if !strings.Contains(hints[0].ExpectedShape, "abc1234") ||
		!strings.Contains(hints[0].ExpectedShape, "def5678") {
		t.Fatalf("history-list hint should name omitted commit members, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " abc1234 (docs: plan contract); def5678 (agent: preserve VCS summaries)."
	if got := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx); len(got) != 0 {
		t.Fatalf("visible commit members should satisfy history member_set coverage, got %+v", got)
	}
}

func TestRunPreEmitChecks_AggregateMemberSetCoverageHardForOriginSpecificPrincipalList(t *testing.T) {
	mu := types.NewMutableState("incident list rollup")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "open incidents",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"INC-17 (owner runtime)",
			"INC-42 (owner storage)",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "mcp_resource"}},
	}})
	mu.SetInvestigationComplete("MCP incident list handoff ready")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "当前有两个打开的事故。",
	}}}
	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	if len(hints) == 0 {
		t.Fatal("explicit principal MCP member_set must not be silently collapsed to a summary")
	}
	if !strings.Contains(hints[0].ExpectedShape, "INC-17") ||
		!strings.Contains(hints[0].ExpectedShape, "INC-42") {
		t.Fatalf("origin-specific hint should name omitted members, got %+v", hints[0])
	}

	doc.Blocks[0].Text += " INC-17 (owner runtime); INC-42 (owner storage)."
	if got := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx); len(got) != 0 {
		t.Fatalf("visible origin-specific members should satisfy coverage, got %+v", got)
	}
}

func TestDropUnsupportedDecoratedMemberSets_KeepsVCSCommitMembersWithoutSupportRefs(t *testing.T) {
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "recent commits",
		Value: "2",
		Members: []string{
			"abc1234 (docs: plan contract)",
			"def5678 (agent: preserve VCS summaries)",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "vcs_metadata"}},
	}}
	ctx := &types.BusContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:     types.IntentExplain,
		Predicates: types.SemanticPredicates{IsHistoryLookup: true},
	}}}
	got, notes := dropUnsupportedDecoratedMemberSets(ctx, facts, "optional aggregate handoff", true)
	if len(notes) != 0 {
		t.Fatalf("VCS commit members should rely on VCS provenance instead of current-source support_refs, notes=%v", notes)
	}
	if len(got) != 1 || len(got[0].Members) != 2 {
		t.Fatalf("VCS commit member_set was dropped: %+v", got)
	}
}

func TestDropUnsupportedDecoratedMemberSets_KeepsOriginSpecificMembersWithoutSupportRefs(t *testing.T) {
	for _, origin := range []types.AnswerEvidenceOrigin{
		types.AnswerEvidenceOriginCommandMeasurement,
		types.AnswerEvidenceOriginCrossRepoIndex,
		types.AnswerEvidenceOriginExternalDocument,
		types.AnswerEvidenceOriginWebPage,
		types.AnswerEvidenceOriginMCPResource,
		types.AnswerEvidenceOriginConnectorResource,
	} {
		facts := []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateMemberSet,
			Label: "external principal rows",
			Value: "1",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Members: []string{
				"ServiceA (observed externally)",
			},
			Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: string(origin)}},
		}}
		got, notes := dropUnsupportedDecoratedMemberSets(&types.BusContext{}, facts, "optional aggregate handoff", true)
		if len(notes) != 0 {
			t.Fatalf("origin %q should rely on origin-specific provenance instead of current-source support_refs, notes=%v", origin, notes)
		}
		if len(got) != 1 || len(got[0].Members) != 1 {
			t.Fatalf("origin %q member_set was dropped: %+v", origin, got)
		}
	}
}

func TestDropUnsupportedDecoratedMemberSets_CurrentSourceStillRequiresSupportRefs(t *testing.T) {
	facts := []types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "code rows",
		Value: "1",
		Role:  types.AnswerAggregateRoleSupportingCoverage,
		Members: []string{
			"ServiceA (current source role)",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "origin", Value: "current_source"}},
	}}
	got, notes := dropUnsupportedDecoratedMemberSets(&types.BusContext{}, facts, "optional aggregate handoff", true)
	if len(notes) == 0 {
		t.Fatal("current-source decorated member without support_refs should still be dropped from optional handoff")
	}
	if len(got) != 0 {
		t.Fatalf("current-source unsupported member_set should be dropped, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_GroupedCountNarrativeIsSupportOnly(t *testing.T) {
	mu := types.NewMutableState("architecture grouped count handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateGroupedCount,
		Label:   "codrax 四层架构成员",
		Value:   "4",
		Unit:    "layers",
		Members: []string{"Ground (ground/ground.go)", "Gate (analysis/gate/gate.go)", "Reviewer (orchestrator/)", "Contract (orchestrator/contract_check.go)"},
	}})
	mu.SetInvestigationComplete("structured grouped count accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Complexity: types.ComplexityComplex,
				Predicates: types.SemanticPredicates{IsCrossComponent: true},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "codrax 的四层架构包括 Ground 层、Gate 层、Reviewer 层和 Contract 层，重点是比较职责分层而不是输出逐字枚举。",
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("architecture grouped_count members should not force verbatim visible strings, got %+v", got)
	}
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("architecture grouped_count support should not bind narrative count claims, got %+v", got)
	}
}

func TestPreCheckAggregateCardinalityConsistency_GroupedCountNarrativeBindsVisibleMemberCounts(t *testing.T) {
	mu := types.NewMutableState("architecture grouped count visible count drift")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateGroupedCount,
		Label:   "opencode 防护机制成员",
		Value:   "5",
		Unit:    "mechanisms",
		Members: []string{"evaluate", "assertExternalDirectory", "Permission.ask", "ToolRegistry", "ApplyPatchTool"},
	}})
	mu.SetInvestigationComplete("structured grouped count accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentExplain,
				Scenario:   types.ScenarioArchitectureExplain,
				Complexity: types.ComplexityComplex,
				Predicates: types.SemanticPredicates{IsCrossComponent: true},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "opencode",
		Kind: types.BlockSection,
		Text: "opencode 的三层机制包括 evaluate、ToolRegistry 和 ApplyPatchTool。",
	}}}

	hints := preCheckAggregateCardinalityConsistency(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("visible grouped_count member count drift should be deterministic, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "expected_count=5") || !strings.Contains(hints[0].ExpectedShape, "visible_count=3") {
		t.Fatalf("hint should name grouped_count drift, got %+v", hints[0])
	}
}

func TestPreCheckAggregateMemberSetCoverage_PathSetsAreCoverageForArchitecture(t *testing.T) {
	mu := types.NewMutableState("architecture coverage aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "files inspected",
		Value:   "3",
		Members: []string{"internal/agent/explorer.go", "internal/agent/agent.go", "internal/tool/propose_sub_agents.go"},
	}})
	mu.SetInvestigationComplete("structured coverage set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:      types.IntentExplain,
				Scenario:    types.ScenarioArchitectureExplain,
				Complexity:  types.ComplexityComplex,
				Predicates:  types.SemanticPredicates{IsCrossComponent: true},
				DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "explorer 的工作流分成预扫描、深读、证据归纳和最终交付四步。",
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("path-only coverage member_set must not force visible principal rows for architecture answers, got %+v", got)
	}
	doc.Blocks[0].Text = "explorer 的工作流有 4 个阶段。"
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("path-only coverage member_set must not bind unrelated explanation counts, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_TotalCountPathMembersAreCoverageForArchitecture(t *testing.T) {
	mu := types.NewMutableState("architecture count coverage aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateTotalCount,
		Label:   "core files",
		Value:   "3",
		Unit:    "files",
		Members: []string{"internal/agent/explorer.go", "internal/agent/sub_explorer.go", "internal/agent/agent.go"},
	}})
	mu.SetInvestigationComplete("structured count coverage accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:      types.IntentExplain,
				Scenario:    types.ScenarioArchitectureExplain,
				Complexity:  types.ComplexityComplex,
				Predicates:  types.SemanticPredicates{IsCrossComponent: true},
				DiagramHint: &types.DiagramHint{Kind: types.DiagramArchitecture},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "explorer 的工作流由基础 ReAct 循环、探索评估器和子代理运行时协同完成。",
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("path-only total_count support members must not force visible principal rows for architecture answers, got %+v", got)
	}
	doc.Blocks[0].Text = "explorer 的工作流可以概括为 4 个协作层次。"
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("path-only total_count support members must not bind unrelated explanation counts, got %+v", got)
	}
}

func TestPreCheckAggregateCardinalityConsistency_RejectsScopedCountMismatch(t *testing.T) {
	members := []string{
		"AnalyzerAgent", "ExplorerAgent", "ExtractorAgent", "FinalizerAgent",
		"PipelineStage", "AgentContext", "StageOutput", "BaseAgent",
		"SubAgentRuntime", "SubAgentRegistry",
	}
	mu := types.NewMutableState("aggregate cardinality handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "typed handoff participants",
		Value:   "10",
		Members: members,
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	items := make([]types.AnswerBlockItem, 0, len(members))
	for _, member := range members {
		items = append(items, types.AnswerBlockItem{Label: member})
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "typed handoff participants 一共有 8 个，下面列全。",
	}, {
		ID:    "members",
		Kind:  types.BlockOrderedList,
		Items: items,
	}}}

	hints := preCheckAggregateCardinalityConsistency(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("summary count must match member_set cardinality, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "expected_count=10") ||
		!strings.Contains(hints[0].ExpectedShape, "visible_count=8") {
		t.Fatalf("hint should report scoped count mismatch, got %+v", hints[0])
	}
	if hard, advisory := splitPreEmitHintsByGate(tagPreEmitHints(types.ViolCardinalityShort, hints)); len(hard) != 1 || len(advisory) != 0 {
		t.Fatalf("principal member_set visible count drift must force same-turn repair, hard=%+v advisory=%+v", hard, advisory)
	}

	doc.Blocks[0].Text = "typed handoff participants 一共有 10 个；其中 `AnalyzerAgent` 的关键证据在 internal/agent/analyzer.go:1649。"
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("correct count plus source line number should pass, got %+v", got)
	}
}

func TestPreCheckAggregateCardinalityConsistency_IgnoresSystemMissingMemberSupplementCounts(t *testing.T) {
	mu := types.NewMutableState("diff plus current source")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "commit c9d1fe22 涉及文件",
		Value: "4",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"internal/agent/explorer.go",
			"internal/agent/explorer_test.go",
			"internal/agent/answer_intent_contract_prompt_test.go",
			"docs/design/local_model_eval_compat_20260523.md",
		},
		Dimensions: []types.AnswerAggregateDimension{{Name: "evidence_origin", Value: "vcs_metadata"}},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:    "supplement",
		Kind:  types.BlockTable,
		Title: "系统按已验证证据补充缺失成员：commit c9d1fe22 涉及文件（3）",
		Columns: []string{
			"文件",
		},
		Items: []types.AnswerBlockItem{
			{Label: "internal/agent/explorer_test.go"},
			{Label: "internal/agent/answer_intent_contract_prompt_test.go"},
			{Label: "docs/design/local_model_eval_compat_20260523.md"},
		},
	}}}

	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("system missing-member supplement is partial by design and must not trip aggregate count drift, got %+v", got)
	}
}

func TestPreCheckAggregateCardinalityConsistency_MultipleSetsRequireExplicitLabelBinding(t *testing.T) {
	mu := types.NewMutableState("multi member-set cardinality handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "导出类型列表",
		Value:   "3",
		Members: []string{"Kind", "Env", "Result"},
	}, {
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "导出函数列表",
		Value:   "5",
		Members: []string{"Eval", "EvalAll", "SetExternalArtifactFloor", "IsRegistered", "RegisteredKinds"},
	}, {
		Kind:  types.AnswerAggregateMemberSet,
		Label: "Kind 常量完整列表",
		Value: "25",
		Members: []string{
			"KindSymbolPresent", "KindNoCallSites", "KindAnswerSetBounded",
			"KindAnswerSetUnbounded", "KindMultipleResolutionChains",
			"KindUserClauseUnresolved", "KindUntrustedReachesSink",
			"KindInvariantBroken", "KindNoRelevantEvidence", "KindSignalPresent",
			"KindHasEnoughFacts", "KindAllHypothesesDecided",
			"KindContractSatisfied", "KindBudgetExhausted", "KindEvidenceCount",
			"KindCitationCountGE", "KindContainsSymbol", "KindRegexMatch",
			"KindCounterfactualBranchesDecided", "KindRelationAbsent",
			"KindPlanReady", "KindPatchApplies", "KindTestsPass",
			"KindNoRegression", "KindExternalArtifactDecoded",
		},
	}})
	mu.SetInvestigationComplete("structured member sets accepted")
	ctx := &types.BusContext{Mutable: mu}

	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "caveat",
		Kind: types.BlockCaveat,
		Text: "搜索范围覆盖 grammar.go 和 eval.go；Eval 的定义在 eval.go，Kind 常量完整列表有 25 个。",
	}}}
	if got := preCheckAggregateCardinalityConsistency(doc, ctx); len(got) != 0 {
		t.Fatalf("multi-set count binding must ignore bare member mentions in shared caveats, got %+v", got)
	}

	doc.Blocks[0].Text = "导出函数列表 有 25 个。"
	hints := preCheckAggregateCardinalityConsistency(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("explicit set label with wrong count must still reject, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, `label="导出函数列表"`) ||
		!strings.Contains(hints[0].ExpectedShape, "expected_count=5") ||
		!strings.Contains(hints[0].ExpectedShape, "visible_count=25") {
		t.Fatalf("hint should be scoped to the explicit set label, got %+v", hints[0])
	}
}

func TestPreCheckRelationMemberSetAnswerShape_RequiresStructuredRowsForMultiMemberRelations(t *testing.T) {
	mu := types.NewMutableState("relation aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "subpackage entries",
		Value: "2",
		Members: []string{
			"aggregator → Aggregate",
			"compiler → Compile",
		},
	}})
	mu.SetInvestigationComplete("structured relation member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "summary",
		Kind: types.BlockSummary,
		Text: "subpackage entries 是 aggregator → Aggregate 和 compiler → Compile；机制上由 analyzer 编译后分发。",
	}}}

	hints := preCheckRelationMemberSetAnswerShape(doc, ctx)
	if len(hints) != 1 {
		t.Fatalf("multi-member relation set should require direct rows, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "members=2") {
		t.Fatalf("hint should describe the relation member_set size, got %+v", hints[0])
	}

	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "entries",
		Kind: types.BlockTable,
		Items: []types.AnswerBlockItem{{
			Label: "aggregator",
			Text:  "入口函数是 `Aggregate`。",
		}, {
			Label: "compiler",
			Text:  "入口函数是 `Compile`。",
		}},
	})
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("table rows with split relation parts should satisfy relation answer shape, got %+v", got)
	}
}

func TestPreCheckRelationMemberSetAnswerShape_AcceptsMarkdownTableTextRows(t *testing.T) {
	mu := types.NewMutableState("decorated file member set handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "东湖前后台感知机制涉及的核心文件",
		Value: "2",
		Members: []string{
			"ActivityManagerService.java (前后台状态通知/mForegroundPackages系统级聚合)",
			"ProcessList.java (进程生命周期通知/topApp前台临时提示)",
		},
		SupportRefs: []string{
			"ActivityManagerService.java (前后台状态通知/mForegroundPackages系统级聚合) @ java/com/android/server/am/ActivityManagerService.java:1618",
			"ProcessList.java (进程生命周期通知/topApp前台临时提示) @ java/com/android/server/am/ProcessList.java:3180",
		},
	}})
	mu.SetInvestigationComplete("decorated file table accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "files_table",
		Kind: types.BlockTable,
		Text: "| 文件 | 感知机制类型 |\n|---|---|\n| ActivityManagerService.java | 前后台状态通知/mForegroundPackages系统级聚合 |\n| ProcessList.java | 进程生命周期通知/topApp前台临时提示 |",
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("markdown table text with split decorated members should satisfy member_set coverage, got %+v", got)
	}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("markdown table text rows should satisfy relation member_set shape, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsRelationDisplayVariants(t *testing.T) {
	mu := types.NewMutableState("entry function aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "subpackage directory and entry function",
		Value:   "2",
		Members: []string{"aggregator/Aggregate", "compiler: Compile"},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "agg",
				Label: "aggregator → Aggregate",
			}, {
				ID:    "comp",
				Label: "compiler/Compile",
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("equivalent relation displays should satisfy member_set visibility, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsSplitRelationInSameItem(t *testing.T) {
	mu := types.NewMutableState("entry function aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "subpackage directory and entry function",
		Value:   "2",
		Members: []string{"aggregator → Aggregate", "compiler/Compile"},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "agg",
				Label: "aggregator",
				Text:  "入口函数是 `Aggregate`。",
			}, {
				ID:    "comp",
				Label: "compiler",
				Text:  "入口函数是 `Compile`。",
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("split label/text relation members should satisfy member_set visibility, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsCompactSourceSupportRefs(t *testing.T) {
	mu := types.NewMutableState("LoopController implementer aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "LoopController implementers",
		Value: "3",
		Members: []string{
			"analyzerEvaluator@internal/agent/analyzer.go:887",
			"explorerEvaluator@internal/agent/explorer.go:8496",
			"Ability@entry/src/main/ets/pages/Index.ets:20",
		},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all implementers",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/agent/analyzer.go", Line: 887},
			{File: "internal/agent/explorer.go", Line: 8496},
			{File: "entry/src/main/ets/pages/Index.ets", Line: 20},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "items",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				Label:       "analyzerEvaluator",
				Text:        "实现了 LoopController。",
				CitationRef: 0,
			}, {
				Label:       "explorerEvaluator",
				Text:        "实现了 LoopController。",
				CitationRef: 1,
			}, {
				Label:       "Ability",
				Text:        "ArkTS 侧的同形实现条目。",
				CitationRef: 2,
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("label rows with matching citations should satisfy compact support-ref member_set coverage, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsSignatureSupportRefSplitRows(t *testing.T) {
	mu := types.NewMutableState("signature relation aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "2",
		Members: []string{
			"aggregator → New(cfg Config) *Aggregator @ internal/analysis/aggregator/aggregator.go:112",
			"amplifier → Amplify(rm types.RequestModel) (types.RequestModel, []Observation) @ internal/analysis/amplifier/amplifier.go:100",
		},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages and entry functions",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/analysis/aggregator/aggregator.go", Line: 112},
			{File: "internal/analysis/amplifier/amplifier.go", Line: 100},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "entries",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "aggregator",
				Label:       "aggregator",
				Text:        "New(cfg Config) *Aggregator",
				CitationRef: 0,
			}, {
				ID:          "amplify",
				Label:       "Amplify",
				Text:        "amplifier: Amplify(rm types.RequestModel) (types.RequestModel, []Observation)",
				CitationRef: 1,
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("signature support-ref split rows should satisfy member_set visibility, got %+v", got)
	}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("signature relation rows should satisfy relation answer shape, got %+v", got)
	}
	if !preEmitCitationSupportsAggregateItem(ctx, "Amplify", doc.Blocks[0].Items[1].Text, doc.Citations[1]) {
		t.Fatal("right-label plus left text should align to the signature support-ref citation")
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsCompositeRelationSplitRows(t *testing.T) {
	mu := types.NewMutableState("composite relation aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "internal/analysis subpackages and entry functions",
		Value: "2",
		Members: []string{
			"perftriage → MergePerfBundles + CorroborateStallFiles",
			"logtriage → ResolveJavaFile 和 ResolveFrameFile",
		},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages and entry functions",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "entries",
		Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{
			ID:    "perf-merge",
			Label: "perftriage",
			Text:  "入口函数：MergePerfBundles",
		}, {
			ID:    "perf-corroborate",
			Label: "perftriage",
			Text:  "入口函数：CorroborateStallFiles",
		}, {
			ID:    "log-java",
			Label: "logtriage",
			Text:  "入口函数：ResolveJavaFile",
		}, {
			ID:    "log-frame",
			Label: "logtriage",
			Text:  "入口函数：ResolveFrameFile",
		}},
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("same-left split rows should satisfy composite relation member visibility, got %+v", got)
	}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("same-left split rows should satisfy relation answer shape, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_CompositeRelationRequiresSameLeftAxis(t *testing.T) {
	mu := types.NewMutableState("composite relation aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "internal/analysis subpackages and entry functions",
		Value:   "2",
		Members: []string{"perftriage → MergePerfBundles + CorroborateStallFiles", "stopcond → ShouldStop"},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "entries",
		Kind: types.BlockOrderedList,
		Items: []types.AnswerBlockItem{{
			ID:    "perf-merge",
			Label: "perftriage",
			Text:  "入口函数：MergePerfBundles",
		}, {
			ID:    "wrong-left",
			Label: "logtriage",
			Text:  "入口函数：CorroborateStallFiles",
		}, {
			ID:    "stop",
			Label: "stopcond",
			Text:  "入口函数：ShouldStop",
		}},
	}}}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) == 0 {
		t.Fatal("composite relation target under a different left axis must not satisfy member visibility")
	}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) == 0 {
		t.Fatal("composite relation target under a different left axis must not satisfy relation answer shape")
	}
}

func TestPreCheckAggregateMemberSetCoverage_IgnoresUnsupportedSameLeftAxisAlternate(t *testing.T) {
	mu := types.NewMutableState("same left-axis alternate aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "supported package entry functions",
		Value:   "2",
		Members: []string{"perftriage → Corroborate", "stopcond → Evaluate"},
		SupportRefs: []string{
			"internal/analysis/perftriage/corroborate.go:29",
			"internal/analysis/stopcond/stopcond.go:8",
		},
	}, {
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "stale typed graph package entries",
		Value:   "2",
		Members: []string{"perftriage.CorroborateStallFiles", "stopcond.ShouldStop"},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/analysis/perftriage/corroborate.go", Line: 29},
			{File: "internal/analysis/stopcond/stopcond.go", Line: 8},
		},
		Blocks: []types.AnswerBlock{{
			ID:   "entries",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "perftriage",
				Label:       "perftriage",
				Text:        "入口函数 Corroborate。",
				CitationRef: 0,
			}, {
				ID:          "stopcond",
				Label:       "stopcond",
				Text:        "入口函数 Evaluate。",
				CitationRef: 1,
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("unsupported same-left-axis alternate should not be a second principal member_set, got %+v", got)
	}
	if got := preCheckPrincipalSupportMemberCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("support plan should use the supported relation set as the principal slate, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_RelationSlateSubsumesLeftAxis(t *testing.T) {
	mu := types.NewMutableState("entry relation aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "subpackage directories",
			Value:   "2",
			Members: []string{"aggregator", "compiler"},
		},
		{
			Kind:    types.AnswerAggregateMemberSet,
			Label:   "subpackage entries",
			Value:   "2",
			Members: []string{"aggregator → Aggregate", "compiler → Compile"},
		},
	})
	mu.SetInvestigationComplete("structured member sets accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true, IsRelationalLookup: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages and their entry functions",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "agg",
				Label: "aggregator",
				Text:  "入口函数是 `Aggregate`。",
			}, {
				ID:    "comp",
				Label: "compiler",
				Text:  "入口函数是 `Compile`。",
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("left-axis coverage set should not require duplicate principal rows, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsDecoratedSplitRelation(t *testing.T) {
	mu := types.NewMutableState("entry function aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "sub-packages and their entry-point functions",
		Value: "2",
		Members: []string{
			"declarative → New (Classifier) @ internal/analysis/declarative/classifier.go:138",
			"priority → Score(priority) @ internal/analysis/priority/score.go:34",
		},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "decl",
				Label: "declarative",
				Text:  "入口函数是 `New(Classifier)`。",
			}, {
				ID:    "priority",
				Label: "priority",
				Text:  "入口函数是 `Score(priority)`。",
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("decorated split relation members should satisfy member_set visibility, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsSourceLineDecoratedSplitRelation(t *testing.T) {
	mu := types.NewMutableState("entry function aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "sub-packages and their entry-point functions",
		Value: "2",
		Members: []string{
			"aggregator → Aggregate (line 132)",
			"prescan → ClassifyToken (行 29)",
		},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:     types.IntentEnumerate,
				Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages and entry functions",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "agg",
				Label: "aggregator",
				Text:  "入口函数是 `Aggregate`。",
			}, {
				ID:    "prescan",
				Label: "prescan",
				Text:  "入口函数是 `ClassifyToken`。",
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("source-line support decorators should not be required in visible member identity, got %+v", got)
	}
}

func TestPreCheckAggregateMemberSetCoverage_AcceptsDecoratorListSplitRelation(t *testing.T) {
	mu := types.NewMutableState("entry function aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "sub-packages and their entry-point functions",
		Value: "2",
		Members: []string{
			"hint → Composer (New + Compose)",
			"patcher → Engine (New + Submit/Apply)",
		},
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentEnumerate,
				Predicates: types.SemanticPredicates{
					IsCategoryEnumeration: true,
					IsRelationalLookup:    true,
				},
				CompletenessObligation: &types.CompletenessObligation{
					Required:    true,
					SourceQuote: "all subpackages and entry functions",
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "list",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:    "hint",
				Label: "Composer (New + Compose)",
				Text:  "hint 子包的入口类型。",
			}, {
				ID:    "patcher",
				Label: "Engine (New + Submit/Apply)",
				Text:  "patcher 子包的入口类型。",
			}},
		}},
	}

	if got := preCheckAggregateMemberSetCoverage(doc, ctx); len(got) != 0 {
		t.Fatalf("decorator-list split relation members should satisfy member_set visibility, got %+v", got)
	}
	if got := preCheckRelationMemberSetAnswerShape(doc, ctx); len(got) != 0 {
		t.Fatalf("decorator-list split relation rows should satisfy relation answer shape, got %+v", got)
	}
}

func TestPreEmitAggregateMemberSupportRefMember_CitesLabelAtLocation(t *testing.T) {
	mu := types.NewMutableState("implementer aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "LoopController implementers",
		Value:   "1",
		Members: []string{"explorerEvaluator (internal/agent/explorer.go:30)"},
	}})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "explorer-definition",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/explorer.go",
		LineStart:       12,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "explorerEvaluator",
		Subject:         "explorerEvaluator",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	item := types.AnswerBlockItem{
		ID:          "explorer",
		Label:       "explorerEvaluator",
		Text:        "implements LoopController.",
		CitationRef: 0,
	}
	citation := types.Citation{File: "internal/agent/explorer.go", Line: 30}

	if !preEmitCitationSupportsAggregateItem(ctx, item.Label, item.Text, citation) {
		t.Fatal("label plus matching citation should satisfy support-ref aggregate member")
	}
	if !preEmitCitationSupportsAggregateItem(ctx, item.Label, item.Text, types.Citation{File: "internal/agent/explorer.go", Line: 12}) {
		t.Fatal("typed definition citation for the same aggregate member should also satisfy support-ref aggregate member")
	}
	got := preEmitCandidateCitationLocationsForAggregateItem(ctx, item.Label, item.Text, 4)
	if len(got) != 2 || got[0] != "internal/agent/explorer.go:30" || got[1] != "internal/agent/explorer.go:12" {
		t.Fatalf("support-ref aggregate member should suggest support and definition anchors, got %v", got)
	}
}

func TestPreEmitAggregateMemberGenericSupportRef_CitesMemberLocation(t *testing.T) {
	mu := types.NewMutableState("default subagent aggregate handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "default SubAgent names",
		Value:       "1",
		Members:     []string{"explorer"},
		SupportRefs: []string{"Member @ internal/agent/sub_explorer.go:31"},
	}})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "subexplorer-name",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/sub_explorer.go",
		LineStart:       31,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "Name",
		Snippet:         "func (s *SubExplorer) Name() string {\n\treturn \"explorer\"\n}",
		Producer:        "explorer.emit_evidence",
		GroundingStatus: types.GroundingGrounded,
		GroundingTier:   types.TierLineText,
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	item := types.AnswerBlockItem{
		ID:          "explorer",
		Label:       "explorer",
		Text:        "is the default registered SubAgent name.",
		CitationRef: 0,
	}

	if !preEmitCitationSupportsAggregateItem(ctx, item.Label, item.Text, types.Citation{File: "internal/agent/sub_explorer.go", Line: 31}) {
		t.Fatal("generic Member support_ref should let the final answer cite the member's support location")
	}
	got := preEmitCandidateCitationLocationsForAggregateItem(ctx, item.Label, item.Text, 4)
	if len(got) == 0 || got[0] != "internal/agent/sub_explorer.go:31" {
		t.Fatalf("generic support-ref aggregate member should suggest the citable support anchor, got %v", got)
	}
}

func TestPreEmitAggregateMemberGenericSupportRef_RejectsUncorroboratedPositionalCitation(t *testing.T) {
	mu := types.NewMutableState("subagent call chain handoff")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "SubAgent call-chain nodes",
		Value: "2",
		Members: []string{
			"Explorer Agent (LLM 调用 propose_sub_agents 工具)",
			"SubAgentRuntime.Run (执行 SubAgent 并合并结果)",
		},
		SupportRefs: []string{
			"Member @ internal/agent/subagent.go:12",
			"Member @ internal/agent/subagent_runtime.go:220",
		},
	}})
	mu.AppendEvidence([]types.EvidenceItem{{
		ID:              "subagent-interface",
		Kind:            types.EvidenceDirect,
		Scope:           types.ScopeLine,
		Source:          "internal/agent/subagent.go",
		LineStart:       12,
		AnchorKind:      types.AnchorDefinition,
		AnchorSymbol:    "SubAgent",
		Subject:         "SubAgent",
		Snippet:         "type SubAgent interface {",
		GroundingStatus: types.GroundingGrounded,
	}})
	mu.SetInvestigationComplete("structured member set accepted")
	ctx := &types.BusContext{Mutable: mu}
	label := "Explorer Agent (LLM 调用 propose_sub_agents 工具)"
	text := "Explorer Agent 通过 buildToolSchemas 注入的 propose_sub_agents 工具发起请求。"
	citation := types.Citation{File: "internal/agent/subagent.go", Line: 12}

	if preEmitCitationSupportsAggregateItem(ctx, label, text, citation) {
		t.Fatal("positional generic support_ref must not let a natural-language hop borrow an unrelated SubAgent interface citation")
	}
	if got := preEmitCandidateCitationLocationsForAggregateItem(ctx, label, text, 4); len(got) != 0 {
		t.Fatalf("uncorroborated positional support_ref should not be suggested as a candidate, got %v", got)
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{citation},
		Blocks: []types.AnswerBlock{{
			ID:   "chain",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "h1",
				Label:       label,
				Text:        text,
				CitationRef: 0,
			}},
		}},
	}
	if hints := preCheckItemCitationAlignment(doc, nil, ctx); len(hints) == 0 {
		t.Fatal("misaligned positional support_ref citation should produce a citation-alignment hint")
	}
}

func TestPreCheckCitationPoolIntegrity_CatchesMissingAndOutOfRangeRefs(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "files",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{
				{ID: "a", CitationRef: 0},
				{ID: "b", CitationRef: 2},
			},
		}},
	}
	hints := preCheckCitationPoolIntegrity(doc)
	if len(hints) != 1 {
		t.Fatalf("missing citation pool should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].Field, "citations") {
		t.Fatalf("hint should target citations[], got %+v", hints[0])
	}

	doc.Citations = []types.Citation{
		{File: "a.go", Line: 1},
		{File: "b.go", Line: 2},
	}
	hints = preCheckCitationPoolIntegrity(doc)
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, "at least 3") {
		t.Fatalf("out-of-range citation_ref should ask for enough citation entries, got %+v", hints)
	}

	doc.Citations = append(doc.Citations, types.Citation{File: "c.go", Line: 3})
	if hints := preCheckCitationPoolIntegrity(doc); len(hints) != 0 {
		t.Fatalf("complete citation pool should pass, got %+v", hints)
	}
}

func TestRunPreEmitChecks_CitationPoolShortCircuitsSemanticMemberHints(t *testing.T) {
	mut := types.NewMutableState("citation carrier failure")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:    types.AnswerAggregateMemberSet,
		Label:   "mechanisms",
		Value:   "1",
		Members: []string{"PreEmitCheck"},
	}})
	mut.SetInvestigationComplete("accepted aggregate member set")
	ctx := &types.BusContext{Mutable: mut}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "mechanisms",
			Kind: types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{
				ID:          "other",
				Label:       "OtherMechanism",
				Text:        "not the aggregate member",
				CitationRef: 2,
			}},
		}},
	}

	hints := runPreEmitChecks(doc, &types.AnswerSemanticView{}, nil, ctx)
	if len(hints) != 1 {
		t.Fatalf("citation-carrier failure should short-circuit downstream semantic hints, got %+v", hints)
	}
	if !strings.Contains(hints[0].Field, "citations") {
		t.Fatalf("first and only hint should repair citation carrier, got %+v", hints[0])
	}
	if strings.Contains(hints[0].ExpectedShape+hints[0].Reason, "PreEmitCheck") {
		t.Fatalf("carrier short-circuit should not mix member_set diagnostics into citation repair, got %+v", hints[0])
	}
}

func TestPreCheckSummaryLeadBlock_RequiresSummaryFirstWhenViewRequiresIt(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "list", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1", Label: "A"}}},
			{ID: "summary", Kind: types.BlockSummary, Text: "lead"},
		},
	}
	hints := preCheckSummaryLeadBlock(doc, view)
	if len(hints) != 1 {
		t.Fatalf("summary after detail blocks should produce one hint, got %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "first") {
		t.Fatalf("hint should ask for summary first, got %+v", hints[0])
	}

	doc.Blocks[0], doc.Blocks[1] = doc.Blocks[1], doc.Blocks[0]
	if hints := preCheckSummaryLeadBlock(doc, view); len(hints) != 0 {
		t.Fatalf("summary-first document should pass, got %+v", hints)
	}
}

// TestPreCheckRequiredBlocks_MinCount — required block kind under-emitted.
func TestPreCheckRequiredBlocks_MinCount(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1, Rationale: "enumeration"},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := preCheckRequiredBlocks(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "ordered_list") {
		t.Errorf("hint field should name the block kind; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "at least 1") {
		t.Errorf("hint should state the required minimum; got %q", hints[0].ExpectedShape)
	}
}

func TestPreEmitBlockCoverageMissingIsHardAtToolBoundary(t *testing.T) {
	hints := []emitFixHint{{
		Field:         "blocks[].kind=diagram",
		ExpectedShape: "emit at least 1 block(s) of kind=diagram",
		Reason:        "explicit diagram request",
		Kind:          types.ViolBlockCoverageMissing,
	}}
	hard, advisory := splitPreEmitHintsByGate(hints)
	if len(hard) != 1 || len(advisory) != 0 {
		t.Fatalf("missing required block must be hard pre-emit, hard=%+v advisory=%+v", hard, advisory)
	}
}

// TestPreCheckRequiredBlocks_MaxCount — required block kind over-emitted.
func TestPreCheckRequiredBlocks_MaxCount(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1, Rationale: "lead"},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "s2", Kind: types.BlockSummary, Text: "y"},
		},
	}
	hints := preCheckRequiredBlocks(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].ExpectedShape, "at most 1") {
		t.Errorf("hint should state the maximum cap; got %q", hints[0].ExpectedShape)
	}
}

// TestPreCheckRequiredBlocks_HappyPath — exact match returns no hints.
func TestPreCheckRequiredBlocks_HappyPath(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockSummary, Required: true, MinCount: 1, MaxCount: 1},
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "l1", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "i1"}}},
		},
	}
	if hints := preCheckRequiredBlocks(doc, view); len(hints) != 0 {
		t.Errorf("happy path should produce no hints; got %v", hints)
	}
}

func TestPreCheckRequiredBlocks_AlternativeKindSatisfiesMin(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:             types.BlockOrderedList,
				AlternativeKinds: []types.AnswerBlockKind{types.BlockTable, types.BlockBulletList},
				Required:         true,
				MinCount:         1,
				Rationale:        "enumeration",
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:      "members",
				Kind:    types.BlockTable,
				Columns: []string{"file", "role"},
				Text:    "| file | role |\n|---|---|\n| a.go:1 | member |",
			},
		},
	}
	if hints := preCheckRequiredBlocks(doc, view); len(hints) != 0 {
		t.Fatalf("table should satisfy flexible enumeration carrier; got %v", hints)
	}
}

// TestPreCheckPrincipalClaimUse_Missing — principal block missing claim_uses.
func TestPreCheckPrincipalClaimUse_Missing(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "l1",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items:       []types.AnswerBlockItem{{ID: "i1"}},
				// no ClaimUses → fix hint expected
			},
		},
	}
	hints := preCheckPrincipalClaimUse(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "claim_uses") {
		t.Errorf("hint field should name claim_uses; got %q", hints[0].Field)
	}
	if !strings.Contains(hints[0].ExpectedShape, "definition_fact") {
		t.Errorf("hint should list the acceptable forms; got %q", hints[0].ExpectedShape)
	}
}

func TestPreCheckPrincipalClaimUse_AlternativeKindInheritsClaimContract(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				AlternativeKinds:     []types.AnswerBlockKind{types.BlockTable, types.BlockBulletList},
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "members",
				Kind:        types.BlockTable,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "| member |\n|---|\n| A |",
				Items:       []types.AnswerBlockItem{{ID: "i1"}},
			},
		},
	}
	hints := preCheckPrincipalClaimUse(doc, view)
	if len(hints) != 1 {
		t.Fatalf("alternative table carrier should inherit principal claim_use contract; got %v", hints)
	}
}

func TestPreCheckRequiredCandidateRoles_UsesTypedItemRoles(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredCandidateRoles: []types.AnswerCandidateRole{
			types.AnswerCandidateRoleBudgetCap,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "scalar",
			Kind:        types.BlockScalar,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "`EmitStageRetryAttempt` is nearby but not the requested cap.",
			Items: []types.AnswerBlockItem{{
				ID:            "attempt",
				Label:         "EmitStageRetryAttempt",
				CandidateRole: types.AnswerCandidateRoleAttemptCounter,
			}},
		}},
	}
	hints := preCheckRequiredCandidateRoles(doc, view)
	if len(hints) != 1 {
		t.Fatalf("missing budget_cap role should be rejected: %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "budget_cap") {
		t.Fatalf("hint should name missing typed role, got %+v", hints[0])
	}
	doc.Blocks[0].Items = append(doc.Blocks[0].Items, types.AnswerBlockItem{
		ID:            "cap",
		Label:         "MaxRetriesPerStage",
		CandidateRole: types.AnswerCandidateRoleBudgetCap,
	})
	if hints := preCheckRequiredCandidateRoles(doc, view); len(hints) != 0 {
		t.Fatalf("typed budget_cap item should satisfy role contract: %+v", hints)
	}
}

func TestPreCheckErrorGranularityVerdict_UsesTypedDecisionField(t *testing.T) {
	view := &types.AnswerSemanticView{
		ErrorGranularityProfile: &types.ErrorGranularityProfile{
			IsGranularityQuestion: true,
			RequestedVerdictOptions: []types.ErrorGranularityVerdict{
				types.ErrorGranularityPerItemRejection,
				types.ErrorGranularityWholeBatch,
			},
			Confidence: 0.9,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "d",
			Kind:        types.BlockDecision,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "The prose says item-level rejection, but the typed verdict is missing.",
		}},
	}
	hints := preCheckErrorGranularityVerdict(doc, view)
	if len(hints) != 1 {
		t.Fatalf("missing error_granularity_verdict should be rejected: %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "per_item_rejection") ||
		!strings.Contains(hints[0].Field, "error_granularity_verdict") {
		t.Fatalf("hint should name typed verdict field and enum values, got %+v", hints[0])
	}
	doc.Blocks[0].ErrorGranularityVerdict = types.ErrorGranularityPerItemRejection
	if hints := preCheckErrorGranularityVerdict(doc, view); len(hints) != 0 {
		t.Fatalf("typed verdict should satisfy error granularity contract: %+v", hints)
	}
	doc.Blocks[0].ErrorGranularityVerdict = types.ErrorGranularityPartialSuccess
	hints = preCheckErrorGranularityVerdict(doc, view)
	if len(hints) != 1 || !strings.Contains(hints[0].ExpectedShape, "per_item_rejection") {
		t.Fatalf("option mismatch should be rejected with requested options: %+v", hints)
	}
}

func TestPreCheckCurrentStatusVerdict_UsesTypedDecisionField(t *testing.T) {
	view := &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{
			Required: true,
			AllowedVerdicts: []types.CurrentStatusVerdict{
				types.CurrentStatusStillPresent,
				types.CurrentStatusFixed,
				types.CurrentStatusNotEnoughEvidence,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "d",
			Kind:        types.BlockDecision,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "fixed: prose alone should not satisfy the typed verdict contract.",
		}},
	}
	hints := preCheckCurrentStatusVerdict(doc, view)
	if len(hints) != 1 {
		t.Fatalf("missing current_status_verdict should be rejected: %+v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, "not_enough_evidence") ||
		!strings.Contains(hints[0].Field, "current_status_verdict") {
		t.Fatalf("hint should name typed verdict field and enum values, got %+v", hints[0])
	}
	doc.Blocks[0].Text = "看起来已经修复，但这里的自然语言不参与硬判断。"
	doc.Blocks[0].CurrentStatusVerdict = types.CurrentStatusFixed
	if hints := preCheckCurrentStatusVerdict(doc, view); len(hints) != 0 {
		t.Fatalf("typed verdict should satisfy current-status contract: %+v", hints)
	}
}

func TestPreCheckInactiveTypedDecisionVerdicts_RejectsWrongLaneVerdict(t *testing.T) {
	view := &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{
			Required: true,
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:                      "d",
			Kind:                    types.BlockDecision,
			SurfaceRole:             types.SurfacePrincipal,
			Text:                    "Current-status rationale.",
			CurrentStatusVerdict:    types.CurrentStatusStillPresent,
			ErrorGranularityVerdict: types.ErrorGranularityNotEnoughEvidence,
		}},
	}
	hints := preCheckInactiveTypedDecisionVerdicts(doc, view)
	if len(hints) != 1 {
		t.Fatalf("inactive error_granularity_verdict should be rejected: %+v", hints)
	}
	if !strings.Contains(hints[0].Field, "error_granularity_verdict") ||
		!strings.Contains(hints[0].ExpectedShape, "omit") {
		t.Fatalf("hint should remove inactive typed verdict field, got %+v", hints[0])
	}
	doc.Blocks[0].ErrorGranularityVerdict = ""
	if hints := preCheckInactiveTypedDecisionVerdicts(doc, view); len(hints) != 0 {
		t.Fatalf("current-status verdict should be allowed when its contract is active: %+v", hints)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_TypedDecisionCarrier(t *testing.T) {
	view := &types.AnswerSemanticView{
		CurrentStatusDiagnostic: &types.CurrentStatusDiagnosticContract{
			Required: true,
		},
		RequiredBlocks: []types.BlockRequirement{{
			Kind:     types.BlockDecision,
			MinCount: 1,
			MaxCount: 1,
			Required: true,
			AcceptableClaimForms: []types.ClaimForm{
				types.ClaimDefinitionFact,
				types.ClaimCallEdge,
				types.ClaimGuardCondition,
				types.ClaimAbsenceFact,
			},
			SurfaceRoleHint: types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:                      "verdict1",
			Kind:                    types.BlockDecision,
			SurfaceRole:             types.SurfacePrincipal,
			Text:                    "typed verdict carries the decision; prose is only rationale",
			CurrentStatusVerdict:    types.CurrentStatusStillPresent,
			ErrorGranularityVerdict: types.ErrorGranularityNotEnoughEvidence,
		}},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 1 {
		t.Fatalf("expected one inactive typed-lane field to be normalized, got %d", fixed)
	}
	if got := doc.Blocks[0].ErrorGranularityVerdict; got != "" {
		t.Fatalf("inactive error_granularity_verdict should be cleared, got %q", got)
	}
	if len(doc.Blocks[0].ClaimUses) != 0 {
		t.Fatalf("normalizer should not guess decision claim_uses, got %+v", doc.Blocks[0].ClaimUses)
	}
	if hints := runPreEmitChecks(doc, view, nil); len(hints) != 0 {
		t.Fatalf("active typed decision carrier should avoid JSON repair hints, got %+v", hints)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_CoalescesExtraSummaryBlocks(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{
			Kind:     types.BlockSummary,
			MinCount: 1,
			MaxCount: 1,
			Required: true,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "a.go", Line: 1}, {File: "b.go", Line: 2}},
		Blocks: []types.AnswerBlock{
			{
				ID:          "s1",
				Kind:        types.BlockSummary,
				Text:        "first lead",
				FacetIDs:    []string{"current_code_path"},
				SurfaceRole: types.SurfacePrincipal,
				Items:       []types.AnswerBlockItem{{ID: "a", CitationRef: 0}},
			},
			{
				ID:       "s2",
				Kind:     types.BlockSummary,
				Text:     "second lead",
				FacetIDs: []string{"uncertainty_boundary"},
				Items:    []types.AnswerBlockItem{{ID: "b", CitationRef: 1}},
			},
		},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 1 {
		t.Fatalf("expected one excess summary block to be coalesced, got %d", fixed)
	}
	if len(doc.Blocks) != 1 || doc.Blocks[0].ID != "s1" {
		t.Fatalf("summary coalescing should keep first summary identity, got %+v", doc.Blocks)
	}
	if !strings.Contains(doc.Blocks[0].Text, "first lead") ||
		!strings.Contains(doc.Blocks[0].Text, "second lead") {
		t.Fatalf("summary text should preserve both blocks, got %q", doc.Blocks[0].Text)
	}
	if len(doc.Blocks[0].Items) != 2 || doc.Blocks[0].Items[1].CitationRef != 1 {
		t.Fatalf("summary items/citations should be preserved, got %+v", doc.Blocks[0].Items)
	}
	if hints := preCheckRequiredBlocks(doc, view); len(hints) != 0 {
		t.Fatalf("coalesced summary should satisfy max-count gate, got %+v", hints)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_AddsImplicitDefinitionClaimUse(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{
			Kind:     types.BlockSection,
			MinCount: 1,
			Required: true,
			AcceptableClaimForms: []types.ClaimForm{
				types.ClaimDefinitionFact,
				types.ClaimCallEdge,
				types.ClaimImportEdge,
			},
			SurfaceRoleHint: types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:       "sec_top",
			Kind:     types.BlockSection,
			Text:     "Top-level service responsibilities are already visible here.",
			FacetIDs: []string{"component_relation"},
		}},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 1 {
		t.Fatalf("expected one implicit claim_use repair, got %d", fixed)
	}
	got := doc.Blocks[0].ClaimUses
	if len(got) != 1 || got[0].ClaimForm != types.ClaimDefinitionFact || got[0].FacetID != "component_relation" {
		t.Fatalf("implicit definition claim_use not attached safely: %+v", got)
	}
	if hints := preCheckPrincipalClaimUse(doc, view); len(hints) != 0 {
		t.Fatalf("normalized principal block should not ask the model to retry: %+v", hints)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_AddsAutoRepairableRequiredFacetID(t *testing.T) {
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{{
				Kind:     types.FacetCurrentCodePath,
				Required: types.FacetHardRequired,
			}},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/foo.go", Line: 12}},
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "The answer already cites internal/foo.go:12 and names the current code path.",
		}},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 1 {
		t.Fatalf("expected one auto-repairable facet_id repair, got %d", fixed)
	}
	if got := doc.Blocks[0].FacetIDs; len(got) != 1 || got[0] != string(types.FacetCurrentCodePath) {
		t.Fatalf("current_code_path facet not attached to visible principal block: %+v", got)
	}
	if hints := preCheckFacetCoverage(doc, view); len(hints) != 0 {
		t.Fatalf("auto-repaired facet should avoid a finalizer rewrite hint: %+v", hints)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_DropsScalarWhenExactResolutionAbsent(t *testing.T) {
	view := &types.AnswerSemanticView{
		ExactResolution: &types.ExactResolutionContract{
			Targets: []string{"missing_key"},
		},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "target is absent"},
			{ID: "nearby", Kind: types.BlockScalar, Text: "neighbor_key", SurfaceRole: types.SurfacePrincipal},
			{ID: "layers", Kind: types.BlockOrderedList, Items: []types.AnswerBlockItem{{ID: "l1", Label: "default", Text: "absent"}}},
		},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 1 {
		t.Fatalf("expected scalar removal repair, got %d", fixed)
	}
	for _, block := range doc.Blocks {
		if block.Kind == types.BlockScalar {
			t.Fatalf("absent exact-resolution doc must not retain scalar block: %+v", doc.Blocks)
		}
	}
	if len(doc.Blocks) != 2 || doc.Blocks[0].ID != "summary" || doc.Blocks[1].ID != "layers" {
		t.Fatalf("unexpected blocks after scalar removal: %+v", doc.Blocks)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_KeepsScalarWhenAbsentExactResolutionHasMultipleTargets(t *testing.T) {
	view := &types.AnswerSemanticView{
		ExactResolution: &types.ExactResolutionContract{
			Targets: []string{"missing_key", "existing_key"},
		},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "one target is absent; another value is reported separately"},
			{ID: "existing", Kind: types.BlockScalar, Text: "42", SurfaceRole: types.SurfacePrincipal},
		},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 0 {
		t.Fatalf("multi-target absent exact-resolution must not remove unrelated scalar, got %d", fixed)
	}
	if len(doc.Blocks) != 2 || doc.Blocks[1].Kind != types.BlockScalar {
		t.Fatalf("mixed-target scalar should be retained: %+v", doc.Blocks)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_KeepsScalarWhenExactResolutionMatches(t *testing.T) {
	view := &types.AnswerSemanticView{}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionExactMatch},
		Blocks: []types.AnswerBlock{
			{ID: "summary", Kind: types.BlockSummary, Text: "target resolved"},
			{ID: "value", Kind: types.BlockScalar, Text: "42", SurfaceRole: types.SurfacePrincipal},
		},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 0 {
		t.Fatalf("expected no scalar removal for exact match, got %d", fixed)
	}
	if len(doc.Blocks) != 2 || doc.Blocks[1].Kind != types.BlockScalar {
		t.Fatalf("exact match scalar should be retained: %+v", doc.Blocks)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_DoesNotInventShapeBearingFacetID(t *testing.T) {
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{{
				Kind:     types.FacetDiagramSpine,
				Required: types.FacetHardRequired,
			}},
		},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "internal/foo.go", Line: 12}},
		Blocks: []types.AnswerBlock{{
			ID:          "summary",
			Kind:        types.BlockSummary,
			SurfaceRole: types.SurfacePrincipal,
			Text:        "Plain prose cannot stand in for a requested diagram spine.",
		}},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 0 {
		t.Fatalf("shape-bearing facet must not be auto-declared, got %d repair(s)", fixed)
	}
	if len(doc.Blocks[0].FacetIDs) != 0 {
		t.Fatalf("unexpected shape-bearing facet mutation: %+v", doc.Blocks[0].FacetIDs)
	}
	if hints := preCheckFacetCoverage(doc, view); len(hints) == 0 {
		t.Fatal("missing diagram_spine should remain a hard hint")
	}
}

func TestNormalizeViewCompatibleAnswerDocument_DoesNotInventNonDefinitionClaimUse(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{
			Kind:                 types.BlockSection,
			MinCount:             1,
			Required:             true,
			AcceptableClaimForms: []types.ClaimForm{types.ClaimCallEdge, types.ClaimGuardCondition},
			SurfaceRoleHint:      types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "sec_call",
			Kind: types.BlockSection,
			Text: "A call/guard relation would need an explicit typed carrier.",
		}},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 0 {
		t.Fatalf("normalizer must not choose among non-definition relation forms, got %d", fixed)
	}
	if len(doc.Blocks[0].ClaimUses) != 0 {
		t.Fatalf("unexpected claim_use invention: %+v", doc.Blocks[0].ClaimUses)
	}
}

func TestNormalizeViewCompatibleAnswerDocument_DoesNotAutoClaimListRelations(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{{
			Kind:                 types.BlockOrderedList,
			MinCount:             1,
			Required:             true,
			AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge},
			SurfaceRoleHint:      types.SurfacePrincipal,
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:    "steps",
			Kind:  types.BlockOrderedList,
			Items: []types.AnswerBlockItem{{ID: "i1", Label: "A -> B"}},
		}},
	}
	if fixed := normalizeViewCompatibleAnswerDocument(doc, view); fixed != 0 {
		t.Fatalf("list relation carriers need explicit model intent, got %d repair(s)", fixed)
	}
	if len(doc.Blocks[0].ClaimUses) != 0 {
		t.Fatalf("unexpected list claim_use invention: %+v", doc.Blocks[0].ClaimUses)
	}
}

func TestNormalizeObservedArtifactClaimUseCarriers_RepairsFacetOnlyBlock(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "timeout", Message: "first_byte_timeout exceeded after 40s"}},
	}
	rm := types.RequestModel{
		RawRequest: "第 3 行 first_byte_timeout 表示什么？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		LogTriage:  bundle,
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	mut.SetLogTriage(bundle)
	ctx := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
	}
	if !types.AnswerRequiresObservedArtifactCarrier(ctx) {
		t.Fatal("test setup should require an observed-artifact carrier")
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:       "artifact_boundary",
			Kind:     types.BlockCaveat,
			FacetIDs: []string{string(types.FacetObservedArtifactFact)},
			Text:     "日志第 3 行的 first_byte_timeout 是运行时观察到的信号。",
		}},
	}

	if fixed := normalizeObservedArtifactClaimUseCarriers(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1; doc=%+v", fixed, doc.Blocks[0].ClaimUses)
	}
	if got := doc.Blocks[0].ClaimUses; len(got) != 1 ||
		got[0].ClaimForm != types.ClaimExternalObservation ||
		got[0].FacetID != string(types.FacetObservedArtifactFact) {
		t.Fatalf("observed-artifact claim_use was not repaired correctly: %+v", got)
	}
	if fixed := normalizeObservedArtifactClaimUseCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("repair must be idempotent, got fixed=%d doc=%+v", fixed, doc.Blocks[0].ClaimUses)
	}
}

func TestNormalizeObservedArtifactClaimUseCarriers_DoesNotInventCarrierWithoutFacet(t *testing.T) {
	bundle := &types.LogBundle{
		Errors: []types.LogError{{Type: "timeout", Message: "first_byte_timeout exceeded after 40s"}},
	}
	rm := types.RequestModel{
		RawRequest: "第 3 行 first_byte_timeout 表示什么？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		LogTriage:  bundle,
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	mut.SetLogTriage(bundle)
	ctx := &types.BusContext{
		Mutable:    mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: rm},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "summary",
			Kind: types.BlockSummary,
			Text: "当前源码说明超时后的重试路径。",
		}},
	}

	if fixed := normalizeObservedArtifactClaimUseCarriers(doc, ctx); fixed != 0 {
		t.Fatalf("normalizer must not invent observed-artifact facts without a typed facet, fixed=%d doc=%+v", fixed, doc.Blocks[0].ClaimUses)
	}
	if len(doc.Blocks[0].ClaimUses) != 0 {
		t.Fatalf("unexpected claim_use invention: %+v", doc.Blocks[0].ClaimUses)
	}
}

// TestPreCheckPrincipalClaimUse_SingleFormRelaxation — when contract
// declares exactly one AcceptableClaimForm AND the block carries
// structural grounding (facet_ids + cited items), the missing
// claim_uses is treated as implicit-default and NOT flagged.
func TestPreCheckPrincipalClaimUse_SingleFormRelaxation(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{
				Kind:                 types.BlockOrderedList,
				Required:             true,
				MinCount:             1,
				AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact},
				SurfaceRoleHint:      types.SurfacePrincipal,
			},
		},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{
				ID:          "l1",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				FacetIDs:    []string{"enumeration_item"},
				Items:       []types.AnswerBlockItem{{ID: "i1", CitationRef: 0}},
			},
		},
	}
	if hints := preCheckPrincipalClaimUse(doc, view); len(hints) != 0 {
		t.Errorf("single-form relaxation should suppress the hint; got %v", hints)
	}
}

// TestPreCheckUncertaintyBlock_MissingWhenRequired
func TestPreCheckUncertaintyBlock_MissingWhenRequired(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := preCheckUncertaintyBlock(doc, view)
	if len(hints) != 1 {
		t.Fatalf("want 1 fix hint, got %d", len(hints))
	}
	if !strings.Contains(hints[0].Field, "caveat") {
		t.Errorf("hint should name caveat block; got %q", hints[0].Field)
	}
}

// TestPreCheckUncertaintyBlock_PresentSuppresses
func TestPreCheckUncertaintyBlock_PresentSuppresses(t *testing.T) {
	view := &types.AnswerSemanticView{
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
			{ID: "c1", Kind: types.BlockCaveat, Text: "what we did not search"},
		},
	}
	if hints := preCheckUncertaintyBlock(doc, view); len(hints) != 0 {
		t.Errorf("caveat present should suppress; got %v", hints)
	}
}

// TestPreCheckUncertaintyBlock_NoRules
func TestPreCheckUncertaintyBlock_NoRules(t *testing.T) {
	view := &types.AnswerSemanticView{}
	doc := &types.AnswerDocumentV2{}
	if hints := preCheckUncertaintyBlock(doc, view); len(hints) != 0 {
		t.Errorf("no rules → no hints; got %v", hints)
	}
}

func TestPreCheckFacetCoverage_SoftEvidenceAvailableDoesNotReject(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "answer cites current code through normal citations",
		}},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{{
				Kind:            types.FacetCurrentCodePath,
				Required:        types.FacetSoftRequired,
				PromotionPolicy: types.PromotionWhenEvidenceSufficient,
				SourceCandidate: []string{"ev-current-code"},
			}},
		},
	}
	if hints := preCheckFacetCoverage(doc, view); len(hints) != 0 {
		t.Fatalf("evidence-supported soft facet must stay advisory at emit time, got %v", hints)
	}
}

func TestPreCheckFacetCoverage_HardStillRejects(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "answer without the required bucket label facet",
		}},
	}
	view := &types.AnswerSemanticView{
		FacetCoverage: &types.FacetCoverageContract{
			Required: []types.FacetRequirement{{
				Kind:            types.FacetBucketLabel,
				Required:        types.FacetHardRequired,
				PromotionPolicy: types.PromotionAlwaysHard,
			}},
		},
	}
	hints := preCheckFacetCoverage(doc, view)
	if len(hints) != 1 {
		t.Fatalf("hard facet should still reject when undeclared, got %v", hints)
	}
	if !strings.Contains(hints[0].ExpectedShape, string(types.FacetBucketLabel)) {
		t.Fatalf("hard facet hint should name the missing facet, got %+v", hints[0])
	}
}

func TestPreCheckInactiveScopeDisclosure_DeferredToSystemCaveat(t *testing.T) {
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{
			Status: types.AnswerExactResolutionAbsent,
		},
	}
	busCtx := &types.BusContext{
		PendingSubRepos: []string{"repo-tools-py"},
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent: types.IntentExplain,
				AnalyzerHints: types.AnalyzerHints{
					ExactTargets: []string{"process_request"},
				},
			},
		},
	}
	if hints := preCheckInactiveScopeDisclosure(doc, busCtx); len(hints) != 0 {
		t.Fatalf("inactive scope is a system boundary note, not an emit-time rewrite hint: %v", hints)
	}
}

// TestFormatEmitFixHints_RedlineAudit — pin the rejection envelope
// prose for R6 (no internal vocab leak) + R4 (generic) + LLM-facing
// purity (no third natural language).
func TestFormatEmitFixHints_RedlineAudit(t *testing.T) {
	hints := []emitFixHint{
		{Field: "blocks[].kind=ordered_list", ExpectedShape: "emit at least 1 block of kind=ordered_list", Reason: "enumeration"},
		{Field: "blocks[id=\"l1\"].claim_uses", ExpectedShape: "add a one-element claim_uses[]", Reason: "principal claim required"},
	}
	got := formatEmitFixHints(hints)

	// R6: must not leak internal vocab.
	for _, banned := range []string{
		"orchestrator", "ViolKind", "ViolationKind", "ClusterKey",
		"SuspectedRoot", "BuildRepairPlan", "RepairCluster",
		"AnswerDocumentV2", "FacetCoverage", "AnchoredCount",
		"DeclaredCount", "TaskGraph", "BusContext",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("rejection prose must not leak internal token %q; got %q", banned, got)
		}
	}

	// No third natural language.
	for _, tok := range []string{"diese", "todos", "tutti", "cette", "это", "هذا"} {
		if strings.Contains(got, tok) {
			t.Errorf("rejection prose must be EN only; banned %q in %q", tok, got)
		}
	}

	// Each hint renders verbatim Field + ExpectedShape.
	for _, h := range hints {
		if !strings.Contains(got, h.Field) {
			t.Errorf("hint Field %q missing from output; got %q", h.Field, got)
		}
		if !strings.Contains(got, h.ExpectedShape) {
			t.Errorf("hint ExpectedShape %q missing from output; got %q", h.ExpectedShape, got)
		}
	}

	// Empty list → empty string.
	if got := formatEmitFixHints(nil); got != "" {
		t.Errorf("empty hints → empty string; got %q", got)
	}
}

func TestEmitFixHintsRepair_CarriesTypedFieldsAndKinds(t *testing.T) {
	hints := []emitFixHint{
		{
			Field:         "blocks[].kind=ordered_list",
			ExpectedShape: "emit at least 1 block of kind=ordered_list",
			Reason:        "enumeration",
			Kind:          types.ViolBlockCoverageMissing,
		},
		{
			Field:         "blocks[id=\"l1\"].claim_uses",
			ExpectedShape: "add a one-element claim_uses[]",
			Reason:        "principal claim required",
			Kind:          types.ViolPrincipalClaimUseMissing,
		},
		{
			Field:         "blocks[id=\"l1\"].claim_uses",
			ExpectedShape: "duplicate field should not duplicate repair field",
			Kind:          types.ViolPrincipalClaimUseMissing,
		},
	}
	repair := emitFixHintsRepair(hints)
	if repair == nil {
		t.Fatal("expected structured repair")
	}
	if repair.Code != "answer_doc_pre_emit_contract" {
		t.Fatalf("repair code = %q", repair.Code)
	}
	for _, want := range []string{"blocks[].kind=ordered_list", "blocks[id=\"l1\"].claim_uses"} {
		if !slices.Contains(repair.Fields, want) {
			t.Fatalf("repair fields missing %q: %+v", want, repair.Fields)
		}
	}
	if len(repair.Fields) != 2 {
		t.Fatalf("repair fields should be de-duplicated, got %+v", repair.Fields)
	}
	for _, want := range []string{string(types.ViolBlockCoverageMissing), string(types.ViolPrincipalClaimUseMissing)} {
		if !strings.Contains(repair.Metadata["violation_kinds"], want) {
			t.Fatalf("repair metadata missing kind %q: %+v", want, repair.Metadata)
		}
	}
	if repair.Metadata["hint_count"] != "3" {
		t.Fatalf("repair hint_count = %q", repair.Metadata["hint_count"])
	}
	if !strings.Contains(repair.Hint, "Preserve existing answer facts") {
		t.Fatalf("repair hint should preserve model content, got %q", repair.Hint)
	}
}

func TestEmitFixHintsRepair_Empty(t *testing.T) {
	if repair := emitFixHintsRepair(nil); repair != nil {
		t.Fatalf("empty hints should not fabricate repair metadata: %+v", repair)
	}
}

func TestFormatEmitAdvisoryHints_DoesNotLookLikeRejection(t *testing.T) {
	hints := []emitFixHint{
		{Field: "blocks[].items[].label", ExpectedShape: "keep the cited item label aligned with an already-grounded symbol", Reason: "soft label polish"},
	}
	got := formatEmitAdvisoryHints(hints)
	if strings.Contains(got, "does not yet meet") || strings.Contains(got, "re-emit") || strings.Contains(got, "rejection") {
		t.Fatalf("advisory prose must not look like a retry/rejection hint, got %q", got)
	}
	if !strings.Contains(got, "blocks[].items[].label") {
		t.Fatalf("advisory summary should preserve the affected field, got %q", got)
	}
	if got := formatEmitAdvisoryHints(nil); got != "" {
		t.Fatalf("empty advisory hints -> empty string, got %q", got)
	}
}

func TestNormalizeCurrentSourceCitationSupplement_MaterializesDroppedCitationPool(t *testing.T) {
	mu := types.NewMutableState("解释日志 first_byte_timeout 并结合当前源码说明重试")
	logBundle := &types.LogBundle{Observations: []types.LogObservation{{
		Kind:      types.LogObservationRuntimeEvent,
		LineStart: 3,
		Summary:   "first_byte_timeout exceeded after 40s",
	}}}
	mu.SetLogTriage(logBundle)
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceMechanism,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "StreamFirstByteTimeoutError",
			Source:          "internal/llm/stream_errors.go",
			LineStart:       97,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Origin:          types.ClaimOriginCurrentRepo,
			Summary:         "首字节超时的具体错误类型，记录看门狗等待时长。",
		},
		{
			Kind:            types.EvidenceDirect,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "defaultStreamFirstByteTimeout",
			Source:          "internal/llm/openai.go",
			LineStart:       670,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Origin:          types.ClaimOriginCurrentRepo,
			Summary:         "默认首字节超时阈值为 40 秒。",
		},
		{
			Kind:            types.EvidenceRegistration,
			AnchorKind:      types.AnchorStringLiteral,
			AnchorSymbol:    "模型响应出错,正在重新理解问题",
			Source:          "internal/render/status_messages.go",
			LineStart:       208,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Origin:          types.ClaimOriginCurrentRepo,
			Summary:         "分析阶段模型响应错误时的用户可见重试提示。",
		},
	})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:    types.IntentRootCause,
				Scenario:  types.ScenarioRootCause,
				Language:  "zh",
				LogTriage: logBundle,
				AnalyzerHints: types.AnalyzerHints{
					RequiredFileHints: []types.RequiredFileHint{
						{Path: "internal/llm/openai.go", Confidence: 0.9, Rationale: "first-byte timeout implementation"},
						{Path: "internal/render/status_messages.go", Confidence: 0.9, Rationale: "retry message rendering"},
					},
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "s1",
		Kind:        types.BlockSummary,
		Text:        "first_byte_timeout 表示默认 40 秒内没有收到首个有效 SSE 数据块；系统会识别 StreamFirstByteTimeoutError 并提示用户模型响应出错。",
		SurfaceRole: types.SurfacePrincipal,
		Items:       []types.AnswerBlockItem{{ID: "c0", CitationRef: 4}},
	}}}

	fixed := normalizeCurrentSourceCitationSupplement(doc, ctx, newPreEmitCheckContext(ctx))
	if fixed < 2 {
		t.Fatalf("expected source supplement rows for dropped citation pool, got %d", fixed)
	}
	visible := answerDocumentVisibleText(doc)
	for _, want := range []string{
		"系统按已验证证据补充关键源码锚点",
		"internal/llm/openai.go:670",
		"internal/render/status_messages.go:208",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible supplement missing %q:\n%s", want, visible)
		}
	}
	if len(doc.Citations) < fixed {
		t.Fatalf("citations not materialized with supplement rows: citations=%d rows=%d", len(doc.Citations), fixed)
	}
}

func TestNormalizeCurrentSourceCitationSupplement_DoesNotAppendGenericAnchorAppendix(t *testing.T) {
	mu := types.NewMutableState("结合当前源码说明重试体验")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "defaultStreamFirstByteTimeout",
			Source:          "internal/llm/openai.go",
			LineStart:       670,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Origin:          types.ClaimOriginCurrentRepo,
			Summary:         "默认首字节超时阈值为 40 秒。",
		},
	})
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: types.RequestModel{
				Intent:   types.IntentExplain,
				Scenario: types.ScenarioArchitectureExplain,
				Language: "zh",
				AnalyzerHints: types.AnalyzerHints{
					RequiredFileHints: []types.RequiredFileHint{{
						Path:       "internal/llm/openai.go",
						Confidence: 0.9,
						Rationale:  "first-byte timeout implementation",
					}},
				},
			},
		},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:          "s1",
		Kind:        types.BlockSummary,
		Text:        "这次调整主要让慢模型等待更稳，避免把连接/流式异常误显示成语义重写。",
		SurfaceRole: types.SurfacePrincipal,
	}}}

	if fixed := normalizeCurrentSourceCitationSupplement(doc, ctx, newPreEmitCheckContext(ctx)); fixed != 0 {
		t.Fatalf("required_file hints alone must not create a generic source-anchor appendix, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if visible := answerDocumentVisibleText(doc); strings.Contains(visible, "系统按已验证证据补充关键源码锚点") {
		t.Fatalf("generic source-anchor supplement leaked into visible answer:\n%s", visible)
	}
}

func TestNormalizeAggregateNegativeProofSupplement_MaterializesRepoNegativeSearch(t *testing.T) {
	mu := types.NewMutableState("确认 legacy_config_key 是否仍存在")
	mu.SetInvestigationResultKind("absence")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeSearch,
		Label: "legacy_config_key absent",
		Value: "0",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "repo", Value: "."},
			{Name: "pattern", Value: "legacy_config_key"},
			{Name: "scope", Value: "codrax.yaml.example"},
			{Name: "result_count", Value: "0"},
			{Name: "searched_at", Value: "explore iteration 3"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "当前没有找到该配置项。",
		}},
	}

	if fixed := normalizeAggregateNegativeProofSupplement(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	visible := answerDocumentVisibleText(doc)
	for _, want := range []string{
		"系统按已验证证据补充未命中范围",
		"legacy_config_key",
		"codrax.yaml.example",
		"0 个匹配",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible supplement missing %q:\n%s", want, visible)
		}
	}
	if len(doc.Citations) != 1 || doc.Citations[0].Scope != types.ScopeNegative ||
		doc.Citations[0].NegativePattern != "legacy_config_key" {
		t.Fatalf("negative citation not materialized: %+v", doc.Citations)
	}
	if hints := preCheckAbsenceScopeBound(doc); len(hints) != 0 {
		t.Fatalf("materialized negative citation should satisfy absence scope bound: %+v", hints)
	}
}

func TestNormalizeAggregateNegativeProofSupplement_MaterializesNegativeObservationWithoutRepoCitation(t *testing.T) {
	mu := types.NewMutableState("最近历史里有没有 Backport Foo")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "history search found no backport commits",
		Value: "0",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginVCSMetadata)},
			{Name: "target", Value: "Backport Foo"},
			{Name: "commit_range", Value: "HEAD~50..HEAD"},
			{Name: "result_count", Value: "0"},
			{Name: "window_count", Value: "50"},
			{Name: "unmatched", Value: "50"},
			{Name: "order", Value: "recent"},
			{Name: "window_path", Value: "internal/orchestrator"},
			{Name: "tool_result", Value: "git_history_search[0]"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s1",
		Kind: types.BlockSummary,
		Text: "最近历史没有相关提交。",
	}}}

	if fixed := normalizeAggregateNegativeProofSupplement(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	visible := answerDocumentVisibleText(doc)
	for _, want := range []string{
		"版本历史",
		"Backport Foo",
		"HEAD~50..HEAD",
		"0 个匹配",
		"检索窗口=50",
		"未命中项=50",
		"顺序=recent",
		"历史路径=internal/orchestrator",
		"工具结果=git_history_search[0]",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible supplement missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "vcs_metadata") {
		t.Fatalf("visible supplement should localize typed origin instead of leaking enum names:\n%s", visible)
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("non-repo negative observation must not invent repo citations: %+v", doc.Citations)
	}
	if len(doc.Blocks[len(doc.Blocks)-1].ClaimUses) != 1 ||
		doc.Blocks[len(doc.Blocks)-1].ClaimUses[0].ClaimForm != types.ClaimExternalObservation {
		t.Fatalf("negative observation supplement should carry external observation claim_use: %+v", doc.Blocks[len(doc.Blocks)-1].ClaimUses)
	}
}

func TestNormalizeAggregateNegativeProofSupplement_StructuredAbsentModelAnswerSuppressesDuplicateSupplement(t *testing.T) {
	mu := types.NewMutableState("最近历史里有没有 Backport Foo")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "history search found no backport commits",
		Value: "0",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginVCSMetadata)},
			{Name: "target", Value: "Backport Foo"},
			{Name: "commit_range", Value: "HEAD~50..HEAD"},
			{Name: "result_count", Value: "0"},
			{Name: "tool_result", Value: "git_history_search[0]"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{Status: types.AnswerExactResolutionAbsent},
		Blocks: []types.AnswerBlock{{
			ID:   "s1",
			Kind: types.BlockSummary,
			Text: "Backport Foo 在 HEAD~50..HEAD 范围内没有相关提交；这个结论来自版本历史检索。",
		}},
	}

	if fixed := normalizeAggregateNegativeProofSupplement(doc, ctx); fixed != 0 {
		t.Fatalf("structured absent answer with visible target/scope should not get a duplicate system supplement, fixed=%d doc=%+v", fixed, doc.Blocks)
	}
	if visible := answerDocumentVisibleText(doc); strings.Contains(visible, "系统按已验证证据补充未命中范围") {
		t.Fatalf("duplicate negative supplement leaked into visible answer:\n%s", visible)
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("VCS negative observation must not invent repo citations: %+v", doc.Citations)
	}
}

func TestNormalizeAggregateNegativeProofSupplement_MaterializesFutureExternalObservationOrigins(t *testing.T) {
	mu := types.NewMutableState("外部资料里是否存在这些条目")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{
		{
			Kind:  types.AnswerAggregateNegativeObservation,
			Label: "web no-hit",
			Value: "0",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginWebPage)},
				{Name: "target", Value: "Deprecated API"},
				{Name: "scope", Value: "https://example.test/spec"},
				{Name: "result_count", Value: "0"},
				{Name: "payload_ref", Value: "blob://payload/spec.html"},
			},
		},
		{
			Kind:  types.AnswerAggregateNegativeObservation,
			Label: "mcp no-hit",
			Value: "0",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginMCPResource)},
				{Name: "target", Value: "INC-999"},
				{Name: "scope", Value: "mcp://docs/incidents"},
				{Name: "result_count", Value: "0"},
				{Name: "row_set_ref", Value: "blob://rows/incidents.jsonl"},
			},
		},
		{
			Kind:  types.AnswerAggregateNegativeObservation,
			Label: "connector no-hit",
			Value: "0",
			Role:  types.AnswerAggregateRolePrincipalAnswer,
			Dimensions: []types.AnswerAggregateDimension{
				{Name: "origin", Value: string(types.AnswerEvidenceOriginConnectorResource)},
				{Name: "target", Value: "JIRA-404"},
				{Name: "scope", Value: "jira://project/search"},
				{Name: "result_count", Value: "0"},
				{Name: "tool_result", Value: "jira_search[0]"},
			},
		},
	})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "zh",
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s1",
		Kind: types.BlockSummary,
		Text: "外部资料没有发现相关条目。",
	}}}

	if fixed := normalizeAggregateNegativeProofSupplement(doc, ctx); fixed != 3 {
		t.Fatalf("fixed=%d, want 3", fixed)
	}
	visible := answerDocumentVisibleText(doc)
	for _, want := range []string{
		"系统按已验证证据补充未命中范围",
		"网页",
		"Deprecated API",
		"https://example.test/spec",
		"MCP 资源",
		"INC-999",
		"mcp://docs/incidents",
		"连接器资源",
		"JIRA-404",
		"jira://project/search",
		"载荷=blob://payload/spec.html",
		"行集=blob://rows/incidents.jsonl",
		"工具结果=jira_search[0]",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible external no-hit supplement missing %q:\n%s", want, visible)
		}
	}
	for _, banned := range []string{"web_page", "mcp_resource", "connector_resource"} {
		if strings.Contains(visible, banned) {
			t.Fatalf("visible supplement should localize typed origin %q:\n%s", banned, visible)
		}
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("external no-hit observations must not invent current-source citations: %+v", doc.Citations)
	}
}

func TestNormalizeAggregateNegativeProofSupplement_LocalizesEnglishNegativeObservationOrigin(t *testing.T) {
	mu := types.NewMutableState("Did the last 50 commits mention Backport Foo?")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateNegativeObservation,
		Label: "history search found no backport commits",
		Value: "0",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "origin", Value: string(types.AnswerEvidenceOriginVCSMetadata)},
			{Name: "target", Value: "Backport Foo"},
			{Name: "commit_range", Value: "HEAD~50..HEAD"},
			{Name: "result_count", Value: "0"},
			{Name: "window_count", Value: "50"},
			{Name: "tool_result", Value: "git_history_search[0]"},
		},
	}})
	mu.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mu,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentExplain,
			Language: "en",
		}},
	}
	doc := &types.AnswerDocumentV2{Blocks: []types.AnswerBlock{{
		ID:   "s1",
		Kind: types.BlockSummary,
		Text: "No related commit was found in the recent history window.",
	}}}

	if fixed := normalizeAggregateNegativeProofSupplement(doc, ctx); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	visible := answerDocumentVisibleText(doc)
	for _, want := range []string{
		"System-verified no-hit scope supplement",
		"VCS history",
		"Backport Foo",
		"HEAD~50..HEAD",
		"0 matches",
		"window=50",
		"tool result=git_history_search[0]",
	} {
		if !strings.Contains(visible, want) {
			t.Fatalf("visible English supplement missing %q:\n%s", want, visible)
		}
	}
	if strings.Contains(visible, "vcs_metadata") {
		t.Fatalf("visible English supplement should localize typed origin instead of leaking enum names:\n%s", visible)
	}
	if len(doc.Citations) != 0 {
		t.Fatalf("external negative observation must not create repo citations: %+v", doc.Citations)
	}
}

// TestRunPreEmitChecks_AggregatesAcrossAllAxes — happy + fail mix.
// Pin that the four checks all wire through the top-level dispatcher.
func TestRunPreEmitChecks_AggregatesAcrossAllAxes(t *testing.T) {
	view := &types.AnswerSemanticView{
		RequiredBlocks: []types.BlockRequirement{
			{Kind: types.BlockOrderedList, Required: true, MinCount: 1, AcceptableClaimForms: []types.ClaimForm{types.ClaimDefinitionFact, types.ClaimCallEdge}, SurfaceRoleHint: types.SurfacePrincipal},
		},
		UncertaintyRules: []types.UncertaintyRule{{TriggerFacet: ""}},
	}
	// Doc that fails 3 of 4 checks: missing ordered_list, missing
	// caveat, missing facet declarations (when we'd add facet
	// coverage), missing principal claim_use is moot because
	// ordered_list itself is missing.
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{
			{ID: "s1", Kind: types.BlockSummary, Text: "x"},
		},
	}
	hints := runPreEmitChecks(doc, view, nil)
	if len(hints) < 2 {
		t.Errorf("want ≥2 fix hints (block coverage + uncertainty); got %d (%v)", len(hints), hints)
	}
}

func TestNormalizePrincipalSupportMemberCarriers_DoesNotAuthorMissingTypedMembers(t *testing.T) {
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:         "ActivityManagerService exposes isAppForeground",
				Location:     "java/com/android/server/am/ActivityManagerService.java:19031",
				Source:       "java/com/android/server/am/ActivityManagerService.java",
				LineStart:    19031,
				ClaimForm:    types.ClaimDefinitionFact,
				AnchorSymbol: "isAppForeground",
				SurfaceTerms: []string{"isAppForeground", "isAppForeground(uid)"},
			}, {
				Text:         "ActivityTaskManagerInternal declares IAncoActivityManager",
				Location:     "java/com/android/server/wm/ActivityTaskManagerInternal.java:47",
				Source:       "java/com/android/server/wm/ActivityTaskManagerInternal.java",
				LineStart:    47,
				ClaimForm:    types.ClaimDefinitionFact,
				AnchorSymbol: "IAncoActivityManager",
				SurfaceTerms: []string{"IAncoActivityManager"},
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{{File: "java/com/android/server/am/ActivityManagerService.java", Line: 19031}},
		Blocks: []types.AnswerBlock{{
			ID:          "principal",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "fg",
				Label:       "isAppForeground",
				Text:        "Definition evidence.",
				CitationRef: 0,
			}},
		}},
	}
	if missing := types.MissingPrincipalSupportMembers(doc, plan); len(missing) != 1 {
		t.Fatalf("test setup missing=%d, want 1: %+v", len(missing), missing)
	}
	if fixed := normalizePrincipalSupportMemberCarriers(doc, plan); fixed != 0 {
		t.Fatalf("fixed=%d, want 0; normalizer must not author visible answer members", fixed)
	}
	if missing := types.MissingPrincipalSupportMembers(doc, plan); len(missing) != 1 {
		t.Fatalf("coverage obligation should remain visible to the checker, got %+v", missing)
	}
	if len(doc.Citations) != 1 || len(doc.Blocks[0].Items) != 1 {
		t.Fatalf("normalizer must leave citations/items unchanged: citations=%+v items=%+v", doc.Citations, doc.Blocks[0].Items)
	}
}

func TestNormalizePrincipalSupportMemberCarriers_AddsHiddenCitationSidecarForVisibleMarkdownTable(t *testing.T) {
	member := "ActivityManagerService.java (前后台状态通知/mForegroundPackages系统级聚合)"
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:         member,
				Location:     "java/com/android/server/am/ActivityManagerService.java:1618",
				Source:       "java/com/android/server/am/ActivityManagerService.java",
				LineStart:    1618,
				ClaimForm:    types.ClaimDefinitionFact,
				SurfaceTerms: []string{member},
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Blocks: []types.AnswerBlock{{
			ID:          "files_table",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Text:        "| 文件 | 感知机制类型 |\n|---|---|\n| ActivityManagerService.java | 前后台状态通知/mForegroundPackages系统级聚合 |",
		}},
	}

	if missing := types.MissingPrincipalSupportMembers(doc, plan); len(missing) != 1 {
		t.Fatalf("test setup missing=%d, want 1: %+v", len(missing), missing)
	}
	if fixed := normalizePrincipalSupportMemberCarriers(doc, plan); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	if len(doc.Citations) != 1 ||
		!strings.EqualFold(doc.Citations[0].File, "java/com/android/server/am/ActivityManagerService.java") ||
		doc.Citations[0].Line != 1618 {
		t.Fatalf("expected appended typed citation, got %+v", doc.Citations)
	}
	if len(doc.Blocks[0].Items) != 1 || doc.Blocks[0].Items[0].CitationRef != 0 {
		t.Fatalf("expected hidden citation sidecar item, got %+v", doc.Blocks[0].Items)
	}
	if got, want := doc.Blocks[0].Items[0].ID, "support-activitymanagerservice_java_mforegroundpackages"; got != want {
		t.Fatalf("hidden sidecar item id = %q, want stable id %q", got, want)
	}
	if missing := types.MissingPrincipalSupportMembers(doc, plan); len(missing) != 0 {
		t.Fatalf("hidden citation sidecar should satisfy visible markdown table member, got %+v", missing)
	}
}

func TestNormalizePrincipalSupportMemberCarriers_RepairsVisibleSplitItemCitation(t *testing.T) {
	member := "ActivityManagerService.java (前后台状态通知/mForegroundPackages系统级聚合)"
	plan := &types.AnswerSupportPlan{
		Family:                  types.QFEnumeration,
		PrincipalMemberCoverage: types.PrincipalMemberCoveragePolicyRequired,
		Lanes: []types.AnswerSupportLane{{
			Kind: types.SupportLanePrincipalEvidence,
			Entries: []types.AnswerSupportEntry{{
				Text:         member,
				Location:     "java/com/android/server/am/ActivityManagerService.java:1618",
				Source:       "java/com/android/server/am/ActivityManagerService.java",
				LineStart:    1618,
				ClaimForm:    types.ClaimDefinitionFact,
				SurfaceTerms: []string{member},
			}},
		}},
	}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "java/com/android/server/wm/ActivityTaskManagerService.java", Line: 1153},
			{File: "java/com/android/server/am/ActivityManagerService.java", Line: 1618},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "files_table",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "f2",
				Label:       "ActivityManagerService.java",
				Text:        "前后台状态通知/mForegroundPackages系统级聚合",
				CitationRef: 0,
			}},
		}},
	}

	if missing := types.MissingPrincipalSupportMembers(doc, plan); len(missing) != 1 {
		t.Fatalf("test setup missing=%d, want 1: %+v", len(missing), missing)
	}
	if fixed := normalizePrincipalSupportMemberCarriers(doc, plan); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 1 {
		t.Fatalf("citation_ref=%d, want 1", got)
	}
	if missing := types.MissingPrincipalSupportMembers(doc, plan); len(missing) != 0 {
		t.Fatalf("repaired split visible item should satisfy support-member coverage, got %+v", missing)
	}
}

func TestNormalizeItemCitationRefsByUniqueLabelCitation_UsesDecoratedAggregateFileMemberSupportRef(t *testing.T) {
	member := "ActivityManagerService.java (前后台状态通知/mForegroundPackages系统级聚合)"
	mu := types.NewMutableState("decorated file aggregate citation repair")
	mu.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:        types.AnswerAggregateMemberSet,
		Label:       "东湖前后台感知机制涉及的核心文件",
		Value:       "1",
		Members:     []string{member},
		SupportRefs: []string{member + " @ java/com/android/server/am/ActivityManagerService.java:1618"},
	}})
	mu.SetInvestigationComplete("decorated citation repair")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "java/com/android/server/wm/ActivityTaskManagerService.java", Line: 1153},
			{File: "java/com/android/server/am/ActivityManagerService.java", Line: 1618},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "files_table",
			Kind:        types.BlockTable,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "f2",
				Label:       "ActivityManagerService.java",
				Text:        "前后台状态通知/mForegroundPackages系统级聚合",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, nil, ctx, newPreEmitCheckContext(ctx)); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 1 {
		t.Fatalf("citation_ref=%d, want 1", got)
	}
	if got := preCheckItemCitationAlignmentWithContext(doc, nil, newPreEmitCheckContext(ctx)); len(got) != 0 {
		t.Fatalf("repaired decorated aggregate file item should pass citation alignment, got %+v", got)
	}
}

func TestNormalizeItemCitationRefsByUniqueLabelCitation_KeepsDirectoryScopedEnumerationCitation(t *testing.T) {
	block := types.AnswerBlock{
		ID:       "enum",
		Kind:     types.BlockOrderedList,
		FacetIDs: []string{string(types.FacetEnumerationItem)},
	}
	if !preEmitEnumerationDirectoryLabelCitationScoped(block, "aggregator", types.Citation{File: "internal/analysis/aggregator/aggregator.go", Line: 112}) {
		t.Fatal("directory-scoped package label should protect an already same-package citation")
	}
	if preEmitEnumerationDirectoryLabelCitationScoped(block, "aggregator", types.Citation{File: "internal/analysis/hint/composer.go", Line: 142}) {
		t.Fatal("different-package citation must not be treated as scoped to the label")
	}

	mu := types.NewMutableState("列出 internal/analysis 子包入口函数")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "internal/analysis/aggregator/aggregator.go", Line: 112},
			{File: "internal/analysis/hint/composer.go", Line: 142},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "enum",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetEnumerationItem)},
			Items: []types.AnswerBlockItem{{
				ID:          "p1",
				Label:       "aggregator",
				Text:        "入口函数 New（构造函数），实例化 Aggregator 并返回 *Aggregator。",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, nil, ctx, newPreEmitCheckContext(ctx)); fixed != 0 {
		t.Fatalf("same-directory citation should not be rewritten, fixed=%d doc=%+v", fixed, doc)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 0 {
		t.Fatalf("citation_ref changed to %d; system must not move package row citations across directories", got)
	}
}

func TestNormalizeItemCitationRefsByUniqueLabelCitation_UsesExplicitSurfaceLocationBeforeFuzzyCandidate(t *testing.T) {
	mu := types.NewMutableState("梳理调用链")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "include/linux/bpf.h", Line: 83},
			{File: "kernel/bpf/syscall.c", Line: 1766},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "anchors",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetResolvedLiteralOrSymbol)},
			Items: []types.AnswerBlockItem{{
				ID:          "a3",
				Label:       "map_update_elem",
				Text:        "BPF_MAP_UPDATE_ELEM 命令处理函数，定义于 kernel/bpf/syscall.c:1766。",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, nil, ctx, newPreEmitCheckContext(ctx)); fixed != 1 {
		t.Fatalf("fixed=%d, want 1", fixed)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 1 {
		t.Fatalf("citation_ref=%d, want explicit syscall.c:1766 citation", got)
	}
}

func TestNormalizeItemCitationRefsByUniqueLabelCitation_DoesNotRewriteAmbiguousExplicitLocations(t *testing.T) {
	mu := types.NewMutableState("梳理调用链")
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "kernel/bpf/hashtab.c", Line: 2355},
			{File: "kernel/bpf/syscall.c", Line: 1766},
			{File: "include/linux/bpf.h", Line: 107},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "anchors",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetResolvedLiteralOrSymbol)},
			Items: []types.AnswerBlockItem{{
				ID:          "a3",
				Label:       "map_update_elem",
				Text:        "定义于 kernel/bpf/syscall.c:1766；接口指针在 include/linux/bpf.h:107。",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, nil, ctx, newPreEmitCheckContext(ctx)); fixed != 0 {
		t.Fatalf("ambiguous explicit locations must not be auto-rewritten, fixed=%d doc=%+v", fixed, doc)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 0 {
		t.Fatalf("citation_ref changed to %d; ambiguous item should remain model-authored", got)
	}
}

func TestNormalizeItemCitationRefsByUniqueLabelCitation_DoesNotUseFuzzyFirstCandidateForAmbiguousCallChain(t *testing.T) {
	mu := types.NewMutableState("梳理调用链")
	mu.AppendEvidence([]types.EvidenceItem{
		{
			Kind:            types.EvidenceDirect,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "map_update_elem",
			Source:          "kernel/bpf/syscall.c",
			LineStart:       1766,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "map_update_elem handles BPF_MAP_UPDATE_ELEM.",
		},
		{
			Kind:            types.EvidenceDirect,
			AnchorKind:      types.AnchorDefinition,
			AnchorSymbol:    "map_update_elem",
			Source:          "include/linux/bpf.h",
			LineStart:       107,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "bpf_map_ops exposes the map_update_elem function pointer.",
		},
	})
	ctx := &types.BusContext{Mutable: mu}
	doc := &types.AnswerDocumentV2{
		Citations: []types.Citation{
			{File: "kernel/bpf/hashtab.c", Line: 2355},
			{File: "kernel/bpf/syscall.c", Line: 1766},
			{File: "include/linux/bpf.h", Line: 107},
		},
		Blocks: []types.AnswerBlock{{
			ID:          "anchors",
			Kind:        types.BlockOrderedList,
			SurfaceRole: types.SurfacePrincipal,
			FacetIDs:    []string{string(types.FacetResolvedLiteralOrSymbol)},
			Items: []types.AnswerBlockItem{{
				ID:          "a3",
				Label:       "map_update_elem",
				Text:        "命令处理和 ops 函数指针都围绕该符号展开。",
				CitationRef: 0,
			}},
		}},
	}

	if fixed := normalizeItemCitationRefsByUniqueLabelCitationWithContext(doc, nil, ctx, newPreEmitCheckContext(ctx)); fixed != 0 {
		t.Fatalf("ambiguous endpoint candidates must not fall back to first fuzzy candidate, fixed=%d doc=%+v", fixed, doc)
	}
	if got := doc.Blocks[0].Items[0].CitationRef; got != 0 {
		t.Fatalf("citation_ref changed to %d; fuzzy ambiguity should remain model-authored", got)
	}
}

// === preEmitDisplaySurfaceAppears typographic normalisation ===
//
// docs/design/post_phase2a_forensic_followups.md §2.3 — finalizer
// retries hit raw fallback when the investigator's member surface
// used EN paren + no inner spaces and the finalizer rendered the
// same identity with zh paren + ASCII-CJK boundary spaces. The
// strict comparator rejected the typographic variant; these tests
// pin the normalised fallback behaviour without relaxing the
// strict path on pure-ASCII identifiers.

func TestPreEmitDisplaySurfaceAppears_ZhParenEquivalentToEnParen(t *testing.T) {
	value := "Foo (bar)"
	surface := "下面三个组件: Foo（bar） 是关键。"
	if !preEmitDisplaySurfaceAppears(value, surface) {
		t.Fatalf("zh-paren surface should accept EN-paren member (typographic fold)\nvalue=%q\nsurface=%q", value, surface)
	}
	// Symmetric: EN-paren surface should also accept zh-paren member.
	if !preEmitDisplaySurfaceAppears("Foo（bar）", "Use Foo (bar) here.") {
		t.Fatal("EN-paren surface should accept zh-paren member")
	}
}

func TestPreEmitDisplaySurfaceAppears_AsciiCjkBoundarySpaceFolds(t *testing.T) {
	// Member written by investigator (no spaces around vs) vs prose
	// (spaces around vs) — typographic difference only.
	if !preEmitDisplaySurfaceAppears("vs正文", "比较 vs 正文 是关键") {
		t.Fatal("ASCII-CJK boundary spaces in surface should be folded so member 'vs正文' matches 'vs 正文'")
	}
	if !preEmitDisplaySurfaceAppears("vs 正文", "summary比较vs正文是关键") {
		t.Fatal("symmetric: member with ASCII-CJK space should match folded surface")
	}
}

func TestPreEmitDisplaySurfaceAppears_PureAsciiIdentifierUnchanged(t *testing.T) {
	// Helper itself must be a no-op on pure-ASCII content so the
	// strict identifier-boundary semantics stay intact.
	if got := preEmitNormalizeForMixedCJK("gate.RunWith"); got != "gate.RunWith" {
		t.Fatalf("pure-ASCII identifier must pass through byte-identical, got %q", got)
	}
	// Strict comparator behaviour preserved: 'gate.RunWith' must
	// NOT match 'gate.RunWithTimeout' (identifier-suffix boundary
	// check still fires; normalised fallback does not relax it).
	if preEmitDisplaySurfaceAppears("gate.RunWith", "Call gate.RunWithTimeout(ctx) now") {
		t.Fatal("normalisation must not relax identifier-suffix boundary; gate.RunWith should not match gate.RunWithTimeout")
	}
	// Positive control: identical pure-ASCII still matches strictly.
	if !preEmitDisplaySurfaceAppears("gate.RunWith", "Use gate.RunWith for this") {
		t.Fatal("pure-ASCII identifier should still match identical token in surface")
	}
}

func TestPreEmitDisplaySurfaceAppears_AbsentMemberStillRejected(t *testing.T) {
	// A genuinely absent member must NOT pass — neither typographic
	// nor whitespace folding makes the token appear.
	if preEmitDisplaySurfaceAppears("NotInDoc", "finalizer 写了 SelfConsistencyReviewer（摘要 vs 正文独立 LLM） 等组件") {
		t.Fatal("absent member must be rejected by the normalised fallback (no substring-search relaxation)")
	}
	if preEmitDisplaySurfaceAppears("AlphaOnly (xxx)", "BetaOnly（xxx）出现在文中") {
		t.Fatal("typographic fold must not match a member whose name token is absent from surface")
	}
}

func TestPreEmitDisplaySurfaceAppears_NineNineOhNineForensicReplay(t *testing.T) {
	// Verbatim 09:09 production case shape: investigator emitted EN
	// paren + no spaces around "vs"; finalizer wrote zh paren +
	// spaces. Strict comparator failed 3× → quota burn → raw
	// fallback. The normalised fallback must accept.
	member := "SelfConsistencyReviewer (摘要vs正文独立LLM)"
	surface := "本节列举四个组件:SelfConsistencyReviewer（摘要 vs 正文独立 LLM）是其中一个。"
	if !preEmitDisplaySurfaceAppears(member, surface) {
		t.Fatalf("09:09 forensic case must match after typographic fold\nmember=%q\nsurface=%q\nmember normalised=%q\nsurface normalised=%q",
			member, surface,
			preEmitNormalizeForMixedCJK(member),
			preEmitNormalizeForMixedCJK(surface))
	}
}

func TestPreEmitSystemEnumerationRowSupplementBlock_RecognizesAllSystemSupplementTitles(t *testing.T) {
	systemTitles := []string{
		"系统按已验证证据补充缺失成员：公开函数（2）",
		"System-verified missing member supplement: exported functions (2)",
		"系统按已验证证据补充成员：公开函数（2）",
		"System-verified member supplement: exported functions (2)",
		"系统按已验证证据补充可校验字段：公开函数（2）",
		"System-verified field supplement: exported functions (2)",
		"系统按已验证证据补充说明：公开函数（2）",
		"System-verified note supplement: exported functions (2)",
	}
	for _, title := range systemTitles {
		if !preEmitSystemEnumerationRowSupplementBlock(types.AnswerBlock{Title: title}) {
			t.Fatalf("system supplement title was not recognized: %q", title)
		}
	}
	modelTitles := []string{
		"模型给出的公开函数表",
		"Exported functions",
		"系统设计说明：公开函数",
	}
	for _, title := range modelTitles {
		if preEmitSystemEnumerationRowSupplementBlock(types.AnswerBlock{Title: title}) {
			t.Fatalf("model-authored title must not be classified as system supplement: %q", title)
		}
	}
}
