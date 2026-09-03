package orchestrator

import (
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/types"
)

// write_verify_failure_render.go holds the verify-stage FAILURE wording
// ("测试未通过" / "Tests did not pass") — moved out of orchestrator.go in
// the F-run-tests round-three fold-in (§40.36) so the failure outcome can
// render the shared worktree-audit note beside the success and unverified
// outcomes (write_verify_render.go / write_verify_worktree_render.go)
// without growing the god-file. The orchestrator.go line ratchet tightened
// by the moved block.

// renderVerifyFailure builds the Mutable.Result message for a
// verify-stage failure. Three blocks, each at most a few lines:
//
//  1. Header — plain language ("测试未通过" / "Tests did not pass"),
//     no internal "Verify" jargon.
//  2. Reason — exactly ONE source: report.FailureSummary if non-
//     empty, else the agent-side message with the "verify failed: "
//     prefix stripped, else a count-only fallback. Capped at
//     verifyFailureSummaryMaxChars so a multi-megabyte stderr dump
//     cannot drown the rest of the prompt.
//  3. Failing test list — only when failing test names add
//     information beyond the summary. Skipped entirely when every
//     failing test name already appears verbatim in the summary
//     (otherwise the user reads the same names twice). Capped at
//     verifyFailureMaxNamesShown.
//  4. Next step — one short sentence pointing at the retry path.
//
// Pre-2026-04-30 this rendered as: "Verify FAILED" header + the full
// summary + the literal "agentError" (which started with the same
// summary again as a "verify failed: ..." prefix, so users saw the
// reason printed twice) + a 10-name list (which usually duplicated
// names already in the summary) + a tip. Three sources of the same
// reason in one block.
func renderVerifyFailure(report *types.ChangeReport, agentError, lang string) string {
	zh := isLangZh(lang)
	var b strings.Builder

	// Header — uses the same "did not pass" wording as the stage
	// row's failed phrase so the inline message and the dock label
	// agree.
	if zh {
		b.WriteString("## 测试未通过\n\n")
	} else {
		b.WriteString("## Tests did not pass\n\n")
	}

	// Reason — single source, capped.
	reason := strings.TrimSpace(verifyFailureReason(report, agentError))
	if reason != "" {
		if len([]rune(reason)) > verifyFailureSummaryMaxChars {
			rs := []rune(reason)
			reason = string(rs[:verifyFailureSummaryMaxChars]) + "…"
		}
		b.WriteString(reason)
		b.WriteString("\n\n")
	}

	// Failing test list — skipped when redundant with the reason.
	if report != nil {
		failedNames := failingAssertionNames(report.TestResults)
		if len(failedNames) > 0 && !reasonNamesEveryFailure(reason, failedNames) {
			shown := failedNames
			if len(shown) > verifyFailureMaxNamesShown {
				shown = shown[:verifyFailureMaxNamesShown]
			}
			if zh {
				fmt.Fprintf(&b, "失败测试: %s", strings.Join(shown, ", "))
				if len(failedNames) > len(shown) {
					fmt.Fprintf(&b, " (还有 %d 个)", len(failedNames)-len(shown))
				}
				b.WriteString("\n\n")
			} else {
				fmt.Fprintf(&b, "Failing tests: %s", strings.Join(shown, ", "))
				if len(failedNames) > len(shown) {
					fmt.Fprintf(&b, " (+%d more)", len(failedNames)-len(shown))
				}
				b.WriteString("\n\n")
			}
		}
	}

	// Next step — one short line.
	if zh {
		b.WriteString("下一步:`/mode write` 后再发请求,失败上下文会自动带进去。")
	} else {
		b.WriteString("Next: `/mode write` and re-send the request; failure context is carried in automatically.")
	}
	// Worktree-audit note (F-run-tests round three, finding F): a refused
	// run is a failure outcome, so the retained untracked outputs and the
	// disclosed lockfile / formatter rows are named here too — the same
	// shared predicate every verify outcome renders. The reason above stays
	// refused-rows-only (that is the planner's failure summary).
	b.WriteString(renderVerifyReportWorktreeAuditNote(report, zh))
	return b.String()
}

// verifyFailureSummaryMaxChars caps the runner stderr / failure
// summary the user sees inline. Anything past this is truncated with
// "…" — the full content stays in .codrax/plans/<id>.report.json.
// 800 runes is roughly 12-15 visual lines at typical widths, enough
// for a panic + 5 stack frames or 3 assertion explanations.
const verifyFailureSummaryMaxChars = 800

// verifyFailureMaxNamesShown caps the "Failing tests: a, b, c" list.
// 5 is enough to disambiguate without overwhelming when the runner
// emitted dozens of test names.
const verifyFailureMaxNamesShown = 5

// verifyFailureReason picks ONE source for the inline failure
// reason. Priority:
//
//  1. report.FailureSummary — already curated by the runner parser.
//  2. agentError minus the "verify failed: " prefix (which is the
//     verifier's structural wrapper around (1); when (1) is empty
//     this fallback at least carries the count line).
//  3. Count fallback — N test(s) failed.
//  4. Empty.
//
// Splitting these three sources into a helper makes the dedup
// rule above ("don't append a redundant test list") readable and
// individually unit-testable.
func verifyFailureReason(report *types.ChangeReport, agentError string) string {
	if report != nil && strings.TrimSpace(report.FailureSummary) != "" {
		return report.FailureSummary
	}
	clean := strings.TrimSpace(agentError)
	clean = strings.TrimPrefix(clean, "verify failed: ")
	if clean != "" {
		return clean
	}
	if report != nil {
		failed := 0
		for _, r := range report.TestResults {
			if !r.Passed {
				failed++
			}
		}
		if failed > 0 {
			return fmt.Sprintf("%d test(s) failed", failed)
		}
	}
	return ""
}

// failingAssertionNames returns the AssertionIDs of failing tests
// in the order they appear in the report. Empty slice when nothing
// failed. Helper so the dedup check below operates on a clean list
// without re-walking TestResults at every comparison.
func failingAssertionNames(results []types.TestResult) []string {
	var names []string
	for _, r := range results {
		if r.Passed {
			continue
		}
		if r.AssertionID != "" {
			names = append(names, r.AssertionID)
		}
	}
	return names
}

// reasonNamesEveryFailure reports whether every failing assertion
// name already appears verbatim in the reason text. When true, the
// caller should skip the explicit "Failing tests: ..." list because
// it would repeat names the user just read in the summary.
//
// Conservative: requires every name to be present (any one missing
// → the list adds information and should be shown). Substring
// match because some runner summaries embed names in larger
// context like "FAIL: TestX (0.02s)".
func reasonNamesEveryFailure(reason string, names []string) bool {
	if reason == "" || len(names) == 0 {
		return false
	}
	for _, n := range names {
		if !strings.Contains(reason, n) {
			return false
		}
	}
	return true
}
