package orchestrator

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_verify_worktree_render_witness_test.go — V5-2 (§40.11): the verify
// note names disclosed side effects (path + class) in both languages and
// says the verdict stands while the change is not delivered; refused drift
// keeps rendering nothing here (the failure surface owns it).
func TestRenderVerificationWorktreeAuditNoteDisclosedLane(t *testing.T) {
	audit := &types.VerificationWorktreeAudit{
		Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, ReasonCode: types.VerificationTrackedSideEffectDisclosedReason,
		Effects: []types.VerificationWorktreeEffect{
			{Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh, OwnerRunner: "rust", Disposition: types.VerificationWorktreeEffectDisclosed},
			{Path: "main.go", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftFormatterNoSemanticDiff, OwnerRunner: "go", Disposition: types.VerificationWorktreeEffectDisclosed},
		},
	}
	zh := renderVerificationWorktreeAuditNote(audit, true)
	for _, want := range []string{"`Cargo.lock`（dependency_lockfile_refresh）", "`main.go`（formatter_no_semantic_diff）", "验证结论保持有效", "未纳入交付提交", "未被自动回退"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("zh note missing %q:\n%s", want, zh)
		}
	}
	en := renderVerificationWorktreeAuditNote(audit, false)
	for _, want := range []string{"`Cargo.lock` (dependency_lockfile_refresh)", "`main.go` (formatter_no_semantic_diff)", "The verdict stands", "not part of the delivery commit", "not auto-reverted"} {
		if !strings.Contains(en, want) {
			t.Fatalf("en note missing %q:\n%s", want, en)
		}
	}
	// Refused drift renders nothing in the note (existing behaviour, pinned).
	refused := &types.VerificationWorktreeAudit{Status: types.VerificationWorktreeAuditTrackedDrift,
		Effects: []types.VerificationWorktreeEffect{{Path: "src/x.rs", Kind: types.VerificationWorktreeEffectTrackedChanged, Disposition: types.VerificationWorktreeEffectRefused}}}
	if got := renderVerificationWorktreeAuditNote(refused, true); got != "" {
		t.Fatalf("refused drift is owned by the failure surface, note must stay empty: %q", got)
	}
}

// A disclosed run also names its retained untracked outputs in both languages.
func TestRenderVerificationWorktreeAuditNoteDisclosedRunKeepsUntrackedParagraph(t *testing.T) {
	audit := &types.VerificationWorktreeAudit{
		Status: types.VerificationWorktreeAuditTrackedDriftDisclosed, UntrackedEffectCount: 1,
		Effects: []types.VerificationWorktreeEffect{
			{Path: "Cargo.lock", Kind: types.VerificationWorktreeEffectTrackedChanged, DriftClass: types.VerificationWorktreeDriftDependencyLockfileRefresh, Disposition: types.VerificationWorktreeEffectDisclosed},
			{Path: "generated.bin", Kind: types.VerificationWorktreeEffectUntrackedCreated},
		},
	}
	for _, zh := range []bool{true, false} {
		note := renderVerificationWorktreeAuditNote(audit, zh)
		if !strings.Contains(note, "`Cargo.lock`") || !strings.Contains(note, "`generated.bin`") {
			t.Fatalf("zh=%v note must name both lanes:\n%s", zh, note)
		}
	}
}
