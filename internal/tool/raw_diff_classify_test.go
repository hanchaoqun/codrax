package tool

import (
	"strings"
	"testing"
)

// PIB-W2 W-4 (ledger docs/design/pi_borrow_analysis_20260729.md §7.2):
// typed classification of raw unified-diff failures — the last untyped
// rejection lane on the apply surface.

func TestClassifyGitApplyFailure_Table(t *testing.T) {
	substitutionPatch := "--- a/f.go\n+++ b/f.go\n@@ -1,3 +1,4 @@\n context\n retrun value\n+return value\n tail\n"
	plainPatch := "--- a/f.go\n+++ b/f.go\n@@ -1,2 +1,2 @@\n-old\n+new\n"
	cases := []struct {
		name, gitErr, patch, wantCode string
		wantPath                      string
		wantLine                      int
	}{
		{"context mismatch with locator",
			"error: patch failed: internal/a.go:42\nerror: internal/a.go: patch does not apply",
			plainPatch, rawDiffReasonContextMismatch, "internal/a.go", 42},
		{"substitution wins over plain mismatch",
			"error: patch failed: internal/a.go:7\nerror: internal/a.go: patch does not apply",
			substitutionPatch, rawDiffReasonSubstitution, "internal/a.go", 7},
		{"corrupt patch",
			"fatal: corrupt patch at line 12",
			plainPatch, rawDiffReasonCorruptPatch, "", 0},
		{"target missing",
			"error: ghost.go: No such file or directory",
			plainPatch, rawDiffReasonTargetMissing, "", 0},
		{"already exists",
			"error: dup.go: already exists in working directory",
			plainPatch, rawDiffReasonAlreadyApplied, "", 0},
		{"unclassified floor never guesses",
			"error: something completely novel",
			plainPatch, rawDiffReasonUnclassified, "", 0},
	}
	for _, tc := range cases {
		diag := classifyGitApplyFailure(tc.gitErr, tc.patch)
		if diag.ReasonCode != tc.wantCode {
			t.Errorf("%s: code = %q, want %q", tc.name, diag.ReasonCode, tc.wantCode)
		}
		if diag.Path != tc.wantPath || diag.ConflictLine != tc.wantLine {
			t.Errorf("%s: locator = %s:%d, want %s:%d", tc.name, diag.Path, diag.ConflictLine, tc.wantPath, tc.wantLine)
		}
		if strings.TrimSpace(diag.RetryInstruction) == "" {
			t.Errorf("%s: every code must carry a retry instruction", tc.name)
		}
	}

	// Message rendering carries the locator when present.
	diag := classifyGitApplyFailure("error: patch failed: x.go:9\nerror: x.go: patch does not apply", plainPatch)
	if msg := rawDiffRepairMessage(diag); !strings.Contains(msg, "x.go:9") {
		t.Errorf("repair message must carry the conflict locator: %q", msg)
	}
}
