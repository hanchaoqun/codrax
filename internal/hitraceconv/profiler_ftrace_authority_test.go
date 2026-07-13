package hitraceconv

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestParseProfilerPluginDataHardRouteFieldsFailClosed(t *testing.T) {
	bodyA := []byte("body-a")
	bodyB := []byte("body-b")
	valid := func(parts ...[]byte) []byte {
		return protoPayload(parts...)
	}
	tests := []struct {
		name  string
		input []byte
		issue string
	}{
		{
			name:  "name missing",
			input: valid(protoBytes(3, bodyA)),
			issue: "plugin_name_missing",
		},
		{
			name:  "name wrong wire",
			input: valid(protoVarint(1, 1), protoBytes(3, bodyA)),
			issue: "plugin_field1_wrong_wire",
		},
		{
			name:  "data wrong wire",
			input: valid(protoBytes(1, []byte("ftrace-plugin")), protoVarint(3, 1)),
			issue: "plugin_field3_wrong_wire",
		},
		{
			name: "same name duplicate",
			input: valid(
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(3, bodyA),
			),
			issue: "plugin_field1_duplicate",
		},
		{
			name: "conflicting name duplicate",
			input: valid(
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(1, []byte("other-plugin")),
				protoBytes(3, bodyA),
			),
			issue: "plugin_field1_duplicate",
		},
		{
			name: "same data duplicate",
			input: valid(
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(3, bodyA),
				protoBytes(3, bodyA),
			),
			issue: "plugin_field3_duplicate",
		},
		{
			name: "conflicting data duplicate",
			input: valid(
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(3, bodyA),
				protoBytes(3, bodyB),
			),
			issue: "plugin_field3_duplicate",
		},
		{
			name: "malformed tail",
			input: append(valid(
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(3, bodyA),
			), 0x80),
			issue: "plugin_message_malformed_wire",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decoded := parseProfilerPluginData(test.input)
			if decoded.Accepted {
				t.Fatalf("hard route corruption must reject the plugin message: %+v", decoded)
			}
			if !reflect.DeepEqual(decoded.Plugin, profilerPluginData{}) {
				t.Fatalf("rejected message must not retain a partial route: %+v", decoded.Plugin)
			}
			if !profilerPluginIssuePresent(decoded.IssueCensus, test.issue) {
				t.Fatalf("missing issue %q in %s", test.issue, decoded.IssueCensus.summary())
			}
		})
	}
}

func TestParseProfilerPluginDataAllowsAbsentPayloadAsExactDefault(t *testing.T) {
	decoded := parseProfilerPluginData(protoBytes(1, []byte("ftrace-plugin")))
	if !decoded.Accepted || decoded.Plugin.Name != "ftrace-plugin" || decoded.Plugin.Data != nil {
		t.Fatalf("proto3-absent data must remain the exact empty payload without weakening name authority: %+v", decoded)
	}
}

func TestParseProfilerPluginDataMetadataDamageDoesNotChangeRoute(t *testing.T) {
	body := []byte{0x12, 0x00, 0x2a, 0x01, 0x00}
	tests := []struct {
		name   string
		fields [][]byte
		issues []string
	}{
		{
			name: "metadata duplicate",
			fields: [][]byte{
				protoVarint(2, 1),
				protoVarint(2, 2),
				protoBytes(7, []byte("v1")),
				protoBytes(7, []byte("v2")),
			},
			issues: []string{"plugin_field2_duplicate", "plugin_field7_duplicate"},
		},
		{
			name: "metadata wrong wire",
			fields: [][]byte{
				protoBytes(2, []byte("not-a-status")),
				protoVarint(7, 1),
			},
			issues: []string{"plugin_field2_wrong_wire", "plugin_field7_wrong_wire"},
		},
		{
			name: "metadata overflow",
			fields: [][]byte{
				protoVarint(2, uint64(^uint32(0))+1),
				protoVarint(4, 12),
				protoVarint(6, 1_000_000_000),
				protoVarint(8, uint64(^uint32(0))+1),
			},
			issues: []string{
				"plugin_status_out_of_range",
				"plugin_clock_id_out_of_range",
				"plugin_tv_nsec_out_of_range",
				"plugin_sample_interval_out_of_range",
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parts := [][]byte{
				protoBytes(1, []byte("ftrace-plugin")),
				protoBytes(3, body),
			}
			parts = append(parts, test.fields...)
			decoded := parseProfilerPluginData(protoPayload(parts...))
			if !decoded.Accepted {
				t.Fatalf("metadata damage must degrade locally, not reject routing: %+v", decoded)
			}
			if decoded.Plugin.Name != "ftrace-plugin" || !bytes.Equal(decoded.Plugin.Data, body) {
				t.Fatalf("metadata damage changed the hard route: %+v", decoded.Plugin)
			}
			for _, issue := range test.issues {
				if !profilerPluginIssuePresent(decoded.IssueCensus, issue) {
					t.Fatalf("missing issue %q in %s", issue, decoded.IssueCensus.summary())
				}
			}
		})
	}
}

func TestDecodeProfilerTracePluginResultRetainsRepeatedCPUPages(t *testing.T) {
	pageA := protoPayload(protoVarint(1, 3), protoVarint(3, 1))
	pageB := protoPayload(protoVarint(1, 3), protoVarint(3, 2))
	result := decodeProfilerTracePluginResult(protoPayload(
		protoBytes(2, pageA),
		protoBytes(2, pageB),
	))

	if result.Disposition != profilerFtracePayloadStructured {
		t.Fatalf("two legal CPU-detail occurrences must remain structured: %+v", result)
	}
	if len(result.CPUDetails) != 2 || !bytes.Equal(result.CPUDetails[0], pageA) || !bytes.Equal(result.CPUDetails[1], pageB) {
		t.Fatalf("repeated same-CPU pages must be retained in wire order: %+v", result.CPUDetails)
	}
	summary, recognized, err := decodeProfilerFtraceSummaryResult(result)
	if err != nil || !recognized {
		t.Fatalf("summarize repeated CPU pages: recognized=%t err=%v", recognized, err)
	}
	if summary.DetailMessages != 2 || len(summary.DetailCPUs) != 1 || !summary.DetailCPUs[3] {
		t.Fatalf("same CPU may legally span multiple detail pages: %+v", summary)
	}
}

func TestRenderProfilerTracePluginResultRetainsRepeatedEventsAcrossSameCPUPages(t *testing.T) {
	event := func(ts uint64, marker string) []byte {
		return syntheticTracePluginFtraceEvent(ts, 7, 7, "worker", 1109, protoBytes(2, []byte(marker)))
	}
	pageA := protoPayload(
		protoVarint(1, 3),
		event(5_000_000_000, "B|7|first"),
		event(5_000_001_000, "E|7|"),
	)
	pageB := protoPayload(
		protoVarint(1, 3),
		event(5_000_002_000, "B|7|second"),
	)
	result := decodeProfilerTracePluginResult(protoPayload(
		protoBytes(2, pageA),
		protoBytes(2, pageB),
	))
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredResult(result, &seq, sink)
	if err != nil {
		t.Fatalf("render repeated detail/event fields: %v", err)
	}
	if rows != 3 || seq != 3 || sink.stats.RowsAccepted != 3 {
		t.Fatalf("official repeated detail/event fields must render every legal occurrence: rows=%d seq=%d sink=%+v coverage=%+v", rows, seq, sink.stats, coverage)
	}
	if !coverageHasEmitted(coverage, "builtin_modern_ftrace:trace_marker", "print", 3) {
		t.Fatalf("repeated print records missing typed coverage: %+v", coverage)
	}
}

func TestDecodeProfilerTracePluginResultWrongWireDoesNotStarveLegalSibling(t *testing.T) {
	page := protoPayload(protoVarint(1, 4))
	result := decodeProfilerTracePluginResult(protoPayload(
		protoVarint(2, 99),
		protoBytes(2, page),
	))

	if result.Disposition != profilerFtracePayloadStructured || len(result.CPUDetails) != 1 || !bytes.Equal(result.CPUDetails[0], page) {
		t.Fatalf("wrong-wire occurrence must not erase a legal repeated sibling: %+v", result)
	}
	if !profilerAuthorityIssuePresent(result.Issues, "envelope_trace_plugin_field2_wrong_wire") {
		t.Fatalf("wrong-wire occurrence needs explicit envelope coverage: %v", result.Issues)
	}
}

func TestDecodeProfilerTracePluginResultInvalidVersionDuplicateIsNotAuthoritative(t *testing.T) {
	result := decodeProfilerTracePluginResult(protoPayload(
		protoBytes(7, []byte("trace-plugin-v1")),
		protoVarint(7, 1),
	))

	if result.Disposition != profilerFtracePayloadStructured {
		t.Fatalf("version damage is local envelope degradation: %+v", result)
	}
	if len(result.Versions) != 0 {
		t.Fatalf("a valid+wrong-wire duplicate cannot publish a chosen version: %q", result.Versions)
	}
	for _, issue := range []string{
		"envelope_trace_plugin_field7_wrong_wire",
		"envelope_trace_plugin_version_duplicate",
	} {
		if !profilerAuthorityIssuePresent(result.Issues, issue) {
			t.Fatalf("missing issue %q in %v", issue, result.Issues)
		}
	}
}

func TestDecodeProfilerTracePluginResultMalformedTailClearsPartialAuthority(t *testing.T) {
	page := protoPayload(protoVarint(1, 5))
	payload := append(protoBytes(2, page), 0x12, 0x80)
	result := decodeProfilerTracePluginResult(payload)

	if result.Disposition != profilerFtracePayloadMalformed {
		t.Fatalf("known structured prefix plus malformed tail must stay typed-malformed: %+v", result)
	}
	if len(result.CPUStats) != 0 || len(result.CPUDetails) != 0 || len(result.Symbols) != 0 ||
		len(result.Clocks) != 0 || len(result.Versions) != 0 || len(result.CommDicts) != 0 {
		t.Fatalf("malformed envelope must not retain partial typed authority: %+v", result)
	}
	if !profilerAuthorityIssuePresent(result.Issues, "envelope_trace_plugin_malformed_wire") {
		t.Fatalf("malformed envelope issue missing: %v", result.Issues)
	}
}

func TestDecodeProfilerTracePluginResultKnownTruncationAndUnsupportedWireStayMalformed(t *testing.T) {
	tests := []struct {
		name    string
		payload []byte
	}{
		{name: "truncated field2 length", payload: []byte{0x12, 0x80}},
		{name: "unsupported field2 group wire", payload: []byte{0x13}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := decodeProfilerTracePluginResult(test.payload)
			if result.Disposition != profilerFtracePayloadMalformed ||
				!profilerAuthorityIssuePresent(result.Issues, "envelope_trace_plugin_malformed_wire") {
				t.Fatalf("known top-level key must retain typed malformed provenance: %+v", result)
			}
		})
	}
}

func TestProfilerFtraceNestedMetadataRejectsAmbiguousValues(t *testing.T) {
	statsCases := [][]byte{
		protoPayload(protoVarint(1, 0), protoVarint(1, 1)),
		protoVarint(1, 2),
		protoBytes(3, []byte("boot\ninjected")),
		protoMessage(2, protoVarint(1, 1), protoVarint(1, 2)),
	}
	for index, payload := range statsCases {
		if _, err := decodeProfilerFtraceCPUStats(payload); err == nil {
			t.Fatalf("CPU stats case %d must reject duplicate/range/display ambiguity", index)
		}
	}
	clockCases := [][]byte{
		protoVarint(1, 7),
		protoPayload(protoVarint(1, 1), protoVarint(1, 2)),
		protoMessage(2, protoVarint(1, uint64(^uint32(0))+1)),
		protoMessage(2, protoVarint(2, 1_000_000_000)),
		protoPayload(protoMessage(2), protoMessage(2)),
	}
	for index, payload := range clockCases {
		if _, err := decodeProfilerFtraceClockDetail(payload); err == nil {
			t.Fatalf("clock detail case %d must reject enum/duplicate/source-width/nsec ambiguity", index)
		}
	}
	if _, err := decodeProfilerFtraceSymbolDetail(protoBytes(2, []byte("symbol\ninjected"))); err == nil {
		t.Fatal("symbol_name must be safe single-line display metadata")
	}
	for _, payload := range [][]byte{
		protoPayload(protoVarint(1, 7), protoVarint(1, 8)),
		protoBytes(2, []byte("worker\ninjected")),
		protoVarint(1, uint64(^uint32(0))),
	} {
		if err := decodeProfilerFtraceCommDict(payload); err == nil {
			t.Fatalf("ambiguous comm_dict must remain coverage-only: %x", payload)
		}
	}
}

func TestProfilerFtraceCPUDetailMixedSiblingSummaryRenderParity(t *testing.T) {
	good := syntheticTracePluginFtraceEvent(5_000_000_000, 7, 7, "worker", 1109, protoBytes(2, []byte("B|7|good")))
	page := protoPayload(
		protoVarint(1, 3),
		good,
		protoBytes(2, []byte{0x80}),
		protoVarint(2, 1),
		protoVarint(3, 1),
		protoVarint(3, 2),
	)
	result := decodeProfilerTracePluginResult(protoBytes(2, page))
	summary, recognized, err := decodeProfilerFtraceSummaryResult(result)
	if err != nil || !recognized {
		t.Fatalf("summarize mixed CPU-detail page: recognized=%t err=%v", recognized, err)
	}
	if summary.DetailMessages != 1 || summary.DetailEventCount != 1 || summary.EventFieldCounts[1109] != 1 || summary.DetailOverwriteOK {
		t.Fatalf("summary must retain only the legal repeated event and suppress ambiguous overwrite: %+v", summary)
	}
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 0
	rows, coverage, err := renderProfilerFtraceStructuredResult(result, &seq, sink)
	if err != nil {
		t.Fatalf("render mixed CPU-detail page: %v", err)
	}
	if rows != 1 || sink.stats.RowsAccepted != 1 {
		t.Fatalf("bad repeated siblings must not starve the legal event: rows=%d sink=%+v coverage=%+v", rows, sink.stats, coverage)
	}
	if !coverageTableHasSkipped(coverage, "__event_envelope__", "envelope_event_malformed_wire") ||
		!coverageTableHasSkipped(coverage, "__cpu_detail_envelope__", "envelope_event_container_wrong_wire") ||
		!coverageTableHasSkipped(coverage, "__cpu_detail_envelope__", "envelope_overwrite_invalid") {
		t.Fatalf("mixed sibling damage missing scoped typed coverage: %+v", coverage)
	}
}

func TestProfilerFtraceMetadataAggregatesFailClosedOnOverflow(t *testing.T) {
	stats := protoPayload(
		protoMessage(2, protoVarint(1, 0), protoVarint(2, ^uint64(0))),
		protoMessage(2, protoVarint(1, 1), protoVarint(2, 1)),
	)
	statsResult := decodeProfilerTracePluginResult(protoBytes(1, stats))
	statsSummary, _, err := decodeProfilerFtraceSummaryResult(statsResult)
	if err != nil {
		t.Fatalf("summarize overflowing stats: %v", err)
	}
	if statsSummary.StartTotalsValid || !profilerAuthorityIssuePresent(statsSummary.Issues, "ftrace_cpu_stats_start_aggregate_overflow") {
		t.Fatalf("overflowing CPU totals must not wrap into authoritative metadata: %+v", statsSummary)
	}
	if strings.Contains(profilerFtraceSummaryCaveat(statsSummary), "observed_entries=") {
		t.Fatalf("overflowed aggregate must not be published: %s", profilerFtraceSummaryCaveat(statsSummary))
	}

	detailResult := decodeProfilerTracePluginResult(protoPayload(
		protoBytes(2, protoPayload(protoVarint(1, 0), protoVarint(3, ^uint64(0)))),
		protoBytes(2, protoPayload(protoVarint(1, 0), protoVarint(3, 1))),
	))
	detailSummary, _, err := decodeProfilerFtraceSummaryResult(detailResult)
	if err != nil {
		t.Fatalf("summarize overflowing detail overwrite: %v", err)
	}
	if detailSummary.DetailOverwriteOK || !profilerAuthorityIssuePresent(detailSummary.Issues, "ftrace_cpu_detail_overwrite_aggregate_overflow") {
		t.Fatalf("overflowing detail aggregate must remain explicit degradation: %+v", detailSummary)
	}
	if strings.Contains(profilerFtraceSummaryCaveat(detailSummary), "detail_overwrite=") {
		t.Fatalf("overflowed detail aggregate must not be published: %s", profilerFtraceSummaryCaveat(detailSummary))
	}
}

func TestProfilerPluginMetadataDisclosesProto3RealtimeClockDefault(t *testing.T) {
	decoded := parseProfilerPluginData(protoPayload(
		protoBytes(1, []byte("ftrace-plugin")),
		protoVarint(5, 12),
		protoVarint(6, 34),
	))
	if !decoded.Accepted {
		t.Fatalf("parse outer timing tuple: %+v", decoded)
	}
	caveat := profilerPluginMetadataCaveat(decoded.Plugin.Name, decoded.Plugin)
	if !strings.Contains(caveat, "clock_id=REALTIME") || !strings.Contains(caveat, "tv=12.000000034") {
		t.Fatalf("outer timing tuple must disclose proto3 default clock domain: %s", caveat)
	}
}

func TestProfilerPluginMetadataNeverDefaultsDamagedTimingTuple(t *testing.T) {
	tests := []struct {
		name   string
		fields [][]byte
	}{
		{
			name: "clock wrong wire",
			fields: [][]byte{
				protoBytes(4, []byte("bad")),
				protoVarint(5, 12),
				protoVarint(6, 34),
			},
		},
		{
			name: "nsec out of range",
			fields: [][]byte{
				protoVarint(5, 12),
				protoVarint(6, 1_000_000_000),
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			parts := [][]byte{protoBytes(1, []byte("ftrace-plugin"))}
			parts = append(parts, test.fields...)
			decoded := parseProfilerPluginData(protoPayload(parts...))
			if !decoded.Accepted {
				t.Fatalf("damaged soft timing metadata must not change hard routing: %+v", decoded)
			}
			caveat := profilerPluginMetadataCaveat(decoded.Plugin.Name, decoded.Plugin)
			if strings.Contains(caveat, "clock_id=") || strings.Contains(caveat, "tv=") {
				t.Fatalf("damaged timing fields must not be presented as proto3 defaults: %s issues=%s", caveat, decoded.IssueCensus.summary())
			}
		})
	}
}

func TestAddStrictSystraceRowsRejectsWholePayloadAtomically(t *testing.T) {
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 17
	valid := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|valid"
	payload := []byte(valid + "\nthis fragment only resembles prose at 5.001000")

	rows, classified, err := addStrictSystraceRowsFromBytes(payload, &seq, sink)
	if err != nil {
		t.Fatalf("reject malformed legacy payload: %v", err)
	}
	if rows != 0 || classified || seq != 17 || sink.stats.RowsAccepted != 0 || len(sink.rows) != 0 {
		t.Fatalf("one malformed fragment must reject before any row/sequence mutation: rows=%d classified=%t seq=%d sink=%+v", rows, classified, seq, sink.stats)
	}
}

func TestAddStrictSystraceRowsRejectsNULDelimitedRows(t *testing.T) {
	row := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|valid"
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 9
	rows, classified, err := addStrictSystraceRowsFromBytes([]byte(row+"\x00"+row), &seq, sink)
	if err != nil {
		t.Fatalf("reject NUL-delimited payload: %v", err)
	}
	if rows != 0 || classified || seq != 9 || sink.stats.RowsAccepted != 0 {
		t.Fatalf("NUL is payload control data, not a legacy line delimiter: rows=%d classified=%t seq=%d sink=%+v", rows, classified, seq, sink.stats)
	}
}

func TestAddStrictSystraceRowsRequiresAnchoredFiniteTimestamp(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{
			name: "timestamp looking prose is not a row",
			line: "evidence mentions 5.000000: tracing_mark_write: B|7|not-a-header",
		},
		{
			name: "timestamp cannot overflow nanoseconds",
			line: "worker-7  ( 7) [001] ....  18446744074.000000: tracing_mark_write: B|7|overflow",
		},
		{
			name: "non finite spelling is not admitted",
			line: "worker-7  ( 7) [001] ....  Inf: tracing_mark_write: B|7|infinite",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink("", 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 3
			rows, classified, err := addStrictSystraceRowsFromBytes([]byte(test.line), &seq, sink)
			if err != nil {
				t.Fatalf("classify invalid line: %v", err)
			}
			if rows != 0 || classified || seq != 3 || sink.stats.RowsAccepted != 0 {
				t.Fatalf("invalid timestamp/header entered compatibility lane: rows=%d classified=%t seq=%d sink=%+v", rows, classified, seq, sink.stats)
			}
		})
	}
}

func TestAddStrictSystraceRowsAuditsCommentUTF8AndControls(t *testing.T) {
	validRow := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|valid"
	tests := []struct {
		name    string
		comment []byte
	}{
		{name: "invalid UTF-8", comment: []byte{'#', ' ', 0xff}},
		{name: "embedded control", comment: []byte("# invalid\x01comment")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sink, err := newTraceDBRowSink("", 128)
			if err != nil {
				t.Fatal(err)
			}
			defer sink.cleanup()
			seq := 11
			payload := append(append(append([]byte(nil), test.comment...), '\n'), []byte(validRow)...)
			rows, classified, err := addStrictSystraceRowsFromBytes(payload, &seq, sink)
			if err != nil {
				t.Fatalf("audit comment: %v", err)
			}
			if rows != 0 || classified || seq != 11 || sink.stats.RowsAccepted != 0 {
				t.Fatalf("invalid comment must reject the whole compatibility payload: rows=%d classified=%t seq=%d sink=%+v", rows, classified, seq, sink.stats)
			}
		})
	}
}

func TestAddStrictSystraceRowsAcceptsStandardCRLFWithUnicodeComment(t *testing.T) {
	rowA := "worker-7  ( 7) [001] ....  5.000000: tracing_mark_write: B|7|first"
	rowB := "worker-7  ( 7) [001] ....  5.001000: tracing_mark_write: E|7|"
	payload := strings.Join([]string{"# 东湖兼容格式", rowA, rowB, ""}, "\r\n")
	sink, err := newTraceDBRowSink("", 128)
	if err != nil {
		t.Fatal(err)
	}
	defer sink.cleanup()
	seq := 23

	rows, classified, err := addStrictSystraceRowsFromBytes([]byte(payload), &seq, sink)
	if err != nil {
		t.Fatalf("accept standard CRLF payload: %v", err)
	}
	if rows != 2 || !classified || seq != 25 || sink.stats.RowsAccepted != 2 || len(sink.rows) != 2 {
		t.Fatalf("standard CRLF payload was not admitted exactly once per data row: rows=%d classified=%t seq=%d sink=%+v", rows, classified, seq, sink.stats)
	}
	if sink.rows[0].line != rowA || sink.rows[1].line != rowB || strings.Contains(sink.rows[0].line+sink.rows[1].line, "\r") {
		t.Fatalf("CRLF normalization changed row content: %+v", sink.rows)
	}
}

func profilerAuthorityIssuePresent(issues []string, want string) bool {
	for _, issue := range issues {
		if issue == want {
			return true
		}
	}
	return false
}

func profilerPluginIssuePresent(census profilerPluginIssueCensus, want string) bool {
	for kind := profilerPluginIssueKind(0); kind < profilerPluginIssueKindCount; kind++ {
		if kind.label() == want {
			return census.Occurrences[int(kind)] > 0
		}
	}
	return false
}
