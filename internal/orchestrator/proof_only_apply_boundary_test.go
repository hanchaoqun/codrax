package orchestrator

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

func b676ProofOnlyApplyPlan() *types.ChangePlan {
	return &types.ChangePlan{
		ID:          "proof-apply-boundary",
		Status:      types.PlanStatusNoChangeRequired,
		Summary:     "Observe the existing implementation without changing it",
		TargetPaths: []string{"subject.py"},
		VerificationProbes: []types.VerificationProbe{{
			ID: "observe", Language: "python", Code: "assert True",
		}},
	}
}

func b676ApplyBoundaryFixture(t *testing.T, plan *types.ChangePlan, imported bool) (*Orchestrator, string, []byte, []byte) {
	t.Helper()
	root := t.TempDir()
	source := []byte("def subject():\n    return 7\n")
	if err := os.WriteFile(filepath.Join(root, "subject.py"), source, 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "saved-plan.json")
	// Write the exact input snapshot, including legacy unmarked JSON. The
	// public loader below must establish identity before apply is attempted.
	planBytes, err := json.MarshalIndent(plan, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, planBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	o := writeApprovalApplyPreFixture(path, writeflow.ApprovalPolicyAutoSafe)
	o.busCtx.MainRepoRoot = root
	o.busCtx.RepoRoot = root
	if !imported {
		o.busCtx.Mutable.SetChangePlan(plan)
	}
	return o, path, planBytes, source
}

func b676AssertApplyBoundaryUnchanged(t *testing.T, o *Orchestrator, planPath string, planBytes, source []byte) {
	t.Helper()
	after, err := os.ReadFile(planPath)
	if err != nil || !bytes.Equal(after, planBytes) {
		t.Errorf("empty apply must not rewrite the saved plan or approval: read=%v before=%s after=%s", err, planBytes, after)
	}
	afterSource, err := os.ReadFile(filepath.Join(o.busCtx.MainRepoRoot, "subject.py"))
	if err != nil || !bytes.Equal(afterSource, source) {
		t.Errorf("empty apply changed source bytes: read=%v got=%q", err, afterSource)
	}
	if o.busCtx.WorktreePath != "" || o.busCtx.RepoRoot != o.busCtx.MainRepoRoot {
		t.Errorf("empty apply provisioned/swapped a worktree: %+v", o.busCtx.WorktreePath)
	}
	if _, err := os.Stat(filepath.Join(o.busCtx.MainRepoRoot, ".git")); !os.IsNotExist(err) {
		t.Errorf("empty apply unexpectedly initialized a repository: %v", err)
	}
}

// The legacy sentinel is already accepted by the real durable loader. This
// regression therefore exercises the apply hook, not a loader refusal.
func TestB676ProofOnlyApplyLegacySentinelCannotEnterCoder(t *testing.T) {
	for _, entry := range []string{"public_hook", "controller"} {
		for _, imported := range []bool{false, true} {
			name := entry + "/mutable"
			if imported {
				name = entry + "/imported"
			}
			t.Run(name, func(t *testing.T) {
				o, path, planBytes, source := b676ApplyBoundaryFixture(t, b676ProofOnlyApplyPlan(), imported)
				if loaded, err := types.LoadChangePlanFromFile(path); err != nil || loaded == nil {
					t.Fatalf("premise: legacy proof sentinel must be loadable: %v", err)
				}
				var err error
				steps := 0
				if entry == "controller" {
					err = o.runControllerApplyPlan(&steps)
				} else {
					err = runStagePreHook(o, types.StageApply)
				}
				if err == nil || !strings.Contains(err.Error(), "plan has no file changes") {
					t.Errorf("empty plan must stop at the apply-payload boundary before approval/coder, got %v", err)
				}
				if steps != 0 || o.busCtx.ActiveAgent != "" {
					t.Errorf("empty plan reached a coder dispatch: steps=%d agent=%q", steps, o.busCtx.ActiveAgent)
				}
				got := o.busCtx.Mutable.ChangePlan()
				if got == nil || got.Approval != nil || got.Status != types.PlanStatusNoChangeRequired {
					t.Errorf("apply refusal must not grant approval or rewrite lifecycle: %+v", got)
				}
				b676AssertApplyBoundaryUnchanged(t, o, path, planBytes, source)
			})
		}
	}
}

func TestB676EmptyApplyDoesNotDescribeMalformedPlanAsProof(t *testing.T) {
	plan := &types.ChangePlan{ID: "ordinary-empty", Status: types.PlanStatusPending, TargetPaths: []string{"subject.py"}}
	o, path, planBytes, source := b676ApplyBoundaryFixture(t, plan, false)
	before, err := json.Marshal(plan)
	if err != nil {
		t.Fatal(err)
	}
	err = runStagePreHook(o, types.StageApply)
	if err == nil || !strings.Contains(err.Error(), "plan has no file changes") {
		t.Errorf("ordinary empty payload must also stop before apply: %v", err)
	}
	if err != nil && (strings.Contains(err.Error(), "--write-phase=verify") || strings.Contains(err.Error(), "proof-only")) {
		t.Errorf("malformed ordinary plan must not be described as a valid proof plan: %v", err)
	}
	after, marshalErr := json.Marshal(o.busCtx.Mutable.ChangePlan())
	if marshalErr != nil || !bytes.Equal(before, after) {
		t.Errorf("ordinary empty refusal changed plan: marshal=%v before=%s after=%s", marshalErr, before, after)
	}
	b676AssertApplyBoundaryUnchanged(t, o, path, planBytes, source)
}

func TestB676PersistedProofIdentityDoesNotGrantApplyAcrossLifecycle(t *testing.T) {
	for _, status := range []string{
		types.PlanStatusNoChangeRequired, types.PlanStatusApplied,
		types.PlanStatusUnverified, types.PlanStatusVerifyFailed,
		types.PlanStatusMerged, types.PlanStatusRejected,
	} {
		for _, imported := range []bool{false, true} {
			for _, approved := range []bool{false, true} {
				name := status + "/mutable"
				if imported {
					name = status + "/imported_controller"
				}
				if approved {
					name += "/approved"
				}
				t.Run(name, func(t *testing.T) {
					plan := b676ProofOnlyApplyPlan()
					types.PreserveProofProbeOnlyPlanIdentity(plan)
					plan.Status = status
					if !types.IsPersistedProofProbeOnlyPlan(plan) {
						t.Fatal("premise: known strict proof payload must remain readable across lifecycle")
					}
					if approved {
						assessment := writeflow.AssessWriteRisk(writeflow.AssessmentInput{Plan: plan})
						decision := writeflow.DecideWriteApproval(writeflow.ApprovalPolicyManual, assessment)
						plan.Approval = writeflow.NewApprovalRecord(assessment, decision, "test", "approved", types.PlanFingerprint(plan), "")
					}
					o, path, planBytes, source := b676ApplyBoundaryFixture(t, plan, imported)
					if approved {
						o.writeApprovalPolicy = writeflow.ApprovalPolicyManual
					}
					if imported {
						run := o.seedWriteWorkflowRun()
						if len(run.Batches) != 1 || run.Batches[0].PlanID != plan.ID {
							t.Fatalf("premise: actual import did not seed the persisted plan: %+v", run.Batches)
						}
					}
					before, err := json.Marshal(o.busCtx.Mutable.ChangePlan())
					if err != nil {
						t.Fatal(err)
					}
					fingerprint := types.PlanFingerprint(o.busCtx.Mutable.ChangePlan())
					steps := 0
					if imported {
						err = o.runControllerApplyPlan(&steps)
					} else {
						err = runStagePreHook(o, types.StageApply)
					}
					if err == nil || !strings.Contains(err.Error(), "plan has no file changes") || !strings.Contains(err.Error(), "--write-phase=verify") {
						t.Fatalf("readable proof payload must refuse apply with proof-specific navigation: %v", err)
					}
					if steps != 0 || o.busCtx.ActiveAgent != "" {
						t.Fatalf("imported proof payload reached coder: steps=%d agent=%q", steps, o.busCtx.ActiveAgent)
					}
					after, err := json.Marshal(o.busCtx.Mutable.ChangePlan())
					if err != nil || !bytes.Equal(before, after) || fingerprint != types.PlanFingerprint(o.busCtx.Mutable.ChangePlan()) {
						t.Errorf("apply refusal changed approval, status, payload or fingerprint: err=%v before=%s after=%s", err, before, after)
					}
					b676AssertApplyBoundaryUnchanged(t, o, path, planBytes, source)
				})
			}
		}
	}
}

func TestB676NonemptyApplyStillUsesExistingApprovalBoundary(t *testing.T) {
	for _, policy := range []writeflow.ApprovalPolicy{writeflow.ApprovalPolicyAutoSafe, writeflow.ApprovalPolicyManual} {
		t.Run(string(policy), func(t *testing.T) {
			plan := &types.ChangePlan{
				ID: "ordinary-source-change", Status: types.PlanStatusPending,
				TargetPaths: []string{"subject.py"},
				Changes:     []types.FileChange{{Path: "subject.py", Kind: "modify", NewContent: "def subject():\n    return 8\n"}},
			}
			o, _, _, _ := b676ApplyBoundaryFixture(t, plan, true)
			o.writeApprovalPolicy = policy
			err := runStagePreHook(o, types.StageApply)
			if err == nil || strings.Contains(err.Error(), "plan has no file changes") {
				t.Fatalf("ordinary payload did not retain its existing approval/worktree path: %v", err)
			}
			if policy == writeflow.ApprovalPolicyManual && !strings.Contains(err.Error(), "write approval required") {
				t.Fatalf("manual approval boundary changed: %v", err)
			}
			if policy == writeflow.ApprovalPolicyAutoSafe && !strings.Contains(err.Error(), "worktree base directory not configured") {
				t.Fatalf("auto-safe payload no longer reaches worktree provisioning: %v", err)
			}
			if got := o.busCtx.Mutable.ChangePlan(); got.Approval == nil || got.PersistenceKind != "" {
				t.Fatalf("ordinary approval or persistence identity changed: %+v", got)
			}
		})
	}
}

func TestB676EmptyApplyUnknownIdentityNeverGetsProofNavigation(t *testing.T) {
	for _, shape := range []string{"unknown_kind", "pending", "missing_probe", "unmarked_applied"} {
		t.Run(shape, func(t *testing.T) {
			plan := b676ProofOnlyApplyPlan()
			plan.PersistenceKind = types.PlanPersistenceProofProbeOnly
			switch shape {
			case "unknown_kind":
				plan.PersistenceKind = "future_kind"
			case "pending":
				plan.Status = types.PlanStatusPending
			case "missing_probe":
				plan.VerificationProbes = nil
			case "unmarked_applied":
				plan.Status, plan.PersistenceKind = types.PlanStatusApplied, ""
			}
			o, path, planBytes, source := b676ApplyBoundaryFixture(t, plan, false)
			before, err := json.Marshal(plan)
			if err != nil {
				t.Fatal(err)
			}
			err = runStagePreHook(o, types.StageApply)
			if err == nil || !strings.Contains(err.Error(), "plan has no file changes") ||
				strings.Contains(err.Error(), "--write-phase=verify") || strings.Contains(err.Error(), "proof-only") {
				t.Fatalf("unknown identity must refuse apply without inventing proof authority: %v", err)
			}
			after, marshalErr := json.Marshal(o.busCtx.Mutable.ChangePlan())
			if marshalErr != nil || !bytes.Equal(before, after) {
				t.Errorf("unknown identity changed during refusal: marshal=%v before=%s after=%s", marshalErr, before, after)
			}
			b676AssertApplyBoundaryUnchanged(t, o, path, planBytes, source)
		})
	}
}
