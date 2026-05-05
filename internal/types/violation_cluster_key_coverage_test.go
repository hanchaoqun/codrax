package types

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestAllViolationProducersSetClusterKey is the typed-cluster
// structural gate for the retry architecture. The runtime closure /
// repair planner still has legacy fallbacks that can fingerprint a
// violation from Detail prose, but production producers are expected
// to surface an explicit ClusterKey so retry routing does not depend
// on wording drift.
//
// The test walks every non-test Go file under internal/ and inspects
// Violation composite literals. Any literal that sets a Violation
// Kind without also setting ClusterKey in the surrounding literal
// window fails the test.
//
// This is intentionally source-level: it catches a new producer the
// moment someone lands code that appends a violation without typed
// cluster identity, rather than waiting for a flaky retry loop to
// expose the omission.
func TestAllViolationProducersSetClusterKey(t *testing.T) {
	repoRoot := findRepoRoot(t)
	sources := collectGoSourcesExcludingTests(t, filepath.Join(repoRoot, "internal"))
	kindPattern := regexp.MustCompile(`Kind:\s*(?:types\.)?Viol[A-Za-z0-9_]+`)

	var offenders []string
	for _, path := range sources {
		body := readFileForTest(t, path)
		lines := strings.Split(body, "\n")
		for i, line := range lines {
			if !kindPattern.MatchString(line) {
				continue
			}
			start := i - 20
			if start < 0 {
				start = 0
			}
			end := i + 20
			if end >= len(lines) {
				end = len(lines) - 1
			}
			window := strings.Join(lines[start:end+1], "\n")
			if !strings.Contains(window, "Violation{") && !strings.Contains(window, "types.Violation{") {
				continue
			}
			if strings.Contains(window, "ClusterKey:") {
				continue
			}
			offenders = append(offenders, relToRepo(path, repoRoot)+":"+lineSummary(i+1, strings.TrimSpace(line)))
		}
	}
	if len(offenders) > 0 {
		t.Fatalf("every production Violation producer must set ClusterKey; missing on:\n%s",
			strings.Join(offenders, "\n"))
	}
}

func lineSummary(line int, text string) string {
	return strconv.Itoa(line) + ": " + text
}
