package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestClassifyGateFailure_NoFailuresIsClean(t *testing.T) {
	report := types.GateReport{
		Checks: []types.GateCheck{
			{Name: "coverage", Passed: true, Detail: "ok"},
			{Name: "dag_closure", Passed: true, Detail: "ok"},
		},
	}
	hard, detail := classifyGateFailure(report)
	if hard {
		t.Errorf("all-passed report should yield hard=false; got true")
	}
	if detail != "" {
		t.Errorf("clean report should yield empty detail; got %q", detail)
	}
}

func TestClassifyGateFailure_SoftOnlyIsClean(t *testing.T) {
	// pending_fields_wellformed is the only SOFT check; failing it
	// alone must not flag hard.
	report := types.GateReport{
		Checks: []types.GateCheck{
			{Name: "pending_fields_wellformed", Passed: false, Detail: "pending field x"},
		},
	}
	hard, _ := classifyGateFailure(report)
	if hard {
		t.Errorf("soft-only failure should yield hard=false; got true")
	}
}

// TestClassifyGateFailure_MultiHardFailuresJoined pins the
// commit-55 Batch B fix: when several hard checks fail in one
// gate run, the detail string lists EVERY one (joined by "; ")
// instead of returning only the first. Pre-fix the single-check
// return caused N retry rounds for N simultaneous failures.
func TestClassifyGateFailure_MultiHardFailuresJoined(t *testing.T) {
	report := types.GateReport{
		Checks: []types.GateCheck{
			{Name: "coverage", Passed: false, Detail: "score=0.4 threshold=0.7"},
			{Name: "dag_closure", Passed: false, Detail: "missing edge A→B"},
			{Name: "budget_sanity", Passed: false, Detail: "files=2 min=5"},
			{Name: "pending_fields_wellformed", Passed: false, Detail: "ignored soft"},
		},
	}
	hard, detail := classifyGateFailure(report)
	if !hard {
		t.Fatalf("multi-hard failures should yield hard=true")
	}
	for _, want := range []string{"coverage", "dag_closure", "budget_sanity"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q missing failed check %q", detail, want)
		}
	}
	if strings.Contains(detail, "pending_fields_wellformed") {
		t.Errorf("soft check leaked into hard detail: %q", detail)
	}
	// Joined format = "name: detail; name: detail; …".
	parts := strings.Split(detail, "; ")
	if len(parts) != 3 {
		t.Errorf("expected 3 join parts; got %d in %q", len(parts), detail)
	}
}

// TestClassifyGateFailure_NewlineStripped pins the format-safety:
// a check Detail that contains \n should not break the joined
// retry-hint render — newlines are flattened to spaces.
func TestClassifyGateFailure_NewlineStripped(t *testing.T) {
	report := types.GateReport{
		Checks: []types.GateCheck{
			{Name: "coverage", Passed: false, Detail: "line1\nline2\rline3"},
		},
	}
	_, detail := classifyGateFailure(report)
	if strings.ContainsAny(detail, "\n\r") {
		t.Errorf("newlines should be flattened in joined detail; got %q", detail)
	}
}
