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
// must be removed from BOTH the markdown and the evidence slice,
// and the missing file must show up as a PendingRead on the
// closure.
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
	closure := types.NewEvidenceClosure()

	out := applyChainPromotion(in, readSet, closure)

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

	// PendingReads: subagent.go must be queued.
	pendings := closure.PendingReads()
	var found bool
	for _, p := range pendings {
		if p.File == "internal/agent/subagent.go" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected PendingRead for internal/agent/subagent.go, got %v", pendings)
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
	closure := types.NewEvidenceClosure()

	out := applyChainPromotion(in, readSet, closure)

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

// Conservative: a chain whose anchor list is ONE-IN-ONE-OUT (one
// anchor read, the other not) is demoted entirely. The chain
// depends on both anchors being legitimate cite sources.
func TestApplyChainPromotion_PartialAnchorMissing_Demoted(t *testing.T) {
	chain := "`A.foo()` binds NewB → `B.bar()` returns \"x\""
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + chain + "\n\n",
		evidence: []types.EvidenceItem{
			{Kind: types.EvidenceDataflowPath, Predicate: "resolution_chain", Subject: chain, Summary: chain},
		},
		chainAnchors: []chainAnchorInfo{
			{Summary: chain, Files: []string{"internal/a.go", "internal/b.go"}, Origin: "concrete_values_tracer"},
		},
	}
	readSet := map[string]bool{"internal/a.go": true} // b.go missing
	closure := types.NewEvidenceClosure()

	out := applyChainPromotion(in, readSet, closure)

	if strings.Contains(out.markdown, chain) {
		t.Errorf("partial-miss chain should be demoted from markdown")
	}
	if len(out.evidence) != 0 {
		t.Errorf("partial-miss chain should be dropped from evidence, got %v", out.evidence)
	}
	pendings := closure.PendingReads()
	if len(pendings) != 1 || pendings[0].File != "internal/b.go" {
		t.Errorf("expected one PendingRead for internal/b.go, got %v", pendings)
	}
}

// Closure==nil path returns input unchanged (caching contract).
func TestApplyChainPromotion_NilClosure_Identity(t *testing.T) {
	in := concreteValuesResult{
		markdown:     "stuff",
		evidence:     []types.EvidenceItem{{Kind: types.EvidenceDataflowPath, Subject: "x"}},
		chainAnchors: []chainAnchorInfo{{Summary: "x", Files: []string{"a.go"}}},
	}
	out := applyChainPromotion(in, nil, nil)
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
