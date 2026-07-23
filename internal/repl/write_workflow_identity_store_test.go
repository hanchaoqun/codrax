package repl

import (
	"bytes"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// WFID-1 store-layer pins: FindActiveRunMatching filters candidates through
// the single-point identity gate; /workflow resume stamps the one-shot
// explicit authorization the orchestrator consumes.

func wfidStoreIdentity(root, goal string) *types.WriteWorkflowRepoIdentity {
	return &types.WriteWorkflowRepoIdentity{
		IdentitySchema:    types.WriteWorkflowRepoIdentitySchemaVersion,
		CanonicalRepoRoot: root,
		GoalHash:          types.WriteWorkflowGoalHash(goal),
	}
}

func TestFindActiveRunMatchingSkipsMismatchedIdentity(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	if _, err := store.Save(&types.WriteWorkflowRun{
		RunID:         "wf-repo-a",
		Goal:          "goal a",
		Identity:      wfidStoreIdentity("/repo/a", "goal a"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches:       []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	current := *wfidStoreIdentity("/repo/b", "goal b")
	run, skips, err := store.FindActiveRunMatching(current)
	if err != nil {
		t.Fatalf("FindActiveRunMatching: %v", err)
	}
	if run != nil {
		t.Fatalf("mismatched-identity run must not be returned, got %+v", run)
	}
	if len(skips) != 1 || skips[0].RunID != "wf-repo-a" ||
		skips[0].ReasonCode != types.WriteWorkflowIdentityReasonRepoRootMismatch {
		t.Fatalf("mismatch must surface as a typed skip, got %+v", skips)
	}

	// Legacy FindActiveRun keeps its identity-blind semantics for the
	// explicit REPL surfaces.
	legacyRun, err := store.FindActiveRun()
	if err != nil || legacyRun == nil || legacyRun.RunID != "wf-repo-a" {
		t.Fatalf("FindActiveRun must stay identity-blind for explicit surfaces, got %+v err=%v", legacyRun, err)
	}
}

func TestFindActiveRunMatchingReturnsMatchingRun(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	if _, err := store.Save(&types.WriteWorkflowRun{
		RunID:         "wf-match",
		Goal:          "goal b",
		Identity:      wfidStoreIdentity("/repo/b", "goal b"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches:       []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	run, skips, err := store.FindActiveRunMatching(*wfidStoreIdentity("/repo/b", "goal b"))
	if err != nil {
		t.Fatalf("FindActiveRunMatching: %v", err)
	}
	if run == nil || run.RunID != "wf-match" || len(skips) != 0 {
		t.Fatalf("exact-identity run must be returned without skips, got %+v skips=%+v", run, skips)
	}
}

func TestFindActiveRunMatchingHonorsExplicitAuthorization(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	if _, err := store.Save(&types.WriteWorkflowRun{
		RunID:               "wf-authorized-legacy",
		Goal:                "old goal",
		Status:              types.WriteWorkflowRunInProgress,
		ActiveBatchID:       "batch-1",
		ResumeAuthorization: types.WriteWorkflowResumeAuthorizationExplicit,
		Batches:             []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	run, _, err := store.FindActiveRunMatching(*wfidStoreIdentity("/repo/b", "new goal"))
	if err != nil {
		t.Fatalf("FindActiveRunMatching: %v", err)
	}
	if run == nil || run.RunID != "wf-authorized-legacy" ||
		run.ResumeAuthorization != types.WriteWorkflowResumeAuthorizationExplicit {
		t.Fatalf("explicitly authorized run must be returned despite identity mismatch, got %+v", run)
	}
}

func TestWorkflowResumeStampsExplicitAuthorization(t *testing.T) {
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(&types.WriteWorkflowRun{
		RunID:         "wf-stamp",
		Status:        types.WriteWorkflowRunPlanned,
		ActiveBatchID: "batch-1",
		Batches:       []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner:                stubRunner{},
		Out:                   out,
		RepoRoot:              "/tmp/repo",
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
	})

	r.handleWorkflowCmd("/workflow resume wf-stamp")

	loaded, err := workflowStore.Load("wf-stamp")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.ResumeAuthorization != types.WriteWorkflowResumeAuthorizationExplicit || loaded.ResumeAuthorizedAt.IsZero() {
		t.Fatalf("/workflow resume must stamp the one-shot explicit authorization, got %q at %v",
			loaded.ResumeAuthorization, loaded.ResumeAuthorizedAt)
	}
}
