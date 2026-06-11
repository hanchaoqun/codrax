package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func exactResolutionDoc(status types.AnswerExactResolutionStatus, anchor string) *types.AnswerDocumentV2 {
	return &types.AnswerDocumentV2{
		ExactResolution: &types.AnswerExactResolution{
			Status: status,
			Anchor: anchor,
		},
	}
}

// No-op surfaces: the validator must never fire when there is no
// exact-target declaration to check, when the absence validator owns
// the status, when the anchor is empty (legal for exact_match), or
// when the support pool is empty (set-side absence is noise).
func TestExactResolutionGroundingNoOps(t *testing.T) {
	if v := validateExactResolutionGrounding(nil, nil); v != nil {
		t.Fatalf("nil doc must no-op")
	}
	if v := validateExactResolutionGrounding(&types.AnswerDocumentV2{}, nil); v != nil {
		t.Fatalf("nil exact_resolution must no-op")
	}
	if v := validateExactResolutionGrounding(exactResolutionDoc(types.AnswerExactResolutionAbsent, "x"), nil); v != nil {
		t.Fatalf("status=absent is owned by validateAbsenceScopeBound")
	}
	if v := validateExactResolutionGrounding(exactResolutionDoc(types.AnswerExactResolutionExactMatch, ""), nil); v != nil {
		t.Fatalf("empty anchor is legal for exact_match")
	}
	// Anchor declared, but the document carries no typed anchors at
	// all (scalar answer): empty pool must not fire.
	if v := validateExactResolutionGrounding(exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget"), nil); v != nil {
		t.Fatalf("empty support pool must no-op, got %v", v)
	}
}

// Grounded anchors: each typed channel alone must satisfy the check.
func TestExactResolutionGroundingSupportChannels(t *testing.T) {
	// Channel 1: citation enclosing_function.
	doc := exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget")
	doc.Citations = []types.Citation{{File: "a.go", Line: 10, EnclosingFunction: "transientRetryBudget"}}
	if v := validateExactResolutionGrounding(doc, nil); v != nil {
		t.Fatalf("enclosing_function channel must ground the anchor, got %v", v)
	}

	// Channel 2: evidence-item anchor fields via MutableState.
	doc = exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget")
	mut := types.NewMutableState("t")
	mut.AppendEvidence([]types.EvidenceItem{{AnchorSymbol: "transientRetryBudget"}})
	if v := validateExactResolutionGrounding(doc, mut); v != nil {
		t.Fatalf("evidence anchor channel must ground the anchor, got %v", v)
	}

	// Channel 3: diagram edge endpoints.
	doc = exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget")
	doc.Blocks = []types.AnswerBlock{{
		EdgeAnchors: []types.DiagramEdgeAnchor{{FromNode: "transientRetryBudget", ToNode: "other"}},
	}}
	if v := validateExactResolutionGrounding(doc, nil); v != nil {
		t.Fatalf("edge anchor channel must ground the anchor, got %v", v)
	}
}

// Canonicalization: underscore/case variants and qualified anchors
// must match a pool that carries another spelling of the same symbol.
func TestExactResolutionGroundingCanonicalization(t *testing.T) {
	doc := exactResolutionDoc(types.AnswerExactResolutionAliasMatch, "transient_retry_budget")
	doc.Citations = []types.Citation{{File: "a.go", Line: 5, EnclosingFunction: "TransientRetryBudget"}}
	if v := validateExactResolutionGrounding(doc, nil); v != nil {
		t.Fatalf("NormalizeCodeKey variants must match, got %v", v)
	}

	doc = exactResolutionDoc(types.AnswerExactResolutionExactMatch, "pkg.TransientRetryBudget")
	doc.Citations = []types.Citation{{File: "a.go", Line: 5, EnclosingFunction: "transientRetryBudget"}}
	if v := validateExactResolutionGrounding(doc, nil); v != nil {
		t.Fatalf("qualified anchor must match on its terminal segment, got %v", v)
	}
}

// Sub-shape (a): a declared anchor missing from a NON-empty pool
// fires exactly one soft violation naming the anchor.
func TestExactResolutionGroundingUngrounded(t *testing.T) {
	doc := exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget")
	doc.Citations = []types.Citation{{File: "a.go", Line: 10, EnclosingFunction: "completelyDifferentFn"}}
	v := validateExactResolutionGrounding(doc, nil)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %v", v)
	}
	if v[0].Kind != types.ViolExactResolutionUngrounded {
		t.Fatalf("kind = %s", v[0].Kind)
	}
	if !strings.Contains(v[0].Detail, "transientRetryBudget") {
		t.Fatalf("detail must name the anchor: %q", v[0].Detail)
	}
	if v[0].SuspectedRoot.IRField != "exact_resolution.anchor" {
		t.Fatalf("IRField = %q", v[0].SuspectedRoot.IRField)
	}
}

// Sub-shape (b): a negative-scope citation whose negative_pattern
// names the SAME symbol contradicts the declaration — and wins even
// when the anchor is otherwise grounded.
func TestExactResolutionGroundingNegatedByAbsenceCitation(t *testing.T) {
	doc := exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget")
	doc.Citations = []types.Citation{
		{File: "a.go", Line: 10, EnclosingFunction: "transientRetryBudget"},
		{File: "b.go", Line: 1, Scope: types.ScopeNegative, NegativePattern: "transient_retry_budget"},
	}
	v := validateExactResolutionGrounding(doc, nil)
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %v", v)
	}
	if !strings.Contains(v[0].Detail, "simultaneously claims and disproves") {
		t.Fatalf("detail must describe the contradiction: %q", v[0].Detail)
	}
}

// A negative-scope citation about a DIFFERENT symbol is fine — it
// must not poison an otherwise grounded declaration.
func TestExactResolutionGroundingUnrelatedNegativeCitation(t *testing.T) {
	doc := exactResolutionDoc(types.AnswerExactResolutionExactMatch, "transientRetryBudget")
	doc.Citations = []types.Citation{
		{File: "a.go", Line: 10, EnclosingFunction: "transientRetryBudget"},
		{File: "b.go", Line: 1, Scope: types.ScopeNegative, NegativePattern: "someOtherKnob"},
	}
	if v := validateExactResolutionGrounding(doc, nil); v != nil {
		t.Fatalf("unrelated negative citation must not fire, got %v", v)
	}
}

// Registry contract: the kind must stay soft-by-default + promotable
// with finalizer locus — the support pool is complete only for
// answers that actually carry citations/evidence, so the default
// must never hard-loop the finalizer.
func TestExactResolutionUngroundedRegistrySpec(t *testing.T) {
	spec, ok := types.ViolKindSpecFor(types.ViolExactResolutionUngrounded)
	if !ok {
		t.Fatalf("kind not registered")
	}
	if !spec.SoftByDefault || !spec.Promotable {
		t.Fatalf("spec must be SoftByDefault+Promotable: %+v", spec)
	}
	if spec.FallbackLocus != types.LocusFinalizer {
		t.Fatalf("FallbackLocus = %v", spec.FallbackLocus)
	}
}
