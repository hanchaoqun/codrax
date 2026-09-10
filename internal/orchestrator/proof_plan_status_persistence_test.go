package orchestrator

import (
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestProofPlanIdentityPreservedBeforeMutableStatusTransitions(t *testing.T) {
	for _, lane := range []string{"status", "status-with-applied", "verify"} {
		for _, status := range []string{types.PlanStatusApplied, types.PlanStatusUnverified, types.PlanStatusVerifyFailed} {
			t.Run(lane+"/"+status, func(t *testing.T) {
				plan := &types.ChangePlan{ID: "proof-only", Status: types.PlanStatusNoChangeRequired,
					TargetPaths:        []string{"src/widget.ts"},
					VerificationProbes: []types.VerificationProbe{{ID: "proof", Language: "javascript", Code: "true"}}}
				mu := types.NewMutableState("proof lifecycle")
				mu.SetChangePlan(plan)
				dir := t.TempDir()
				o := &Orchestrator{busCtx: &types.BusContext{Mutable: mu, WorkDir: dir}}
				fingerprint := types.PlanFingerprint(plan)
				switch lane {
				case "status":
					o.persistPlanStatus(status, nil)
				case "status-with-applied":
					o.persistPlanStatusWithApplied(status, nil, nil)
				case "verify":
					report := &types.ChangeReport{PlanID: plan.ID, Passed: status == types.PlanStatusApplied}
					if status == types.PlanStatusUnverified {
						report.FailureKind = types.FailureKindRunnerMissing
					}
					o.syncMutablePlanStatusAfterVerify(report, nil)
				}
				loaded, err := types.LoadChangePlanFromFile(filepath.Join(dir, "plans", plan.ID+".json"))
				if err != nil {
					t.Fatal(err)
				}
				if loaded.Status != status || !types.IsPersistedProofProbeOnlyPlan(loaded) || types.PlanFingerprint(loaded) != fingerprint {
					t.Fatalf("lifecycle/identity/payload mismatch: %+v", loaded)
				}
				if !types.IsPersistedProofProbeOnlyPlan(mu.ChangePlan()) {
					t.Fatal("mutable identity missing before snapshot")
				}
			})
		}
	}
}
