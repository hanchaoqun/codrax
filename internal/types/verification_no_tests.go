package types

import "strings"

// NoTestsWithoutAssertionVerdict is the report-level meaning of the legacy
// runner census. The census itself remains lossless: one empty invocation must
// not erase a native assertion from another invocation, even of the same runner.
// Conversely a synthetic build row, skipped/aggregate result or model-authored
// plain probe is not a native assertion. Only the producer-owned assertion scope
// can discharge this particular absence; path and contract authority remain
// independent gates. Legacy empty reports retain their unavailable meaning even
// when their old Passed flag is false.
func (r *ChangeReport) NoTestsWithoutAssertionVerdict() bool {
	if r == nil || len(r.NoTestsRunners) == 0 || r.BuildFailed {
		return false
	}
	if r.FailureKind != "" && r.FailureKind != FailureKindNoTests && r.FailureKind != FailureKindTestsFailed {
		return false // Explicit failures/unavailability keep their own category.
	}
	for _, result := range r.TestResults {
		if !result.Passed {
			return false // Including legacy failures with no observation scope.
		}
		if (result.Kind == "" || result.Kind == TestResultKindUnit) &&
			result.ObservationScope == TestObservationScopeAssertion &&
			strings.TrimSpace(result.AssertionID) != "" && strings.TrimSpace(result.Suite) != "" {
			return false
		}
	}
	for _, cmd := range r.ExecutedCommands {
		if ExecutedCommandFailed(cmd) && executedCommandUnavailableReasonCode(cmd) == "" {
			return false // A real command failure cannot become a no-tests warning.
		}
	}
	return true
}

// NoTestsInvocationLabels is display-only. Recover known empty invocations
// from executor-owned rows, never from command text or a runner-wide guess.
// Older census entries without a matching receipt disclose the missing scope.
func (r *ChangeReport) NoTestsInvocationLabels() []string {
	if r == nil || len(r.NoTestsRunners) == 0 {
		return nil
	}
	var labels []string
	seen, scoped := map[string]bool{}, map[string]bool{}
	add := func(label string) {
		if label != "" && !seen[label] {
			seen[label] = true
			labels = append(labels, label)
		}
	}
	for _, cmd := range r.ExecutedCommands {
		if cmd.Outcome != ExecutedCommandOutcomeZeroTests && cmd.Outcome != ExecutedCommandOutcomeSyntheticNoTests {
			continue
		}
		runner := strings.TrimSpace(cmd.Runner)
		if runner == "" {
			continue
		}
		label := runner
		if framework := strings.TrimSpace(cmd.Framework); framework != "" {
			label += "/" + framework
		}
		if dir := strings.TrimSpace(cmd.WorkingDir); dir != "" {
			label += "@" + dir
		} else {
			label += " (working directory unavailable)"
		}
		add(label)
		scoped[runner] = true
	}
	for _, raw := range r.NoTestsRunners {
		if runner := strings.TrimSpace(raw); runner != "" && !scoped[runner] {
			add(runner + " (invocation scope unavailable)")
		}
	}
	return labels
}
