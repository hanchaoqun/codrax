package axis

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestAffinityKnownCombos(t *testing.T) {
	cases := []struct {
		name string
		axis types.PredicateAxis
		kind types.AnchorKind
		want float64
	}{
		// AxisCall × * — the motivating row.
		{"call_vs_call_boost", types.AxisCall, types.AnchorCall, 1.5},
		{"call_vs_definition_demote", types.AxisCall, types.AnchorDefinition, 0.7},
		{"call_vs_return_mild_boost", types.AxisCall, types.AnchorReturn, 1.1},
		{"call_vs_assignment_demote", types.AxisCall, types.AnchorAssignment, 0.9},

		// AxisRegister × * — registration realised as call+assignment.
		{"register_vs_assignment_boost", types.AxisRegister, types.AnchorAssignment, 1.4},
		{"register_vs_call_mild_boost", types.AxisRegister, types.AnchorCall, 1.2},
		{"register_vs_definition_demote", types.AxisRegister, types.AnchorDefinition, 0.8},

		{"define_vs_definition_boost", types.AxisDefine, types.AnchorDefinition, 1.5},
		{"define_vs_call_demote", types.AxisDefine, types.AnchorCall, 0.8},

		{"return_vs_return_boost", types.AxisReturn, types.AnchorReturn, 1.6},

		{"condition_vs_condition_boost", types.AxisCondition, types.AnchorCondition, 1.5},

		{"configure_vs_assignment_boost", types.AxisConfigure, types.AnchorAssignment, 1.4},

		{"implement_vs_definition_boost", types.AxisImplement, types.AnchorDefinition, 1.3},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Affinity(tc.axis, tc.kind)
			if got != tc.want {
				t.Errorf("Affinity(%q, %q) = %v, want %v", tc.axis, tc.kind, got, tc.want)
			}
		})
	}
}

func TestAffinityUnknownAxisNeutral(t *testing.T) {
	// AxisUnknown disables every column → always 1.0.
	for _, kind := range types.AllAnchorKinds() {
		if got := Affinity(types.AxisUnknown, kind); got != 1.0 {
			t.Errorf("Affinity(AxisUnknown, %q) = %v, want 1.0", kind, got)
		}
	}
}

func TestAffinityEmptyKindNeutral(t *testing.T) {
	// Empty kind (LLM didn't emit one) must never demote.
	for _, axis := range types.AllPredicateAxes() {
		if got := Affinity(axis, ""); got != 1.0 {
			t.Errorf("Affinity(%q, '') = %v, want 1.0", axis, got)
		}
	}
}

func TestAffinityUnknownPairNeutral(t *testing.T) {
	// Unlisted pair (e.g. AxisReturn × AnchorCondition) → 1.0.
	if got := Affinity(types.AxisReturn, types.AnchorCondition); got != 1.0 {
		t.Errorf("Affinity(AxisReturn, AnchorCondition) = %v, want 1.0 (unlisted pair)", got)
	}
	if got := Affinity(types.AxisCall, types.AnchorImport); got != 1.0 {
		t.Errorf("Affinity(AxisCall, AnchorImport) = %v, want 1.0 (unlisted pair)", got)
	}
}

func TestAffinityBoundsDiscipline(t *testing.T) {
	// Curation rule: no boost above 1.6, no demote below 0.7.
	for _, axis := range types.AllPredicateAxes() {
		for _, kind := range types.AllAnchorKinds() {
			w := Affinity(axis, kind)
			if w > 1.6 {
				t.Errorf("Affinity(%q, %q) = %v exceeds boost ceiling 1.6", axis, kind, w)
			}
			if w < 0.7 {
				t.Errorf("Affinity(%q, %q) = %v below demote floor 0.7", axis, kind, w)
			}
		}
	}
}
