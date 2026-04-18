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

// TestAddRepair_ExpandSearch_BumpsStats is the B1 producer
// regression: every RepairExpandSearch written to the closure MUST
// bump ClosureStats.ExpandSearchRaised (on top of the generic
// RepairsRaised counter) so the CGEC summary line surfaces
// "search coverage pressure" as a distinct observability column.
func TestAddRepair_ExpandSearch_BumpsStats(t *testing.T) {
	c := NewEvidenceClosure()
	c.AddRepair(RepairDirective{
		Kind:     RepairExpandSearch,
		Keywords: []string{"explorer", "skill"},
		Origin:   "phase0.broaden_exhausted",
	})
	s := c.Stats()
	if s.ExpandSearchRaised != 1 {
		t.Errorf("ExpandSearchRaised=%d, want 1", s.ExpandSearchRaised)
	}
	if s.RepairsRaised != 1 {
		t.Errorf("RepairsRaised=%d, want 1 (generic counter must also bump)", s.RepairsRaised)
	}
	if !s.HasActivity() {
		t.Error("HasActivity must be true after RepairExpandSearch raised")
	}
	// Dedup on repeat.
	c.AddRepair(RepairDirective{
		Kind:     RepairExpandSearch,
		Keywords: []string{"explorer", "skill"},
		Origin:   "phase0.broaden_exhausted",
	})
	if s2 := c.Stats(); s2.ExpandSearchRaised != 1 {
		t.Errorf("ExpandSearchRaised=%d after dedup, want 1", s2.ExpandSearchRaised)
	}
}

// TestAddRepair_SwapShape_BumpsStats is the B2 producer regression:
// every RepairSwapShape MUST bump ClosureStats.ShapeSwapRaised.
func TestAddRepair_SwapShape_BumpsStats(t *testing.T) {
	c := NewEvidenceClosure()
	c.AddRepair(RepairDirective{
		Kind:    RepairSwapShape,
		Subject: "from=boolean,to=value",
		Origin:  "emit_answer_document.shape_mismatch",
	})
	s := c.Stats()
	if s.ShapeSwapRaised != 1 {
		t.Errorf("ShapeSwapRaised=%d, want 1", s.ShapeSwapRaised)
	}
	if s.RepairsRaised != 1 {
		t.Errorf("RepairsRaised=%d, want 1", s.RepairsRaised)
	}
	// Distinct Subject = distinct directive, so another one bumps.
	c.AddRepair(RepairDirective{
		Kind:    RepairSwapShape,
		Subject: "from=explanation,to=value",
		Origin:  "pre_complete.subject_shape_mismatch",
	})
	if s2 := c.Stats(); s2.ShapeSwapRaised != 2 {
		t.Errorf("ShapeSwapRaised=%d, want 2 (distinct Subject)", s2.ShapeSwapRaised)
	}
}

// TestIsScanned_EmptySet_ReturnsTrue covers the D1/D4 back-compat
// pass-through: when ScannedSet has never been populated (old tests,
// analyzer-only dispatches, sub-agents that bypass keywordSearch),
// IsScanned MUST return true for every file so downstream
// enforcers do not spuriously flag files as ghost paths.
func TestIsScanned_EmptySet_ReturnsTrue(t *testing.T) {
	c := NewEvidenceClosure()
	if !c.IsScanned("internal/agent/explorer.go") {
		t.Error("empty ScannedSet must return true (back-compat)")
	}
	if !c.IsScanned("anything/at/all.go") {
		t.Error("empty ScannedSet must return true for every path")
	}
}

// TestIsScanned_WithSet_ReportsMembership covers D1 + D2 + D4
// membership test: once SetScannedSet populates the set, IsScanned
// returns true only for members, false for non-members.
func TestIsScanned_WithSet_ReportsMembership(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetScannedSet(map[string]bool{
		"internal/agent/explorer.go":       true,
		"internal/skill/defaults.go":       true,
		"internal/agent/subagent.go":       false, // value-false keys must NOT enter
	})
	if !c.IsScanned("internal/agent/explorer.go") {
		t.Error("file present in set must return true")
	}
	if !c.IsScanned("internal/skill/defaults.go") {
		t.Error("file present in set must return true")
	}
	if c.IsScanned("internal/agent/subagent.go") {
		t.Error("file with value-false entry must NOT count as scanned")
	}
	if c.IsScanned("ghost/path.go") {
		t.Error("file absent from non-empty set must return false")
	}
}

// TestScannedSet_DefensiveCopy verifies that ScannedSet returns a
// defensive copy so callers cannot mutate the internal state by
// writing to the returned map.
func TestScannedSet_DefensiveCopy(t *testing.T) {
	c := NewEvidenceClosure()
	c.SetScannedSet(map[string]bool{"a.go": true})
	snap := c.ScannedSet()
	if snap == nil || !snap["a.go"] {
		t.Fatal("expected a.go in snapshot")
	}
	// Mutate the snapshot — must not leak.
	snap["b.go"] = true
	if c.IsScanned("b.go") {
		t.Error("snapshot mutation leaked into closure state")
	}
}

// TestAddRepair_RebindSubject_StillCountsRepairsRaised asserts that
// the generic RepairsRaised counter bumps for every kind, including
// RepairRebindSubject and RepairForceCompleteDowngrade — those two
// have no per-kind counter (they reuse RepairsRaised + StallHardHits
// respectively), so the generic bump is their only observability
// signal via ClosureStats.
func TestAddRepair_RebindSubject_StillCountsRepairsRaised(t *testing.T) {
	c := NewEvidenceClosure()
	c.AddRepair(RepairDirective{
		Kind:    RepairRebindSubject,
		Subject: "skill_name",
		Origin:  "chain_ranker",
	})
	c.AddRepair(RepairDirective{
		Kind:      RepairForceCompleteDowngrade,
		Rationale: "3 identical fingerprints",
		Origin:    "convergence_detector",
	})
	s := c.Stats()
	if s.RepairsRaised != 2 {
		t.Errorf("RepairsRaised=%d, want 2 (generic counter must fire for every kind)", s.RepairsRaised)
	}
	if s.ExpandSearchRaised != 0 || s.ShapeSwapRaised != 0 {
		t.Errorf("per-kind counters must not fire for other kinds: ExpandSearch=%d ShapeSwap=%d", s.ExpandSearchRaised, s.ShapeSwapRaised)
	}
}
