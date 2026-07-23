package orchestrator

// WFID-1 (§29.213 件1): write workflow auto-resume repo/task identity gate.
// The pins here drive production runWriteControllerWorkflow through
// loadOrSeedWriteWorkflowRun so the gate is exercised end to end: A→B
// cross-repo refusal, base-HEAD drift refusal, goal-change refusal, legacy
// (identity-less) refusal with the explicit one-shot resume escape, and the
// preserved happy path on a real git repo.

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/agent"
	"github.com/hanchaoqun/codrax/internal/skill"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/writeflow"
)

// fakeIdentityMatcherStore exercises the identity-aware finder lane
// (WriteWorkflowRunIdentityMatchedLoader) of loadOrSeedWriteWorkflowRun.
type fakeIdentityMatcherStore struct {
	fakeWorkflowRunStore
	matched *types.WriteWorkflowRun
	skips   []types.WriteWorkflowIdentitySkip
}

func (s *fakeIdentityMatcherStore) FindActiveRunMatching(types.WriteWorkflowRepoIdentity) (*types.WriteWorkflowRun, []types.WriteWorkflowIdentitySkip, error) {
	return s.matched, s.skips, nil
}

func wfidTestOrchestrator(t *testing.T, store WriteWorkflowRunSaver, request, seedSummary, repoRoot string, decisions []writeflow.WriteWorkflowDecision, controllerCalls *int) *Orchestrator {
	t.Helper()
	mu := types.NewMutableState(request)
	if seedSummary != "" {
		mu.SetWriteAnalysisIR(&types.WriteAnalysisIR{Request: types.WriteRequestModel{Task: types.WriteTask{Summary: seedSummary}}})
	}
	handler := func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error) {
		t.Fatalf("identity-gate refusal must stop before controller dispatch")
		return &agent.StageOutput{}, nil
	}
	if decisions != nil {
		handler = scriptedController(t, decisions, controllerCalls)
	}
	ar, sr, sar := buildRegistries(map[types.AgentName]func(*types.AgentContext, *skill.Config) (*agent.StageOutput, error){
		types.AgentWriteController: handler,
	})
	o := New(types.PipelineSettings{WriteWorkflowEngine: types.WriteWorkflowEngineController}, ar, sr, sar)
	o.busCtx = &types.BusContext{Mutable: mu, Mode: types.ModeApply, AnalysisIR: &types.AnalysisIR{}, RepoRoot: repoRoot}
	o.cancelToken = NewCancelToken()
	o.writeWorkflowRunStore = store
	return o
}

func wfidInitGitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git init unavailable: %v\n%s", err, out)
	}
	wfidGitCommit(t, repo, "init")
	return repo
}

func wfidGitCommit(t *testing.T, repo, msg string) {
	t.Helper()
	cmd := exec.Command("git", "-c", "user.email=t@t", "-c", "user.name=t",
		"commit", "--allow-empty", "-m", msg)
	cmd.Dir = repo
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git commit unavailable: %v\n%s", err, out)
	}
}

// Pin 1 (A→B adversarial): the CWD-shared store holds repo A's active run;
// starting a write in repo B must NOT auto-resume it — fail closed with
// explicit-resume guidance, before any controller dispatch or persistence.
func TestRunWriteControllerWorkflow_CrossRepoActiveRunNotAutoResumed(t *testing.T) {
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID: "wf-repo-a",
		Goal:  "add feature X",
		Identity: &types.WriteWorkflowRepoIdentity{
			IdentitySchema:    types.WriteWorkflowRepoIdentitySchemaVersion,
			CanonicalRepoRoot: "/srv/checkouts/repo-a",
			GoalHash:          types.WriteWorkflowGoalHash("add feature X"),
		},
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	o := wfidTestOrchestrator(t, store, "add feature X", "add feature X", t.TempDir(), nil, nil)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil ||
		!strings.Contains(err.Error(), "write workflow auto-resume refused") ||
		!strings.Contains(err.Error(), types.WriteWorkflowIdentityReasonRepoRootMismatch) ||
		!strings.Contains(err.Error(), "/workflow resume wf-repo-a") {
		t.Fatalf("cross-repo active run must fail closed with explicit-resume guidance, got %v", err)
	}
	if store.last != nil {
		t.Fatalf("refusal must not persist a resumed or freshly seeded run, got %+v", store.last)
	}
	if result := o.busCtx.Mutable.Result(); !strings.Contains(result, "write workflow auto-resume refused") {
		t.Fatalf("refusal guidance must reach the user-facing result, got %q", result)
	}
}

// Pin 1b: the identity-aware finder lane — no matching run but mismatched
// active runs exist → fail closed naming the first skip.
func TestRunWriteControllerWorkflow_IdentitySkipsFailClosed(t *testing.T) {
	store := &fakeIdentityMatcherStore{skips: []types.WriteWorkflowIdentitySkip{{
		RunID:      "wf-other",
		Goal:       "other goal",
		ReasonCode: types.WriteWorkflowIdentityReasonRepoRootMismatch,
		Detail:     "stored repo root \"/srv/checkouts/repo-a\", current repo root \"/srv/checkouts/repo-b\"",
	}, {
		RunID:      "wf-third",
		ReasonCode: types.WriteWorkflowIdentityReasonGoalMismatch,
	}}}
	o := wfidTestOrchestrator(t, store, "new goal", "new goal", "", nil, nil)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil ||
		!strings.Contains(err.Error(), "write workflow auto-resume refused") ||
		!strings.Contains(err.Error(), "wf-other") ||
		!strings.Contains(err.Error(), "1 more active run(s) were skipped") {
		t.Fatalf("skipped active runs must fail closed naming the first skip, got %v", err)
	}
	if store.last != nil {
		t.Fatalf("refusal must not persist anything, got %+v", store.last)
	}
}

// Pin 2 (same repo, base HEAD drift): the stored identity was minted at
// commit 1; a second commit moves HEAD and auto-resume must refuse with the
// base-head reason.
func TestRunWriteControllerWorkflow_BaseHeadDriftNotAutoResumed(t *testing.T) {
	repo := wfidInitGitRepo(t)
	mint := wfidTestOrchestrator(t, &fakeWorkflowRunStore{}, "drift goal", "drift goal", repo, nil, nil)
	stored := mint.currentWriteWorkflowRepoIdentity("drift goal")
	if stored.BaseHeadSHA == "" {
		t.Fatalf("mint on a real git repo must record base HEAD, got %+v", stored)
	}
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID:         "wf-drift",
		Goal:          "drift goal",
		Identity:      &stored,
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	wfidGitCommit(t, repo, "drift")
	o := wfidTestOrchestrator(t, store, "drift goal", "drift goal", repo, nil, nil)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil ||
		!strings.Contains(err.Error(), "write workflow auto-resume refused") ||
		!strings.Contains(err.Error(), types.WriteWorkflowIdentityReasonBaseHeadMismatch) {
		t.Fatalf("base HEAD drift must fail closed with the base-head reason, got %v", err)
	}
	if store.last != nil {
		t.Fatalf("refusal must not persist anything, got %+v", store.last)
	}
}

// Pin 2b (real-git happy path): same repo, same base, same goal — auto-resume
// works exactly as before the identity gate.
func TestRunWriteControllerWorkflow_MatchingIdentityAutoResumesRealGit(t *testing.T) {
	repo := wfidInitGitRepo(t)
	mint := wfidTestOrchestrator(t, &fakeWorkflowRunStore{}, "same goal", "same goal", repo, nil, nil)
	stored := mint.currentWriteWorkflowRepoIdentity("same goal")
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID:         "wf-same",
		Goal:          "same goal",
		Identity:      &stored,
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	controllerCalls := 0
	o := wfidTestOrchestrator(t, store, "same goal", "same goal", repo, []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}, &controllerCalls)
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("matching identity must auto-resume: %v", err)
	}
	if store.last == nil || store.last.RunID != "wf-same" ||
		!workflowProgressHasReason(store.last.ProgressLedger, "workflow_resumed") {
		t.Fatalf("matching identity should resume the stored run, got %+v", store.last)
	}
}

// Pin 3 (goal change): same repo/base but a different goal must not
// auto-resume.
func TestRunWriteControllerWorkflow_GoalChangeNotAutoResumed(t *testing.T) {
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID:         "wf-goal",
		Goal:          "original goal",
		Identity:      testWorkflowIdentity("original goal"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	o := wfidTestOrchestrator(t, store, "different goal", "different goal", "", nil, nil)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil ||
		!strings.Contains(err.Error(), "write workflow auto-resume refused") ||
		!strings.Contains(err.Error(), types.WriteWorkflowIdentityReasonGoalMismatch) ||
		!strings.Contains(err.Error(), "/workflow resume wf-goal") {
		t.Fatalf("goal change must fail closed with the goal-mismatch reason, got %v", err)
	}
	if store.last != nil {
		t.Fatalf("refusal must not persist anything, got %+v", store.last)
	}
}

// Pin 4 (legacy + explicit resume): a pre-WFID-1 run file (no identity) never
// auto-resumes; after the explicit /workflow resume one-shot token it resumes
// once, the token is consumed, and the identity is re-stamped to the current
// context.
func TestRunWriteControllerWorkflow_LegacyRunRequiresExplicitResume(t *testing.T) {
	legacy := func() *types.WriteWorkflowRun {
		return &types.WriteWorkflowRun{
			RunID:         "wf-legacy",
			Goal:          "legacy goal",
			Status:        types.WriteWorkflowRunInProgress,
			ActiveBatchID: "batch-1",
			Batches: []types.WriteWorkflowBatch{{
				ID:     "batch-1",
				Status: types.WriteWorkflowBatchReadyToPlan,
			}},
		}
	}

	// Arm A: legacy file → refused.
	store := &fakeWorkflowRunStore{active: legacy()}
	o := wfidTestOrchestrator(t, store, "legacy goal", "legacy goal", "", nil, nil)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil ||
		!strings.Contains(err.Error(), "write workflow auto-resume refused") ||
		!strings.Contains(err.Error(), types.WriteWorkflowIdentityReasonIdentityMissing) {
		t.Fatalf("legacy run must fail closed as identity_missing, got %v", err)
	}
	if store.last != nil {
		t.Fatalf("legacy refusal must not persist anything, got %+v", store.last)
	}

	// Arm B: explicit one-shot authorization → resumes once, token consumed,
	// identity adopted.
	authorized := legacy()
	authorized.ResumeAuthorization = types.WriteWorkflowResumeAuthorizationExplicit
	authorized.ResumeAuthorizedAt = time.Now()
	store = &fakeWorkflowRunStore{active: authorized}
	controllerCalls := 0
	o = wfidTestOrchestrator(t, store, "legacy goal", "legacy goal", "", []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}, &controllerCalls)
	steps = 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("explicitly authorized legacy run must resume: %v", err)
	}
	if store.last == nil || store.last.RunID != "wf-legacy" ||
		!workflowProgressHasReason(store.last.ProgressLedger, "workflow_resumed_explicit") {
		t.Fatalf("explicit resume should adopt the legacy run, got %+v", store.last)
	}
	if store.last.ResumeAuthorization != "" || !store.last.ResumeAuthorizedAt.IsZero() {
		t.Fatalf("one-shot token must be consumed on load, got %q at %v", store.last.ResumeAuthorization, store.last.ResumeAuthorizedAt)
	}
	if store.last.Identity == nil ||
		store.last.Identity.IdentitySchema != types.WriteWorkflowRepoIdentitySchemaVersion ||
		store.last.Identity.GoalHash != types.WriteWorkflowGoalHash("legacy goal") {
		t.Fatalf("explicit resume must re-stamp the identity to the current context, got %+v", store.last.Identity)
	}
}
