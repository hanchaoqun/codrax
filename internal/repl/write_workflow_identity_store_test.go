package repl

import (
	"bytes"
	"strings"
	"testing"
	"time"

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
		RunID:         "wf-authorized-legacy",
		Goal:          "old goal",
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		// FIX-2: the one-shot token is root-bound — it authorizes only in
		// the repo context it was minted in ("/repo/b" here, matching the
		// current identity below).
		ResumeAuthorization:      types.WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedRepoRoot: "/repo/b",
		Batches:                  []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
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

// FIX-2 (token root binding, store lane): a token minted in another repo
// context never authorizes here — the run falls through to the identity gate
// and surfaces as a typed skip instead of being returned.
func TestFindActiveRunMatchingRejectsCrossContextAuthorization(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	if _, err := store.Save(&types.WriteWorkflowRun{
		RunID:                    "wf-foreign-token",
		Goal:                     "goal a",
		Identity:                 wfidStoreIdentity("/repo/a", "goal a"),
		Status:                   types.WriteWorkflowRunInProgress,
		ActiveBatchID:            "batch-1",
		ResumeAuthorization:      types.WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedRepoRoot: "/repo/a",
		Batches:                  []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}); err != nil {
		t.Fatalf("Save: %v", err)
	}
	run, skips, err := store.FindActiveRunMatching(*wfidStoreIdentity("/repo/b", "goal b"))
	if err != nil {
		t.Fatalf("FindActiveRunMatching: %v", err)
	}
	if run != nil {
		t.Fatalf("cross-context token must not authorize the run, got %+v", run)
	}
	if len(skips) != 1 || skips[0].RunID != "wf-foreign-token" ||
		skips[0].ReasonCode != types.WriteWorkflowIdentityReasonRepoRootMismatch {
		t.Fatalf("cross-context-token run must surface as a typed identity skip, got %+v", skips)
	}
}

// FIX-2 (one-shot made literal): ClearResumeAuthorizationsExcept clears and
// persists residual tokens on every run except the one consumed this turn.
func TestClearResumeAuthorizationsExceptSweepsResidualTokens(t *testing.T) {
	store := NewWriteWorkflowRunStore(t.TempDir())
	for _, id := range []string{"wf-consumed", "wf-residual"} {
		if _, err := store.Save(&types.WriteWorkflowRun{
			RunID:                    id,
			Status:                   types.WriteWorkflowRunInProgress,
			ActiveBatchID:            "batch-1",
			ResumeAuthorization:      types.WriteWorkflowResumeAuthorizationExplicit,
			ResumeAuthorizedRepoRoot: "/repo/a",
			ResumeAuthorizedAt:       time.Now(),
			Batches:                  []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
		}); err != nil {
			t.Fatalf("Save %s: %v", id, err)
		}
	}
	cleared, err := store.ClearResumeAuthorizationsExcept("wf-consumed")
	if err != nil {
		t.Fatalf("ClearResumeAuthorizationsExcept: %v", err)
	}
	if cleared != 1 {
		t.Fatalf("exactly the residual run must be swept, got %d", cleared)
	}
	residual, err := store.Load("wf-residual")
	if err != nil {
		t.Fatalf("Load residual: %v", err)
	}
	if residual.ResumeAuthorization != "" || residual.ResumeAuthorizedRepoRoot != "" || !residual.ResumeAuthorizedAt.IsZero() {
		t.Fatalf("residual token must be cleared and persisted, got %q root %q at %v",
			residual.ResumeAuthorization, residual.ResumeAuthorizedRepoRoot, residual.ResumeAuthorizedAt)
	}
	kept, err := store.Load("wf-consumed")
	if err != nil {
		t.Fatalf("Load consumed: %v", err)
	}
	if kept.ResumeAuthorization != types.WriteWorkflowResumeAuthorizationExplicit {
		t.Fatalf("the excepted run's token must stay untouched, got %+v", kept)
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
	// FIX-2: the token records the repo root it was minted in (no mint func
	// wired in this fixture → the trimmed raw root).
	if loaded.ResumeAuthorizedRepoRoot != "/tmp/repo" {
		t.Fatalf("/workflow resume must bind the token to the minting repo root, got %q", loaded.ResumeAuthorizedRepoRoot)
	}
}

// wfidResumeREPL builds a REPL whose identity mint returns a fixed canonical
// root, plus a store holding one active run. Used by the FIX-3 bare-resume
// gate pins.
func wfidResumeREPL(t *testing.T, run *types.WriteWorkflowRun, mintRoot string) (*REPL, *WriteWorkflowRunStore, *bytes.Buffer) {
	t.Helper()
	planStore := NewPlanStore(t.TempDir())
	workflowStore := NewWriteWorkflowRunStore(planStore.PlanDir())
	if _, err := workflowStore.Save(run); err != nil {
		t.Fatalf("Save workflow: %v", err)
	}
	out := &bytes.Buffer{}
	r := New(Config{
		Runner: stubRunner{},
		// Non-nil In forces line-oriented mode so r.info writes to Out and
		// the verdict wording is assertable.
		In:                    strings.NewReader(""),
		Out:                   out,
		RepoRoot:              mintRoot,
		Branch:                "main",
		Render:                renderNothing,
		PlanStore:             planStore,
		WriteWorkflowRunStore: workflowStore,
		Language:              "en",
		WriteWorkflowIdentityMint: func(repoRoot string) types.WriteWorkflowRepoIdentity {
			return types.WriteWorkflowRepoIdentity{
				IdentitySchema:    types.WriteWorkflowRepoIdentitySchemaVersion,
				CanonicalRepoRoot: mintRoot,
			}
		},
	})
	return r, workflowStore, out
}

// FIX-3: the bare /workflow resume form picks a run the user never named, so
// it must run the identity gate — a mismatching run is NOT stamped and the
// verdict names the run, the arm, and the explicit escape hatch.
func TestBareWorkflowResumeRefusesIdentityMismatch(t *testing.T) {
	r, workflowStore, out := wfidResumeREPL(t, &types.WriteWorkflowRun{
		RunID:         "wf-cross",
		Goal:          "goal a",
		Identity:      wfidStoreIdentity("/repo/a", "goal a"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches:       []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}, "/repo/b")

	r.handleWorkflowCmd("/workflow resume")

	loaded, err := workflowStore.Load("wf-cross")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.ResumeAuthorization != "" || loaded.ResumeAuthorizedRepoRoot != "" {
		t.Fatalf("bare resume must not stamp a token on identity mismatch, got %q root %q",
			loaded.ResumeAuthorization, loaded.ResumeAuthorizedRepoRoot)
	}
	text := out.String()
	if !bytes.Contains([]byte(text), []byte("wf-cross")) ||
		!bytes.Contains([]byte(text), []byte(types.WriteWorkflowIdentityReasonRepoRootMismatch)) ||
		!bytes.Contains([]byte(text), []byte("/workflow resume wf-cross")) {
		t.Fatalf("bare resume mismatch verdict must name the run, the arm, and the explicit escape, got %q", text)
	}
}

// FIX-3 (goal-arm projection): the bare form has no current write goal, so a
// same-repo run whose goal hash differs from anything current must still be
// resumable — only the repo-context arms gate this surface.
func TestBareWorkflowResumeMatchingContextStampsRootBoundToken(t *testing.T) {
	r, workflowStore, _ := wfidResumeREPL(t, &types.WriteWorkflowRun{
		RunID:         "wf-here",
		Goal:          "some past goal",
		Identity:      wfidStoreIdentity("/repo/b", "some past goal"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches:       []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}, "/repo/b")

	r.handleWorkflowCmd("/workflow resume")

	loaded, err := workflowStore.Load("wf-here")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.ResumeAuthorization != types.WriteWorkflowResumeAuthorizationExplicit ||
		loaded.ResumeAuthorizedRepoRoot != "/repo/b" {
		t.Fatalf("bare resume in the matching context must stamp the root-bound token, got %q root %q",
			loaded.ResumeAuthorization, loaded.ResumeAuthorizedRepoRoot)
	}
}

// FIX-3 (explicit form unchanged): naming the run IS the authorization — the
// explicit-ID form stamps even across contexts, recording the current
// canonical root on the token.
func TestExplicitWorkflowResumeCrossContextStillStamps(t *testing.T) {
	r, workflowStore, _ := wfidResumeREPL(t, &types.WriteWorkflowRun{
		RunID:         "wf-named",
		Goal:          "goal a",
		Identity:      wfidStoreIdentity("/repo/a", "goal a"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches:       []types.WriteWorkflowBatch{{ID: "batch-1", Status: types.WriteWorkflowBatchReadyToPlan}},
	}, "/repo/b")

	r.handleWorkflowCmd("/workflow resume wf-named")

	loaded, err := workflowStore.Load("wf-named")
	if err != nil {
		t.Fatalf("Load workflow: %v", err)
	}
	if loaded.ResumeAuthorization != types.WriteWorkflowResumeAuthorizationExplicit ||
		loaded.ResumeAuthorizedRepoRoot != "/repo/b" {
		t.Fatalf("explicit resume must stamp the root-bound token across contexts, got %q root %q",
			loaded.ResumeAuthorization, loaded.ResumeAuthorizedRepoRoot)
	}
}
