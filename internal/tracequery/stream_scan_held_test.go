package tracequery

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeHeldStreamFixture(t *testing.T, body string) (*os.File, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "held.systrace")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = file.Close() })
	return file, path
}

func heldStreamWakeupLine() string {
	return "waker-10 (10) [001] .... 1.000000: sched_wakeup: comm=app pid=20 prio=53 target_cpu=002\n"
}

func TestStreamScanHeldFileUsesSingleO1ParserLoop(t *testing.T) {
	body := heldStreamWakeupLine() +
		"app-20 (20) [002] .... 1.000001: tracing_mark_write: C|20|queue_depth|3\n"
	file, path := writeHeldStreamFixture(t, body)
	callbacks := 0
	idx, err := StreamScanHeldFile(context.Background(), file, "customer-output.systrace", TraceFlavorAuto, 1<<20, func(ev Event) bool {
		callbacks++
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if idx.Path != "customer-output.systrace" || idx.Size != int64(len(body)) || idx.LineCount != 2 ||
		idx.ScannedLineCount != 2 || idx.ParsedKnown != 2 || callbacks != 2 || len(idx.Events) != 0 ||
		idx.UnparsedLines != 0 || idx.ParseLinePanics != 0 || idx.ClockRegressions != 0 {
		t.Fatalf("held scan accounting drifted: path=%q source=%q callbacks=%d idx=%+v", idx.Path, path, callbacks, idx)
	}
}

func TestStreamScanHeldFileLineBudgetAndCanonicalTerminator(t *testing.T) {
	exact := strings.Repeat("x", 1024) + "\n"
	file, _ := writeHeldStreamFixture(t, exact)
	idx, err := StreamScanHeldFile(context.Background(), file, "exact.systrace", TraceFlavorAuto, 1024, func(Event) bool { return true })
	if err != nil {
		t.Fatalf("exact line budget rejected: %v", err)
	}
	if idx.LineCount != 1 || idx.UnparsedLines != 1 {
		t.Fatalf("exact line was not scanned atomically: %+v", idx)
	}
	if _, err := StreamScanHeldFile(context.Background(), file, "over.systrace", TraceFlavorAuto, 1023, func(Event) bool { return true }); err == nil ||
		!strings.Contains(err.Error(), "physical line exceeds 1023 bytes") {
		t.Fatalf("cap+1 line did not fail bounded: %v", err)
	} else if _, joined := err.(interface{ Unwrap() []error }); joined {
		t.Fatalf("single scan failure was misrepresented as a joined authority failure: %T %v", err, err)
	}

	for name, body := range map[string]string{
		"missing-lf": strings.TrimSuffix(heldStreamWakeupLine(), "\n"),
		"crlf":       strings.TrimSuffix(heldStreamWakeupLine(), "\n") + "\r\n",
	} {
		t.Run(name, func(t *testing.T) {
			file, _ := writeHeldStreamFixture(t, body)
			if _, err := StreamScanHeldFile(context.Background(), file, name+".systrace", TraceFlavorAuto, 1<<20, func(Event) bool { return true }); err == nil ||
				!strings.Contains(err.Error(), "non-canonical generated line terminator") {
				t.Fatalf("non-canonical generated line was accepted: %v", err)
			}
		})
	}
}

func TestStreamScanHeldFileExceedsIndexBudgetWithoutMaterializingEvents(t *testing.T) {
	const rows = 250_001
	path := filepath.Join(t.TempDir(), "large.systrace")
	out, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	writer := bufio.NewWriterSize(out, 256*1024)
	line := heldStreamWakeupLine()
	for row := 0; row < rows; row++ {
		if _, err := writer.WriteString(line); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Flush(); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	callbacks := 0
	idx, err := StreamScanHeldFile(context.Background(), file, "large.systrace", TraceFlavorAuto, 1<<20, func(Event) bool {
		callbacks++
		return true
	})
	if err != nil {
		t.Fatal(err)
	}
	if callbacks != rows || idx.ParsedKnown != rows || idx.LineCount != rows || len(idx.Events) != 0 {
		t.Fatalf("large held scan hit an index budget or retained events: callbacks=%d idx=%+v", callbacks, idx)
	}
}

func TestStreamScanHeldFileRejectsSameInodeRestoredMtimeMutation(t *testing.T) {
	body := heldStreamWakeupLine() + heldStreamWakeupLine()
	file, path := writeHeldStreamFixture(t, body)
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	mutated := false
	_, err = StreamScanHeldFile(context.Background(), file, "mutated.systrace", TraceFlavorAuto, 1<<20, func(Event) bool {
		if !mutated {
			mutated = true
			if _, writeErr := file.WriteAt([]byte{'X'}, 0); writeErr != nil {
				t.Fatalf("mutate held file: %v", writeErr)
			}
			if _, writeErr := file.WriteAt([]byte{body[0]}, 0); writeErr != nil {
				t.Fatalf("restore held bytes: %v", writeErr)
			}
			if chtimesErr := os.Chtimes(path, info.ModTime(), info.ModTime()); chtimesErr != nil {
				t.Fatalf("restore mtime: %v", chtimesErr)
			}
		}
		return true
	})
	if err == nil || !strings.Contains(err.Error(), "source generation changed during validation") {
		t.Fatalf("same-inode restored-mtime mutation escaped held generation gate: %v", err)
	}
}

func TestStreamScanHeldFilePreservesScanAndGenerationFailures(t *testing.T) {
	const limit = 128
	body := heldStreamWakeupLine() + strings.Repeat("z", limit+1) + "\n"
	file, path := writeHeldStreamFixture(t, body)
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}
	mutated := false
	_, err = StreamScanHeldFile(context.Background(), file, "double-fault.systrace", TraceFlavorAuto, limit, func(Event) bool {
		if !mutated {
			mutated = true
			if _, writeErr := file.WriteAt([]byte{'X'}, 0); writeErr != nil {
				t.Fatalf("mutate held file: %v", writeErr)
			}
			if _, writeErr := file.WriteAt([]byte{body[0]}, 0); writeErr != nil {
				t.Fatalf("restore held bytes: %v", writeErr)
			}
			if chtimesErr := os.Chtimes(path, info.ModTime(), info.ModTime()); chtimesErr != nil {
				t.Fatalf("restore mtime: %v", chtimesErr)
			}
		}
		return true
	})
	if err == nil || !strings.Contains(err.Error(), "physical line exceeds 128 bytes") ||
		!strings.Contains(err.Error(), "source generation changed during validation") {
		t.Fatalf("scan+generation error graph was not preserved: %v", err)
	}
	if _, joined := err.(interface{ Unwrap() []error }); !joined {
		t.Fatalf("genuine scan+generation double fault lost its joined error topology: %T %v", err, err)
	}
}

func TestStreamScanHeldFileDoesNotReadConcurrentAppend(t *testing.T) {
	body := heldStreamWakeupLine()
	file, _ := writeHeldStreamFixture(t, body)
	callbacks := 0
	_, err := StreamScanHeldFile(context.Background(), file, "append.systrace", TraceFlavorAuto, 1<<20, func(Event) bool {
		callbacks++
		if callbacks == 1 {
			if _, writeErr := file.WriteAt([]byte(heldStreamWakeupLine()), int64(len(body))); writeErr != nil {
				t.Fatalf("append held file: %v", writeErr)
			}
		}
		return true
	})
	if callbacks != 1 {
		t.Fatalf("initial-size SectionReader consumed concurrent append: callbacks=%d", callbacks)
	}
	if err == nil || !strings.Contains(err.Error(), "source generation changed during validation") {
		t.Fatalf("concurrent append escaped generation gate: %v", err)
	}
}

func TestStreamScanHeldFileCancellationAfterProgress(t *testing.T) {
	file, _ := writeHeldStreamFixture(t, strings.Repeat(heldStreamWakeupLine(), 32))
	ctx, cancel := context.WithCancel(context.Background())
	callbacks := 0
	_, err := StreamScanHeldFile(ctx, file, "cancel-progress.systrace", TraceFlavorAuto, 1<<20, func(Event) bool {
		callbacks++
		if callbacks == 3 {
			cancel()
		}
		return true
	})
	if callbacks != 3 || !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-scan cancellation drifted: callbacks=%d err=%v", callbacks, err)
	}
}

func TestStreamScanHeldFileCancellationAndArgumentGates(t *testing.T) {
	file, _ := writeHeldStreamFixture(t, heldStreamWakeupLine())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := StreamScanHeldFile(ctx, file, "cancel.systrace", TraceFlavorAuto, 1<<20, func(Event) bool { return true }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation identity lost: %v", err)
	}
	for _, test := range []struct {
		name string
		file *os.File
		path string
		cap  int
		fn   func(Event) bool
	}{
		{name: "nil-file", path: "x", cap: 1, fn: func(Event) bool { return true }},
		{name: "empty-path", file: file, cap: 1, fn: func(Event) bool { return true }},
		{name: "zero-cap", file: file, path: "x", fn: func(Event) bool { return true }},
		{name: "oversized-cap", file: file, path: "x", cap: 16<<20 + 1, fn: func(Event) bool { return true }},
		{name: "nil-callback", file: file, path: "x", cap: 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := StreamScanHeldFile(context.Background(), test.file, test.path, TraceFlavorAuto, test.cap, test.fn); err == nil {
				t.Fatal("invalid held stream arguments were accepted")
			}
		})
	}
}

func TestReadStreamScanPhysicalLineBudgetDoesNotAllocatePastLimit(t *testing.T) {
	reader := bufio.NewReaderSize(strings.NewReader(strings.Repeat("z", 4096)), 64)
	if _, err := readStreamScanPhysicalLine(reader, 128); err == nil || !strings.Contains(fmt.Sprint(err), "exceeds 128 bytes") {
		t.Fatalf("overlong unterminated line escaped bounded reader: %v", err)
	}
}
