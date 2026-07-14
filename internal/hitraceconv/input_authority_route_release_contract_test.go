package hitraceconv

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type sourceGenerationConversionOutcome struct {
	result Result
	err    error
}

const sourceGenerationFixtureTimeout = 30 * time.Second

func TestReleaseConversionOptionValidationDoesNotInspectInput(t *testing.T) {
	opts := Options{
		InputPath:         filepath.Join(t.TempDir(), "missing-or-blocking-input"),
		TraceEngine:       traceEngineBuiltin,
		TraceStreamerPath: "configured-but-route-specific",
		KeepTraceDB:       true,
	}
	if err := ValidateOptions(opts); err == nil || !strings.Contains(err.Error(), "--trace-engine=builtin bypasses trace_streamer") {
		t.Fatalf("content-independent builtin conflict was not validated statically: %v", err)
	}
	if err := ValidateOptions(Options{InputPath: filepath.Join(t.TempDir(), "definitely-missing")}); err != nil {
		t.Fatalf("static validation consulted a missing input path: %v", err)
	}
	if err := ValidateOptions(Options{TraceEngine: "invented"}); err == nil {
		t.Fatal("invalid trace engine escaped static enum validation")
	}
	if err := ValidateOptions(Options{PerfParser: "invented"}); err == nil {
		t.Fatal("invalid perf parser escaped static enum validation")
	}

	for _, function := range []string{"ValidateOptions", "validateOptionEnums", "validateBuiltinTraceOptions"} {
		body := sourceGenerationFunctionBody(t, "trace_provider.go", function)
		for _, forbidden := range []string{
			"traceInputUsesDirectPerfRoute", "detectPerfInputFormat", "validateTraceOutputPathCollisions",
			"canonicalTracePath", "os.Stat", "os.Open", "os.Lstat",
		} {
			if strings.Contains(body, forbidden) {
				t.Fatalf("%s regained input/filesystem dependency %q:\n%s", function, forbidden, body)
			}
		}
	}
}

func TestReleaseConversionRouteProbeClassifierIsFixedAndContentOnly(t *testing.T) {
	tests := []struct {
		name string
		data []byte
		want perfInputFormat
	}{
		{name: "linux perf", data: append([]byte(perfMagic2), bytes.Repeat([]byte{0}, 80)...), want: perfInputLinuxPerfData},
		{name: "simpleperf proto", data: []byte("SIMPLEPERF\x00typed"), want: perfInputSimpleperfReportProto},
		{name: "perftrace text", data: []byte("header data perf_sample: cpu=1"), want: perfInputPerfTraceText},
		{name: "short unknown", data: []byte("PER"), want: perfInputUnknown},
		{name: "marker outside fixed probe", data: append(bytes.Repeat([]byte("x"), conversionInputProbeSize), []byte("perf_sample:")...), want: perfInputUnknown},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := detectPerfInputFormatProbe(test.data); got != test.want {
				t.Fatalf("format=%q want=%q", got, test.want)
			}
		})
	}
}

func TestReleaseConversionAuthorityRejectsInputOutputHardlinkAliases(t *testing.T) {
	tests := []struct {
		name      string
		configure func(input, output string) (Options, string)
		want      string
	}{
		{
			name: "systrace output aliases input",
			configure: func(input, output string) (Options, string) {
				if err := os.Link(input, output); err != nil {
					t.Fatal(err)
				}
				return Options{InputPath: input, OutputPath: output}, output
			},
			want: "trace input and systrace output must be different files",
		},
		{
			name: "tracebundle output aliases input",
			configure: func(input, output string) (Options, string) {
				bundle := traceSidecarBase(input, output) + ".tracebundle.json"
				if err := os.Link(input, bundle); err != nil {
					t.Fatal(err)
				}
				return Options{InputPath: input, OutputPath: output}, bundle
			},
			want: "trace input and tracebundle output must be different files",
		},
		{
			name: "trace DB output aliases input",
			configure: func(input, output string) (Options, string) {
				db := filepath.Join(filepath.Dir(output), "retained.trace.db")
				if err := os.Link(input, db); err != nil {
					t.Fatal(err)
				}
				return Options{InputPath: input, OutputPath: output, TraceDBOutputPath: db}, db
			},
			want: "trace input and trace DB output must be different files",
		},
		{
			name: "trace DB companion output aliases input",
			configure: func(input, output string) (Options, string) {
				db := filepath.Join(filepath.Dir(output), "retained.trace.db")
				companion := db + ".ohos.ts"
				if err := os.Link(input, companion); err != nil {
					t.Fatal(err)
				}
				return Options{InputPath: input, OutputPath: output, TraceDBOutputPath: db}, companion
			},
			want: "trace input and trace DB companion output must be different files",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			original := []byte("stable input generation")
			if err := os.WriteFile(input, original, 0o640); err != nil {
				t.Fatal(err)
			}
			opts, alias := test.configure(input, output)
			before, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			authority, err := openConversionInputAuthority(input)
			if unavailableConversionInputAuthority(t, err) {
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			defer authority.Close()
			if _, err := validateOptionsForInput(opts, authority, perfInputUnknown); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("hardlink alias was not rejected precisely: %v", err)
			}
			after, err := os.Stat(input)
			if err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(input)
			if err != nil {
				t.Fatal(err)
			}
			aliasInfo, err := os.Stat(alias)
			if err != nil {
				t.Fatal(err)
			}
			if !os.SameFile(before, after) || !os.SameFile(after, aliasInfo) || before.Mode() != after.Mode() || before.Size() != after.Size() || !bytes.Equal(got, original) {
				t.Fatalf("collision validation mutated input: before=%v after=%v bytes=%q", before, after, got)
			}
		})
	}
}

func TestReleaseConversionPreservesTrimmedRelativeDisplayPath(t *testing.T) {
	dir := t.TempDir()
	absInput := filepath.Join(dir, "relative-display.sys")
	if err := os.WriteFile(absInput, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	relative, err := filepath.Rel(cwd, absInput)
	if err != nil {
		t.Fatal(err)
	}
	result, err := ConvertFile(context.Background(), Options{
		InputPath:   " \t" + relative + " \n",
		TraceEngine: traceEngineBuiltin,
	})
	if conversionInputErrorCode(err) == ConversionInputCodeStrongIdentityUnavailable {
		t.Skipf("platform cannot prove strong conversion identity: %v", err)
	}
	if err != nil {
		t.Fatalf("convert relative display path: %v", err)
	}
	if result.InputPath != relative || result.OutputPath != DefaultOutputPath(relative) {
		t.Fatalf("display path drifted: input=%q output=%q want_input=%q", result.InputPath, result.OutputPath, relative)
	}
	for _, decision := range result.TraceDecisions {
		if decision.InputPath != "" && decision.InputPath != relative {
			t.Fatalf("trace decision leaked a second input namespace: %+v", decision)
		}
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		InputPath string `json:"input_path"`
	}
	if err := json.Unmarshal(bundle, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.InputPath != relative || manifest.InputPath == filepath.Clean(absInput) {
		t.Fatalf("tracebundle display identity drifted: got=%q want=%q\n%s", manifest.InputPath, relative, bundle)
	}
}

func TestReleaseConversionPreCommitGenerationChangeRollsBackDirectPerf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX report adapter")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.perf.data")
	original := syntheticRawPerfData()
	if err := os.WriteFile(input, original, 0o644); err != nil {
		t.Fatal(err)
	}
	probe, err := openConversionInputAuthority(input)
	if unavailableConversionInputAuthority(t, err) {
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	if err := probe.Close(); err != nil {
		t.Fatal(err)
	}
	originalInfo, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	tool, signal, resume := writeSourceGenerationBlockingSimpleperfTool(t, dir)
	defer func() { _ = os.WriteFile(resume, []byte("test cleanup\n"), 0o600) }()
	output := filepath.Join(dir, "result.systrace")
	done := make(chan sourceGenerationConversionOutcome, 1)
	go func() {
		result, err := ConvertFile(context.Background(), Options{
			InputPath: input, OutputPath: output, SimpleperfReportPath: tool,
		})
		done <- sourceGenerationConversionOutcome{result: result, err: err}
	}()
	waitForSourceGenerationSignalOrEarlyResult(t, signal, done)
	mutated := bytes.Repeat([]byte{0x5a}, len(original))
	if err := os.WriteFile(input, mutated, originalInfo.Mode().Perm()); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(input, originalInfo.ModTime(), originalInfo.ModTime()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resume, []byte("continue\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var outcome sourceGenerationConversionOutcome
	select {
	case outcome = <-done:
	case <-time.After(sourceGenerationFixtureTimeout):
		t.Fatal("conversion did not leave the controlled adapter stage")
	}
	var typed *ConversionInputError
	if !errors.As(outcome.err, &typed) || typed.Code != ConversionInputCodeGenerationChanged || typed.Stage != conversionInputStagePreCommit.String() {
		t.Fatalf("pre-commit mutation lost typed generation verdict: result=%+v err=%v", outcome.result, outcome.err)
	}
	if !reflect.DeepEqual(outcome.result, Result{}) {
		t.Fatalf("failed generation transaction leaked result authority: %+v", outcome.result)
	}
	after, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	if !os.SameFile(originalInfo, after) || after.Size() != originalInfo.Size() || !after.ModTime().Equal(originalInfo.ModTime()) {
		t.Fatalf("controlled same-size/restored-mtime mutation profile drifted: before=%v after=%v", originalInfo, after)
	}
	assertNoSourceGenerationPublication(t, input, output)
}

func TestReleaseConversionCancellationKeepsSentinelAndRollsBackDirectPerf(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fixture uses a POSIX report adapter")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "cancel.perf.data")
	if err := os.WriteFile(input, syntheticRawPerfData(), 0o644); err != nil {
		t.Fatal(err)
	}
	tool, signal, _ := writeSourceGenerationBlockingSimpleperfTool(t, dir)
	output := filepath.Join(dir, "cancel.systrace")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan sourceGenerationConversionOutcome, 1)
	go func() {
		result, err := ConvertFile(ctx, Options{InputPath: input, OutputPath: output, SimpleperfReportPath: tool})
		done <- sourceGenerationConversionOutcome{result: result, err: err}
	}()
	waitForSourceGenerationSignalOrEarlyResult(t, signal, done)
	cancel()
	select {
	case outcome := <-done:
		if !errors.Is(outcome.err, context.Canceled) || !reflect.DeepEqual(outcome.result, Result{}) {
			t.Fatalf("cancellation identity/result drifted: result=%+v err=%v", outcome.result, outcome.err)
		}
	case <-time.After(sourceGenerationFixtureTimeout):
		t.Fatal("canceled conversion did not terminate")
	}
	assertNoSourceGenerationPublication(t, input, output)
}

func TestReleaseConvertFileRouteAndCommitAreStructurallyAuthorityOwned(t *testing.T) {
	convertBody := sourceGenerationFunctionBody(t, "convert.go", "ConvertFile")
	if strings.Count(convertBody, "openConversionInputAuthority(") != 1 {
		t.Fatalf("ConvertFile must have exactly one input-authority open:\n%s", convertBody)
	}
	for _, forbidden := range []string{"traceInputUsesDirectPerfRoute(opts)", "detectPerfInputFormat(input)", "os.Stat(input)", "buildTraceProviderPlan(opts"} {
		if strings.Contains(convertBody, forbidden) {
			t.Fatalf("ConvertFile regained path-based input authority %q", forbidden)
		}
	}
	for _, required := range []string{
		"validateOptionEnums(opts)", "opts.InputPath = input", "authority.Probe()", "detectPerfInputFormatProbe(probe)",
		"validateOptionsForInput(opts, authority, inputFormat)", "buildTraceProviderPlanWithInput(opts", "newConversionFileLedgerForAuthority(authority)",
		"newDirectPerfInputBinding(authority, inputFormat)", "maybeConvertDirectSimpleperfPerfData(ctx, opts, directPlan, directInput, output, ledger)",
	} {
		if !strings.Contains(convertBody, required) {
			t.Fatalf("ConvertFile lost authority route step %q", required)
		}
	}
	assertSourceGenerationOrder(t, convertBody,
		"authority.Probe()",
		"detectPerfInputFormatProbe(probe)",
		"validateOptionsForInput(opts, authority, inputFormat)",
		"newConversionFileLedgerForAuthority(authority)",
	)
	commitAt := strings.Index(convertBody, "commit := func")
	if commitAt < 0 {
		t.Fatal("ConvertFile lost commit closure")
	}
	commitBody := convertBody[commitAt:]
	assertSourceGenerationOrder(t, commitBody,
		"authority.Validate(conversionInputStagePreCommit)",
		"authority.Close()",
		"ledger.validateOwnedPaths()",
		"committed = true",
	)

	directBody := sourceGenerationFunctionBody(t, "simpleperf_text.go", "maybeConvertDirectSimpleperfPerfData")
	if !strings.Contains(directBody, "input directPerfInputBinding") ||
		!strings.Contains(directBody, "input.inputFormat") ||
		strings.Contains(directBody, "detectPerfInputFormat") {
		t.Fatalf("direct perf consumer does not exclusively consume the authority-bound typed route:\n%s", directBody)
	}
	for _, function := range []string{"maybeConvertRawPerfData", "maybeConvertRawPerfDataWithDecision"} {
		body := sourceGenerationFunctionBody(t, "raw_perfdata.go", function)
		if !strings.Contains(body, "inputFormat perfInputFormat") || strings.Contains(body, "detectPerfInputFormat") {
			t.Fatalf("%s regained path-based format provenance:\n%s", function, body)
		}
	}
	ledgerBody := sourceGenerationFunctionBody(t, "transaction.go", "newConversionFileLedgerForAuthority")
	if !strings.Contains(ledgerBody, "authority.canonicalIdentity()") || strings.Contains(ledgerBody, "canonicalTracePath") {
		t.Fatalf("conversion ledger did not consume the admitted authority identity:\n%s", ledgerBody)
	}
	for _, helper := range []struct {
		file string
		name string
	}{
		{file: "trace_provider.go", name: "validateOptionsForInput"},
		{file: "trace_tools.go", name: "buildTraceProviderPlanWithInput"},
	} {
		body := sourceGenerationFunctionBody(t, helper.file, helper.name)
		if strings.Contains(body, "detectPerfInputFormat") || strings.Contains(body, "traceInputUsesDirectPerfRoute") {
			t.Fatalf("%s regained path-based route classification:\n%s", helper.name, body)
		}
	}
}

func writeSourceGenerationBlockingSimpleperfTool(t *testing.T, dir string) (tool, signal, resume string) {
	t.Helper()
	report := filepath.Join(dir, "report.txt")
	if err := os.WriteFile(report, []byte(syntheticSimpleperfReport()), 0o600); err != nil {
		t.Fatal(err)
	}
	tool = filepath.Join(dir, "report_sample")
	signal = filepath.Join(dir, "adapter.ready")
	resume = filepath.Join(dir, "adapter.continue")
	script := `#!/bin/sh
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-o" ]; then
    shift
    out="$1"
  fi
  shift
done
cp "$SOURCE_GEN_REPORT" "$out" || exit 31
: > "$SOURCE_GEN_SIGNAL" || exit 32
while [ ! -e "$SOURCE_GEN_RESUME" ]; do
  sleep 0.01
done
`
	if err := os.WriteFile(tool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOURCE_GEN_REPORT", report)
	t.Setenv("SOURCE_GEN_SIGNAL", signal)
	t.Setenv("SOURCE_GEN_RESUME", resume)
	return tool, signal, resume
}

func waitForSourceGenerationSignalOrEarlyResult(t *testing.T, path string, done <-chan sourceGenerationConversionOutcome) {
	t.Helper()
	deadline := time.Now().Add(sourceGenerationFixtureTimeout)
	for time.Now().Before(deadline) {
		select {
		case outcome := <-done:
			t.Fatalf("conversion returned before adapter control signal: %v", outcome.err)
		default:
		}
		if _, err := os.Lstat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("adapter did not publish control signal %s", path)
}

func assertNoSourceGenerationPublication(t *testing.T, input, output string) {
	t.Helper()
	base := traceSidecarBase(input, output)
	for _, path := range []string{output, base + ".perftrace", base + ".tracebundle.json"} {
		if _, err := os.Lstat(path); !os.IsNotExist(err) {
			t.Fatalf("failed input transaction retained publication %s: %v", path, err)
		}
	}
	staging, err := filepath.Glob(filepath.Join(filepath.Dir(base), "."+filepath.Base(base)+".perftrace.*.simpleperf"))
	if err != nil {
		t.Fatal(err)
	}
	if len(staging) != 0 {
		t.Fatalf("failed input transaction retained simpleperf staging: %v", staging)
	}
}

func sourceGenerationFunctionBody(t *testing.T, filename, function string) string {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve source-generation release test path")
	}
	path := filepath.Join(filepath.Dir(current), filename)
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range parsed.Decls {
		candidate, ok := declaration.(*ast.FuncDecl)
		if !ok || candidate.Name.Name != function {
			continue
		}
		start := fset.Position(candidate.Pos()).Offset
		end := fset.Position(candidate.End()).Offset
		return string(source[start:end])
	}
	t.Fatalf("function %s not found in %s", function, path)
	return ""
}

func assertSourceGenerationOrder(t *testing.T, source string, fragments ...string) {
	t.Helper()
	previous := -1
	for _, fragment := range fragments {
		position := strings.Index(source, fragment)
		if position < 0 || position <= previous {
			t.Fatalf("source-generation order lost at %q after offset %d:\n%s", fragment, previous, source)
		}
		previous = position
	}
}
