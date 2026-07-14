//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd

package hitraceconv

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

type traceStreamerConversionOutcome struct {
	result Result
	err    error
}

func TestTraceStreamerProviderConsumesPrivateSnapshotAcrossPublicSymlinkABA(t *testing.T) {
	dir := t.TempDir()
	sourceA := filepath.Join(dir, "source-a.htrace")
	sourceB := filepath.Join(dir, "source-b.htrace")
	input := filepath.Join(dir, "hiprofiler_data_20260714_010203_123.htrace")
	original := []byte("authoritative-generation-A")
	if err := os.WriteFile(sourceA, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sourceB, bytes.Repeat([]byte("B"), len(original)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sourceA, input); err != nil {
		t.Skipf("symlink fixture unavailable: %v", err)
	}

	fixtureDB := createTraceDBFixture(t, traceStreamerIntegrationDBStatements())
	tool := writeFakeTraceStreamer(t, dir, 0)
	ready := filepath.Join(dir, "tool.ready")
	consumedReady := filepath.Join(dir, "consumed.ready")
	consumed := filepath.Join(dir, "consumed.bin")
	release := filepath.Join(dir, "release.fifo")
	finish := filepath.Join(dir, "finish.fifo")
	for _, fifo := range []string{release, finish} {
		if err := unix.Mkfifo(fifo, 0o600); err != nil {
			t.Skipf("FIFO fixture unavailable: %v", err)
		}
	}
	argsLog := filepath.Join(dir, "args.log")
	t.Setenv("TRACE_STREAMER_FIXTURE_DB", fixtureDB)
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_READY", ready)
	t.Setenv("TRACE_STREAMER_RELEASE_FIFO", release)
	t.Setenv("TRACE_STREAMER_CONSUMED_INPUT", consumed)
	t.Setenv("TRACE_STREAMER_CONSUMED_READY", consumedReady)
	t.Setenv("TRACE_STREAMER_FINISH_FIFO", finish)
	output := filepath.Join(dir, "out.systrace")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan traceStreamerConversionOutcome, 1)
	go func() {
		result, err := ConvertFile(ctx, Options{
			InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer,
			TraceStreamerPath: tool,
		})
		done <- traceStreamerConversionOutcome{result: result, err: err}
	}()
	waitForTraceStreamerTestPath(t, ready)
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sourceB, input); err != nil {
		t.Fatal(err)
	}
	releaseTraceStreamerFIFO(t, release)
	waitForTraceStreamerTestPath(t, consumedReady)
	if err := os.Remove(input); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(sourceA, input); err != nil {
		t.Fatal(err)
	}
	releaseTraceStreamerFIFO(t, finish)

	outcome := waitForTraceStreamerConversion(t, done)
	if outcome.err != nil {
		t.Fatalf("trace_streamer symlink A->B->A conversion failed: %v", outcome.err)
	}
	if outcome.result.InputPath != input || outcome.result.OutputPath != output || outcome.result.EventsWritten == 0 {
		t.Fatalf("trace_streamer symlink conversion result drifted: %+v", outcome.result)
	}
	got, err := os.ReadFile(consumed)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, original) {
		t.Fatalf("trace_streamer consumed public replacement instead of immutable snapshot: got=%q want=%q", got, original)
	}
	args := traceStreamerTestArgs(t, argsLog)
	if args[0] == input || filepath.Base(args[0]) != filepath.Base(input) || filepath.Dir(args[0]) != filepath.Dir(args[2]) {
		t.Fatalf("trace_streamer snapshot argv lost exact private-name binding: %#v", args)
	}
	for _, path := range []string{args[0], args[2], filepath.Dir(args[0])} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("successful trace_streamer conversion leaked private path %q: %v", path, statErr)
		}
	}
}

func TestTraceStreamerProviderSourceMutationIsHardAndDominatesChildFailure(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.sys")
	original := syntheticBinaryHitrace(t)
	if err := os.WriteFile(input, original, 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.Stat(input)
	if err != nil {
		t.Fatal(err)
	}
	tool := writeFakeTraceStreamer(t, dir, 7)
	ready := filepath.Join(dir, "tool.ready")
	release := filepath.Join(dir, "release.fifo")
	if err := unix.Mkfifo(release, 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	argsLog := filepath.Join(dir, "args.log")
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_ECHO_ARGS", "1")
	t.Setenv("TRACE_STREAMER_READY", ready)
	t.Setenv("TRACE_STREAMER_RELEASE_FIFO", release)
	output := filepath.Join(dir, "out.systrace")
	var progress []ProgressEvent
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan traceStreamerConversionOutcome, 1)
	go func() {
		result, err := ConvertFile(ctx, Options{
			InputPath: input, OutputPath: output, TraceStreamerPath: tool,
			Progress: func(event ProgressEvent) {
				progress = append(progress, event)
			},
		})
		done <- traceStreamerConversionOutcome{result: result, err: err}
	}()
	waitForTraceStreamerTestPath(t, ready)
	mutated := append([]byte(nil), original...)
	mutated[len(mutated)/2] ^= 0xff
	if err := os.WriteFile(input, mutated, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(input, before.ModTime(), before.ModTime()); err != nil {
		t.Fatal(err)
	}
	releaseTraceStreamerFIFO(t, release)

	outcome := waitForTraceStreamerConversion(t, done)
	if outcome.err == nil {
		t.Fatal("same-inode source mutation unexpectedly fell back to the built-in parser")
	}
	var inputErr *ConversionInputError
	if !errors.As(outcome.err, &inputErr) || inputErr.Code != ConversionInputCodeGenerationChanged ||
		inputErr.Stage != conversionInputStageExternalTool.String() || inputErr.Path != input {
		t.Fatalf("source mutation lost typed external-tool generation failure: %v", outcome.err)
	}
	if !strings.Contains(outcome.err.Error(), "exit status 7") {
		t.Fatalf("hard source-generation error lost child failure evidence: %v", outcome.err)
	}
	if strings.Contains(outcome.err.Error(), "codrax-trace-streamer-") {
		t.Fatalf("hard trace_streamer failure exposed private staging path: %v", outcome.err)
	}
	var boundaryFailed bool
	for _, event := range progress {
		if event.Stage == "trace_streamer_export" && event.Status == ProgressStatusComplete {
			t.Fatalf("source generation drift was reported as a successful trace_streamer export: %+v", progress)
		}
		if event.Stage == "trace_streamer_export" && event.Status == ProgressStatusFailed &&
			event.Message == "trace_streamer command boundary rejected" {
			boundaryFailed = true
		}
	}
	if !boundaryFailed {
		t.Fatalf("source generation drift lost terminal boundary-failure progress: %+v", progress)
	}
	if outcome.result.InputPath != "" || outcome.result.OutputPath != "" || outcome.result.BundlePath != "" || len(outcome.result.Artifacts) != 0 {
		t.Fatalf("hard trace_streamer generation failure published a partial result: %+v", outcome.result)
	}
	for _, path := range []string{output, traceSidecarBase(input, output) + ".tracebundle.json"} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("hard trace_streamer generation failure published %q: %v", path, statErr)
		}
	}
	args := traceStreamerTestArgs(t, argsLog)
	for _, path := range []string{args[0], args[2], filepath.Dir(args[0])} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("hard trace_streamer generation failure leaked private path %q: %v", path, statErr)
		}
	}
}

func TestTraceStreamerProviderCancellationClosesLeaseAndPublishesNothing(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "capture.htrace")
	if err := os.WriteFile(input, []byte("immutable input"), 0o600); err != nil {
		t.Fatal(err)
	}
	tool := writeFakeTraceStreamer(t, dir, 0)
	ready := filepath.Join(dir, "tool.ready")
	release := filepath.Join(dir, "release.fifo")
	if err := unix.Mkfifo(release, 0o600); err != nil {
		t.Skipf("FIFO fixture unavailable: %v", err)
	}
	argsLog := filepath.Join(dir, "args.log")
	t.Setenv("TRACE_STREAMER_ARGS_LOG", argsLog)
	t.Setenv("TRACE_STREAMER_READY", ready)
	t.Setenv("TRACE_STREAMER_RELEASE_FIFO", release)
	output := filepath.Join(dir, "out.systrace")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan traceStreamerConversionOutcome, 1)
	go func() {
		result, err := ConvertFile(ctx, Options{
			InputPath: input, OutputPath: output, TraceEngine: traceEngineTraceStreamer,
			TraceStreamerPath: tool,
		})
		done <- traceStreamerConversionOutcome{result: result, err: err}
	}()
	waitForTraceStreamerTestPath(t, ready)
	cancel()
	outcome := waitForTraceStreamerConversion(t, done)
	if !errors.Is(outcome.err, context.Canceled) {
		t.Fatalf("trace_streamer cancellation lost context identity: %v", outcome.err)
	}
	if outcome.result.InputPath != "" || outcome.result.OutputPath != "" || outcome.result.BundlePath != "" {
		t.Fatalf("cancelled trace_streamer conversion published a result: %+v", outcome.result)
	}
	args := traceStreamerTestArgs(t, argsLog)
	for _, path := range []string{args[0], args[2], filepath.Dir(args[0]), output, traceSidecarBase(input, output) + ".tracebundle.json"} {
		if _, statErr := os.Lstat(path); !os.IsNotExist(statErr) {
			t.Fatalf("cancelled trace_streamer conversion leaked %q: %v", path, statErr)
		}
	}
}

func waitForTraceStreamerTestPath(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Lstat(path); err == nil {
			return
		} else if !os.IsNotExist(err) {
			t.Fatalf("wait for trace_streamer fixture path %q: %v", path, err)
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for trace_streamer fixture path %q", path)
}

func releaseTraceStreamerFIFO(t *testing.T, path string) {
	t.Helper()
	done := make(chan error, 1)
	go func() {
		file, err := os.OpenFile(path, os.O_WRONLY, 0)
		if err == nil {
			_, err = file.WriteString("continue\n")
			err = traceDBJoinPreservingSingle(err, file.Close())
		}
		done <- err
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("release trace_streamer FIFO %q: %v", path, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("timed out releasing trace_streamer FIFO %q", path)
	}
}

func waitForTraceStreamerConversion(t *testing.T, done <-chan traceStreamerConversionOutcome) traceStreamerConversionOutcome {
	t.Helper()
	select {
	case outcome := <-done:
		return outcome
	case <-time.After(20 * time.Second):
		t.Fatal("timed out waiting for trace_streamer conversion")
		return traceStreamerConversionOutcome{}
	}
}

func traceStreamerTestArgs(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	args := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(args) < 3 || args[1] != "-e" {
		t.Fatalf("malformed trace_streamer fixture argv: %#v", args)
	}
	return args
}
