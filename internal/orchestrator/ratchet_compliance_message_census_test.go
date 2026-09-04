package orchestrator

// ratchet_compliance_message_census_test.go — §40.27 V7-5 / §40.55.
//
// Three hot-file LOC ratchets exist in the repo (single-source table below).
// Their tripwire messages are the only teaching a developer sees at trip
// time, and before this pin all three said "split concern-specific code"
// without saying what is NOT compliance: 4c7a0d0a3 stayed under the
// orchestrator.go ceiling by compressing the §29.60 pendingCompletionReset
// declaration comment and deleting the throat comment (later restored in
// §40.55), and 727366b32 stayed under a 78-LOC source-inventory ceiling by
// rewriting a comment. The ratchet's intent is concern extraction; paying it
// with comments/blank lines/dead-line trimming leaves the god-file the same
// size in code and silently erodes the documentation the ratchet exists to
// protect.
//
// This census pins one shared sentence into every ratchet's failure message.
// It is deliberately a SOFT signal (message wording + docs, §11.8): a
// comment-line floor or comment-density ratchet would hard-gate on a noisy
// count (comments legitimately move with the code they describe), which the
// precise-signals-for-hard-gates rule forbids. The census itself is precise:
// verbatim substring on a closed file list, fail-loud on any unreadable file.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ratchetComplianceRule is the sentence every LOC-ratchet tripwire must carry
// verbatim. Edit it here and in each listed test in the same change.
const ratchetComplianceRule = "Comment/blank-line compression and dead-line trimming are NOT ratchet compliance — extract a concern file and lower this ceiling in the same change."

// locRatchetTestFiles is the single-source table of hot-file LOC ratchets
// (paths relative to this package). A new ratchet is registered here so it
// inherits the rule; an unreadable entry fails the census rather than being
// skipped.
var locRatchetTestFiles = []string{
	"ir_delivery_ratchet_test.go",
	filepath.Join("..", "dataquery", "loc_ratchet_test.go"),
	filepath.Join("..", "tool", "source_inventory_convergence_test.go"),
}

func TestLOCRatchetMessagesCarryComplianceRule(t *testing.T) {
	t.Parallel()
	if len(locRatchetTestFiles) < 3 {
		t.Fatalf("locRatchetTestFiles lost entries: %v", locRatchetTestFiles)
	}
	for _, path := range locRatchetTestFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read ratchet test %s: %v (a moved or renamed ratchet must update locRatchetTestFiles)", path, err)
		}
		src := string(data)
		if !strings.Contains(src, "t.Fatalf(") && !strings.Contains(src, "t.Errorf(") {
			t.Fatalf("%s does not look like a ratchet test (no t.Fatalf/t.Errorf tripwire)", path)
		}
		if !strings.Contains(src, ratchetComplianceRule) {
			t.Errorf("%s: LOC-ratchet failure message does not carry the compliance rule verbatim:\n  %s", path, ratchetComplianceRule)
		}
	}
}
