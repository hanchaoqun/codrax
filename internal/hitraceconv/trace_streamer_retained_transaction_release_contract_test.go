//go:build linux || darwin

package hitraceconv

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestReleaseRetainedTraceDBBundleRaceRollsBackOwnedPublications(t *testing.T) {
	for _, tc := range releaseRetainedDBCases() {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			tool := writeFakeTraceStreamer(t, dir, 0)
			argsLog := filepath.Join(dir, "args.log")
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)

			opts := Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: tool}
			finalDB := tc.configure(&opts, input, output, dir)
			bundlePath := traceSidecarBase(input, output) + ".tracebundle.json"
			const externalBundle = "external bundle owner\n"
			var once sync.Once
			var raceErr error
			opts.Progress = func(event ProgressEvent) {
				if event.Stage == "trace_db_normalize" && event.Status == ProgressStatusComplete {
					once.Do(func() { raceErr = os.WriteFile(bundlePath, []byte(externalBundle), 0o600) })
				}
			}

			_, err := ConvertFile(context.Background(), opts)
			if raceErr != nil {
				t.Fatalf("create external bundle race: %v", raceErr)
			}
			if err == nil || !strings.Contains(err.Error(), "output file already exists") {
				t.Fatalf("bundle publication race did not hard-fail: %v", err)
			}
			assertReleasePathAbsent(t, finalDB)
			assertReleasePathAbsent(t, finalDB+".ohos.ts")
			assertReleasePathAbsent(t, output)
			body, readErr := os.ReadFile(bundlePath)
			if readErr != nil || string(body) != externalBundle {
				t.Fatalf("external racing bundle was changed: body=%q err=%v", body, readErr)
			}
			assertReleaseTraceStreamerStagingGone(t, argsLog)
		})
	}
}

func TestReleaseRetainedTraceDBFinalReplacementFailsCommitAndPreservesExternalOwner(t *testing.T) {
	for _, replaced := range []struct {
		name   string
		suffix string
	}{
		{name: "db"},
		{name: "companion", suffix: ".ohos.ts"},
	} {
		t.Run(replaced.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			finalDB := filepath.Join(dir, "operator.trace.db")
			if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			tool := writeFakeTraceStreamer(t, dir, 0)
			argsLog := filepath.Join(dir, "args.log")
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)

			victim := finalDB + replaced.suffix
			other := finalDB
			if replaced.suffix == "" {
				other = finalDB + ".ohos.ts"
			}
			external := []byte("external replacement owner\n")
			var once sync.Once
			var raceErr error
			opts := Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer,
				TraceStreamerPath: tool, TraceDBOutputPath: finalDB,
			}
			opts.Progress = func(event ProgressEvent) {
				if event.Stage == "trace_db_normalize" && event.Status == ProgressStatusComplete {
					once.Do(func() {
						displaced := victim + ".displaced-owned"
						if err := os.Rename(victim, displaced); err != nil {
							raceErr = err
							return
						}
						raceErr = os.WriteFile(victim, external, 0o600)
					})
				}
			}
			_, err := ConvertFile(context.Background(), opts)
			if raceErr != nil {
				t.Fatalf("replace retained pair member: %v", raceErr)
			}
			if err == nil || !strings.Contains(err.Error(), "changed") {
				t.Fatalf("retained final replacement passed commit or was misclassified: %v", err)
			}
			body, readErr := os.ReadFile(victim)
			if readErr != nil || string(body) != string(external) {
				t.Fatalf("external replacement owner was changed: body=%q err=%v", body, readErr)
			}
			assertReleasePathAbsent(t, other)
			assertReleasePathAbsent(t, output)
			assertReleasePathAbsent(t, traceSidecarBase(input, output)+".tracebundle.json")
			assertReleaseTraceStreamerStagingGone(t, argsLog)
		})
	}
}

func TestReleaseRetainedTraceDBCancellationAfterNormalizeRollsBackPair(t *testing.T) {
	for _, tc := range releaseRetainedDBCases() {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			tool := writeFakeTraceStreamer(t, dir, 0)
			argsLog := filepath.Join(dir, "args.log")
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
			t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			opts := Options{InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: tool}
			finalDB := tc.configure(&opts, input, output, dir)
			opts.Progress = func(event ProgressEvent) {
				if event.Stage == "trace_db_normalize" && event.Status == ProgressStatusComplete {
					cancel()
				}
			}
			_, err := ConvertFile(ctx, opts)
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("post-normalize cancellation identity lost: %T %v", err, err)
			}
			assertReleasePathAbsent(t, finalDB)
			assertReleasePathAbsent(t, finalDB+".ohos.ts")
			assertReleasePathAbsent(t, output)
			assertReleasePathAbsent(t, traceSidecarBase(input, output)+".tracebundle.json")
			assertReleaseTraceStreamerStagingGone(t, argsLog)
		})
	}
}

func TestReleaseRetainedTraceDBNoRowsIsTypedCommittedPartialResult(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	output := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, []string{"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)"})
	tool := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer,
		TraceStreamerPath: tool, KeepTraceDB: true,
	})
	if err != nil {
		t.Fatalf("typed partial result should commit with nil error: %v", err)
	}
	finalDB := traceSidecarBase(input, output) + ".trace.db"
	if result.OutputPath != "" || result.EventsWritten != 0 || result.BundlePath == "" {
		t.Fatalf("no-row result falsely claimed systrace or omitted bundle: %+v", result)
	}
	artifact, ok := releaseArtifactByType(result.Artifacts, ArtifactTraceDB)
	if !ok || artifact.Path != finalDB || !releaseContains(artifact.Caveats, "timestamp_companion="+finalDB+".ohos.ts") {
		t.Fatalf("committed partial DB artifact malformed: %+v", result.Artifacts)
	}
	decision, ok := releaseTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer)
	if !ok || !decision.Attempted || decision.Succeeded || decision.Reason != "trace_db_no_rows" || decision.DBPath != finalDB {
		t.Fatalf("committed partial provider decision malformed: %+v", result.TraceDecisions)
	}
	if _, err := os.Stat(finalDB); err != nil {
		t.Fatalf("committed partial DB missing: %v", err)
	}
	if _, err := os.Stat(finalDB + ".ohos.ts"); err != nil {
		t.Fatalf("committed partial timestamp companion missing: %v", err)
	}
	meta := releaseReadTraceBundle(t, result.BundlePath)
	bundleDB, ok := releaseArtifactByType(meta.Artifacts, ArtifactTraceDB)
	if !ok || bundleDB.Path != finalDB {
		t.Fatalf("partial tracebundle lost retained DB provenance: %+v", meta.Artifacts)
	}
	bundleDecision, ok := releaseTraceDecision(meta.TraceDecisions, traceProviderNameTraceStreamer)
	if !ok || bundleDecision.Reason != "trace_db_no_rows" || bundleDecision.DBPath != finalDB {
		t.Fatalf("partial tracebundle lost typed decision: %+v", meta.TraceDecisions)
	}
}

func TestReleaseRetainedTraceDBNormalizeFailureIsTypedCommittedPartialResult(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	output := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer,
		TraceStreamerPath: tool, KeepTraceDB: true,
	})
	if err != nil {
		t.Fatalf("retained normalize failure should commit a typed partial result: %v", err)
	}
	finalDB := traceSidecarBase(input, output) + ".trace.db"
	if result.OutputPath != "" || result.EventsWritten != 0 || result.BundlePath == "" {
		t.Fatalf("normalize failure falsely claimed systrace or omitted bundle: %+v", result)
	}
	artifact, ok := releaseArtifactByType(result.Artifacts, ArtifactTraceDB)
	if !ok || artifact.Path != finalDB || !releaseContains(artifact.Caveats, "timestamp_companion="+finalDB+".ohos.ts") {
		t.Fatalf("retained normalize-failure DB artifact malformed: %+v", result.Artifacts)
	}
	decision, ok := releaseTraceDecision(result.TraceDecisions, traceProviderNameTraceStreamer)
	if !ok || decision.Succeeded || decision.Reason != "trace_db_normalize_failed" || decision.DBPath != finalDB {
		t.Fatalf("retained normalize-failure decision malformed: %+v", result.TraceDecisions)
	}
	if _, err := os.Stat(finalDB); err != nil {
		t.Fatalf("retained normalize-failure DB missing: %v", err)
	}
	if _, err := os.Stat(finalDB + ".ohos.ts"); err != nil {
		t.Fatalf("retained normalize-failure companion missing: %v", err)
	}
	meta := releaseReadTraceBundle(t, result.BundlePath)
	bundleDB, ok := releaseArtifactByType(meta.Artifacts, ArtifactTraceDB)
	if !ok || bundleDB.Path != finalDB {
		t.Fatalf("normalize-failure tracebundle lost retained DB provenance: %+v", meta.Artifacts)
	}
}

func TestReleaseRetainedNoRowsCompositeNamesRollbackNotRetention(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "invalid.sys")
	output := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(input, releaseBuiltinHeader(0xbeef, 54, 99), 0o600); err != nil {
		t.Fatal(err)
	}
	fixtureDB := createTraceDBFixture(t, []string{"CREATE TABLE trace_range (start_ts INT)", "INSERT INTO trace_range VALUES (0)"})
	tool := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	t.Setenv("TRACE_STREAMER_CREATE_OHOS_TS", "1")
	finalDB := traceSidecarBase(input, output) + ".trace.db"

	_, err := ConvertFile(context.Background(), Options{InputPath: input, OutputPath: output, TraceStreamerPath: tool, KeepTraceDB: true})
	if err == nil {
		t.Fatal("no-row SQL plus strict built-in rejection unexpectedly succeeded")
	}
	var composite *TraceProviderFallbackError
	if !errors.As(err, &composite) || composite.RolledBackDB != finalDB || composite.FirstDecision.DBPath != "" {
		t.Fatalf("rollback provenance was not sanitized into the composite: %+v err=%v", composite, err)
	}
	if !strings.Contains(err.Error(), "rolled_back_db=") || strings.Contains(err.Error(), "retained_db") {
		t.Fatalf("hard failure mislabeled rolled-back DB as retained: %v", err)
	}
	var decodeErr *BuiltinSysDecodeError
	if !errors.As(err, &decodeErr) || decodeErr.Code != builtinSysDecodeInvalidMagic {
		t.Fatalf("hard failure lost strict built-in cause: %+v", decodeErr)
	}
	assertReleasePathAbsent(t, finalDB)
	assertReleasePathAbsent(t, finalDB+".ohos.ts")
	assertReleasePathAbsent(t, output)
}

type releaseRetainedDBCase struct {
	name      string
	configure func(*Options, string, string, string) string
}

func releaseRetainedDBCases() []releaseRetainedDBCase {
	return []releaseRetainedDBCase{
		{name: "keep-derived", configure: func(opts *Options, input, output, _ string) string {
			opts.KeepTraceDB = true
			return traceSidecarBase(input, output) + ".trace.db"
		}},
		{name: "explicit-path", configure: func(opts *Options, _, _, dir string) string {
			path := filepath.Join(dir, "operator.trace.db")
			opts.TraceDBOutputPath = path
			return path
		}},
	}
}

func assertReleasePathAbsent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !os.IsNotExist(err) {
		t.Fatalf("conversion-owned path survived rollback: %s err=%v", path, err)
	}
}

func assertReleaseTraceStreamerStagingGone(t *testing.T, argsLog string) {
	t.Helper()
	stagingDB := releaseTraceStreamerDBArg(t, argsLog)
	assertReleasePathAbsent(t, stagingDB)
	assertReleasePathAbsent(t, stagingDB+".ohos.ts")
	assertReleasePathAbsent(t, filepath.Dir(stagingDB))
}
