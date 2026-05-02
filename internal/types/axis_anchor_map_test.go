package types

import "testing"

// TestPredicateAxisToAnchorKinds_AllAxesCovered pins the structural
// completeness rule: every declared PredicateAxis (excluding
// AxisUnknown) MUST have at least one allowed AnchorKind. A new
// PredicateAxis constant added without updating the map fails this
// test loud.
func TestPredicateAxisToAnchorKinds_AllAxesCovered(t *testing.T) {
	for _, axis := range AllPredicateAxes() {
		got := PredicateAxisToAnchorKinds(axis)
		if len(got) == 0 {
			t.Errorf("PredicateAxis %q has no AnchorKind mapping (Block 2 requires every axis to map to ≥1)", axis)
		}
	}
}

func TestPredicateAxisToAnchorKinds_KnownPairs(t *testing.T) {
	cases := []struct {
		axis PredicateAxis
		want AnchorKind
	}{
		{AxisCall, AnchorCall},
		{AxisDefine, AnchorDefinition},
		{AxisReturn, AnchorReturn},
		{AxisCondition, AnchorCondition},
		{AxisImplement, AnchorDefinition},
	}
	for _, tc := range cases {
		got := PredicateAxisToAnchorKinds(tc.axis)
		found := false
		for _, k := range got {
			if k == tc.want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("PredicateAxisToAnchorKinds(%q) = %v, want includes %q", tc.axis, got, tc.want)
		}
	}
}

func TestPredicateAxisToAnchorKinds_RegisterAcceptsCallAndAssignment(t *testing.T) {
	got := PredicateAxisToAnchorKinds(AxisRegister)
	hasCall := false
	hasAssign := false
	for _, k := range got {
		if k == AnchorCall {
			hasCall = true
		}
		if k == AnchorAssignment {
			hasAssign = true
		}
	}
	if !hasCall || !hasAssign {
		t.Errorf("AxisRegister allowed set = %v; want both AnchorCall and AnchorAssignment", got)
	}
}

func TestPredicateAxisToAnchorKinds_ConfigureAcceptsAssignmentAndDefinition(t *testing.T) {
	got := PredicateAxisToAnchorKinds(AxisConfigure)
	hasAssign := false
	hasDef := false
	for _, k := range got {
		if k == AnchorAssignment {
			hasAssign = true
		}
		if k == AnchorDefinition {
			hasDef = true
		}
	}
	if !hasAssign || !hasDef {
		t.Errorf("AxisConfigure allowed set = %v; want both AnchorAssignment and AnchorDefinition", got)
	}
}

func TestPredicateAxisToAnchorKinds_UnknownReturnsNil(t *testing.T) {
	if got := PredicateAxisToAnchorKinds(AxisUnknown); got != nil {
		t.Errorf("AxisUnknown should map to nil; got %v", got)
	}
}

func TestPredicateAxisHasMatchingAnchor_HitMissUnknown(t *testing.T) {
	cases := []struct {
		name string
		axis PredicateAxis
		item EvidenceItem
		want bool
	}{
		{
			name: "AxisCall + AnchorCall -> match",
			axis: AxisCall,
			item: EvidenceItem{AnchorKind: AnchorCall},
			want: true,
		},
		{
			name: "AxisCall + AnchorDefinition -> miss",
			axis: AxisCall,
			item: EvidenceItem{AnchorKind: AnchorDefinition},
			want: false,
		},
		{
			name: "AxisRegister + AnchorAssignment -> match (registration via map literal)",
			axis: AxisRegister,
			item: EvidenceItem{AnchorKind: AnchorAssignment},
			want: true,
		},
		{
			name: "AxisRegister + AnchorCall -> match (registry method call)",
			axis: AxisRegister,
			item: EvidenceItem{AnchorKind: AnchorCall},
			want: true,
		},
		{
			name: "AxisUnknown -> always miss",
			axis: AxisUnknown,
			item: EvidenceItem{AnchorKind: AnchorCall},
			want: false,
		},
		{
			name: "empty AnchorKind -> always miss",
			axis: AxisCall,
			item: EvidenceItem{},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := PredicateAxisHasMatchingAnchor(tc.axis, tc.item); got != tc.want {
				t.Errorf("PredicateAxisHasMatchingAnchor() = %v, want %v", got, tc.want)
			}
		})
	}
}
