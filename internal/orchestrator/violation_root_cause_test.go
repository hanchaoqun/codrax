package orchestrator

import (
	"testing"

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

// TestComputeRootCauseClosure_EmptyInput — nil/empty in -> nil out.
func TestComputeRootCauseClosure_EmptyInput(t *testing.T) {
	if got := ComputeRootCauseClosure(nil); got != nil {
		t.Errorf("nil input should give nil output, got %v", got)
	}
	if got := ComputeRootCauseClosure([]types.Violation{}); got != nil {
		t.Errorf("empty input should give nil output, got %v", got)
	}
}
