package types

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"
)

func proofPersistenceFixture() *ChangePlan {
	return &ChangePlan{
		ID: "proof-persistence", Status: PlanStatusNoChangeRequired,
		TargetPaths:        []string{"src/widget.ts"},
		VerificationProbes: []VerificationProbe{{ID: "behavior", Language: "javascript", Code: "if (!true) throw new Error('failed')"}},
	}
}

func TestProofPlanPersistenceSurvivesVerificationLifecycle(t *testing.T) {
	for _, status := range []string{PlanStatusApplied, PlanStatusUnverified, PlanStatusVerifyFailed} {
		t.Run(status, func(t *testing.T) {
			plan := proofPersistenceFixture()
			// The legacy producer shape is accepted; changing its lifecycle must
			// not destroy the identity that made that same payload loadable.
			path := seedPlanFile(t, t.TempDir(), plan)
			fingerprint := PlanFingerprint(plan)
			if err := UpdatePlanStatusOnDisk(path, status, nil, "preserved-worktree"); err != nil {
				t.Fatal(err)
			}
			loaded, err := LoadChangePlanFromFile(path)
			if err != nil {
				t.Fatalf("verification lifecycle erased proof-only identity: %v", err)
			}
			if loaded.Status != status || loaded.WorktreePath != "preserved-worktree" || len(loaded.Changes) != 0 ||
				!reflect.DeepEqual(loaded.TargetPaths, plan.TargetPaths) || !reflect.DeepEqual(loaded.VerificationProbes, plan.VerificationProbes) ||
				PlanFingerprint(loaded) != fingerprint {
				t.Fatalf("proof payload/lifecycle drift: %+v", loaded)
			}
			if err := UpdatePlanStatusOnDisk(path, PlanStatusUnverified, nil, ""); err != nil {
				t.Fatalf("second lifecycle persist: %v", err)
			}
			if err := PersistAppliedRecoveryOnDisk(path, "existing-worktree-checkpoint"); err != nil {
				t.Fatalf("recovery metadata persist: %v", err)
			}
			if err := WriteBestPlanReportPair(loaded, &ChangeReport{}, path); err != nil {
				t.Fatal(err)
			}
			best, _, err := LoadBestPlanReportPair(path)
			if err != nil || best == nil {
				t.Fatalf("best pair reload: plan=%v err=%v", best, err)
			}
			bestPath := filepath.Join(filepath.Dir(path), "best-restored.json")
			if err := WritePlanToFile(best, bestPath); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadChangePlanFromFile(bestPath); err != nil {
				t.Fatalf("best pair lost durable identity: %v", err)
			}
		})
	}
}

func TestProofPlanPersistenceDoesNotInferTransitionedLegacyIdentity(t *testing.T) {
	for _, status := range []string{PlanStatusApplied, PlanStatusUnverified, PlanStatusVerifyFailed, PlanStatusMerged, PlanStatusRejected} {
		plan := proofPersistenceFixture()
		plan.Status = status
		path := seedPlanFile(t, t.TempDir(), plan)
		if _, err := LoadChangePlanFromFile(path); err == nil {
			t.Fatalf("unmarked historical %s must not acquire proof-only identity", status)
		}
	}
}

func TestProofPlanPersistenceStrictPayloadAndLifecycle(t *testing.T) {
	mutations := map[string]func(*ChangePlan){
		"no-target":          func(p *ChangePlan) { p.TargetPaths = nil },
		"blank-target":       func(p *ChangePlan) { p.TargetPaths = []string{" "} },
		"duplicate-target":   func(p *ChangePlan) { p.TargetPaths = append(p.TargetPaths, " src/widget.ts ") },
		"no-probes":          func(p *ChangePlan) { p.VerificationProbes = nil },
		"missing-id":         func(p *ChangePlan) { p.VerificationProbes[0].ID = " " },
		"missing-language":   func(p *ChangePlan) { p.VerificationProbes[0].Language = " " },
		"missing-code":       func(p *ChangePlan) { p.VerificationProbes[0].Code = " " },
		"project-test-claim": func(p *ChangePlan) { p.ProjectTestObservations = []ProjectTestObservation{{}} },
		"unknown-kind":       func(p *ChangePlan) { p.PersistenceKind = "future-kind" },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			plan := proofPersistenceFixture()
			plan.PersistenceKind = PlanPersistenceProofProbeOnly
			mutate(plan)
			path := seedPlanFile(t, t.TempDir(), plan)
			if _, err := LoadChangePlanFromFile(path); err == nil {
				t.Fatal("marker must not bypass the exact proof payload shape")
			}
		})
	}
	for _, status := range []string{"", "future", PlanStatusPending, PlanStatusAppliedPendingVerify, PlanStatusApplyFailed, PlanStatusPartiallyApplied, PlanStatusBlocked} {
		plan := proofPersistenceFixture()
		plan.PersistenceKind, plan.Status = PlanPersistenceProofProbeOnly, status
		if _, err := LoadChangePlanFromFile(seedPlanFile(t, t.TempDir(), plan)); err == nil {
			t.Fatalf("marker must not permit empty apply/unknown state %q", status)
		}
	}
	for _, status := range []string{PlanStatusNoChangeRequired, PlanStatusApplied, PlanStatusUnverified, PlanStatusVerifyFailed, PlanStatusMerged, PlanStatusRejected} {
		plan := proofPersistenceFixture()
		plan.PersistenceKind, plan.Status = PlanPersistenceProofProbeOnly, status
		if _, err := LoadChangePlanFromFile(seedPlanFile(t, t.TempDir(), plan)); err != nil {
			t.Fatalf("valid durable proof state %q: %v", status, err)
		}
	}
}

func TestProofPlanPersistenceWriterDoesNotMutateOrChangeApprovalFingerprint(t *testing.T) {
	for _, ordinary := range []bool{false, true} {
		plan := proofPersistenceFixture()
		if ordinary {
			plan.Status = PlanStatusPending
			plan.Changes = []FileChange{{Path: "src/widget.ts", Kind: "modify", NewContent: "exact bytes"}}
		}
		before, _ := json.Marshal(plan)
		fingerprint := PlanFingerprint(plan)
		path := filepath.Join(t.TempDir(), "plan.json")
		if err := WritePlanToFile(plan, path); err != nil {
			t.Fatal(err)
		}
		after, _ := json.Marshal(plan)
		if string(before) != string(after) {
			t.Fatal("writer mutated caller snapshot")
		}
		loaded, err := LoadChangePlanFromFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if PlanFingerprint(loaded) != fingerprint {
			t.Fatal("persistence metadata changed approval fingerprint")
		}
		if ordinary && loaded.PersistenceKind != "" {
			t.Fatal("ordinary plan acquired proof identity")
		}
		if !ordinary && loaded.PersistenceKind != PlanPersistenceProofProbeOnly {
			t.Fatal("strict legacy sentinel was not preserved")
		}
	}
}
