package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestMergeEvidenceItemsIfChangedSkipsNoopSnapshot(t *testing.T) {
	item := types.EvidenceItem{
		Kind:       types.EvidenceDirect,
		Subject:    "A",
		Predicate:  "calls",
		Object:     "B",
		Source:     "a.go",
		LineStart:  10,
		Confidence: 0.8,
		Producer:   "explorer.emit_evidence",
	}
	existing := mergeEvidenceItems([]types.EvidenceItem{item})

	got, changed := MergeEvidenceItemsIfChanged(existing, []types.EvidenceItem{item})
	if changed {
		t.Fatal("expected no-op snapshot to skip canonical merge")
	}
	if len(got) != 1 || got[0].ID == "" {
		t.Fatalf("unexpected merged evidence: %+v", got)
	}
}

func TestMergeEvidenceItemsIfChangedFallsBackOnEnrichment(t *testing.T) {
	existing := mergeEvidenceItems([]types.EvidenceItem{{
		Kind:       types.EvidenceDirect,
		Subject:    "A",
		Predicate:  "calls",
		Object:     "B",
		Source:     "a.go",
		LineStart:  10,
		Confidence: 0.7,
	}})
	incoming := existing[0]
	incoming.Confidence = 0.95
	incoming.SurfaceTerms = []string{"A"}

	got, changed := MergeEvidenceItemsIfChanged(existing, []types.EvidenceItem{incoming})
	if !changed {
		t.Fatal("expected confidence/surface-term enrichment to use canonical merge")
	}
	if got[0].Confidence != 0.95 {
		t.Fatalf("confidence was not preserved through canonical merge: %+v", got[0])
	}
	if len(got[0].SurfaceTerms) != 1 || got[0].SurfaceTerms[0] != "A" {
		t.Fatalf("surface terms were not merged: %+v", got[0].SurfaceTerms)
	}
}

func TestMergeEvidenceItemsCarriesExplicitSalience(t *testing.T) {
	base := types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Subject:      "A",
		Predicate:    "calls",
		Object:       "B",
		Source:       "a.go",
		LineStart:    10,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "B",
		Producer:     "dataflow.engine",
		Salience:     types.SalienceContext,
	}
	incoming := base
	incoming.Producer = "explorer.emit_evidence"
	incoming.Salience = types.SalienceLoadBearing

	got := mergeEvidenceItems([]types.EvidenceItem{base}, []types.EvidenceItem{incoming})
	if len(got) != 1 {
		t.Fatalf("got %d items, want 1", len(got))
	}
	if got[0].Salience != types.SalienceLoadBearing {
		t.Fatalf("salience = %q, want load_bearing", got[0].Salience)
	}

	unset := base
	unset.Salience = types.SalienceUnset
	got = mergeEvidenceItems([]types.EvidenceItem{incoming}, []types.EvidenceItem{unset})
	if got[0].Salience != types.SalienceLoadBearing {
		t.Fatalf("unset merge must not clear explicit salience, got %q", got[0].Salience)
	}
}

func TestMergeFlowFindingsIfChangedSkipsNoopSnapshot(t *testing.T) {
	item := types.FlowFindingDigest{
		Path:       []string{"A", "B"},
		Conditions: []string{"ready"},
		Sources:    []string{"a.go:10"},
		Sinks:      []string{"b.go:20"},
		Confidence: 0.8,
	}
	existing := mergeFlowFindings([]types.FlowFindingDigest{item})

	got, changed := MergeFlowFindingsIfChanged(existing, []types.FlowFindingDigest{item})
	if changed {
		t.Fatal("expected no-op flow snapshot to skip canonical merge")
	}
	if len(got) != 1 || got[0].ID == "" {
		t.Fatalf("unexpected merged findings: %+v", got)
	}
}
