package types

import (
	"math"
	"sort"
	"strconv"
	"strings"
)

// TraceSchedulerMeasurementSources retains the native partitions consulted by
// a derived scheduler row. This is a source inventory, NOT an identity for that
// row's numeric value, a completeness claim, or permission to add two rows.
// Capture, clock and publication identity still belong to the enclosing source.
// HasUnknown is sticky when any contributing input lacks its own receipt.
type TraceSchedulerMeasurementSources struct {
	Domains    []TraceSchedulerMeasurementDomain `json:"domains,omitempty"`
	HasUnknown bool                              `json:"has_unknown,omitempty"`
}

func TraceSchedulerMeasurementSourcesFromDomain(in *TraceSchedulerMeasurementDomain) *TraceSchedulerMeasurementSources {
	if in == nil {
		return nil
	}
	return &TraceSchedulerMeasurementSources{Domains: []TraceSchedulerMeasurementDomain{*in}}
}

func CloneTraceSchedulerMeasurementSources(in *TraceSchedulerMeasurementSources) *TraceSchedulerMeasurementSources {
	if in == nil {
		return nil
	}
	out := *in
	if in.Domains != nil {
		out.Domains = append([]TraceSchedulerMeasurementDomain{}, in.Domains...)
	}
	return &out
}

// MergeTraceSchedulerMeasurementSources unions references from ALL supplied
// contributors without a display cap. Passing nil is an unknown contributor;
// callers must not pass an initial nil accumulator for the first known member.
// Legacy all-nil inputs remain absent; merging that absence with a known source
// later marks the union incomplete rather than silently borrowing its receipt.
func MergeTraceSchedulerMeasurementSources(inputs ...*TraceSchedulerMeasurementSources) *TraceSchedulerMeasurementSources {
	var present, unknown bool
	byKey := make(map[string]TraceSchedulerMeasurementDomain)
	for _, input := range inputs {
		if input == nil {
			unknown = true
			continue
		}
		present = true
		unknown = unknown || input.HasUnknown || len(input.Domains) == 0
		for _, domain := range input.Domains {
			byKey[traceSchedulerMeasurementDomainKey(domain)] = domain
		}
	}
	if !present {
		return nil
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := &TraceSchedulerMeasurementSources{HasUnknown: unknown}
	for _, key := range keys {
		out.Domains = append(out.Domains, byKey[key])
	}
	return out
}

// Full-field, bit-exact source equality: rounded durations, envelope windows,
// names and evidence IDs must never substitute for the native receipt.
func traceSchedulerMeasurementDomainKey(d TraceSchedulerMeasurementDomain) string {
	var b strings.Builder
	part := func(value string) {
		b.WriteString(strconv.Itoa(len(value)))
		b.WriteByte(':')
		b.WriteString(value)
	}
	part(strconv.Itoa(d.Version))
	part(d.Status)
	part(d.Method)
	part(strconv.Itoa(d.TargetTID))
	part(strconv.FormatUint(math.Float64bits(d.WindowStartTs), 16))
	part(strconv.FormatUint(math.Float64bits(d.WindowEndTs), 16))
	part(strconv.Itoa(d.QueryLineStart))
	part(strconv.Itoa(d.QueryLineEnd))
	part(d.PartitionID)
	return b.String()
}
