package render

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
)

// withTempLogger installs a fresh on-disk Logger writing to a
// per-test directory, restores the previous Default on exit, and
// returns a getter that reads the accumulated log content. The
// flush-and-close is performed inside the getter so callers can
// snapshot mid-test if they want.
func withTempLogger(t *testing.T) func() string {
	t.Helper()
	dir := t.TempDir()
	prev := logging.Default
	lg, err := logging.NewFromFlags(dir, "info", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	logging.SetDefault(lg)
	t.Cleanup(func() {
		_ = lg.Close()
		logging.SetDefault(prev)
	})
	return func() string {
		// Logger writes synchronously inside log() under a mutex,
		// so a Close-and-reread is the safest snapshot. We close
		// at end-of-test only; mid-test reads pick up whatever has
		// been flushed by the OS file buffer (Go's *os.File writes
		// are not buffered in userland, so any returned-from-Write
		// bytes are visible immediately).
		entries, err := filepath.Glob(filepath.Join(dir, "codrax-*.log"))
		if err != nil {
			t.Fatalf("glob log files: %v", err)
		}
		var sb strings.Builder
		for _, f := range entries {
			data, err := os.ReadFile(f)
			if err != nil {
				t.Fatalf("read %s: %v", f, err)
			}
			sb.Write(data)
		}
		return sb.String()
	}
}

// TestLogFidelity_DockCommitsMirrorToInfo verifies that every
// dock.commitToScrollback call also produces an INFO log entry
// with ANSI escapes stripped. Without this guard, a refactor that
// stops mirroring (or re-introduces ANSI) would silently disable
// post-run audit trails — the user's screen content would no longer
// be reconstructible from the log alone.
func TestLogFidelity_DockCommitsMirrorToInfo(t *testing.T) {
	getLogs := withTempLogger(t)

	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventObjectiveStarted, Timestamp: t0, Objective: "demo question"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventAgentThinking, Timestamp: t0.Add(20 * time.Millisecond), Iteration: 0})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(30 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})

	logs := getLogs()
	for _, want := range []string{
		"[render]",
		"demo question",
		"✓ analyze",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("INFO log missing %q; full log:\n%s", want, logs)
		}
	}
	// Mirrored lines MUST NOT contain ANSI escapes — the whole
	// point of the mirror is to give a plain-text replay of the
	// screen.
	if strings.Contains(logs, "\x1b[") {
		t.Errorf("mirrored INFO log contains ANSI escapes — strip is broken; log:\n%s", logs)
	}
}

// TestLogFidelity_NoANSILeakage drives a synthetic dock event
// stream with strongly-styled lines (every column of the live bar
// uses an ANSI-bearing pterm style) and asserts the log records
// remain plain text. Catches a regression where someone forgets
// to call mirrorDockLineToLog in a new emit path.
func TestLogFidelity_NoANSILeakage(t *testing.T) {
	getLogs := withTempLogger(t)

	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventObjectiveStarted, Timestamp: t0, Objective: "pipeline question"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventAnalysisReady, Timestamp: t0.Add(5 * time.Millisecond), TaskNodes: []TaskNodeInfo{
		{ID: "n_evidence_t0", Type: "evidence", Objective: "topic A"},
		{ID: "n_evidence_t1", Type: "evidence", Objective: "topic B"},
	}})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(6 * time.Millisecond), NodeID: "n_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(20 * time.Millisecond), NodeID: "n_evidence_t0"})

	logs := getLogs()
	if strings.Contains(logs, "\x1b[") {
		t.Errorf("dock log mirror leaked ANSI escapes; log:\n%s", logs)
	}
	// Sub-topic enumeration mirror must arrive, including the
	// circled-digit prefix so the topic identity survives in log.
	for _, want := range []string{"分析识别到", "topic A", "topic B"} {
		if !strings.Contains(logs, want) {
			t.Errorf("sub-topic mirror missing %q; log:\n%s", want, logs)
		}
	}
}

// TestLogFidelity_AdapterRetryMirrorsToInfo verifies that
// EventAdapterRetry produces a permanent log record so a 60-second
// backoff window is auditable later. Without the mirror, retry
// activity would only appear on the dock's transient row 1 status,
// gone after StopSpinner runs.
func TestLogFidelity_AdapterRetryMirrorsToInfo(t *testing.T) {
	getLogs := withTempLogger(t)

	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:         EventAdapterRetry,
		Timestamp:    t0,
		RetryAttempt: 2,
		RetryDelay:   4 * time.Second,
		RetryReason:  "rate limit",
	})

	logs := getLogs()
	// Post-2026-05-02 user-facing language: spell out "重新请求模型"
	// (the LLM call being retried) instead of internal "L1" layer
	// jargon, and localize "rate limit" → "限流(请求过频)" so a zh
	// operator does not see English mixed into Chinese prose.
	for _, want := range []string{"[render]", "重新请求模型", "限流"} {
		if !strings.Contains(logs, want) {
			t.Errorf("adapter retry mirror missing %q; log:\n%s", want, logs)
		}
	}
	// Negative assertions: the old internal-jargon shape MUST NOT
	// appear (regression guard).
	for _, banned := range []string{"L1 重试", "L1 retry", "#2"} {
		if strings.Contains(logs, banned) {
			t.Errorf("adapter retry mirror leaked internal jargon %q; log:\n%s", banned, logs)
		}
	}
	if strings.Contains(logs, "\x1b[") {
		t.Errorf("retry mirror leaked ANSI; log:\n%s", logs)
	}
}

// TestLogFidelity_NonTTYMirrors verifies the non-TTY path also
// mirrors to INFO — operators running CI / piped output without a
// dock should still get a complete log replay.
func TestLogFidelity_NonTTYMirrors(t *testing.T) {
	getLogs := withTempLogger(t)

	// Non-TTY mode: dock disabled but state mutations still run.
	// emitNonTTYLine writes to stdout AND mirrors to log. We can
	// invoke handleEventNonTTY directly via the disabled-dock
	// branch in handleEvent.
	r := newTestRenderer("zh")
	r.dockSuppressed = true
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventObjectiveStarted, Timestamp: t0, Objective: "ci question"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(time.Millisecond), Stage: "analyze"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(2 * time.Millisecond), Stage: "analyze"})

	logs := getLogs()
	for _, want := range []string{
		"[render]",
		"ci question",
		"→ analyze",
		"✓ analyze",
	} {
		if !strings.Contains(logs, want) {
			t.Errorf("non-TTY mirror missing %q; log:\n%s", want, logs)
		}
	}
}

func TestLogFidelity_NonTTYRetryDoesNotDoubleMirror(t *testing.T) {
	getLogs := withTempLogger(t)

	r := newTestRenderer("zh")
	r.dockSuppressed = true
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:         EventAdapterRetry,
		Timestamp:    t0,
		RetryAttempt: 1,
		RetryDelay:   2 * time.Second,
		RetryReason:  "rate limit",
	})

	logs := getLogs()
	if got := strings.Count(logs, "重新请求模型"); got != 1 {
		t.Fatalf("non-TTY retry should mirror exactly once, got %d occurrences:\n%s", got, logs)
	}
	if strings.Contains(logs, "已重新请求模型") {
		t.Fatalf("non-TTY retry should use the live retry wording only, not the dock scrollback wording:\n%s", logs)
	}
}

func TestLogFidelity_NonTTYFallbackDoesNotDoubleMirror(t *testing.T) {
	getLogs := withTempLogger(t)

	r := newTestRenderer("zh")
	r.dockSuppressed = true
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:         EventAdapterFallback,
		Timestamp:    t0,
		FallbackFrom: "provider-A",
		FallbackTo:   "provider-B",
		RetryReason:  "rate limit",
	})

	logs := getLogs()
	if got := strings.Count(logs, "切换 LLM 服务"); got != 1 {
		t.Fatalf("non-TTY fallback should mirror exactly once, got %d occurrences:\n%s", got, logs)
	}
	if strings.Contains(logs, "切换 provider") {
		t.Fatalf("non-TTY fallback should not leak the dock scrollback wording:\n%s", logs)
	}
}
