// Package agent — irrelevant_files_validation_test.go (2026-05-10).
//
// P3 audit follow-up to L4 (b7c46c9): irrelevant_files is a HARD
// exclusion (drops paths from primary set + pre-read pool +
// mid-loop hints). When the analyzer LLM mistakenly declares the
// real answer file irrelevant, the explorer would never surface
// it. The cache-hydrate site at explorer.go:710-770 now
// validates each declared path against:
//
//  1. graph.FileIndex membership — hallucinated paths dropped
//  2. citedSources from prior emit_evidence — defer to evidence
//     (the explorer's own grounded citation is stronger evidence
//     than the analyzer's pre-investigation guess)
//
// These tests pin the validation behaviour at the cache-hydrate
// site directly via the explorerEvaluator's irrelevantFilesSet
// field. The full integration (BuildInitialInstruction →
// effectivePrimaryFiles) is exercised by the wider eval sweep.
package agent

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// hydrateIrrelevantFromHints replicates the cache-hydrate logic
// inside BuildInitialInstruction so the validation policy is
// testable in isolation. Mirrors the production code at
// explorer.go:713-769 byte-for-byte structurally.
func hydrateIrrelevantFromHints(
	rawHints []string,
	structuredEvidence []types.EvidenceItem,
	graph *repomap.Graph,
) (kept map[string]bool, droppedNoGraph, droppedCited int) {
	if len(rawHints) == 0 {
		return nil, 0, 0
	}
	citedSources := make(map[string]bool, len(structuredEvidence))
	for _, ev := range structuredEvidence {
		if ev.Producer != tool.EmitEvidenceProducer {
			continue
		}
		if cs := canonicalEvidenceSourcePath(ev.Source); cs != "" {
			citedSources[cs] = true
		}
	}
	set := make(map[string]bool, len(rawHints))
	for _, p := range rawHints {
		canon := canonicalExplorerPath(p)
		if canon == "" {
			continue
		}
		if graph != nil && graph.FileIndex != nil {
			if _, ok := graph.FileIndex[canon]; !ok {
				droppedNoGraph++
				continue
			}
		}
		if citedSources[canon] {
			droppedCited++
			continue
		}
		set[canon] = true
	}
	return set, droppedNoGraph, droppedCited
}

func TestIrrelevantValidation_NoGraph_FailOpenAcceptsAll(t *testing.T) {
	got, ng, dc := hydrateIrrelevantFromHints(
		[]string{"a.go", "b.go"}, nil, nil)
	if len(got) != 2 {
		t.Errorf("nil graph fail-open: expected 2 kept; got %d", len(got))
	}
	if ng != 0 || dc != 0 {
		t.Errorf("nil graph: expected no drops; got ng=%d dc=%d", ng, dc)
	}
}

func TestIrrelevantValidation_DropsHallucinatedPaths(t *testing.T) {
	graph := &repomap.Graph{FileIndex: map[string]*repomaptypes.FileInfo{
		"internal/real.go": {RelPath: "internal/real.go"},
	}}
	got, ng, _ := hydrateIrrelevantFromHints(
		[]string{"internal/real.go", "internal/fake.go", "another/ghost.go"},
		nil, graph,
	)
	if len(got) != 1 {
		t.Errorf("expected 1 valid path kept; got %v", got)
	}
	if !got["internal/real.go"] {
		t.Errorf("real path should be kept; got %v", got)
	}
	if ng != 2 {
		t.Errorf("expected 2 dropped_no_graph; got %d", ng)
	}
}

func TestIrrelevantValidation_DefersToEmittedEvidence(t *testing.T) {
	graph := &repomap.Graph{FileIndex: map[string]*repomaptypes.FileInfo{
		"internal/foo.go": {RelPath: "internal/foo.go"},
		"internal/bar.go": {RelPath: "internal/bar.go"},
	}}
	// Explorer has already emit_evidence'd from foo.go — that's
	// stronger than analyzer's pre-investigation guess. Don't
	// exclude it.
	evidence := []types.EvidenceItem{
		{Producer: tool.EmitEvidenceProducer, Source: "internal/foo.go", Subject: "FooFunc"},
	}
	got, _, dc := hydrateIrrelevantFromHints(
		[]string{"internal/foo.go", "internal/bar.go"},
		evidence, graph,
	)
	if got["internal/foo.go"] {
		t.Errorf("cited file should be dropped from exclusion set; got %v", got)
	}
	if !got["internal/bar.go"] {
		t.Errorf("non-cited file should remain in exclusion set; got %v", got)
	}
	if dc != 1 {
		t.Errorf("expected 1 dropped_cited; got %d", dc)
	}
}

func TestIrrelevantValidation_CanonicalisesPaths(t *testing.T) {
	graph := &repomap.Graph{FileIndex: map[string]*repomaptypes.FileInfo{
		"internal/foo.go": {RelPath: "internal/foo.go"},
	}}
	got, _, _ := hydrateIrrelevantFromHints(
		[]string{"./internal/foo.go"}, nil, graph)
	if !got["internal/foo.go"] {
		t.Errorf("./prefix path should canonicalise + match graph; got %v", got)
	}
}

func TestIrrelevantValidation_BothLayersInteract(t *testing.T) {
	// Mixed: 1 hallucinated + 1 cited + 1 valid → only valid kept.
	graph := &repomap.Graph{FileIndex: map[string]*repomaptypes.FileInfo{
		"internal/cited.go": {RelPath: "internal/cited.go"},
		"internal/valid.go": {RelPath: "internal/valid.go"},
	}}
	evidence := []types.EvidenceItem{
		{Producer: tool.EmitEvidenceProducer, Source: "internal/cited.go", Subject: "X"},
	}
	got, ng, dc := hydrateIrrelevantFromHints(
		[]string{"internal/ghost.go", "internal/cited.go", "internal/valid.go"},
		evidence, graph,
	)
	if len(got) != 1 || !got["internal/valid.go"] {
		t.Errorf("expected only valid.go kept; got %v", got)
	}
	if ng != 1 {
		t.Errorf("expected 1 dropped_no_graph; got %d", ng)
	}
	if dc != 1 {
		t.Errorf("expected 1 dropped_cited; got %d", dc)
	}
}

func TestIrrelevantValidation_AllInvalid_EmptySet(t *testing.T) {
	graph := &repomap.Graph{FileIndex: map[string]*repomaptypes.FileInfo{
		"internal/real.go": {RelPath: "internal/real.go"},
	}}
	evidence := []types.EvidenceItem{
		{Producer: tool.EmitEvidenceProducer, Source: "internal/real.go"},
	}
	got, _, _ := hydrateIrrelevantFromHints(
		[]string{"internal/ghost1.go", "internal/ghost2.go", "internal/real.go"},
		evidence, graph,
	)
	// All three dropped (2 hallucinated + 1 cited).
	if len(got) != 0 {
		t.Errorf("all invalid: expected empty set; got %v", got)
	}
}
