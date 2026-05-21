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

func TestAppendSoftContractCaveatsToAnswerForBus_ObservationOnlyUsesBoundaryCaveat(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	mut := types.NewMutableState("这是什么错误？")
	logBundle := &types.LogBundle{
		Errors: []types.LogError{{
			Type:    "panic",
			Message: "called Option::unwrap() on a None value",
		}},
	}
	mut.SetRequestModel(types.RequestModel{
		RawRequest: "这是什么错误？",
		Intent:     types.IntentRootCause,
		Scenario:   types.ScenarioRootCause,
		LogTriage:  logBundle,
		DiagnosticProfile: types.DiagnosticIntentProfile{
			IsDiagnostic: true,
		},
	})
	ctx := &types.BusContext{Mutable: mut}
	out := AppendSoftContractCaveatsToAnswerForBus("正文", []types.Violation{
		{Kind: types.ViolBlockCoverageMissing, ClusterKey: types.BlockKindClusterKey(types.BlockSummary, "answer_block_coverage")},
		{Kind: types.ViolUncertaintyBlockMissing, ClusterKey: types.BlockKindClusterKey(types.BlockCaveat, "uncertainty_block")},
	}, "zh", ctx)

	if !strings.Contains(out, "**补充说明：**") {
		t.Fatalf("boundary caveat heading missing:\n%s", out)
	}
	if !strings.Contains(out, "栈帧未映射到当前仓库") {
		t.Fatalf("expected precise runtime-artifact boundary caveat:\n%s", out)
	}
	for _, banned := range []string{"覆盖度可能不充分", "结合源码进一步核对", "answer_block_coverage", "uncertainty_block"} {
		if strings.Contains(out, banned) {
			t.Fatalf("observation-only soft caveat leaked generic/internal wording %q:\n%s", banned, out)
		}
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
