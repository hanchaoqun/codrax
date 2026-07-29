package tool

import (
	"fmt"
	"strings"
)

// PIB-W2 W-4 (ledger docs/design/pi_borrow_analysis_20260729.md §7.2):
// raw unified-diff failures were the last untyped rejection lane on
// the apply surface — structured edits carry reason codes with retry
// instructions, but git-apply failures returned prose only. This
// classifier derives a precise reason code from the discriminators
// that already exist (the git stderr locator regex and the
// substitution-shape detector); no new pattern inference. The prose
// rejection stays byte-identical — the typed layer is a pure additive
// side channel.

const (
	rawDiffReasonContextMismatch = "raw_diff_context_mismatch"
	rawDiffReasonAlreadyApplied  = "raw_diff_already_applied"
	rawDiffReasonCorruptPatch    = "raw_diff_corrupt_patch"
	rawDiffReasonTargetMissing   = "raw_diff_target_missing"
	rawDiffReasonSubstitution    = "raw_diff_substitution_left_in_context"
	rawDiffReasonUnclassified    = "raw_diff_unclassified"
)

type rawDiffDiagnostic struct {
	ReasonCode       string
	Path             string
	ConflictLine     int
	RetryInstruction string
}

// classifyGitApplyFailure maps a git-apply/patch failure to a typed
// diagnostic. Precedence: structural shapes that name their own cause
// (corrupt / missing target / already-present) first; then the
// context-mismatch family, where the substitution-left-in-context
// detector wins because its retry instruction is strictly more
// actionable; unclassified is the honest floor, never a guess.
func classifyGitApplyFailure(gitErr, patchPayload string) rawDiffDiagnostic {
	lower := strings.ToLower(gitErr)
	diag := rawDiffDiagnostic{ReasonCode: rawDiffReasonUnclassified,
		RetryInstruction: "Re-read the target region and regenerate the patch against the file's current bytes."}
	diag.Path, diag.ConflictLine = parseGitConflictLocator(gitErr)
	switch {
	case strings.Contains(lower, "corrupt patch"):
		diag.ReasonCode = rawDiffReasonCorruptPatch
		diag.RetryInstruction = "Regenerate the whole unified diff; do not hand-edit hunk headers or counts."
	case strings.Contains(lower, "no such file or directory") ||
		strings.Contains(lower, "does not exist in working directory"):
		diag.ReasonCode = rawDiffReasonTargetMissing
		diag.RetryInstruction = "The target file does not exist at this path; use kind=create for new files or fix the path."
	case strings.Contains(lower, "already exists in working directory"):
		diag.ReasonCode = rawDiffReasonAlreadyApplied
		diag.RetryInstruction = "The content appears already present; re-read the file and emit only still-missing changes."
	case strings.Contains(lower, "patch failed") || strings.Contains(lower, "patch does not apply"):
		if detectSubstitutionInContextMistake(patchPayload) != "" {
			diag.ReasonCode = rawDiffReasonSubstitution
			diag.RetryInstruction = "A line you meant to replace stayed in the context lines; regenerate the hunk with a paired '-' removal and '+' addition."
		} else {
			diag.ReasonCode = rawDiffReasonContextMismatch
			diag.RetryInstruction = "Re-read the file around the conflict line and regenerate the hunk from the CURRENT bytes; do not resend the failed hunk."
		}
	}
	return diag
}

// rawDiffRepairMessage renders the pack message: the retry move plus
// the conflict locator when one was parsed.
func rawDiffRepairMessage(diag rawDiffDiagnostic) string {
	if diag.Path != "" && diag.ConflictLine > 0 {
		return fmt.Sprintf("%s (conflict at %s:%d)", diag.RetryInstruction, diag.Path, diag.ConflictLine)
	}
	return diag.RetryInstruction
}
