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

type profilerPairAdmission struct {
	Kind      pairRenderKind
	Governed  bool
	Admitted  bool
	LaneKnown bool
	Lane      string
}

func (a profilerPairAdmission) poison(sink *traceDBRowSink) {
	if !a.Governed || sink == nil {
		return
	}
	if a.Kind == pairRenderF2FS && a.LaneKnown {
		sink.poisonPairLane(a.Kind, a.Lane)
		return
	}
	sink.poisonPairKind(a.Kind)
}

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
	Issues            profilerTracePluginIssueCensus
	IssueOverflow     bool
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
			if kind, ok := profilerTracePluginWrongWireIssue(field); !ok || !out.Issues.observe(kind, 1) {
				out.IssueOverflow = true
			}
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
		if !out.Issues.observe(profilerTracePluginIssueMalformedWire, 1) {
			out.IssueOverflow = true
		}
		return out
	}
	if !recognized {
		out.Disposition = profilerFtracePayloadNotStructured
		return out
	}
	if versionOccurrences > 1 {
		if !out.Issues.observeVersionDuplicate(uint64(versionOccurrences - 1)) {
			out.IssueOverflow = true
		}
		out.Versions = nil
	}
	out.Disposition = profilerFtracePayloadStructured
	return out
}

func profilerTracePluginResultEvents(result profilerTracePluginResult) ([]profilerFtraceEventRecord, error) {
	var out []profilerFtraceEventRecord
	if result.Disposition == profilerFtracePayloadMalformed {
		if result.PairFamilies != 0 || result.PairCaptureOpaque {
			record := profilerFtraceEventRecord{
				Field: profilerFtraceCPUDetailEnvelopeField, PairFamilies: result.PairFamilies,
				PairCaptureOpaque: result.PairCaptureOpaque,
			}
			if issueErr := record.appendEnvelopeIssue(profilerFtraceEventIssueEnvelopeTracePluginMalformedWire); issueErr != nil {
				return nil, issueErr
			}
			out = append(out, record)
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
	if result.Issues.empty() {
		return nil
	}
	total, ok := result.Issues.totalOccurrences()
	if !ok {
		return nil
	}
	rowsRead, ok := profilerContainerCountToInt(total)
	if !ok {
		return nil
	}
	fields := map[string]string{
		"schema_profile": "TracePluginResult repeated fields 1/2/5/6/8 and singular version field 7",
	}
	result.Issues.appendFieldSources(fields)
	return []TraceDBCoverage{{
		Family:       "builtin_modern_ftrace:trace_plugin_envelope",
		Table:        "__trace_plugin_envelope__",
		Role:         "unsupported_input",
		Found:        true,
		RowsRead:     rowsRead,
		Skipped:      result.Issues.summary(),
		FieldSources: fields,
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
	var rejectedPairs []profilerPairAdmission
	observed := map[pairRenderKind]bool{}
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
			kind, governed, _ := profilerTextPairCensus(part)
			if governed {
				observed[kind] = true
			}
			rejected = true
			continue
		}
		line := string(part)
		pair := profilerTextPairAdmission(line)
		if pair.Governed {
			observed[pair.Kind] = true
			if !pair.Admitted {
				rejectedPairs = append(rejectedPairs, pair)
			}
		}
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
		if pair.Governed {
			row.pairKind = pair.Kind
			row.pairLane = pair.Lane
		}
		staged = append(staged, row)
	}
	if rejected {
		for kind := range observed {
			sink.poisonPairKind(kind)
		}
		// This is the exact ftrace-plugin compatibility lane. Once the whole
		// payload fails complete text classification, unread bytes may instead
		// be a malformed structured envelope. Record delayed opacity even when
		// no exact text header was recoverable; it suppresses nothing unless the
		// same physical source actually contains MMC or F2FS endpoints.
		sink.markPairCaptureOpaque(pairRenderMMC)
		sink.markPairCaptureOpaque(pairRenderF2FS)
		return 0, false, nil
	}
	if len(staged) == 0 {
		return 0, false, nil
	}
	if sink == nil || seq == nil {
		return 0, false, fmt.Errorf("strict systrace row sink or sequence is nil")
	}
	for _, pair := range rejectedPairs {
		pair.poison(sink)
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
func profilerTextPairAdmission(line string) profilerPairAdmission {
	if !strings.Contains(line, "mmc_request_") && !strings.Contains(line, "f2fs_") {
		return profilerPairAdmission{}
	}
	ev, ok := tracequery.ParseLine(1, line, nil)
	if ok {
		kind, governed := profilerPairKindForExactName(ev.Name)
		if !governed {
			return profilerPairAdmission{}
		}
		verdict := tracequery.FingerprintPairingEvent(ev)
		admitted := verdict.Recognized && verdict.KeyKnown && verdict.PayloadAdmitted &&
			verdict.EmitterKnown && verdict.EmitterAdmitted
		return profilerPairAdmission{
			Kind: kind, Governed: true, Admitted: admitted,
			LaneKnown: verdict.KeyKnown && verdict.SemanticKey != "", Lane: verdict.SemanticKey,
		}
	}
	name, complete := tracequery.ProbeEventNamePrefix(line)
	if complete {
		if kind, governed := profilerPairKindForExactName(name); governed {
			return profilerPairAdmission{Kind: kind, Governed: true}
		}
	}
	return profilerPairAdmission{}
}

func profilerPairKindForExactName(name string) (pairRenderKind, bool) {
	switch name {
	case "mmc_request_start", "mmc_request_done":
		return pairRenderMMC, true
	case "f2fs_sync_file_enter", "f2fs_sync_file_exit", "f2fs_direct_IO_enter", "f2fs_direct_IO_exit",
		"f2fs_write_begin", "f2fs_write_end":
		return pairRenderF2FS, true
	default:
		return pairRenderUnknown, false
	}
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
		if kind, governed := profilerPairKindForExactName(name); governed {
			return kind, true, false
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
