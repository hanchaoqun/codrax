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
		string(types.AgentExplorer), types.StageExplore, 12, 150*time.Second, "MiniMax-M2.7", "zh", "探查", 0))
	if !strings.Contains(zh, "等待模型响应 已 2m30s") || !strings.Contains(zh, "MiniMax-M2.7") {
		t.Fatalf("zh heartbeat line wrong: %q", zh)
	}
	if strings.Contains(zh, "首字节上限") {
		t.Fatalf("zero deadline must render without the ceiling segment: %q", zh)
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
		string(types.AgentExplorer), types.StageExplore, 0, 30*time.Second, "m-1", "en", "", 0))
	if !strings.Contains(en, "still waiting for the model · 30s elapsed") || !strings.Contains(en, "m-1") {
		t.Fatalf("en heartbeat line wrong: %q", en)
	}
	if !strings.Contains(en, "Explore · Round 1") {
		t.Fatalf("en heartbeat line must carry the trace label: %q", en)
	}

	// Unknown model id → segment omitted, no dangling separator.
	noModel := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 0, time.Minute, "  ", "zh", "", 0))
	if strings.Contains(noModel, "· ·") || strings.HasSuffix(strings.TrimSpace(noModel), "·") {
		t.Fatalf("empty model id must not leave a dangling separator: %q", noModel)
	}
}

// TestFormatLLMWaitHeartbeatLine_DeadlineSegment pins the §29.92 件4
// wording: when the adapter reports its first-byte ceiling, the
// heartbeat says how much longer the system will wait for the server
// to start speaking — "已 30s / 首字节上限 3m0s" — so a user watching
// a reasoning model think knows the wait is bounded. The label names
// the FIRST-BYTE ceiling specifically (not a bare "上限"): it bounds
// only the wait for the first byte, other watchdogs own later phases.
func TestFormatLLMWaitHeartbeatLine_DeadlineSegment(t *testing.T) {
	zh := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentAnalyzer), types.StageAnalyze, 0, 30*time.Second, "MiniMax-M2.7", "zh", "", 3*time.Minute))
	if !strings.Contains(zh, "等待模型响应 已 30s / 首字节上限 3m0s") {
		t.Fatalf("zh heartbeat must carry the first-byte ceiling: %q", zh)
	}
	en := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentAnalyzer), types.StageAnalyze, 0, 30*time.Second, "m-1", "en", "", 3*time.Minute))
	if !strings.Contains(en, "still waiting for the model · 30s elapsed / first-byte ceiling 3m0s") {
		t.Fatalf("en heartbeat must carry the first-byte ceiling: %q", en)
	}
}

func TestFormatLLMWaitHeartbeatLine_StreamActivityClassification(t *testing.T) {
	transportOnly := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 1, 4*time.Minute, "m", "zh", "", 3*time.Minute,
		llmWaitActivityDisplay{transportSeen: true}))
	if !strings.Contains(transportOnly, "仅收到传输字节") || !strings.Contains(transportOnly, "尚无模型语义输出") {
		t.Fatalf("transport-only wait must be explicit: %q", transportOnly)
	}
	protocolOnly := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 1, 4*time.Minute, "m", "zh", "", 3*time.Minute,
		llmWaitActivityDisplay{transportSeen: true, protocolSeen: true}))
	if !strings.Contains(protocolOnly, "已收到有效协议数据") || !strings.Contains(protocolOnly, "尚无正文/工具语义增量") {
		t.Fatalf("protocol-only wait must be explicit: %q", protocolOnly)
	}
	semantic := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 1, 4*time.Minute, "m", "zh", "", 3*time.Minute,
		llmWaitActivityDisplay{transportSeen: true, protocolSeen: true, semanticSeen: true}))
	if !strings.Contains(semantic, "已收到模型语义输出，持续生成中") {
		t.Fatalf("semantic-progress wait must be explicit: %q", semantic)
	}
	english := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 1, 4*time.Minute, "m", "en", "", 3*time.Minute,
		llmWaitActivityDisplay{transportSeen: true}))
	if !strings.Contains(english, "transport bytes only") || !strings.Contains(english, "no semantic model output yet") {
		t.Fatalf("English transport-only classification missing: %q", english)
	}
}

// TestFormatLLMWaitHeartbeatLine_ExceededCeilingAnnotated pins the
// §29.174 RUN2AUDIT-1 F5 broken-promise guard against the
// runnable_2.txt witness: "已 1m0s / 首字节上限 40s" rendered 7 times
// (:27/:38/:44/:48/:66/:83/:91) — elapsed past the advertised ceiling
// with zero explanation. The ceiling is a byte-liveness sliding window
// (keep-alive bytes reset the §29.92 watchdog), so once elapsed exceeds
// the static number the line MUST say the deadline slid with server
// liveness; the bare contradiction form is forbidden.
func TestFormatLLMWaitHeartbeatLine_ExceededCeilingAnnotated(t *testing.T) {
	zh := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, time.Minute, "MiniMax-M2.7", "zh", "探查", 40*time.Second))
	if strings.Contains(zh, "已 1m0s / 首字节上限 40s") {
		t.Fatalf("naked elapsed>ceiling contradiction form must not ship: %q", zh)
	}
	if !strings.Contains(zh, "已 1m0s") || !strings.Contains(zh, "40s") {
		t.Fatalf("annotated form must keep both the elapsed and the static cap visible: %q", zh)
	}
	// Both mechanisms must be named: the watchdog heartbeats for the
	// WHOLE Chat call, so a long post-first-byte generation renders
	// this line too — an annotation asserting only keep-alive resets
	// would misstate that case (merge review 2026-07-20).
	if !strings.Contains(zh, "保活重置或首字节已到") || !strings.Contains(zh, "未判超时") {
		t.Fatalf("elapsed past the static cap must carry the two-mechanism liveness annotation: %q", zh)
	}

	en := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, time.Minute, "m-1", "en", "", 40*time.Second))
	if strings.Contains(en, "1m0s elapsed / first-byte ceiling 40s") {
		t.Fatalf("naked EN contradiction form must not ship: %q", en)
	}
	if !strings.Contains(en, "keep-alive resets or first byte already arrived, not timed out") {
		t.Fatalf("EN annotated form missing: %q", en)
	}

	// Negative arm: elapsed at or below the ceiling keeps the
	// pre-§29.174 wording byte-stable — no annotation noise while the
	// promise still holds.
	within := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, 30*time.Second, "MiniMax-M2.7", "zh", "", 40*time.Second))
	if !strings.Contains(within, "等待模型响应 已 30s / 首字节上限 40s") {
		t.Fatalf("within-ceiling form must stay unchanged: %q", within)
	}
	if strings.Contains(within, "顺延") {
		t.Fatalf("within-ceiling form must not carry the liveness annotation: %q", within)
	}
	boundary := stripAnsiEscapes(formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, 40*time.Second, "MiniMax-M2.7", "zh", "", 40*time.Second))
	if !strings.Contains(boundary, "已 40s / 首字节上限 40s") {
		t.Fatalf("elapsed == ceiling is not a contradiction; form must stay unchanged: %q", boundary)
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
		string(types.AgentExplorer), types.StageExplore, 4, 2*time.Minute, "m-shared", "zh", "第 1 路", 0)
	lineB := formatLLMWaitHeartbeatLine(
		string(types.AgentExplorer), types.StageExplore, 4, 2*time.Minute, "m-shared", "zh", "第 2 路", 0)
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
