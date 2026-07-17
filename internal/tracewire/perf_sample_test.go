package tracewire

import (
	"errors"
	"math"
	"strconv"
	"strings"
	"testing"
)

func canonicalSimpleperfTextRow() PerfSampleRow {
	return PerfSampleRow{
		Layout:              PerfSampleLayoutBase,
		CPU:                 5,
		CPUKnown:            true,
		PID:                 1234,
		TID:                 5678,
		ThreadComm:          "Render Thread",
		SampleWeight:        10000,
		Event:               "cpu-cycles",
		Symbol:              "Foo::bar",
		DSO:                 "libfoo.so",
		IP:                  "0x1234",
		Callchain:           "main;A;Foo::bar",
		Source:              PerfSampleSourceSimpleperfReportSample,
		SymbolizationStatus: PerfSymbolizationSymbolized,
		Clock:               PerfSampleClockRecord,
		ClockConfidence:     PerfClockConfidenceAssumed,
		CallchainStatus:     PerfCallchainStatusSymbolized,
	}
}

func TestBuildPerfSampleBodyExactFiveWriterLayouts(t *testing.T) {
	tests := []struct {
		name string
		row  PerfSampleRow
		want string
	}{
		{
			name: "simpleperf text",
			row:  canonicalSimpleperfTextRow(),
			want: `perf_sample: cpu=5 cpu_known=true pid=1234 tid=5678 thread_comm="Render Thread" sample_weight=10000 event="cpu-cycles" symbol="Foo::bar" dso="libfoo.so" ip="0x1234" callchain="main;A;Foo::bar" source=simpleperf_report_sample symbolization_status=symbolized clock=record clock_confidence=assumed callchain_status=symbolized`,
		},
		{
			name: "simpleperf proto",
			row: PerfSampleRow{
				Layout: PerfSampleLayoutBase, CPU: -1, CPUKnown: false, PID: 20, TID: 21,
				ThreadComm: "worker", SampleWeight: 99, Event: "cpu-clock", Symbol: "Foo::bar",
				DSO: "libfoo.so", IP: "0x20", Callchain: "main;Foo::bar",
				Source: PerfSampleSourceSimpleperfReportProto, SampleKind: PerfSampleKindOnCPU,
				SymbolizationStatus: PerfSymbolizationSymbolized, Clock: PerfSampleClockSimpleperfRecord,
				ClockConfidence: PerfClockConfidenceAssumed, CallchainStatus: PerfCallchainStatusSymbolized,
			},
			want: `perf_sample: cpu=-1 cpu_known=false pid=20 tid=21 thread_comm="worker" sample_weight=99 event="cpu-clock" symbol="Foo::bar" dso="libfoo.so" ip="0x20" callchain="main;Foo::bar" source=simpleperf_report_proto sample_kind=on_cpu symbolization_status=symbolized clock=simpleperf_record clock_confidence=assumed callchain_status=symbolized`,
		},
		{
			name: "hiperf proto",
			row: PerfSampleRow{
				Layout: PerfSampleLayoutBase, CPU: -1, CPUKnown: false, PID: 30, TID: 31,
				ThreadComm: "hiperf", SampleWeight: 99, Event: "cycles", Symbol: "doWork",
				DSO: "libapp.so", IP: "0x30", Callchain: "main;doWork",
				Source: PerfSampleSourceHiperfProto, SymbolizationStatus: PerfSymbolizationSymbolized,
				Clock: PerfSampleClockMonotonicRaw, ClockConfidence: PerfClockConfidenceAssumed,
				CallchainStatus: PerfCallchainStatusSymbolized,
			},
			want: `perf_sample: cpu=-1 cpu_known=false pid=30 tid=31 thread_comm="hiperf" sample_weight=99 event="cycles" symbol="doWork" dso="libapp.so" ip="0x30" callchain="main;doWork" source=hiperf_proto symbolization_status=symbolized clock=monotonic_raw clock_confidence=assumed callchain_status=symbolized`,
		},
		{
			name: "raw perf extended",
			row: PerfSampleRow{
				Layout: PerfSampleLayoutRawExtended, CPU: 2, CPUKnown: true, PID: 40, TID: 41,
				ThreadComm: "raw", SampleWeight: 9, Event: "sched:sched_switch", Symbol: "0x1234",
				DSO: "unknown", IP: "0x1234", Callchain: "0x1000;0x1234",
				Source: PerfSampleSourceRawPerfDataFallback, SampleKind: PerfSampleKindOffCPU,
				SymbolizationStatus: PerfSymbolizationUnsymbolized, Clock: PerfSampleClockPerfData,
				ClockConfidence: PerfClockConfidenceAssumed, CallchainStatus: PerfCallchainStatusIPOnly,
				Raw: PerfRawSampleFields{
					Addr: 0xfeed, SampleID: 202, StreamID: 303, PerfWeight: 123, DataSource: 0x45,
					Transaction: 0x67, PhysicalAddr: 0x89000, CGroupID: 404, DataPageSize: 4096,
					CodePageSize: 16384, RawSize: 3, BranchCount: 1, UserRegsABI: 2,
					UserRegsCount: 8, UserStackSize: 512, AuxSize: 64,
				},
				ParserCaveats: "raw fallback caveat",
			},
			want: `perf_sample: cpu=2 cpu_known=true pid=40 tid=41 thread_comm="raw" sample_weight=9 event="sched:sched_switch" symbol="0x1234" dso="unknown" ip="0x1234" callchain="0x1000;0x1234" source=raw_perfdata_fallback sample_kind=off_cpu symbolization_status=unsymbolized clock=perf_data clock_confidence=assumed callchain_status=ip_only addr=0xfeed sample_id=202 stream_id=303 perf_weight=123 data_src=0x45 transaction=0x67 phys_addr=0x89000 cgroup_id=404 data_page_size=4096 code_page_size=16384 raw_size=3 branch_count=1 user_regs_abi=2 user_regs_count=8 user_stack_size=512 aux_size=64 parser_caveats="raw fallback caveat"`,
		},
		{
			name: "sql resolved",
			row: PerfSampleRow{
				Layout: PerfSampleLayoutResolvedIdentity, CPU: 3, CPUKnown: true, PID: 50, TID: 51,
				ThreadComm: "sql", SampleWeight: 7, Event: "cycles", Symbol: "Hot", DSO: "lib.so",
				IP: "0x50", Callchain: "root;Hot", Source: PerfSampleSourceTraceStreamerDB,
				SampleKind: PerfSampleKindOnCPU, SymbolizationStatus: PerfSymbolizationSymbolized,
				Clock: PerfSampleClockTraceStreamerDB, ClockConfidence: PerfClockConfidenceCalibrated,
				CallchainStatus:  PerfCallchainStatusSymbolized,
				SampleKindSource: PerfSampleKindSourceSchedulerRunning, PerfThreadComm: "perf alias",
				CommSource: PerfIdentitySourceTraceThread, ProcessIDSource: PerfIdentitySourceTraceThread,
			},
			want: `perf_sample: cpu=3 cpu_known=true pid=50 tid=51 thread_comm="sql" sample_weight=7 event="cycles" symbol="Hot" dso="lib.so" ip="0x50" callchain="root;Hot" source=trace_streamer_db sample_kind=on_cpu symbolization_status=symbolized clock=trace_streamer_db clock_confidence=calibrated callchain_status=symbolized thread_identity_known=true resolution=resolved lifecycle_unverified=false sample_kind_source=scheduler_running perf_thread_comm="perf alias" comm_source=trace_thread process_id_source=trace_thread`,
		},
		{
			name: "sql source only",
			row: PerfSampleRow{
				Layout: PerfSampleLayoutSourceOnlyIdentity, CPU: 4, CPUKnown: true, PID: 0, TID: 0,
				ThreadComm: "", SampleWeight: 5, Event: "cycles", Symbol: "Hot", DSO: "lib.so",
				IP: "", Callchain: "", Source: PerfSampleSourceTraceStreamerDB,
				SampleKind: PerfSampleKindUnknown, SymbolizationStatus: PerfSymbolizationPartial,
				Clock: PerfSampleClockTraceStreamerDB, ClockConfidence: PerfClockConfidenceCalibrated,
				CallchainStatus: PerfCallchainStatusMissing, PerfSourceTID: 777, PerfSourcePID: 0,
				PerfSourceComm: "source worker",
			},
			want: `perf_sample: cpu=4 cpu_known=true pid=0 tid=0 thread_comm="" sample_weight=5 event="cycles" symbol="Hot" dso="lib.so" ip="" callchain="" source=trace_streamer_db sample_kind=unknown symbolization_status=partial clock=trace_streamer_db clock_confidence=calibrated callchain_status=missing thread_identity_known=false resolution=perf_source_only lifecycle_unverified=true perf_source_tid=777 perf_source_pid=0 perf_source_comm="source worker"`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := BuildPerfSampleBody(tc.row)
			if err != nil {
				t.Fatalf("BuildPerfSampleBody: %v", err)
			}
			if got != tc.want {
				t.Fatalf("wire drift:\n got %s\nwant %s", got, tc.want)
			}
			fields, parseErr := ParsePerfKV(strings.TrimPrefix(got, "perf_sample:"))
			if parseErr != nil || len(fields) == 0 {
				t.Fatalf("writer produced unreadable body: fields=%d err=%v", len(fields), parseErr)
			}
		})
	}
}

func TestBuildPerfSampleBodyMetadataAndCallchainBudgets(t *testing.T) {
	type mutation struct {
		name  string
		field string
		set   func(*PerfSampleRow, string)
	}
	mutations := []mutation{
		{"thread comm", "thread_comm", func(row *PerfSampleRow, value string) { row.ThreadComm = value }},
		{"event", "event", func(row *PerfSampleRow, value string) { row.Event = value }},
		{"symbol", "symbol", func(row *PerfSampleRow, value string) { row.Symbol = value }},
		{"dso", "dso", func(row *PerfSampleRow, value string) { row.DSO = value }},
		{"ip", "ip", func(row *PerfSampleRow, value string) { row.IP = value }},
	}
	for _, mutation := range mutations {
		t.Run(mutation.name, func(t *testing.T) {
			row := canonicalSimpleperfTextRow()
			mutation.set(&row, "  "+strings.Repeat("x", MaxPerfMetadataBytes)+"  ")
			if _, err := BuildPerfSampleBody(row); err != nil {
				t.Fatalf("inclusive decoded metadata cap rejected: %v", err)
			}
			mutation.set(&row, strings.Repeat("x", MaxPerfMetadataBytes+1))
			assertPerfWireError(t, BuildPerfSampleBodyError(row), mutation.field, "decoded_value_too_long", MaxPerfMetadataBytes, MaxPerfMetadataBytes+1)
		})
	}

	row := canonicalSimpleperfTextRow()
	row.Callchain = strings.Repeat("c", MaxPerfCallchainBytes)
	if _, err := BuildPerfSampleBody(row); err != nil {
		t.Fatalf("inclusive callchain cap rejected: %v", err)
	}
	row.Callchain += "c"
	assertPerfWireError(t, BuildPerfSampleBodyError(row), "callchain", "decoded_value_too_long", MaxPerfCallchainBytes, MaxPerfCallchainBytes+1)

	raw := canonicalRawPerfRow()
	raw.ParserCaveats = strings.Repeat("c", MaxPerfMetadataBytes+1)
	if _, err := BuildPerfSampleBody(raw); err != nil {
		t.Fatalf("non-identity parser caveat inherited metadata cap: %v", err)
	}
	raw.ParserCaveats = strings.Repeat("c", MaxPerfParserCaveatsBytes)
	if _, err := BuildPerfSampleBody(raw); err != nil {
		t.Fatalf("inclusive parser caveat cap rejected: %v", err)
	}
	raw.ParserCaveats += "c"
	assertPerfWireError(t, BuildPerfSampleBodyError(raw), "parser_caveats", "decoded_value_too_long", MaxPerfParserCaveatsBytes, MaxPerfParserCaveatsBytes+1)
}

func TestBuildPerfSampleBodyRawInt64BackedFieldsCloseWriterReaderDomain(t *testing.T) {
	fields := []struct {
		name string
		set  func(*PerfRawSampleFields, uint64)
	}{
		{"perf_weight", func(raw *PerfRawSampleFields, value uint64) { raw.PerfWeight = value }},
		{"data_page_size", func(raw *PerfRawSampleFields, value uint64) { raw.DataPageSize = value }},
		{"code_page_size", func(raw *PerfRawSampleFields, value uint64) { raw.CodePageSize = value }},
		{"raw_size", func(raw *PerfRawSampleFields, value uint64) { raw.RawSize = value }},
		{"branch_count", func(raw *PerfRawSampleFields, value uint64) { raw.BranchCount = value }},
		{"user_stack_size", func(raw *PerfRawSampleFields, value uint64) { raw.UserStackSize = value }},
		{"aux_size", func(raw *PerfRawSampleFields, value uint64) { raw.AuxSize = value }},
	}
	for _, field := range fields {
		t.Run(field.name, func(t *testing.T) {
			row := canonicalRawPerfRow()
			field.set(&row.Raw, math.MaxInt64)
			if _, err := BuildPerfSampleBody(row); err != nil {
				t.Fatalf("MaxInt64 rejected: %v", err)
			}
			field.set(&row.Raw, math.MaxInt64+1)
			assertPerfWireError(t, BuildPerfSampleBodyError(row), field.name, "out_of_range", math.MaxInt64, math.MaxInt64+1)
		})
	}
}

func TestBuildPerfSampleBodyHostileQuotedMetadataRoundTrips(t *testing.T) {
	row := canonicalSimpleperfTextRow()
	row.ThreadComm = `  Render "鸿蒙" Thread  `
	row.Symbol = "类验证\t阶段\n下一层"
	row.DSO = `C:\Program Files\鸿蒙\libfoo.dll`
	body, err := BuildPerfSampleBody(row)
	if err != nil {
		t.Fatal(err)
	}
	fields, parseErr := ParsePerfKV(strings.TrimPrefix(body, "perf_sample:"))
	if parseErr != nil {
		t.Fatal(parseErr)
	}
	values := map[string]string{}
	for _, field := range fields {
		values[field.Key] = field.Value
	}
	if values["thread_comm"] != `Render "鸿蒙" Thread` || values["symbol"] != row.Symbol || values["dso"] != row.DSO {
		t.Fatalf("quoted round-trip drift: %+v", values)
	}

	for _, tc := range []struct {
		field string
		set   func(*PerfSampleRow)
	}{
		{"symbol", func(row *PerfSampleRow) { row.Symbol = string([]byte{'x', 0xff}) }},
		{"callchain", func(row *PerfSampleRow) { row.Callchain = string([]byte{'x', 0xff}) }},
	} {
		t.Run(tc.field+" invalid UTF-8", func(t *testing.T) {
			bad := canonicalSimpleperfTextRow()
			tc.set(&bad)
			assertPerfWireError(t, BuildPerfSampleBodyError(bad), tc.field, "invalid_utf8", 0, 2)
		})
	}
}

func TestBuildPerfSampleBodyWeightAndClosedLayoutFailures(t *testing.T) {
	row := canonicalSimpleperfTextRow()
	row.SampleWeight = math.MaxInt64
	body, err := BuildPerfSampleBody(row)
	if err != nil || !strings.Contains(body, "sample_weight="+strconv.FormatInt(math.MaxInt64, 10)) {
		t.Fatalf("MaxInt64 weight: body=%q err=%v", body, err)
	}
	for _, value := range []int64{0, -1} {
		bad := canonicalSimpleperfTextRow()
		bad.SampleWeight = value
		var typed *PerfWireBuildError
		if err := BuildPerfSampleBodyError(bad); !errors.As(err, &typed) || typed.Field != "sample_weight" || typed.Reason != "not_positive" {
			t.Fatalf("weight %d error=%T %v", value, err, err)
		}
	}

	badProto := canonicalSimpleperfTextRow()
	badProto.Source = PerfSampleSourceSimpleperfReportProto
	badProto.Clock = PerfSampleClockSimpleperfRecord
	if err := BuildPerfSampleBodyError(badProto); err == nil {
		t.Fatal("simpleperf proto without presence-bearing sample_kind accepted")
	}
	badLate := canonicalSimpleperfTextRow()
	badLate.Raw.Addr = 1
	if err := BuildPerfSampleBodyError(badLate); err == nil {
		t.Fatal("raw late field escaped base layout")
	}
	badNote := canonicalSQLResolvedRow()
	badNote.PerfThreadComm = "alias"
	badNote.CommSource = ""
	if err := BuildPerfSampleBodyError(badNote); err == nil {
		t.Fatal("incomplete SQL identity note accepted")
	}
	zeroText := canonicalSimpleperfTextRow()
	zeroText.PID = 0
	zeroText.TID = 0
	if _, err := BuildPerfSampleBody(zeroText); err != nil {
		t.Fatalf("simpleperf text present idle/pseudo identity was rejected: %v", err)
	}

	identityCases := []struct {
		name   string
		row    PerfSampleRow
		field  string
		reason string
	}{
		{name: "simpleperf proto zero pid remains rejected", row: func() PerfSampleRow {
			row := canonicalSimpleperfTextRow()
			row.Source = PerfSampleSourceSimpleperfReportProto
			row.Clock = PerfSampleClockSimpleperfRecord
			row.CPU = -1
			row.CPUKnown = false
			row.SampleKind = PerfSampleKindOnCPU
			row.PID = 0
			return row
		}(), field: "pid", reason: "out_of_range"},
		{name: "hiperf proto zero tid remains rejected", row: func() PerfSampleRow {
			row := canonicalSimpleperfTextRow()
			row.Source = PerfSampleSourceHiperfProto
			row.Clock = PerfSampleClockMonotonicRaw
			row.CPU = -1
			row.CPUKnown = false
			row.TID = 0
			return row
		}(), field: "tid", reason: "out_of_range"},
		{name: "simpleperf text unknown cpu", row: func() PerfSampleRow {
			row := canonicalSimpleperfTextRow()
			row.CPU = -1
			row.CPUKnown = false
			return row
		}(), field: "layout", reason: "profile_mismatch"},
		{name: "resolved zero pid", row: func() PerfSampleRow { row := canonicalSQLResolvedRow(); row.PID = 0; return row }(), field: "pid", reason: "out_of_range"},
		{name: "resolved zero tid", row: func() PerfSampleRow { row := canonicalSQLResolvedRow(); row.TID = 0; return row }(), field: "tid", reason: "out_of_range"},
		{name: "resolved unknown cpu", row: func() PerfSampleRow { row := canonicalSQLResolvedRow(); row.CPU = -1; row.CPUKnown = false; return row }(), field: "layout", reason: "profile_mismatch"},
		{name: "source-only common pid", row: func() PerfSampleRow { row := canonicalSQLSourceOnlyRow(); row.PID = 1; return row }(), field: "thread_identity", reason: "source_only_common_identity_present"},
		{name: "source-only common tid", row: func() PerfSampleRow { row := canonicalSQLSourceOnlyRow(); row.TID = 1; return row }(), field: "thread_identity", reason: "source_only_common_identity_present"},
		{name: "source-only common comm", row: func() PerfSampleRow { row := canonicalSQLSourceOnlyRow(); row.ThreadComm = "claimed"; return row }(), field: "thread_identity", reason: "source_only_common_identity_present"},
		{name: "source-only missing source tid", row: func() PerfSampleRow { row := canonicalSQLSourceOnlyRow(); row.PerfSourceTID = 0; return row }(), field: "perf_source_tid", reason: "out_of_range"},
		{name: "source-only negative source pid", row: func() PerfSampleRow { row := canonicalSQLSourceOnlyRow(); row.PerfSourcePID = -1; return row }(), field: "perf_source_pid", reason: "out_of_range"},
		{name: "source-only scheduler provenance", row: func() PerfSampleRow {
			row := canonicalSQLSourceOnlyRow()
			row.SampleKindSource = PerfSampleKindSourceSchedulerRunning
			return row
		}(), field: "layout", reason: "late_field_conflict"},
		{name: "resolved unknown with running provenance", row: func() PerfSampleRow {
			row := canonicalSQLResolvedRow()
			row.SampleKind = PerfSampleKindUnknown
			row.SampleKindSource = PerfSampleKindSourceSchedulerRunning
			return row
		}(), field: "sample_kind_source", reason: "provenance_mismatch"},
		{name: "raw on-cpu kind", row: func() PerfSampleRow {
			row := canonicalRawPerfRow()
			row.SampleKind = PerfSampleKindOnCPU
			return row
		}(), field: "layout", reason: "profile_mismatch"},
	}
	for _, tc := range identityCases {
		t.Run(tc.name, func(t *testing.T) {
			var typed *PerfWireBuildError
			err := BuildPerfSampleBodyError(tc.row)
			if !errors.As(err, &typed) || typed.Field != tc.field || typed.Reason != tc.reason {
				t.Fatalf("error=%T %v, want field=%s reason=%s", err, err, tc.field, tc.reason)
			}
		})
	}
}

func TestPerfSampleBodyBuilderLexicalBudgets(t *testing.T) {
	keyExact := newPerfSampleBodyBuilder()
	if err := keyExact.appendField(strings.Repeat("k", MaxPerfKVKeyBytes), "1"); err != nil {
		t.Fatalf("inclusive key cap rejected: %v", err)
	}
	keyLong := newPerfSampleBodyBuilder()
	assertPerfWireError(t, keyLong.appendField(strings.Repeat("k", MaxPerfKVKeyBytes+1), "1"), "key", "key_too_long", MaxPerfKVKeyBytes, MaxPerfKVKeyBytes+1)

	valueExact := newPerfSampleBodyBuilder()
	if err := valueExact.appendField("k", strings.Repeat("x", MaxPerfKVEncodedValueBytes)); err != nil {
		t.Fatalf("inclusive encoded value cap rejected: %v", err)
	}
	valueLong := newPerfSampleBodyBuilder()
	assertPerfWireError(t, valueLong.appendField("k", strings.Repeat("x", MaxPerfKVEncodedValueBytes+1)), "k", "encoded_value_too_long", MaxPerfKVEncodedValueBytes, MaxPerfKVEncodedValueBytes+1)

	fieldCap := newPerfSampleBodyBuilder()
	for i := 0; i < MaxPerfKVFields; i++ {
		if err := fieldCap.appendField("k", "1"); err != nil {
			t.Fatalf("field %d/%d: %v", i+1, MaxPerfKVFields, err)
		}
	}
	assertPerfWireError(t, fieldCap.appendField("k", "1"), "fields", "field_count_exceeded", MaxPerfKVFields, MaxPerfKVFields+1)

	bodyCap := newPerfSampleBodyBuilder()
	for i := 0; i < 15; i++ {
		if err := bodyCap.appendField("k", strings.Repeat("x", MaxPerfKVEncodedValueBytes)); err != nil {
			t.Fatalf("body prefix %d: %v", i, err)
		}
	}
	remainingValue := MaxPerfKVBodyBytes - bodyCap.kvBytes - 3 // leading space + k + '='
	if err := bodyCap.appendField("k", strings.Repeat("x", remainingValue)); err != nil {
		t.Fatalf("inclusive body cap rejected: %v", err)
	}
	if bodyCap.kvBytes != MaxPerfKVBodyBytes {
		t.Fatalf("body cap accounting=%d want=%d", bodyCap.kvBytes, MaxPerfKVBodyBytes)
	}
	assertPerfWireError(t, bodyCap.appendField("k", "1"), "body", "body_too_long", MaxPerfKVBodyBytes, MaxPerfKVBodyBytes+4)
}

func canonicalSQLResolvedRow() PerfSampleRow {
	return PerfSampleRow{
		Layout: PerfSampleLayoutResolvedIdentity, CPU: 0, CPUKnown: true, PID: 1, TID: 1,
		ThreadComm: "sql", SampleWeight: 1, Event: "cycles", Symbol: "Hot", DSO: "lib.so",
		IP: "0x1", Callchain: "Hot", Source: PerfSampleSourceTraceStreamerDB,
		SampleKind: PerfSampleKindOnCPU, SymbolizationStatus: PerfSymbolizationSymbolized,
		Clock: PerfSampleClockTraceStreamerDB, ClockConfidence: PerfClockConfidenceCalibrated,
		CallchainStatus: PerfCallchainStatusSymbolized,
	}
}

func canonicalSQLSourceOnlyRow() PerfSampleRow {
	return PerfSampleRow{
		Layout: PerfSampleLayoutSourceOnlyIdentity, CPU: 0, CPUKnown: true, PID: 0, TID: 0,
		ThreadComm: "", SampleWeight: 1, Event: "cycles", Symbol: "Hot", DSO: "lib.so",
		IP: "0x1", Callchain: "Hot", Source: PerfSampleSourceTraceStreamerDB,
		SampleKind: PerfSampleKindUnknown, SymbolizationStatus: PerfSymbolizationSymbolized,
		Clock: PerfSampleClockTraceStreamerDB, ClockConfidence: PerfClockConfidenceCalibrated,
		CallchainStatus: PerfCallchainStatusSymbolized, PerfSourceTID: 7,
	}
}

func canonicalRawPerfRow() PerfSampleRow {
	return PerfSampleRow{
		Layout: PerfSampleLayoutRawExtended, CPU: 0, CPUKnown: true, PID: 1, TID: 1,
		ThreadComm: "raw", SampleWeight: 1, Event: "cycles", Symbol: "0x1", DSO: "unknown",
		IP: "0x1", Callchain: "0x1", Source: PerfSampleSourceRawPerfDataFallback,
		SymbolizationStatus: PerfSymbolizationUnsymbolized, Clock: PerfSampleClockPerfData,
		ClockConfidence: PerfClockConfidenceAssumed, CallchainStatus: PerfCallchainStatusIPOnly,
	}
}

func BuildPerfSampleBodyError(row PerfSampleRow) error {
	_, err := BuildPerfSampleBody(row)
	return err
}

func assertPerfWireError(t *testing.T, err error, field, reason string, limit, actual uint64) {
	t.Helper()
	var typed *PerfWireBuildError
	if !errors.As(err, &typed) {
		t.Fatalf("error=%T %v, want *PerfWireBuildError", err, err)
	}
	if typed.Field != field || typed.Reason != reason || typed.Limit != limit || typed.Actual != actual {
		t.Fatalf("typed error=%+v want field=%s reason=%s limit=%d actual=%d", typed, field, reason, limit, actual)
	}
}
