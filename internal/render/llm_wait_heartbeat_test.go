package render

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestLLMWaitHeartbeatDurable pins the power-of-two durable-line gate.
// Ticks arrive at the watchdog's fixed 30s cadence, so tick 1/2/4/8/16
// map to the 30s/1m/2m/4m/8m escalation sequence; everything else is
// a live-only tick that must NOT earn a scrollback line. The gate is a
// precise integer signal — this table is the contract.
func TestLLMWaitHeartbeatDurable(t *testing.T) {
	durable := []int{1, 2, 4, 8, 16, 32, 1024}
	for _, tick := range durable {
		if !llmWaitHeartbeatDurable(tick) {
			t.Errorf("tick %d must be durable", tick)
		}
	}
	silent := []int{0, -1, -8, 3, 5, 6, 7, 9, 12, 15, 17, 33, 1000}
	for _, tick := range silent {
		if llmWaitHeartbeatDurable(tick) {
			t.Errorf("tick %d must NOT be durable", tick)
		}
	}
}

// TestFormatLLMWaitHeartbeatLine pins the low-key line shape: progress
// "›" lead-in, stage/round trace label (2026-07-03 review WF3: an
// unlabelled "waiting" line is unattributable in interleaved runs),
// elapsed rounded to seconds, model id appended when known. No ↻
// (nothing failed) and no ⋯ thinking glyph (the model has produced
// nothing) — the line must read as quiet status, not as a retry or as
// LLM prose.
func TestFormatLLMWaitHeartbeatLine(t *testing.T) {
	zh := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 12, 150*time.Second, "MiniMax-M2.7", "zh", "探查"))
	if !strings.Contains(zh, "等待模型响应 已 2m30s") || !strings.Contains(zh, "MiniMax-M2.7") {
		t.Fatalf("zh heartbeat line wrong: %q", zh)
	}
	if !strings.Contains(zh, "探索 · 探查 · 第 13 轮") {
		t.Fatalf("zh heartbeat line must lead with the stage/unit/round trace label: %q", zh)
	}
	if !strings.Contains(zh, "›") {
		t.Fatalf("heartbeat line must use the progress lead-in: %q", zh)
	}
	if strings.Contains(zh, "↻") || strings.Contains(zh, string(glyphReasoning)) {
		t.Fatalf("heartbeat line must not borrow retry/thinking glyphs: %q", zh)
	}

	en := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 0, 30*time.Second, "m-1", "en", ""))
	if !strings.Contains(en, "still waiting for the model · 30s elapsed") || !strings.Contains(en, "m-1") {
		t.Fatalf("en heartbeat line wrong: %q", en)
	}
	if !strings.Contains(en, "Explore · Round 1") {
		t.Fatalf("en heartbeat line must carry the trace label: %q", en)
	}

	// Unknown model id → segment omitted, no dangling separator.
	noModel := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 0, time.Minute, "  ", "zh", ""))
	if strings.Contains(noModel, "· ·") || strings.HasSuffix(strings.TrimSpace(noModel), "·") {
		t.Fatalf("empty model id must not leave a dangling separator: %q", noModel)
	}
}

// TestFormatLLMWaitHeartbeatLine_ParallelUnitsByteDistinct pins the
// second half of the WF3 fix: two parallel units waiting on the SAME
// model with the SAME elapsed second used to render byte-identical
// lines, so commitLineLocked's lastCommittedLine consecutive-duplicate
// suppression silently swallowed the second unit's heartbeat. The
// unit label embedded in the trace label makes the two lines
// byte-distinct, so the dedupe (a precise byte-equality gate) can
// never fire across units.
func TestFormatLLMWaitHeartbeatLine_ParallelUnitsByteDistinct(t *testing.T) {
	lineA := formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, 2*time.Minute, "m-shared", "zh", "第 1 路")
	lineB := formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, 2*time.Minute, "m-shared", "zh", "第 2 路")
	if lineA == lineB {
		t.Fatalf("same-second same-model heartbeats from different units must differ: %q", lineA)
	}
	if !strings.Contains(stripAnsiEscapes(lineA), "第 1 路") ||
		!strings.Contains(stripAnsiEscapes(lineB), "第 2 路") {
		t.Fatalf("unit labels must surface in the rendered lines: %q / %q", lineA, lineB)
	}
}

// TestRenderer_LLMWaitingHeartbeatRateLimitedNonTTY drives the full
// event path on the non-TTY branch — the surface that was completely
// silent in the 2026-07-03 berlin.systrace customer session (14m35s
// model response, last visible line frozen on a trace_query call).
// Ticks 1..8 arrive; only the power-of-two ticks (1,2,4,8) may print.
func TestRenderer_LLMWaitingHeartbeatRateLimitedNonTTY(t *testing.T) {
	var buf strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&buf)
	emit := r.Emitter()

	for tick := 1; tick <= 8; tick++ {
		emit(Event{
			Kind:        EventAgentLLMWaiting,
			Agent:       types.AgentExplorer,
			Stage:       types.StageExplore,
			Iteration:   12,
			ModelID:     "MiniMax-M2.7",
			WaitTick:    tick,
			WaitElapsed: time.Duration(tick) * 30 * time.Second,
		})
	}
	out := buf.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 4 {
		t.Fatalf("ticks 1..8 must yield exactly 4 durable lines (1,2,4,8), got %d:\n%s", len(lines), out)
	}
	for _, want := range []string{"已 30s", "已 1m0s", "已 2m0s", "已 4m0s"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing durable heartbeat for %s:\n%s", want, out)
		}
	}
	for _, forbidden := range []string{"已 1m30s", "已 2m30s", "已 3m0s", "已 3m30s"} {
		if strings.Contains(out, forbidden) {
			t.Errorf("non-power-of-two tick leaked to scrollback (%s):\n%s", forbidden, out)
		}
	}
	// WF3: every durable line carries the stage/round attribution.
	for i, line := range lines {
		if !strings.Contains(line, "探索 · 第 13 轮") {
			t.Errorf("durable heartbeat line %d missing trace label: %q", i, line)
		}
	}
}
