package types

import (
	"strings"
	"testing"
)

func TestMutableStateMergeExploreForkPreservesDistinctSameAnchorSummaries(t *testing.T) {
	mu := NewMutableState("列出 Kind 常量")
	weak := EvidenceItem{
		Kind:            EvidenceDirect,
		Subject:         "KindSymbolPresent",
		Predicate:       "defines",
		Source:          "internal/analysis/criterion/grammar.go",
		LineStart:       29,
		AnchorSymbol:    "KindSymbolPresent",
		AnchorKind:      AnchorDefinition,
		Scope:           ScopeLine,
		GroundingStatus: GroundingGrounded,
		Summary:         "KindSymbolPresent = Kind(types.CritSymbolPresent)",
	}
	weak.ID = StableEvidenceID(weak)
	rich := weak
	rich.Summary = "read-mode Kind: 检查符号是否存在于证据槽或答案列表"

	forkA := mu.ForkForExploreDispatch()
	forkA.AppendEvidence([]EvidenceItem{weak})
	mu.MergeExploreFork(forkA)

	forkB := mu.ForkForExploreDispatch()
	forkB.AppendEvidence([]EvidenceItem{rich})
	mu.MergeExploreFork(forkB)

	ev := mu.EmittedEvidence()
	if len(ev) != 1 {
		t.Fatalf("merged evidence count = %d, want 1: %+v", len(ev), ev)
	}
	if !strings.Contains(ev[0].Summary, weak.Summary) || !strings.Contains(ev[0].Summary, rich.Summary) {
		t.Fatalf("parallel fork merge lost same-anchor summaries: %q", ev[0].Summary)
	}
}
