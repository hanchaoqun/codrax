package hitraceconv

import (
	"context"
	"errors"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestEmbeddedTraceStreamerELFInterpreterReadsPinnedLinuxChild(t *testing.T) {
	child := filepath.Join(embeddedTraceStreamerDir, "linux-amd64", "trace_streamer")
	interpreter, err := embeddedTraceStreamerELFInterpreter(child)
	if err != nil {
		t.Fatalf("read pinned Linux child PT_INTERP: %v", err)
	}
	if interpreter != "/lib64/ld-linux-x86-64.so.2" {
		t.Fatalf("interpreter=%q want /lib64/ld-linux-x86-64.so.2", interpreter)
	}
}

func TestEmbeddedTraceStreamerRuntimeProbeResultIsTypedAndBounded(t *testing.T) {
	long := strings.Repeat("x", 4096)
	tests := []struct {
		name       string
		contextErr error
		runErr     error
		output     string
		want       string
	}{
		{name: "ready"},
		{name: "timeout", contextErr: context.DeadlineExceeded, runErr: context.DeadlineExceeded, want: "probe_timeout"},
		{name: "missing library", runErr: errors.New("exit status 127"), output: "libgcc_s.so.1 => not found", want: "shared_library_missing"},
		{name: "old glibc", runErr: errors.New("exit status 1"), output: "version `GLIBC_2.34' not found", want: "glibc_too_old_or_symbol_missing"},
		{name: "loader failure", runErr: errors.New("exit status 1"), output: long, want: "loader_probe_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := embeddedTraceStreamerRuntimeProbeResult(test.contextErr, test.runErr, test.output, "/lib64/ld-linux-x86-64.so.2")
			if test.want == "" {
				if err != nil {
					t.Fatalf("ready probe failed: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("probe error=%v want class %s", err, test.want)
			}
			if len(err.Error()) > 1400 {
				t.Fatalf("probe error is not bounded: %d bytes", len(err.Error()))
			}
		})
	}
}

func TestEmbeddedRuntimeReasonIsSingleLineUTF8AndBounded(t *testing.T) {
	got := boundedEmbeddedRuntimeReason("lib路径\x1b[31m\r\n\t" + strings.Repeat("界", 1024))
	if !utf8.ValidString(got) {
		t.Fatalf("bounded runtime reason split UTF-8: %q", got)
	}
	if strings.ContainsAny(got, "\x1b\r\n\t") {
		t.Fatalf("bounded runtime reason retained terminal controls: %q", got)
	}
	for _, want := range []string{"lib路径", `\u{1b}`, `\r`, `\n`, `\t`, "…"} {
		if !strings.Contains(got, want) {
			t.Fatalf("bounded runtime reason missing %q: %q", want, got)
		}
	}
	if len(got) > 1024 {
		t.Fatalf("bounded runtime reason exceeds byte cap: %d", len(got))
	}
}

func TestVerifiedEmbeddedResolutionCarriesRuntimeAuthority(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("runtime-compatible-embedded-fixture")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	starveExternalTraceStreamerDiscovery(t)

	resolution := resolveTraceStreamerToolResolution(Options{})
	if resolution.EmbeddedLinuxRuntime != (runtime.GOOS == "linux") || resolution.Path == "" || !strings.Contains(resolution.Source, "embedded trace_streamer") {
		t.Fatalf("embedded runtime authority was not preserved: %+v", resolution)
	}
}

func TestEmbeddedTraceStreamerRuntimeProbeSkipsNonLinuxChild(t *testing.T) {
	if err := probeEmbeddedTraceStreamerRuntime(filepath.Join(t.TempDir(), "missing-child"), embeddedTraceStreamerPlatform{GOOS: "windows", GOARCH: "amd64"}); err != nil {
		t.Fatalf("non-Linux child expanded runtime probe execution: %v", err)
	}
}

func TestEmbeddedTraceStreamerRuntimeEnvironmentIsDeterministic(t *testing.T) {
	got := embeddedTraceStreamerRuntimeEnvironment([]string{
		"PATH=/usr/bin",
		"HOME=/home/customer",
		"LANG=zh_CN.UTF-8",
		"LANGUAGE=zh_CN",
		"LC_TIME=de_DE.UTF-8",
		"GLIBC_TUNABLES=glibc.malloc.check=3",
		"LD_LIBRARY_PATH=/tmp/runtime",
		"LD_PRELOAD=/tmp/inject.so",
		"LD_AUDIT=/tmp/audit.so",
		"LD_DEBUG=all",
		"LD_DEBUG_OUTPUT=/tmp/debug",
		"LD_PROFILE=child",
		"LD_SHOW_AUXV=1",
		"LD_TRACE_LOADED_OBJECTS=1",
		"MALFORMED",
	})
	joined := strings.Join(got, "\n")
	for _, want := range []string{"PATH=/usr/bin", "HOME=/home/customer", "LANG=C", "LC_ALL=C"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("probe environment lost %q: %q", want, got)
		}
	}
	for _, denied := range []string{"LD_", "GLIBC_TUNABLES=", "LANG=zh_CN", "LANGUAGE=", "LC_TIME=", "MALFORMED"} {
		if strings.Contains(joined, denied) {
			t.Fatalf("probe environment retained %q: %q", denied, got)
		}
	}
}

func TestTraceToolStatusReportsEmbeddedRuntimeIncompatibility(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("runtime-incompatible-embedded-fixture")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	embeddedTraceStreamerRuntimeProbe = func(string, embeddedTraceStreamerPlatform) error {
		return errors.New("loader_missing: /lib64/ld-linux-x86-64.so.2")
	}
	starveExternalTraceStreamerDiscovery(t)

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if status.TraceStreamer.Available || status.TraceStreamer.Path != "" || status.TraceStreamer.Source != "embedded_runtime_incompatible" {
		t.Fatalf("runtime-incompatible embedded child was advertised as usable: %+v", status.TraceStreamer)
	}
	if status.SelectedEngine != traceEngineBuiltin {
		t.Fatalf("auto did not select builtin fallback: %+v", status)
	}
	joined := strings.Join(status.TraceStreamer.Caveats, "\n")
	for _, want := range []string{"embedded trace_streamer runtime is incompatible", "parent_runtime_independent=true", "loader_missing"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("runtime caveat missing %q: %s", want, joined)
		}
	}
	var runtimeCaveat string
	for _, caveat := range status.TraceStreamer.Caveats {
		if strings.Contains(caveat, "embedded trace_streamer runtime is incompatible") {
			runtimeCaveat = caveat
			break
		}
	}
	if runtimeCaveat == "" {
		t.Fatalf("runtime caveat missing from status: %+v", status.TraceStreamer.Caveats)
	}
	zh := LocalizeEmbeddedTraceStreamerCaveatZh(runtimeCaveat)
	for _, want := range []string{"子工具运行时不兼容", "Codrax 父程序", "配置兼容的外部 trace_streamer"} {
		if !strings.Contains(zh, want) {
			t.Fatalf("localized runtime caveat missing %q: %s", want, zh)
		}
	}
}

func TestCompatiblePATHProviderTakesOverAndPreservesEmbeddedRuntimeCaveat(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("runtime-incompatible-embedded-fixture")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	embeddedTraceStreamerRuntimeProbe = func(string, embeddedTraceStreamerPlatform) error {
		return errors.New("shared_library_missing: libgcc_s.so.1")
	}
	starveExternalTraceStreamerDiscovery(t)
	toolDir := t.TempDir()
	tool := filepath.Join(toolDir, traceStreamerBinaryName())
	mode := os.FileMode(0o755)
	if runtime.GOOS == "windows" {
		mode = 0o644
	}
	if err := os.WriteFile(tool, []byte("host-compatible-provider"), mode); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", toolDir)

	status, err := BuildTraceToolStatus(Options{TraceEngine: traceEngineAuto})
	if err != nil {
		t.Fatal(err)
	}
	if !status.TraceStreamer.Available || status.TraceStreamer.Path != tool || !strings.Contains(status.TraceStreamer.Source, "on PATH") {
		t.Fatalf("PATH provider did not take over incompatible embedded child: %+v", status.TraceStreamer)
	}
	if joined := strings.Join(status.TraceStreamer.Caveats, "\n"); !strings.Contains(joined, "embedded trace_streamer runtime is incompatible") {
		t.Fatalf("embedded runtime caveat was lost after PATH takeover: %s", joined)
	}
}

func TestConvertFilePreservesEmbeddedRuntimeAndBuiltinFailures(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("runtime-incompatible-embedded-fixture")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	embeddedTraceStreamerRuntimeProbe = func(string, embeddedTraceStreamerPlatform) error {
		return errors.New("glibc_too_old_or_symbol_missing: GLIBC_2.34 not found")
	}
	starveExternalTraceStreamerDiscovery(t)

	dir := t.TempDir()
	input := filepath.Join(dir, "invalid.sys")
	if err := os.WriteFile(input, releaseBuiltinHeader(0xdf49, 54, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output})
	if err == nil {
		t.Fatalf("both failed provider lanes unexpectedly succeeded: %+v", result)
	}
	var composite *TraceProviderFallbackError
	if !errors.As(err, &composite) {
		t.Fatalf("failure is not typed composite: %T %v", err, err)
	}
	if composite.FirstSource != "embedded_runtime_incompatible" ||
		composite.FirstStage != "trace_streamer_discovery" ||
		composite.FirstCode != "trace_streamer_unavailable" {
		t.Fatalf("embedded runtime lane identity was lost: %+v", composite)
	}
	joined := strings.Join(composite.FirstCaveats, "\n")
	if !strings.Contains(joined, "glibc_too_old_or_symbol_missing") || !strings.Contains(err.Error(), "invalid_magic") {
		t.Fatalf("dual-lane evidence was lost: %v", err)
	}
	if result.InputPath != "" || result.OutputPath != "" || result.BundlePath != "" ||
		len(result.Artifacts) != 0 || len(result.ProviderDecisions) != 0 || len(result.TraceDecisions) != 0 ||
		len(result.TraceDBCoverage) != 0 || len(result.TraceCoverage) != 0 || len(result.Caveats) != 0 ||
		result.InputBytes != 0 || result.OutputBytes != 0 || result.EventsWritten != 0 ||
		result.MissingFormatCount != 0 || result.UnknownEventCount != 0 ||
		result.FirstTimestampSec != 0 || result.LastTimestampSec != 0 {
		t.Fatalf("failed conversion published a partial result: %+v", result)
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatalf("failed conversion leaked output %s: %v", output, statErr)
	}
}

func TestExplicitTraceStreamerPreservesEmbeddedRuntimeFailure(t *testing.T) {
	binaryRel := path.Join(runtime.GOOS+"-"+runtime.GOARCH, traceStreamerBinaryName())
	binaryBody := []byte("runtime-incompatible-explicit-fixture")
	manifest := embeddedTraceStreamerTestManifest(binaryRel, binaryBody)
	fsys := embeddedTraceStreamerTestFS(t, manifest, binaryRel, binaryBody, 0o444)
	withEmbeddedTraceStreamerState(t, fsys, true, "")
	embeddedTraceStreamerRuntimeProbe = func(string, embeddedTraceStreamerPlatform) error {
		return errors.New("loader_missing: /lib64/ld-linux-x86-64.so.2")
	}
	starveExternalTraceStreamerDiscovery(t)

	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	if err := os.WriteFile(input, releaseBuiltinHeader(0xdf49, 54, 0), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer})
	if err == nil {
		t.Fatalf("explicit runtime-incompatible provider unexpectedly succeeded: %+v", result)
	}
	var providerFailure *TraceProviderFailureError
	if !errors.As(err, &providerFailure) {
		t.Fatalf("explicit failure lost typed provider identity: %T %v", err, err)
	}
	if providerFailure.Source != "embedded_runtime_incompatible" || providerFailure.Stage != "trace_streamer_discovery" || providerFailure.Code != "trace_streamer_unavailable" {
		t.Fatalf("explicit failure provenance mismatch: %+v", providerFailure)
	}
	for _, want := range []string{"embedded_runtime_incompatible", "loader_missing", "trace_streamer_discovery", "trace_streamer_unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("explicit failure publication missing %q: %v", want, err)
		}
	}
	if _, statErr := os.Lstat(output); !os.IsNotExist(statErr) {
		t.Fatalf("explicit failed conversion leaked output %s: %v", output, statErr)
	}
}
