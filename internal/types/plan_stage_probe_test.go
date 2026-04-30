package types

import "testing"

// plan_stage_probe_test.go pins Batch 4 Module E's MutableState
// channel: plan-stage dry-run probe reports stay separate from the
// authoritative ChangeReport so the verify→plan retry loop never
// reads a probe as a verify outcome.

func TestMutableState_AppendPlanStageProbeReport_AppendOrder(t *testing.T) {
	mu := NewMutableState("test")
	mu.AppendPlanStageProbeReport(&ChangeReport{PlanID: "probe-1"})
	mu.AppendPlanStageProbeReport(&ChangeReport{PlanID: "probe-2"})
	got := mu.PlanStageProbeReports()
	if len(got) != 2 {
		t.Fatalf("expected 2 probes; got %d", len(got))
	}
	if got[0].PlanID != "probe-1" || got[1].PlanID != "probe-2" {
		t.Errorf("append order broken; got %v / %v", got[0].PlanID, got[1].PlanID)
	}
}

func TestMutableState_AppendPlanStageProbeReport_NilSafe(t *testing.T) {
	mu := NewMutableState("test")
	mu.AppendPlanStageProbeReport(nil)
	if got := mu.PlanStageProbeReports(); len(got) != 0 {
		t.Errorf("nil append should be a no-op; got %d entries", len(got))
	}
}

func TestMutableState_PlanStageProbeReports_DoesNotAffectChangeReport(t *testing.T) {
	mu := NewMutableState("test")
	mu.AppendPlanStageProbeReport(&ChangeReport{PlanID: "probe-1"})
	if mu.ChangeReport() != nil {
		t.Errorf("probe append must not write to ChangeReport channel; got %+v", mu.ChangeReport())
	}
}

func TestMutableState_ResetPlanStageProbeReports(t *testing.T) {
	mu := NewMutableState("test")
	mu.AppendPlanStageProbeReport(&ChangeReport{PlanID: "probe-1"})
	mu.ResetPlanStageProbeReports()
	if got := mu.PlanStageProbeReports(); len(got) != 0 {
		t.Errorf("Reset should empty the slice; got %d entries", len(got))
	}
}
