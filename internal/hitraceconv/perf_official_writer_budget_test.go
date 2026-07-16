package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestPerfOfficialWritersPreserveCanonicalBodyBytes(t *testing.T) {
	t.Run("simpleperf text", func(t *testing.T) {
		var out bytes.Buffer
		sample := simpleperfSample{
			Comm: "Render", PID: 1234, TID: 5678, CPU: 5, Timestamp: 1.25, Period: 10000, Event: "cpu-cycles",
			Leaf: simpleperfFrame{IP: "0x1234", Symbol: "Foo::bar", DSO: "libfoo.so"},
			CallFrames: []simpleperfFrame{
				{IP: "0x2000", Symbol: "A", DSO: "libfoo.so"},
				{IP: "0x3000", Symbol: "main", DSO: "libfoo.so"},
			},
		}
		if err := writeSimpleperfPerfTrace(context.Background(), &out, []simpleperfSample{sample}); err != nil {
			t.Fatal(err)
		}
		want := `perf_sample: cpu=5 cpu_known=true pid=1234 tid=5678 thread_comm="Render" sample_weight=10000 event="cpu-cycles" symbol="Foo::bar" dso="libfoo.so" ip="0x1234" callchain="main@libfoo.so;A@libfoo.so;Foo::bar@libfoo.so" source=simpleperf_report_sample symbolization_status=symbolized clock=record clock_confidence=assumed callchain_status=symbolized`
		assertSinglePerfBody(t, out.String(), want)
	})

	t.Run("simpleperf proto", func(t *testing.T) {
		var out bytes.Buffer
		data := canonicalSimpleperfProtoWriterData(99)
		if err := writeSimpleperfProtoPerfTrace(context.Background(), &out, data); err != nil {
			t.Fatal(err)
		}
		want := `perf_sample: cpu=-1 cpu_known=false pid=1234 tid=5678 thread_comm="Render" sample_weight=99 event="cpu-cycles" symbol="Foo::bar" dso="libfoo.so" ip="0x1234" callchain="main@libfoo.so;Foo::bar@libfoo.so" source=simpleperf_report_proto sample_kind=on_cpu symbolization_status=symbolized clock=simpleperf_record clock_confidence=assumed callchain_status=symbolized`
		assertSinglePerfBody(t, out.String(), want)
	})

	t.Run("hiperf proto", func(t *testing.T) {
		var out bytes.Buffer
		data := canonicalHiperfWriterData(99)
		if err := writeHiperfPerfTrace(context.Background(), &out, data); err != nil {
			t.Fatal(err)
		}
		want := `perf_sample: cpu=-1 cpu_known=false pid=1234 tid=5678 thread_comm="Render" sample_weight=99 event="cpu-cycles" symbol="doWork" dso="libfoo.so" ip="0x1234" callchain="main@libfoo.so;doWork@libfoo.so" source=hiperf_proto symbolization_status=symbolized clock=monotonic_raw clock_confidence=assumed callchain_status=symbolized`
		assertSinglePerfBody(t, out.String(), want)
	})
}

func TestPerfOfficialWriterWeightDomain(t *testing.T) {
	writers := []struct {
		name  string
		write func(uint64) (string, error)
	}{
		{
			name: "simpleperf text",
			write: func(weight uint64) (string, error) {
				var out bytes.Buffer
				err := writeSimpleperfPerfTrace(context.Background(), &out, []simpleperfSample{{
					Comm: "Render", PID: 1234, TID: 5678, CPU: 5, Timestamp: 1.25, Period: weight,
					Event: "cpu-cycles", Leaf: simpleperfFrame{IP: "0x1", Symbol: "Hot", DSO: "lib.so"},
				}})
				return out.String(), err
			},
		},
		{
			name: "simpleperf proto",
			write: func(weight uint64) (string, error) {
				var out bytes.Buffer
				err := writeSimpleperfProtoPerfTrace(context.Background(), &out, canonicalSimpleperfProtoWriterData(weight))
				return out.String(), err
			},
		},
		{
			name: "hiperf proto",
			write: func(weight uint64) (string, error) {
				var out bytes.Buffer
				err := writeHiperfPerfTrace(context.Background(), &out, canonicalHiperfWriterData(weight))
				return out.String(), err
			},
		},
	}
	for _, writer := range writers {
		t.Run(writer.name, func(t *testing.T) {
			for _, tc := range []struct {
				weight uint64
				want   int64
			}{{0, 1}, {1, 1}, {math.MaxInt64, math.MaxInt64}} {
				wire, err := writer.write(tc.weight)
				if err != nil {
					t.Fatalf("weight %d: %v", tc.weight, err)
				}
				if !strings.Contains(wire, "sample_weight="+strconv.FormatInt(tc.want, 10)) {
					t.Fatalf("weight %d did not normalize to %d:\n%s", tc.weight, tc.want, wire)
				}
			}
			for _, weight := range []uint64{math.MaxInt64 + 1, math.MaxUint64} {
				_, err := writer.write(weight)
				var typed *tracewire.PerfWireBuildError
				if !errors.As(err, &typed) || typed.Field != "sample_weight" || typed.Reason != "out_of_range" || typed.Actual != weight {
					t.Fatalf("weight %d error=%T %v typed=%+v", weight, err, err, typed)
				}
			}
		})
	}
}

func TestPerfOfficialWriterBudgetFailuresAreTyped(t *testing.T) {
	overlong := strings.Repeat("x", tracewire.MaxPerfMetadataBytes+1)
	overlongCallchain := strings.Repeat("c", tracewire.MaxPerfCallchainBytes+1)
	tests := []struct {
		name  string
		field string
		write func() error
	}{
		{
			name: "simpleperf metadata", field: "symbol",
			write: func() error {
				var out bytes.Buffer
				return writeSimpleperfPerfTrace(context.Background(), &out, []simpleperfSample{{
					Comm: "Render", PID: 1, TID: 1, CPU: 0, Timestamp: 1, Period: 1,
					Event: "cycles", Leaf: simpleperfFrame{Symbol: overlong, DSO: "unknown"},
				}})
			},
		},
		{
			name: "simpleperf callchain", field: "callchain",
			write: func() error {
				var out bytes.Buffer
				return writeSimpleperfPerfTrace(context.Background(), &out, []simpleperfSample{{
					Comm: "Render", PID: 1, TID: 1, CPU: 0, Timestamp: 1, Period: 1,
					Event: "cycles", Leaf: simpleperfFrame{Symbol: "leaf", DSO: "unknown"},
					CallFrames: []simpleperfFrame{{Symbol: overlongCallchain, DSO: "unknown"}},
				}})
			},
		},
		{
			name: "simpleperf proto metadata", field: "symbol",
			write: func() error {
				var out bytes.Buffer
				data := canonicalSimpleperfProtoWriterData(1)
				data.Files[1] = simpleperfProtoFile{ID: 1, Path: "unknown", Symbols: []string{overlong, "main"}}
				return writeSimpleperfProtoPerfTrace(context.Background(), &out, data)
			},
		},
		{
			name: "hiperf metadata", field: "symbol",
			write: func() error {
				var out bytes.Buffer
				data := canonicalHiperfWriterData(1)
				data.Files[1] = hiperfProtoFile{ID: 1, Path: "unknown", FunctionNames: []string{overlong, "main"}}
				return writeHiperfPerfTrace(context.Background(), &out, data)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.write()
			var typed *tracewire.PerfWireBuildError
			if !errors.As(err, &typed) || typed.Field != tc.field {
				t.Fatalf("error=%T %v typed=%+v want field=%s", err, err, typed, tc.field)
			}
		})
	}
}

func TestPerfOfficialWritersCheckCancellationInsideCallchain(t *testing.T) {
	writers := []struct {
		name  string
		write func(context.Context, *bytes.Buffer) error
	}{
		{
			name: "simpleperf text",
			write: func(ctx context.Context, out *bytes.Buffer) error {
				return writeSimpleperfPerfTrace(ctx, out, []simpleperfSample{{
					Comm: "Render", PID: 1, TID: 1, CPU: 0, Timestamp: 1, Period: 1, Event: "cycles",
					Leaf: simpleperfFrame{Symbol: "leaf"}, CallFrames: []simpleperfFrame{{Symbol: "root"}},
				}})
			},
		},
		{
			name: "simpleperf proto",
			write: func(ctx context.Context, out *bytes.Buffer) error {
				return writeSimpleperfProtoPerfTrace(ctx, out, canonicalSimpleperfProtoWriterData(1))
			},
		},
		{
			name: "hiperf proto",
			write: func(ctx context.Context, out *bytes.Buffer) error {
				return writeHiperfPerfTrace(ctx, out, canonicalHiperfWriterData(1))
			},
		},
	}
	for _, tc := range writers {
		t.Run(tc.name, func(t *testing.T) {
			ctx := &cancelAfterErrCalls{Context: context.Background(), cancelAt: 2}
			var out bytes.Buffer
			if err := tc.write(ctx, &out); !errors.Is(err, context.Canceled) {
				t.Fatalf("error=%v, want context.Canceled", err)
			}
			if strings.Contains(out.String(), "perf_sample:") {
				t.Fatalf("cancelled callchain published a perf row:\n%s", out.String())
			}
		})
	}
}

func TestPerfOfficialOwnedFilesRollbackOnLaterWeightFailure(t *testing.T) {
	tests := []struct {
		name  string
		write func(string, *conversionFileLedger) error
	}{
		{
			name: "simpleperf text",
			write: func(path string, ledger *conversionFileLedger) error {
				base := simpleperfSample{Comm: "Render", PID: 1, TID: 1, CPU: 0, Timestamp: 1, Period: 1, Event: "cycles", Leaf: simpleperfFrame{Symbol: "Hot", DSO: "unknown"}}
				bad := base
				bad.Timestamp = 2
				bad.Period = math.MaxInt64 + 1
				return writeSimpleperfSamplesToPerfTraceWithLedger(context.Background(), []simpleperfSample{base, bad}, path, ledger)
			},
		},
		{
			name: "simpleperf proto",
			write: func(path string, ledger *conversionFileLedger) error {
				data := canonicalSimpleperfProtoWriterData(1)
				bad := data.Samples[0]
				bad.TimeNS++
				bad.EventCount = math.MaxInt64 + 1
				data.Samples = append(data.Samples, bad)
				return writeSimpleperfProtoDataToPerfTraceWithLedger(context.Background(), data, path, ledger)
			},
		},
		{
			name: "hiperf proto",
			write: func(path string, ledger *conversionFileLedger) error {
				data := canonicalHiperfWriterData(1)
				bad := data.Samples[0]
				bad.TimeNS++
				bad.EventCount = math.MaxInt64 + 1
				data.Samples = append(data.Samples, bad)
				return writeHiperfProtoDataToPerfTraceWithLedger(context.Background(), data, path, ledger)
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "failed.perftrace")
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			err = tc.write(path, ledger)
			var typed *tracewire.PerfWireBuildError
			if !errors.As(err, &typed) || typed.Field != "sample_weight" {
				t.Fatalf("error=%T %v typed=%+v", err, err, typed)
			}
			if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
				t.Fatalf("failed writer left a partial public file: %v", statErr)
			}
			if cleanupErr := ledger.cleanup(); cleanupErr != nil {
				t.Fatalf("ledger cleanup: %v", cleanupErr)
			}
		})
	}
}

func canonicalSimpleperfProtoWriterData(weight uint64) simpleperfProtoData {
	return simpleperfProtoData{
		Files: map[uint32]simpleperfProtoFile{1: {
			ID: 1, Path: "libfoo.so", Symbols: []string{"Foo::bar", "main"},
		}},
		Threads: map[uint32]simpleperfProtoThread{5678: {TID: 5678, PID: 1234, Name: "Render"}},
		Meta:    simpleperfProtoMeta{EventTypes: []string{"cpu-cycles"}},
		Samples: []simpleperfProtoSample{{
			TimeNS: 1_250_000_000, ThreadID: 5678, EventCount: weight, EventTypeID: 0, EventTypeSet: true,
			Frames: []simpleperfProtoFrame{
				{VaddrInFile: 0x1234, FileID: 1, FileSet: true, SymbolID: 0, SymbolSet: true},
				{VaddrInFile: 0x2000, FileID: 1, FileSet: true, SymbolID: 1, SymbolSet: true},
			},
		}},
	}
}

func canonicalHiperfWriterData(weight uint64) hiperfProtoData {
	return hiperfProtoData{
		Files: map[uint32]hiperfProtoFile{1: {
			ID: 1, Path: "libfoo.so", FunctionNames: []string{"doWork", "main"},
		}},
		Threads:     map[uint32]hiperfProtoThread{5678: {TID: 5678, PID: 1234, Name: "Render"}},
		ConfigNames: []string{"cpu-cycles"},
		Samples: []hiperfProtoSample{{
			TimeNS: 1_250_000_000, TID: 5678, EventCount: weight, ConfigNameID: 0, ConfigSet: true,
			Frames: []hiperfProtoFrame{
				{SymbolsVaddr: 0x1234, SymbolsFileID: 1, SymbolsFileSet: true, FunctionNameID: 0, FunctionSet: true},
				{SymbolsVaddr: 0x2000, SymbolsFileID: 1, SymbolsFileSet: true, FunctionNameID: 1, FunctionSet: true},
			},
		}},
	}
}

func assertSinglePerfBody(t *testing.T, wire, want string) {
	t.Helper()
	var got string
	for _, line := range strings.Split(wire, "\n") {
		if index := strings.Index(line, "perf_sample:"); index >= 0 {
			if got != "" {
				t.Fatalf("writer emitted multiple perf rows:\n%s", wire)
			}
			got = line[index:]
		}
	}
	if got != want {
		t.Fatalf("perf body drift:\n got %s\nwant %s", got, want)
	}
}

type cancelAfterErrCalls struct {
	context.Context
	calls    int
	cancelAt int
}

func (c *cancelAfterErrCalls) Err() error {
	c.calls++
	if c.calls >= c.cancelAt {
		return context.Canceled
	}
	return nil
}
