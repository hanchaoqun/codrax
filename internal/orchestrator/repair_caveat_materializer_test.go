package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestMaterializeCaveats_GroupsByFamily — multiple violations in the
// same CaveatFamily collapse to one user-visible caveat. Mirrors the
// qfa-mr3 forensic case: 3 violations, 2 share answer_coverage, 1 is
// diagram_fidelity → 2 caveats output.
// TestSoftDemotedKinds_T3_MaterializeToCaveats pins the 2026-05-17 T3
// soft-demote contract: each of the five oracle kinds whose default
// severity was flipped to SeveritySoft materialises as a user-visible
// caveat through the standard caveat family pipeline (no retry, no
// raw fallback). Catches CaveatFamilyID drift on these specific
// kinds — if a future PR clears their CaveatFamilyID, the answer
// would silently lose the disclosure channel after the demotion.
func TestSoftDemotedKinds_T3_MaterializeToCaveats(t *testing.T) {
	for _, kind := range []types.ViolationKind{
		types.ViolRichnessGlaringGap,
		types.ViolPrincipalProseUnderfilled,
		types.ViolUncertaintyBlockMissing,
		types.ViolStructuralEnumerationDivergence,
		types.ViolSymbolAnchorMismatch,
	} {
		spec, ok := types.ViolKindSpecFor(kind)
		if !ok {
			t.Errorf("kind=%q: missing registry spec", kind)
			continue
		}
		if spec.CaveatFamilyID == "" {
			t.Errorf("kind=%q: empty CaveatFamilyID; T3 demoted kinds need a user-visible disclosure channel", kind)
			continue
		}
		violations := []types.Violation{{Kind: kind}}
		caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
		if len(caveats) == 0 {
			t.Errorf("kind=%q: produced zero caveats; expected one via family %q", kind, spec.CaveatFamilyID)
		}
	}
}

func TestMaterializeCaveats_GroupsByFamily(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolFacetUncovered},
		{Kind: types.ViolFacetUncovered},
		{Kind: types.ViolDiagramEdgeUnsupported},
	}
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
	if len(caveats) != 2 {
		t.Fatalf("expected 2 caveats (1 per family), got %d: %v", len(caveats), caveats)
	}
}

func TestAppendSoftContractCaveatsToAnswer_MaterializesDefaultSoftConcerns(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	out := AppendSoftContractCaveatsToAnswer("正文", []types.Violation{
		{Kind: types.ViolUncertaintyBlockMissing},
		{Kind: types.ViolSuccessCriterion},
	}, "zh")
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("soft caveat heading missing:\n%s", out)
	}
	if strings.Contains(out, "uncertainty_block_missing") || strings.Contains(out, "success_criterion") {
		t.Fatalf("soft caveat leaked internal violation names:\n%s", out)
	}
	if strings.Count(out, "- ") != 2 {
		t.Fatalf("expected two soft caveat bullets; got output:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswer_MaterializesSpecificSelfContradiction(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	out := AppendSoftContractCaveatsToAnswer("正文", []types.Violation{{
		Kind:       types.ViolSelfContradiction,
		Detail:     `self_contradiction[counts] — SUMMARY: "c9d1fe22 4 个、9419be91 2 个" ⇄ BODY: "c9d1fe22 2 个、9419be91 4 个"`,
		ClusterKey: types.TopicClusterKey("counts", "answer_summary_body_consistency"),
	}}, "zh")
	for _, want := range []string{
		"**补充说明：**",
		"答案有一处前后不一致",
		"c9d1fe22 4 个、9419be91 2 个",
		"c9d1fe22 2 个、9419be91 4 个",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("specific self-contradiction caveat missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "答案前后某些表述存在不完全一致") {
		t.Fatalf("self-contradiction should use the specific reviewer claims, not the generic caveat:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_ObservationOnlyUsesBoundaryCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := observationOnlyRuntimeCaveatTestContext("这是什么错误？")
	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolPrincipalProseUnderfilled, ClusterKey: types.BlockKindClusterKey(types.BlockSummary, "answer_prose_density")},
		{Kind: types.ViolBlockCoverageMissing, ClusterKey: types.BlockKindClusterKey(types.BlockSummary, "answer_block_coverage")},
		{Kind: types.ViolUncertaintyBlockMissing, ClusterKey: types.BlockKindClusterKey(types.BlockCaveat, "uncertainty_block")},
	}, "zh", ctx)

	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("boundary caveat heading missing:\n%s", out)
	}
	if !strings.Contains(out, "栈帧未映射到当前仓库") {
		t.Fatalf("expected precise runtime-artifact boundary caveat:\n%s", out)
	}
	for _, banned := range []string{"覆盖度可能不充分", "结合源码进一步核对", "answer_block_coverage", "uncertainty_block", "answer_prose_density"} {
		if strings.Contains(out, banned) {
			t.Fatalf("observation-only soft caveat leaked generic/internal wording %q:\n%s", banned, out)
		}
	}
}

func TestAppendUserCaveatsToAnswerForBus_ObservationOnlySuppressesGenericSelfContradiction(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := observationOnlyRuntimeCaveatTestContext("只分析这段 trace")
	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolSelfContradiction,
		ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
	}}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("generic runtime self-contradiction caveat should stay telemetry-only, got:\n%s", out)
	}
}

func TestAppendUserCaveatsToAnswerForBus_ObservationOnlyKeepsSpecificSelfContradiction(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := observationOnlyRuntimeCaveatTestContext("只分析这段 trace")
	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolSelfContradiction,
		Detail:     `self_contradiction[trace] — SUMMARY: "running=3.5ms" ⇄ BODY: "running=2.7ms"`,
		ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
	}}, "zh", ctx)
	for _, want := range []string{"**补充说明：**", "running=3.5ms", "running=2.7ms"} {
		if !strings.Contains(out, want) {
			t.Fatalf("specific runtime self-contradiction caveat missing %q:\n%s", want, out)
		}
	}
}

func TestAppendUserCaveatsToAnswerForBus_RuntimeAnswerSurfaceSuppressesGenericCaveats(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceCaveatTestContext("分析这段 trace", false)
	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{
		{
			Kind:       types.ViolSelfContradiction,
			ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
		},
		{
			Kind:       types.ViolEnumerationEvidenceUnderspecified,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
		{
			Kind:       types.ViolEnumerationLabelUngrounded,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
		{
			Kind:       types.ViolEnumerationLabelHallucinated,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("generic runtime answer-surface caveats should stay telemetry-only, got:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_RuntimeAnswerSurfaceSuppressesGenericCaveats(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceOnlyCaveatTestContext(true)
	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{
			Kind:       types.ViolSelfContradiction,
			ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
		},
		{
			Kind:       types.ViolEnumerationEvidenceUnderspecified,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
		{
			Kind:       types.ViolEnumerationLabelUngrounded,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
		{
			Kind:       types.ViolEnumerationItemLabelExtractorDrift,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
		{
			Kind:       types.ViolEnumerationLabelHallucinated,
			ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
		},
		{
			Kind:       types.ViolDeniedTokenUndeclared,
			Detail:     `answer block "d1" names token "trace中无UI渲染span可关联" without disclosing it as unverified / external`,
			ClusterKey: "denied_token_undeclared:d1",
		},
		{
			Kind:       types.ViolPrincipalProseUnderfilled,
			ClusterKey: types.BlockKindClusterKey(types.BlockSummary, "answer_prose_density"),
		},
		{
			Kind:       types.ViolUncertaintyBlockMissing,
			ClusterKey: types.BlockKindClusterKey(types.BlockCaveat, "uncertainty_block"),
		},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("generic runtime answer-surface soft caveats should stay telemetry-only, got:\n%s", out)
	}
}

func TestAppendUserCaveatsToAnswerForBus_RuntimeAnswerSurfaceUsesPrincipalLikeBlocks(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceOnlyCaveatTestContext(false)
	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolSelfContradiction,
		ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
	}}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("principal-like external observation blocks should suppress generic caveats, got:\n%s", out)
	}
}

func TestAppendUserCaveatsToAnswerForBus_RuntimeAnswerSurfaceKeepsMixedVisibleBlocks(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceOnlyCaveatTestContext(false)
	doc := ctx.Mutable.AnswerDocumentV2()
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "unannotated",
		Kind: types.BlockSection,
		Text: "current-source explanation without typed external claim",
	})
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolSelfContradiction,
		ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
	}}, "zh", ctx)
	if !strings.Contains(out, "答案前后某些表述") {
		t.Fatalf("mixed visible blocks should keep generic caveat disclosure:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_RuntimeAnswerSurfaceKeepsDeniedTokenForMixedBlocks(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceOnlyCaveatTestContext(false)
	doc := ctx.Mutable.AnswerDocumentV2()
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:   "unannotated",
		Kind: types.BlockSection,
		Text: "current-source explanation without typed external claim",
	})
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolDeniedTokenUndeclared,
		Detail:     `answer block "d1" names token "trace中无UI渲染span可关联" without disclosing it as unverified / external`,
		ClusterKey: "denied_token_undeclared:d1",
	}}, "zh", ctx)
	if !strings.Contains(out, "答案前后某些表述") {
		t.Fatalf("mixed visible blocks should keep typed-denial caveat disclosure:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_RuntimeAnswerSurfaceKeepsUncertaintyForMixedBlocks(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceCaveatTestContext("分析这段 trace", false)
	doc := ctx.Mutable.AnswerDocumentV2()
	doc.Blocks = append(doc.Blocks, types.AnswerBlock{
		ID:          "unannotated",
		Kind:        types.BlockSection,
		SurfaceRole: types.SurfacePrincipal,
		Text:        "current-source explanation without typed external claim",
	})
	ctx.Mutable.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, doc)

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolUncertaintyBlockMissing,
		ClusterKey: types.BlockKindClusterKey(types.BlockCaveat, "uncertainty_block"),
	}}, "zh", ctx)
	if !strings.Contains(out, "覆盖度可能不充分") {
		t.Fatalf("mixed visible blocks should keep uncertainty caveat disclosure:\n%s", out)
	}
}

func TestAppendUserCaveatsToAnswerForBus_RuntimeAnswerSurfaceKeepsSpecificContradiction(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceCaveatTestContext("分析这段 trace", false)
	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolSelfContradiction,
		Detail:     `self_contradiction[trace] — SUMMARY: "runnable=5.0ms" ⇄ BODY: "runnable=2.0ms"`,
		ClusterKey: types.TopicClusterKey("trace metrics", "answer_summary_body_consistency"),
	}}, "zh", ctx)
	for _, want := range []string{"**补充说明：**", "runnable=5.0ms", "runnable=2.0ms"} {
		if !strings.Contains(out, want) {
			t.Fatalf("specific runtime answer-surface contradiction missing %q:\n%s", want, out)
		}
	}
}

func TestAppendUserCaveatsToAnswerForBus_RuntimePrincipalEnumerationKeepsEnumerationCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	ctx := runtimeAnswerSurfaceCaveatTestContext("列出 trace 中所有事件", true)
	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind:       types.ViolEnumerationEvidenceUnderspecified,
		ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "block_items_label"),
	}}, "zh", ctx)
	if !strings.Contains(out, "枚举类条目") {
		t.Fatalf("principal runtime enumeration should keep enumeration caveat:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_PureHistoryNarrativeSuppressesCurrentSourceCaveats(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "最近一次合入的是什么特性？",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioGeneric,
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqHistory)},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: rm,
		},
	}
	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered},
		{Kind: types.ViolBlockCoverageMissing},
		{Kind: types.ViolEnumerationLabelUngrounded},
		{Kind: types.ViolAnswerSemanticUnderfilled},
		{Kind: types.ViolCitation},
		{Kind: types.ViolAcceptance},
		{Kind: types.ViolSymbolAnchorMismatch},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("pure VCS history narrative should not get generic current-source coverage caveats:\n%s", out)
	}

	mixed := rm
	mixed.ChangeImpactProfile = &types.ChangeImpactProfile{IsChangeImpact: true}
	mixedMut := types.NewMutableState("分析这次提交对当前代码的影响")
	mixedMut.SetRequestModel(mixed)
	mixedCtx := &types.BusContext{Mutable: mixedMut}
	mixedOut := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered},
	}, "zh", mixedCtx)
	if !strings.Contains(mixedOut, "**补充说明：**") {
		t.Fatalf("mixed history/current-code request should keep caveat disclosure:\n%s", mixedOut)
	}
}

func observationOnlyRuntimeCaveatTestContext(request string) *types.BusContext {
	mut := types.NewMutableState(request)
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "called Option::unwrap() on a None value",
		}},
	}
	mut.SetRequestModel(types.RequestModel{
		RawRequest: request,
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		LogTriage:  logBundle,
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			CurrentSourceMode: types.ExternalObservationCurrentSourceExclude,
			ExclusionKind:     types.ExternalObservationSourceExclusionExplicitUserBoundary,
			SourceQuotes:      []string{"只分析日志"},
			Confidence:        0.9,
		},
	})
	return &types.BusContext{Mutable: mut}
}

func runtimeAnswerSurfaceCaveatTestContext(request string, enumerate bool) *types.BusContext {
	mut := types.NewMutableState(request)
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "trace",
			Message: "state_churn runnable dominates",
		}},
	}
	intent := types.IntentRootCause
	if enumerate {
		intent = types.IntentEnumerate
	}
	mut.SetRequestModel(types.RequestModel{
		RawRequest: request,
		Intent:     intent,
		Scenario:   types.ScenarioPerformanceBottleneck,
		LogTriage:  logBundle,
		Predicates: types.SemanticPredicates{
			IsDiagnosticQuestion: true,
		},
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
		ExternalObservationPolicy: &types.ExternalObservationPolicy{
			ArtifactCitationMode: types.ExternalObservationArtifactCitationExternalOnly,
			CurrentSourceMode:    types.ExternalObservationCurrentSourceDefault,
			Confidence:           0.9,
		},
	})
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: types.SurfacePrincipal,
				Text:        "runtime trace summary",
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
					FacetID:   string(types.FacetObservedArtifactFact),
				}},
			},
			{
				ID:          "metrics",
				Kind:        types.BlockOrderedList,
				SurfaceRole: types.SurfacePrincipal,
				Items: []types.AnswerBlockItem{
					{ID: "dominant", Label: "dominant_state", Text: "runnable"},
				},
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
					FacetID:   string(types.FacetObservedArtifactFact),
				}},
			},
		},
	})
	return &types.BusContext{Mutable: mut}
}

func runtimeAnswerSurfaceOnlyCaveatTestContext(useSurfaceRole bool) *types.BusContext {
	mut := types.NewMutableState("分析外部 trace 观测")
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "分析外部 trace 观测",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioGeneric,
	})
	surfaceRole := types.SurfaceRole("")
	if useSurfaceRole {
		surfaceRole = types.SurfacePrincipal
	}
	mut.SetAnswerDocumentV2WithMutation(types.MutationReplaceAll, &types.AnswerDocumentV2{
		DocumentModel: "v2",
		Blocks: []types.AnswerBlock{
			{
				ID:          "summary",
				Kind:        types.BlockSummary,
				SurfaceRole: surfaceRole,
				Text:        "external observation summary",
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
					FacetID:   string(types.FacetObservedArtifactFact),
				}},
			},
			{
				ID:          "metrics",
				Kind:        types.BlockOrderedList,
				SurfaceRole: surfaceRole,
				Items: []types.AnswerBlockItem{
					{ID: "dominant", Label: "dominant_state", Text: "runnable", CitationRef: -1},
				},
				ClaimUses: []types.RenderedClaimUse{{
					ClaimForm: types.ClaimExternalObservation,
					FacetID:   string(types.FacetObservedArtifactFact),
				}},
			},
		},
	})
	return &types.BusContext{Mutable: mut}
}

func TestAppendSoftContractCaveatsToAnswerForBus_MechanismSuppressesGenericEnumerationAdvisories(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "解释 explorer 如何调用 subagent",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Predicates: types.SemanticPredicates{},
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqMechanism),
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolEnumerationEvidenceUnderspecified},
		{Kind: types.ViolEnumerationLabelUngrounded},
		{Kind: types.ViolPrincipalClaimUseMissing},
		{Kind: types.ViolBlockCoverageMissing, ClusterKey: types.BlockKindClusterKey(types.BlockOrderedList, "answer_block_coverage")},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("mechanism answers should not surface generic enumeration/metadata caveats on accepted path:\n%s", out)
	}
}

func TestAppendUserCaveatsToAnswerForBus_MechanismSuppressesGenericUnclusteredCoverage(t *testing.T) {
	rm := types.RequestModel{
		RawRequest: "解释数据任务工作流",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqMechanism),
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered},
		{Kind: types.ViolAnswerSemanticUnderfilled},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("accepted mechanism answer should suppress generic coverage/semantic caveats:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_MechanismSuppressesGenericCitationAndAcceptanceAdvisories(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "解释 Criterion 和 Hypothesis 的工作流",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqMechanism),
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolCitation},
		{Kind: types.ViolGhostAnchor},
		{Kind: types.ViolChainDemoted},
		{Kind: types.ViolLiteralFormFailed},
		{Kind: types.ViolSymbolAnchorMismatch},
		{Kind: types.ViolAcceptance},
		{Kind: types.ViolSuccessCriterion},
		{Kind: types.ViolFamilyMismatch},
		{Kind: types.ViolClaimFormUnsupported},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("accepted mechanism answers should not surface generic citation/acceptance caveats:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_CallChainKeepsCitationButSuppressesGenericAcceptance(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "梳理入口函数到实现函数的调用链",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqCallChain),
		},
		SourceInventoryProfile: &types.SourceInventoryProfile{
			IsSourceInventory: true,
			TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleField},
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolCitation},
		{Kind: types.ViolAcceptance},
		{Kind: types.ViolClaimFormUnsupported},
		{Kind: types.ViolEnumerationEvidenceUnderspecified},
		{Kind: types.ViolEnumerationLabelUngrounded},
		{Kind: types.ViolEnumerationLabelHallucinated},
		{Kind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolDiagramRelationLabelOnly},
		{Kind: types.ViolDiagramEdgeEndpointHallucinated},
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetBranchGuard), "answer_facet_coverage")},
	}, "zh", ctx)
	if !strings.Contains(out, "锚点") {
		t.Fatalf("call-chain answers should keep concrete citation grounding caveat:\n%s", out)
	}
	if strings.Contains(out, "验收检查") {
		t.Fatalf("soft accept path must not surface generic acceptance caveat:\n%s", out)
	}
	if strings.Contains(out, "枚举类") {
		t.Fatalf("call-chain answers are hop/path surfaces, not principal enumerations:\n%s", out)
	}
	for _, banned := range []string{"覆盖度可能不充分", "图示中部分边"} {
		if strings.Contains(out, banned) {
			t.Fatalf("accepted call-chain path should suppress non-actionable telemetry caveat %q:\n%s", banned, out)
		}
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_ScalarKeepsCitationGroundingCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "defaultMaxSteps 的默认值是多少？",
		Intent:     types.IntentReturnValue,
		Predicates: types.SemanticPredicates{
			IsScalarAnswer: true,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind: string(types.ReqReturnValue),
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolCitation},
		{Kind: types.ViolSymbolAnchorMismatch},
	}, "zh", ctx)
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("exact scalar answers should keep citation-boundary disclosure:\n%s", out)
	}
	if !strings.Contains(out, "锚点") {
		t.Fatalf("expected citation-grounding caveat for scalar answer:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_EnumerationKeepsEnumerationBoundaryCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "列出全部公开函数",
		Intent:     types.IntentEnumerate,
		Predicates: types.SemanticPredicates{IsCategoryEnumeration: true},
		CompletenessObligation: &types.CompletenessObligation{
			Required:    true,
			SourceQuote: "全部公开函数",
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolEnumerationEvidenceUnderspecified},
	}, "zh", ctx)
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("true enumeration gaps should remain visible caveats:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_CrossComponentEnumerationKeepsEnumerationCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "分别列出 A 和 B 的全部公开函数",
		Intent:     types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
		Buckets: []types.QuestionBucket{{Label: "A"}, {Label: "B"}},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolEnumerationEvidenceUnderspecified},
	}, "zh", ctx)
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("cross-component enumeration still has a principal enumeration surface:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_DiagramRequestKeepsDiagramBlockCoverageCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest:    "解释流程并画图",
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioArchitectureExplain,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolBlockCoverageMissing, ClusterKey: types.BlockKindClusterKey(types.BlockDiagram, "answer_block_coverage")},
	}, "zh", ctx)
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("explicit diagram requests should keep precise diagram-missing disclosure:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_DiagramRequestKeepsDiagramFacetCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest:    "解释流程并画图",
		Intent:        types.IntentExplain,
		Scenario:      types.ScenarioArchitectureExplain,
		DiagramHint:   &types.DiagramHint{Kind: types.DiagramArchitecture},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetDiagramSpine), "answer_facet_coverage")},
	}, "zh", ctx)
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("explicit diagram requests should keep diagram facet disclosure:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_AcceptedMechanismSuppressesOptionalFacetTelemetry(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "分析这两次提交对当前代码路径的影响",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Predicates: types.SemanticPredicates{
			IsHistoryLookup: true,
		},
		AnalyzerHints:       types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		ChangeImpactProfile: &types.ChangeImpactProfile{IsChangeImpact: true},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetComponentRelation), "answer_facet_coverage")},
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetDiagramSpine), "answer_richness_facet_coverage")},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("accepted mechanism/history answers should not surface generic facet telemetry:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_RelationalComparisonKeepsComponentRelationFacetCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "这两个模块通过哪些接口交互？",
		Intent:     types.IntentExplain,
		Scenario:   types.ScenarioArchitectureExplain,
		Predicates: types.SemanticPredicates{
			IsCrossComponent:   true,
			IsRelationalLookup: true,
		},
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqMechanism)},
		Buckets:       []types.QuestionBucket{{Label: "A"}, {Label: "B"}},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetComponentRelation), "answer_facet_coverage")},
	}, "zh", ctx)
	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("relational comparison should keep component-relation disclosure:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_NonRelationalComparisonSuppressesFacetMetadata(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "分别列出两个子仓的核心导出入口和功能用途",
		Intent:     types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
		Buckets: []types.QuestionBucket{{Label: "repo-a"}, {Label: "repo-b"}},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetComponentRelation), "answer_facet_coverage")},
		{Kind: types.ViolFacetUncovered, ClusterKey: types.FacetClusterKey(string(types.FacetCurrentCodePath), "answer_facet_coverage")},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("principal comparison/enumeration answers should keep generic facet metadata in telemetry:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_CrossRepoComparisonSuppressesStructuralEnumerationDivergence(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "对比 repo-a 和 repo-b 的核心导出入口",
		Intent:     types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			IsCrossComponent:      true,
		},
		Buckets: []types.QuestionBucket{{Label: "repo-a"}, {Label: "repo-b"}},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolStructuralEnumerationDivergence},
	}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("cross-component comparison should keep structural enumeration divergence in telemetry:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswerForBus_PrincipalEnumerationKeepsStructuralEnumerationDivergence(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	rm := types.RequestModel{
		RawRequest: "列出接口的所有实现",
		Intent:     types.IntentEnumerate,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
		},
	}
	mut := types.NewMutableState(rm.RawRequest)
	mut.SetRequestModel(rm)
	ctx := &types.BusContext{Mutable: mut, AnalysisIR: &types.AnalysisIR{RequestModel: rm}}

	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolStructuralEnumerationDivergence},
	}, "zh", ctx)
	if !strings.Contains(out, "枚举类条目") {
		t.Fatalf("principal enumeration should keep structural divergence visible:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswer_SkipsStrictPromotedConcern(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolUncertaintyBlockMissing)})

	out := AppendSoftContractCaveatsToAnswer("正文", []types.Violation{
		{Kind: types.ViolUncertaintyBlockMissing},
	}, "zh")
	if out != "正文" {
		t.Fatalf("strict-promoted concern should stay actionable, not become a soft caveat:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswer_SkipsLaneBlockKindTelemetry(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	out := AppendSoftContractCaveatsToAnswer("正文", []types.Violation{
		{Kind: types.ViolLaneBlockKindMismatch},
	}, "zh")
	if out != "正文" {
		t.Fatalf("lane/block-kind metadata mismatch should remain telemetry-only on the accepted path:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswer_SkipsRichnessRegressionTelemetry(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	out := AppendSoftContractCaveatsToAnswer("正文", []types.Violation{
		{
			Kind:       types.ViolRichnessRegression,
			Detail:     `optional richness facet "diagram_spine" has 1 evidence candidate(s) but no block surfaced it (telemetry only — answer ships unchanged)`,
			ClusterKey: types.FacetClusterKey("diagram_spine", "answer_richness_facet_coverage"),
		},
	}, "zh")
	if out != "正文" {
		t.Fatalf("richness regression must remain telemetry-only, not a generic coverage caveat:\n%s", out)
	}
}

func TestAppendSoftContractCaveatsToAnswer_SuppressesEnumDepthWhenMemberSetFullyCited(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	mut := types.NewMutableState("列出 Kind 常量")
	mut.AppendEvidence([]types.EvidenceItem{
		{
			ID:              "ev-symbol-present",
			Kind:            types.EvidenceDirect,
			Subject:         "KindSymbolPresent",
			AnchorSymbol:    "KindSymbolPresent",
			AnchorKind:      types.AnchorDefinition,
			Source:          "internal/analysis/criterion/grammar.go",
			LineStart:       29,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "KindSymbolPresent is defined here.",
		},
		{
			ID:              "ev-no-call-sites",
			Kind:            types.EvidenceDirect,
			Subject:         "KindNoCallSites",
			AnchorSymbol:    "KindNoCallSites",
			AnchorKind:      types.AnchorDefinition,
			Source:          "internal/analysis/criterion/grammar.go",
			LineStart:       30,
			Scope:           types.ScopeLine,
			GroundingStatus: types.GroundingGrounded,
			Summary:         "KindNoCallSites is defined here.",
		},
	})
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateMemberSet,
		Label: "Kind constants",
		Value: "2",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Members: []string{
			"KindSymbolPresent",
			"KindNoCallSites",
		},
		SupportRefs: []string{
			"KindSymbolPresent @ internal/analysis/criterion/grammar.go:29",
			"KindNoCallSites @ internal/analysis/criterion/grammar.go:30",
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:   types.IntentEnumerate,
			Language: "zh",
			Predicates: types.SemanticPredicates{
				IsCategoryEnumeration: true,
			},
			SourceInventoryProfile: &types.SourceInventoryProfile{
				IsSourceInventory: true,
				TargetRoles:       []types.AnswerCandidateRole{types.AnswerCandidateRoleFunction},
				RequestedFields: []types.SourceInventoryRequestedField{
					types.SourceInventoryFieldName,
					types.SourceInventoryFieldLocation,
				},
			},
		}},
	}

	out := AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind: types.ViolEnumerationLabelUngrounded,
	}}, "zh", ctx)
	if out != "正文" {
		t.Fatalf("fully cited accepted enumeration should suppress repaired enum-depth caveat:\n%s", out)
	}

	out = AppendUserCaveatsToAnswerForBus("正文", []types.Violation{{
		Kind: types.ViolEnumerationLabelHallucinated,
	}}, "zh", ctx)
	if !strings.Contains(out, "枚举类条目") {
		t.Fatalf("hallucinated enumeration labels must still display the enum-depth caveat:\n%s", out)
	}
}

// TestMaterializeCaveats_NoInternalJargon — output strings must not
// contain ViolKind names, IR field names, confidence numbers, or
// orchestration tokens.
func TestMaterializeCaveats_NoInternalJargon(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolRichnessRegression},
		{Kind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolEnumerationLabelUngrounded},
		{Kind: types.ViolAuthorityOverreach},
	}
	for _, lang := range []string{"zh", "en"} {
		caveats := MaterializeUnresolvedViolationsAsCaveats(violations, lang)
		for _, c := range caveats {
			for _, token := range []string{
				"yield kill", "Pipeline terminated", "retryUsed",
				"ViolKind", "IRField", "conf=", "event(s)",
				"block_items_label", "diagram_edges", "facet_uncovered",
				"answer_authority", "answer_richness_facet_coverage",
			} {
				if strings.Contains(c, token) {
					t.Errorf("[%s] caveat contains forbidden token %q: %q", lang, token, c)
				}
			}
		}
	}
}

// TestMaterializeCaveats_SkipsOperatorOnly — violations whose
// ViolKind is reviewer / frequency-bridge produce NO caveat.
func TestMaterializeCaveats_SkipsOperatorOnly(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolPlanCritic},
		{Kind: types.ViolReflectorObservation},
		{Kind: types.ViolDemotionStorm},
	}
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
	if len(caveats) != 0 {
		t.Errorf("operator-only violations must produce zero caveats; got %d: %v", len(caveats), caveats)
	}
}

// TestMaterializeCaveats_LangFallback — empty/unknown lang defaults
// to ZH (project default). Explicit "en" returns English template.
func TestMaterializeCaveats_LangFallback(t *testing.T) {
	violations := []types.Violation{{Kind: types.ViolFacetUncovered}}
	zh := MaterializeUnresolvedViolationsAsCaveats(violations, "")
	en := MaterializeUnresolvedViolationsAsCaveats(violations, "en")
	if len(zh) != 1 || len(en) != 1 {
		t.Fatalf("expected 1 caveat each; got zh=%d en=%d", len(zh), len(en))
	}
	if zh[0] == en[0] {
		t.Errorf("ZH and EN templates rendered identically — language switch broken")
	}
	if !strings.ContainsAny(zh[0], "答案在某些维度") {
		t.Errorf("ZH caveat does not look Chinese: %q", zh[0])
	}
	if strings.ContainsAny(en[0], "答案") {
		t.Errorf("EN caveat unexpectedly contains Chinese: %q", en[0])
	}
}

// TestMaterializeCaveats_CapAt3 — output capped at MaxMaterializedCaveats
// to avoid drowning the user. Test fires 5 distinct families.
func TestMaterializeCaveats_CapAt3(t *testing.T) {
	violations := []types.Violation{
		{Kind: types.ViolFacetUncovered},             // answer_coverage
		{Kind: types.ViolDiagramEdgeUnsupported},     // diagram_fidelity
		{Kind: types.ViolEnumerationLabelUngrounded}, // enumeration_depth
		{Kind: types.ViolGhostAnchor},                // citation_grounding
		{Kind: types.ViolAuthorityOverreach},         // authority_hedging
	}
	caveats := MaterializeUnresolvedViolationsAsCaveats(violations, "zh")
	if len(caveats) > MaxMaterializedCaveats {
		t.Errorf("cap broken: got %d caveats, max %d", len(caveats), MaxMaterializedCaveats)
	}
}

// TestMaterializeCaveats_EmptyInput — nil/empty input → nil output.
func TestMaterializeCaveats_EmptyInput(t *testing.T) {
	if got := MaterializeUnresolvedViolationsAsCaveats(nil, "zh"); got != nil {
		t.Errorf("nil input should give nil output, got %v", got)
	}
	if got := MaterializeUnresolvedViolationsAsCaveats([]types.Violation{}, "zh"); got != nil {
		t.Errorf("empty input should give nil output, got %v", got)
	}
}

// TestMaterializeCaveats_StableOrder — same violations in different
// input order produce same output. Important for retry parity (so
// repeated runs don't see caveats shuffle order).
func TestMaterializeCaveats_StableOrder(t *testing.T) {
	a := []types.Violation{
		{Kind: types.ViolFacetUncovered},
		{Kind: types.ViolDiagramEdgeUnsupported},
	}
	b := []types.Violation{
		{Kind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolFacetUncovered},
	}
	out1 := MaterializeUnresolvedViolationsAsCaveats(a, "zh")
	out2 := MaterializeUnresolvedViolationsAsCaveats(b, "zh")
	if strings.Join(out1, "|") != strings.Join(out2, "|") {
		t.Errorf("input order changed output: %v vs %v", out1, out2)
	}
}

func TestAppendResidualConcernDetails_PrintsTypedItemsNoDockLeak(t *testing.T) {
	violations := []types.Violation{
		{
			Kind:       types.ViolMustInclude,
			ClusterKey: types.IdentityClusterKey("term:AnalyzerAgent", "must_include"),
			Detail:     `required symbol "AnalyzerAgent" missing from answer`,
			Repair:     "include AnalyzerAgent in the final answer",
		},
		{
			Kind:       types.ViolMustInclude,
			ClusterKey: types.IdentityClusterKey("term:FinalizerAgent", "must_include"),
			Detail:     `required symbol "FinalizerAgent" missing from answer`,
			Repair:     "include FinalizerAgent in the final answer",
		},
	}

	out := AppendResidualConcernDetailsToAnswer("answer body", violations, "zh")
	for _, want := range []string{
		"**补充说明：**",
		"质量审阅仍有 2 项未完全解决",
		"`AnalyzerAgent`",
		"`FinalizerAgent`",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("residual detail output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"答案已交付", "详见下方说明", "Viol", "must_include"} {
		if strings.Contains(out, banned) {
			t.Fatalf("residual detail output leaked %q:\n%s", banned, out)
		}
	}
	if got := strings.Count(out, "**补充说明：**"); got != 1 {
		t.Fatalf("system notes heading count = %d, want 1:\n%s", got, out)
	}
}

func TestAppendResidualConcernDetails_SemanticConcernUsesStructuredObservation(t *testing.T) {
	violations := []types.Violation{
		{
			Kind:       types.ViolAnswerSemanticUnderfilled,
			ClusterKey: types.TopicClusterKey("source to sink call chain", "answer_semantic_quality"),
			Detail:     "answer_underfilled[coverage:source to sink call chain] — observation: missing the middle compile and bind calls",
			Repair:     `[1/1] expand the answer to address "source to sink call chain". Add the missing middle calls. Reviewer rationale: internal reviewer text`,
		},
	}

	out := AppendResidualConcernDetailsToAnswer("answer body", violations, "en")
	for _, want := range []string{
		"**Additional notes:**",
		"`source to sink call chain`",
		"missing the middle compile and bind calls",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("semantic residual output missing %q:\n%s", want, out)
		}
	}
	for _, banned := range []string{"answer_underfilled", "Reviewer rationale", "ViolKind", "IRField"} {
		if strings.Contains(out, banned) {
			t.Fatalf("semantic residual output leaked %q:\n%s", banned, out)
		}
	}
}
