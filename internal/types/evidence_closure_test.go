package types

import (
	"testing"
)

// TestEvidenceClosure_AbsolutePathCanonicalizesAgainstRepoRoot pins
// the session-22 canonicalisation fix on the CGEC ReadSet layer.
// Before the fix, SetReadSet / HasRead / HasReadLine all called
// the package-local canonicalizeRepoPath helper which stripped
// "./" and normalised separators but did NOT strip an absolute
// repoRoot prefix. When the LLM passed an evidence source in
// absolute form (observed in the customer trace:
// /mnt/d/opt/codrax-main/internal/...) and the CGEC ReadSet was
// seeded with the repo-relative form from extractFileCoverage,
// HasRead(abs) missed and emit_investigation_complete's
// ReadSet-gate falsely reported the file as unread.
//
// After the fix, NewEvidenceClosure(repoRoot) threads the root
// into every canonicalisation site inside the closure, so both
// the stored ReadSet entries AND lookups on absolute paths land
// on the same repo-relative keys. This test pins both halves.
func TestEvidenceClosure_AbsolutePathCanonicalizesAgainstRepoRoot(t *testing.T) {
	c := NewEvidenceClosure("/mnt/d/opt/codrax-main")
	// Seed readSet with repo-relative form (mirrors what
	// extractFileCoverage now produces post-session-22).
	c.SetReadSet(map[string]bool{"internal/agent/agent.go": true})

	// Absolute-path lookup must resolve through the closure's
	// canonicaliser and hit the repo-relative key.
	if !c.HasRead("/mnt/d/opt/codrax-main/internal/agent/agent.go") {
		t.Errorf("HasRead with absolute path should strip repoRoot prefix and find the repo-relative key")
	}
	// Relative-path lookup still works (unchanged semantics).
	if !c.HasRead("internal/agent/agent.go") {
		t.Errorf("HasRead with repo-relative path should still work")
	}
	// Outside-repo absolute path should NOT spuriously match.
	if c.HasRead("/etc/passwd") {
		t.Errorf("HasRead on an outside-repo path must not match any ReadSet key")
	}

	// Symmetric half: AddReadRanges with an absolute path should
	// land on the same canonical repo-relative key so HasReadLine
	// lookups through the repo-relative form find the range.
	c.AddReadRanges(map[string][]LineRange{
		"/mnt/d/opt/codrax-main/internal/agent/agent.go": {{Start: 100, End: 200}},
	})
	if !c.HasReadLine("internal/agent/agent.go", 150) {
		t.Errorf("HasReadLine with repo-relative form should find range added with absolute path")
	}
	if !c.HasReadLine("/mnt/d/opt/codrax-main/internal/agent/agent.go", 150) {
		t.Errorf("HasReadLine with absolute path should also resolve through canonicaliser")
	}
}

// TestEvidenceClosure_EmptyRepoRootDegradesGracefully pins that a
// closure built with NewEvidenceClosure("") — tests and any
// pre-session-22 caller — retains the historical canonicalizeRepoPath
// behaviour: strip "./" and normalise separators, no absolute-prefix
// stripping. Ensures the signature change is backward-compatible.
func TestEvidenceClosure_EmptyRepoRootDegradesGracefully(t *testing.T) {
	c := NewEvidenceClosure("")
	c.SetReadSet(map[string]bool{"./internal/agent/agent.go": true})
	// The "./" form collapses to the bare relative form by the
	// historical canonicalizer.
	if !c.HasRead("internal/agent/agent.go") {
		t.Errorf("empty repoRoot should still strip './' on seed")
	}
	// Absolute-path lookup without repoRoot cannot resolve against
	// the relative key — the historical behaviour, confirmed.
	if c.HasRead("/mnt/d/opt/codrax-main/internal/agent/agent.go") {
		t.Errorf("empty repoRoot must NOT strip absolute prefix — that is session-22's enhancement, gated on a real root")
	}
}

// TestMutableState_SetRepoRootPropagatesToExistingClosure pins the
// late-propagation path: if a closure was lazy-init'd with an
// empty repoRoot before the orchestrator called SetRepoRoot (a
// race in BuildContext constructors), the subsequent SetRepoRoot
// call must propagate the real root into the already-existing
// closure so later canonicalisations pick it up.
func TestMutableState_SetRepoRootPropagatesToExistingClosure(t *testing.T) {
	m := NewMutableState("q")
	// Force lazy-init WITHOUT repoRoot.
	c1 := m.EvidenceClosure()
	c1.SetReadSet(map[string]bool{"internal/agent/agent.go": true})

	// Absolute-path lookup misses — the closure was built with "".
	if c1.HasRead("/mnt/d/opt/codrax-main/internal/agent/agent.go") {
		t.Fatalf("precondition: closure with empty repoRoot must not resolve absolute paths")
	}

	// Propagate repoRoot AFTER the closure exists.
	m.SetRepoRoot("/mnt/d/opt/codrax-main")

	// Now the same closure should accept the absolute form.
	c2 := m.EvidenceClosure()
	if c1 != c2 {
		t.Fatalf("SetRepoRoot must mutate the existing closure in place, not reconstruct it")
	}
	if !c2.HasRead("/mnt/d/opt/codrax-main/internal/agent/agent.go") {
		t.Errorf("SetRepoRoot did not propagate: absolute-path lookup still misses after the orchestrator supplied the real root")
	}
}

// TestAddRepair_ReadFile_MirrorsToPendingReads is the A1 invariant
// regression: AddRepair for RepairReadFile MUST simultaneously write
// the Files onto the PendingReads queue so runForcedReads (I4) can
// pick them up even after ConsumeRepairs drains the Repairs queue
// for prompt rendering. Without this mirror, the grounder's
// citation-drop feedback only reaches the prompt and an inattentive
// LLM can let the retry budget exhaust.
func TestAddRepair_ReadFile_MirrorsToPendingReads(t *testing.T) {
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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
	c := NewEvidenceClosure("")
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

// ─────────────────────────────────────────────────────────────────
// Session 11 F1 ViolationLedger tests
// ─────────────────────────────────────────────────────────────────

// TestAppendViolation_RejectsZeroKind defends the ledger against
// bug-induced pollution: a caller that forgets to set Kind would
// otherwise insert an uncategorized row that the F2 aggregator
// cannot group. The gate is intentionally on the type (not the
// content of Detail) so silent failure is impossible.
func TestAppendViolation_RejectsZeroKind(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{}) // zero-kind, should be dropped
	if got := c.Stats().ViolationsLogged; got != 0 {
		t.Errorf("ViolationsLogged=%d, want 0 (zero-Kind entries must be rejected)", got)
	}
	if got := c.Violations(); got != nil {
		t.Errorf("Violations()=%v, want nil", got)
	}
}

// TestAppendViolation_BumpsStatsAndRetrieves is the happy-path check:
// a well-formed Violation with SuspectedRoot appears in the ledger,
// bumps the counter, and surfaces in ViolationsByField / ByKind.
func TestAppendViolation_BumpsStatsAndRetrieves(t *testing.T) {
	c := NewEvidenceClosure("")
	v := Violation{
		Kind:   ViolGhostAnchor,
		Detail: "chain anchor foo.go not in ScannedSet",
		Stage:  "explore",
		SuspectedRoot: SuspectedRoot{
			IRField:    "ScannedSet",
			Reason:     "ranker missed declarative file",
			Confidence: 0.70,
		},
	}
	c.AppendViolation(v)

	if got := c.Stats().ViolationsLogged; got != 1 {
		t.Errorf("ViolationsLogged=%d, want 1", got)
	}
	if got := c.Violations(); len(got) != 1 || got[0].Kind != ViolGhostAnchor {
		t.Errorf("Violations()=%v, want 1 ghost_anchor entry", got)
	}
	if got := c.ViolationsByField("ScannedSet"); len(got) != 1 {
		t.Errorf("ViolationsByField(ScannedSet)=%v, want 1 entry", got)
	}
	if got := c.ViolationsByKind(ViolGhostAnchor); len(got) != 1 {
		t.Errorf("ViolationsByKind(ghost_anchor)=%v, want 1 entry", got)
	}
}

// TestViolationFieldTally_SkipsZeroRootEntries verifies that
// Violations with an empty SuspectedRoot.IRField are kept in the
// ledger (for backward compat / audit) but are NOT counted in the
// per-field histogram — the F2 aggregator relies on this to avoid
// noise from legacy three-field writers.
func TestViolationFieldTally_SkipsZeroRootEntries(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{Kind: ViolShape, Detail: "legacy write no root"})
	c.AppendViolation(Violation{
		Kind: ViolShape, Detail: "modern write with root",
		SuspectedRoot: SuspectedRoot{IRField: "answer_shape", Confidence: 0.8},
	})
	if got := c.Stats().ViolationsLogged; got != 2 {
		t.Errorf("ViolationsLogged=%d, want 2 (both entries recorded)", got)
	}
	tally := c.ViolationFieldTally()
	if tally["answer_shape"] != 1 {
		t.Errorf("ViolationFieldTally[answer_shape]=%d, want 1", tally["answer_shape"])
	}
	if _, zero := tally[""]; zero {
		t.Errorf("ViolationFieldTally must not have empty-key entry, got %v", tally)
	}
}

// TestTopSuspectedField_PicksHighestCount verifies the field picker
// returns the IRField with the most events, using max-confidence
// across that field's events as the reported confidence.
func TestTopSuspectedField_PicksHighestCount(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{Kind: ViolShape, SuspectedRoot: SuspectedRoot{IRField: "answer_shape", Confidence: 0.80}})
	c.AppendViolation(Violation{Kind: ViolShape, SuspectedRoot: SuspectedRoot{IRField: "answer_shape", Confidence: 0.85}})
	c.AppendViolation(Violation{Kind: ViolShape, SuspectedRoot: SuspectedRoot{IRField: "answer_shape", Confidence: 0.75}})
	c.AppendViolation(Violation{Kind: ViolCitation, SuspectedRoot: SuspectedRoot{IRField: "ScannedSet", Confidence: 0.90}})

	field, count, conf := c.TopSuspectedField()
	if field != "answer_shape" {
		t.Errorf("TopSuspectedField field=%q, want answer_shape", field)
	}
	if count != 3 {
		t.Errorf("TopSuspectedField count=%d, want 3", count)
	}
	if conf != 0.85 {
		t.Errorf("TopSuspectedField conf=%.2f, want 0.85 (max within field)", conf)
	}
}

// TestResetEvidenceClosure_ClearsViolations ensures per-task
// isolation: the orchestrator's ResetEvidenceClosure call must wipe
// the ledger so cross-task contamination is impossible (mirror of
// the existing reset invariants for readSet / repairs / stats).
func TestResetEvidenceClosure_ClearsViolations(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{
		Kind:          ViolGhostAnchor,
		SuspectedRoot: SuspectedRoot{IRField: "ScannedSet", Confidence: 0.7},
	})
	if c.Stats().ViolationsLogged != 1 {
		t.Fatalf("pre-Reset: want 1 violation logged, got %d", c.Stats().ViolationsLogged)
	}
	c.Reset()
	if got := c.Stats().ViolationsLogged; got != 0 {
		t.Errorf("post-Reset: ViolationsLogged=%d, want 0", got)
	}
	if got := c.Violations(); got != nil {
		t.Errorf("post-Reset: Violations()=%v, want nil", got)
	}
}

// TestContractResultRoundTrip_ViolationAliasing is the backward-compat
// guard: code that writes contract.Violation (via the type alias)
// and reads back via types.Violation must see the same data. This
// prevents a future refactor that accidentally breaks the alias.
func TestContractResultRoundTrip_ViolationAliasing(t *testing.T) {
	c := NewEvidenceClosure("")
	// Write as a bare Violation (the types package view).
	c.AppendViolation(Violation{
		Kind:   ViolCitation,
		Detail: "0/3 citations in ReadSet",
		Stage:  "finalize",
		SuspectedRoot: SuspectedRoot{
			IRField:    "ScannedSet",
			Reason:     "finalizer citations outside ReadSet",
			Confidence: 0.90,
		},
	})
	got := c.Violations()
	if len(got) != 1 {
		t.Fatalf("want 1 violation, got %d", len(got))
	}
	if got[0].Kind != ViolCitation {
		t.Errorf("Kind=%s, want citation", got[0].Kind)
	}
	if got[0].SuspectedRoot.IRField != "ScannedSet" {
		t.Errorf("SuspectedRoot.IRField=%s, want ScannedSet", got[0].SuspectedRoot.IRField)
	}
	if got[0].SuspectedRoot.Confidence != 0.90 {
		t.Errorf("Confidence=%.2f, want 0.90", got[0].SuspectedRoot.Confidence)
	}
}
