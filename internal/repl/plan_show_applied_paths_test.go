package repl

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestPlanShow_PartiallyAppliedRendersAppliedAndPending pins
// commit 42 P0 #2: when /plan show renders a partially_applied
// plan, it must render WHICH files actually landed (AppliedPaths)
// vs which are still pending (TargetPaths \ AppliedPaths).
// Pre-commit-42 only Status=partially_applied was visible —
// the operator had no way to tell the work distribution from
// the rendered output.
func TestPlanShow_PartiallyAppliedRendersAppliedAndPending(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	r, out := newScriptedREPL(t, store)
	r.writeEnabled = true

	plan := &types.ChangePlan{
		ID:           "plan-partial-test",
		Status:       types.PlanStatusPartiallyApplied,
		CreatedAt:    time.Now(),
		TargetPaths:  []string{"a.go", "b.go", "c.go", "d.go"},
		AppliedPaths: []string{"a.go", "b.go"},
		Changes: []types.FileChange{
			{Path: "a.go", Kind: "create", NewContent: "package a"},
			{Path: "b.go", Kind: "create", NewContent: "package b"},
			{Path: "c.go", Kind: "create", NewContent: "package c"},
			{Path: "d.go", Kind: "create", NewContent: "package d"},
		},
	}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	r.pendingPlanPath = filepath.Join(store.PlanDir(), "plan-partial-test.json")

	r.handlePlanCmd("/plan show")
	got := out.String()
	if !strings.Contains(got, "applied: 2/4") {
		t.Errorf("expected 'applied: 2/4' progress line; got:\n%s", got)
	}
	if !strings.Contains(got, "a.go, b.go") {
		t.Errorf("expected applied list 'a.go, b.go'; got:\n%s", got)
	}
	if !strings.Contains(got, "pending: c.go, d.go") {
		t.Errorf("expected pending list; got:\n%s", got)
	}
}

// TestPlanShow_NonPartialDoesNotRenderApplied pins back-compat:
// applied / verify_failed / pending plans don't get the
// applied/pending split (irrelevant when ALL targets either
// landed or none did).
func TestPlanShow_NonPartialDoesNotRenderApplied(t *testing.T) {
	dir := t.TempDir()
	store := NewPlanStore(dir)
	r, out := newScriptedREPL(t, store)
	r.writeEnabled = true

	plan := &types.ChangePlan{
		ID:           "plan-applied-test",
		Status:       types.PlanStatusApplied,
		CreatedAt:    time.Now(),
		TargetPaths:  []string{"x.go"},
		AppliedPaths: []string{"x.go"},
		Changes:      []types.FileChange{{Path: "x.go", Kind: "create", NewContent: "ok"}},
	}
	if _, err := store.SaveForTest(plan); err != nil {
		t.Fatalf("SaveForTest: %v", err)
	}
	r.pendingPlanPath = filepath.Join(store.PlanDir(), "plan-applied-test.json")

	r.handlePlanCmd("/plan show")
	got := out.String()
	if strings.Contains(got, "applied: 1/1") {
		t.Errorf("non-partial status should not surface applied/pending split; got:\n%s", got)
	}
}
