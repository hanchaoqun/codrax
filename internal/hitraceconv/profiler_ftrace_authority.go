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
	Disposition       profilerFtracePayloadDisposition
	PairFamilies      pairCriticalFormatFamilyMask
	PairCaptureOpaque bool
	CPUStats          [][]byte
	CPUDetails        [][]byte
	Symbols           [][]byte
	Clocks            [][]byte
	Versions          [][]byte
	CommDicts         [][]byte
	Issues            []string
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
			if !valid {
				malformed = true
			} else if length > uint64(len(data[consumed:])) {
				if field == 2 {
					out.PairFamilies |= profilerPairFamiliesFromCPUDetail(data[consumed:])
				}
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
		if field == 2 {
			out.PairFamilies |= profilerPairFamiliesFromCPUDetail(raw)
		}
		*destination = append(*destination, raw)
	}
	if malformed {
		if !recognized {
			return profilerTracePluginResult{Disposition: profilerFtracePayloadNotStructured}
		}
		out.Disposition = profilerFtracePayloadMalformed
		out.PairCaptureOpaque = true
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
		if result.PairFamilies != 0 || result.PairCaptureOpaque {
			out = append(out, profilerFtraceEventRecord{
				Field: profilerFtraceCPUDetailEnvelopeField, PairFamilies: result.PairFamilies,
				PairCaptureOpaque:    result.PairCaptureOpaque,
				EnvelopeDegradations: []string{"envelope_trace_plugin_malformed_wire"},
			})
		}
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
	poisonMMC := false
	observedMMC := false
	rejected := false
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
			_, governed, _ := profilerTextPairCensus(part)
			observedMMC = observedMMC || governed
			rejected = true
			continue
		}
		line := string(part)
		kind, governed, admitted := profilerTextPairAdmission(line)
		observedMMC = observedMMC || governed
		if !traceDBSinglePhysicalLine(line, false) {
			rejected = true
			continue
		}
		if part[0] == '#' {
			continue
		}
		ts, ok := strictSystraceLineTimestampNS(line)
		if !ok {
			rejected = true
			continue
		}
		row := renderedRow{tsNS: ts, line: line}
		if governed {
			row.pairKind = kind
			poisonMMC = poisonMMC || !admitted
		}
		staged = append(staged, row)
	}
	if rejected {
		if observedMMC {
			sink.poisonPairKind(pairRenderMMC)
		}
		// This is the exact ftrace-plugin compatibility lane. Once the whole
		// payload fails complete text classification, unread bytes may instead
		// be a malformed structured envelope. Record delayed opacity even when
		// no exact text header was recoverable; it suppresses nothing unless the
		// same physical source actually contains MMC endpoints.
		sink.markPairCaptureOpaque(pairRenderMMC)
		return 0, false, nil
	}
	if len(staged) == 0 {
		return 0, false, nil
	}
	if sink == nil || seq == nil {
		return 0, false, fmt.Errorf("strict systrace row sink or sequence is nil")
	}
	if poisonMMC {
		sink.poisonPairKind(pairRenderMMC)
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

// profilerTextPairAdmission is the sole bridge for profiler text compatibility
// publishers into the same source-wide pair barrier as structured protobuf
// rows.  Recognition is byte-exact and the body verdict comes from
// tracequery's canonical endpoint authority; prose/substrings never guess a
// family.
func profilerTextPairAdmission(line string) (pairRenderKind, bool, bool) {
	if !strings.Contains(line, "mmc_request_start") && !strings.Contains(line, "mmc_request_done") {
		return pairRenderUnknown, false, false
	}
	ev, ok := tracequery.ParseLine(1, line, nil)
	if ok {
		if ev.Name != "mmc_request_start" && ev.Name != "mmc_request_done" {
			return pairRenderUnknown, false, false
		}
		verdict := tracequery.FingerprintPairingEvent(ev)
		admitted := verdict.Recognized && verdict.KeyKnown && verdict.PayloadAdmitted &&
			verdict.EmitterKnown && verdict.EmitterAdmitted
		return pairRenderMMC, true, admitted
	}
	name, complete := tracequery.ProbeEventNamePrefix(line)
	if complete && (name == "mmc_request_start" || name == "mmc_request_done") {
		return pairRenderMMC, true, false
	}
	return pairRenderUnknown, false, false
}

const profilerTextPairHeaderProbeBytes = 4096

// profilerTextPairCensus uses at most a bounded header prefix. A complete
// header provides exact family provenance; an unterminated oversized prefix
// is marked opaque so a later exact MMC endpoint cannot bridge it.
func profilerTextPairCensus(part []byte) (pairRenderKind, bool, bool) {
	probe := part
	truncated := false
	if len(probe) > profilerTextPairHeaderProbeBytes {
		probe = probe[:profilerTextPairHeaderProbeBytes]
		truncated = true
	}
	name, complete := tracequery.ProbeEventNamePrefix(string(probe))
	if complete {
		if name == "mmc_request_start" || name == "mmc_request_done" {
			return pairRenderMMC, true, false
		}
		return pairRenderUnknown, false, false
	}
	return pairRenderUnknown, false, truncated
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
