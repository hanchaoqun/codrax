package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/tracewire"
)

func TestOwnedPerfProfilesAreClosedSourceClockTuples(t *testing.T) {
	tests := []struct {
		profile ownedTracePerfProfile
		source  tracewire.PerfSampleSource
		clock   tracewire.PerfSampleClock
	}{
		{ownedTracePerfSimpleperfText, tracewire.PerfSampleSourceSimpleperfReportSample, tracewire.PerfSampleClockRecord},
		{ownedTracePerfSimpleperfProto, tracewire.PerfSampleSourceSimpleperfReportProto, tracewire.PerfSampleClockSimpleperfRecord},
		{ownedTracePerfHiperfProto, tracewire.PerfSampleSourceHiperfProto, tracewire.PerfSampleClockMonotonicRaw},
		{ownedTracePerfRaw, tracewire.PerfSampleSourceRawPerfDataFallback, tracewire.PerfSampleClockPerfData},
	}
	for _, test := range tests {
		source, clock, ok := test.profile.sourceClock()
		if !ok || source != string(test.source) || clock != string(test.clock) {
			t.Fatalf("profile %q tuple=(%q,%q,%t), want=(%q,%q,true)", test.profile, source, clock, ok, test.source, test.clock)
		}
	}
	for _, profile := range []ownedTracePerfProfile{"", "simpleperf", "raw_perfdata_fallback"} {
		if source, clock, ok := profile.sourceClock(); ok || source != "" || clock != "" {
			t.Fatalf("open perf profile %q acquired tuple=(%q,%q,%t)", profile, source, clock, ok)
		}
	}
	protoProvider := perfProviderByName(perfProviderNameSimpleperfProto)
	if !protoProvider.Implemented || !perfProviderSupportsInput(protoProvider, perfInputSimpleperfReportProto) {
		t.Fatalf("implemented simpleperf proto route is absent from registry: %+v", protoProvider)
	}
}

func TestFourPerfWritersPublishOnlyValidatedProfileReceipts(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	tests := []struct {
		name    string
		profile ownedTracePerfProfile
		write   func(context.Context, string, *conversionFileLedger) error
	}{
		{
			name: "simpleperf_text", profile: ownedTracePerfSimpleperfText,
			write: func(ctx context.Context, path string, ledger *conversionFileLedger) error {
				return writeSimpleperfSamplesToPerfTraceWithLedger(ctx, []simpleperfSample{{
					Comm: "app", PID: 10, TID: 11, CPU: 1, Timestamp: 1, Period: 7, Event: "cycles",
					Leaf: simpleperfFrame{IP: "0x10", Symbol: "Hot", DSO: "lib.so"},
				}}, path, ledger)
			},
		},
		{
			name: "simpleperf_proto", profile: ownedTracePerfSimpleperfProto,
			write: func(ctx context.Context, path string, ledger *conversionFileLedger) error {
				return writeSimpleperfProtoDataToPerfTraceWithLedger(ctx, simpleperfProtoData{
					Files: map[uint32]simpleperfProtoFile{}, Threads: map[uint32]simpleperfProtoThread{11: {TID: 11, PID: 10, Name: "app"}},
					Samples: []simpleperfProtoSample{{TimeNS: 1_000_000_000, ThreadID: 11, EventCount: 7}},
				}, path, ledger)
			},
		},
		{
			name: "hiperf_proto", profile: ownedTracePerfHiperfProto,
			write: func(ctx context.Context, path string, ledger *conversionFileLedger) error {
				return writeHiperfProtoDataToPerfTraceWithLedger(ctx, hiperfProtoData{
					Files: map[uint32]hiperfProtoFile{}, Threads: map[uint32]hiperfProtoThread{11: {TID: 11, PID: 10, Name: "app"}},
					Samples: []hiperfProtoSample{{TimeNS: 1_000_000_000, TID: 11, EventCount: 7}},
				}, path, ledger)
			},
		},
		{
			name: "raw_perf", profile: ownedTracePerfRaw,
			write: func(ctx context.Context, path string, ledger *conversionFileLedger) error {
				capture := newRawPerfCaptureCompleteness()
				capture.SampleRecords = RawPerfRecordCensus{Physical: 1, Accepted: 1}
				admission := rawPerfTestQueryableAdmission(1)
				return finishRawPerfDataConversion(ctx, "input.perf.data", path, nil, ledger, timeZero(), rawPerfData{
					Samples:             []rawPerfSample{{PID: 10, TID: 11, CPU: 1, CPUValid: true, TimeNS: 1_000_000_000, IP: 0x10, Period: 7}},
					CaptureCompleteness: capture,
					SampleAdmission:     admission,
				}, nil)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			path := filepath.Join(t.TempDir(), test.name+".perftrace")
			if err := test.write(context.Background(), path, ledger); err != nil {
				t.Fatal(err)
			}
			published, ok := ledger.ownedTraceValidation(path)
			if !ok || published.receipt.kind != ownedTraceValidationPerf || published.receipt.perfProfile != test.profile ||
				!published.receipt.queryReady || published.receipt.rows != 1 || published.receipt.known != 1 ||
				published.receipt.unknown != 0 || published.receipt.unparsed != 0 {
				t.Fatalf("validated perf receipt drifted: %+v, ok=%t", published, ok)
			}
			wantSource, wantClock, _ := test.profile.sourceClock()
			if published.receipt.perfSource != wantSource || published.receipt.perfClock != wantClock {
				t.Fatalf("validated perf tuple drifted: %+v", published.receipt)
			}
			forged := published.receipt
			if test.profile == ownedTracePerfRaw {
				forged.perfProfile = ownedTracePerfSimpleperfText
			} else {
				forged.perfProfile = ownedTracePerfRaw
			}
			if err := validateOwnedTraceValidationReceipt(forged); err == nil {
				t.Fatalf("receipt profile relabel escaped: original=%+v forged=%+v", published.receipt, forged)
			}
			info, err := os.Stat(path)
			if err != nil || info.Size() != published.receipt.size {
				t.Fatalf("published size differs from receipt: info=%v receipt=%+v err=%v", info, published.receipt, err)
			}
			idx, err := tracequery.BuildIndex(context.Background(), path)
			if err != nil || len(idx.Events) != 1 || idx.Events[0].Type != tracequery.EventPerfSample {
				t.Fatalf("published perftrace is not queryable: idx=%+v err=%v", idx, err)
			}
		})
	}
}

func TestOwnedPerfWriterRejectsProfileAndCountDriftWithoutPublication(t *testing.T) {
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" && runtime.GOOS != "windows" {
		t.Skip("exact sealed-output publication is intentionally fail-closed on this platform")
	}
	sample := simpleperfSample{
		Comm: "app", PID: 10, TID: 11, CPU: 1, Timestamp: 1, Period: 7, Event: "cycles",
		Leaf: simpleperfFrame{IP: "0x10", Symbol: "Hot", DSO: "lib.so"},
	}
	tests := []struct {
		name     string
		profile  ownedTracePerfProfile
		expected int
		reason   string
	}{
		{name: "wire_profile_mismatch", profile: ownedTracePerfHiperfProto, expected: 1, reason: traceDBPostvalidationEventInvalid},
		{name: "row_count_mismatch", profile: ownedTracePerfSimpleperfText, expected: 2, reason: traceDBPostvalidationCountMismatch},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "rejected.perftrace")
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			_, err = writeValidatedOwnedPerfTraceWithLedger(
				context.Background(), ownedPerfTraceWriteSpec{Profile: test.profile, ExpectedRows: test.expected}, path, ledger,
				func(writer io.Writer) error {
					return writeSimpleperfPerfTrace(context.Background(), writer, []simpleperfSample{sample})
				},
			)
			reason, _, typed := ownedTraceOutputInvariantReason(err)
			if !typed || reason != test.reason || !ownedTraceOutputHardFailure(err) {
				t.Fatalf("drift was not a hard typed invariant: reason=%q err=%v", reason, err)
			}
			if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
				t.Fatalf("rejected generation became public: %v", statErr)
			}
			if _, ok := ledger.ownedTraceValidation(path); ok || len(ledger.created) != 0 {
				t.Fatalf("rejected generation entered ledger: %+v", ledger.created)
			}
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatal(readErr)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".codrax-perftrace-output-") {
					t.Fatalf("private perftrace residue survived: %s", entry.Name())
				}
			}
		})
	}
}

func TestOwnedPerfWriterIOFailureIsHardAndLeavesNoGeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "failed.perftrace")
	ledger, err := newConversionFileLedger()
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.cleanup()
	_, err = writeValidatedOwnedPerfTraceWithLedger(
		context.Background(), ownedPerfTraceWriteSpec{Profile: ownedTracePerfSimpleperfText, ExpectedRows: 1}, path, ledger,
		func(io.Writer) error {
			return &os.PathError{Op: "write", Path: "callback", Err: syscall.ENOSPC}
		},
	)
	var publication *ownedTracePublicationError
	if !ownedTraceOutputHardFailure(err) || !errors.As(err, &publication) || publication == nil || !errors.Is(err, syscall.ENOSPC) {
		t.Fatalf("writer I/O failure lost hard typed identity: %T %v", err, err)
	}
	if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
		t.Fatalf("failed writer became public: %v", statErr)
	}
	if len(ledger.created) != 0 {
		t.Fatalf("failed writer entered ledger: %+v", ledger.created)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".codrax-perftrace-output-") {
			t.Fatalf("failed writer retained private staging: %s", entry.Name())
		}
	}
}

func TestFourPerfWritersRejectZeroSamplesBeforePublication(t *testing.T) {
	tests := []struct {
		name  string
		write func(context.Context, string, *conversionFileLedger) error
	}{
		{"simpleperf_text", func(ctx context.Context, path string, ledger *conversionFileLedger) error {
			return writeSimpleperfSamplesToPerfTraceWithLedger(ctx, nil, path, ledger)
		}},
		{"simpleperf_proto", func(ctx context.Context, path string, ledger *conversionFileLedger) error {
			return writeSimpleperfProtoDataToPerfTraceWithLedger(ctx, simpleperfProtoData{}, path, ledger)
		}},
		{"hiperf_proto", func(ctx context.Context, path string, ledger *conversionFileLedger) error {
			return writeHiperfProtoDataToPerfTraceWithLedger(ctx, hiperfProtoData{}, path, ledger)
		}},
		{"raw_perf", func(ctx context.Context, path string, ledger *conversionFileLedger) error {
			return finishRawPerfDataConversion(ctx, "input.perf.data", path, nil, ledger, timeZero(), rawPerfData{}, nil)
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "zero.perftrace")
			ledger, err := newConversionFileLedger()
			if err != nil {
				t.Fatal(err)
			}
			defer ledger.cleanup()
			if err := test.write(context.Background(), path, ledger); err == nil {
				t.Fatal("zero-sample writer succeeded")
			}
			if _, err := os.Lstat(path); !os.IsNotExist(err) {
				t.Fatalf("zero-sample writer became public: %v", err)
			}
			if len(ledger.created) != 0 {
				t.Fatalf("zero-sample writer entered ledger: %+v", ledger.created)
			}
			entries, err := os.ReadDir(dir)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".codrax-perftrace-output-") {
					t.Fatalf("zero-sample writer retained staging: %s", entry.Name())
				}
			}
		})
	}
}

func TestPerfTextAndRawWritersSortTimeStably(t *testing.T) {
	textSamples := []simpleperfSample{
		{Comm: "app", PID: 10, TID: 11, CPU: 1, Timestamp: 2, Period: 1, Event: "cycles", Leaf: simpleperfFrame{IP: "0x20", Symbol: "late"}},
		{Comm: "app", PID: 10, TID: 11, CPU: 1, Timestamp: 1, Period: 1, Event: "cycles", Leaf: simpleperfFrame{IP: "0x10", Symbol: "equal-first"}},
		{Comm: "app", PID: 10, TID: 11, CPU: 1, Timestamp: 1, Period: 1, Event: "cycles", Leaf: simpleperfFrame{IP: "0x11", Symbol: "equal-second"}},
	}
	var textWire bytes.Buffer
	if err := writeSimpleperfPerfTrace(context.Background(), &textWire, textSamples); err != nil {
		t.Fatal(err)
	}
	assertPerfWireOrder(t, textWire.Bytes(), []string{"0x10", "0x11", "0x20"})
	if textSamples[0].Timestamp != 2 || textSamples[1].Timestamp != 1 || textSamples[2].Timestamp != 1 {
		t.Fatalf("simpleperf writer mutated caller sample order: %+v", textSamples)
	}

	raw := rawPerfData{Samples: []rawPerfSample{
		{PID: 10, TID: 11, CPU: 1, CPUValid: true, TimeNS: 2_000_000_000, IP: 0x20, Period: 1},
		{PID: 10, TID: 11, CPU: 1, CPUValid: true, TimeNS: 1_000_000_000, IP: 0x10, Period: 1},
		{PID: 10, TID: 11, CPU: 1, CPUValid: true, TimeNS: 1_000_000_000, IP: 0x11, Period: 1},
	}}
	var rawWire bytes.Buffer
	if err := writeRawPerfDataPerfTrace(context.Background(), &rawWire, raw); err != nil {
		t.Fatal(err)
	}
	assertPerfWireOrder(t, rawWire.Bytes(), []string{"0x10", "0x11", "0x20"})
	if raw.Samples[0].IP != 0x20 || raw.Samples[1].IP != 0x10 || raw.Samples[2].IP != 0x11 {
		t.Fatalf("raw writer mutated caller sample order: %+v", raw.Samples)
	}
}

func assertPerfWireOrder(t *testing.T, body []byte, wantIP []string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ordered.perftrace")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		t.Fatal(err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	if idx.ClockRegressions != 0 || len(idx.Events) != len(wantIP) {
		t.Fatalf("ordered wire accounting drifted: %+v", idx)
	}
	for index, want := range wantIP {
		if idx.Events[index].PerfFields == nil || idx.Events[index].PerfFields.IP != want {
			t.Fatalf("event %d IP=%+v, want %s", index, idx.Events[index].PerfFields, want)
		}
	}
}

func TestPerfWriterAndProviderHardBoundaryStructure(t *testing.T) {
	writers := []struct {
		file, function, profile string
	}{
		{"simpleperf_text.go", "writeSimpleperfSamplesToPerfTraceWithLedger", "ownedTracePerfSimpleperfText"},
		{"simpleperf_proto.go", "writeSimpleperfProtoDataToPerfTraceWithLedger", "ownedTracePerfSimpleperfProto"},
		{"hiperf_proto.go", "writeHiperfProtoDataToPerfTraceWithLedger", "ownedTracePerfHiperfProto"},
	}
	for _, writer := range writers {
		body := sourceGenerationFunctionBody(t, writer.file, writer.function)
		for _, forbidden := range []string{"openOwnedConversionFile(", "finishOwnedConversionFile(", "os.Lstat("} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s still uses public path writer %q:\n%s", writer.function, forbidden, body)
			}
		}
		for _, required := range []string{"writeValidatedOwnedPerfTraceWithLedger(", writer.profile} {
			if !strings.Contains(body, required) {
				t.Fatalf("%s lacks validated profile %q:\n%s", writer.function, required, body)
			}
		}
	}
	rawBody := sourceGenerationFunctionBody(t, "raw_perfdata.go", "finishRawPerfDataConversionWithPolicy")
	if !strings.Contains(rawBody, "writeValidatedOwnedPerfTraceWithLedger(") || !strings.Contains(rawBody, "ownedTracePerfRaw") ||
		strings.Contains(rawBody, "openOwnedConversionFile(") || strings.Contains(rawBody, "finishOwnedConversionFile(") {
		t.Fatalf("raw perf writer bypassed validated output throat:\n%s", rawBody)
	}
	helperBody := sourceGenerationFunctionBody(t, "owned_perftrace_output.go", "writeValidatedOwnedPerfTraceWithLedger")
	validated := strings.Index(helperBody, "validatedReceipt, _, err := validateOwnedTraceOutput(")
	published := strings.Index(helperBody, "publishValidatedOwnedTraceOutputNoReplace(ctx, target, sealedOutput, validatedReceipt, ledger)")
	readback := strings.Index(helperBody, "ledger.ownedTraceValidation(target.finalBindingPath)")
	if validated < 0 || published <= validated || readback <= published {
		t.Fatalf("perf output throat can relabel or skip validated receipt/public generation readback:\n%s", helperBody)
	}

	providers := []struct {
		file, function, fallback string
	}{
		{"simpleperf_text.go", "maybeConvertSimpleperfPerfData", "maybeRawPerfFallbackForSimpleperf"},
		{"simpleperf_proto.go", "maybeConvertSimpleperfProtoWithDecision", "official_proto_unreadable"},
		{"simpleperf_proto.go", "maybeConvertSimpleperfProtoFromInputWithDecision", "official_proto_unreadable"},
		{"hiperf_proto.go", "maybeConvertHiperfPerfDataFromInput", "maybeRawPerfFallbackFromStandaloneInput"},
	}
	for _, provider := range providers {
		body := sourceGenerationFunctionBody(t, provider.file, provider.function)
		hard := strings.Index(body, "ownedTraceOutputHardFailure(")
		fallback := strings.LastIndex(body, provider.fallback)
		if hard < 0 || fallback < 0 || hard > fallback {
			t.Fatalf("%s can downgrade/fallback before the hard output boundary:\n%s", provider.function, body)
		}
	}
	for _, function := range []string{"maybeConvertRawPerfData", "maybeConvertRawPerfDataFromInput", "maybeConvertRawPerfDataFromStandaloneInput"} {
		body := sourceGenerationFunctionBody(t, "raw_perfdata.go", function)
		if !strings.Contains(body, "ownedTraceOutputHardFailure(") {
			t.Fatalf("%s can soften validated output failure:\n%s", function, body)
		}
	}

	var invariant *ownedTraceOutputInvariantError
	var publication *ownedTracePublicationError
	if !errors.As(newOwnedTracePublicationError("test", "out.perftrace", errors.New("boom")), &publication) || publication == nil {
		t.Fatal("typed publication failure lost its error identity")
	}
	if errors.As(newOwnedTracePublicationError("test", "out.perftrace", errors.New("boom")), &invariant) {
		t.Fatal("publication failure impersonated a semantic invariant")
	}

	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve perf receipt structure test path")
	}
	entries, err := os.ReadDir(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") || name == "trace_validation.go" {
			continue
		}
		body, err := os.ReadFile(filepath.Join(filepath.Dir(current), name))
		if err != nil {
			t.Fatal(err)
		}
		fset := token.NewFileSet()
		parsed, err := parser.ParseFile(fset, name, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(parsed, func(node ast.Node) bool {
			assignment, ok := node.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, expression := range assignment.Lhs {
				selector, ok := expression.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				switch selector.Sel.Name {
				case "perfProfile", "perfSource", "perfClock":
					t.Fatalf("production source %s can relabel validated perf receipt field %s", name, selector.Sel.Name)
				}
			}
			return true
		})
	}
}

// timeZero keeps progress timing deterministic without teaching this test
// about the current wall clock.
func timeZero() time.Time { return time.Time{} }
