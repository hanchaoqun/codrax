package hitraceconv

// P1-a2.2-B1-a: fixed TracePluginResult envelope diagnostics.  Repeated raw
// protobuf members remain untouched for a2.3; only the issue/caveat/coverage
// state is made fixed-cardinality here.

import (
	"fmt"
	"strconv"
	"strings"
)

type profilerTracePluginIssueKind uint8

const (
	profilerTracePluginIssueField1WrongWire profilerTracePluginIssueKind = iota
	profilerTracePluginIssueField2WrongWire
	profilerTracePluginIssueField5WrongWire
	profilerTracePluginIssueField6WrongWire
	profilerTracePluginIssueField7WrongWire
	profilerTracePluginIssueField8WrongWire
	profilerTracePluginIssueMalformedWire
	profilerTracePluginIssueVersionDuplicate
	profilerTracePluginIssueKindCount
)

func profilerTracePluginWrongWireIssue(field int) (profilerTracePluginIssueKind, bool) {
	switch field {
	case 1:
		return profilerTracePluginIssueField1WrongWire, true
	case 2:
		return profilerTracePluginIssueField2WrongWire, true
	case 5:
		return profilerTracePluginIssueField5WrongWire, true
	case 6:
		return profilerTracePluginIssueField6WrongWire, true
	case 7:
		return profilerTracePluginIssueField7WrongWire, true
	case 8:
		return profilerTracePluginIssueField8WrongWire, true
	default:
		return 0, false
	}
}

func (kind profilerTracePluginIssueKind) label() string {
	switch kind {
	case profilerTracePluginIssueField1WrongWire:
		return "envelope_trace_plugin_field1_wrong_wire"
	case profilerTracePluginIssueField2WrongWire:
		return "envelope_trace_plugin_field2_wrong_wire"
	case profilerTracePluginIssueField5WrongWire:
		return "envelope_trace_plugin_field5_wrong_wire"
	case profilerTracePluginIssueField6WrongWire:
		return "envelope_trace_plugin_field6_wrong_wire"
	case profilerTracePluginIssueField7WrongWire:
		return "envelope_trace_plugin_field7_wrong_wire"
	case profilerTracePluginIssueField8WrongWire:
		return "envelope_trace_plugin_field8_wrong_wire"
	case profilerTracePluginIssueMalformedWire:
		return "envelope_trace_plugin_malformed_wire"
	case profilerTracePluginIssueVersionDuplicate:
		return "envelope_trace_plugin_version_duplicate"
	default:
		return "envelope_trace_plugin_issue_invalid"
	}
}

type profilerTracePluginIssueCensus struct {
	Occurrences            [profilerTracePluginIssueKindCount]uint64
	AffectedFrames         [profilerTracePluginIssueKindCount]uint64
	VersionDuplicateExcess uint64
}

func (census *profilerTracePluginIssueCensus) observe(kind profilerTracePluginIssueKind, delta uint64) bool {
	if census == nil || kind >= profilerTracePluginIssueKindCount || delta == 0 {
		return census != nil && kind < profilerTracePluginIssueKindCount
	}
	index := int(kind)
	if census.Occurrences[index] == 0 && !checkedProfilerUint64AddTo(&census.AffectedFrames[index], 1) {
		return false
	}
	return checkedProfilerUint64AddTo(&census.Occurrences[index], delta)
}

func (census *profilerTracePluginIssueCensus) observeVersionDuplicate(excess uint64) bool {
	if excess == 0 {
		return census != nil
	}
	return census.observe(profilerTracePluginIssueVersionDuplicate, 1) &&
		checkedProfilerUint64AddTo(&census.VersionDuplicateExcess, excess)
}

func (census *profilerTracePluginIssueCensus) merge(frame profilerTracePluginIssueCensus) bool {
	if census == nil {
		return false
	}
	for index := range census.Occurrences {
		if !checkedProfilerUint64AddTo(&census.Occurrences[index], frame.Occurrences[index]) ||
			!checkedProfilerUint64AddTo(&census.AffectedFrames[index], frame.AffectedFrames[index]) {
			return false
		}
	}
	return checkedProfilerUint64AddTo(&census.VersionDuplicateExcess, frame.VersionDuplicateExcess)
}

func (census profilerTracePluginIssueCensus) empty() bool {
	for _, count := range census.Occurrences {
		if count > 0 {
			return false
		}
	}
	return true
}

func (census profilerTracePluginIssueCensus) has(kind profilerTracePluginIssueKind) bool {
	return kind < profilerTracePluginIssueKindCount && census.Occurrences[int(kind)] > 0
}

func (census profilerTracePluginIssueCensus) totalOccurrences() (uint64, bool) {
	var total uint64
	for _, count := range census.Occurrences {
		if !checkedProfilerUint64AddTo(&total, count) {
			return 0, false
		}
	}
	return total, true
}

func (census profilerTracePluginIssueCensus) summary() string {
	parts := make([]string, 0, profilerTracePluginIssueKindCount)
	for kind := profilerTracePluginIssueKind(0); kind < profilerTracePluginIssueKindCount; kind++ {
		if count := census.Occurrences[int(kind)]; count > 0 {
			parts = append(parts, fmt.Sprintf("%s=%d", kind.label(), count))
		}
	}
	return strings.Join(parts, ",")
}

func (census profilerTracePluginIssueCensus) appendFieldSources(fields map[string]string) {
	for kind := profilerTracePluginIssueKind(0); kind < profilerTracePluginIssueKindCount; kind++ {
		index := int(kind)
		if census.Occurrences[index] == 0 {
			continue
		}
		prefix := "issue_" + kind.label()
		fields[prefix+"_occurrences"] = strconv.FormatUint(census.Occurrences[index], 10)
		fields[prefix+"_affected_frames"] = strconv.FormatUint(census.AffectedFrames[index], 10)
	}
	if census.VersionDuplicateExcess > 0 {
		fields["issue_envelope_trace_plugin_version_duplicate_excess_occurrences"] = strconv.FormatUint(census.VersionDuplicateExcess, 10)
	}
}

type profilerFtraceEnvelopeDiagnosticLedger struct {
	Issues         profilerTracePluginIssueCensus
	DegradedFrames uint64
	FirstOffset    int64
	LastOffset     int64
}

func (ledger *profilerFtraceEnvelopeDiagnosticLedger) observe(frame profilerTracePluginIssueCensus, offset int64) bool {
	if ledger == nil {
		return false
	}
	if frame.empty() {
		return true
	}
	if ledger.DegradedFrames == 0 {
		ledger.FirstOffset = offset
	}
	if !checkedProfilerUint64AddTo(&ledger.DegradedFrames, 1) || !ledger.Issues.merge(frame) {
		return false
	}
	ledger.LastOffset = offset
	return true
}

func (ledger *profilerFtraceEnvelopeDiagnosticLedger) materialize(out *profilerContainerExtraction) bool {
	if ledger == nil || out == nil {
		return false
	}
	if ledger.DegradedFrames == 0 {
		return true
	}
	total, ok := ledger.Issues.totalOccurrences()
	if !ok {
		return false
	}
	rowsRead, ok := profilerContainerCountToInt(total)
	if !ok {
		return false
	}
	fields := map[string]string{
		"schema_profile":     "TracePluginResult repeated fields 1/2/5/6/8 and singular version field 7",
		"aggregation_policy": "fixed_envelope_issue_census",
		"degraded_frames":    strconv.FormatUint(ledger.DegradedFrames, 10),
		"first_offset":       strconv.FormatInt(ledger.FirstOffset, 10),
		"last_offset":        strconv.FormatInt(ledger.LastOffset, 10),
	}
	ledger.Issues.appendFieldSources(fields)
	out.TraceCoverage = append(out.TraceCoverage, TraceDBCoverage{
		Family: "builtin_modern_ftrace:trace_plugin_envelope", Table: "__trace_plugin_envelope__",
		Role: "unsupported_input", Found: true, RowsRead: rowsRead,
		Skipped: ledger.Issues.summary(), FieldSources: fields,
	})
	out.Caveats = append(out.Caveats, fmt.Sprintf(
		"ftrace-plugin TracePluginResult degraded: frames=%d first_offset=%d last_offset=%d; reasons=%s",
		ledger.DegradedFrames, ledger.FirstOffset, ledger.LastOffset, ledger.Issues.summary()))
	return true
}
