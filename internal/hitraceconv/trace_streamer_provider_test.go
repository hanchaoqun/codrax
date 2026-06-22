package hitraceconv

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tracequery"
)

func TestConvertFileTraceStreamerExplicitProducesSystraceTraceDBBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(input, []byte("modern profiler payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	argsLog := filepath.Join(dir, "args.log")
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	output := filepath.Join(dir, "capture.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:           input,
		OutputPath:          output,
		TraceEngine:         "trace_streamer",
		TraceStreamerPath:   traceStreamer,
		TraceStreamerSoDirs: []string{filepath.Join(dir, "symbols")},
	})
	if err != nil {
		t.Fatalf("convert trace_streamer explicit: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten == 0 || result.OutputBytes == 0 {
		t.Fatalf("trace_streamer should emit systrace rows: %+v", result)
	}
	var db Artifact
	var systrace Artifact
	for _, artifact := range result.Artifacts {
		switch artifact.Type {
		case ArtifactTraceDB:
			db = artifact
		case ArtifactSystrace:
			systrace = artifact
		}
	}
	if systrace.Path != output || systrace.Bytes == 0 || !strings.Contains(systrace.Converter, "trace-streamer-db") {
		t.Fatalf("missing systrace artifact: %+v artifacts=%+v", systrace, result.Artifacts)
	}
	if db.Path != strings.TrimSuffix(output, defaultOutputSuffix)+".trace.db" || db.Bytes == 0 {
		t.Fatalf("missing trace DB artifact: %+v artifacts=%+v", db, result.Artifacts)
	}
	args, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{input, "-e", db.Path, "--So_dir", filepath.Join(dir, "symbols")} {
		if !strings.Contains(string(args), want) {
			t.Fatalf("trace_streamer args missing %q:\n%s", want, args)
		}
	}
	idx, err := tracequery.BuildIndex(context.Background(), output)
	if err != nil {
		t.Fatalf("tracequery parse trace_streamer systrace: %v", err)
	}
	if len(idx.Events) == 0 {
		t.Fatalf("tracequery should see scheduler rows from trace_streamer systrace")
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"type": "systrace"`, `"type": "trace_db"`, `"trace_provider_decisions"`, `"provider_name": "trace_streamer_db"`, `"trace_query_ready": true`, `"trace_db_coverage"`} {
		if !strings.Contains(string(bundle), want) {
			t.Fatalf("bundle missing %q:\n%s", want, bundle)
		}
	}
	var parsed struct {
		Coverage      []TraceDBCoverage `json:"trace_db_coverage"`
		TraceCoverage []TraceDBCoverage `json:"trace_coverage"`
	}
	if err := json.Unmarshal(bundle, &parsed); err != nil {
		t.Fatalf("parse tracebundle: %v\n%s", err, bundle)
	}
	assertCoverageEmitted(t, parsed.Coverage, "scheduler", "sched_slice", 1)
	assertCoverageEmitted(t, parsed.Coverage, "sorter", "__systrace_rows__", result.EventsWritten)
	assertCoverageEmitted(t, parsed.TraceCoverage, "trace_cross_validation", "tracequery_build_index", 1)
}

func TestConvertFileNoPerfTraceKeepsBuiltinAndTraceStreamerPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "donghu-no-perf.sys")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}

	builtinOutput := filepath.Join(dir, "builtin.systrace")
	builtin, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  builtinOutput,
		TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert no-perf trace with built-in engine: %v", err)
	}
	if builtin.OutputPath != builtinOutput || builtin.EventsWritten != 1 ||
		!hasTraceDecision(builtin.TraceDecisions, traceProviderNameBuiltinSys, true) {
		t.Fatalf("built-in no-perf trace path regressed: %+v", builtin)
	}

	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	sqlOutput := filepath.Join(dir, "sql.systrace")
	sql, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        sqlOutput,
		TraceEngine:       "trace_streamer",
		TraceStreamerPath: traceStreamer,
		KeepTraceDB:       true,
	})
	if err != nil {
		t.Fatalf("convert no-perf trace with trace_streamer engine: %v", err)
	}
	if sql.OutputPath != sqlOutput || sql.EventsWritten == 0 ||
		!hasTraceDecision(sql.TraceDecisions, traceProviderNameTraceStreamer, true) ||
		!hasArtifact(sql.Artifacts, ArtifactTraceDB) {
		t.Fatalf("trace_streamer no-perf trace path regressed: %+v", sql)
	}
}

func TestConvertFileNoPerfTraceDefaultSQLDoesNotFallbackToBuiltin(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "donghu-no-perf.sys")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "default.systrace")

	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: filepath.Join(dir, "missing_trace_streamer"),
	})
	if err != nil {
		t.Fatalf("default SQL trace conversion should return a diagnostic bundle, not fall through to built-in: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("default SQL conversion must not claim built-in systrace output: %+v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("default SQL conversion should not write built-in systrace output, stat err=%v", err)
	}
	if !hasTraceDecisionReason(result.TraceDecisions, traceProviderNameTraceStreamer, "trace_streamer_unavailable") ||
		hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinSys, true) ||
		hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected selected SQL diagnostic without built-in fallback: %+v", result.TraceDecisions)
	}
	if !containsString(result.Caveats, "pass --trace-engine=builtin") || result.BundlePath == "" {
		t.Fatalf("default SQL diagnostic should explain explicit built-in opt-in and emit bundle: %+v", result)
	}
}

func TestConvertFileNoPerfTraceBuiltinAndTraceStreamerWakeupParity(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "donghu-no-perf.sys")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o644); err != nil {
		t.Fatal(err)
	}

	builtinOutput := filepath.Join(dir, "builtin.systrace")
	builtin, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  builtinOutput,
		TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert no-perf trace with built-in engine: %v", err)
	}
	if !hasTraceDecision(builtin.TraceDecisions, traceProviderNameBuiltinSys, true) {
		t.Fatalf("built-in provider decision missing: %+v", builtin.TraceDecisions)
	}

	fixtureDB := createTraceDBFixture(t, traceStreamerSysWakeupParityDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	sqlOutput := filepath.Join(dir, "sql.systrace")
	sqlResult, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        sqlOutput,
		TraceEngine:       "trace_streamer",
		TraceStreamerPath: traceStreamer,
		KeepTraceDB:       true,
	})
	if err != nil {
		t.Fatalf("convert no-perf trace with trace_streamer engine: %v", err)
	}
	if !hasTraceDecision(sqlResult.TraceDecisions, traceProviderNameTraceStreamer, true) ||
		!hasArtifact(sqlResult.Artifacts, ArtifactTraceDB) {
		t.Fatalf("trace_streamer provider decision or DB artifact missing: %+v artifacts=%+v", sqlResult.TraceDecisions, sqlResult.Artifacts)
	}
	if !coverageHasEmitted(sqlResult.TraceDBCoverage, "scheduler", "instant", 1) {
		t.Fatalf("SQL wakeup export coverage missing: %+v", sqlResult.TraceDBCoverage)
	}
	if !coverageHasEmitted(sqlResult.TraceCoverage, "trace_cross_validation", "tracequery_build_index", 1) {
		t.Fatalf("SQL trace cross-validation coverage missing: %+v", sqlResult.TraceCoverage)
	}

	builtinWakeup := onlyWakeupEvent(t, builtinOutput)
	sqlWakeup := onlyWakeupEvent(t, sqlOutput)
	assertWakeupSemanticParity(t, builtinWakeup, sqlWakeup)
}

func TestConvertFileNoPerfTraceRootCauseEvidenceParityMatrix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "root-cause-matrix.sys")
	if err := os.WriteFile(input, syntheticRootCauseMatrixSysBinary(t), 0o644); err != nil {
		t.Fatal(err)
	}

	builtinOutput := filepath.Join(dir, "matrix-builtin.systrace")
	builtin, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  builtinOutput,
		TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("convert built-in root-cause matrix fixture: %v", err)
	}
	if !hasTraceDecision(builtin.TraceDecisions, traceProviderNameBuiltinSys, true) {
		t.Fatalf("built-in provider decision missing: %+v", builtin.TraceDecisions)
	}

	fixtureDB := createTraceDBFixture(t, traceStreamerRootCauseMatrixDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	sqlOutput := filepath.Join(dir, "matrix-sql.systrace")
	sqlResult, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        sqlOutput,
		TraceEngine:       "trace_streamer",
		TraceStreamerPath: traceStreamer,
		KeepTraceDB:       true,
	})
	if err != nil {
		t.Fatalf("convert SQL root-cause matrix fixture: %v", err)
	}
	if !hasTraceDecision(sqlResult.TraceDecisions, traceProviderNameTraceStreamer, true) || !hasArtifact(sqlResult.Artifacts, ArtifactTraceDB) {
		t.Fatalf("trace_streamer provider decision or DB artifact missing: decisions=%+v artifacts=%+v", sqlResult.TraceDecisions, sqlResult.Artifacts)
	}
	for _, cov := range []struct {
		family string
		table  string
		min    int
	}{
		{"scheduler", "sched_slice", 1},
		{"scheduler", "instant", 2},
		{"irq", "irq", 4},
		{"counter", "measure", 2},
		{"counter", "measure_filter", 1},
		{"slice", "callstack", 2},
		{"slice", "frame_slice", 2},
	} {
		if !coverageHasEmitted(sqlResult.TraceDBCoverage, cov.family, cov.table, cov.min) {
			t.Fatalf("SQL coverage missing %s/%s >= %d: %+v", cov.family, cov.table, cov.min, sqlResult.TraceDBCoverage)
		}
	}
	if !coverageHasEmitted(sqlResult.TraceCoverage, "trace_cross_validation", "tracequery_build_index", 1) {
		t.Fatalf("SQL cross-validation coverage missing: %+v", sqlResult.TraceCoverage)
	}

	builtinIdx, err := tracequery.BuildIndex(context.Background(), builtinOutput)
	if err != nil {
		t.Fatalf("tracequery parse built-in matrix output: %v", err)
	}
	sqlIdx, err := tracequery.BuildIndex(context.Background(), sqlOutput)
	if err != nil {
		t.Fatalf("tracequery parse SQL matrix output: %v", err)
	}
	assertRootCauseEvidenceMatrix(t, "built-in", builtinIdx)
	assertRootCauseEvidenceMatrix(t, "sql", sqlIdx)
}

func TestConvertFileTraceStreamerExplicitNoRowsProducesPartialBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "empty.htrace")
	if err := os.WriteFile(input, []byte("modern profiler payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
	})
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	output := filepath.Join(dir, "empty.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceEngine:       "trace_streamer",
		TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatalf("convert trace_streamer no-row DB: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("no-row DB must not claim systrace output: %+v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("no-row DB should not create systrace file, stat err=%v", err)
	}
	if !hasArtifact(result.Artifacts, ArtifactTraceDB) || hasArtifact(result.Artifacts, ArtifactSystrace) {
		t.Fatalf("partial bundle should preserve DB only: %+v", result.Artifacts)
	}
	if !hasTraceDecisionReason(result.TraceDecisions, traceProviderNameTraceStreamer, "trace_db_no_rows") {
		t.Fatalf("no-row decision missing: %+v", result.TraceDecisions)
	}
	if !coverageHasSkipped(result.TraceDBCoverage, "scheduler", "sched_slice", "missing table") {
		t.Fatalf("coverage should explain missing scheduler rows: %+v", result.TraceDBCoverage)
	}
}

func TestConvertFileTraceStreamerAutoPrefersDBSystraceAndKeepsDB(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler.htrace")
	textTrace := "worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005\n"
	if err := os.WriteFile(input, syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace))), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: traceStreamer,
		KeepTraceDB:       true,
	})
	if err != nil {
		t.Fatalf("convert auto trace_streamer+profiler: %v", err)
	}
	if result.OutputPath != output || result.EventsWritten <= 1 {
		t.Fatalf("default SQL should use trace_streamer DB systrace: %+v", result)
	}
	if !hasArtifact(result.Artifacts, ArtifactTraceDB) {
		t.Fatalf("keep-trace-db should retain DB artifact: %+v", result.Artifacts)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, true) ||
		hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected trace_streamer success without builtin success: %+v", result.TraceDecisions)
	}
}

func TestConvertFileTraceStreamerAutoMergesPerfSidecarsIntoTraceBundle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "trace-with-perf.htrace")
	body := append([]byte("prefix"), syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.02", syntheticRawPerfData())...)
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	traceStreamer := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)

	output := filepath.Join(dir, "merged.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatalf("convert trace_streamer+perf sidecar: %v", err)
	}
	if result.OutputPath != output || !hasArtifact(result.Artifacts, ArtifactPerfData) || !hasArtifact(result.Artifacts, ArtifactPerfTrace) {
		t.Fatalf("trace_streamer systrace should merge perf sidecars: %+v", result)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range [][]byte{[]byte(`"systrace": "`), []byte(`"type": "perf_data"`), []byte(`"type": "perftrace"`), []byte(`"perf_clock_alignments"`), []byte(`"trace_db_coverage"`), []byte(`"family": "trace_cross_validation"`)} {
		if !bytes.Contains(bundle, want) {
			t.Fatalf("merged tracebundle missing %q:\n%s", want, bundle)
		}
	}
}

func TestConvertFileTracePerfAutoDoesNotFallbackToBuiltinTraceBody(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "trace-with-perf-and-text.htrace")
	textTrace := "worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005\n"
	body := append(syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace))),
		syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.02", syntheticRawPerfData())...)
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}
	traceStreamer := writeFakeTraceStreamer(t, dir, 7)

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatalf("trace+perf auto SQL failure should produce partial bundle, not fail hard: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("trace+perf must not fall back to built-in trace body after SQL failure: %+v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("trace+perf SQL failure should not create legacy systrace output, stat err=%v", err)
	}
	if !hasArtifact(result.Artifacts, ArtifactPerfData) || !hasArtifact(result.Artifacts, ArtifactPerfTrace) {
		t.Fatalf("trace+perf partial result should preserve perf sidecars: %+v", result.Artifacts)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, false) ||
		hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected failed SQL decision and no built-in success: %+v", result.TraceDecisions)
	}
	if !containsString(result.Caveats, tracePerfSQLRequiredCaveat) {
		t.Fatalf("trace+perf partial result should explain SQL-only trace body policy: %+v", result.Caveats)
	}
	bundle, err := os.ReadFile(result.BundlePath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(bundle, []byte(`"type": "systrace"`)) || !bytes.Contains(bundle, []byte(`"type": "perftrace"`)) {
		t.Fatalf("trace+perf SQL failure bundle should preserve perf without systrace:\n%s", bundle)
	}
}

func TestConvertFileTracePerfBuiltinEngineDoesNotUseLegacyTraceBody(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "trace-with-perf-builtin.htrace")
	textTrace := "worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005\n"
	body := append(syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace))),
		syntheticStandaloneProfilerBlock(profilerDataTypeHiperf, "hiperf-plugin", "1.02", syntheticRawPerfData())...)
	if err := os.WriteFile(input, body, 0o644); err != nil {
		t.Fatal(err)
	}

	output := filepath.Join(dir, "builtin.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:   input,
		OutputPath:  output,
		TraceEngine: traceEngineBuiltin,
	})
	if err != nil {
		t.Fatalf("trace+perf built-in request should return sidecar-only partial result: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("trace+perf built-in request must not render legacy trace body: %+v", result)
	}
	if !hasArtifact(result.Artifacts, ArtifactPerfData) || !hasArtifact(result.Artifacts, ArtifactPerfTrace) {
		t.Fatalf("trace+perf built-in partial result should preserve perf sidecars: %+v", result.Artifacts)
	}
	if !hasTraceDecisionReason(result.TraceDecisions, traceProviderNameBuiltinModern, "trace_perf_requires_sql") ||
		hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected built-in trace body rejection for trace+perf: %+v", result.TraceDecisions)
	}
}

func TestConvertFileTraceStreamerAutoFailureDoesNotFallbackToProfiler(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake trace_streamer shell fixture uses /bin/sh")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "profiler.htrace")
	textTrace := "worker-1234  ( 1234) [005] ....     1.234567: sched_wakeup: comm=main pid=5678 prio=53 target_cpu=005\n"
	if err := os.WriteFile(input, syntheticProfilerTraceFile(syntheticProfilerPluginData("bytrace_plugin", []byte(textTrace))), 0o644); err != nil {
		t.Fatal(err)
	}
	traceStreamer := writeFakeTraceStreamer(t, dir, 7)

	output := filepath.Join(dir, "out.systrace")
	result, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		OutputPath:        output,
		TraceStreamerPath: traceStreamer,
	})
	if err != nil {
		t.Fatalf("auto trace_streamer failure should return a diagnostic bundle: %v", err)
	}
	if result.OutputPath != "" || result.EventsWritten != 0 {
		t.Fatalf("auto trace_streamer failure must not claim profiler systrace output: %+v", result)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("auto trace_streamer failure should not write built-in profiler systrace, stat err=%v", err)
	}
	if !hasTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer, false) ||
		hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinModern, true) {
		t.Fatalf("expected failed trace_streamer and no successful built-in decisions: %+v", result.TraceDecisions)
	}
	if !containsString(result.Caveats, "pass --trace-engine=builtin") || result.BundlePath == "" {
		t.Fatalf("diagnostic bundle should guide explicit built-in opt-in: caveats=%+v bundle=%q", result.Caveats, result.BundlePath)
	}
}

func TestConvertFileTraceStreamerExplicitMissingToolFailsFast(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(input, []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ConvertFile(context.Background(), Options{
		InputPath:         input,
		TraceEngine:       "trace_streamer",
		TraceStreamerPath: filepath.Join(dir, "missing_trace_streamer"),
	})
	if err == nil || !strings.Contains(err.Error(), "trace_streamer is not available") {
		t.Fatalf("expected explicit missing trace_streamer failure, got %v", err)
	}
}

func writeFakeTraceStreamer(t *testing.T, dir string, exitCode int) string {
	t.Helper()
	path := filepath.Join(dir, "trace_streamer")
	script := `#!/bin/sh
if [ -n "$TRACE_STREAMER_ARGS_LOG" ]; then
  printf '%s\n' "$@" > "$TRACE_STREAMER_ARGS_LOG"
fi
out=""
while [ "$#" -gt 0 ]; do
  if [ "$1" = "-e" ]; then
    shift
    out="$1"
  fi
  shift
done
if [ ` + strconv.Itoa(exitCode) + ` -ne 0 ]; then
  echo "fake trace_streamer failed" >&2
  exit ` + strconv.Itoa(exitCode) + `
fi
if [ -n "$TRACE_STREAMER_FIXTURE_DB" ]; then
  cp "$TRACE_STREAMER_FIXTURE_DB" "$out"
else
  printf 'SQLite format 3\000fake-db\n' > "$out"
fi
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func traceStreamerIntegrationDBStatements() []string {
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (100)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 200, 'proc')",
		"INSERT INTO process VALUES (2, 300, 'waker_proc')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 200, 1, 'MainApp', 100, 1, 1)",
		"INSERT INTO thread VALUES (2, 201, 1, 'WorkerThread', 100, 0, 1)",
		"INSERT INTO thread VALUES (3, 301, 2, 'Waker', 100, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (1000000, 200000, 1, 'S', 42, 2)",
		"INSERT INTO sched_slice VALUES (1300000, 100000, 1, 'R', 20, 3)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (900000, 'sched_wakeup', 2, 3, 'itid', NULL)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (900000, 'sched_wakeup', 7, 2)",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'vec')",
		"CREATE TABLE args (argset INT, key INT, datatype INT, value INT)",
		"INSERT INTO args VALUES (10, 1, 0, 32)",
		"INSERT INTO args VALUES (10, 2, 1, 3)",
		"INSERT INTO args VALUES (20, 4, 0, 9)",
		"CREATE TABLE irq (ts INT, dur INT, callid INT, cat TEXT, name TEXT, argsetid INT)",
		"INSERT INTO irq VALUES (1500000, 10000, 4, 'irq', 'uart', 10)",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, cat TEXT, depth INT, cookie INT, itid INT)",
		"INSERT INTO callstack VALUES (1, 1700000, 40000, 100, 'DoWork', 'slice', 0, NULL, 2)",
	}
}

func traceStreamerSysWakeupParityDBStatements() []string {
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (0)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 36379, 'com.tencent.mm')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 36379, 1, 'com.tencent.mm', 0, 1, 1)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (2942124416000, 1000000, 0, 'R', 53, 1)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (2942124416000, 'sched_wakeup', 1, 1, 'itid', NULL)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (2942124416000, 'sched_wakeup', 4, 1)",
	}
}

func traceStreamerRootCauseMatrixDBStatements() []string {
	base := int64(2942124416000)
	ts := func(delta int64) string { return strconv.FormatInt(base+delta, 10) }
	return []string{
		"CREATE TABLE trace_range (start_ts INT)",
		"INSERT INTO trace_range VALUES (2942124416000)",
		"CREATE TABLE process (ipid INT, pid INT, name TEXT)",
		"INSERT INTO process VALUES (1, 100, 'AppMain')",
		"CREATE TABLE thread (itid INT, tid INT, ipid INT, name TEXT, start_ts INT, is_main_thread INT, switch_count INT)",
		"INSERT INTO thread VALUES (1, 100, 1, 'AppMain', 2942124416000, 1, 2)",
		"INSERT INTO thread VALUES (2, 200, 1, 'WorkerThread', 2942124416000, 0, 2)",
		"CREATE TABLE sched_slice (ts INT, dur INT, cpu INT, end_state TEXT, priority INT, itid INT)",
		"INSERT INTO sched_slice VALUES (" + ts(0) + ", 1000000, 3, 'S', 52, 1)",
		"INSERT INTO sched_slice VALUES (" + ts(1000000) + ", 1000000, 3, 'R', 20, 2)",
		"CREATE TABLE instant (ts INT, name TEXT, ref INT, wakeup_from INT, ref_type TEXT, value REAL)",
		"INSERT INTO instant VALUES (" + ts(0) + ", 'sched_wakeup', 1, 2, 'itid', NULL)",
		"INSERT INTO instant VALUES (" + ts(0) + ", 'sched_waking', 1, 2, 'itid', NULL)",
		"CREATE TABLE raw (ts INT, name TEXT, cpu INT, itid INT)",
		"INSERT INTO raw VALUES (" + ts(0) + ", 'sched_wakeup', 2, 1)",
		"INSERT INTO raw VALUES (" + ts(0) + ", 'sched_waking', 2, 1)",
		"CREATE TABLE thread_state (itid INT, ts INT, dur INT, cpu INT, state TEXT)",
		"INSERT INTO thread_state VALUES (1, " + ts(0) + ", 6000000, 3, 'Running')",
		"CREATE TABLE data_dict (id INT, data TEXT)",
		"INSERT INTO data_dict VALUES (1, 'irq')",
		"INSERT INTO data_dict VALUES (2, 'irq_ret')",
		"INSERT INTO data_dict VALUES (3, 'handled')",
		"INSERT INTO data_dict VALUES (4, 'vec')",
		"CREATE TABLE args (argset INT, key INT, datatype INT, value INT)",
		"INSERT INTO args VALUES (10, 1, 0, 42)",
		"INSERT INTO args VALUES (10, 2, 1, 3)",
		"INSERT INTO args VALUES (20, 4, 0, 9)",
		"CREATE TABLE irq (ts INT, dur INT, callid INT, cat TEXT, name TEXT, argsetid INT)",
		"INSERT INTO irq VALUES (" + ts(3200000) + ", 100000, 2, 'irq', 'kirq', 10)",
		"INSERT INTO irq VALUES (" + ts(3400000) + ", 100000, 2, 'softirq', 'RCU', 20)",
		"CREATE TABLE measure (ts INT, value REAL, filter_id INT)",
		"CREATE TABLE cpu_measure_filter (id INT, name TEXT, cpu INT)",
		"INSERT INTO cpu_measure_filter VALUES (1, 'cpu_frequency', 4)",
		"INSERT INTO cpu_measure_filter VALUES (2, 'cpu_idle', 4)",
		"INSERT INTO measure VALUES (" + ts(1600000) + ", 2200000, 1)",
		"INSERT INTO measure VALUES (" + ts(1700000) + ", 1, 2)",
		"CREATE TABLE measure_filter (id INT, name TEXT, type TEXT)",
		"INSERT INTO measure_filter VALUES (10, 'ddr_freq', 'clock_rate_filter')",
		"INSERT INTO measure VALUES (" + ts(1800000) + ", 400, 10)",
		"CREATE TABLE callstack (id INT, ts INT, dur INT, callid INT, name TEXT, flag TEXT, cookie TEXT, chainId TEXT)",
		"INSERT INTO callstack VALUES (1, " + ts(2000000) + ", 200000, 1, 'DoWork', '', NULL, NULL)",
		"CREATE TABLE native_hook (start_ts INT, end_ts INT, event_type TEXT, heap_size INT, all_heap_size INT, itid INT, ipid INT)",
		"INSERT INTO native_hook VALUES (" + ts(2300000) + ", " + ts(2310000) + ", 'malloc', 64, 8192, 1, 1)",
		"CREATE TABLE frame_slice (ts INT, dur INT, type_desc TEXT, vsync INT, flag TEXT, ipid INT, itid INT)",
		"INSERT INTO frame_slice VALUES (" + ts(2600000) + ", 200000, 'actural', 123, 1, 1, 1)",
	}
}

func onlyWakeupEvent(t *testing.T, path string) tracequery.Event {
	t.Helper()
	idx, err := tracequery.BuildIndex(context.Background(), path)
	if err != nil {
		t.Fatalf("tracequery parse converted trace %s: %v", path, err)
	}
	var out []tracequery.Event
	for _, ev := range idx.Events {
		if ev.Type == tracequery.EventSchedWakeup {
			out = append(out, ev)
		}
	}
	if len(out) != 1 {
		t.Fatalf("expected exactly one sched_wakeup in %s, got %+v", path, out)
	}
	return out[0]
}

func syntheticRootCauseMatrixSysBinary(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	writeFileHeader(&b, 1)
	writeSegment(&b, segmentEventsFormat, []byte(syntheticRootCauseMatrixEventFormat()))
	writeSegment(&b, segmentCmdlines, []byte("100 AppMain\n200 WorkerThread\n"))
	writeSegment(&b, segmentTGIDs, []byte("100 100\n200 100\n"))
	writeSegment(&b, segmentRawTrace, syntheticRawPageEvents([]syntheticRawEvent{
		{EventID: 90, OffsetNS: 1_000_000, Content: syntheticMatrixSchedSwitchContent()},
		{EventID: 91, OffsetNS: 500_000, Content: syntheticMatrixWakeupContent(91)},
		{EventID: 92, OffsetNS: 700_000, Content: syntheticMatrixWakeupContent(92)},
		{EventID: 93, OffsetNS: 1_600_000, Content: syntheticMatrixCPUContent(93, 2_200_000, 4)},
		{EventID: 94, OffsetNS: 1_700_000, Content: syntheticMatrixCPUContent(94, 1, 4)},
		{EventID: 95, OffsetNS: 1_800_000, Content: syntheticMatrixClockContent()},
		{EventID: 96, OffsetNS: 2_000_000, Content: syntheticMatrixTraceMarkContent(96, 100, "B|100|DoWork")},
		{EventID: 96, OffsetNS: 2_200_000, Content: syntheticMatrixTraceMarkContent(96, 100, "E|100")},
		{EventID: 96, OffsetNS: 2_300_000, Content: syntheticMatrixTraceMarkContent(96, 100, "C|100|HeapSize|8192")},
		{EventID: 96, OffsetNS: 2_600_000, Content: syntheticMatrixTraceMarkContent(96, 100, "B|100|FrameActual-123")},
		{EventID: 96, OffsetNS: 2_800_000, Content: syntheticMatrixTraceMarkContent(96, 100, "E|100")},
		{EventID: 97, OffsetNS: 3_200_000, Content: syntheticMatrixIRQEntryContent()},
		{EventID: 98, OffsetNS: 3_300_000, Content: syntheticMatrixIRQExitContent()},
		{EventID: 99, OffsetNS: 3_400_000, Content: syntheticMatrixSoftIRQContent(99)},
		{EventID: 100, OffsetNS: 3_500_000, Content: syntheticMatrixSoftIRQContent(100)},
	}))
	return b.Bytes()
}

func syntheticRootCauseMatrixEventFormat() string {
	var lines []string
	lines = append(lines, syntheticFormatBlock("sched_switch", 90, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("char", "prev_comm[16]", 8, 16, false),
		syntheticField("int", "prev_pid", 24, 4, true),
		syntheticField("int", "prev_prio", 28, 4, true),
		syntheticField("unsigned long", "prev_state", 32, 4, false),
		syntheticField("char", "next_comm[16]", 36, 16, false),
		syntheticField("int", "next_pid", 52, 4, true),
		syntheticField("int", "next_prio", 56, 4, true),
		syntheticField("unsigned int", "expeller_type", 60, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("sched_wakeup", 91, matrixWakeupFields())...)
	lines = append(lines, syntheticFormatBlock("sched_waking", 92, matrixWakeupFields())...)
	lines = append(lines, syntheticFormatBlock("cpu_frequency", 93, matrixCPUFields())...)
	lines = append(lines, syntheticFormatBlock("cpu_idle", 94, matrixCPUFields())...)
	lines = append(lines, syntheticFormatBlock("clock_set_rate", 95, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("char", "name[16]", 8, 16, false),
		syntheticField("unsigned int", "state", 24, 4, false),
		syntheticField("unsigned int", "cpu_id", 28, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("tracing_mark_write", 96, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("__data_loc char[]", "buf", 8, 4, false),
	})...)
	lines = append(lines, syntheticFormatBlock("irq_handler_entry", 97, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("int", "irq", 8, 4, true),
		syntheticField("char", "name[16]", 12, 16, false),
	})...)
	lines = append(lines, syntheticFormatBlock("irq_handler_exit", 98, []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("int", "irq", 8, 4, true),
		syntheticField("int", "ret", 12, 4, true),
	})...)
	lines = append(lines, syntheticFormatBlock("softirq_entry", 99, matrixSoftIRQFields())...)
	lines = append(lines, syntheticFormatBlock("softirq_exit", 100, matrixSoftIRQFields())...)
	return strings.Join(lines, "\n")
}

func matrixWakeupFields() []string {
	return []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("char", "comm[16]", 8, 16, false),
		syntheticField("int", "pid", 24, 4, true),
		syntheticField("int", "prio", 28, 4, true),
		syntheticField("int", "target_cpu", 32, 4, true),
	}
}

func matrixCPUFields() []string {
	return []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned int", "state", 8, 4, false),
		syntheticField("unsigned int", "cpu_id", 12, 4, false),
	}
}

func matrixSoftIRQFields() []string {
	return []string{
		syntheticField("int", "common_pid", 4, 4, true),
		syntheticField("unsigned int", "vec", 8, 4, false),
	}
}

func syntheticMatrixSchedSwitchContent() []byte {
	content := make([]byte, 64)
	binary.LittleEndian.PutUint16(content[0:2], 90)
	binary.LittleEndian.PutUint32(content[4:8], 100)
	copy(content[8:24], []byte("AppMain"))
	binary.LittleEndian.PutUint32(content[24:28], 100)
	binary.LittleEndian.PutUint32(content[28:32], 52)
	binary.LittleEndian.PutUint32(content[32:36], 1)
	copy(content[36:52], []byte("WorkerThread"))
	binary.LittleEndian.PutUint32(content[52:56], 200)
	binary.LittleEndian.PutUint32(content[56:60], 20)
	return content
}

func syntheticMatrixWakeupContent(eventID uint16) []byte {
	content := make([]byte, 36)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], 200)
	copy(content[8:24], []byte("AppMain"))
	binary.LittleEndian.PutUint32(content[24:28], 100)
	binary.LittleEndian.PutUint32(content[28:32], 52)
	binary.LittleEndian.PutUint32(content[32:36], 3)
	return content
}

func syntheticMatrixCPUContent(eventID uint16, state, cpu uint32) []byte {
	content := make([]byte, 16)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[8:12], state)
	binary.LittleEndian.PutUint32(content[12:16], cpu)
	return content
}

func syntheticMatrixClockContent() []byte {
	content := make([]byte, 32)
	binary.LittleEndian.PutUint16(content[0:2], 95)
	copy(content[8:24], []byte("ddr_freq"))
	binary.LittleEndian.PutUint32(content[24:28], 400)
	return content
}

func syntheticMatrixTraceMarkContent(eventID uint16, pid uint32, payload string) []byte {
	data := append([]byte(payload), 0)
	content := make([]byte, 12+len(data))
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[4:8], pid)
	binary.LittleEndian.PutUint32(content[8:12], uint32((len(data)<<16)|12))
	copy(content[12:], data)
	return content
}

func syntheticMatrixIRQEntryContent() []byte {
	content := make([]byte, 28)
	binary.LittleEndian.PutUint16(content[0:2], 97)
	binary.LittleEndian.PutUint32(content[8:12], 42)
	copy(content[12:28], []byte("kirq"))
	return content
}

func syntheticMatrixIRQExitContent() []byte {
	content := make([]byte, 16)
	binary.LittleEndian.PutUint16(content[0:2], 98)
	binary.LittleEndian.PutUint32(content[8:12], 42)
	binary.LittleEndian.PutUint32(content[12:16], 1)
	return content
}

func syntheticMatrixSoftIRQContent(eventID uint16) []byte {
	content := make([]byte, 12)
	binary.LittleEndian.PutUint16(content[0:2], eventID)
	binary.LittleEndian.PutUint32(content[8:12], 9)
	return content
}

func assertRootCauseEvidenceMatrix(t *testing.T, label string, idx *tracequery.Index) {
	t.Helper()
	if idx == nil {
		t.Fatalf("%s index is nil", label)
	}
	minCounts := map[tracequery.EventType]int{
		tracequery.EventSchedSwitch:  1,
		tracequery.EventSchedWakeup:  1,
		tracequery.EventSchedWaking:  1,
		tracequery.EventCPUFrequency: 1,
		tracequery.EventCPUIdle:      1,
		tracequery.EventClockSetRate: 1,
		tracequery.EventTraceMark:    5,
		tracequery.EventIRQ:          2,
		tracequery.EventSoftIRQ:      2,
	}
	counts := map[tracequery.EventType]int{}
	for _, ev := range idx.Events {
		counts[ev.Type]++
	}
	for typ, min := range minCounts {
		if counts[typ] < min {
			t.Fatalf("%s missing %s count >= %d, got %d events=%+v", label, typ, min, counts[typ], idx.Events)
		}
	}
	assertEventExists(t, label, idx, "sched_switch AppMain->WorkerThread", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventSchedSwitch && ev.PrevPID == 100 && ev.NextPID == 200 && ev.PrevPrio == 52 && ev.NextPrio == 20 && strings.Contains(ev.PrevState, "S")
	})
	assertEventExists(t, label, idx, "sched_wakeup AppMain", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventSchedWakeup && ev.PID == 200 && ev.WakeePID == 100 && ev.WakeePrio == 52 && ev.TargetCPU == 3
	})
	assertEventExists(t, label, idx, "sched_waking AppMain", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventSchedWaking && ev.PID == 200 && ev.WakeePID == 100 && ev.WakeePrio == 52 && ev.TargetCPU == 3
	})
	assertEventExists(t, label, idx, "cpu_frequency cpu4", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventCPUFrequency && ev.Frequency == 2200000 && ev.CPUForFieldValid && ev.CPUForField == 4
	})
	assertEventExists(t, label, idx, "cpu_idle cpu4", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventCPUIdle && ev.State == 1 && ev.CPUForFieldValid && ev.CPUForField == 4
	})
	assertEventExists(t, label, idx, "clock_set_rate ddr", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventClockSetRate && ev.ClockName == "ddr_freq" && ev.Frequency == 400
	})
	assertEventExists(t, label, idx, "DoWork begin", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventTraceMark && ev.SpanAction == "B" && ev.SpanPID == 100 && ev.SpanName == "DoWork"
	})
	assertEventExists(t, label, idx, "FrameActual begin", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventTraceMark && ev.SpanAction == "B" && ev.SpanPID == 100 && ev.SpanName == "FrameActual-123"
	})
	assertEventExists(t, label, idx, "HeapSize counter", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventTraceMark && ev.SpanAction == "C" && ev.SpanPID == 100 && ev.SpanName == "HeapSize" && ev.SpanValue == "8192"
	})
	assertEventExists(t, label, idx, "irq entry", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventIRQ && ev.Name == "irq_handler_entry" && ev.IRQID == 42 && ev.IRQName == "kirq"
	})
	assertEventExists(t, label, idx, "irq exit", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventIRQ && ev.Name == "irq_handler_exit" && ev.IRQID == 42
	})
	assertEventExists(t, label, idx, "softirq entry", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventSoftIRQ && ev.Name == "softirq_entry" && ev.IRQID == 9
	})
	assertEventExists(t, label, idx, "softirq exit", func(ev tracequery.Event) bool {
		return ev.Type == tracequery.EventSoftIRQ && ev.Name == "softirq_exit" && ev.IRQID == 9
	})
}

func assertEventExists(t *testing.T, label string, idx *tracequery.Index, want string, match func(tracequery.Event) bool) {
	t.Helper()
	for _, ev := range idx.Events {
		if match(ev) {
			return
		}
	}
	t.Fatalf("%s missing %s in events: %+v", label, want, idx.Events)
}

func assertWakeupSemanticParity(t *testing.T, builtin, sql tracequery.Event) {
	t.Helper()
	if builtin.WakeePID != sql.WakeePID ||
		builtin.WakeeComm != sql.WakeeComm ||
		builtin.WakeePrio != sql.WakeePrio ||
		builtin.TargetCPU != sql.TargetCPU ||
		builtin.PID != sql.PID ||
		builtin.CPU != sql.CPU {
		t.Fatalf("wakeup semantic mismatch:\nbuiltin=%+v\nsql=%+v", builtin, sql)
	}
	if builtin.WakeePID != 36379 || builtin.WakeeComm != "com.tencent.mm" ||
		builtin.WakeePrio != 53 || builtin.TargetCPU != 0 {
		t.Fatalf("synthetic wakeup guard drifted: %+v", builtin)
	}
}

func hasArtifact(artifacts []Artifact, typ string) bool {
	for _, artifact := range artifacts {
		if artifact.Type == typ {
			return true
		}
	}
	return false
}

func hasTraceDecision(decisions []TraceProviderDecision, name string, succeeded bool) bool {
	for _, decision := range decisions {
		if decision.ProviderName == name && decision.Succeeded == succeeded {
			return true
		}
	}
	return false
}

func hasTraceDecisionReason(decisions []TraceProviderDecision, name, reason string) bool {
	for _, decision := range decisions {
		if decision.ProviderName == name && decision.Reason == reason {
			return true
		}
	}
	return false
}
