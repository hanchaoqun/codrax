package agent

import (
	"reflect"
	"sort"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestMergeTurnAArtifactsWithPrior_NilPriorReturnsCurrent is the
// single-window baseline: a first-window dispatch has no prior snapshot,
// so the merge must be a no-op.
func TestMergeTurnAArtifactsWithPrior_NilPriorReturnsCurrent(t *testing.T) {
	current := types.TurnAArtifacts{
		UserQuestion:          "what does Foo return?",
		InvestigationNotes:    []string{"iter 1"},
		ReadFiles:             []string{"a.go"},
		ToolResults:           []types.ToolResult{{ToolName: "grep", Success: true}},
		EvidenceItems:         []types.EvidenceItem{{ID: "ev1", Summary: "a.go:1"}},
		FlowFindings:          []types.FlowFindingDigest{{ID: "ff1"}},
		TerminalEvidenceCount: 2,
	}
	got := mergeTurnAArtifactsWithPrior(nil, current)
	if !reflect.DeepEqual(got, current) {
		t.Errorf("nil prior must return current unchanged; got %+v", got)
	}
}

// TestMergeTurnAArtifactsWithPrior_AccumulatesAcrossWindows is the
// primary bug-fix lock: Window 2 that only greps (no read_file) must not
// erase Window 1's rich ReadFiles / ToolResults / EvidenceItems /
// FlowFindings snapshot.
func TestMergeTurnAArtifactsWithPrior_AccumulatesAcrossWindows(t *testing.T) {
	prior := &types.TurnAArtifacts{
		UserQuestion:          "what does Foo return?",
		ReadFiles:             []string{"a.go", "b.go"},
		ToolResults:           []types.ToolResult{{ToolName: "read_file", Summary: "a.go"}},
		EvidenceItems:         []types.EvidenceItem{{ID: "ev1", Summary: "a.go:1"}},
		FlowFindings:          []types.FlowFindingDigest{{ID: "ff1"}},
		TerminalEvidenceCount: 3,
	}
	current := types.TurnAArtifacts{
		UserQuestion:          "what does Foo return?",
		ReadFiles:             nil,
		ToolResults:           []types.ToolResult{{ToolName: "grep", Summary: "repomap"}},
		EvidenceItems:         []types.EvidenceItem{{ID: "ev2", Summary: "c.go:1"}},
		FlowFindings:          []types.FlowFindingDigest{{ID: "ff2"}},
		TerminalEvidenceCount: 0,
	}
	got := mergeTurnAArtifactsWithPrior(prior, current)

	// ReadFiles must include both prior and current; the critical case
	// is prior non-empty + current empty, which used to erase prior.
	sort.Strings(got.ReadFiles)
	if !reflect.DeepEqual(got.ReadFiles, []string{"a.go", "b.go"}) {
		t.Errorf("ReadFiles after merge = %v, want [a.go b.go]", got.ReadFiles)
	}
	if len(got.ToolResults) != 2 {
		t.Errorf("ToolResults len = %d, want 2 (concat)", len(got.ToolResults))
	}
	if got.ToolResults[0].ToolName != "read_file" || got.ToolResults[1].ToolName != "grep" {
		t.Errorf("ToolResults order not prior-then-current: %+v", got.ToolResults)
	}
	if len(got.EvidenceItems) != 2 {
		t.Errorf("EvidenceItems len = %d, want 2 (union by ID)", len(got.EvidenceItems))
	}
	if len(got.FlowFindings) != 2 {
		t.Errorf("FlowFindings len = %d, want 2 (union by ID)", len(got.FlowFindings))
	}
	if got.TerminalEvidenceCount != 3 {
		t.Errorf("TerminalEvidenceCount = %d, want 3 (max must not regress)", got.TerminalEvidenceCount)
	}
}

func TestMergeTurnAArtifactsWithPrior_PreservesAcceptedClosureWhenCurrentEmpty(t *testing.T) {
	prior := &types.TurnAArtifacts{
		AcceptedClosureReason: "resolved 4 grounded sites after excluding comments",
		AcceptedResultKind:    "resolved",
		AcceptedAggregateFacts: []types.AnswerAggregateFact{{
			Kind:  types.AnswerAggregateUniqueCount,
			Label: "unique files",
			Value: "3",
		}},
		RuntimeObservationOnlyCompletion: true,
	}
	current := types.TurnAArtifacts{
		UserQuestion: "same question",
	}

	got := mergeTurnAArtifactsWithPrior(prior, current)
	if got.AcceptedClosureReason != prior.AcceptedClosureReason {
		t.Fatalf("AcceptedClosureReason = %q, want prior %q", got.AcceptedClosureReason, prior.AcceptedClosureReason)
	}
	if got.AcceptedResultKind != prior.AcceptedResultKind {
		t.Fatalf("AcceptedResultKind = %q, want prior %q", got.AcceptedResultKind, prior.AcceptedResultKind)
	}
	if len(got.AcceptedAggregateFacts) != 1 || got.AcceptedAggregateFacts[0].Value != "3" {
		t.Fatalf("AcceptedAggregateFacts = %+v, want prior", got.AcceptedAggregateFacts)
	}
	if !got.RuntimeObservationOnlyCompletion {
		t.Fatal("RuntimeObservationOnlyCompletion must survive closure-only retry windows")
	}
}

// TestMergeTurnAArtifactsWithPrior_EvidenceDedupesByID ensures an item
// reported in both windows collapses rather than doubling up — matching
// mergeEvidenceItems's documented contract.
func TestMergeTurnAArtifactsWithPrior_EvidenceDedupesByID(t *testing.T) {
	prior := &types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{ID: "ev1", Summary: "a.go:1", Confidence: 0.7},
		},
	}
	current := types.TurnAArtifacts{
		EvidenceItems: []types.EvidenceItem{
			{ID: "ev1", Summary: "a.go:1", Confidence: 0.9},
			{ID: "ev2", Summary: "b.go:1"},
		},
	}
	got := mergeTurnAArtifactsWithPrior(prior, current)
	if len(got.EvidenceItems) != 2 {
		t.Fatalf("EvidenceItems len = %d, want 2 (ev1 deduped)", len(got.EvidenceItems))
	}
	var ev1 types.EvidenceItem
	for _, e := range got.EvidenceItems {
		if e.ID == "ev1" {
			ev1 = e
		}
	}
	if ev1.Confidence != 0.9 {
		t.Errorf("on dedupe, higher confidence must win; got %.2f, want 0.90", ev1.Confidence)
	}
}

// TestMergeTurnAArtifactsWithPrior_TerminalCountNeverRegresses locks the
// decision rule: TerminalEvidenceCount is the cardinality baseline Turn B
// uses to validate completeness claims. A later window that filtered out
// chains (e.g. mechanism-kind filter drops all answer_chains) must not
// lower the baseline and thus silently relax the validator.
func TestMergeTurnAArtifactsWithPrior_TerminalCountNeverRegresses(t *testing.T) {
	prior := &types.TurnAArtifacts{TerminalEvidenceCount: 5}
	current := types.TurnAArtifacts{TerminalEvidenceCount: 2}
	if got := mergeTurnAArtifactsWithPrior(prior, current); got.TerminalEvidenceCount != 5 {
		t.Errorf("TerminalEvidenceCount = %d, want 5 (prior > current kept)", got.TerminalEvidenceCount)
	}
	// current > prior: use current
	prior = &types.TurnAArtifacts{TerminalEvidenceCount: 2}
	current = types.TurnAArtifacts{TerminalEvidenceCount: 5}
	if got := mergeTurnAArtifactsWithPrior(prior, current); got.TerminalEvidenceCount != 5 {
		t.Errorf("TerminalEvidenceCount = %d, want 5 (current > prior kept)", got.TerminalEvidenceCount)
	}
}
