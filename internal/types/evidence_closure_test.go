package types

import (
	"testing"
)

// TestAddRepair_ReadFile_MirrorsToPendingReads is the A1 invariant
// regression: AddRepair for RepairReadFile MUST simultaneously write
// the Files onto the PendingReads queue so runForcedReads (I4) can
// pick them up even after ConsumeRepairs drains the Repairs queue
// for prompt rendering. Without this mirror, the grounder's
// citation-drop feedback only reaches the prompt and an inattentive
// LLM can let the retry budget exhaust.
func TestAddRepair_ReadFile_MirrorsToPendingReads(t *testing.T) {
	c := NewEvidenceClosure()
	c.AddRepair(RepairDirective{
		Kind:      RepairReadFile,
		Files:     []string{"internal/agent/subagent.go", "internal/agent/agent.go"},
		Rationale: "citation cited but file is not in Turn A's ReadSet",
		Origin:    "emit_answer_document.grounder",
	})
	pr := c.PendingReads()
	if len(pr) != 2 {
		t.Fatalf("expected 2 PendingReads mirrored from AddRepair, got %d", len(pr))
	}
	seen := map[string]PendingRead{}
	for _, p := range pr {
		seen[p.File] = p
	}
	for _, want := range []string{"internal/agent/subagent.go", "internal/agent/agent.go"} {
		p, ok := seen[want]
		if !ok {
			t.Errorf("expected PendingRead for %q, not found", want)
			continue
		}
		if p.Rationale == "" {
			t.Errorf("PendingRead[%s]: Rationale dropped during mirror", want)
		}
		// Origin must be namespaced so the operator can tell mirror
		// entries apart from chain_promotion entries.
		if p.Origin != "auto_bridge.emit_answer_document.grounder" {
			t.Errorf("PendingRead[%s]: Origin=%q, want auto_bridge.emit_answer_document.grounder", want, p.Origin)
		}
	}
	// Repairs queue is still populated — ConsumeRepairs should still
	// render the RepairReadFile into the retry-hint prompt.
	if got := len(c.PendingRepairs()); got != 1 {
		t.Errorf("Repairs queue len=%d, want 1 (ConsumeRepairs must still see the directive)", got)
	}
}

// TestAddRepair_NonReadFile_DoesNotMirror asserts the A1 bridge is
// kind-specific: RepairExpandSearch / RepairSwapShape / etc. MUST
// NOT silently write PendingReads (those kinds have no file list
// and the semantics differ).
func TestAddRepair_NonReadFile_DoesNotMirror(t *testing.T) {
	c := NewEvidenceClosure()
	c.AddRepair(RepairDirective{
		Kind:     RepairExpandSearch,
		Keywords: []string{"foo", "bar"},
		Origin:   "phase0.broaden_failed",
	})
	c.AddRepair(RepairDirective{
		Kind:    RepairSwapShape,
		Subject: "from=config_value,to=value",
		Origin:  "emit_answer_document.shape_mismatch",
	})
	c.AddRepair(RepairDirective{
		Kind:    RepairRebindSubject,
		Subject: "skill_name",
		Origin:  "chain_ranker",
	})
	if pr := c.PendingReads(); len(pr) != 0 {
		t.Errorf("non-RepairReadFile directives must not write PendingReads, got %d entries", len(pr))
	}
}

// TestAddRepair_ReadFile_Dedupe asserts the mirror respects the
// existing repair-level dedupe (Kind + Subject + Files), so the
// same file path raised multiple times does not inflate either
// queue. This matches the MergeRepairs contract — the prompt
// should render the directive once, not N times.
func TestAddRepair_ReadFile_Dedupe(t *testing.T) {
	c := NewEvidenceClosure()
	for i := 0; i < 3; i++ {
		c.AddRepair(RepairDirective{
			Kind:   RepairReadFile,
			Files:  []string{"a.go"},
			Origin: "grounder",
		})
	}
	if pr := c.PendingReads(); len(pr) != 1 {
		t.Errorf("repeated AddRepair with same Files must dedupe PendingReads, got %d", len(pr))
	}
	// Distinct files produce distinct PendingReads.
	c.AddRepair(RepairDirective{
		Kind:   RepairReadFile,
		Files:  []string{"b.go"},
		Origin: "grounder",
	})
	if pr := c.PendingReads(); len(pr) != 2 {
		t.Errorf("distinct files must produce distinct PendingReads, got %d", len(pr))
	}
}
