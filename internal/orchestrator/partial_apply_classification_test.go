package orchestrator

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestClassifyApplyFailureStatus pins the partial-vs-fail
// distinction. When apply produces SOME successful units before
// failure, the status is partially_applied; when it produces ZERO,
// the status is applied_failed.
func TestClassifyApplyFailureStatus(t *testing.T) {
	cases := []struct {
		name        string
		targetPaths []string
		applied     []string // entries to mark applied via WriteClosure
		want        string
	}{
		{
			name:        "zero applied → applied_failed",
			targetPaths: []string{"a.go", "b.go", "c.go"},
			applied:     nil,
			want:        types.PlanStatusApplyFailed,
		},
		{
			name:        "one of three applied → partially_applied",
			targetPaths: []string{"a.go", "b.go", "c.go"},
			applied:     []string{"a.go"},
			want:        types.PlanStatusPartiallyApplied,
		},
		{
			name:        "two of three applied → partially_applied",
			targetPaths: []string{"a.go", "b.go", "c.go"},
			applied:     []string{"a.go", "b.go"},
			want:        types.PlanStatusPartiallyApplied,
		},
		{
			name:        "all applied (post-apply failure) → applied_failed defensive fallback",
			targetPaths: []string{"a.go"},
			applied:     []string{"a.go"},
			// When AppliedSet covers EVERY target but err is set,
			// the failure happened post-apply (commit-checkpoint
			// or similar). applied_failed is the conservative
			// fallback so /merge stays blocked until the operator
			// investigates.
			want: types.PlanStatusApplyFailed,
		},
		{
			name:        "applied set has unrelated paths → those don't count → applied_failed",
			targetPaths: []string{"a.go", "b.go"},
			applied:     []string{"unrelated.go"}, // not in target_paths
			want:        types.PlanStatusApplyFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mu := types.NewMutableState("test")
			mu.SetChangePlan(&types.ChangePlan{
				ID:          "plan-x",
				TargetPaths: tc.targetPaths,
			})
			wc := mu.WriteClosure()
			for _, p := range tc.applied {
				wc.MarkApplied(p)
			}
			busCtx := &types.BusContext{Mutable: mu}
			got := classifyApplyFailureStatus(busCtx)
			if got != tc.want {
				t.Errorf("classifyApplyFailureStatus = %q; want %q (applied=%v targets=%v)",
					got, tc.want, tc.applied, tc.targetPaths)
			}
		})
	}
}

// TestClassifyApplyFailureStatus_NilSafe pins defensive paths.
func TestClassifyApplyFailureStatus_NilSafe(t *testing.T) {
	if got := classifyApplyFailureStatus(nil); got != types.PlanStatusApplyFailed {
		t.Errorf("nil bus → applied_failed; got %q", got)
	}
	if got := classifyApplyFailureStatus(&types.BusContext{}); got != types.PlanStatusApplyFailed {
		t.Errorf("nil mutable → applied_failed; got %q", got)
	}
	mu := types.NewMutableState("test")
	if got := classifyApplyFailureStatus(&types.BusContext{Mutable: mu}); got != types.PlanStatusApplyFailed {
		t.Errorf("no plan → applied_failed; got %q", got)
	}
}
