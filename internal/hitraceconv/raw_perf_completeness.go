package hitraceconv

import "fmt"

const (
	rawPerfCaptureProfile = "raw_perf_record_census_v1"
	rawPerfCaptureSource  = "linux_perf_data_record_stream"

	rawPerfAggregateNotReported = "not_reported"
	rawPerfAggregateExact       = "exact"
	rawPerfAggregateUnknown     = "unknown"

	rawPerfUnknownAggregateOverflow    = "aggregate_overflow"
	rawPerfUnknownMalformedAggregate   = "malformed_aggregate"
	rawPerfUnknownMalformedAndOverflow = "malformed_aggregate_and_overflow"
)

func newRawPerfCaptureCompleteness() RawPerfCaptureCompleteness {
	return RawPerfCaptureCompleteness{
		Profile: rawPerfCaptureProfile,
		Source:  rawPerfCaptureSource,
		LostEvents: RawPerfAggregateTotal{
			State: rawPerfAggregateNotReported,
		},
		LostSamples: RawPerfAggregateTotal{
			State: rawPerfAggregateNotReported,
		},
		AuxBytes: RawPerfAggregateTotal{
			State: rawPerfAggregateNotReported,
		},
	}
}

func observeRawPerfRecord(census *RawPerfRecordCensus, accepted bool) {
	// The fixed input is at most MaxInt64 bytes and every physical record is
	// at least eight bytes, so these uint64 record counts cannot overflow.
	census.Physical++
	if accepted {
		census.Accepted++
		return
	}
	census.Rejected++
}

func finishRawPerfCaptureCompleteness(data *rawPerfData) error {
	if data == nil {
		return fmt.Errorf("raw perf capture completeness data is nil")
	}
	capture := &data.CaptureCompleteness
	capture.LostEvents = rawPerfAggregateFromCounter(capture.LostRecords, data.LostEvents)
	capture.LostSamples = rawPerfAggregateFromCounter(capture.LostSampleRecords, data.LostSamples)
	capture.AuxBytes = rawPerfAggregateFromCounter(capture.AuxRecords, data.AuxBytes)

	if capture.SampleRecords.Accepted != uint64(len(data.Samples)) {
		return fmt.Errorf("sample accepted=%d parsed=%d", capture.SampleRecords.Accepted, len(data.Samples))
	}
	if capture.LostRecords.Accepted != data.LostRecords {
		return fmt.Errorf("lost accepted=%d parsed=%d", capture.LostRecords.Accepted, data.LostRecords)
	}
	if capture.LostSampleRecords.Accepted != data.LostSampleRecords {
		return fmt.Errorf("lost_samples accepted=%d parsed=%d", capture.LostSampleRecords.Accepted, data.LostSampleRecords)
	}
	if capture.AuxRecords.Accepted != data.AuxRecords {
		return fmt.Errorf("aux accepted=%d parsed=%d", capture.AuxRecords.Accepted, data.AuxRecords)
	}
	if reason := validateRawPerfCaptureCompleteness(*capture); reason != "" {
		return fmt.Errorf("%s", reason)
	}
	return nil
}

func rawPerfAggregateFromCounter(records RawPerfRecordCensus, counter rawPerfQualityCounter) RawPerfAggregateTotal {
	if records.Physical == 0 {
		return RawPerfAggregateTotal{State: rawPerfAggregateNotReported}
	}
	if records.Rejected > 0 || counter.Overflow {
		reason := rawPerfUnknownMalformedAggregate
		switch {
		case records.Rejected > 0 && counter.Overflow:
			reason = rawPerfUnknownMalformedAndOverflow
		case counter.Overflow:
			reason = rawPerfUnknownAggregateOverflow
		}
		return RawPerfAggregateTotal{State: rawPerfAggregateUnknown, Reason: reason}
	}
	return RawPerfAggregateTotal{State: rawPerfAggregateExact, Value: counter.Value}
}

// rawPerfCaptureHasPublicationIssue is deliberately narrower than "contains
// quality metadata". Accepted AUX inventory and exact zero loss counters do
// not prove capture loss and therefore cannot open the later zero-sample
// inventory lane.
func rawPerfCaptureHasPublicationIssue(capture RawPerfCaptureCompleteness) (bool, error) {
	if reason := validateRawPerfCaptureCompleteness(capture); reason != "" {
		return false, fmt.Errorf("invalid raw perf capture completeness: %s", reason)
	}
	if capture.SampleRecords.Rejected > 0 ||
		capture.LostRecords.Rejected > 0 ||
		capture.LostSampleRecords.Rejected > 0 ||
		capture.AuxRecords.Rejected > 0 {
		return true, nil
	}
	return rawPerfAggregateIsIssue(capture.LostEvents) ||
		rawPerfAggregateIsIssue(capture.LostSamples), nil
}

func rawPerfAggregateIsIssue(total RawPerfAggregateTotal) bool {
	return total.State == rawPerfAggregateUnknown ||
		(total.State == rawPerfAggregateExact && total.Value > 0)
}

func cloneRawPerfCaptureCompleteness(capture RawPerfCaptureCompleteness) *RawPerfCaptureCompleteness {
	cloned := capture
	return &cloned
}

func rawPerfCaptureCompletenessPointerEqual(left, right *RawPerfCaptureCompleteness) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func validateRawPerfCaptureCompleteness(capture RawPerfCaptureCompleteness) string {
	if capture.Profile != rawPerfCaptureProfile {
		return "profile must be raw_perf_record_census_v1"
	}
	if capture.Source != rawPerfCaptureSource {
		return "source must be linux_perf_data_record_stream"
	}
	records := []struct {
		name   string
		census RawPerfRecordCensus
	}{
		{name: "sample", census: capture.SampleRecords},
		{name: "lost", census: capture.LostRecords},
		{name: "lost_samples", census: capture.LostSampleRecords},
		{name: "aux", census: capture.AuxRecords},
	}
	for _, record := range records {
		if record.census.Accepted > record.census.Physical ||
			record.census.Rejected != record.census.Physical-record.census.Accepted {
			return fmt.Sprintf("%s record census does not close", record.name)
		}
	}
	totals := []struct {
		name    string
		records RawPerfRecordCensus
		total   RawPerfAggregateTotal
	}{
		{name: "lost_events", records: capture.LostRecords, total: capture.LostEvents},
		{name: "lost_samples", records: capture.LostSampleRecords, total: capture.LostSamples},
		{name: "aux_bytes", records: capture.AuxRecords, total: capture.AuxBytes},
	}
	for _, item := range totals {
		if reason := validateRawPerfAggregateTotal(item.records, item.total); reason != "" {
			return item.name + " " + reason
		}
	}
	return ""
}

func validateRawPerfAggregateTotal(records RawPerfRecordCensus, total RawPerfAggregateTotal) string {
	if records.Physical == 0 {
		if total.State != rawPerfAggregateNotReported || total.Value != 0 || total.Reason != "" {
			return "must be not_reported without physical records"
		}
		return ""
	}
	switch total.State {
	case rawPerfAggregateExact:
		if records.Rejected != 0 || total.Reason != "" {
			return "exact total cannot contain rejected records or a reason"
		}
	case rawPerfAggregateUnknown:
		if total.Value != 0 {
			return "unknown total must not expose a numeric prefix"
		}
		switch total.Reason {
		case rawPerfUnknownAggregateOverflow:
			if records.Rejected != 0 || records.Accepted < 2 {
				return "aggregate_overflow requires at least two accepted records and no rejected records"
			}
		case rawPerfUnknownMalformedAggregate:
			if records.Rejected == 0 {
				return "malformed_aggregate requires a rejected record"
			}
		case rawPerfUnknownMalformedAndOverflow:
			if records.Rejected == 0 || records.Accepted < 2 {
				return "malformed_aggregate_and_overflow requires rejected and overflowing accepted records"
			}
		default:
			return "unknown total has an invalid reason"
		}
	default:
		return "has an invalid state"
	}
	return ""
}
