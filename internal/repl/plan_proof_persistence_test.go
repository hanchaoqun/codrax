package repl

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestPlanStoreProofIdentitySurvivesLegacyLoadAndSettle(t *testing.T) {
	for _, status := range []string{types.PlanStatusMerged, types.PlanStatusRejected} {
		t.Run(status, func(t *testing.T) {
			dir := t.TempDir()
			store := NewPlanStore(dir)
			plan := &types.ChangePlan{ID: "proof-only", Status: types.PlanStatusNoChangeRequired,
				TargetPaths:        []string{"src/widget.ts"},
				VerificationProbes: []types.VerificationProbe{{ID: "proof", Language: "javascript", Code: "true"}}}
			path := filepath.Join(dir, plan.ID+".json")
			legacy, _ := json.Marshal(plan)
			if err := os.WriteFile(path, legacy, 0600); err != nil {
				t.Fatal(err)
			}
			loaded, err := store.Load(plan.ID)
			if err != nil || !types.IsPersistedProofProbeOnlyPlan(loaded) {
				t.Fatalf("legacy load lost identity: %v, %v", loaded, err)
			}
			if err := store.Settle(plan.ID, status, "user decision"); err != nil {
				t.Fatal(err)
			}
			settled, err := types.LoadChangePlanFromFile(path)
			if err != nil || settled.Status != status || types.PlanFingerprint(settled) != types.PlanFingerprint(plan) {
				t.Fatalf("settled plan must remain readable without payload change: %v, %v", settled, err)
			}
		})
	}
}
