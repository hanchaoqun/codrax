package tracequery

import "fmt"

// formatCPUFrequencyLimitSummary is shared by rank and evidence publication.
// It describes the existing inventory/representative split without changing
// accumulation, selection, numbers or causal eligibility.
func formatCPUFrequencyLimitSummary(limit CPUFrequencyLimit) string {
	role := "strictest positive maximum representative"
	if limit.MaxFrequency <= 0 {
		role = "no positive maximum ceiling in these records; zero-maximum inventory representative"
	}
	return fmt.Sprintf("cpu=%d: %d valid frequency-limit record(s) in the selected query scope; %s: min=%dkHz max=%dkHz line=%d ts=%.6f; total count is not the repetition count of this min/max pair",
		limit.CPU, limit.Count, role, limit.MinFrequency, limit.MaxFrequency, limit.Line, limit.Ts)
}
