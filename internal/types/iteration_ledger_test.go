package types

import (
	"strings"
	"testing"
	"time"
)

// iteration_ledger_test.go pins Batch 2 Module C's data + render
// contract. RenderIterationLedger emits raw data sections only; no
// system pre-classification or prescriptive prose may leak in.

func TestRenderIterationLedger_EmptyReturnsEmpty(t *testing.T) {
	if got := RenderIterationLedger(nil); got != "" {
		t.Errorf("empty ledger should produce empty string; got %q", got)
	}
	if got := RenderIterationLedger([]IterationRecord{}); got != "" {
		t.Errorf("zero-length ledger should produce empty string; got %q", got)
	}
}

func TestRenderIterationLedger_VerbatimContent(t *testing.T) {
	ledger := []IterationRecord{
		{
			Attempt:        1,
			PlanID:         "plan-001",
			PlanSummary:    "Add pygame import; create main.py.",
			ChangedFiles:   []string{"main.py", "requirements.txt"},
			PassedCount:    0,
			FailedCount:    3,
			FailureSummary: "ModuleNotFoundError: No module named 'pygame'\n  File main.py:1",
			Timestamp:      time.Date(2026, 4, 30, 14, 0, 0, 0, time.UTC),
		},
		{
			Attempt:        2,
			PlanID:         "plan-002",
			PlanSummary:    "Add pygame to requirements.txt; rerun.",
			ChangedFiles:   []string{"requirements.txt", "main.py"},
			PassedCount:    2,
			FailedCount:    1,
			FailureSummary: "AssertionError: expected width=10, got width=8",
			Timestamp:      time.Date(2026, 4, 30, 14, 5, 0, 0, time.UTC),
		},
	}
	got := RenderIterationLedger(ledger)
	// Section heading.
	if !strings.Contains(got, "## Iteration history") {
		t.Errorf("missing section header; got:\n%s", got)
	}
	// Both attempts.
	for _, want := range []string{
		"Attempt 1", "plan-001", "Add pygame import; create main.py.",
		"main.py", "requirements.txt",
		"ModuleNotFoundError: No module named 'pygame'",
		"Attempt 2", "plan-002", "AssertionError: expected width=10",
		"2 passed, 1 failed",
		"0 passed, 3 failed",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered ledger missing %q; got:\n%s", want, got)
		}
	}
	// Forbidden — must NOT contain prescribed prose.
	for _, banned := range []string{
		"Most common cause", "Revise to", "DO NOT", "you should",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("rendered ledger must not contain prescriptive prose %q; got:\n%s", banned, got)
		}
	}
}

func TestRenderIterationLedger_SurfaceBlobRefWhenSet(t *testing.T) {
	ledger := []IterationRecord{
		{
			Attempt:               1,
			FailureSummary:        "first 4 KB of stderr…",
			FailureSummaryBlobRef: "/tmp/codrax/blob/run_tests-abc123.txt",
		},
	}
	got := RenderIterationLedger(ledger)
	if !strings.Contains(got, "/tmp/codrax/blob/run_tests-abc123.txt") {
		t.Errorf("rendered ledger must surface blob ref so planner can page; got:\n%s", got)
	}
	if !strings.Contains(got, "read_file with offset/limit") {
		t.Errorf("rendered ledger should mention read_file paging; got:\n%s", got)
	}
}

func TestMutableState_AppendIteration_DefensiveCopy(t *testing.T) {
	mu := NewMutableState("test")
	original := []string{"a.go", "b.go"}
	mu.AppendIteration(IterationRecord{Attempt: 1, ChangedFiles: original})

	// Mutate the original slice the caller passed; the ledger should
	// be unaffected.
	original[0] = "MUTATED"
	got := mu.IterationLedger()
	if len(got) != 1 {
		t.Fatalf("ledger should have 1 entry; got %d", len(got))
	}
	if got[0].ChangedFiles[0] != "a.go" {
		t.Errorf("AppendIteration must defensive-copy ChangedFiles; ledger now reads %q", got[0].ChangedFiles[0])
	}
}

func TestMutableState_IterationLedger_AppendOrder(t *testing.T) {
	mu := NewMutableState("test")
	mu.AppendIteration(IterationRecord{Attempt: 1})
	mu.AppendIteration(IterationRecord{Attempt: 2})
	mu.AppendIteration(IterationRecord{Attempt: 3})
	got := mu.IterationLedger()
	if len(got) != 3 {
		t.Fatalf("expected 3 entries; got %d", len(got))
	}
	for i, rec := range got {
		if rec.Attempt != i+1 {
			t.Errorf("entry %d attempt = %d; want %d", i, rec.Attempt, i+1)
		}
	}
}

func TestMutableState_ResetIterationLedger(t *testing.T) {
	mu := NewMutableState("test")
	mu.AppendIteration(IterationRecord{Attempt: 1})
	mu.ResetIterationLedger()
	if len(mu.IterationLedger()) != 0 {
		t.Errorf("ResetIterationLedger should empty the ledger")
	}
}
