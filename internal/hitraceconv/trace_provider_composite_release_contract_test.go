//go:build !windows

package hitraceconv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestReleaseTraceProviderCompositePreservesBothRealFailures executes an
// external provider which really exits non-zero, then feeds the same capture to
// the strict built-in decoder. The public composite must stay bounded while
// preserving both machine-readable causes.
func TestReleaseTraceProviderCompositePreservesBothRealFailures(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "invalid.sys")
	if err := os.WriteFile(input, releaseBuiltinHeader(0xbeef, 54, 99), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, attempted := writeReleaseFailingTraceStreamer(t, dir)
	output := filepath.Join(dir, "out.systrace")

	_, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: tool,
	})
	if err == nil {
		t.Fatal("auto conversion unexpectedly succeeded after both providers failed")
	}
	if _, statErr := os.Stat(attempted); statErr != nil {
		t.Fatalf("fake trace_streamer was not actually attempted: %v", statErr)
	}
	if len(err.Error()) > 8192 {
		t.Fatalf("composite error exceeds its 8192-byte publication bound: %d", len(err.Error()))
	}
	var composite *TraceProviderFallbackError
	if !errors.As(err, &composite) {
		t.Fatalf("error is not *TraceProviderFallbackError: %T %v", err, err)
	}
	if composite.FirstCause == nil || !errors.Is(err, composite.FirstCause) {
		t.Fatalf("composite lost the actual first provider cause: %+v", composite)
	}
	if composite.FirstStage != "trace_streamer_export" || composite.FirstCode != "trace_streamer_failed" ||
		!composite.FirstDecision.Attempted || composite.FirstDecision.Succeeded {
		t.Fatalf("first-provider attempt evidence mismatch: %+v", composite)
	}
	var decodeErr *BuiltinSysDecodeError
	if !errors.As(err, &decodeErr) || decodeErr.Code != builtinSysDecodeInvalidMagic {
		t.Fatalf("composite lost strict built-in decode evidence: %T %+v", err, decodeErr)
	}
	if !strings.Contains(err.Error(), "fallback:") || !strings.Contains(err.Error(), "invalid_magic") {
		t.Fatalf("composite publication omits lane context: %v", err)
	}
	if _, statErr := os.Stat(output); !os.IsNotExist(statErr) {
		t.Fatalf("hard failure published output %s: %v", output, statErr)
	}
}

// TestReleaseTraceProviderCancellationBypassesComposite uses a real failed
// first-lane result, then pins that cancellation from the fallback lane remains
// the exact terminal authority rather than being buried in a composite.
func TestReleaseTraceProviderCancellationBypassesComposite(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, releaseBuiltinHeader(0xbeef, 54, 99), 0o600); err != nil {
		t.Fatal(err)
	}
	tool, _ := writeReleaseFailingTraceStreamer(t, dir)
	opts := Options{InputPath: input, OutputPath: filepath.Join(dir, "out.systrace"), TraceStreamerPath: tool}
	plan, err := buildTraceProviderPlan(opts, false)
	if err != nil {
		t.Fatal(err)
	}
	ledger, err := newConversionFileLedger(input)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := openConversionInputAuthority(input)
	if err != nil {
		t.Fatal(err)
	}
	defer authority.Close()
	first, err := runTraceStreamerExport(context.Background(), opts, plan.TraceStreamer, authority, opts.OutputPath, false, ledger)
	if err != nil {
		t.Fatal(err)
	}
	if first.Cause == nil || !first.Decision.Attempted || first.Decision.Succeeded {
		t.Fatalf("fixture did not produce a real first-lane failure: %+v", first)
	}
	got := traceProviderFallbackFailure(plan, first, context.Canceled)
	if !errors.Is(got, context.Canceled) {
		t.Fatalf("cancellation identity lost: %T %v", got, got)
	}
	var composite *TraceProviderFallbackError
	if errors.As(got, &composite) {
		t.Fatalf("cancellation was incorrectly wrapped as provider composite: %+v", composite)
	}
	if cleanupErr := ledger.cleanup(); cleanupErr != nil {
		t.Fatal(cleanupErr)
	}
}

func writeReleaseFailingTraceStreamer(t *testing.T, dir string) (toolPath, attemptedPath string) {
	t.Helper()
	toolPath = filepath.Join(dir, "release-failing-trace_streamer")
	attemptedPath = filepath.Join(dir, "trace_streamer.attempted")
	script := "#!/bin/sh\n" +
		"printf attempted > " + shellSingleQuote(attemptedPath) + "\n" +
		"i=0\n" +
		"while [ \"$i\" -lt 500 ]; do printf 'fake-trace-streamer-output-0123456789' >&2; i=$((i + 1)); done\n" +
		"exit 7\n"
	if err := os.WriteFile(toolPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}
	return toolPath, attemptedPath
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
