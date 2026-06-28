package types

import (
	"testing"
)

func TestEvidenceClosure_ExploreForkMergeDoesNotDuplicateBaselineEvents(t *testing.T) {
	parent := NewEvidenceClosure("")
	parent.AddPendingRead(PendingRead{File: "a.go", Origin: "base", Stage: "explore"})
	parent.AddRepair(RepairDirective{Kind: RepairExpandSearch, Subject: "base", Stage: "explore"})
	parent.AppendUnverifiedFinding(UnverifiedFinding{Token: "missing.go", Kind: "path"})
	parent.AppendViolation(Violation{Kind: ViolGhostAnchor, Stage: "explore", Detail: "base"})
	parent.AppendFingerprint(ClosureFingerprint{ReadSetHash: 1})
	parent.IncrementSymbolEmitRejection()

	fork := parent.CloneForExploreDispatch()
	fork.AddPendingRead(PendingRead{
		File:       "a.go",
		Origin:     "base",
		Stage:      "explore",
		LineRanges: []LineRange{{Start: 3, End: 4}},
	})
	fork.AddPendingRead(PendingRead{File: "b.go", Origin: "fork", Stage: "explore"})
	fork.AddRepair(RepairDirective{Kind: RepairExpandSearch, Subject: "base", Stage: "explore"})
	fork.AddRepair(RepairDirective{Kind: RepairExpandSearch, Subject: "fork", Stage: "explore"})
	fork.AppendUnverifiedFinding(UnverifiedFinding{Token: "missing.go", Kind: "path"})
	fork.AppendUnverifiedFinding(UnverifiedFinding{Token: "other.go", Kind: "path"})
	fork.AppendViolation(Violation{Kind: ViolGhostAnchor, Stage: "explore", Detail: "base"})
	fork.AppendViolation(Violation{Kind: ViolGhostAnchor, Stage: "explore", Detail: "fork"})
	fork.AppendFingerprint(ClosureFingerprint{ReadSetHash: 2})
	fork.IncrementSymbolEmitRejection()

	parent.MergeFrom(fork)

	if got := len(parent.PendingReads()); got != 2 {
		t.Fatalf("pending reads = %d, want 2", got)
	}
	if got := len(parent.PendingRepairs()); got != 2 {
		t.Fatalf("pending repairs = %d, want 2", got)
	}
	if got := len(parent.UnverifiedFindings()); got != 2 {
		t.Fatalf("unverified findings = %d, want 2", got)
	}
	if got := len(parent.Violations()); got != 3 {
		t.Fatalf("violations = %d, want 3 (baseline + two fork events)", got)
	}
	if got := len(parent.Fingerprints()); got != 2 {
		t.Fatalf("fingerprints = %d, want 2", got)
	}
	if got := parent.SymbolEmitRejections(); got != 2 {
		t.Fatalf("symbol emit rejections = %d, want 2", got)
	}
	stats := parent.Stats()
	if got := stats.RepairsRaised; got != 2 {
		t.Fatalf("repairs raised = %d, want 2", got)
	}
	if got := stats.ViolationsLogged; got != 3 {
		t.Fatalf("violations logged = %d, want 3", got)
	}
	if got := stats.PerStage["explore"].PendingReads; got != 2 {
		t.Fatalf("explore pending-read stats = %d, want 2", got)
	}
}

func TestEvidenceClosure_SourceInventoryObservationCloneMergeReset(t *testing.T) {
	parent := NewEvidenceClosure("")
	parent.RecordSourceInventoryObservation(SourceInventoryObservation{
		Active:   true,
		Complete: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleProduction,
			Count:    2,
			Complete: true,
		}},
	})
	fork := parent.CloneForExploreDispatch()
	fork.RecordSourceInventoryObservation(SourceInventoryObservation{
		Active:   true,
		Complete: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleThirdParty,
			Count:    1,
			Complete: true,
		}},
	})

	parent.MergeFrom(fork)
	got := parent.SourceInventoryObservation()
	counts := map[SourcePathRole]int{}
	for _, class := range got.SourceClasses {
		counts[class.Role] = class.Count
	}
	if counts[SourcePathRoleProduction] != 2 || counts[SourcePathRoleThirdParty] != 1 {
		t.Fatalf("merged source-class universe = %+v, want production=2 thirdparty=1", got.SourceClasses)
	}

	cloned := parent.Clone()
	cloned.RecordSourceInventoryObservation(SourceInventoryObservation{
		Active:   true,
		Complete: true,
		SourceClasses: []SourceInventorySourceClassCount{{
			Role:     SourcePathRoleVendor,
			Count:    1,
			Complete: true,
		}},
	})
	for _, class := range parent.SourceInventoryObservation().SourceClasses {
		if class.Role == SourcePathRoleVendor {
			t.Fatal("clone mutation leaked into parent source-class universe")
		}
	}

	parent.Reset()
	if got := parent.SourceInventoryObservation(); got.IsActive() {
		t.Fatalf("reset must clear source inventory observation: %+v", got)
	}
}

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

func TestEvidenceClosure_AddRepairCarriesAcceptedEvidenceSnapshot(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendAcceptedEvidenceItems([]EvidenceItem{
		{
			ID:              "ev-owner",
			Kind:            EvidenceDirect,
			Scope:           ScopeLine,
			Source:          "internal/owner.go",
			LineStart:       12,
			Subject:         "Owner",
			OwnerSymbol:     "Owner",
			GroundingStatus: GroundingGrounded,
		},
	})
	c.AddRepair(RepairDirective{
		Kind:      RepairEmitEvidence,
		Files:     []string{"internal/owner.go"},
		Rationale: "materialize exact owner evidence",
	})

	repairs := c.PendingRepairs()
	if len(repairs) != 1 {
		t.Fatalf("PendingRepairs len=%d, want 1", len(repairs))
	}
	if len(repairs[0].AcceptedEvidence) != 1 {
		t.Fatalf("AcceptedEvidence len=%d, want 1: %+v", len(repairs[0].AcceptedEvidence), repairs[0].AcceptedEvidence)
	}
	ref := repairs[0].AcceptedEvidence[0]
	if ref.ID != "ev-owner" || ref.Source != "internal/owner.go" || ref.SourcePathRole != SourcePathRoleProduction {
		t.Fatalf("unexpected accepted evidence ref: %+v", ref)
	}
}

func TestMutableStateAppendEvidenceSeedsRepairAcceptedEvidence(t *testing.T) {
	m := &MutableState{}
	m.AppendEvidence([]EvidenceItem{
		{
			ID:              "ev-shared",
			Kind:            EvidenceMechanism,
			Scope:           ScopeLine,
			Source:          "internal/shared.go",
			LineStart:       42,
			Subject:         "shared",
			GroundingStatus: GroundingRecovered,
		},
	})
	c := m.EvidenceClosure()
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-shared" {
		t.Fatalf("closure accepted evidence refs = %+v, want ev-shared", refs)
	}
	c.AddRepair(RepairDirective{
		Kind:    RepairRebindSubject,
		Subject: string(SubjectFunctionName),
		Origin:  "test",
	})
	repairs := c.PendingRepairs()
	if len(repairs) != 1 {
		t.Fatalf("PendingRepairs len=%d, want 1", len(repairs))
	}
	if len(repairs[0].AcceptedEvidence) != 1 || repairs[0].AcceptedEvidence[0].ID != "ev-shared" {
		t.Fatalf("repair accepted evidence = %+v, want ev-shared", repairs[0].AcceptedEvidence)
	}
}

// TestAddRepair_NonReadFile_DoesNotMirror asserts the A1 bridge is
// kind-specific: RepairExpandSearch / RepairSwapView / etc. MUST
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
		Kind:    RepairSwapView,
		Subject: "from=config_value,to=value",
		Origin:  "emit_answer_document.shape_mismatch",
	})
	c.AddRepair(RepairDirective{
		Kind:      RepairEmitEvidence,
		Files:     []string{"internal/types/config.go"},
		Rationale: "already-read file needs grounded evidence materialization",
		Origin:    "emit_investigation_complete.context_materialize",
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

// TestActiveRepairs_PrefersPendingReadsOverHistoricalReadRepairs pins
// the shared live-repair view used by explorer and scheduler. When a
// historical RepairReadFile directive targets a file that has already
// been satisfied, but pendingReads now points at a different live
// blocker, ActiveRepairs must surface the current pending file rather
// than the stale historical directive.
func TestActiveRepairs_PrefersPendingReadsOverHistoricalReadRepairs(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddRepair(RepairDirective{
		Kind:      RepairReadFile,
		Files:     []string{"internal/agent/analyzer.go"},
		Rationale: "old read target",
		Origin:    "emit_investigation_complete.old",
	})
	c.ClearPendingReadFor("internal/agent/analyzer.go")
	c.AddPendingRead(PendingRead{
		File:      "internal/types/analysis_ir.go",
		Rationale: "new live blocker",
		Origin:    "emit_investigation_complete.current",
	})

	repairs := c.ActiveRepairs()
	if len(repairs) != 1 {
		t.Fatalf("ActiveRepairs len=%d, want 1: %+v", len(repairs), repairs)
	}
	if repairs[0].Kind != RepairReadFile {
		t.Fatalf("ActiveRepairs[0].Kind=%q, want %q", repairs[0].Kind, RepairReadFile)
	}
	if len(repairs[0].Files) != 1 || repairs[0].Files[0] != "internal/types/analysis_ir.go" {
		t.Fatalf("ActiveRepairs read files=%v, want [internal/types/analysis_ir.go]", repairs[0].Files)
	}
	if repairs[0].Rationale != "new live blocker" {
		t.Fatalf("ActiveRepairs rationale=%q, want current pending-read rationale", repairs[0].Rationale)
	}
}

func TestActiveRepairs_SkipsAdvisoryRepairs(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddRepair(RepairDirective{
		Kind:      RepairEmitEvidence,
		Files:     []string{"internal/agent/analyzer.go"},
		Rationale: "optional extra call-chain detail",
		Origin:    "pre_complete.call_chain_qualified_intermediate",
		Advisory:  true,
		Stage:     "explore",
	})
	if got := c.ActiveRepairs(); len(got) != 0 {
		t.Fatalf("advisory repairs must not be active/blocking, got %+v", got)
	}
	pending := c.PendingRepairs()
	if len(pending) != 1 || !pending[0].Advisory {
		t.Fatalf("advisory repair should remain in audit ledger, got %+v", pending)
	}
	if got := c.ConsumeRepairs(); len(got) != 0 {
		t.Fatalf("advisory repairs must not render as retry directives, got %+v", got)
	}
	if got := c.PendingRepairs(); len(got) != 0 {
		t.Fatalf("ConsumeRepairs should drain advisory ledger after audit handoff, got %+v", got)
	}
}

func TestMergeExploreFork_AcceptedCompletionClearsSiblingRepairs(t *testing.T) {
	parent := NewMutableState("trace call chain")

	blocked := parent.ForkForExploreDispatch()
	blocked.EvidenceClosure().AddRepair(RepairDirective{
		Kind:      RepairEmitEvidence,
		Files:     []string{"internal/agent/analyzer.go"},
		Rationale: "stale sibling span repair",
		Origin:    "pre_complete.call_chain_principal_span",
		Stage:     "explore",
	})
	blocked.EvidenceClosure().AddPendingRead(PendingRead{
		File:      "internal/agent/analyzer.go",
		Origin:    "pre_complete.call_chain_principal_span",
		Rationale: "stale sibling forced-read",
		Stage:     "explore",
	})
	parent.MergeExploreFork(blocked)
	if got := parent.EvidenceClosure().ActiveRepairs(); len(got) == 0 {
		t.Fatalf("blocking sibling repair should be active before accepted completion")
	}
	if got := parent.EvidenceClosure().PendingReads(); len(got) == 0 {
		t.Fatalf("blocking sibling pending read should be active before accepted completion")
	}

	complete := parent.ForkForExploreDispatch()
	complete.SetInvestigationComplete("complete branch has accepted closure")
	parent.MergeExploreFork(complete)

	if got := parent.EvidenceClosure().ActiveRepairs(); len(got) != 0 {
		t.Fatalf("accepted parallel completion must clear stale sibling repairs, got %+v", got)
	}
	if got := parent.EvidenceClosure().PendingRepairs(); len(got) != 0 {
		t.Fatalf("accepted parallel completion must clear repair ledger, got %+v", got)
	}
	if got := parent.EvidenceClosure().PendingReads(); len(got) != 0 {
		t.Fatalf("accepted parallel completion must clear stale sibling pending reads, got %+v", got)
	}
}

// TestConsumeRepairs_RetainsLivePendingReadRepairs ensures scheduler
// prompts keep seeing unresolved forced reads even after the queued
// repairs ledger is drained. Read-file surfacing is anchored to
// pendingReads, not to whether the historical RepairReadFile
// directive has already been consumed once.
func TestConsumeRepairs_RetainsLivePendingReadRepairs(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddRepair(RepairDirective{
		Kind:      RepairReadFile,
		Files:     []string{"internal/agent/analyzer.go"},
		Rationale: "current read blocker",
		Origin:    "emit_investigation_complete.current",
	})
	c.AddRepair(RepairDirective{
		Kind:      RepairEmitEvidence,
		Files:     []string{"internal/agent/analyzer.go"},
		Rationale: "materialize evidence after the read",
		Origin:    "emit_investigation_complete.current",
	})

	first := c.ConsumeRepairs()
	if len(first) != 2 {
		t.Fatalf("first ConsumeRepairs len=%d, want 2: %+v", len(first), first)
	}
	second := c.ConsumeRepairs()
	if len(second) != 1 {
		t.Fatalf("second ConsumeRepairs len=%d, want 1 live read repair: %+v", len(second), second)
	}
	if second[0].Kind != RepairReadFile {
		t.Fatalf("second ConsumeRepairs kind=%q, want %q", second[0].Kind, RepairReadFile)
	}
	if len(second[0].Files) != 1 || second[0].Files[0] != "internal/agent/analyzer.go" {
		t.Fatalf("second ConsumeRepairs files=%v, want [internal/agent/analyzer.go]", second[0].Files)
	}
}

func TestClearPendingReadsByDebtClass_DropsOnlySelectedDebt(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:      "phase1.go",
		Origin:    "phase1_unread",
		Rationale: "support breadth",
	})
	c.AddPendingRead(PendingRead{
		File:       "chain.go",
		Origin:     "chain_promotion.bridge_literal",
		Rationale:  "support chain line",
		LineRanges: []LineRange{{Start: 10, End: 12}},
	})
	c.AddPendingRead(PendingRead{
		File:      "anchor.go",
		Origin:    "pre_complete.primary_anchor",
		Rationale: "load-bearing anchor",
	})

	if cleared := c.ClearPendingReadsByDebtClass(RepairDebtAdvisory); cleared != 2 {
		t.Fatalf("cleared advisory pending reads=%d, want 2", cleared)
	}
	pending := c.PendingReads()
	if len(pending) != 1 {
		t.Fatalf("remaining pending reads=%d, want 1: %+v", len(pending), pending)
	}
	if pending[0].File != "anchor.go" || ClassifyPendingReadRepair(pending[0]) != RepairDebtPrincipalBlocking {
		t.Fatalf("remaining pending read should be the principal blocker, got %+v", pending[0])
	}
}

func TestClearPendingReadsMatching_DropsOnlyConcreteEntries(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{
		File:      "./support.go",
		Origin:    "phase1_unread",
		Rationale: "generic support",
	})
	c.AddPendingRead(PendingRead{
		File:      "support.go",
		Origin:    "required_file_hint_unread",
		Rationale: "principal required file",
	})
	c.AddPendingRead(PendingRead{
		File:       "range.go",
		Origin:     "auto_bridge.pre_complete.primary_anchor",
		Rationale:  "old surgical range",
		LineRanges: []LineRange{{Start: 10, End: 12}},
	})
	c.AddPendingRead(PendingRead{
		File:       "range.go",
		Origin:     "pre_complete.primary_anchor",
		Rationale:  "new surgical range",
		LineRanges: []LineRange{{Start: 20, End: 22}},
	})

	cleared := c.ClearPendingReadsMatching(
		PendingRead{File: "support.go", Origin: "phase1_unread"},
		PendingRead{File: "range.go", Origin: "pre_complete.primary_anchor", LineRanges: []LineRange{{Start: 10, End: 12}}},
	)
	if cleared != 2 {
		t.Fatalf("cleared=%d, want 2", cleared)
	}
	pending := c.PendingReads()
	if len(pending) != 2 {
		t.Fatalf("remaining pending reads=%d, want 2: %+v", len(pending), pending)
	}
	if pending[0].File != "support.go" || pending[0].Origin != "required_file_hint_unread" {
		t.Fatalf("required-file pending read should remain, got %+v", pending[0])
	}
	if pending[1].File != "range.go" || len(pending[1].LineRanges) != 1 || pending[1].LineRanges[0].Start != 20 {
		t.Fatalf("non-matching surgical range should remain, got %+v", pending[1])
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
// every RepairSwapView MUST bump ClosureStats.ViewSwapRaised.
func TestAddRepair_SwapShape_BumpsStats(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddRepair(RepairDirective{
		Kind:    RepairSwapView,
		Subject: "from=boolean,to=value",
		Origin:  "emit_answer_document.shape_mismatch",
	})
	s := c.Stats()
	if s.ViewSwapRaised != 1 {
		t.Errorf("ViewSwapRaised=%d, want 1", s.ViewSwapRaised)
	}
	if s.RepairsRaised != 1 {
		t.Errorf("RepairsRaised=%d, want 1", s.RepairsRaised)
	}
	// Distinct Subject = distinct directive, so another one bumps.
	c.AddRepair(RepairDirective{
		Kind:    RepairSwapView,
		Subject: "from=explanation,to=value",
		Origin:  "pre_complete.subject_shape_mismatch",
	})
	if s2 := c.Stats(); s2.ViewSwapRaised != 2 {
		t.Errorf("ViewSwapRaised=%d, want 2 (distinct Subject)", s2.ViewSwapRaised)
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
		"internal/agent/explorer.go": true,
		"internal/skill/defaults.go": true,
		"internal/agent/subagent.go": false, // value-false keys must NOT enter
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
	if s.ExpandSearchRaised != 0 || s.ViewSwapRaised != 0 {
		t.Errorf("per-kind counters must not fire for other kinds: ExpandSearch=%d ShapeSwap=%d", s.ExpandSearchRaised, s.ViewSwapRaised)
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
	c.AppendViolation(Violation{Kind: ViolFamilyMismatch, Detail: "legacy write no root"})
	c.AppendViolation(Violation{
		Kind: ViolFamilyMismatch, Detail: "modern write with root",
		SuspectedRoot: SuspectedRoot{IRField: "question_kind", Confidence: 0.8},
	})
	if got := c.Stats().ViolationsLogged; got != 2 {
		t.Errorf("ViolationsLogged=%d, want 2 (both entries recorded)", got)
	}
	tally := c.ViolationFieldTally()
	if tally["question_kind"] != 1 {
		t.Errorf("ViolationFieldTally[question_kind]=%d, want 1", tally["question_kind"])
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
	c.AppendViolation(Violation{Kind: ViolFamilyMismatch, SuspectedRoot: SuspectedRoot{IRField: "question_kind", Confidence: 0.80}})
	c.AppendViolation(Violation{Kind: ViolFamilyMismatch, SuspectedRoot: SuspectedRoot{IRField: "question_kind", Confidence: 0.85}})
	c.AppendViolation(Violation{Kind: ViolFamilyMismatch, SuspectedRoot: SuspectedRoot{IRField: "question_kind", Confidence: 0.75}})
	c.AppendViolation(Violation{Kind: ViolCitation, SuspectedRoot: SuspectedRoot{IRField: "ScannedSet", Confidence: 0.90}})

	field, count, conf := c.TopSuspectedField()
	if field != "question_kind" {
		t.Errorf("TopSuspectedField field=%q, want question_kind", field)
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

// TestDrainSatisfiedPendingReads pins the fix for the 2026-04-22
// s1a-115146 run-1 bug: an enforcer-enqueued PendingRead lingered
// across every iteration of a single dispatch after the LLM had
// already satisfied it via its own read_file call, because the only
// drain site (runForcedReads) runs at window boundaries, not within
// a ReAct loop. preCompleteContractCheck now drains satisfied entries
// after refreshing ReadSet from ground-context LineIndex.
func TestDrainSatisfiedPendingReads(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "a.go", Origin: "pre_complete.primary_anchor", Rationale: "anchor unread"})
	c.AddPendingRead(PendingRead{File: "b.go", Origin: "phase1_unread", Rationale: "top-K unread"})
	c.AddPendingRead(PendingRead{File: "c.go", Origin: "auto_bridge.emit_answer_document.grounder", Rationale: "chain promotion"})
	c.SetReadSet(map[string]bool{"a.go": true, "c.go": true})

	drained := c.DrainSatisfiedPendingReads()
	if drained != 2 {
		t.Errorf("drained=%d, want 2 (a.go and c.go satisfied by ReadSet)", drained)
	}
	pr := c.PendingReads()
	if len(pr) != 1 {
		t.Fatalf("remaining PendingReads=%d, want 1 (only b.go should survive)", len(pr))
	}
	if pr[0].File != "b.go" {
		t.Errorf("surviving PendingRead.File=%q, want b.go", pr[0].File)
	}
	// Idempotent — second drain call with no further changes drains 0.
	if again := c.DrainSatisfiedPendingReads(); again != 0 {
		t.Errorf("second drain without ReadSet changes: got=%d, want 0", again)
	}
}

// TestDrainSatisfiedPendingReads_Canonicalize pins that the drain
// resolves PendingRead.File through the same canonicaliser used by
// ReadSet lookups, so absolute / "./"-prefixed / repo-relative forms
// all match. Mirrors the session-22 symmetry that
// TestEvidenceClosure_AbsolutePathCanonicalizesAgainstRepoRoot pins
// for HasRead.
func TestDrainSatisfiedPendingReads_Canonicalize(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	c.AddPendingRead(PendingRead{File: "./a.go", Origin: "enforcer"})
	c.AddPendingRead(PendingRead{File: "/repo/b.go", Origin: "enforcer"})
	c.SetReadSet(map[string]bool{"a.go": true, "b.go": true})

	if drained := c.DrainSatisfiedPendingReads(); drained != 2 {
		t.Errorf("drained=%d, want 2 (both entries canonicalise into ReadSet keys)", drained)
	}
	if pr := c.PendingReads(); len(pr) != 0 {
		t.Errorf("PendingReads after drain=%d, want 0", len(pr))
	}
}

// TestDrainSatisfiedPendingReads_NilAndEmpty guards the nil-closure
// and empty-queue paths — both must degrade quietly.
func TestDrainSatisfiedPendingReads_NilAndEmpty(t *testing.T) {
	var nilC *EvidenceClosure
	if got := nilC.DrainSatisfiedPendingReads(); got != 0 {
		t.Errorf("nil closure: got=%d, want 0", got)
	}
	c := NewEvidenceClosure("")
	if got := c.DrainSatisfiedPendingReads(); got != 0 {
		t.Errorf("empty PendingReads: got=%d, want 0", got)
	}
}
