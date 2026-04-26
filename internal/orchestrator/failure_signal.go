package orchestrator

import (
	"strings"
)

// ExtractFailureSignal pulls the most informative lines out of a
// runner's FailureDetail string. The original heuristic took only
// the first line, which on the major runners is fixture / setup
// boilerplate that carries zero information about WHY the test
// failed:
//
//   - pytest first line: `self = <test_module.TestClass testMethod=test_x>`
//     (the actual assertion lives 5-15 lines later, prefixed with `E `
//     for the error and `>` for the call site).
//   - go test first line: `=== RUN   TestX` or `--- FAIL: TestX (0.00s)`
//     (the actual error is in the next `\t<file>:<line>: ...` lines).
//   - JUnit XML failure-message first line: usually the exception class
//     name (e.g. `java.lang.AssertionError`); the message follows.
//
// Strategy: scan for runner-specific error markers in priority order
// and return only those lines. Fall back to the first non-fixture
// line if no marker matches. Cap to maxChars (truncating at a UTF-8-
// safe newline boundary when possible).
//
// The cap is the caller's responsibility to size. buildRetryHint
// uses 600 chars (3 failing tests × 600 = ~2 KB total, well within
// the planner prompt budget). buildReflectorInput uses the same
// signal extractor so the critic LLM sees identical evidence.
//
// Returns "" when input is empty so callers can skip empty-detail
// rows cleanly.
func ExtractFailureSignal(detail string, maxChars int) string {
	if strings.TrimSpace(detail) == "" {
		return ""
	}
	lines := strings.Split(detail, "\n")

	// Priority 1: pytest-style markers. Lines starting with `E `
	// (error message), `>` (the failing call site), or
	// `assert ` are the high-information ones. Walk the whole
	// failure detail and pull every match in source order — the
	// model needs to see them together, not the first one alone.
	var pytestSignal []string
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "E ") ||
			strings.HasPrefix(trimmed, ">") ||
			strings.HasPrefix(trimmed, "assert ") ||
			strings.HasPrefix(trimmed, "AssertionError") {
			pytestSignal = append(pytestSignal, strings.TrimRight(line, " \t"))
		}
	}
	if len(pytestSignal) > 0 {
		return capWithBoundary(strings.Join(pytestSignal, "\n"), maxChars)
	}

	// Priority 2: go test markers. Look for "--- FAIL:" header
	// AND any indented `<file>:<line>:` lines that follow. Those
	// indented lines carry the actual t.Errorf / t.Fatalf message.
	var goSignal []string
	inFailBlock := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "--- FAIL:") || strings.HasPrefix(trimmed, "FAIL\t") {
			goSignal = append(goSignal, line)
			inFailBlock = true
			continue
		}
		if inFailBlock && (strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "    ")) {
			goSignal = append(goSignal, line)
		} else if inFailBlock && trimmed == "" {
			// Allow blank lines within block but stop at first non-
			// indented non-blank line (next test header or summary).
			continue
		} else if inFailBlock && trimmed != "" {
			inFailBlock = false
		}
	}
	if len(goSignal) > 0 {
		return capWithBoundary(strings.Join(goSignal, "\n"), maxChars)
	}

	// Priority 3: panic / Exception markers (Go panic, Java/Kotlin
	// exception, Ruby/Python uncaught traceback header). These are
	// usually accompanied by a stack frame on the next line; capture
	// both.
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "panic:") ||
			strings.Contains(trimmed, "Exception:") ||
			strings.Contains(trimmed, "Traceback (most recent call last)") {
			// Take this line + the next 6 lines (typical depth of
			// the actionable part of a stack trace).
			end := i + 7
			if end > len(lines) {
				end = len(lines)
			}
			return capWithBoundary(strings.Join(lines[i:end], "\n"), maxChars)
		}
	}

	// Fallback: when no runner-specific marker matches, the input is
	// either short (already informative) or generic. Skip ONLY the
	// first line and only when it looks like fixture noise (the
	// pytest "self = <fixture>" header or the go test "=== RUN ..."
	// header). Otherwise keep everything and rely on capWithBoundary
	// to trim. Aggressive first-line stripping was the original bug
	// — it discarded the actual error on synthetic test inputs and
	// any runner whose output we hadn't recognised.
	first := strings.TrimSpace(lines[0])
	if (strings.HasPrefix(first, "self = ") || strings.HasPrefix(first, "=== RUN")) && len(lines) > 1 {
		return capWithBoundary(strings.Join(lines[1:], "\n"), maxChars)
	}
	return capWithBoundary(detail, maxChars)
}

// capWithBoundary truncates s to at most maxChars, preferring to
// break at a newline if one exists in the last 1/4 of the cap window.
// Appends an ellipsis when truncated. Returns the original when
// already short enough.
func capWithBoundary(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return strings.TrimRight(s, " \t\n")
	}
	cut := s[:maxChars]
	// Try to land on a newline in the last quarter of the window so
	// the truncation doesn't slice an error message in half.
	probe := maxChars * 3 / 4
	if idx := strings.LastIndex(cut[probe:], "\n"); idx >= 0 {
		cut = cut[:probe+idx]
	}
	return strings.TrimRight(cut, " \t\n") + " …"
}
