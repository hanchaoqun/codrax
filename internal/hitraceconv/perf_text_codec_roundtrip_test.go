package hitraceconv

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

const perfCodecHostileSymbol = `Hot" pid=888 tid=999 cpu=7 cpu_known=true sample_weight=999 source=evil tail`
const perfCodecWindowsDSO = `C:\Program Files\鸿蒙\libfoo.dll`

type perfCodecRoundTripWant struct {
	PID      int
	TID      int
	CPU      int
	CPUKnown bool
	Weight   int64
	Source   string
	Symbol   string
	DSO      string
}

func assertPerfCodecWriterRoundTrip(t *testing.T, wire string, want perfCodecRoundTripWant) tracequery.Event {
	t.Helper()
	var perfLine string
	for _, line := range strings.Split(wire, "\n") {
		if strings.Contains(line, "perf_sample:") {
			if perfLine != "" {
				t.Fatalf("writer emitted more than one perf row:\n%s", wire)
			}
			perfLine = line
		}
	}
	if perfLine == "" {
		t.Fatalf("writer emitted no perf row:\n%s", wire)
	}
	body := strings.TrimSpace(strings.SplitN(perfLine, "perf_sample:", 2)[1])
	fields, wireErr := tracewire.ParsePerfKV(body)
	if wireErr != nil {
		t.Fatalf("writer produced unreadable perf KV: %v\n%s", wireErr, perfLine)
	}
	counts := map[string]int{}
	for _, field := range fields {
		counts[field.Key]++
	}
	for _, key := range []string{"pid", "tid", "cpu", "cpu_known", "sample_weight", "source"} {
		if counts[key] != 1 {
			t.Fatalf("hard field %s occurrences=%d fields=%+v", key, counts[key], fields)
		}
	}

	path := filepath.Join(t.TempDir(), "writer.perftrace")
	if err := os.WriteFile(path, []byte(wire), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	var samples []tracequery.Event
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventPerfSample {
			samples = append(samples, ev)
		}
	}
	if len(samples) != 1 || samples[0].PerfFields == nil {
		t.Fatalf("reader did not recover exactly one perf row: events=%+v", idx.Events)
	}
	ev := samples[0]
	pf := ev.PerfFields
	if pf.PID != want.PID || pf.TID != want.TID || ev.TGID != want.PID || ev.PID != want.TID {
		t.Fatalf("thread identity round-trip drift: %+v", ev)
	}
	if ev.CPU != want.CPU || pf.CPUKnown == nil || *pf.CPUKnown != want.CPUKnown {
		t.Fatalf("CPU identity round-trip drift: %+v", ev)
	}
	if pf.Period != want.Weight || pf.Source != want.Source || pf.Symbol != want.Symbol || pf.DSO != want.DSO {
		t.Fatalf("perf payload round-trip drift: got=%+v want=%+v", pf, want)
	}
	if pf.PerfTextIntegrity != "" {
		t.Fatalf("canonical writer was degraded by reader: %s", pf.PerfTextIntegrity)
	}
	return ev
}

func TestPerfTextCodecFiveWritersRoundTripHostileMetadata(t *testing.T) {
	t.Run("simpleperf text", func(t *testing.T) {
		var out bytes.Buffer
		err := writeSimpleperfPerfTrace(context.Background(), &out, []simpleperfSample{{
			Comm: "Render", PID: 1234, TID: 5678, CPU: 5, Timestamp: 1.25, Period: 11, Event: "cpu-cycles",
			Leaf: simpleperfFrame{IP: "0x1234", Symbol: perfCodecHostileSymbol, DSO: perfCodecWindowsDSO},
		}})
		if err != nil {
			t.Fatal(err)
		}
		assertPerfCodecWriterRoundTrip(t, out.String(), perfCodecRoundTripWant{
			PID: 1234, TID: 5678, CPU: 5, CPUKnown: true, Weight: 11, Source: "simpleperf_report_sample", Symbol: perfCodecHostileSymbol, DSO: perfCodecWindowsDSO,
		})
	})

	t.Run("simpleperf proto", func(t *testing.T) {
		var out bytes.Buffer
		data := simpleperfProtoData{
			Files:   map[uint32]simpleperfProtoFile{1: {ID: 1, Path: perfCodecWindowsDSO, Symbols: []string{perfCodecHostileSymbol}}},
			Threads: map[uint32]simpleperfProtoThread{5678: {TID: 5678, PID: 1234, Name: "Render"}},
			Meta:    simpleperfProtoMeta{EventTypes: []string{"cpu-cycles"}},
			Samples: []simpleperfProtoSample{{
				TimeNS: 1_250_000_000, ThreadID: 5678, EventCount: 11, EventTypeID: 0, EventTypeSet: true,
				Frames: []simpleperfProtoFrame{{VaddrInFile: 0x1234, FileID: 1, FileSet: true, SymbolID: 0, SymbolSet: true}},
			}},
		}
		if err := writeSimpleperfProtoPerfTrace(context.Background(), &out, data); err != nil {
			t.Fatal(err)
		}
		assertPerfCodecWriterRoundTrip(t, out.String(), perfCodecRoundTripWant{
			PID: 1234, TID: 5678, CPU: -1, CPUKnown: false, Weight: 11, Source: "simpleperf_report_proto", Symbol: perfCodecHostileSymbol, DSO: perfCodecWindowsDSO,
		})
	})

	t.Run("hiperf proto", func(t *testing.T) {
		var out bytes.Buffer
		data := hiperfProtoData{
			Files:       map[uint32]hiperfProtoFile{1: {ID: 1, Path: perfCodecWindowsDSO, FunctionNames: []string{perfCodecHostileSymbol}}},
			Threads:     map[uint32]hiperfProtoThread{5678: {TID: 5678, PID: 1234, Name: "Render"}},
			ConfigNames: []string{"cpu-cycles"},
			Samples: []hiperfProtoSample{{
				TimeNS: 1_250_000_000, TID: 5678, EventCount: 11, ConfigNameID: 0, ConfigSet: true,
				Frames: []hiperfProtoFrame{{SymbolsVaddr: 0x1234, SymbolsFileID: 1, SymbolsFileSet: true, FunctionNameID: 0, FunctionSet: true}},
			}},
		}
		if err := writeHiperfPerfTrace(context.Background(), &out, data); err != nil {
			t.Fatal(err)
		}
		assertPerfCodecWriterRoundTrip(t, out.String(), perfCodecRoundTripWant{
			PID: 1234, TID: 5678, CPU: -1, CPUKnown: false, Weight: 11, Source: "hiperf_proto", Symbol: perfCodecHostileSymbol, DSO: perfCodecWindowsDSO,
		})
	})

	t.Run("raw perfdata", func(t *testing.T) {
		var out bytes.Buffer
		data := rawPerfData{
			Features: rawPerfFeatures{SymbolFiles: []rawPerfSymbolFile{{
				Path: perfCodecWindowsDSO, SymbolType: rawPerfSymbolFileKernel,
				Symbols: []rawPerfSymbol{{Vaddr: 0x1234, Len: 16, Name: perfCodecHostileSymbol}},
			}}},
			Samples: []rawPerfSample{{
				IP: 0x1234, PID: 1234, TID: 5678, TimeNS: 1_250_000_000, CPU: 5, CPUValid: true,
				Period: 11, EventName: "cpu-cycles", Comm: "Render",
			}},
			Caveats: []string{perfCodecHostileSymbol},
		}
		if err := writeRawPerfDataPerfTrace(context.Background(), &out, data); err != nil {
			t.Fatal(err)
		}
		assertPerfCodecWriterRoundTrip(t, out.String(), perfCodecRoundTripWant{
			PID: 1234, TID: 5678, CPU: 5, CPUKnown: true, Weight: 11, Source: "raw_perfdata_fallback", Symbol: perfCodecHostileSymbol, DSO: perfCodecWindowsDSO,
		})
	})

	t.Run("trace streamer SQL resolved", func(t *testing.T) {
		index := traceDBPerfB3AIdentity()
		coverage, body := exportTraceDBPerfB3AFixture(t, []string{
			"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
			"INSERT INTO perf_thread VALUES (50, 500, '" + perfCodecHostileSymbol + "')",
			"CREATE TABLE perf_files (file_id, serial_id, symbol, path)",
			"INSERT INTO perf_files VALUES (7, -1, NULL, '" + perfCodecWindowsDSO + "')",
			"CREATE TABLE perf_callchain (id, callchain_id, depth, name, ip, file_id, symbol_id)",
			"INSERT INTO perf_callchain VALUES (2, 10, 0, '" + perfCodecHostileSymbol + "', 4660, 7, -1)",
			"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
			"INSERT INTO perf_sample VALUES (1, 10, 1000, 50, 11, 2, 'Running')",
		}, index, traceDBLifecycleIndex{}, true, map[int64][]traceDBRunningInterval{
			1: {{Start: 900, End: 1100, CPU: 2, PrefixMaxEnd: 1100}},
		})
		if got := traceDBPerfCoverage(t, coverage, "perf_sample"); got.RowsRead != 1 || got.RowsEmitted != 1 {
			t.Fatalf("SQL perf coverage drift: %+v\n%s", got, body)
		}
		assertPerfCodecWriterRoundTrip(t, body+"\n", perfCodecRoundTripWant{
			PID: 500, TID: 50, CPU: 2, CPUKnown: true, Weight: 11, Source: "trace_streamer_db", Symbol: perfCodecHostileSymbol, DSO: perfCodecWindowsDSO,
		})
	})
}

func TestPerfTextCodecStreamerAnonymousTailCannotReopenHardKeys(t *testing.T) {
	index := newTraceDBThreadIndex(0, true)
	coverage, body := exportTraceDBPerfB3AFixture(t, []string{
		"CREATE TABLE perf_thread (thread_id, process_id, thread_name)",
		"INSERT INTO perf_thread VALUES (44, 400, '" + perfCodecHostileSymbol + "')",
		"CREATE TABLE perf_sample (id, callchain_id, timestamp_trace, thread_id, event_count, cpu_id, thread_state)",
		"INSERT INTO perf_sample VALUES (1, -1, 1000, 44, 11, 3, '-')",
	}, index, traceDBLifecycleIndex{}, true, nil)
	if got := traceDBPerfCoverage(t, coverage, "perf_sample"); got.RowsRead != 1 || got.RowsEmitted != 1 {
		t.Fatalf("anonymous SQL perf coverage drift: %+v\n%s", got, body)
	}
	path := filepath.Join(t.TempDir(), "anonymous.perftrace")
	if err := os.WriteFile(path, []byte(body+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if len(idx.Events) != 1 || idx.Events[0].Type != tracequery.EventPerfSample || idx.Events[0].PerfFields == nil {
		t.Fatalf("anonymous SQL perf row missing: %+v", idx.Events)
	}
	ev, pf := idx.Events[0], idx.Events[0].PerfFields
	if ev.PID != 0 || ev.TGID != 0 || pf.PID != 0 || pf.TID != 0 || pf.ThreadIdentityKnown == nil || *pf.ThreadIdentityKnown ||
		pf.SourcePID != 400 || pf.SourceTID != 44 || pf.SourceComm != perfCodecHostileSymbol || ev.CPU != 3 || pf.Period != 11 || pf.Source != "trace_streamer_db" {
		t.Fatalf("anonymous tail metadata reopened authority: event=%+v perf=%+v\n%s", ev, pf, body)
	}
	if pf.PerfTextIntegrity != "" {
		t.Fatalf("canonical anonymous writer was degraded: %s", pf.PerfTextIntegrity)
	}
}
