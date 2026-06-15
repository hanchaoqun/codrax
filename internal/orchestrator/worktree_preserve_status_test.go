package orchestrator

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPlanStatusForWorktreePreserveKeepsUnverified(t *testing.T) {
	now := time.Date(2026, 6, 15, 11, 40, 0, 0, time.UTC)
	appliedAt := now.Add(-time.Minute)
	status, gotAppliedAt := planStatusForWorktreePreserve(&types.ChangePlan{
		Status:    types.PlanStatusUnverified,
		AppliedAt: &appliedAt,
	}, now)
	if status != types.PlanStatusUnverified {
		t.Fatalf("status = %q, want %q", status, types.PlanStatusUnverified)
	}
	if gotAppliedAt == nil || !gotAppliedAt.Equal(appliedAt) {
		t.Fatalf("appliedAt = %v, want %v", gotAppliedAt, appliedAt)
	}
}

func TestPlanStatusForWorktreePreserveDefaultsApplied(t *testing.T) {
	now := time.Date(2026, 6, 15, 11, 40, 0, 0, time.UTC)
	status, appliedAt := planStatusForWorktreePreserve(&types.ChangePlan{}, now)
	if status != types.PlanStatusApplied {
		t.Fatalf("status = %q, want %q", status, types.PlanStatusApplied)
	}
	if appliedAt == nil || !appliedAt.Equal(now) {
		t.Fatalf("appliedAt = %v, want %v", appliedAt, now)
	}
}
