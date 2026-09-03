package types

import (
	"strings"
	"testing"
)

// verification_worktree_drift_fold_in_test.go — F-run-tests fold-in of V5-2
// (§40.36 复核收编): the typed lockfile fixed-point witness state is a closed
// set whose unproven members carry a single-sourced plain-words disclosure in
// both languages (F3); the untracked-lane predicate reads effect rows only
// (F6); the final report and the write handoff render both (F3/F6).

func TestVerificationLockfileFixedPointClosedSetAndDisclosure(t *testing.T) {
	seen := map[VerificationLockfileFixedPoint]bool{}
	for _, fp := range AllVerificationLockfileFixedPoints() {
		if fp == "" || seen[fp] {
			t.Fatalf("closed set carries an empty or duplicate member: %q", fp)
		}
		seen[fp] = true
		zh := VerificationLockfileFixedPointDisclosure(fp, true)
		en := VerificationLockfileFixedPointDisclosure(fp, false)
		if VerificationLockfileFixedPointUnproven(fp) {
			if zh == "" || en == "" || !strings.Contains(en, "UNPROVEN") || !strings.Contains(zh, "未证明") {
				t.Fatalf("unproven fixed point %q must carry a plain-words disclosure in both languages: zh=%q en=%q", fp, zh, en)
			}
			if strings.ContainsAny(zh, ",") || strings.ContainsAny(en, ",") {
				t.Fatalf("disclosure phrases must stay comma-free for comma-joined risk details: zh=%q en=%q", zh, en)
			}
		} else if zh != "" || en != "" {
			t.Fatalf("fixed point %q is not unproven and must render no disclosure: zh=%q en=%q", fp, zh, en)
		}
	}
	if VerificationLockfileFixedPointDisclosure("", false) != "" || VerificationLockfileFixedPointUnproven("") || VerificationLockfileFixedPointDeclared("") {
		t.Fatalf("the empty state belongs to rows of OTHER classes only: neither unproven, nor disclosed, nor a declared member (a lockfile row never carries it)")
	}
}

func TestVerificationWorktreeUntrackedRetainedPathsReadsRowsOnly(t *testing.T) {
	effects := []VerificationWorktreeEffect{
		{Path: "src/x.rs", Kind: VerificationWorktreeEffectTrackedChanged},
		{Path: "b.bin", Kind: VerificationWorktreeEffectUntrackedCreated},
		{Path: " ", Kind: VerificationWorktreeEffectUntrackedCreated},
		{Path: "a.bin", Kind: VerificationWorktreeEffectUntrackedCreated},
		{Path: "b.bin", Kind: VerificationWorktreeEffectUntrackedCreated},
	}
	if got := VerificationWorktreeUntrackedRetainedPaths(effects, 0); strings.Join(got, ",") != "b.bin,a.bin" {
		t.Fatalf("untracked paths = %v", got)
	}
	if got := VerificationWorktreeUntrackedRetainedPaths(effects, 1); strings.Join(got, ",") != "b.bin" {
		t.Fatalf("limit must bound the list: %v", got)
	}
	var nilAudit *VerificationWorktreeAudit
	if nilAudit.HasUntrackedRetainedOutput() || nilAudit.UntrackedRetainedPaths(8) != nil {
		t.Fatalf("nil audit must be safe")
	}
	// The predicate ignores the status: a refused run with an untracked row
	// has untracked output; a disclosed run without one has none.
	refused := &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDrift, Effects: effects}
	disclosedOnly := &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDriftDisclosed, Effects: effects[:1]}
	if !refused.HasUntrackedRetainedOutput() || disclosedOnly.HasUntrackedRetainedOutput() {
		t.Fatalf("predicate must read rows, not status")
	}
}

// F6 (final report): a refused run still raises the untracked warning.
func TestWriteFinalResidualRisksRefusedRunKeepsUntrackedWarning(t *testing.T) {
	report := WriteFinalReport{}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDrift
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{
		{Path: "src/other.rs", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftUnclassified, Disposition: VerificationWorktreeEffectRefused},
		{Path: "generated.bin", Kind: VerificationWorktreeEffectUntrackedCreated},
	}
	risks := writeFinalResidualRisks(report)
	if !writeFinalHasRisk(risks, "verification_worktree_tracked_drift", "error", "src/other.rs") {
		t.Fatalf("refused drift must stay an error risk: %+v", risks)
	}
	if !writeFinalHasRisk(risks, "verification_worktree_untracked_side_effects", "warning", "generated.bin") {
		t.Fatalf("a refused run must still name its retained untracked output: %+v", risks)
	}
}

// F3 (final report): a disclosed lockfile row with an unproven fixed point
// carries the plain-words disclosure in the warning detail.
func TestWriteFinalResidualRisksNameUnprovenLockfileFixedPoint(t *testing.T) {
	report := WriteFinalReport{}
	report.Verification.WorktreeAuditStatus = VerificationWorktreeAuditTrackedDriftDisclosed
	report.Verification.WorktreeEffects = []VerificationWorktreeEffect{{
		Path: "Cargo.lock", Kind: VerificationWorktreeEffectTrackedChanged, DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh,
		OwnerRunner: "rust", Disposition: VerificationWorktreeEffectDisclosed, LockfileFixedPoint: VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded,
	}}
	want := VerificationLockfileFixedPointDisclosure(VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, false)
	risks := writeFinalResidualRisks(report)
	if !writeFinalHasRisk(risks, VerificationTrackedSideEffectDisclosedReason, "warning", "Cargo.lock=dependency_lockfile_refresh") ||
		!writeFinalHasRisk(risks, VerificationTrackedSideEffectDisclosedReason, "warning", want) {
		t.Fatalf("the disclosed warning must name the unproven fixed point: %+v", risks)
	}
}

// F3 (write handoff): the context pack effect row carries the typed fixed
// point token and its plain-words disclosure.
func TestWriteContextPackCarriesLockfileFixedPoint(t *testing.T) {
	pack := WriteContextPackFromChangeReport(&ChangeReport{
		PlanID: "plan-drift", Passed: true,
		WorktreeAudit: &VerificationWorktreeAudit{
			Status: VerificationWorktreeAuditTrackedDriftDisclosed,
			Effects: []VerificationWorktreeEffect{{
				Path: "Cargo.lock", Kind: VerificationWorktreeEffectTrackedChanged, Ownership: "git_tracked",
				Action: "disclosed_not_committed_not_auto_reverted", DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh,
				OwnerRunner: "rust", Disposition: VerificationWorktreeEffectDisclosed, LockfileFixedPoint: VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded,
			}},
		},
	})
	want := VerificationLockfileFixedPointDisclosure(VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, false)
	rowToken, disclosure := false, false
	for _, item := range pack.Items {
		if item.Kind == "verification_worktree_effect" && strings.Contains(item.Text, "path=Cargo.lock") &&
			strings.Contains(item.Text, "lockfile_fixed_point=unproven_suite_infra_downgraded") {
			rowToken = true
		}
		if item.Kind == "verification_lockfile_fixed_point" && item.Priority == WriteContextP1 &&
			strings.Contains(item.Text, "path=Cargo.lock") && strings.Contains(item.Text, want) {
			disclosure = true
		}
	}
	if !rowToken || !disclosure {
		t.Fatalf("lockfile fixed point token/disclosure missing from the write handoff (token=%v disclosure=%v): %+v", rowToken, disclosure, pack.Items)
	}
	// A proven fixed point carries the token only — no disclosure item.
	pack = WriteContextPackFromChangeReport(&ChangeReport{PlanID: "plan-drift", Passed: true,
		WorktreeAudit: &VerificationWorktreeAudit{Status: VerificationWorktreeAuditTrackedDriftDisclosed,
			Effects: []VerificationWorktreeEffect{{Path: "Cargo.lock", Kind: VerificationWorktreeEffectTrackedChanged,
				DriftClass: VerificationWorktreeDriftDependencyLockfileRefresh, Disposition: VerificationWorktreeEffectDisclosed,
				LockfileFixedPoint: VerificationLockfileFixedPointProven}}}})
	for _, item := range pack.Items {
		if item.Kind == "verification_lockfile_fixed_point" {
			t.Fatalf("a proven fixed point must not raise the unproven disclosure: %+v", item)
		}
	}
}
