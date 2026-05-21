package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/analysis/contract"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestComputeRootCauseClosure_DiagramRotation pins the qfa-mr3
// case: ViolDiagramEdgeUnsupported and ViolDiagramEdgeLabelMismatch
// firing on the same fingerprint should collapse to one root
// violation. Without this collapse, qfa-mr3 saw stable=0 every
// round even though the underlying root never moved.
func TestComputeRootCauseClosure_DiagramRotation(t *testing.T) {
	fp := types.SuspectedRoot{IRField: "diagram_edges"}
	violations := []types.Violation{
		{
			Kind:          types.ViolDiagramEdgeUnsupported,
			Detail:        "block id=\"d1\" edge missing typed anchor",
			SuspectedRoot: fp,
		},
		{
			Kind:          types.ViolDiagramEdgeLabelMismatch,
			Detail:        "block id=\"d1\" edge label drifts",
			SuspectedRoot: fp,
		},
	}
	out := ComputeRootCauseClosure(violations)
	if len(out) != 2 {
		t.Fatalf("output length changed: got %d", len(out))
	}
	// EdgeUnsupported is the root (Implies includes EdgeLabelMismatch).
	for _, v := range out {
		if v.Kind == types.ViolDiagramEdgeUnsupported {
			if v.IsDerived {
				t.Errorf("root %q should not be marked derived", v.Kind)
			}
		}
		if v.Kind == types.ViolDiagramEdgeLabelMismatch {
			if !v.IsDerived {
				t.Errorf("symptom %q should be marked IsDerived", v.Kind)
			}
			if v.RootKind != types.ViolDiagramEdgeUnsupported {
				t.Errorf("RootKind = %q, want %q", v.RootKind, types.ViolDiagramEdgeUnsupported)
			}
		}
	}
}

func TestClaimBindingsDescribeNonHardNarrativeSupport_UsesSharedOriginClass(t *testing.T) {
	bindings := []types.AnswerClaimBinding{{
		Origin:           types.AnswerEvidenceOriginMCPResource,
		AggregateRole:    types.AnswerAggregateRolePrincipalAnswer,
		GroundingPolicy:  types.ClaimGroundingRepairable,
		RequestedOutputs: []types.AnswerRequestedOutput{types.AnswerRequestedOutputMechanism},
	}}
	if !claimBindingsDescribeNonHardNarrativeSupport(bindings) {
		t.Fatal("external/MCP principal support should use the shared origin-specific support class")
	}
	bindings[0].Origin = types.AnswerEvidenceOriginCurrentSource
	bindings[0].GroundingPolicy = types.ClaimGroundingHard
	if claimBindingsDescribeNonHardNarrativeSupport(bindings) {
		t.Fatal("current-source hard support must not be treated as non-hard narrative support")
	}
}

// TestComputeRootCauseClosure_DifferentFingerprintsStaySeparate —
// only same-fingerprint groups collapse. Two ViolFacetUncovered on
// different facets stay independent even though their kinds match.
func TestComputeRootCauseClosure_DifferentFingerprintsStaySeparate(t *testing.T) {
	violations := []types.Violation{
		{
			Kind:          types.ViolFacetUncovered,
			Detail:        "facet \"diagram_spine\" not covered",
			SuspectedRoot: types.SuspectedRoot{IRField: "diagram_spine"},
		},
		{
			Kind:          types.ViolRichnessRegression,
			Detail:        "facet \"component_relation\" not covered",
			SuspectedRoot: types.SuspectedRoot{IRField: "component_relation"},
		},
	}
	out := ComputeRootCauseClosure(violations)
	for _, v := range out {
		if v.IsDerived {
			t.Errorf("violation %q with unique fingerprint should not be derived; got RootKind=%q",
				v.Kind, v.RootKind)
		}
	}
}

// TestComputeRootCauseClosure_LeafKindsPassThrough — violations
// whose Kind has empty Implies AND aren't pointed-to by any other
// kind's Implies in the group remain as roots themselves.
func TestComputeRootCauseClosure_LeafKindsPassThrough(t *testing.T) {
	violations := []types.Violation{
		{
			Kind:          types.ViolGhostAnchor,
			SuspectedRoot: types.SuspectedRoot{IRField: "ghost"},
		},
		{
			Kind:          types.ViolSelfContradiction,
			SuspectedRoot: types.SuspectedRoot{IRField: "self"},
		},
	}
	out := ComputeRootCauseClosure(violations)
	for _, v := range out {
		if v.IsDerived {
			t.Errorf("leaf kind %q should not be derived", v.Kind)
		}
	}
}

// TestComputeRootCauseClosure_BlockMissingImpliesEnumeration — the
// W2.2 Implies entry for ViolBlockCoverageMissing covers
// ViolEnumerationLabelUngrounded. When both fire on the same fp,
// the enumeration violation is the symptom.
func TestComputeRootCauseClosure_BlockMissingImpliesEnumeration(t *testing.T) {
	fp := types.SuspectedRoot{IRField: "block_items_label"}
	violations := []types.Violation{
		{
			Kind:          types.ViolBlockCoverageMissing,
			SuspectedRoot: fp,
		},
		{
			Kind:          types.ViolEnumerationLabelUngrounded,
			SuspectedRoot: fp,
		},
	}
	out := ComputeRootCauseClosure(violations)
	for _, v := range out {
		if v.Kind == types.ViolEnumerationLabelUngrounded {
			if !v.IsDerived {
				t.Errorf("expected ViolEnumerationLabelUngrounded to be derived under BlockCoverageMissing")
			}
			if v.RootKind != types.ViolBlockCoverageMissing {
				t.Errorf("RootKind = %q, want ViolBlockCoverageMissing", v.RootKind)
			}
		}
	}
}

// TestFilterDerivedViolations — sanity check the public surface.
func TestFilterDerivedViolations(t *testing.T) {
	in := []types.Violation{
		{Kind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolDiagramEdgeLabelMismatch, IsDerived: true, RootKind: types.ViolDiagramEdgeUnsupported},
		{Kind: types.ViolGhostAnchor},
	}
	out := FilterDerivedViolations(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 root violations, got %d", len(out))
	}
	for _, v := range out {
		if v.IsDerived {
			t.Errorf("filter kept derived: %v", v)
		}
	}
}

// TestRenderViolations_SkipsDerived pins the W2.4 wiring: when
// renderViolations sees a result whose only violations are
// IsDerived=true, the retry hint comes back empty (the root's
// derivation has been collapsed by ComputeRootCauseClosure
// upstream).
func TestRenderViolations_SkipsDerived(t *testing.T) {
	res := stubResultWithViolations(
		types.Violation{
			Kind:      types.ViolDiagramEdgeLabelMismatch,
			IsDerived: true,
			RootKind:  types.ViolDiagramEdgeUnsupported,
		},
	)
	if got := renderViolations(res); got != "" {
		t.Errorf("renderViolations should suppress all-derived input, got %q", got)
	}
}

// TestRenderViolations_KeepsRoot — a root violation co-existing
// with its derivation surfaces only the root in the retry hint.
func TestRenderViolations_KeepsRoot(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolDiagramEdgeUnsupported)})

	res := stubResultWithViolations(
		types.Violation{Kind: types.ViolDiagramEdgeUnsupported, Detail: "edge needs typed anchor"},
		types.Violation{
			Kind:      types.ViolDiagramEdgeLabelMismatch,
			IsDerived: true,
			RootKind:  types.ViolDiagramEdgeUnsupported,
		},
	)
	got := renderViolations(res)
	if got == "" {
		t.Fatal("renderViolations should surface the root violation")
	}
}

func TestFilterActionableRootViolations_DropsSoftTelemetry(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolSuccessCriterion)})

	in := []types.Violation{
		{Kind: types.ViolRichnessRegression, Detail: "optional richness telemetry"},
		{Kind: types.ViolDiagramRelationLabelOnly, Detail: "label-only diagram telemetry"},
		{Kind: types.ViolSuccessCriterion, Detail: "citation count failed"},
	}
	out := FilterActionableRootViolations(in)
	if len(out) != 1 {
		t.Fatalf("expected only the hard retry root, got %d: %+v", len(out), out)
	}
	if out[0].Kind != types.ViolSuccessCriterion {
		t.Fatalf("kind = %q, want %q", out[0].Kind, types.ViolSuccessCriterion)
	}
}

func TestFilterActionableRootViolations_KeepsDefaultSoftPromotableForRepairPlanning(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	in := []types.Violation{
		{Kind: types.ViolUncertaintyBlockMissing, Detail: "uncertainty disclosure is absent"},
		{Kind: types.ViolRichnessGlaringGap, Detail: "optional facet is underfilled"},
	}
	out := FilterActionableRootViolations(in)
	if len(out) != 2 {
		t.Fatalf("generic repair planning should retain promotable soft roots, got %d: %+v", len(out), out)
	}
}

func TestFilterFinalizerRetryRootViolations_DropsDefaultSoftPromotableKinds(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, nil)

	in := []types.Violation{
		{Kind: types.ViolUncertaintyBlockMissing, Detail: "uncertainty disclosure is absent"},
		{Kind: types.ViolRichnessGlaringGap, Detail: "optional facet is underfilled"},
		{Kind: types.ViolSuccessCriterion, Detail: "citation count failed"},
	}
	out := FilterFinalizerRetryRootViolations(in)
	if len(out) != 0 {
		t.Fatalf("default-soft promotable kinds should not drive finalizer retry, got %d: %+v", len(out), out)
	}
}

func TestFilterFinalizerRetryRootViolations_StrictPromotionKeepsKindActionable(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolUncertaintyBlockMissing)})

	in := []types.Violation{
		{Kind: types.ViolUncertaintyBlockMissing, Detail: "uncertainty disclosure is absent"},
	}
	out := FilterFinalizerRetryRootViolations(in)
	if len(out) != 1 {
		t.Fatalf("strict-promoted kind should remain actionable, got %d: %+v", len(out), out)
	}
	if out[0].Kind != types.ViolUncertaintyBlockMissing {
		t.Fatalf("kind = %q, want %q", out[0].Kind, types.ViolUncertaintyBlockMissing)
	}
}

func TestFilterFinalizerRetryRootViolations_StrictCannotPromoteNonPromotableKind(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolDiagramEdgeEndpointHallucinated)})

	in := []types.Violation{
		{Kind: types.ViolDiagramEdgeEndpointHallucinated, Detail: "diagram endpoint is a component label"},
	}
	out := FilterFinalizerRetryRootViolations(in)
	if len(out) != 0 {
		t.Fatalf("non-promotable diagram endpoint concern must not drive finalizer retry, got %d: %+v", len(out), out)
	}
}

func TestFilterFinalizerRetryRootViolationsForBus_ClaimBindingSuppressesGenericStrictCoverage(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolFacetUncovered)})

	mut := types.NewMutableState("最近一次合入的是什么特性？")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "latest merge feature",
		Value: "优化 repo map 缓存",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "vcs_metadata"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentExplain,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
			},
		}},
	}

	in := []types.Violation{{
		Kind:   types.ViolFacetUncovered,
		Detail: "generic current-code facet not declared",
	}}
	out := FilterFinalizerRetryRootViolationsForBus(in, bus)
	if len(out) != 0 {
		t.Fatalf("non-exact VCS narrative support should not force strict finalizer retry, got %+v", out)
	}
}

func TestFilterFinalizerRetryRootViolationsForBus_KeepsExactAndRequestedShapeDefects(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{
		string(types.ViolFacetUncovered),
		string(types.ViolBlockCoverageMissing),
		string(types.ViolMustInclude),
	})

	mut := types.NewMutableState("最近一次合入的 commit id 是多少？")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind:  types.AnswerAggregateScalar,
		Label: "latest merge commit",
		Value: "abc1234",
		Role:  types.AnswerAggregateRolePrincipalAnswer,
		Dimensions: []types.AnswerAggregateDimension{
			{Name: "evidence_origin", Value: "vcs_metadata"},
		},
	}})
	mut.RetainInvestigationAggregateFacts()
	bus := &types.BusContext{
		Mutable: mut,
		AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent: types.IntentReturnValue,
			Predicates: types.SemanticPredicates{
				IsHistoryLookup: true,
				IsScalarAnswer:  true,
			},
		}},
	}

	in := []types.Violation{
		{Kind: types.ViolFacetUncovered, Detail: "facet missing"},
		{Kind: types.ViolBlockCoverageMissing, MissingBlockKind: types.BlockDiagram, Detail: "diagram block missing"},
		{Kind: types.ViolMustInclude, Detail: "required literal missing"},
	}
	out := FilterFinalizerRetryRootViolationsForBus(in, bus)
	if len(out) != 3 {
		t.Fatalf("exact scalar / requested-shape / must_include defects must stay actionable, got %+v", out)
	}
}

func TestRenderViolations_SuppressesSoftTelemetryBesideHardFailure(t *testing.T) {
	t.Cleanup(func() { SetSoftViolationKinds(nil, nil) })
	SetSoftViolationKinds(nil, []string{string(types.ViolSuccessCriterion)})

	res := stubResultWithViolations(
		types.Violation{Kind: types.ViolRichnessRegression, Detail: "optional richness telemetry"},
		types.Violation{Kind: types.ViolDiagramRelationLabelOnly, Detail: "label-only diagram telemetry"},
		types.Violation{Kind: types.ViolSuccessCriterion, Detail: "citation count failed"},
	)
	got := renderViolations(res)
	if got == "" {
		t.Fatal("hard success criterion should still render")
	}
	if strings.Contains(got, "richness") || strings.Contains(got, "label-only") {
		t.Fatalf("soft telemetry leaked into retry hint: %q", got)
	}
	if !strings.Contains(got, "citation count failed") {
		t.Fatalf("hard violation missing from retry hint: %q", got)
	}
}

// stubResultWithViolations is a small test helper. We cannot import
// contract package here directly because of cycle constraints — so
// we go through the type-aliased contract.Violation.
func stubResultWithViolations(vs ...types.Violation) contractResultStub {
	return contractResultStub{Passed: false, Violations: vs}
}

// contractResultStub has the same fields as contract.Result needed
// by renderViolations. Because Result has unexported fields, we
// emulate the public surface needed.
type contractResultStub = contract.Result

// TestIncrementAttemptsAndCheckExhausted_PinsCrossScopeBudget pins
// W2.6: same root violation across N attempts triggers exhaustion
// after MaxRepairAttemptsPerRoot regardless of which scope the
// retries came from.
func TestIncrementAttemptsAndCheckExhausted_PinsCrossScopeBudget(t *testing.T) {
	hist := types.NewRepairAttemptHistory()
	root := []types.Violation{{
		Kind:          types.ViolDiagramEdgeUnsupported,
		Detail:        "block id=\"d1\" edge missing typed anchor",
		SuspectedRoot: types.SuspectedRoot{IRField: "diagram_edges"},
	}}
	for i := 0; i < types.MaxRepairAttemptsPerRoot-1; i++ {
		if exhausted := IncrementAttemptsAndCheckExhausted(root, hist); exhausted {
			t.Fatalf("attempt %d: exhausted=true earlier than MaxRepairAttemptsPerRoot=%d",
				i+1, types.MaxRepairAttemptsPerRoot)
		}
	}
	if !IncrementAttemptsAndCheckExhausted(root, hist) {
		t.Fatalf("attempt %d: exhausted=false but should be true", types.MaxRepairAttemptsPerRoot)
	}
}

// TestIncrementAttemptsAndCheckExhausted_DerivedSkipped — derived
// violations don't count toward the per-root budget. The root carries
// the budget; derivations are noise the LLM should not be asked to
// fix.
func TestIncrementAttemptsAndCheckExhausted_DerivedSkipped(t *testing.T) {
	hist := types.NewRepairAttemptHistory()
	mixed := []types.Violation{
		{Kind: types.ViolDiagramEdgeUnsupported, SuspectedRoot: types.SuspectedRoot{IRField: "diagram_edges"}},
		{
			Kind:      types.ViolDiagramEdgeLabelMismatch,
			IsDerived: true,
			RootKind:  types.ViolDiagramEdgeUnsupported,
		},
	}
	IncrementAttemptsAndCheckExhausted(mixed, hist)
	rootKey := types.FpKey{
		Kind:        types.ViolDiagramEdgeUnsupported,
		Fingerprint: clusterFingerprintOf(mixed[0]),
	}
	derivedKey := types.FpKey{
		Kind:        types.ViolDiagramEdgeLabelMismatch,
		Fingerprint: clusterFingerprintOf(mixed[1]),
	}
	if got := hist.Get(rootKey); got != 1 {
		t.Errorf("root attempt count = %d, want 1", got)
	}
	if got := hist.Get(derivedKey); got != 0 {
		t.Errorf("derived attempt count = %d, want 0 (must not be incremented)", got)
	}
}

// TestIncrementAttemptsAndCheckExhausted_NilHistoryPassthrough —
// pre-W2 callers without a history (nil) must not panic and must
// return false so legacy retry budget continues to gate.
func TestIncrementAttemptsAndCheckExhausted_NilHistoryPassthrough(t *testing.T) {
	violations := []types.Violation{{Kind: types.ViolDiagramEdgeUnsupported}}
	if got := IncrementAttemptsAndCheckExhausted(violations, nil); got {
		t.Errorf("nil history should return false, got true")
	}
}

// TestComputeRootCauseClosure_EmptyInput — nil/empty in -> nil out.
func TestComputeRootCauseClosure_EmptyInput(t *testing.T) {
	if got := ComputeRootCauseClosure(nil); got != nil {
		t.Errorf("nil input should give nil output, got %v", got)
	}
	if got := ComputeRootCauseClosure([]types.Violation{}); got != nil {
		t.Errorf("empty input should give nil output, got %v", got)
	}
}
