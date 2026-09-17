package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// These are producer marker bytes, not a request to infer a jank interval or
// causal relationship. In particular, payload timestamps do not authorize
// replacing the ftrace header clock. The real tieba fixture has this same
// different-clock shape (34579... header versus 29822... payload).
const jankConversionMarkerName = "jank_event_sync: start_ts=25175823383662, end_ts=25175970920781, jank_frames=8, appid=27599"

const jankConversionHeaderNS = uint64(34_579_594_371_123)

func TestJankMarkerConversionBuiltinPreservesPayloadAndHeaderNanoseconds(t *testing.T) {
	payload := "B|27599|" + jankConversionMarkerName
	unknown := "B|27599|vendor_unknown_marker: token=9007199254740993, state=opaque"
	for _, test := range []struct {
		name  string
		build func([]byte) directMarkerTestFixture
	}{
		{"print_cstring", func(raw []byte) directMarkerTestFixture {
			return directMarkerCStringFixture("print", raw, true)
		}},
		{"print_compact_data_loc", directMarkerHarmonyCompactPrintFixture},
		{"trace_marker_data_loc", func(raw []byte) directMarkerTestFixture {
			return directMarkerDataLocFixture("tracing_mark_write", raw, false)
		}},
		{"trace_marker_fixed_array", func(raw []byte) directMarkerTestFixture {
			return directMarkerFixedFixture("tracing_mark_write", raw, false, 512)
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := test.build([]byte(payload))
			id := fixture.format.ID
			if id == 0 {
				id = 501
			}
			fields := make([]string, 0, len(fixture.format.Fields))
			for _, field := range fixture.format.Fields {
				fields = append(fields, syntheticField(field.Type, field.Name, field.Offset, field.Size, field.Signed))
			}
			events := make([]syntheticRawEvent, 0, 4)
			for index, raw := range []string{payload, "E|27599|", unknown, "E|27599|"} {
				events = append(events, syntheticRawEvent{EventID: uint16(id), OffsetNS: uint32(index * 1001), Content: test.build([]byte(raw)).content})
			}
			page := syntheticRawPageEvents(events)
			binary.LittleEndian.PutUint64(page[:8], jankConversionHeaderNS)
			var capture bytes.Buffer
			writeFileHeader(&capture, 1)
			writeSegment(&capture, segmentEventsFormat, []byte(strings.Join(syntheticFormatBlock(fixture.format.Name, id, fields), "\n")))
			writeSegment(&capture, segmentCmdlines, []byte("100 marker-emitter\n"))
			writeSegment(&capture, segmentTGIDs, []byte("100 100\n"))
			writeSegment(&capture, segmentRawTrace, page)

			result, text := convertJankMarkerCapture(t, "marker.sys", capture.Bytes())
			if result.EventsWritten != 4 || result.UnknownEventCount != 0 || result.MissingFormatCount != 0 {
				t.Fatalf("marker records were lost: result=%+v\n%s", result, text)
			}
			assertJankConversionLine(t, text, "34579.594371123", fixture.format.Name+": "+payload)
			assertJankConversionLine(t, text, "34579.594372124", fixture.format.Name+": E|27599|")
			assertJankConversionLine(t, text, "34579.594373125", fixture.format.Name+": "+unknown)
			assertJankConversionLine(t, text, "34579.594374126", fixture.format.Name+": E|27599|")
		})
	}
}

func TestJankMarkerConversionProfilerPreservesPayloadAndUnknownInventory(t *testing.T) {
	payload := "B|27599|" + jankConversionMarkerName
	unknown := "B|27599|vendor_unknown_marker: token=9007199254740993, state=opaque"
	parts := [][]byte{protoVarint(1, 2)}
	for index, raw := range []string{payload, "E|27599|", unknown, "E|27599|"} {
		parts = append(parts, syntheticTracePluginFtraceEvent(
			jankConversionHeaderNS+uint64(index*1001), 100, 100, "marker-emitter", 1109,
			protoPayload(protoVarint(1, 0x1234), protoBytes(2, []byte(raw))),
		))
	}
	// A truly unknown protobuf event keeps its separate coverage-only lane.
	// Supporting this marker must not invent a schema or promote that record.
	parts = append(parts, syntheticTracePluginFtraceEvent(
		jankConversionHeaderNS+4004, 100, 100, "marker-emitter", 9999, protoVarint(1, 8)))
	capture := syntheticProfilerTraceFile(syntheticProfilerPluginData("ftrace-plugin", protoMessage(2, parts...)))
	result, text := convertJankMarkerCapture(t, "marker.htrace", capture)
	if result.EventsWritten != 4 || result.UnknownEventCount != 1 || result.BundlePath == "" {
		t.Fatalf("print markers or unknown-event coverage changed: result=%+v\n%s", result, text)
	}
	assertJankConversionLine(t, text, "34579.594371123", "print: "+payload)
	assertJankConversionLine(t, text, "34579.594372124", "print: E|27599|")
	assertJankConversionLine(t, text, "34579.594373125", "print: "+unknown)
	assertJankConversionLine(t, text, "34579.594374126", "print: E|27599|")
}

func TestJankMarkerConversionCallstackTablePreservesPayloadAndHeaderNanoseconds(t *testing.T) {
	unknown := "vendor_unknown_marker: token=9007199254740993, state=opaque"
	path := createTraceDBCallstackFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 27599, 'app')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'marker-emitter', 0, 0, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		fmt.Sprintf("INSERT INTO thread_state VALUES (1, %d, 10000, 2, 'Running')", jankConversionHeaderNS-1),
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie INT)",
		fmt.Sprintf("INSERT INTO callstack VALUES (1, %d, 1001, 1, '%s', '', NULL)", jankConversionHeaderNS, jankConversionMarkerName),
		fmt.Sprintf("INSERT INTO callstack VALUES (2, %d, 1001, 1, '%s', '', NULL)", jankConversionHeaderNS+2002, unknown),
	})
	for _, route := range []string{"database_export", "public_provider_conversion"} {
		t.Run(route, func(t *testing.T) {
			dir := t.TempDir()
			output := filepath.Join(dir, "marker-table.systrace")
			var coverage []TraceDBCoverage
			if route == "database_export" {
				result, err := exportTraceDBToSystrace(context.Background(), path, output)
				if err != nil {
					t.Fatal(err)
				}
				coverage = result.Coverage
			} else {
				if runtime.GOOS == "windows" {
					t.Skip("fake trace_streamer shell fixture uses /bin/sh")
				}
				// The external parser is fixture-controlled; all product provider,
				// sealed database, exporter and output-publication code is real.
				input := filepath.Join(dir, "marker.htrace")
				if err := os.WriteFile(input, []byte("modern profiler payload"), 0o600); err != nil {
					t.Fatal(err)
				}
				t.Setenv("TRACE_STREAMER_FIXTURE_DB", path)
				result, err := ConvertFile(context.Background(), Options{
					InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer,
					TraceStreamerPath: writeFakeTraceStreamer(t, dir, 0),
				})
				if err != nil {
					t.Fatal(err)
				}
				coverage = result.TraceDBCoverage
			}
			assertCoverageEmitted(t, coverage, "slice", "callstack", 4)
			body, err := os.ReadFile(output)
			if err != nil {
				t.Fatal(err)
			}
			text := string(body)
			assertJankConversionLine(t, text, "34579.594371123", "tracing_mark_write: B|27599|"+jankConversionMarkerName)
			assertJankConversionLine(t, text, "34579.594372124", "tracing_mark_write: E|27599|")
			assertJankConversionLine(t, text, "34579.594373125", "tracing_mark_write: B|27599|"+unknown)
			assertJankConversionLine(t, text, "34579.594374126", "tracing_mark_write: E|27599|")
		})
	}
}

func convertJankMarkerCapture(t *testing.T, name string, capture []byte) (Result, string) {
	t.Helper()
	dir := t.TempDir()
	input, output := filepath.Join(dir, name), filepath.Join(dir, "out.systrace")
	if err := os.WriteFile(input, capture, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineBuiltin})
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatal(err)
	}
	return result, string(body)
}

func assertJankConversionLine(t *testing.T, text, timestamp, body string) {
	t.Helper()
	want := timestamp + ": " + body
	count := 0
	for _, line := range strings.Split(text, "\n") {
		if !strings.HasPrefix(line, "#") && strings.HasSuffix(line, want) {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("converted source line count=%d want=1 for %q:\n%s", count, want, text)
	}
}
