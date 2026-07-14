//go:build !windows

package hitraceconv

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestTraceStreamerProviderRejectsInitialSQLiteAuxiliaryState(t *testing.T) {
	for _, suffix := range []string{"-journal", "-wal", "-shm"} {
		t.Run(strings.TrimPrefix(suffix, "-"), func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixture := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			argsLog := filepath.Join(dir, "args.log")
			tool := writeFakeTraceStreamer(t, dir, 0)
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixture)
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
			t.Setenv("TRACE_STREAMER_CREATE_SQLITE_AUX", suffix)

			_, err := ConvertFile(context.Background(), Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: tool,
			})
			if err == nil || !strings.Contains(err.Error(), "SQLite auxiliary state") {
				t.Fatalf("initial SQLite auxiliary %q did not fail explicitly: %v", suffix, err)
			}
			assertReleasePathAbsent(t, output)
			assertReleasePathAbsent(t, traceSidecarBase(input, output)+".tracebundle.json")
			assertReleaseTraceStreamerStagingGone(t, argsLog)
		})
	}
}

func TestTraceStreamerAutoMayFallbackFromInitialSQLiteAuxiliaryState(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	output := filepath.Join(dir, "capture.systrace")
	if err := os.WriteFile(input, syntheticBinaryHitrace(t), 0o600); err != nil {
		t.Fatal(err)
	}
	fixture := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	argsLog := filepath.Join(dir, "args.log")
	tool := writeFakeTraceStreamer(t, dir, 0)
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixture)
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_CREATE_SQLITE_AUX", "-wal")

	result, err := ConvertFile(context.Background(), Options{
		InputPath: input, OutputPath: output, TraceStreamerPath: tool,
	})
	if err != nil {
		t.Fatalf("auto route did not fall back from stable producer-shape failure: %v", err)
	}
	if !hasTraceDecisionReason(result.TraceDecisions, traceProviderNameTraceStreamer, "trace_db_auxiliary_state") ||
		!hasTraceDecision(result.TraceDecisions, traceProviderNameBuiltinSys, true) {
		t.Fatalf("auto auxiliary fallback decisions drifted: %+v", result.TraceDecisions)
	}
	assertReleaseTraceStreamerStagingGone(t, argsLog)
}

func TestTraceStreamerProviderHardFailsLateOutputGenerationDrift(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate func(string, []byte) error
		want   string
	}{
		{
			name: "wal-appears",
			mutate: func(dbPath string, _ []byte) error {
				return os.WriteFile(dbPath+"-wal", []byte("late wal"), 0o600)
			},
			want: "SQLite auxiliary state",
		},
		{
			name: "companion-appears",
			mutate: func(dbPath string, _ []byte) error {
				return os.WriteFile(dbPath+".ohos.ts", []byte("late timestamp"), 0o600)
			},
			want: "appeared after output adoption",
		},
		{
			name: "main-path-replaced",
			mutate: func(dbPath string, replacement []byte) error {
				if err := os.Rename(dbPath, dbPath+".held"); err != nil {
					return err
				}
				return os.WriteFile(dbPath, replacement, 0o600)
			},
			want: "identity changed",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			input := filepath.Join(dir, "capture.sys")
			output := filepath.Join(dir, "capture.systrace")
			if err := os.WriteFile(input, []byte("trace payload"), 0o600); err != nil {
				t.Fatal(err)
			}
			fixture := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
			replacementPath := createTraceDBFixture(t, []string{
				"CREATE TABLE trace_range (start_ts INT)",
				"INSERT INTO trace_range VALUES (0)",
			})
			replacement, err := os.ReadFile(replacementPath)
			if err != nil {
				t.Fatal(err)
			}
			argsLog := filepath.Join(dir, "args.log")
			tool := writeFakeTraceStreamer(t, dir, 0)
			t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixture)
			t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
			var once sync.Once
			var mutationErr error
			opts := Options{
				InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer, TraceStreamerPath: tool,
			}
			opts.Progress = func(event ProgressEvent) {
				if event.Stage == "trace_db_normalize" && event.Status == ProgressStatusStarted {
					once.Do(func() {
						args := traceStreamerTestArgs(t, argsLog)
						mutationErr = test.mutate(args[2], replacement)
					})
				}
			}
			_, err = ConvertFile(context.Background(), opts)
			if mutationErr != nil {
				t.Fatalf("install deterministic output drift: %v", mutationErr)
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("late output drift was not a hard integrity failure: want=%q err=%v", test.want, err)
			}
			assertReleasePathAbsent(t, output)
			assertReleasePathAbsent(t, traceSidecarBase(input, output)+".tracebundle.json")
			assertReleaseTraceStreamerStagingGone(t, argsLog)
		})
	}
}
