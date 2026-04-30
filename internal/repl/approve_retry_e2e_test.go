package repl

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApproveRetry_PartiallyAppliedRendersGuidance pins the
// commit 17 #1 honest message: /approve --retry against a
// partially_applied plan tells the operator that apply re-runs
// from scratch in a fresh worktree (because /approve always
// dispatches a new Run with a clean worktree), NOT that "the
// coder skips already-applied units" (which was the original
// commit-13 misleading wording — the new Run has no AppliedSet
// to skip). The status-gate routing is exercised and the
// orchestrator dispatch is stubbed out.
func TestApproveRetry_PartiallyAppliedRendersGuidance(t *testing.T) {
	planDir := t.TempDir()

	// Persist the partially_applied plan to PlanStore. WorktreePath
	// is empty (the typical case for partially_applied — the
	// failing Run's worktree was discarded by the cleanup defer).
	store := NewPlanStore(planDir)
	plan := &types.ChangePlan{
		ID:          "plan-partial-retry-test",
		Status:      types.PlanStatusPartiallyApplied,
		CreatedAt:   time.Now(),
		Summary:     "partial-state retry test",
		TargetPaths: []string{"a.go", "b.go", "c.go", "d.go"},
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", NewContent: "package x\n"},
			{Path: "b.go", Kind: "create", NewContent: "package x\n"},
			{Path: "c.go", Kind: "create", NewContent: "package x\n"},
			{Path: "d.go", Kind: "create", NewContent: "package x\n"},
		},
	}
	planPath, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	out := &bytes.Buffer{}
	r := &REPL{
		runner:          stubRunner{},
		planStore:       store,
		runtimeAnchor:   t.TempDir(),
		out:             out,
		in:              strings.NewReader(""),
		colorMode:       render.ColorNever,
		language:        "en",
		writeEnabled:    true,
		pendingPlanPath: planPath,
	}

	r.handleApproveCmd("/approve --retry")

	got := out.String()
	if !strings.Contains(got, "retrying plan plan-partial-retry-test") {
		t.Errorf("expected retry message; got %q", got)
	}
	// The honest message names the count + the fresh-worktree
	// reality and warns about no-op repeat without source change.
	if !strings.Contains(got, "all 4 target paths in a fresh worktree") {
		t.Errorf("expected 'all 4 target paths in a fresh worktree'; got %q", got)
	}
	if !strings.Contains(got, "idempotent") {
		t.Errorf("expected idempotency note; got %q", got)
	}
	// The pre-commit-17 misleading wording must NOT appear.
	if strings.Contains(got, "skips already-applied units") {
		t.Errorf("misleading 'skips already-applied units' wording leaked back; got %q", got)
	}
}

// TestApproveRetry_UnverifiedSuggestsVerify pins commit 17 #1
// /verify-recommendation: /approve --retry on unverified prints
// a guidance line pointing at /verify <plan-id> as the better
// command (the bytes already landed; the operator wants to
// re-run tests, not re-apply).
func TestApproveRetry_UnverifiedSuggestsVerify(t *testing.T) {
	planDir := t.TempDir()
	store := NewPlanStore(planDir)
	plan := &types.ChangePlan{
		ID:          "plan-unverified-test",
		Status:      types.PlanStatusUnverified,
		CreatedAt:   time.Now(),
		Summary:     "unverified retry test",
		TargetPaths: []string{"a.go"},
		Changes:     []types.FileChange{{Path: "a.go", Kind: "create"}},
	}
	planPath, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}
	out := &bytes.Buffer{}
	r := &REPL{
		runner:          stubRunner{},
		planStore:       store,
		runtimeAnchor:   t.TempDir(),
		out:             out,
		in:              strings.NewReader(""),
		colorMode:       render.ColorNever,
		language:        "en",
		writeEnabled:    true,
		pendingPlanPath: planPath,
	}
	r.handleApproveCmd("/approve --retry")
	got := out.String()
	if !strings.Contains(got, "/verify plan-unverified-test") {
		t.Errorf("expected /verify suggestion; got %q", got)
	}
	if !strings.Contains(got, "apply already landed") {
		t.Errorf("expected 'apply already landed' explanation; got %q", got)
	}
}

// TestApproveRetry_RefusedWithoutRetryFlag pins the negative
// case: /approve (no --retry) against a partially_applied plan
// surfaces the actionable hint and refuses to dispatch.
func TestApproveRetry_RefusedWithoutRetryFlag(t *testing.T) {
	planDir := t.TempDir()
	store := NewPlanStore(planDir)
	plan := &types.ChangePlan{
		ID:          "plan-needs-retry",
		Status:      types.PlanStatusPartiallyApplied,
		CreatedAt:   time.Now(),
		Summary:     "needs --retry",
		TargetPaths: []string{"x.go"},
		Changes:     []types.FileChange{{Path: "x.go", Kind: "create"}},
	}
	planPath, err := store.Save(plan)
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	out := &bytes.Buffer{}
	r := &REPL{
		runner:          stubRunner{},
		planStore:       store,
		runtimeAnchor:   t.TempDir(),
		out:             out,
		in:              strings.NewReader(""),
		colorMode:       render.ColorNever,
		language:        "en",
		writeEnabled:    true,
		pendingPlanPath: planPath,
	}
	r.handleApproveCmd("/approve") // no --retry
	got := out.String()
	if !strings.Contains(got, "use `/approve --retry`") {
		t.Errorf("expected --retry hint in output; got %q", got)
	}
	if !strings.Contains(got, "/reject") {
		t.Errorf("expected /reject hint in output; got %q", got)
	}
}
