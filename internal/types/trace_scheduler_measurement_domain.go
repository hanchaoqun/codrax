package types

// TraceSchedulerMeasurementDomain identifies one already-constructed native
// scheduler partition. It is not evidence of complete capture input, causal
// attribution, or equivalence to another measurement algorithm. Capture and
// clock identity remain separately owned by the enclosing source receipt.
// A missing legacy carrier stays unknown; never derive it from totals/prose.
type TraceSchedulerMeasurementDomain struct {
	Version        int     `json:"version"`
	Status         string  `json:"status"`
	Method         string  `json:"method"`
	TargetTID      int     `json:"target_tid"`
	WindowStartTs  float64 `json:"window_start_ts"`
	WindowEndTs    float64 `json:"window_end_ts"`
	QueryLineStart int     `json:"query_line_start"`
	QueryLineEnd   int     `json:"query_line_end"`
	PartitionID    string  `json:"partition_id"`
}

func CloneTraceSchedulerMeasurementDomain(in *TraceSchedulerMeasurementDomain) *TraceSchedulerMeasurementDomain {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}
