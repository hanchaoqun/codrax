package tracequery

import "fmt"

const PerfBranchRecordCensusTeaching = "When present, quality.branch_records counts declared branch slots only in samples with a recorded count; samples_without_count are unknown, not zero. This does not decode branch targets, hardware validity, cycles, calls, or causality."

// Compact keeps a bounded, weight-independent census together when emitting
// text summaries. The JSON census is the authority for numeric consumers.
func (c *PerfBranchRecordCensus) Compact() string {
	if c == nil {
		return ""
	}
	return fmt.Sprintf("branch_records: samples_with_count=%d samples_with_entries=%d samples_without_count=%d recorded_entries=%d",
		c.SamplesWithCount, c.SamplesWithEntries, c.SamplesWithoutCount, c.RecordedEntries)
}
