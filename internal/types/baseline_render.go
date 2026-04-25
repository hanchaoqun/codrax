package types

import (
	"fmt"
	"strings"
)

// CollectBaselineFailures returns the subset of baseline TestResults
// that failed, preserving the order the runner emitted them. Empty
// or nil baseline returns nil; callers use len(...) to gate section
// rendering. Build-error rows (Kind=TestResultKindBuildError) are
// excluded — a baseline with build errors means the unmodified
// worktree wouldn't even compile, which is a different class of
// problem than "tests already broken".
//
// Shared by the coder and verifier agents so both prompts surface
// the same baseline picture without drift.
func CollectBaselineFailures(baseline *ChangeReport) []TestResult {
	if baseline == nil || len(baseline.TestResults) == 0 {
		return nil
	}
	out := make([]TestResult, 0, len(baseline.TestResults))
	for _, r := range baseline.TestResults {
		if r.Kind == TestResultKindBuildError {
			continue
		}
		if !r.Passed {
			out = append(out, r)
		}
	}
	return out
}

// RenderBaselineFailureList writes a markdown-list-shaped block
// describing the first cap baseline failures, with an overflow
// marker when there are more. Each line is "- <id> (<suite>)" or
// "- <id>" when Suite is empty. Returns an empty string when
// failures is empty so callers can guard the surrounding section
// header without re-counting.
//
// cap is bounded at >=1; pass 15 for the default rendering shape.
// Shared by coder + verifier prompt builders.
func RenderBaselineFailureList(failures []TestResult, cap int) string {
	if len(failures) == 0 {
		return ""
	}
	if cap < 1 {
		cap = 1
	}
	var b strings.Builder
	shown := 0
	for _, r := range failures {
		if shown >= cap {
			fmt.Fprintf(&b, "- … (+%d more)\n", len(failures)-shown)
			break
		}
		if r.Suite != "" {
			fmt.Fprintf(&b, "- %s (%s)\n", r.AssertionID, r.Suite)
		} else {
			fmt.Fprintf(&b, "- %s\n", r.AssertionID)
		}
		shown++
	}
	return b.String()
}
