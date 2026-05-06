package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestApplyChainPromotion_RangeAware_DemoteOutsideSlice is the
// Commit A regression: a chain whose terminal sits on a line outside
// every fetched range must be demoted even when the anchor file is
// in readSet. Without row-level enforcement the chain would have
// sailed through (the file was "touched") and misled Turn B.
func TestApplyChainPromotion_RangeAware_DemoteOutsideSlice(t *testing.T) {
	summary := "`A.foo()` binds NewB → `B.bar()` returns \"answer\""
	file := "internal/skill/defaults.go"
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + summary + "\n",
		evidence: []types.EvidenceItem{{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Subject:   summary,
			Summary:   summary,
			Source:    file,
		}},
		chainAnchors: []chainAnchorInfo{{
			Summary:   summary,
			Files:     []string{file},
			FileLines: []int{350}, // definition sits in an unread slice
			Origin:    "concrete_values_tracer",
		}},
	}
	readSet := map[string]bool{file: true} // file touched, partial read
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	closure.SetReadRanges(map[string][]types.LineRange{
		file: {{Start: 1, End: 50}, {Start: 100, End: 200}}, // excludes 350
	})

	out := applyChainPromotion(in, readSet, closure, "", 0)

	// The chain must disappear from markdown.
	if contains(out.markdown, summary) {
		t.Errorf("chain with anchor in unread slice must be demoted from markdown")
	}
	// Evidence item must be dropped.
	for _, it := range out.evidence {
		if it.Kind == types.EvidenceDataflowPath && it.Predicate == "resolution_chain" {
			t.Errorf("dataflow_path evidence must be dropped when chain is demoted")
		}
	}
	// PendingRead should be queued.
	if len(closure.PendingReads()) == 0 {
		t.Error("expected a PendingRead for the demoted chain's file")
	}
}

// TestApplyChainPromotion_RangeAware_PromoteInsideSlice: the mirror
// case — when the terminal line falls inside a fetched slice, the
// chain is kept even though only 30% of the file was read.
func TestApplyChainPromotion_RangeAware_PromoteInsideSlice(t *testing.T) {
	summary := "`A.foo()` binds NewB → `B.bar()` returns \"answer\""
	file := "internal/skill/defaults.go"
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + summary + "\n",
		evidence: []types.EvidenceItem{{
			Kind:      types.EvidenceDataflowPath,
			Predicate: "resolution_chain",
			Subject:   summary,
			Summary:   summary,
			Source:    file,
		}},
		chainAnchors: []chainAnchorInfo{{
			Summary:   summary,
			Files:     []string{file},
			FileLines: []int{150}, // inside [100, 200]
			Origin:    "concrete_values_tracer",
		}},
	}
	readSet := map[string]bool{file: true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	closure.SetReadRanges(map[string][]types.LineRange{
		file: {{Start: 1, End: 50}, {Start: 100, End: 200}},
	})

	out := applyChainPromotion(in, readSet, closure, "", 0)

	if !contains(out.markdown, summary) {
		t.Error("chain with anchor inside a fetched slice must be kept")
	}
	kept := 0
	for _, it := range out.evidence {
		if it.Kind == types.EvidenceDataflowPath && it.Predicate == "resolution_chain" {
			kept++
		}
	}
	if kept != 1 {
		t.Errorf("expected 1 kept dataflow_path item, got %d", kept)
	}
}

// TestApplyChainPromotion_RangeAware_ZeroLineFallsBackToFileLevel:
// when the anchor has no line metadata (FileLines[i]==0 or slice
// shorter than Files), the enforcer must degrade to the historical
// file-level check. Guards the backward-compatibility promise in
// dedupeAnchorFilesWithLines.
func TestApplyChainPromotion_RangeAware_ZeroLineFallsBackToFileLevel(t *testing.T) {
	summary := "bridge chain with no line metadata"
	file := "internal/agent/subagent.go"
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + summary + "\n",
		chainAnchors: []chainAnchorInfo{{
			Summary: summary,
			Files:   []string{file},
			// FileLines omitted — behaves like the legacy chainAnchorInfo.
			Origin: "bridge_literal",
		}},
	}
	readSet := map[string]bool{file: true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	// Even with ranges that exclude line 150, a zero-line anchor must
	// still promote via the file-level short-circuit.
	closure.SetReadRanges(map[string][]types.LineRange{
		file: {{Start: 1, End: 50}},
	})

	out := applyChainPromotion(in, readSet, closure, "", 0)
	if !contains(out.markdown, summary) {
		t.Error("zero-line anchor must fall back to file-level grant")
	}
}

// TestApplyChainPromotion_RangeAware_OldTestsStillPass guards the
// invariant that closures which never call SetReadRanges (every
// existing chain_promotion_test.go case) still behave exactly as
// before: file-level readSet alone decides kept vs demoted.
func TestApplyChainPromotion_RangeAware_OldTestsStillPass(t *testing.T) {
	summary := "`X.foo()` binds NewY → `Y.bar()` returns \"stale\""
	file := "internal/agent/subagent.go"
	in := concreteValuesResult{
		markdown: "### Resolution Chains\n\n- " + summary + "\n",
		chainAnchors: []chainAnchorInfo{{
			Summary:   summary,
			Files:     []string{file},
			FileLines: []int{42}, // line exists but no ranges registered
			Origin:    "concrete_values_tracer",
		}},
	}
	readSet := map[string]bool{file: true}
	closure := types.NewEvidenceClosure("")
	closure.SetReadSet(readSet)
	// No SetReadRanges call: closure.HasReadLine grants file-level.

	out := applyChainPromotion(in, readSet, closure, "", 0)
	if !contains(out.markdown, summary) {
		t.Error("closure without ranges must grant file-level coverage")
	}
}

