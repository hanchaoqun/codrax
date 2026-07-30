package orchestrator

// WFID-1 (§29.213 件1): write workflow auto-resume repo/task identity gate.
// The pins here drive production runWriteControllerWorkflow through
// loadOrSeedWriteWorkflowRun so the gate is exercised end to end: A→B
// cross-repo refusal, base-HEAD drift refusal, goal-change refusal, legacy
// (identity-less) refusal with the explicit one-shot resume escape, and the
// preserved happy path on a real git repo.

import (
	"os/exec"
	"path/filepath"
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
	o.cancelTokenPtr.Store(NewCancelToken())
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

// Pin 1 (EVAL-W1-L): a legacy FindActiveRun-only store returns just its newest
// active run. When that run belongs to repo A, the controller cannot prove an
// older repo-B active run is not hidden behind it. Starting a fresh repo-B run
// would risk two same-repo workflows, so this compatibility lane fails closed.
// The production identity-aware finder still enumerates every candidate; its
// all-cross-repo fresh-seed behavior is pinned by Pin 1c below.
func TestRunWriteControllerWorkflow_LegacyCrossRepoActiveRunFailsClosed(t *testing.T) {
	crossRepo := &types.WriteWorkflowRun{
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
	}
	store := &fakeWorkflowRunStore{active: crossRepo}
	controllerCalls := 0
	o := wfidTestOrchestrator(t, store, "add feature X", "add feature X", t.TempDir(), nil, &controllerCalls)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "write workflow auto-resume refused") ||
		!strings.Contains(err.Error(), "additional same-repo active runs cannot be excluded") {
		t.Fatalf("legacy single-result cross-repo lookup must fail closed, got %v", err)
	}
	if store.last != nil {
		t.Fatalf("legacy ambiguity must not seed or persist a competing run: %+v", store.last)
	}
	if crossRepo.Status != types.WriteWorkflowRunInProgress {
		t.Fatalf("the other repo's run must stay untouched: %+v", crossRepo)
	}
	if controllerCalls != 0 {
		t.Fatalf("legacy ambiguity must stop before controller dispatch, calls=%d", controllerCalls)
	}
}

// Pin 1b (GAP-EVAL-W1 evolution): the identity-aware finder lane — only
// SAME-REPO mismatches block; the refusal names the first same-repo skip
// (cross-repo skips are disclosed, never blocking, and never counted in the
// "more skipped" tally).
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
		!strings.Contains(err.Error(), "wf-third") {
		t.Fatalf("the same-repo skip must fail closed by name, got %v", err)
	}
	if strings.Contains(err.Error(), "/workflow resume wf-other") {
		t.Fatalf("the cross-repo run must not be the named blocker: %v", err)
	}
	if strings.Contains(err.Error(), "more active run(s) were skipped") {
		t.Fatalf("cross-repo skips must not inflate the skipped tally: %v", err)
	}
	if store.last != nil {
		t.Fatalf("refusal must not persist anything, got %+v", store.last)
	}
}

// Pin 1c (GAP-EVAL-W1): ALL skips out-of-repo → nothing blocks; a fresh run
// seeds for THIS repo (the eval parallel-contamination shape: two cases in
// one CWD, each driving its own --repo checkout).
func TestRunWriteControllerWorkflow_AllCrossRepoSkipsSeedFreshRun(t *testing.T) {
	store := &fakeIdentityMatcherStore{skips: []types.WriteWorkflowIdentitySkip{{
		RunID:      "wf-other",
		Goal:       "other goal",
		ReasonCode: types.WriteWorkflowIdentityReasonRepoRootMismatch,
	}, {
		RunID:      "wf-another",
		ReasonCode: types.WriteWorkflowIdentityReasonRepoRootMismatch,
	}}}
	controllerCalls := 0
	o := wfidTestOrchestrator(t, store, "new goal", "new goal", t.TempDir(), []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}, &controllerCalls)
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("all-cross-repo skips must not block a fresh write: %v", err)
	}
	if store.last == nil || store.last.RunID == "wf-other" || store.last.RunID == "wf-another" {
		t.Fatalf("a fresh run must be seeded: %+v", store.last)
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

// FIX-1 pin (deterministic goal hash): the identity gate hashes the USER
// REQUEST text, not the LLM's Task.Summary. The same byte-identical request
// re-issued with a differently-worded summary (LLM non-determinism) must
// still auto-resume; before FIX-1 the summary drift alone refused the happy
// path (noisy signal in a hard gate).
func TestRunWriteControllerWorkflow_SummaryRephraseStillAutoResumes(t *testing.T) {
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID:         "wf-rephrase",
		Goal:          "Add feature X to the parser",
		Identity:      testWorkflowIdentity("add feature X to the parser please"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	controllerCalls := 0
	// Same raw request, differently-worded LLM summary.
	o := wfidTestOrchestrator(t, store, "add feature X to the parser please",
		"Implement parser feature X (rephrased by a second classification run)", "",
		[]writeflow.WriteWorkflowDecision{{Action: writeflow.ActionFinish, ReasonCode: "done"}}, &controllerCalls)
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("summary rephrasing alone must not refuse auto-resume: %v", err)
	}
	if store.last == nil || store.last.RunID != "wf-rephrase" ||
		!workflowProgressHasReason(store.last.ProgressLedger, "workflow_resumed") {
		t.Fatalf("same-request run should resume despite summary drift, got %+v", store.last)
	}
}

// FIX-2 + EVAL-W1-L pin (token root binding, legacy consumption lane): a
// one-shot token minted in another repo context must not be spendable here.
// It is cleared and persisted, then the single-result ambiguity still fails
// closed instead of fresh-seeding a possibly competing same-repo run.
func TestRunWriteControllerWorkflow_CrossContextResumeTokenRefusedAndCleared(t *testing.T) {
	store := &fakeWorkflowRunStore{active: &types.WriteWorkflowRun{
		RunID: "wf-foreign-token",
		Goal:  "foreign goal",
		Identity: &types.WriteWorkflowRepoIdentity{
			IdentitySchema:    types.WriteWorkflowRepoIdentitySchemaVersion,
			CanonicalRepoRoot: "/srv/checkouts/repo-a",
			GoalHash:          types.WriteWorkflowGoalHash("foreign goal"),
		},
		ResumeAuthorization:      types.WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedRepoRoot: "/srv/checkouts/repo-a",
		ResumeAuthorizedAt:       time.Now(),
		Status:                   types.WriteWorkflowRunInProgress,
		ActiveBatchID:            "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	// Capture the clear through a save recorder: even though lookup refuses,
	// a foreign token left on disk would stay spendable in its mint context.
	var tokenClearSave *types.WriteWorkflowRun
	recorder := &recordingWorkflowRunStore{inner: store, onSave: func(run *types.WriteWorkflowRun) {
		if run != nil && run.RunID == "wf-foreign-token" {
			cp := types.CloneWriteWorkflowRun(*run)
			tokenClearSave = &cp
		}
	}}
	controllerCalls := 0
	o := wfidTestOrchestrator(t, recorder, "foreign goal", "foreign goal", t.TempDir(), []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}, &controllerCalls)
	steps := 0
	err := o.runWriteControllerWorkflow(&steps)
	if err == nil || !strings.Contains(err.Error(), "additional same-repo active runs cannot be excluded") {
		t.Fatalf("legacy cross-repo token clear must still end fail-closed, got %v", err)
	}
	if tokenClearSave == nil {
		t.Fatal("the invalid token clear must be persisted before the fresh seed")
	}
	if tokenClearSave.ResumeAuthorization != "" || tokenClearSave.ResumeAuthorizedRepoRoot != "" || !tokenClearSave.ResumeAuthorizedAt.IsZero() {
		t.Fatalf("cross-context token must be cleared, got %q root %q at %v",
			tokenClearSave.ResumeAuthorization, tokenClearSave.ResumeAuthorizedRepoRoot, tokenClearSave.ResumeAuthorizedAt)
	}
	if workflowProgressHasReason(tokenClearSave.ProgressLedger, "workflow_resumed") ||
		workflowProgressHasReason(tokenClearSave.ProgressLedger, "workflow_resumed_explicit") {
		t.Fatalf("the token-clear persist must not smuggle in a resume, got %+v", tokenClearSave.ProgressLedger)
	}
	if store.last == nil || store.last.RunID != "wf-foreign-token" {
		t.Fatalf("only the foreign token clear may be persisted: %+v", store.last)
	}
	if controllerCalls != 0 {
		t.Fatalf("legacy ambiguity must stop before controller dispatch, calls=%d", controllerCalls)
	}
}

// Pin (§15.12 批丙 P3): an identity-aware store that RETURNS a cross-repo
// mismatching run (not as a skip — a third-party enumerating store may
// surface it as the matched candidate) keeps the documented arm semantics:
// the foreign one-shot token is cleared AND persisted, the run is left
// untouched, and a fresh run is seeded for this repo. Batch F's pin flip
// left this identity-aware side of the arm with zero coverage.
func TestRunWriteControllerWorkflow_IdentityAwareReturnedCrossRepoRunSeedsFresh(t *testing.T) {
	inner := &fakeIdentityMatcherStore{matched: &types.WriteWorkflowRun{
		RunID: "wf-cross-returned",
		Goal:  "foreign goal",
		Identity: &types.WriteWorkflowRepoIdentity{
			IdentitySchema:    types.WriteWorkflowRepoIdentitySchemaVersion,
			CanonicalRepoRoot: "/srv/checkouts/repo-a",
			GoalHash:          types.WriteWorkflowGoalHash("foreign goal"),
		},
		ResumeAuthorization:      types.WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedRepoRoot: "/srv/checkouts/repo-a",
		ResumeAuthorizedAt:       time.Now(),
		Status:                   types.WriteWorkflowRunInProgress,
		ActiveBatchID:            "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}}
	var tokenClearSave *types.WriteWorkflowRun
	store := &recordingIdentityMatcherStore{inner: inner, onSave: func(run *types.WriteWorkflowRun) {
		if run != nil && run.RunID == "wf-cross-returned" {
			cp := types.CloneWriteWorkflowRun(*run)
			tokenClearSave = &cp
		}
	}}
	controllerCalls := 0
	o := wfidTestOrchestrator(t, store, "new goal here", "new goal here", t.TempDir(), []writeflow.WriteWorkflowDecision{
		{Action: writeflow.ActionFinish, ReasonCode: "done"},
	}, &controllerCalls)
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("identity-aware cross-repo returned run must not block a fresh write: %v", err)
	}
	if tokenClearSave == nil {
		t.Fatal("the foreign token clear must be persisted before the fresh seed")
	}
	if tokenClearSave.ResumeAuthorization != "" || tokenClearSave.ResumeAuthorizedRepoRoot != "" || !tokenClearSave.ResumeAuthorizedAt.IsZero() {
		t.Fatalf("cross-context token must be cleared, got %q root %q at %v",
			tokenClearSave.ResumeAuthorization, tokenClearSave.ResumeAuthorizedRepoRoot, tokenClearSave.ResumeAuthorizedAt)
	}
	if inner.last == nil || inner.last.RunID == "wf-cross-returned" {
		t.Fatalf("a fresh run must be seeded for this repo: %+v", inner.last)
	}
	if controllerCalls == 0 {
		t.Fatal("the fresh run must reach controller dispatch")
	}
}

// recordingIdentityMatcherStore — identity-aware variant of the Save
// recorder (the wrapper must keep the identity-aware method set, or the
// gate would silently drop to the legacy fail-close lane mid-test).
type recordingIdentityMatcherStore struct {
	inner  *fakeIdentityMatcherStore
	onSave func(*types.WriteWorkflowRun)
}

func (s *recordingIdentityMatcherStore) Save(run *types.WriteWorkflowRun) (string, error) {
	if s.onSave != nil {
		s.onSave(run)
	}
	return s.inner.Save(run)
}

func (s *recordingIdentityMatcherStore) FindActiveRun() (*types.WriteWorkflowRun, error) {
	return s.inner.FindActiveRun()
}

func (s *recordingIdentityMatcherStore) FindActiveRunMatching(identity types.WriteWorkflowRepoIdentity) (*types.WriteWorkflowRun, []types.WriteWorkflowIdentitySkip, error) {
	return s.inner.FindActiveRunMatching(identity)
}

// recordingWorkflowRunStore wraps a fake store to observe individual Save
// calls (the singular `last` field only keeps the final one).
type recordingWorkflowRunStore struct {
	inner  *fakeWorkflowRunStore
	onSave func(*types.WriteWorkflowRun)
}

func (s *recordingWorkflowRunStore) Save(run *types.WriteWorkflowRun) (string, error) {
	if s.onSave != nil {
		s.onSave(run)
	}
	return s.inner.Save(run)
}

func (s *recordingWorkflowRunStore) FindActiveRun() (*types.WriteWorkflowRun, error) {
	return s.inner.FindActiveRun()
}

// fakeSweeperMatcherStore drives the FIX-2 sweep pins: an identity-aware
// finder plus the WriteWorkflowResumeAuthorizationSweeper capability over an
// in-memory residual-run map.
type fakeSweeperMatcherStore struct {
	fakeWorkflowRunStore
	matched    *types.WriteWorkflowRun
	residual   map[string]*types.WriteWorkflowRun
	sweepCalls []string
}

func (s *fakeSweeperMatcherStore) FindActiveRunMatching(types.WriteWorkflowRepoIdentity) (*types.WriteWorkflowRun, []types.WriteWorkflowIdentitySkip, error) {
	return s.matched, nil, nil
}

func (s *fakeSweeperMatcherStore) ClearResumeAuthorizationsExcept(exceptRunID string) (int, error) {
	s.sweepCalls = append(s.sweepCalls, exceptRunID)
	cleared := 0
	for id, run := range s.residual {
		if id == exceptRunID || run == nil || run.ResumeAuthorization == "" {
			continue
		}
		run.ResumeAuthorization = ""
		run.ResumeAuthorizedRepoRoot = ""
		run.ResumeAuthorizedAt = time.Time{}
		cleared++
	}
	return cleared, nil
}

func residualTokenRun(id string) *types.WriteWorkflowRun {
	return &types.WriteWorkflowRun{
		RunID:               id,
		Status:              types.WriteWorkflowRunInProgress,
		ResumeAuthorization: types.WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedAt:  time.Now(),
	}
}

// FIX-2 pin (window b — --plan-file import lane): the import lane never
// consults the finder, so before this fix a stamped token survived it
// indefinitely. Now any successful loadOrSeed sweeps residual tokens, the
// import/fresh-seed lanes included.
func TestLoadOrSeedWriteWorkflow_ImportLaneSweepsResidualTokens(t *testing.T) {
	store := &fakeSweeperMatcherStore{residual: map[string]*types.WriteWorkflowRun{
		"wf-stale-token": residualTokenRun("wf-stale-token"),
	}}
	o := wfidTestOrchestrator(t, store, "import a plan", "import a plan", "", nil, nil)
	planFile := filepath.Join(t.TempDir(), "imported.plan.json")
	if err := types.WritePlanToFile(&types.ChangePlan{
		ID: "plan-imported", Status: types.PlanStatusPending, Summary: "seed", Request: "fix it",
		TargetPaths: []string{"fix.go"},
		Changes:     []types.FileChange{{Path: "fix.go", Kind: "modify", NewContent: "package main\n"}},
	}, planFile); err != nil {
		t.Fatalf("seed plan write: %v", err)
	}
	o.busCtx.PlanPath = planFile
	run, err := o.loadOrSeedWriteWorkflowRun()
	if err != nil {
		t.Fatalf("loadOrSeedWriteWorkflowRun: %v", err)
	}
	if len(store.sweepCalls) != 1 || store.sweepCalls[0] != run.RunID {
		t.Fatalf("import lane must sweep residual tokens excepting the fresh seed, got %v (run %q)", store.sweepCalls, run.RunID)
	}
	if got := store.residual["wf-stale-token"]; got.ResumeAuthorization != "" {
		t.Fatalf("residual token must be cleared after the import-lane write turn, got %+v", got)
	}
}

// FIX-2 pin (window c — an updated matching run wins the finder): the
// consumed/returned run keeps its lifecycle, but the OLDER run's stamped
// token must not survive the turn.
func TestLoadOrSeedWriteWorkflow_MatchedResumeSweepsOlderTokens(t *testing.T) {
	matched := &types.WriteWorkflowRun{
		RunID:         "wf-new",
		Goal:          "sweep goal",
		Identity:      testWorkflowIdentity("sweep goal"),
		Status:        types.WriteWorkflowRunInProgress,
		ActiveBatchID: "batch-1",
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	store := &fakeSweeperMatcherStore{matched: matched, residual: map[string]*types.WriteWorkflowRun{
		"wf-old-token": residualTokenRun("wf-old-token"),
	}}
	o := wfidTestOrchestrator(t, store, "sweep goal", "sweep goal", "", nil, nil)
	run, err := o.loadOrSeedWriteWorkflowRun()
	if err != nil {
		t.Fatalf("loadOrSeedWriteWorkflowRun: %v", err)
	}
	if run.RunID != "wf-new" || !workflowProgressHasReason(run.ProgressLedger, "workflow_resumed") {
		t.Fatalf("matching run must resume, got %+v", run)
	}
	if len(store.sweepCalls) != 1 || store.sweepCalls[0] != "wf-new" {
		t.Fatalf("matched resume must sweep excepting the consumed run, got %v", store.sweepCalls)
	}
	if got := store.residual["wf-old-token"]; got.ResumeAuthorization != "" {
		t.Fatalf("older run's token must be cleared after the turn, got %+v", got)
	}
}

// FIX-4 pin: adoption re-stamps the identity AND refreshes the display goal —
// run.Goal must not keep narrating the pre-adoption goal against the
// re-stamped identity. The identity hash stays on the deterministic request
// text (FIX-1 decoupling).
func TestRunWriteControllerWorkflow_ExplicitResumeAdoptionRefreshesDisplayGoal(t *testing.T) {
	adopted := &types.WriteWorkflowRun{
		RunID:               "wf-adopt-display",
		Goal:                "old display goal",
		Status:              types.WriteWorkflowRunInProgress,
		ActiveBatchID:       "batch-1",
		ResumeAuthorization: types.WriteWorkflowResumeAuthorizationExplicit,
		ResumeAuthorizedAt:  time.Now(),
		Batches: []types.WriteWorkflowBatch{{
			ID:     "batch-1",
			Status: types.WriteWorkflowBatchReadyToPlan,
		}},
	}
	store := &fakeWorkflowRunStore{active: adopted}
	controllerCalls := 0
	o := wfidTestOrchestrator(t, store, "adopt me into this context", "fresh display goal",
		"", []writeflow.WriteWorkflowDecision{{Action: writeflow.ActionFinish, ReasonCode: "done"}}, &controllerCalls)
	steps := 0
	if err := o.runWriteControllerWorkflow(&steps); err != nil {
		t.Fatalf("explicitly authorized run must adopt: %v", err)
	}
	if store.last == nil || store.last.RunID != "wf-adopt-display" {
		t.Fatalf("adoption must persist the run, got %+v", store.last)
	}
	if store.last.Goal != "fresh display goal" {
		t.Fatalf("adoption must refresh the display goal to the current candidate, got %q", store.last.Goal)
	}
	if store.last.Identity == nil || store.last.Identity.GoalHash != types.WriteWorkflowGoalHash("adopt me into this context") {
		t.Fatalf("adopted identity hash must ride the deterministic request text, got %+v", store.last.Identity)
	}
}
