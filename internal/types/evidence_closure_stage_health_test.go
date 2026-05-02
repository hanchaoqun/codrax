package types

import (
	"testing"
)

// TestStageHealthSnapshot_AppendViolationBumpsPerStage pins the
// Block 1 invariant that closure.AppendViolation increments the
// per-stage Violations counter and special-cases ViolSelfContradiction
// into the Contradictions sub-counter.
func TestStageHealthSnapshot_AppendViolationBumpsPerStage(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize"})
	c.AppendViolation(Violation{Kind: ViolSelfContradiction, Stage: "finalize"})
	c.AppendViolation(Violation{Kind: ViolPlanCritic, Stage: "plan"})
	snap := c.StageHealthSnapshot()
	if got := snap["finalize"].Violations; got != 2 {
		t.Errorf("finalize.Violations = %d, want 2", got)
	}
	if got := snap["finalize"].Contradictions; got != 1 {
		t.Errorf("finalize.Contradictions = %d, want 1", got)
	}
	if got := snap["plan"].Violations; got != 1 {
		t.Errorf("plan.Violations = %d, want 1", got)
	}
}

// TestStageHealthSnapshot_IncrementStageRetry pins the per-stage
// retry counter API used by AppendAnswerRetryEvent and the orchestrator
// finalize/verify retry sites.
func TestStageHealthSnapshot_IncrementStageRetry(t *testing.T) {
	c := NewEvidenceClosure("")
	c.IncrementStageRetry("analyze")
	c.IncrementStageRetry("analyze")
	c.IncrementStageRetry("finalize")
	c.IncrementStageRetry("") // empty stage MUST be silently dropped
	snap := c.StageHealthSnapshot()
	if got := snap["analyze"].Retries; got != 2 {
		t.Errorf("analyze.Retries = %d, want 2", got)
	}
	if got := snap["finalize"].Retries; got != 1 {
		t.Errorf("finalize.Retries = %d, want 1", got)
	}
	if _, ok := snap[""]; ok {
		t.Errorf("empty-stage bucket leaked into snapshot")
	}
}

// TestStageHealthSnapshot_PendingReadDerivedStage pins the lock-free
// helper that maps PendingRead.Origin → stage. Origins from
// explorer-side enforcers must bucket into "explore"; grounder /
// pre_complete origins bucket into "finalize".
func TestStageHealthSnapshot_PendingReadDerivedStage(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AddPendingRead(PendingRead{File: "a.go", Origin: "chain_promotion"})
	c.AddPendingRead(PendingRead{File: "b.go", Origin: "grounder_reject"})
	c.AddPendingRead(PendingRead{File: "c.go", Origin: "phase1_unread"})
	c.AddPendingRead(PendingRead{File: "d.go", Origin: "pre_complete.citation_floor_low"})
	snap := c.StageHealthSnapshot()
	if got := snap["explore"].PendingReads; got != 2 {
		t.Errorf("explore.PendingReads = %d, want 2 (chain_promotion + phase1_unread)", got)
	}
	if got := snap["finalize"].PendingReads; got != 2 {
		t.Errorf("finalize.PendingReads = %d, want 2 (grounder_reject + pre_complete.citation_floor_low)", got)
	}
}

// TestViolationStageTally_BucketsByStage pins the histogram API used
// by Block 3's selective fallback when the dominant violation kind
// doesn't pin a stage.
func TestViolationStageTally_BucketsByStage(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize"})
	c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize"})
	c.AppendViolation(Violation{Kind: ViolPlanCritic, Stage: "plan"})
	c.AppendViolation(Violation{Kind: ViolReflectorObservation, Stage: "verify"})
	tally := c.ViolationStageTally()
	if tally["finalize"] != 2 {
		t.Errorf("tally[finalize] = %d, want 2", tally["finalize"])
	}
	if tally["plan"] != 1 {
		t.Errorf("tally[plan] = %d, want 1", tally["plan"])
	}
	if tally["verify"] != 1 {
		t.Errorf("tally[verify] = %d, want 1", tally["verify"])
	}
}

// TestTopSuspectedStage_PicksHighestCount pins the API used by the
// end-of-Run summary line and Block 3's fallback policy.
func TestTopSuspectedStage_PicksHighestCount(t *testing.T) {
	c := NewEvidenceClosure("")
	c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize",
		SuspectedRoot: SuspectedRoot{IRField: "answer_shape", Confidence: 0.7}})
	c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize",
		SuspectedRoot: SuspectedRoot{IRField: "answer_shape", Confidence: 0.9}})
	c.AppendViolation(Violation{Kind: ViolPlanCritic, Stage: "plan",
		SuspectedRoot: SuspectedRoot{IRField: "plan_critic_risk", Confidence: 0.5}})
	stage, count, conf := c.TopSuspectedStage()
	if stage != "finalize" || count != 2 {
		t.Errorf("TopSuspectedStage = (%s, %d), want (finalize, 2)", stage, count)
	}
	if conf < 0.85 || conf > 0.95 {
		t.Errorf("TopSuspectedStage maxConf = %.2f, want ~0.9", conf)
	}
}

// TestEvidenceClosureReset_AllFieldsCleared pins the Reset hygiene
// contract — every map/slice field that holds Run-scoped data must
// be cleared so the next task does not see stale signal. This test
// is the structural gate the comment on Reset references.
func TestEvidenceClosureReset_AllFieldsCleared(t *testing.T) {
	c := NewEvidenceClosure("/repo")
	c.SetReadSet(map[string]bool{"foo.go": true})
	c.AddReadRanges(map[string][]LineRange{"foo.go": {{Start: 1, End: 10}}})
	c.RecordFileTotalLines("foo.go", 100)
	c.SetScannedSet(map[string]bool{"bar.go": true})
	c.RecordCitation("foo.go", 5)
	c.AddPendingRead(PendingRead{File: "baz.go", Origin: "test"})
	c.AppendUnverifiedFinding(UnverifiedFinding{Token: "Foo"})
	c.SetSubjectMatch("chain", 0.5)
	c.AppendFingerprint(ClosureFingerprint{ReadSetHash: 1})
	c.AppendViolation(Violation{Kind: ViolShape, Stage: "finalize"})
	c.IncrementStageRetry("finalize")
	c.MarkPhase1UnreadFired()

	c.Reset()

	if len(c.ReadSet()) != 0 {
		t.Error("ReadSet not cleared")
	}
	// readRanges/fileTotalLines are unexported but their effects
	// surface via FileTotalLines / ReadRanges queries.
	if c.FileTotalLines("foo.go") != 0 {
		t.Errorf("fileTotalLines not cleared (the Block-1 latent bug)")
	}
	if r := c.ReadRanges("foo.go"); len(r) != 0 {
		t.Error("readRanges not cleared")
	}
	// IsScanned returns true on empty ScannedSet (historical "no
	// pre-scan, anything goes" semantics) so we directly inspect
	// the cleared map size instead of using IsScanned as a proxy.
	if got := c.ScannedSet(); len(got) != 0 {
		t.Errorf("scannedSet not cleared: %v", got)
	}
	if refs := c.CitedRefs(); len(refs) != 0 {
		t.Error("citedRefs not cleared")
	}
	if pr := c.PendingReads(); len(pr) != 0 {
		t.Error("pendingReads not cleared")
	}
	if uf := c.UnverifiedFindings(); len(uf) != 0 {
		t.Error("unverifiedFinds not cleared")
	}
	if vs := c.Violations(); len(vs) != 0 {
		t.Error("violations not cleared")
	}
	if c.Stats().HasActivity() {
		t.Error("stats not cleared")
	}
	if snap := c.StageHealthSnapshot(); len(snap) != 0 {
		t.Errorf("PerStage snapshot not cleared: %+v", snap)
	}
	if c.Phase1UnreadFired() {
		t.Error("phase1UnreadFired latch not cleared")
	}
}

// TestStageStats_IsEmpty pins the helper used by the renderer to
// skip empty stage rows.
func TestStageStats_IsEmpty(t *testing.T) {
	if !(StageStats{}.IsEmpty()) {
		t.Error("zero StageStats must be empty")
	}
	if (StageStats{Retries: 1}).IsEmpty() {
		t.Error("StageStats with retries must NOT be empty")
	}
	if (StageStats{Contradictions: 1}).IsEmpty() {
		t.Error("StageStats with contradictions must NOT be empty")
	}
}

// TestClosureStats_HasActivity_TouchesPerStage pins the contract that
// HasActivity returns true when ONLY PerStage data is set (i.e. the
// scalar counters are all zero but a stage retry was recorded).
func TestClosureStats_HasActivity_TouchesPerStage(t *testing.T) {
	c := NewEvidenceClosure("")
	c.IncrementStageRetry("finalize")
	if !c.Stats().HasActivity() {
		t.Error("HasActivity must return true when only PerStage is non-zero")
	}
}
