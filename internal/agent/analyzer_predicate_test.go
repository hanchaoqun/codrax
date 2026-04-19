package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestReconcilePredicateAxis: schema-v4 reconcile is a pass-through.
// The LLM emits the axis directly via emit_analysis.predicate_axis;
// no Go-side extraction or override happens here.
func TestReconcilePredicateAxis(t *testing.T) {
	cases := []struct {
		name     string
		declared types.PredicateAxis
	}{
		{"AxisCall passes through", types.AxisCall},
		{"AxisRegister passes through", types.AxisRegister},
		{"AxisDefine passes through", types.AxisDefine},
		{"AxisReturn passes through", types.AxisReturn},
		{"AxisConfigure passes through", types.AxisConfigure},
		{"AxisCondition passes through", types.AxisCondition},
		{"AxisImplement passes through", types.AxisImplement},
		{"AxisUnknown passes through (no override on empty)", types.AxisUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, reason := reconcilePredicateAxis(c.declared)
			if got != c.declared {
				t.Errorf("got %q, want %q (declared preserved)", got, c.declared)
			}
			if reason != "" {
				t.Errorf("expected empty reason on pass-through, got %q", reason)
			}
		})
	}
}

func TestPredicateAxisEnumValidity(t *testing.T) {
	axes := types.AllPredicateAxes()
	if len(axes) == 0 {
		t.Fatal("AllPredicateAxes returned empty slice")
	}
	for _, a := range axes {
		if !a.IsValid() {
			t.Errorf("declared axis %q failed IsValid", a)
		}
	}
	if !types.AxisUnknown.IsValid() {
		t.Error("AxisUnknown should be valid (explicit zero sentinel)")
	}
	if types.PredicateAxis("bogus").IsValid() {
		t.Error("bogus axis should not be valid")
	}
}
