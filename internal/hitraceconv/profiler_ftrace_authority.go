package hitraceconv

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

type profilerFtracePayloadDisposition uint8

const (
	profilerFtracePayloadNotStructured profilerFtracePayloadDisposition = iota
	profilerFtracePayloadStructured
	profilerFtracePayloadMalformed
)

// profilerTracePluginResult is the single top-level TracePluginResult
// authority shared by metadata summarization and row rendering. Repeated
// protobuf fields remain repeated; only each occurrence's wire is audited.
type profilerTracePluginResult struct {
	Disposition profilerFtracePayloadDisposition
	CPUStats    [][]byte
	CPUDetails  [][]byte
	Symbols     [][]byte
	Clocks      [][]byte
	Versions    [][]byte
	CommDicts   [][]byte
	Issues      []string
}

func decodeProfilerTracePluginResult(data []byte) profilerTracePluginResult {
	var out profilerTracePluginResult
	recognized := false
	versionOccurrences := 0
	malformed := false
	for len(data) > 0 {
		key, n, ok := consumeProtoVarint(data)
		if !ok {
			malformed = true
			break
		}
		data = data[n:]
		fieldNumber := key >> 3
		if fieldNumber < 1 || fieldNumber > (1<<29)-1 {
			malformed = true
			break
		}
		field := int(fieldNumber)
		wire := int(key & 0x7)
		known := field == 1 || field == 2 || field == 5 || field == 6 || field == 7 || field == 8
		if known {
			// Attribute a truncated or unsupported-wire field to the official
			// envelope as soon as its complete key identifies a known member.
			recognized = true
			if field == 7 {
				versionOccurrences++
			}
		}

		var raw []byte
		switch wire {
		case 0:
			_, consumed, valid := consumeProtoVarint(data)
			if !valid {
				malformed = true
			} else {
				data = data[consumed:]
			}
		case 1:
			if len(data) < 8 {
				malformed = true
			} else {
				raw = data[:8]
				data = data[8:]
			}
		case 2:
			length, consumed, valid := consumeProtoVarint(data)
			if !valid || length > uint64(len(data[consumed:])) {
				malformed = true
			} else {
				raw = data[consumed : consumed+int(length)]
				data = data[consumed+int(length):]
			}
		case 5:
			if len(data) < 4 {
				malformed = true
			} else {
				raw = data[:4]
				data = data[4:]
			}
		default:
			malformed = true
		}
		if malformed {
			break
		}
		if !known {
			continue
		}

		var destination *[][]byte
		switch field {
		case 1:
			destination = &out.CPUStats
		case 2:
			destination = &out.CPUDetails
		case 5:
			destination = &out.Symbols
		case 6:
			destination = &out.Clocks
		case 7:
			destination = &out.Versions
		case 8:
			destination = &out.CommDicts
		default:
			continue
		}
		if wire != 2 {
			out.Issues = append(out.Issues, fmt.Sprintf("envelope_trace_plugin_field%d_wrong_wire", field))
			continue
		}
		*destination = append(*destination, raw)
	}
	if malformed {
		if !recognized {
			return profilerTracePluginResult{Disposition: profilerFtracePayloadNotStructured}
		}
		out.Disposition = profilerFtracePayloadMalformed
		out.CPUStats = nil
		out.CPUDetails = nil
		out.Symbols = nil
		out.Clocks = nil
		out.Versions = nil
		out.CommDicts = nil
		out.Issues = append(out.Issues, "envelope_trace_plugin_malformed_wire")
		return out
	}
	if !recognized {
		out.Disposition = profilerFtracePayloadNotStructured
		return out
	}
	if versionOccurrences > 1 {
		out.Issues = append(out.Issues, "envelope_trace_plugin_version_duplicate")
		out.Versions = nil
	}
	out.Disposition = profilerFtracePayloadStructured
	return out
}

func profilerTracePluginResultEvents(result profilerTracePluginResult) ([]profilerFtraceEventRecord, error) {
	var out []profilerFtraceEventRecord
	if result.Disposition == profilerFtracePayloadMalformed {
		return out, nil
	}
	for _, raw := range result.CPUDetails {
		events, err := decodeProfilerFtraceCPUDetailEvents(raw)
		if err != nil {
			return nil, err
		}
		out = append(out, events...)
	}
	return out, nil
}

func profilerTracePluginResultCoverage(result profilerTracePluginResult) []TraceDBCoverage {
	if len(result.Issues) == 0 {
		return nil
	}
	return []TraceDBCoverage{{
		Family:   "builtin_modern_ftrace:trace_plugin_envelope",
		Table:    "__trace_plugin_envelope__",
		Role:     "unsupported_input",
		Found:    true,
		RowsRead: len(result.Issues),
		Skipped:  profilerTracePluginIssueSummary(result.Issues),
		FieldSources: map[string]string{
			"schema_profile": "TracePluginResult repeated fields 1/2/5/6/8 and singular version field 7",
		},
	}}
}

// addStrictSystraceRowsFromBytes is the compatibility lane for an exact
// ftrace-plugin payload that is demonstrably not TracePluginResult protobuf.
// It stages the complete payload first: one non-comment malformed fragment
// rejects text classification, so embedded protobuf strings cannot mint a
// partial text row.
func addStrictSystraceRowsFromBytes(data []byte, seq *int, sink *traceDBRowSink) (int, bool, error) {
	if len(data) == 0 {
		return 0, false, nil
	}
	var staged []renderedRow
	for start := 0; start < len(data); {
		end := start
		for end < len(data) && data[end] != '\n' {
			end++
		}
		part := data[start:end]
		if len(part) > 0 && part[len(part)-1] == '\r' {
			part = part[:len(part)-1]
		}
		part = bytes.Trim(part, " ")
		if end < len(data) {
			start = end + 1
		} else {
			start = len(data)
		}
		if len(part) == 0 {
			continue
		}
		if len(part) > maxProfilerTextLineBytes {
			return 0, false, nil
		}
		line := string(part)
		if !traceDBSinglePhysicalLine(line, false) {
			return 0, false, nil
		}
		if part[0] == '#' {
			continue
		}
		ts, ok := strictSystraceLineTimestampNS(line)
		if !ok {
			return 0, false, nil
		}
		staged = append(staged, renderedRow{tsNS: ts, line: line})
	}
	if len(staged) == 0 {
		return 0, false, nil
	}
	if sink == nil || seq == nil {
		return 0, false, fmt.Errorf("strict systrace row sink or sequence is nil")
	}
	for index := range staged {
		staged[index].seq = *seq
		if err := sink.add(staged[index]); err != nil {
			return index, true, err
		}
		(*seq)++
	}
	return len(staged), true, nil
}

func strictSystraceLineTimestampNS(line string) (uint64, bool) {
	if _, ok := tracequery.ParseLine(1, line, nil); !ok {
		return 0, false
	}
	return tracequery.ParseLineTimestampNS(line)
}

func profilerTracePluginIssueSummary(issues []string) string {
	counts := make(map[string]int)
	for _, issue := range issues {
		if issue = strings.TrimSpace(issue); issue != "" {
			counts[issue]++
		}
	}
	return traceDBCountSummary(counts)
}
