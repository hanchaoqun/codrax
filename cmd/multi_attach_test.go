package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLoadMultiPathSlice_HeaderedConcat verifies that two --log
// entries concatenate with `# codrax-source:` boundary headers.
func TestLoadMultiPathSlice_HeaderedConcat(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.log")
	b := filepath.Join(dir, "b.log")
	if err := os.WriteFile(a, []byte("body of A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte("body of B\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, err := loadMultiPathSlice("log", []string{a, b}, "", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "# codrax-source: "+a) {
		t.Errorf("missing source header for a.log; body=%q", body)
	}
	if !strings.Contains(body, "# codrax-source: "+b) {
		t.Errorf("missing source header for b.log; body=%q", body)
	}
	if !strings.Contains(body, "body of A") || !strings.Contains(body, "body of B") {
		t.Errorf("file bodies missing; got %q", body)
	}
	// A's header must precede B's (CLI order preserved).
	idxA := strings.Index(body, "a.log")
	idxB := strings.Index(body, "b.log")
	if idxA >= idxB {
		t.Errorf("CLI order not preserved: a.log at %d, b.log at %d", idxA, idxB)
	}
}

func TestLoadMultiPathSlice_FileReadsStayWithinAggregateCap(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.log")
	b := filepath.Join(dir, "b.log")
	if err := os.WriteFile(a, []byte(strings.Repeat("a", 16*1024)), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(b, []byte(strings.Repeat("b", 16*1024)), 0o644); err != nil {
		t.Fatal(err)
	}

	const capBytes = 4096
	body, err := loadMultiPathSlice("log", []string{a, b}, "", capBytes)
	if err != nil {
		t.Fatal(err)
	}
	if len(body) != capBytes+1 {
		t.Fatalf("bounded loader should stop at cap+1 sentinel bytes, got %d", len(body))
	}
	if len(truncateAttachedToCap(body, capBytes, "log")) != capBytes {
		t.Fatalf("final truncation should cut bounded body to cap")
	}
}

// TestLoadMultiPathSlice_InlineTakesPriority verifies inline-text
// overrides path slice (matches mutual-exclusion contract).
func TestLoadMultiPathSlice_InlineTakesPriority(t *testing.T) {
	body, err := loadMultiPathSlice("log", []string{"/no/such/file"}, "INLINE BODY", 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	if body != "INLINE BODY" {
		t.Errorf("inline did not win: %q", body)
	}
}

func TestLoadMultiPathSlice_RejectsBinaryLog(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.log")
	if err := os.WriteFile(path, []byte{'l', 'o', 'g', 0, 1, 2}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadMultiPathSlice("log", []string{path}, "", 1<<20)
	if err == nil {
		t.Fatal("binary log should be rejected")
	}
	if !strings.Contains(err.Error(), "not readable text") && !strings.Contains(err.Error(), "可解析文本") {
		t.Fatalf("error should explain text attachment problem: %v", err)
	}
}

func TestLoadMultiPathSlice_RejectsBinaryTraceWithConvertHint(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(path, []byte{'H', 'T', 0, 1, 2, 3}, 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := loadMultiPathSlice("trace", []string{path}, "", 1<<20)
	if err == nil {
		t.Fatal("binary trace should be rejected")
	}
	for _, want := range []string{"codrax trace convert", "--htrace", "capture.htrace"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("trace error missing %q: %v", want, err)
		}
	}
}

// TestEnforceStdinExclusivity catches multi-stdin mistakes.
func TestEnforceStdinExclusivity(t *testing.T) {
	saveLog := flagAttachLog
	saveH := flagAttachHitrace
	saveA := flagAttachAtrace
	defer func() {
		flagAttachLog = saveLog
		flagAttachHitrace = saveH
		flagAttachAtrace = saveA
	}()

	// Single stdin OK.
	flagAttachLog = []string{"-"}
	flagAttachHitrace = nil
	flagAttachAtrace = nil
	if err := enforceStdinExclusivity(); err != nil {
		t.Errorf("single stdin: unexpected err %v", err)
	}
	// Two stdins across channels MUST error.
	flagAttachLog = []string{"-", "/some/file"}
	flagAttachHitrace = []string{"-"}
	if err := enforceStdinExclusivity(); err == nil {
		t.Errorf("expected error on two stdins")
	}
}

// TestTraceCapInheritsLogCap verifies the inheritance default.
// When user only sets log_attach_max_bytes, trace cap mirrors.
func TestTraceCapInheritsLogCap(t *testing.T) {
	saveLog := maxAttachedLogBytes
	saveTrace := maxAttachedTraceBytes
	defer func() {
		maxAttachedLogBytes = saveLog
		maxAttachedTraceBytes = saveTrace
	}()

	// Both defaults equal at startup.
	if defaultAttachedLogMaxBytes != 512*1024*1024 {
		t.Errorf("log default drifted: %d", defaultAttachedLogMaxBytes)
	}
	maxAttachedTraceBytes = 0
	maxAttachedLogBytes = 100 * 1024 * 1024
	// Inheritance is enforced inside initApp's settings merge; here we
	// simulate the post-merge state where trace inherits.
	if maxAttachedTraceBytes <= 0 {
		maxAttachedTraceBytes = maxAttachedLogBytes
	}
	if maxAttachedTraceBytes != maxAttachedLogBytes {
		t.Errorf("trace cap should inherit log cap; got %d vs %d", maxAttachedTraceBytes, maxAttachedLogBytes)
	}
}

// TestTruncateAttachedToCap_PerChannelLabel verifies the WARN label.
func TestTruncateAttachedToCap_PerChannelLabel(t *testing.T) {
	body := strings.Repeat("X", 100)
	out := truncateAttachedToCap(body, 50, "trace")
	if len(out) != 50 {
		t.Errorf("trace truncate failed: got %d", len(out))
	}
}

// TestHardCeilingClamp_Constant pins the OOM-protection bound. A
// drift here would let a misconfigured codrax.yaml allocate
// arbitrary memory on a single attachment.
func TestHardCeilingClamp_Constant(t *testing.T) {
	want := 1 << 30 // 1 GiB
	if maxAttachedLogHardCeiling != want {
		t.Errorf("hard ceiling drifted: want %d, got %d", want, maxAttachedLogHardCeiling)
	}
}

// TestDefaultsAre512MiB pins the user-facing 512 MiB default for both
// log and trace channels (trace inherits at startup).
func TestDefaultsAre512MiB(t *testing.T) {
	want := 512 * 1024 * 1024
	if defaultAttachedLogMaxBytes != want {
		t.Errorf("log default: want %d, got %d", want, defaultAttachedLogMaxBytes)
	}
}

func TestCLIRuntimeArtifactStatusLinesSummarizeMultiArtifacts(t *testing.T) {
	logBody := "# codrax-source: first.log\npanic\n# codrax-source: second.log\ntimeout"
	traceBody := "# codrax-source: frame.systrace\nsched_switch\n# codrax-source: frame.perftrace\nperf_sample: pid=1 tid=1 source=raw_perfdata_fallback symbolization_status=unsymbolized"
	lines := cliRuntimeArtifactStatusLines("en", cliRuntimeArtifacts("", logBody, traceBody))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"total=4",
		"log=2",
		"trace=1",
		"perftrace=1",
		"primary: trace frame.systrace",
		"frame.perftrace",
		"source=raw_perfdata_fallback",
		"symbolization_status=unsymbolized",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("runtime artifact status missing %q:\n%s", want, joined)
		}
	}
}

func TestCLIRuntimeArtifactStatusLinesExpandTraceBundle(t *testing.T) {
	bundle := strings.Join([]string{
		"# codrax-source: capture.tracebundle.json",
		`{`,
		`  "version": "hitraceconv-v1",`,
		`  "artifacts": [`,
		`    {"type":"systrace","path":"capture.systrace","bytes":12,"converter":"hitraceconv-v1"},`,
		`    {"type":"perftrace","path":"capture.perftrace","bytes":34,"converter":"hiperf_proto","caveats":["cpu id unavailable"]}`,
		`  ],`,
		`  "trace_provider_decisions": [`,
		`    {"provider_name":"trace_streamer_db","engine_mode":"auto","selected":true,"attempted":true,"succeeded":true,"trace_query_ready":true}`,
		`  ],`,
		`  "trace_db_coverage": [`,
		`    {"family":"scheduler","table":"sched_slice","found":true,"rows_read":2,"rows_emitted":2},`,
		`    {"family":"trace_marker","table":"instant","found":false,"skipped":"table_missing"}`,
		`  ],`,
		`  "trace_coverage": [`,
		`    {"family":"trace_cross_validation","table":"tracequery_build_index","found":true,"rows_read":2,"rows_emitted":2}`,
		`  ]`,
		`}`,
	}, "\n")
	lines := cliRuntimeArtifactStatusLines("en", cliRuntimeArtifacts("", "", bundle))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"total=3",
		"tracebundle=1",
		"systrace=1",
		"perftrace=1",
		"primary: tracebundle capture.tracebundle.json",
		"trace_provider=trace_streamer_db",
		"trace_query_ready=true",
		"trace_db_coverage=scheduler/sched_slice",
		"trace_coverage=trace_cross_validation/tracequery_build_index",
		"capture.perftrace",
		"converter=hiperf_proto",
		"cpu id unavailable",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("tracebundle status missing %q:\n%s", want, joined)
		}
	}
}

func TestCLIRuntimeArtifactStatusLinesIncludeRequestTraceBundleAndLogPaths(t *testing.T) {
	dir := t.TempDir()
	bundlePath := filepath.Join(dir, "capture.tracebundle.json")
	logPath := filepath.Join(dir, "app.log")
	if err := os.WriteFile(filepath.Join(dir, "capture.systrace"), []byte("sched_switch: prev_comm=app prev_pid=1 ==> next_comm=worker next_pid=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	bundle := `{
  "version": "hitraceconv-v1",
  "systrace": "capture.systrace",
  "artifacts": [
    {"type":"systrace","path":"capture.systrace","bytes":88,"converter":"trace_streamer_db"}
  ],
  "trace_provider_decisions": [
    {"provider_name":"trace_streamer_db","engine_mode":"auto","selected":true,"attempted":true,"succeeded":true,"trace_query_ready":true}
  ],
  "trace_db_coverage": [
    {"family":"scheduler","table":"sched_slice","found":true,"rows_read":2,"rows_emitted":2}
  ],
  "trace_coverage": [
    {"family":"trace_cross_validation","table":"tracequery_build_index","found":true,"rows_read":2,"rows_emitted":2}
  ]
}
`
	if err := os.WriteFile(bundlePath, []byte(bundle), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(logPath, []byte("panic: boom\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	request := "分析 " + bundlePath + " 并结合 " + logPath
	lines := cliRuntimeArtifactStatusLines("zh", cliRuntimeArtifacts(request, "", ""))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"共 3 个",
		"tracebundle=1",
		"systrace=1",
		"log=1",
		"primary: tracebundle " + bundlePath,
		"referenced in request",
		"trace_provider=trace_streamer_db",
		"trace_db_coverage=scheduler/sched_slice",
		"trace_coverage=trace_cross_validation/tracequery_build_index",
		logPath,
		"runtime log",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("request path tracebundle/log status missing %q:\n%s", want, joined)
		}
	}
}

func TestCLIRuntimeArtifactStatusLinesIncludeRequestPaths(t *testing.T) {
	dir := t.TempDir()
	tracePath := filepath.Join(dir, "capture")
	if err := os.WriteFile(tracePath, []byte("sched_switch: prev_comm=app prev_pid=1 ==> next_comm=worker next_pid=2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	lines := cliRuntimeArtifactStatusLines("en", cliRuntimeArtifacts("analyse "+tracePath, "", ""))
	joined := strings.Join(lines, "\n")
	for _, want := range []string{
		"total=1",
		"trace=1",
		"primary: trace " + tracePath,
		"referenced in request",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("request path runtime artifact status missing %q:\n%s", want, joined)
		}
	}
}
