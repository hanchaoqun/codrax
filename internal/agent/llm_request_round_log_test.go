package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/logging"
)

// withTempDebugLogger installs a fresh on-disk Logger at DEBUG level
// writing to a per-test directory, restores the previous Default on
// exit, and returns a getter that reads the accumulated log content.
// Mirror of internal/render's withTempLogger, at debug level because
// the llm_request diag lines under test are Debug-severity.
func withTempDebugLogger(t *testing.T) func() string {
	t.Helper()
	dir := t.TempDir()
	prev := logging.Default
	lg, err := logging.NewFromFlags(dir, "debug", false)
	if err != nil {
		t.Fatalf("NewFromFlags: %v", err)
	}
	logging.SetDefault(lg)
	t.Cleanup(func() {
		_ = lg.Close()
		logging.SetDefault(prev)
	})
	return func() string {
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

// TestLLMRequestWatchdogLogCarriesRound pins the H14 counter-parity
// fix (berlin.systrace audit, 2026-07-03): the CLI renders the agent
// loop counter 1-based as "第 N 轮" (renderer.go derives
// round := iteration + 1) while the diag log printed only the
// 0-based iter=N, so an operator cross-referencing CLI "第 13 轮"
// against log "iter=12" read them as two different dispatches. The
// llm_request diag lines now carry BOTH: iter= keeps its 0-based
// value (existing log tooling unaffected) and round= carries the
// CLI-aligned 1-based value. This test drives the watchdog stop line
// (the same iter/round pair is used verbatim by the request-start
// Debug line and the still-running Warning line, which share the
// startLLMRequestWatchdog iter argument).
func TestLLMRequestWatchdogLogCarriesRound(t *testing.T) {
	getLogs := withTempDebugLogger(t)

	b := &BaseAgent{name: "analyzer"}
	stop := b.startLLMRequestWatchdog(nil, 12, llm.RequestTelemetry{ModelID: "test-model"})
	stop()

	logs := getLogs()
	if !strings.Contains(logs, "iter=12 round=13 phase=llm_request") {
		t.Errorf("watchdog done line must carry iter=12 (0-based, unchanged) AND round=13 (CLI-aligned 1-based); log:\n%s", logs)
	}
	// iter= must NOT have been rebased to 1-based — existing log
	// analysis keys on the 0-based value.
	if strings.Contains(logs, "iter=13") {
		t.Errorf("iter= must stay 0-based (12), got a rebased iter=13; log:\n%s", logs)
	}
}
