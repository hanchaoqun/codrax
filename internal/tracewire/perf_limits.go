package tracewire

import (
	"fmt"
	"math"
)

// These are semantic identity budgets, not merely lexer limits. Writers must
// reject values above them so a row accepted by Codrax cannot later be
// truncated into the identity of a different perf sample by tracequery.
const (
	MaxPerfMetadataBytes      = 512
	MaxPerfCallchainBytes     = 2048
	MaxPerfParserCaveatsBytes = 4096
)

// PerfWireBuildError is the stable, machine-inspectable failure returned by
// Codrax-owned perf_sample builders. Field and Reason form the contract;
// Limit and Actual carry exact byte/value evidence where applicable.
type PerfWireBuildError struct {
	Field  string
	Reason string
	Limit  uint64
	Actual uint64
}

func (e *PerfWireBuildError) Error() string {
	if e == nil {
		return ""
	}
	message := fmt.Sprintf("perf sample wire: field=%s reason=%s", e.Field, e.Reason)
	if e.Limit != 0 || e.Actual != 0 {
		message += fmt.Sprintf(" limit=%d actual=%d", e.Limit, e.Actual)
	}
	return message
}

// CheckedPerfSampleWeight closes the uint64 producer domain over the signed
// wire/reader domain. Producers that historically treat zero as one must do
// that normalization before calling this function.
func CheckedPerfSampleWeight(value uint64) (int64, error) {
	if value == 0 {
		return 0, &PerfWireBuildError{Field: "sample_weight", Reason: "not_positive", Actual: value}
	}
	if value > math.MaxInt64 {
		return 0, &PerfWireBuildError{
			Field:  "sample_weight",
			Reason: "out_of_range",
			Limit:  math.MaxInt64,
			Actual: value,
		}
	}
	return int64(value), nil
}
