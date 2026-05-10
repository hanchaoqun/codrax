// Package agent — irrelevant_files_test.go (2026-05-10).
//
// L4 of the forced-read remediation: analyzer-declared
// irrelevant_files. The LLM may explicitly tell the system "I have
// inspected these files and they are off-topic — do not re-inject
// them." Honored across pre-read pools, primary-file selection, and
// mid-loop "Read these next:" hints.
package agent

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// === excludeReadAndIrrelevantFromCtx union ===

func TestExcludeReadAndIrrelevantFromCtx_NilCtx_NilSet_ReturnsNil(t *testing.T) {
	e := &explorerEvaluator{}
	if got := e.excludeReadAndIrrelevantFromCtx(nil); got != nil {
		t.Errorf("nil ctx + nil irrelevant: expected nil; got %v", got)
	}
}

func TestExcludeReadAndIrrelevantFromCtx_NilCtxButHasIrrelevant_ReturnsCopy(t *testing.T) {
	e := &explorerEvaluator{
		irrelevantFilesSet: map[string]bool{"a.go": true, "b.go": true},
	}
	got := e.excludeReadAndIrrelevantFromCtx(nil)
	if len(got) != 2 || !got["a.go"] || !got["b.go"] {
		t.Errorf("nil ctx but irrelevant set: expected copy of irrelevant; got %v", got)
	}
}

func TestExcludeReadAndIrrelevantFromCtx_UnionsBothSets(t *testing.T) {
	mut := types.NewMutableState("test")
	mut.EvidenceClosure().SetReadSet(map[string]bool{"read1.go": true, "read2.go": true})
	ctx := &types.AgentContext{Mutable: mut}
	e := &explorerEvaluator{
		irrelevantFilesSet: map[string]bool{"irrel.go": true, "read1.go": true},
	}
	got := e.excludeReadAndIrrelevantFromCtx(ctx)
	want := map[string]bool{"read1.go": true, "read2.go": true, "irrel.go": true}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("union should merge ReadSet + irrelevantFilesSet; got %v, want %v", got, want)
	}
}

// === effectivePrimaryFiles drops irrelevant ===

func TestEffectivePrimaryFiles_IrrelevantDropsHighConfHint(t *testing.T) {
	// LLM emits a high-confidence hint AND declares the same file
	// irrelevant — irrelevant wins (defensive contradiction handling).
	e := &explorerEvaluator{
		requiredFileHints: []types.RequiredFileHint{
			{Path: "internal/contradicted.go", Confidence: 0.9},
			{Path: "internal/clean.go", Confidence: 0.9},
		},
		irrelevantFilesSet: map[string]bool{
			"internal/contradicted.go": true,
		},
	}
	got := e.effectivePrimaryFiles()
	if len(got) != 1 || got[0] != "internal/clean.go" {
		t.Errorf("contradicted hint should be dropped by irrelevant; got %v", got)
	}
}

func TestEffectivePrimaryFiles_IrrelevantDropsCitedFile(t *testing.T) {
	// Defensive: if explorer cited a file BUT analyzer declared it
	// irrelevant, the analyzer's negative declaration wins.
	e := &explorerEvaluator{
		structuredEvidence: []types.EvidenceItem{
			{Producer: "explorer.emit_evidence", Source: "internal/foo.go"},
		},
		irrelevantFilesSet: map[string]bool{"internal/foo.go": true},
	}
	got := e.effectivePrimaryFiles()
	if len(got) != 0 {
		t.Errorf("irrelevant should drop cited file; got %v", got)
	}
}

// === Cross-Run safety: empty irrelevant set has no effect ===

func TestEffectivePrimaryFiles_EmptyIrrelevantSet_NoEffect(t *testing.T) {
	e := &explorerEvaluator{
		requiredFileHints: []types.RequiredFileHint{
			{Path: "a.go", Confidence: 0.9},
		},
		irrelevantFilesSet: nil,
	}
	got := e.effectivePrimaryFiles()
	if len(got) != 1 || got[0] != "a.go" {
		t.Errorf("nil irrelevant: expected hint to flow through; got %v", got)
	}
}
