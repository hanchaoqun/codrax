package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_verify_worktree_render_fold_in_test.go — F-run-tests fold-in of
// V5-2 (§40.36 复核收编): the verify note renders the untracked lane
// independently of the tracked lane's disposition (F6) and names an
// unproven lockfile fixed point in plain words in both languages (F3).

// F6: a refused run (tracked_drift) still names its retained untracked
// outputs; the refused tracked rows stay with the failure surface.
func TestRenderVerificationWorktreeAuditNoteRefusedRunKeepsUntrackedParagraph(t *testing.T) {
	audit := &types.VerificationWorktreeAudit{
		Status: types.VerificationWorktreeAuditTrackedDrift, TrackedEffectCount: 1, UntrackedEffectCount: 1, RefusedTrackedEffectCount: 1,
		Effects: []types.VerificationWorktreeEffect{
			{Path: "src/x.rs", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftUnclassified, Disposition: types.VerificationWorktreeEffectRefused},
			{Path: "generated.bin", Kind: types.VerificationWorktreeEffectUntrackedCreated},
		},
	}
	for _, zh := range []bool{true, false} {
		note := renderVerificationWorktreeAuditNote(audit, zh)
		if !strings.Contains(note, "`generated.bin`") {
			t.Fatalf("zh=%v refused run must still disclose its untracked output:\n%s", zh, note)
		}
		if strings.Contains(note, "src/x.rs") {
			t.Fatalf("zh=%v refused tracked rows belong to the failure surface:\n%s", zh, note)
		}
	}
}

// F3: a disclosed lockfile row whose fixed point is unproven says so in
// plain words (single-sourced phrase) in zh and en; a proven row does not.
func TestRenderVerificationWorktreeAuditNoteNamesUnprovenLockfileFixedPoint(t *testing.T) {
	audit := &types.VerificationWorktreeAudit{
		Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, ReasonCode: types.VerificationTrackedSideEffectDisclosedReason,
		Effects: []types.VerificationWorktreeEffect{{
			Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh,
			OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed, LockfileFixedPoint: types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded,
		}},
	}
	for _, zh := range []bool{true, false} {
		want := types.VerificationLockfileFixedPointDisclosure(types.VerificationLockfileFixedPointUnprovenSuiteInfraDowngraded, zh)
		note := renderVerificationWorktreeAuditNote(audit, zh)
		if want == "" || !strings.Contains(note, "`Cargo.lock`") || !strings.Contains(note, want) {
			t.Fatalf("zh=%v note must name the unproven fixed point on the row (want %q):\n%s", zh, want, note)
		}
	}
	audit.Effects[0].LockfileFixedPoint = types.VerificationLockfileFixedPointProven
	if note := renderVerificationWorktreeAuditNote(audit, false); strings.Contains(note, "UNPROVEN") {
		t.Fatalf("a proven fixed point must not be disclosed as unproven:\n%s", note)
	}
}
