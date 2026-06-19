package types

import (
	"fmt"
	"regexp"
	"strings"
)

const defaultFailureSignalMaxChars = 900

var (
	failureLocationRE         = regexp.MustCompile(`(?m)([A-Za-z0-9_./\\-]+):([0-9]+)(?::[0-9]+)?`)
	pythonTracebackLocationRE = regexp.MustCompile(`(?m)File "([^"]+)", line ([0-9]+)`)
	expectedGotRE             = regexp.MustCompile(`(?i)\bexpected\s+(.+?)(?:,\s*(?:but\s+)?|\s+)got\s+(.+)`)
	wantGotRE                 = regexp.MustCompile(`(?i)\bwant(?:ed)?\s+(.+?)(?:,\s*|\s+)got\s+(.+)`)
	assertionEqualsRE         = regexp.MustCompile(`(?i)AssertionError:\s*(.+?)\s*(?:==|!=)\s*(.+)`)
	pytestAssertionLine       = regexp.MustCompile(`(?m)^\s*E\s+AssertionError:\s*(.+)$`)
)

// TestFailureSignal is a compact, typed projection of a failed unit-test row.
// It is soft guidance for replan and handoff consumers: hard workflow gates
// continue to read ChangeReport.Passed / VerificationStatus / FailureKind.
type TestFailureSignal struct {
	AssertionID string         `json:"assertion_id,omitempty"`
	Suite       string         `json:"suite,omitempty"`
	Kind        TestResultKind `json:"kind,omitempty"`
	Location    string         `json:"location,omitempty"`
	Signal      string         `json:"signal,omitempty"`
	Expected    string         `json:"expected,omitempty"`
	Actual      string         `json:"actual,omitempty"`
}

// ExtractTestFailureSignal projects a TestResult into a bounded replan signal.
// The input is runner output, not user/model prose, and the result is rendered
// only as guidance. The extractor is intentionally cross-runner and conservative:
// it captures common assertion lines, optional expected/actual pairs, and the
// first file:line location when present.
func ExtractTestFailureSignal(result TestResult, maxChars int) TestFailureSignal {
	if maxChars <= 0 {
		maxChars = defaultFailureSignalMaxChars
	}
	out := TestFailureSignal{
		AssertionID: strings.TrimSpace(result.AssertionID),
		Suite:       strings.TrimSpace(result.Suite),
		Kind:        result.Kind,
		Signal:      ExtractFailureSignal(result.FailureDetail, maxChars),
		Location:    extractFailureLocation(result.FailureDetail),
	}
	if out.Kind == "" {
		out.Kind = TestResultKindUnit
	}
	out.Expected, out.Actual = extractExpectedActual(result.FailureDetail)
	return out
}

func RenderTestFailureSignal(signal TestFailureSignal) string {
	var parts []string
	if signal.AssertionID != "" {
		parts = append(parts, "assertion="+signal.AssertionID)
	}
	if signal.Suite != "" {
		parts = append(parts, "suite="+signal.Suite)
	}
	if signal.Location != "" {
		parts = append(parts, "location="+signal.Location)
	}
	if signal.Expected != "" || signal.Actual != "" {
		parts = append(parts, fmt.Sprintf("expected=%q actual=%q", signal.Expected, signal.Actual))
	}
	if signal.Signal != "" {
		parts = append(parts, "signal="+signal.Signal)
	}
	return strings.TrimSpace(strings.Join(parts, " "))
}

func renderTestFailureSignal(signal TestFailureSignal) string {
	return RenderTestFailureSignal(signal)
}

func extractFailureLocation(detail string) string {
	if match := pythonTracebackLocationRE.FindStringSubmatch(detail); len(match) >= 3 {
		return filepathSlash(match[1]) + ":" + match[2]
	}
	match := failureLocationRE.FindStringSubmatch(detail)
	if len(match) < 3 {
		return ""
	}
	return filepathSlash(match[1]) + ":" + match[2]
}

func extractExpectedActual(detail string) (string, string) {
	for _, re := range []*regexp.Regexp{expectedGotRE, wantGotRE, assertionEqualsRE, pytestAssertionLine} {
		match := re.FindStringSubmatch(detail)
		if len(match) < 3 {
			continue
		}
		expected := cleanFailureValue(match[1])
		actual := cleanFailureValue(match[2])
		if expected != "" || actual != "" {
			return expected, actual
		}
	}
	return "", ""
}

func cleanFailureValue(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.Trim(raw, "`")
	raw = strings.Trim(raw, "\"")
	raw = strings.Trim(raw, "'")
	raw = strings.TrimSpace(raw)
	if len([]rune(raw)) > 160 {
		rs := []rune(raw)
		raw = string(rs[:160]) + "..."
	}
	return raw
}

func filepathSlash(raw string) string {
	return strings.ReplaceAll(strings.TrimSpace(raw), "\\", "/")
}

// ExtractFailureSignal pulls the most informative lines out of a runner's
// FailureDetail string. The original heuristic took only the first line, which
// on the major runners is fixture / setup boilerplate that carries little
// information about why the test failed.
func ExtractFailureSignal(detail string, maxChars int) string {
	if strings.TrimSpace(detail) == "" {
		return ""
	}
	lines := strings.Split(detail, "\n")

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
		return capFailureSignalWithBoundary(strings.Join(pytestSignal, "\n"), maxChars)
	}

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
			continue
		} else if inFailBlock && trimmed != "" {
			inFailBlock = false
		}
	}
	if len(goSignal) > 0 {
		return capFailureSignalWithBoundary(strings.Join(goSignal, "\n"), maxChars)
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "panic:") ||
			strings.Contains(trimmed, "Exception:") ||
			strings.Contains(trimmed, "Traceback (most recent call last)") {
			end := i + 7
			if end > len(lines) {
				end = len(lines)
			}
			return capFailureSignalWithBoundary(strings.Join(lines[i:end], "\n"), maxChars)
		}
	}

	first := strings.TrimSpace(lines[0])
	if (strings.HasPrefix(first, "self = ") || strings.HasPrefix(first, "=== RUN")) && len(lines) > 1 {
		return capFailureSignalWithBoundary(strings.Join(lines[1:], "\n"), maxChars)
	}
	return capFailureSignalWithBoundary(detail, maxChars)
}

func capFailureSignalWithBoundary(s string, maxChars int) string {
	if maxChars <= 0 || len(s) <= maxChars {
		return strings.TrimRight(s, " \t\n")
	}
	cut := s[:maxChars]
	probe := maxChars * 3 / 4
	if idx := strings.LastIndex(cut[probe:], "\n"); idx >= 0 {
		cut = cut[:probe+idx]
	}
	return strings.TrimRight(cut, " \t\n") + "…"
}
