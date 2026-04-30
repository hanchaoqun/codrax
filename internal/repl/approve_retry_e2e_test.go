package repl

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApproveRetry_PartiallyAppliedRendersGuidance pins the
// commit 9 #1 + commit 13 #3 surface end-to-end: the REPL
// receives a /approve --retry against a partially_applied plan,
// loads from PlanStore, runs through the status-gate's
// "partially_applied + retry" branch, and prints the progress
// guidance message. We don't dispatch the orchestrator — the
// test bypasses runner dispatch — but we DO exercise:
//
//  - parseApproveArgsAll's --retry parsing
//  - status-gate routing for partially_applied
//  - approveRetryProgress helper using the worktree state
//  - the full message format the operator sees
//
// This is an integration-shape test (REPL → planStore disk →
// status gate → message render) that catches drift across the
// commit-9-and-13 chain in one shot.
func TestApproveRetry_PartiallyAppliedRendersGuidance(t *testing.T) {
	planDir := t.TempDir()
	worktree := t.TempDir()

	// Seed the worktree to simulate a partial apply state:
	// 2 of 4 target paths already on disk, 2 remaining.
	for _, p := range []string{"a.go", "b.go"} {
		full := filepath.Join(worktree, p)
		if err := os.WriteFile(full, []byte("package x\n"), 0o644); err != nil {
			t.Fatalf("seed worktree file: %v", err)
		}
	}

	// Persist the partially_applied plan to PlanStore.
	store := NewPlanStore(planDir)
	plan := &types.ChangePlan{
		ID:           "plan-partial-retry-test",
		Status:       types.PlanStatusPartiallyApplied,
		CreatedAt:    time.Now(),
		Summary:      "partial-state retry test",
		WorktreePath: worktree,
		TargetPaths:  []string{"a.go", "b.go", "c.go", "d.go"},
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

	// Construct a minimal REPL pointed at the PlanStore. interactive=false
	// so info / warn write to r.out for capture.
	out := &bytes.Buffer{}
	r := &REPL{
		runner:           stubRunner{}, // no-op runner; the test stops at the message-print stage before dispatch
		planStore:        store,
		runtimeAnchor:    t.TempDir(),
		out:              out,
		in:               strings.NewReader(""),
		colorMode:        render.ColorNever,
		language:         "en",
		writeEnabled:     true,
		pendingPlanPath:  planPath,
	}

	// Drive /approve --retry. The status gate should route to the
	// partially_applied branch and print the progress message.
	r.handleApproveCmd("/approve --retry")

	got := out.String()
	if !strings.Contains(got, "retrying plan plan-partial-retry-test") {
		t.Errorf("expected retry message; got %q", got)
	}
	if !strings.Contains(got, "2 of 4 target paths already applied") {
		t.Errorf("expected progress count '2 of 4'; got %q", got)
	}
	if !strings.Contains(got, "remaining:") {
		t.Errorf("expected remaining-paths line; got %q", got)
	}
	if !strings.Contains(got, "c.go") || !strings.Contains(got, "d.go") {
		t.Errorf("expected pending paths c.go and d.go listed; got %q", got)
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
