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

func TestMutableStateEmittedEvidenceCompactsSameIDAmendments(t *testing.T) {
	mu := NewMutableState("修订证据")
	base := EvidenceItem{
		Kind:            EvidenceDirect,
		Subject:         "ProviderAuth.api",
		Predicate:       "defines",
		Source:          "packages/opencode/src/auth.ts",
		LineStart:       42,
		LineEnd:         42,
		AnchorSymbol:    "ProviderAuth",
		AnchorKind:      AnchorCall,
		Scope:           ScopeLine,
		GroundingStatus: GroundingGrounded,
		Summary:         "ProviderAuth.api participates in auth transport",
	}
	base.ID = StableEvidenceID(base)
	amended := base
	amended.AnchorKind = AnchorDefinition
	amended.SurfaceTerms = []string{"ProviderAuth.api"}
	amended.Summary = "ProviderAuth.api is the transport-facing definition used by auth.set"

	mu.AppendEvidence([]EvidenceItem{base})
	mu.AppendEvidence([]EvidenceItem{amended})

	full := mu.EmittedEvidence()
	if len(full) != 1 {
		t.Fatalf("full emitted evidence should compact same-ID amendments, got %d: %+v", len(full), full)
	}
	if full[0].AnchorKind != AnchorDefinition {
		t.Fatalf("anchor kind = %q, want corrected %q", full[0].AnchorKind, AnchorDefinition)
	}
	if !strings.Contains(full[0].Summary, base.Summary) || !strings.Contains(full[0].Summary, amended.Summary) {
		t.Fatalf("amendment merge lost summaries: %q", full[0].Summary)
	}
	if len(full[0].SurfaceTerms) != 1 || full[0].SurfaceTerms[0] != "ProviderAuth.api" {
		t.Fatalf("amendment merge lost surface terms: %+v", full[0].SurfaceTerms)
	}

	tail, total := mu.EmittedEvidenceSince(1)
	if total != 2 || len(tail) != 1 || tail[0].AnchorKind != AnchorDefinition {
		t.Fatalf("raw delta should still expose correction event, total=%d tail=%+v", total, tail)
	}
}
