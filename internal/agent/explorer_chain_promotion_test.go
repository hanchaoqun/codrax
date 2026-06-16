package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApplyChainPromotion_DropsChainsAnchoredOutsideReadSet is the
// CGEC-I1 regression. The unfiltered concreteValuesResult contains
// two chains: one anchored in a file Turn A read, and one anchored
// in a file Turn A did NOT read. After promotion, the second chain
// must be removed from BOTH the markdown and the evidence slice.
//
// X1 update (2026-05-06): a chain whose ENTIRE anchor file set is
// outside readSet is "off-target for this run" — promoting its
// anchor wastes a forced-read on a file the investigator chose to
// skip. The test asserts the new semantic: demoted chain dropped
// from markdown / evidence (still load-bearing); PendingRead for
// the off-target anchor is NOT enqueued (the X1 short-circuit).
// Partial-anchor demotion (chain anchors at [a.go, b.go] with a.go
// inside readSet, b.go outside) continues to enqueue PendingRead
// for b.go — see TestApplyChainPromotion_PartialAnchorMissing_Demoted.
func TestApplyChainPromotion_DropsChainsAnchoredOutsideReadSet(t *testing.T) {
	chainKept := "`A.foo()` binds NewB → `B.bar()` returns \"keep\""
	chainDropped := "`X.foo()` binds NewY → `Y.bar()` returns \"explorer\""

	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n" +
			"These chains trace through the concrete values to resolve conditions:\n\n" +
			"- " + chainKept + "\n" +
			"- " + chainDropped + "\n\n" +
			"### Some Other Section\n\nfoo\n",
		evidence: []types.EvidenceItem{
			{
				Kind:      types.EvidenceDataflowPath,
				Predicate: "resolution_chain",
				Subject:   chainKept,
				Summary:   chainKept,
				Source:    "internal/skill/defaults.go",
			},
			{
				Kind:      types.EvidenceDataflowPath,
				Predicate: "resolution_chain",
				Subject:   chainDropped,
				Summary:   chainDropped,
				Source:    "internal/agent/subagent.go",
			},
			{
				// non-chain evidence item — must be left untouched.
				Kind:      types.EvidenceConcrete,
				Predicate: "returns",
				Source:    "internal/some/other.go",
				Summary:   "unrelated concrete value",
			},
		},
		chainAnchors: []chainAnchorInfo{
			{Summary: chainKept, Files: []string{"internal/skill/defaults.go"}, Origin: "concrete_values_tracer"},
			{Summary: chainDropped, Files: []string{"internal/agent/subagent.go"}, Origin: "concrete_values_tracer"},
		},
	}
	readSet := map[string]bool{
		"internal/skill/defaults.go": true,
		// internal/agent/subagent.go intentionally absent
	}
	closure := types.NewEvidenceClosure("")

	out := applyChainPromotion(in, readSet, closure, "", 0, nil)

	// Markdown: chainKept retained, chainDropped removed.
	if !strings.Contains(out.markdown, chainKept) {
		t.Errorf("kept chain missing from markdown:\n%s", out.markdown)
	}
	if strings.Contains(out.markdown, chainDropped) {
		t.Errorf("demoted chain still in markdown:\n%s", out.markdown)
	}
	// Other markdown sections must be intact.
	if !strings.Contains(out.markdown, "### Some Other Section") {
		t.Errorf("non-chain section was clobbered:\n%s", out.markdown)
	}

	// Evidence: kept chain item present, demoted chain item absent,
	// unrelated concrete-value item retained.
	var keptFound, droppedFound, concreteFound bool
	for _, it := range out.evidence {
		switch {
		case it.Subject == chainKept:
			keptFound = true
		case it.Subject == chainDropped:
			droppedFound = true
		case it.Kind == types.EvidenceConcrete:
			concreteFound = true
		}
	}
	if !keptFound {
		t.Errorf("kept chain dropped from evidence")
	}
	if droppedFound {
		t.Errorf("demoted chain still in evidence")
	}
	if !concreteFound {
		t.Errorf("non-chain concrete-value evidence was incorrectly dropped")
	}

	// PendingReads: subagent.go must NOT be queued — X1 short-
	// circuits "anchor entirely outside readSet" demoted chains
	// because the investigator did not open the file at all
	// (forced-read would burn budget on an off-target location).
	pendings := closure.PendingReads()
	for _, p := range pendings {
		if p.File == "internal/agent/subagent.go" {
			t.Errorf("X1 should suppress PendingRead for an off-target anchor (anchor entirely outside readSet), got %v", pendings)
		}
	}
}

// Conservative behaviour: when ALL chain anchors are in ReadSet,
// promotion is a no-op and the input result is returned unchanged.
func TestApplyChainPromotion_AllAnchorsRead_NoOp(t *testing.T) {
	chain := "`A.foo()` returns \"x\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain, Source: "internal/foo/bar.go"},
		},
		chainAnchors: []chainAnchorInfo{
			{Summary: chain, Files: []string{"internal/foo/bar.go"}, Origin: "concrete_values_tracer"},
		},
	}
	readSet := map[string]bool{"internal/foo/bar.go": true}
	closure := types.NewEvidenceClosure("")

	out := applyChainPromotion(in, readSet, closure, "", 0, nil)

	if !strings.Contains(out.markdown, chain) {
		t.Errorf("expected chain retained, got markdown:\n%s", out.markdown)
	}
	if len(out.evidence) != 1 {
		t.Errorf("expected 1 evidence item retained, got %d", len(out.evidence))
	}
	if len(closure.PendingReads()) != 0 {
		t.Errorf("expected no PendingReads, got %v", closure.PendingReads())
	}
}

// Conservative: a concrete-values chain whose anchor list is
// ONE-IN-ONE-OUT (one anchor read, the other not) is demoted entirely.
// The chain depends on both anchors being legitimate cite sources, but
// the missing new file is not forced into exploration because the chain
// came from a broad deterministic tracer rather than a model-authored
// citation.
func TestApplyChainPromotion_PartialAnchorMissing_Demoted(t *testing.T) {
	chain := "`A.foo()` binds NewB → `B.bar()` returns \"x\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain},
		},
		chainAnchors: []chainAnchorInfo{
			{Summary: chain, Files: []string{"internal/a.go", "internal/b.go"}, FileLines: []int{10, 42}, Origin: "concrete_values_tracer"},
		},
	}
	readSet := map[string]bool{"internal/a.go": true} // b.go missing
	closure := types.NewEvidenceClosure("")

	out := applyChainPromotion(in, readSet, closure, "", 0, nil)

	if strings.Contains(out.markdown, chain) {
		t.Errorf("partial-miss chain should be demoted from markdown")
	}
	if len(out.evidence) != 0 {
		t.Errorf("partial-miss chain should be dropped from evidence, got %v", out.evidence)
	}
	pendings := closure.PendingReads()
	if len(pendings) != 0 {
		t.Errorf("concrete-values missing new file should be demoted without forced-read, got %v", pendings)
	}
}

func TestApplyChainPromotion_NonConcretePartialAnchorQueuesPendingRead(t *testing.T) {
	chain := "`A.foo()` binds NewB → `B.bar()` returns \"x\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain},
		},
		chainAnchors: []chainAnchorInfo{
			{Summary: chain, Files: []string{"internal/a.go", "internal/b.go"}, Origin: "bridge_literal"},
		},
	}
	readSet := map[string]bool{"internal/a.go": true}
	closure := types.NewEvidenceClosure("")

	_ = applyChainPromotion(in, readSet, closure, "", 0, nil)

	pendings := closure.PendingReads()
	if len(pendings) != 1 || pendings[0].File != "internal/b.go" {
		t.Fatalf("expected one PendingRead for non-concrete missing anchor, got %v", pendings)
	}
	if len(pendings[0].LineRanges) != 0 {
		t.Errorf("zero-line non-concrete anchor should preserve legacy full-file fallback, got %+v", pendings[0].LineRanges)
	}
}

func TestApplyChainPromotion_ConsumerGateQueuesUnreadUsageAnchor(t *testing.T) {
	chain := "`BaseAgent.buildToolSchemas()` gates on SubAgents.Get(...) — registry populated by `RegisterDefaultSubAgents()` binding NewSubExplorer → `SubExplorer.Name()` returns \"explorer\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain, Source: "internal/agent/agent.go"},
		},
		chainAnchors: []chainAnchorInfo{
			{
				Summary:   chain,
				Files:     []string{"internal/agent/agent.go"},
				FileLines: []int{3412},
				IsDefFile: []bool{false},
				Origin:    "consumer_gate",
			},
		},
	}
	closure := types.NewEvidenceClosure("")
	closure.SetScannedSet(map[string]bool{"internal/agent/agent.go": true})

	_ = applyChainPromotion(in, map[string]bool{}, closure, "", 0, nil)

	pendings := closure.PendingReads()
	if len(pendings) != 1 || pendings[0].File != "internal/agent/agent.go" {
		t.Fatalf("consumer_gate should queue its unread gate source despite X1/X2, got %v", pendings)
	}
	if pendings[0].Origin != "chain_promotion.consumer_gate" {
		t.Fatalf("pending origin = %q, want chain_promotion.consumer_gate", pendings[0].Origin)
	}
	if len(pendings[0].LineRanges) == 0 {
		t.Fatalf("line-backed consumer_gate should request a surgical read, got %+v", pendings[0])
	}
}

func TestApplyChainPromotion_NonConcreteLineAnchorQueuesSurgicalRead(t *testing.T) {
	chain := "`A.foo()` binds NewB → `B.bar()` returns \"x\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain},
		},
		chainAnchors: []chainAnchorInfo{
			{Summary: chain, Files: []string{"internal/a.go", "internal/b.go"}, FileLines: []int{0, 42}, Origin: "bridge_literal"},
		},
	}
	readSet := map[string]bool{"internal/a.go": true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)

	_ = applyChainPromotion(in, readSet, closure, "", 0, nil)

	pendings := closure.PendingReads()
	if len(pendings) != 1 || pendings[0].File != "internal/b.go" {
		t.Fatalf("expected one PendingRead for non-concrete missing line anchor, got %v", pendings)
	}
	if len(pendings[0].LineRanges) == 0 {
		t.Fatalf("non-concrete line anchor should use surgical read, got %+v", pendings[0])
	}
	if got := pendings[0].LineRanges[0]; got.Start > 42 || got.End < 42 {
		t.Errorf("surgical range must cover anchor line 42, got %+v", got)
	}
}

// TestApplyChainPromotion_SkipsPendingSubRepoAnchor is the C
// regression for the 2026-05-16 finalize-loop fix. Even when L1
// stops pre-scan from emitting pending sub-repo paths, a SymbolLocator
// fan-out (residual A) or some future ranker bug could still resolve
// a chain anchor inside a pending sub-repo. chain_promotion must
// refuse to queue forced-reads on those paths, otherwise the
// explorer hits the same MultiRepoActiveSetGater refusal → waiver
// escape → finalize lockup chain through a different door.
func TestApplyChainPromotion_SkipsPendingSubRepoAnchor(t *testing.T) {
	chain := "`Active.foo()` binds NewPending → `Pending.bar()` returns \"x\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain},
		},
		chainAnchors: []chainAnchorInfo{
			{
				Summary: chain,
				Files:   []string{"active-repo/internal/a.go", "pending-repo/internal/b.go"},
				Origin:  "bridge_literal",
			},
		},
	}
	readSet := map[string]bool{"active-repo/internal/a.go": true}
	closure := types.NewEvidenceClosure("")

	out := applyChainPromotion(in, readSet, closure, "", 0, []string{"pending-repo"})

	// Even though pending/b.go was missing from readSet (would normally
	// trigger a PendingRead in the partial-anchor case), the pending-
	// sub-repo guard must drop it.
	pendings := closure.PendingReads()
	for _, p := range pendings {
		if p.File == "pending-repo/internal/b.go" {
			t.Errorf("pending-sub-repo anchor MUST NOT be queued as PendingRead, got %+v", p)
		}
	}
	// Sanity: the markdown chain was still demoted (its other anchor
	// was missing from readSet too at the chain level; demotion is
	// independent of the pending-sub-repo guard). This is informational
	// — what matters is no forced-read on the pending path.
	_ = out
}

// Closure==nil path returns input unchanged (caching contract).
func TestApplyChainPromotion_NilClosure_Identity(t *testing.T) {
	in := concreteValuesResult{
		markdown:     "stuff",
		evidence:     []types.EvidenceItem{{Kind: types.EvidenceDataflowPath, Subject: "x"}},
		chainAnchors: []chainAnchorInfo{{Summary: "x", Files: []string{"a.go"}}},
	}
	out := applyChainPromotion(in, nil, nil, "", 0, nil)
	if out.markdown != in.markdown {
		t.Errorf("nil closure must not mutate markdown")
	}
	if len(out.evidence) != len(in.evidence) {
		t.Errorf("nil closure must not mutate evidence")
	}
}

// filterResolutionChainSection is the markdown surgery primitive;
// test it can handle the section appearing at the start, middle,
// and end of the document, and absence of the section.
func TestFilterResolutionChainSection_NoSection_Passthrough(t *testing.T) {
	md := "### Other\n\nfoo\n"
	got := filterResolutionChainSection(md, map[string]bool{"x": true})
	if got != md {
		t.Errorf("non-chain markdown should pass through unchanged:\n%q", got)
	}
}

func TestFilterResolutionChainSection_TerminalSection(t *testing.T) {
	md := "### Foo\n\nfoo\n\n### Resolution Chains\n\nintro\n\n- a\n- b\n"
	got := filterResolutionChainSection(md, map[string]bool{"a": true})
	if strings.Contains(got, "- a\n") {
		t.Errorf("demoted line still present: %q", got)
	}
	if !strings.Contains(got, "- b\n") {
		t.Errorf("kept line missing: %q", got)
	}
}
