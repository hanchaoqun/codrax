package render

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/types"
)

// TestDock_FirstPaintNoRewind locks the load-bearing first-paint
// invariant: the very first paintDock MUST NOT issue any cursor-up
// sequence. There is no prior dock to rewind over; doing so would
// scroll terminal content above the cursor up off-screen and corrupt
// the user's scrollback before the dock has even appeared. The first
// paint also emits the dock's leading blank line so the visual gap
// appears immediately on activation.
func TestDock_FirstPaintNoRewind(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"row1", "row2", "row3"})
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("first paintDock must NOT emit any ANSI cursor escape; got: %q", out)
	}
	if want := "\nrow1\nrow2\nrow3\n"; out != want {
		t.Errorf("first paintDock output = %q, want %q", out, want)
	}
	if !d.dirty {
		t.Error("dock should be dirty after first paint")
	}
	if d.firstPaint {
		t.Error("firstPaint flag should flip to false after first paint")
	}
}

// TestDock_SubsequentPaintRewinds verifies that paintDock #2+
// emits the \x1b[4A\x1b[J rewind+clear sequence (4 = leading blank
// + 3 content rows) so the previous dock visual is wiped before the
// new blank + rows are drawn. Without this every paint would stack
// a fresh 4 lines below the previous, which is the exact "drift"
// symptom the dock redesign aimed to eliminate.
func TestDock_SubsequentPaintRewinds(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"a", "b", "c"})
	buf.Reset()
	d.paintDock([dockRowCount]string{"d", "e", "f"})
	out := buf.String()
	if !strings.HasPrefix(out, "\x1b[4A\x1b[J") {
		t.Errorf("subsequent paintDock must start with \\x1b[4A\\x1b[J; got: %q", out)
	}
	if want := "\x1b[4A\x1b[J\nd\ne\nf\n"; out != want {
		t.Errorf("subsequent paint output = %q, want %q", out, want)
	}
}

// TestDock_CommitToScrollbackOrder pins the byte sequence of a
// commit while the dock is dirty: rewind 4 → body → '\n' → leading
// blank → 3 rows → '\n' each. Order matters because the body lands
// ABOVE the re-painted dock, which is what gives the user the
// illusion of "scrollback grows under a fixed dock". The blank
// line is emitted INSIDE the dock region (between body and rows)
// so it gets wiped + rewritten on every commit and never escapes
// into permanent scrollback.
func TestDock_CommitToScrollbackOrder(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"r1", "r2", "r3"})
	buf.Reset()
	d.commitToScrollback("✓ stage done\n", [dockRowCount]string{"r1b", "r2b", "r3b"})
	out := buf.String()
	want := "\x1b[4A\x1b[J✓ stage done\n\nr1b\nr2b\nr3b\n"
	if out != want {
		t.Errorf("commit byte sequence = %q, want %q", out, want)
	}
}

// TestDock_CommitWhenInactiveSkipsRewind verifies that when the
// dock has not been painted yet (first commit before first paint),
// commitToScrollback writes the body directly + paints fresh —
// without an erroneous cursor-up that would scroll real terminal
// content off-screen. The first paintDock then supplies the
// dock's leading blank line as part of the dock region.
func TestDock_CommitWhenInactiveSkipsRewind(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.commitToScrollback("first line\n", [dockRowCount]string{"a", "b", "c"})
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Errorf("commit when dock inactive must NOT rewind; got: %q", out)
	}
	if want := "first line\n\na\nb\nc\n"; out != want {
		t.Errorf("commit-then-firstPaint sequence = %q, want %q", out, want)
	}
}

// TestDock_ClearDockIdempotent locks the contract that clearDock
// can be called any number of times without error and that a second
// clear on an already-cleared dock is a no-op (no extra ANSI bytes).
func TestDock_ClearDockIdempotent(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"a", "b", "c"})
	buf.Reset()
	d.clearDock()
	if !strings.HasPrefix(buf.String(), "\x1b[4A\x1b[J") {
		t.Errorf("clearDock must emit \\x1b[4A\\x1b[J; got: %q", buf.String())
	}
	buf.Reset()
	d.clearDock()
	if buf.Len() != 0 {
		t.Errorf("second clearDock on cleared dock must be no-op; wrote %q", buf.String())
	}
}

// TestActivityPhrase_AllKindsLocalized verifies every activityKind
// has both zh and en phrases. A new kind added to the enum without
// a matching phrase trip falls through to the empty default — this
// test catches that.
func TestActivityPhrase_AllKindsLocalized(t *testing.T) {
	all := []activityKind{
		activityWaitingPipeline,
		activityWaitingDispatch,
		activityWaitingNode,
		activityRequesting,
		activityReceiving,
		activityCallingTool,
		activityFinalizing,
		activityRetrying,
		activitySwitchingProvider,
		activityPreparingWorktree,
		activityCapturingBaseline,
		activityRepoMapScanning,
		activityErrorRecoverable,
		activityErrorFatal,
		activityCancelled,
	}
	for _, kind := range all {
		state := activityState{kind: kind, retryAttempt: 1, retryDelaySec: 4}
		zh := activityPhrase(state, "zh")
		en := activityPhrase(state, "en")
		if zh == "" {
			t.Errorf("activityKind=%d zh phrase missing", kind)
		}
		if en == "" {
			t.Errorf("activityKind=%d en phrase missing", kind)
		}
	}
}

// TestComposeDockRow1_ReceivingHasTail verifies the row-1 contract
// that activityReceiving renders with a `▸ tail` segment containing
// the streamTail value. This is the visual signal "the model is
// streaming bytes RIGHT NOW".
func TestComposeDockRow1_ReceivingHasTail(t *testing.T) {
	state := dockRowState{
		activity:   activityState{kind: activityReceiving},
		streamTail: "with Hi there how can I",
		frame:      "⠋",
		lang:       "zh",
	}
	row := composeDockRow1(state)
	plain := stripAnsiEscapes(row)
	if !strings.Contains(plain, "接收中") {
		t.Errorf("zh receiving row must contain status word '接收中'; got %q", plain)
	}
	if !strings.Contains(plain, "▸") {
		t.Errorf("receiving row must contain tail separator '▸'; got %q", plain)
	}
	if !strings.Contains(plain, "with Hi there how can I") {
		t.Errorf("receiving row must include streamTail body; got %q", plain)
	}
}

func TestStreamTailDisplayBudgetKeepsLongerContext(t *testing.T) {
	preview := "0123456789ABCDEFGHIJklmnopqrstUVWX" + "YZabcdefghijklmnopqrstuvwxyz"
	tail := tailByDisplayWidth(preview, streamTailDisplayCols)
	if streamTailDisplayCols != 50 {
		t.Fatalf("stream tail budget = %d, want the modest 50-col REPL dock budget", streamTailDisplayCols)
	}
	if len(tail) > streamTailDisplayCols {
		t.Fatalf("stream tail exceeded budget: len=%d budget=%d tail=%q", len(tail), streamTailDisplayCols, tail)
	}
	if !strings.Contains(tail, "CDEFGHIJ") {
		t.Fatalf("stream tail budget should preserve the widened recent context, got %q", tail)
	}
	if strings.Contains(tail, "ABCDEFGHIJ") {
		t.Fatalf("stream tail should still be a bounded suffix, got %q", tail)
	}
}

// TestComposeDockRow1_RequestingNoTail verifies the row-1 contract
// that activityRequesting renders ONLY the status word — no `▸
// tail` segment because there's no live stream to show. The dock
// then has horizontal headroom for whatever the next event needs.
func TestComposeDockRow1_RequestingNoTail(t *testing.T) {
	state := dockRowState{
		activity:   activityState{kind: activityRequesting},
		streamTail: "should not appear",
		frame:      "⠋",
		lang:       "zh",
	}
	row := composeDockRow1(state)
	plain := stripAnsiEscapes(row)
	if !strings.Contains(plain, "请求模型中") {
		t.Errorf("requesting row must contain '请求模型中'; got %q", plain)
	}
	if strings.Contains(plain, "▸") {
		t.Errorf("requesting row must NOT include the ▸ tail segment; got %q", plain)
	}
}

func TestLightRouteActivityCountdownUsesCurrentTime(t *testing.T) {
	start := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	state := activityState{
		kind:          activityLightRoute,
		detail:        "操作中：获取模型列表",
		deadline:      start.Add(2 * time.Minute),
		timeoutBudget: 2 * time.Minute,
	}

	first := activityPhrase(lightRouteActivityWithCountdown(state, start, "zh"), "zh")
	later := activityPhrase(lightRouteActivityWithCountdown(state, start.Add(35*time.Second), "zh"), "zh")
	if !strings.Contains(first, "剩余 2m0s / 超时 2m0s") {
		t.Fatalf("initial countdown missing: %q", first)
	}
	if !strings.Contains(later, "剩余 1m25s / 超时 2m0s") {
		t.Fatalf("updated countdown missing: %q", later)
	}
	if first == later {
		t.Fatalf("countdown should change over time: first=%q later=%q", first, later)
	}
	en := activityPhrase(lightRouteActivityWithCountdown(state, start.Add(time.Minute), "en"), "en")
	if !strings.Contains(en, "remaining 1m0s / timeout 2m0s") {
		t.Fatalf("english countdown missing: %q", en)
	}
}

func TestComposeDockRow1_RepoMapScanShowsProgressTail(t *testing.T) {
	plain := stripAnsiEscapes(composeDockRow1(dockRowState{
		activity:   activityState{kind: activityRepoMapScanning},
		streamTail: "已解析 4000/12000 个源文件（总文件 93459）",
		frame:      "⠋",
		lang:       "zh",
	}))
	if !strings.Contains(plain, "扫描仓库索引中") || !strings.Contains(plain, "4000/12000") {
		t.Fatalf("repo-map scan row should show live count detail, got %q", plain)
	}
}

func TestComposeDockRow1_ParallelRequestingAggregatesLanes(t *testing.T) {
	state := dockRowState{
		activity: activityState{kind: activityRequesting},
		stageKey: "evidence",
		frame:    "⠋",
		lang:     "zh",
		parallel: &parallelActivitySnapshot{
			stage:       "explore",
			active:      true,
			total:       3,
			parallelism: 2,
			activeUnits: 2,
			requesting:  2,
		},
	}
	plain := stripAnsiEscapes(composeDockRow1(state))
	if !strings.Contains(plain, "并行请求模型中") {
		t.Fatalf("parallel requesting row must use aggregate wording; got %q", plain)
	}
	if !strings.Contains(plain, "2 路") {
		t.Fatalf("parallel requesting row must show active lane count; got %q", plain)
	}
	if strings.Contains(plain, "▸") {
		t.Fatalf("parallel requesting row must not show a stream tail; got %q", plain)
	}
}

func TestComposeDockRow1_ParallelSnapshotDoesNotCrossStageFamily(t *testing.T) {
	state := dockRowState{
		activity: activityState{kind: activityRequesting},
		stageKey: "extract",
		frame:    "⠋",
		lang:     "zh",
		parallel: &parallelActivitySnapshot{
			stage:       "explore",
			active:      true,
			total:       2,
			parallelism: 2,
			activeUnits: 2,
			requesting:  2,
		},
	}
	plain := stripAnsiEscapes(composeDockRow1(state))
	if strings.Contains(plain, "并行") || strings.Contains(plain, "2 路") {
		t.Fatalf("explore parallel activity must not render on extract row; got %q", plain)
	}
	if !strings.Contains(plain, "请求模型中") {
		t.Fatalf("mismatched parallel snapshot should fall back to regular activity; got %q", plain)
	}
}

func TestComposeDockRow1_ParallelMultiReceivingSuppressesAmbiguousTail(t *testing.T) {
	state := dockRowState{
		activity: activityState{kind: activityReceiving},
		stageKey: "evidence",
		frame:    "⠋",
		lang:     "zh",
		parallel: &parallelActivitySnapshot{
			stage:       "explore",
			active:      true,
			total:       3,
			parallelism: 2,
			activeUnits: 2,
			receiving:   2,
			streamTail:  "tail from one worker",
		},
	}
	plain := stripAnsiEscapes(composeDockRow1(state))
	if !strings.Contains(plain, "并行接收模型输出") {
		t.Fatalf("parallel receiving row must use aggregate wording; got %q", plain)
	}
	if strings.Contains(plain, "tail from one worker") || strings.Contains(plain, "▸") {
		t.Fatalf("parallel multi-stream row must not present one worker's tail as the whole group; got %q", plain)
	}
}

// TestComposeDockRow1_RetryFreezesGlyph confirms that retry / fatal
// states freeze the spinner glyph to ↻ / ✗ instead of cycling
// through spinnerFrames. Animated braille on an error state would
// be confusing — frozen glyphs read as "stuck on error".
func TestComposeDockRow1_RetryFreezesGlyph(t *testing.T) {
	state := dockRowState{
		activity: activityState{kind: activityRetrying, retryAttempt: 2, retryDelaySec: 4},
		frame:    "⠋",
		lang:     "zh",
	}
	row := composeDockRow1(state)
	plain := stripAnsiEscapes(row)
	if !strings.Contains(plain, "↻") {
		t.Errorf("retry state must use frozen ↻ glyph; got %q", plain)
	}
	if strings.Contains(plain, "⠋") {
		t.Errorf("retry state must NOT use spinner braille; got %q", plain)
	}
}

// TestComposeDockRow2_HidesZeroCounters locks the contract that
// counters render only when > 0 — a fresh node start with iteration=0
// + toolCount=0 should not show the noise "第 0 轮 · 0 工具".
func TestComposeDockRow2_HidesZeroCounters(t *testing.T) {
	state := dockRowState{
		stageProgress: "2/6",
		stageLabel:    "探索证据",
		iteration:     0,
		toolCount:     0,
		lang:          "zh",
	}
	row := composeDockRow2(state)
	plain := stripAnsiEscapes(row)
	if strings.Contains(plain, "0 轮") || strings.Contains(plain, "0 工具") {
		t.Errorf("zero counters must not render; got %q", plain)
	}
	if !strings.Contains(plain, "2/6") || !strings.Contains(plain, "探索证据") {
		t.Errorf("non-zero core (K/N + label) must always render; got %q", plain)
	}
}

func TestComposeDockRow2_ParallelMetaSitsWithStageBeforeModel(t *testing.T) {
	state := dockRowState{
		stageKey:      "evidence",
		stageProgress: "2/4",
		stageLabel:    "正在收集证据",
		modelID:       "MiniMax-M2.7-highspeed",
		lang:          "zh",
		parallel: &parallelActivitySnapshot{
			stage:       "explore",
			active:      true,
			total:       3,
			parallelism: 2,
		},
	}
	plain := stripAnsiEscapes(composeDockRow2(state))
	for _, want := range []string{"并行 2 路", "3 个调查单元", "模型 MiniMax-M2.7-highspeed"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("row2 missing %q: %q", want, plain)
		}
	}
	if strings.Index(plain, "并行 2 路") > strings.Index(plain, "模型 MiniMax-M2.7-highspeed") {
		t.Fatalf("parallel stage meta should appear before model telemetry; got %q", plain)
	}
}

func TestComposeDockRow2_ParallelMetaDoesNotCrossStageFamily(t *testing.T) {
	for _, key := range []string{"analyze", "extract", "finalize", "plan", "apply", "verify"} {
		state := dockRowState{
			stageKey:      key,
			stageProgress: "3/4",
			stageLabel:    "正在提炼关键发现",
			lang:          "zh",
			parallel: &parallelActivitySnapshot{
				stage:       "explore",
				active:      true,
				total:       2,
				parallelism: 2,
			},
		}
		plain := stripAnsiEscapes(composeDockRow2(state))
		if strings.Contains(plain, "并行") || strings.Contains(plain, "调查单元") || strings.Contains(plain, "关注点") {
			t.Fatalf("parallel explore meta leaked onto %s row: %q", key, plain)
		}
	}
}

// TestComposeDockRow2_ShowsModelAndContextTokens pins the REPL dock
// placement for per-request LLM telemetry. It belongs to row 2 with
// the other stage/request counters, leaving row 1 for live activity
// and row 3 for time/cancel affordances.
func TestComposeDockRow2_ShowsModelAndContextTokens(t *testing.T) {
	state := dockRowState{
		stageProgress:         "2/6",
		stageLabel:            "探索证据",
		iteration:             2,
		modelID:               "qwen3.5:9b",
		contextTokensEstimate: 31234,
		contextWindowTokens:   260000,
		toolCount:             3,
		lang:                  "zh",
	}
	row := composeDockRow2(state)
	plain := stripAnsiEscapes(row)
	for _, want := range []string{"第 2 轮", "模型 qwen3.5:9b", "约 31k/260k tok", "3 次工具调用"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row 2 must contain %q; got %q", want, plain)
		}
	}
	ordered := []string{"2/6", "探索证据", "模型 qwen3.5:9b", "约 31k/260k tok", "第 2 轮", "3 次工具调用"}
	prev := -1
	for _, part := range ordered {
		idx := strings.Index(plain, part)
		if idx < 0 {
			t.Fatalf("row 2 missing %q; got %q", part, plain)
		}
		if idx <= prev {
			t.Fatalf("row 2 order drift: %q appeared at %d after previous index %d; row=%q", part, idx, prev, plain)
		}
		prev = idx
	}
}

// TestComposeDockRow3_TimeOnly verifies the row-3 minimum content:
// at least the "总 Xs" segment when total elapsed is non-zero and no
// interactive affordance owns the row.
func TestComposeDockRow3_TimeOnly(t *testing.T) {
	state := dockRowState{
		stageElapsed: "5s",
		totalElapsed: "12s",
		lang:         "zh",
	}
	row := composeDockRow3(state)
	plain := stripAnsiEscapes(row)
	for _, want := range []string{"本 5s", "总 12s"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row 3 must contain %q; got %q", want, plain)
		}
	}
}

func TestComposeDockRow3_CancelHintSuppressesLiveElapsed(t *testing.T) {
	state := dockRowState{
		stageElapsed: "5s",
		totalElapsed: "12s",
		cancelHint:   "Ctrl+C 取消",
		lang:         "zh",
	}
	row := composeDockRow3(state)
	plain := stripAnsiEscapes(row)
	if !strings.Contains(plain, "Ctrl+C 取消") {
		t.Fatalf("row 3 must keep the cancel hint; got %q", plain)
	}
	if !strings.Contains(plain, "总 12s") {
		t.Fatalf("cancel-owned row 3 must preserve total elapsed; got %q", plain)
	}
	if strings.Contains(plain, "本 5s") {
		t.Fatalf("cancel-owned row 3 must suppress stage-local elapsed; got %q", plain)
	}
}

// TestRenderer_StateMutationsWithoutDock verifies that handleEvent
// mutates renderer state even when no dock is attached (the test
// fixture path). State assertions tested against in dock-aware
// behavioural tests rely on this contract.
func TestRenderer_StateMutationsWithoutDock(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventObjectiveStarted, Timestamp: t0, Objective: "demo"})
	if r.objective != "demo" {
		t.Errorf("EventObjectiveStarted must set r.objective even without dock; got %q", r.objective)
	}
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	if len(r.tasks) != 1 {
		t.Errorf("EventStageStart must append a row even without dock; got %d", len(r.tasks))
	}
	emit(Event{
		Kind:                  EventAgentThinking,
		Timestamp:             t0.Add(20 * time.Millisecond),
		Iteration:             0,
		ModelID:               "qwen3.5:9b",
		ContextTokensEstimate: 1200,
		ContextWindowTokens:   260000,
	})
	if r.activity.kind != activityRequesting {
		t.Errorf("EventAgentThinking must flip activity to requesting; got %v", r.activity.kind)
	}
	if r.requestModelID != "qwen3.5:9b" || r.requestContextTokensEstimate != 1200 || r.requestContextWindowTokens != 260000 {
		t.Errorf("EventAgentThinking must store renderer-level request telemetry; got model=%q tokens=%d window=%d",
			r.requestModelID, r.requestContextTokensEstimate, r.requestContextWindowTokens)
	}
}

func TestRenderer_ParallelActivityEventsAggregateRequesting(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	now := time.Now()
	emit(Event{
		Kind:            EventParallelDispatchStart,
		Timestamp:       now,
		Stage:           "explore",
		Agent:           "explorer",
		ParallelGroupID: "g",
		ParallelTotal:   2,
		Parallelism:     2,
		ParallelUnitIDs: []string{"u0", "u1"},
	})
	for _, unit := range []string{"u0", "u1"} {
		emit(Event{
			Kind:            EventParallelDispatchUnitStart,
			Timestamp:       now,
			Stage:           "explore",
			Agent:           "explorer",
			ParallelGroupID: "g",
			ParallelUnitID:  unit,
			ParallelTotal:   2,
			Parallelism:     2,
		})
		emit(Event{
			Kind:            EventAgentThinking,
			Timestamp:       now,
			Stage:           "explore",
			Agent:           "explorer",
			ParallelGroupID: "g",
			ParallelUnitID:  unit,
		})
	}
	rows := r.composeCurrentDockRows()
	plain := stripAnsiEscapes(rows[0])
	if !strings.Contains(plain, "并行请求模型中") || !strings.Contains(plain, "2 路") {
		t.Fatalf("renderer should aggregate parallel worker request state; got %q", plain)
	}
}

// TestRenderer_DockUsesRendererLevelTelemetryForFallbackRows verifies
// that model/context telemetry survives row focus churn. The user sees
// pending / fallback rows during stage gaps and review windows; those
// rows must not drop the latest LLM request metadata merely because
// they did not own the EventAgentThinking event.
func TestRenderer_DockUsesRendererLevelTelemetryForFallbackRows(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	t0 := time.Now()
	emit := r.Emitter()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:                  EventAgentThinking,
		Timestamp:             t0.Add(10 * time.Millisecond),
		Iteration:             0,
		ModelID:               "qwen3.5:9b",
		ContextTokensEstimate: 39502,
		ContextWindowTokens:   260000,
	})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(20 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})

	rows := r.composeCurrentDockRows()
	plain := stripAnsiEscapes(rows[1])
	for _, want := range []string{"模型 qwen3.5:9b", "约 40k/260k tok"} {
		if !strings.Contains(plain, want) {
			t.Errorf("fallback row must keep request telemetry %q; got %q", want, plain)
		}
	}
}

func TestRenderer_EventLLMRequestStartFeedsDockTelemetry(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{
		Kind:                  EventLLMRequestStart,
		Timestamp:             time.Now(),
		ModelID:               "reviewer-model",
		ContextTokensEstimate: 7283,
		ContextWindowTokens:   260000,
	})
	if r.activity.kind != activityRequesting {
		t.Errorf("EventLLMRequestStart must flip activity to requesting; got %v", r.activity.kind)
	}
	rows := r.composeCurrentDockRows()
	if row1 := stripAnsiEscapes(rows[0]); !strings.Contains(row1, "请求模型中") {
		t.Errorf("direct LLM request must show requesting activity; got %q", row1)
	}
	plain := stripAnsiEscapes(rows[1])
	for _, want := range []string{"模型 reviewer-model", "约 7.3k/260k tok"} {
		if !strings.Contains(plain, want) {
			t.Errorf("direct LLM request telemetry must render %q; got %q", want, plain)
		}
	}
}

func TestRenderer_IndependentLaneOverridesStalePipelineFocus(t *testing.T) {
	r := newTestRenderer("zh")
	r.SetTotalStages(0)
	r.SetRouteSummary("数据任务", []string{"规划中", "未读源码"})
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "turn_policy"})
	emit(Event{
		Kind:                  EventLLMRequestStart,
		Timestamp:             t0.Add(10 * time.Millisecond),
		Stage:                 "data",
		Agent:                 "data_planner",
		ModelID:               "data-model",
		ContextTokensEstimate: 4700,
		ContextWindowTokens:   200000,
	})

	rows := r.composeCurrentDockRows()
	row2 := stripAnsiEscapes(rows[1])
	if strings.Contains(row2, "1/4") || strings.Contains(row2, "正在理解问题") {
		t.Fatalf("data lane must not inherit stale analyze progress; got %q", row2)
	}
	if !strings.Contains(row2, "正在处理数据任务") {
		t.Fatalf("data lane should surface data task status; got %q", row2)
	}
}

func TestRenderer_EventAgentResponseClearsRequestingActivity(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{
		Kind:                  EventAgentThinking,
		Timestamp:             time.Now(),
		ModelID:               "glm5-fp8",
		ContextTokensEstimate: 63000,
		ContextWindowTokens:   200000,
	})
	emit(Event{Kind: EventAgentResponse, Timestamp: time.Now()})

	if r.activity.kind != activityWaitingNode {
		t.Fatalf("EventAgentResponse must switch from model request to local context work; got %v", r.activity.kind)
	}
	rows := r.composeCurrentDockRows()
	if row1 := stripAnsiEscapes(rows[0]); !strings.Contains(row1, "整理上下文中") {
		t.Errorf("post-response local processing must not keep saying 请求模型中; got %q", row1)
	}
	plain := stripAnsiEscapes(rows[1])
	for _, want := range []string{"模型 glm5-fp8", "约 63k/200k tok"} {
		if !strings.Contains(plain, want) {
			t.Errorf("post-response row must keep model telemetry %q; got %q", want, plain)
		}
	}
}

func TestRenderer_EventLocalWorkStartClearsRequestingActivity(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{
		Kind:                  EventLLMRequestStart,
		Timestamp:             time.Now(),
		ModelID:               "glm5-fp8",
		ContextTokensEstimate: 63000,
		ContextWindowTokens:   200000,
	})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: time.Now()})

	if r.activity.kind != activityWaitingNode {
		t.Fatalf("EventLocalWorkStart must switch from model request to local context work; got %v", r.activity.kind)
	}
	rows := r.composeCurrentDockRows()
	if row1 := stripAnsiEscapes(rows[0]); !strings.Contains(row1, "整理上下文中") {
		t.Errorf("local scheduler work must not keep saying 请求模型中; got %q", row1)
	}
}

func TestRenderer_DockGapAfterExploreShowsExtractBeforeFinalize(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0.Add(20 * time.Millisecond),
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(40 * time.Millisecond), NodeID: "n1_evidence_t0"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	if !strings.Contains(row, "3/4") || !strings.Contains(row, "待提炼关键发现") {
		t.Fatalf("after explore completes, stage-only extract must own the next slot; got %q", row)
	}
	if strings.Contains(row, "4/4") || strings.Contains(row, "撰写最终答案") {
		t.Fatalf("pending finalize must not leapfrog extract in the dock row; got %q", row)
	}
}

func TestFormatAnalysisBreakdownBlockIncludesAnswerDimensions(t *testing.T) {
	got := stripAnsiEscapes(formatAnalysisBreakdownBlock("zh", []TaskNodeInfo{
		{ID: "n1_evidence_t0", Type: "evidence", Objective: "调度链"},
		{ID: "n1_evidence_t1", Type: "evidence", Objective: "CPU 供给"},
	}, []AnswerDimensionInfo{
		{Index: 2, Label: "证据支撑"},
		{Index: 1, Label: "根因结论"},
	}))
	for _, want := range []string{
		"分析拆分为 2 个调查单元",
		"调度链",
		"CPU 供给",
		"分析拆分为 2 个答案维度",
		"① 根因结论",
		"② 证据支撑",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("analysis breakdown missing %q:\n%s", want, got)
		}
	}
	en := stripAnsiEscapes(formatAnalysisBreakdownBlock("en", nil, []AnswerDimensionInfo{
		{Label: "root cause"},
	}))
	if !strings.Contains(en, "Analyzer split the answer into 1 answer dimension:") ||
		!strings.Contains(en, "① root cause") {
		t.Fatalf("english answer dimension breakdown malformed:\n%s", en)
	}

	unitBacked := stripAnsiEscapes(formatAnalysisBreakdownBlock("zh", []TaskNodeInfo{
		{
			ID:                   "evidence_scheduler",
			Type:                 "evidence",
			Objective:            "fallback should not win",
			HasInvestigationUnit: true,
			InvestigationUnit: types.InvestigationUnit{
				ID:      "unit-scheduler",
				Index:   1,
				Label:   "调度链",
				Summary: "唤醒链与 runnable",
			},
		},
		{
			ID:                   "evidence_supply",
			Type:                 "evidence",
			Objective:            "fallback should not win",
			HasInvestigationUnit: true,
			InvestigationUnit: types.InvestigationUnit{
				ID:      "unit-supply",
				Index:   2,
				Label:   "算力供给",
				Summary: "CPU/频点/绑核",
			},
		},
	}, nil))
	for _, want := range []string{
		"分析拆分为 2 个调查单元",
		"① 调度链 — 唤醒链与 runnable",
		"② 算力供给 — CPU/频点/绑核",
	} {
		if !strings.Contains(unitBacked, want) {
			t.Fatalf("unit-backed analysis breakdown missing %q:\n%s", want, unitBacked)
		}
	}
}

func TestRenderer_LiveExploreContentAnchorsDockBeforeExtractFallback(t *testing.T) {
	var out strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&out)
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	seedRendererAfterExploreComplete(emit, t0)

	emit(Event{
		Kind:      EventAgentContent,
		Timestamp: t0.Add(50 * time.Millisecond),
		Stage:     "explore",
		Agent:     "explorer",
		Reasoning: `ents": {"path": "java/com/android/server/am/ActivityManagerService.java"`,
	})

	rows := r.composeCurrentDockRows()
	row1 := stripAnsiEscapes(rows[0])
	row2 := stripAnsiEscapes(rows[1])
	if !strings.Contains(row1, "接收中") {
		t.Fatalf("explore stream content must show receiving activity; got row1=%q", row1)
	}
	for _, want := range []string{"2/4", "正在收集证据"} {
		if !strings.Contains(row2, want) {
			t.Fatalf("live explore content must anchor row2 to explore, missing %q: %q", want, row2)
		}
	}
	if strings.Contains(row2, "3/4") || strings.Contains(row2, "提炼关键发现") {
		t.Fatalf("live explore content must not be presented as extract; got %q", row2)
	}
}

func TestRenderer_LiveExploreToolBatchAnchorsDockBeforeExtractFallback(t *testing.T) {
	var out strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&out)
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	seedRendererAfterExploreComplete(emit, t0)

	emit(Event{
		Kind:          EventAgentToolCallBatch,
		Timestamp:     t0.Add(50 * time.Millisecond),
		Stage:         "explore",
		Agent:         "explorer",
		Iteration:     0,
		ToolNames:     []string{"grep"},
		ToolCallCount: 5,
	})

	row2 := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	for _, want := range []string{"2/4", "正在收集证据"} {
		if !strings.Contains(row2, want) {
			t.Fatalf("explore tool batch must anchor row2 to explore, missing %q: %q", want, row2)
		}
	}
	if strings.Contains(row2, "3/4") || strings.Contains(row2, "提炼关键发现") {
		t.Fatalf("explore tool batch must not be presented as extract; got %q", row2)
	}
}

func TestRenderer_RepoMapNoticeUsesExplicitEventStage(t *testing.T) {
	var out strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&out)
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	seedRendererAfterExploreComplete(emit, t0)

	emit(Event{
		Kind:       EventOrchestratorNotice,
		Timestamp:  t0.Add(50 * time.Millisecond),
		Stage:      "explore",
		NoticeKind: NoticeRepoMapScanProgress,
		Reasoning:  "· 仓库索引 `core` 扫描中：已解析 400/1200 个源文件",
	})

	persistent := stripAnsiEscapes(out.String())
	if !strings.Contains(persistent, "· 2/4 仓库索引 `core` 扫描中") {
		t.Fatalf("repo_map notice must use its explicit explore stage, got output:\n%s", persistent)
	}
	if strings.Contains(persistent, "· 3/4 仓库索引") {
		t.Fatalf("repo_map notice must not inherit extract fallback progress, got output:\n%s", persistent)
	}
	row2 := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	for _, want := range []string{"2/4", "正在收集证据"} {
		if !strings.Contains(row2, want) {
			t.Fatalf("repo_map scan activity must anchor row2 to explore, missing %q: %q", want, row2)
		}
	}
}

func TestRenderer_LiveExploreStageClearsWhenNodeEnds(t *testing.T) {
	var out strings.Builder
	r := newTestRenderer("zh")
	r.SetOutput(&out)
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0,
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(10 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{
		Kind:      EventAgentContent,
		Timestamp: t0.Add(20 * time.Millisecond),
		Stage:     "explore",
		Agent:     "explorer",
		Reasoning: "reading ActivityManagerService",
	})
	if row2 := stripAnsiEscapes(r.composeCurrentDockRows()[1]); !strings.Contains(row2, "2/4") {
		t.Fatalf("live explore content should own row2 before node end; got %q", row2)
	}

	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})

	row2 := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	if !strings.Contains(row2, "3/4") || !strings.Contains(row2, "待提炼关键发现") {
		t.Fatalf("finished explore node must release live-stage ownership to extract fallback; got %q", row2)
	}
}

func seedRendererAfterExploreComplete(emit EventEmitter, t0 time.Time) {
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0.Add(20 * time.Millisecond),
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(40 * time.Millisecond), NodeID: "n1_evidence_t0"})
}

func TestRenderer_DockSuppressesPreFinalizeLocalLeapfrogBeforeExtract(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0.Add(20 * time.Millisecond),
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(40 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: t0.Add(50 * time.Millisecond), Stage: "finalize"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	if !strings.Contains(row, "3/4") || !strings.Contains(row, "正在提炼关键发现") {
		t.Fatalf("pre-finalize local checks must not display final answer before extract; got %q", row)
	}
	if strings.Contains(row, "4/4") || strings.Contains(row, "撰写最终答案") {
		t.Fatalf("local StageFinalize must not leapfrog mandatory extract; got %q", row)
	}
}

func TestRenderer_LocalExtractWorkCannotLeapfrogUnstartedExplore(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0.Add(20 * time.Millisecond),
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: t0.Add(30 * time.Millisecond), Stage: "extract"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	for _, want := range []string{"2/4", "正在收集证据"} {
		if !strings.Contains(row, want) {
			t.Fatalf("local extract prep must stay on unstarted upstream explore, missing %q: %q", want, row)
		}
	}
	if strings.Contains(row, "3/4") || strings.Contains(row, "提炼关键发现") {
		t.Fatalf("local extract prep must not leapfrog an unstarted explore node; got %q", row)
	}
}

func TestRenderer_LocalFinalizeWorkCannotLeapfrogUnstartedExploreWithoutAnalyzeRow(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0,
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: t0.Add(10 * time.Millisecond), Stage: "finalize"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	for _, want := range []string{"2/4", "正在收集证据"} {
		if !strings.Contains(row, want) {
			t.Fatalf("downstream local prep must stay on unstarted upstream explore even without an analyze row, missing %q: %q", want, row)
		}
	}
	if strings.Contains(row, "3/4") || strings.Contains(row, "4/4") || strings.Contains(row, "提炼关键发现") || strings.Contains(row, "撰写最终答案") {
		t.Fatalf("downstream local prep must not leapfrog an unstarted explore node; got %q", row)
	}
}

func TestRenderer_ExtractStageClearsStaleParallelExploreTelemetry(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0,
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(10 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{
		Kind:               EventParallelDispatchStart,
		Timestamp:          t0.Add(20 * time.Millisecond),
		Stage:              "explore",
		Agent:              "explorer",
		ParallelGroupID:    "g",
		ParallelTotal:      2,
		Parallelism:        2,
		ParallelUnitIDs:    []string{"n1_evidence_t0", "n1_evidence_t1"},
		ParallelLaneLabels: []string{"vcs_diff", "current_source", "command_measurement", "mcp"},
	})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(40 * time.Millisecond), Stage: "extract", Agent: "extractor"})

	if r.parallel != nil {
		t.Fatal("extract stage start must clear stale explore parallel state")
	}
	rows := r.composeCurrentDockRows()
	row1 := stripAnsiEscapes(rows[0])
	row2 := stripAnsiEscapes(rows[1])
	if strings.Contains(row1, "并行") || strings.Contains(row2, "并行") ||
		strings.Contains(row2, "调查单元") || strings.Contains(row2, "关注点") ||
		strings.Contains(row2, "证据通道") {
		t.Fatalf("stale parallel/focus telemetry must not leak into extract rows; row1=%q row2=%q", row1, row2)
	}
	if !strings.Contains(row2, "3/4") || !strings.Contains(row2, "正在提炼关键发现") {
		t.Fatalf("extract stage must retain its own progress and label; got %q", row2)
	}
}

func TestRenderer_ActiveExploreParallelAnchorsDockBeforeExtract(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0,
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(10 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{
		Kind:               EventParallelDispatchStart,
		Timestamp:          t0.Add(20 * time.Millisecond),
		Stage:              "explore",
		Agent:              "explorer",
		ParallelGroupID:    "g",
		ParallelTotal:      2,
		Parallelism:        2,
		ParallelUnitIDs:    []string{"n1_evidence_t0", "n1_evidence_t1"},
		ParallelLaneLabels: []string{"vcs_diff", "current_source", "command_measurement", "mcp"},
	})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})

	rows := r.composeCurrentDockRows()
	row1 := stripAnsiEscapes(rows[0])
	row2 := stripAnsiEscapes(rows[1])
	if !strings.Contains(row1, "并行") {
		t.Fatalf("active parallel dispatch must own row1 until the dispatch ends; got %q", row1)
	}
	for _, want := range []string{"2/4", "正在收集证据", "并行 2 路", "2 个调查单元", "证据通道：历史"} {
		if !strings.Contains(row2, want) {
			t.Fatalf("active parallel explore row missing %q: %q", want, row2)
		}
	}
	if strings.Contains(row2, "3/4") || strings.Contains(row2, "提炼关键发现") ||
		strings.Contains(row2, "current_source") || strings.Contains(row2, "vcs_diff") {
		t.Fatalf("active parallel explore must not be presented as extract; got %q", row2)
	}
}

func TestRenderer_ActiveExploreParallelUsesExploreSlotAfterHighWater(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0,
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(10 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(20 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(30 * time.Millisecond), Stage: "extract", Agent: "extractor"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(40 * time.Millisecond), Stage: "extract", Agent: "extractor"})
	emit(Event{
		Kind:            EventParallelDispatchStart,
		Timestamp:       t0.Add(50 * time.Millisecond),
		Stage:           "explore",
		Agent:           "explorer",
		ParallelGroupID: "g",
		ParallelTotal:   2,
		Parallelism:     2,
		ParallelUnitIDs: []string{"n1_evidence_t0", "n1_evidence_t1"},
	})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	for _, want := range []string{"2/4", "正在收集证据", "并行 2 路"} {
		if !strings.Contains(row, want) {
			t.Fatalf("active parallel explore after high-water missing %q: %q", want, row)
		}
	}
	if strings.Contains(row, "3/4") || strings.Contains(row, "提炼关键发现") {
		t.Fatalf("active parallel explore must use its own stage slot, not high-water extract; got %q", row)
	}
}

func TestRenderer_DockBacktrackKeepsProgressAndNamesRepairStage(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0.Add(20 * time.Millisecond),
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(40 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(50 * time.Millisecond), Stage: "extract", Agent: "extractor"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(60 * time.Millisecond), Stage: "extract", Agent: "extractor"})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(70 * time.Millisecond), NodeID: "final"})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: t0.Add(80 * time.Millisecond), Stage: "explore"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	if !strings.Contains(row, "4/4") || !strings.Contains(row, "修复中：正在收集证据") {
		t.Fatalf("post-finalize backtrack should keep primary progress and name repair sub-stage; got %q", row)
	}
	if strings.Contains(row, "2/4") {
		t.Fatalf("repair backtrack must not make the primary progress regress; got %q", row)
	}
}

func TestRenderer_DockLiveFinalizerDeltasDoNotShowStaleExploreRepair(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "analyze", Agent: "analyzer"})
	emit(Event{Kind: EventStageEnd, Timestamp: t0.Add(10 * time.Millisecond), Stage: "analyze", Agent: "analyzer"})
	emit(Event{
		Kind:      EventAnalysisReady,
		Timestamp: t0.Add(20 * time.Millisecond),
		TaskNodes: []TaskNodeInfo{
			{ID: "n1_evidence_t0", Type: "evidence", Objective: "topic A"},
			{ID: "final", Type: "finalize", Objective: "render"},
		},
	})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(40 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: t0.Add(50 * time.Millisecond), NodeID: "final"})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: t0.Add(60 * time.Millisecond), Stage: "explore"})
	emit(Event{Kind: EventToolCallStart, Timestamp: t0.Add(70 * time.Millisecond), ToolName: "emit_answer_document"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	if !strings.Contains(row, "4/4") || !strings.Contains(row, "正在撰写最终答案") {
		t.Fatalf("live finalizer tool deltas should bind to the active finalizer row; got %q", row)
	}
	if strings.Contains(row, "修复中：正在收集证据") {
		t.Fatalf("stale explore repair label must not survive live finalizer tool deltas: %q", row)
	}
}

func TestRenderer_DockBacktrackClarifiesValidateRepairStage(t *testing.T) {
	r := newTestRenderer("zh")
	r.totalStages = 4
	emit := r.Emitter()
	t0 := time.Now()
	emit(Event{Kind: EventStageStart, Timestamp: t0, Stage: "finalize", Agent: "finalizer"})
	emit(Event{Kind: EventLocalWorkStart, Timestamp: t0.Add(10 * time.Millisecond), Stage: "validate"})

	row := stripAnsiEscapes(r.composeCurrentDockRows()[1])
	if !strings.Contains(row, "4/4") || !strings.Contains(row, "修复中：正在校验证据和答案约束") {
		t.Fatalf("validate repair stage should explain answer/evidence validation, got %q", row)
	}
	if strings.Contains(row, "修复中：正在交叉验证证据") {
		t.Fatalf("validate repair stage should not surface the ambiguous generic label: %q", row)
	}
}

func TestRenderer_EventAgentThinkingWithoutCurrentRowShowsRequesting(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{
		Kind:                  EventAgentThinking,
		Timestamp:             time.Now(),
		ModelID:               "glm5-fp8",
		ContextTokensEstimate: 7800,
		ContextWindowTokens:   200000,
	})
	if r.activity.kind != activityRequesting {
		t.Errorf("EventAgentThinking without current row must still show requesting; got %v", r.activity.kind)
	}
	rows := r.composeCurrentDockRows()
	if row1 := stripAnsiEscapes(rows[0]); !strings.Contains(row1, "请求模型中") {
		t.Errorf("requesting row missing; got %q", row1)
	}
	plain := stripAnsiEscapes(rows[1])
	for _, want := range []string{"模型 glm5-fp8", "约 7.8k/200k tok"} {
		if !strings.Contains(plain, want) {
			t.Errorf("orphan LLM request telemetry must render %q; got %q", want, plain)
		}
	}
}

func TestRenderer_LightRouteActivityClearsPipelineAndModelTelemetry(t *testing.T) {
	r := newTestRenderer("zh")
	r.requestModelID = "MiniMax-M2.7-highspeed"
	r.requestContextTokensEstimate = 4100
	r.requestContextWindowTokens = 200000
	r.SetRouteSummary("操作执行", []string{"运行中", "未读仓库"})
	r.SetLightRouteActivity("操作执行")

	rows := r.composeCurrentDockRows()
	row1 := stripAnsiEscapes(rows[0])
	row2 := stripAnsiEscapes(rows[1])
	if !strings.Contains(row1, "操作执行") {
		t.Fatalf("light route row1 should use operation activity, got %q", row1)
	}
	if strings.Contains(row1, "准备流水线") {
		t.Fatalf("light route row1 must not keep pipeline activity, got %q", row1)
	}
	if !strings.Contains(row2, "操作执行") {
		t.Fatalf("light route row2 should keep route summary, got %q", row2)
	}
	for _, banned := range []string{"模型", "MiniMax", "tok", "200k"} {
		if strings.Contains(row2, banned) {
			t.Fatalf("light route row2 leaked stale model telemetry %q: %q", banned, row2)
		}
	}
}

func TestRenderer_StopSpinnerWithoutSummaryClearsDockOnly(t *testing.T) {
	var buf bytes.Buffer
	r := newTestRenderer("zh")
	r.dock = newDock(&buf)
	r.startTime = time.Now().Add(-2 * time.Second)
	r.SetRouteSummary("操作执行", []string{"已完成", "未读仓库"})
	r.SetLightRouteActivity("操作执行")

	buf.Reset()
	r.StopSpinnerWithoutSummary()
	out := stripAnsiEscapes(buf.String())
	if strings.Contains(out, "◇") || strings.Contains(out, "操作执行") || strings.Contains(out, "已完成") {
		t.Fatalf("silent spinner stop must not commit route summary, got %q", out)
	}
	if r.SpinnerActive() {
		t.Fatal("silent spinner stop must clear the live dock")
	}
	if r.routeSummary != nil {
		t.Fatal("silent spinner stop must consume the armed route summary")
	}
}

func TestRenderer_TaskNodeStartShowsPreparingContext(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{Kind: EventAnalysisReady, Timestamp: time.Now(), TaskNodes: []TaskNodeInfo{
		{ID: "fin", Type: "finalize", Objective: "render answer"},
	}})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: time.Now(), NodeID: "fin"})

	rows := r.composeCurrentDockRows()
	if row1 := stripAnsiEscapes(rows[0]); !strings.Contains(row1, "整理上下文中") {
		t.Errorf("node pre-dispatch must not say waiting; got %q", row1)
	}
}

func TestRenderer_FinalizingNoticeUpdatesActivity(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{Kind: EventAnalysisReady, Timestamp: time.Now(), TaskNodes: []TaskNodeInfo{
		{ID: "fin", Type: "finalize", Objective: "render answer"},
	}})
	emit(Event{Kind: EventTaskNodeStart, Timestamp: time.Now(), NodeID: "fin"})
	emit(Event{
		Kind:       EventOrchestratorNotice,
		Timestamp:  time.Now(),
		NoticeKind: NoticeFinalizing,
		Reasoning:  "正在生成最终答案",
	})

	if r.activity.kind != activityFinalizing {
		t.Fatalf("NoticeFinalizing must switch dock activity to finalizing; got %v", r.activity.kind)
	}
	rows := r.composeCurrentDockRows()
	if row1 := stripAnsiEscapes(rows[0]); !strings.Contains(row1, "撰写最终答案") {
		t.Errorf("finalizing notice should update row 1; got %q", row1)
	}
}

func TestRenderer_RepoMapScanProgressUpdatesActivity(t *testing.T) {
	r := newTestRenderer("zh")
	emit := r.Emitter()
	emit(Event{
		Kind:       EventOrchestratorNotice,
		Timestamp:  time.Now(),
		NoticeKind: NoticeRepoMapScanProgress,
		Reasoning:  "· 仓库索引 `linux` 扫描中：已解析 4000/12000 个源文件（总文件 93459）",
	})

	if r.activity.kind != activityRepoMapScanning {
		t.Fatalf("repo-map scan progress must switch dock activity; got %v", r.activity.kind)
	}
	if !strings.Contains(r.streamTail, "4000/12000") {
		t.Fatalf("repo-map scan progress should populate live tail, got %q", r.streamTail)
	}
}

// TestFinalDockSummary_StageCountFiltersSubRows is the regression
// guard for the 2026-05-08 "N 阶段" bug, rewired onto the real
// counter (runSummaryTotalsLocked) after §29.174 RUN2AUDIT-1 F3
// exposed the earlier mirror-loop copy as a fake pin. A bare
// read-mode question walks 4 display stages — analyze, explore,
// extract, finalize — and sub-node / sub-agent rows must never
// inflate that count: node rows fold into their display SLOT (the
// explore family shares 2/4), and sub-agent rows contribute no slot
// at all.
//
// tools is the sum across ALL rows (each tool call increments
// exactly one row); rounds is the run-cumulative LLM-round sum
// (per-row cumulative rounds, legacy iteration fallback), NOT the
// old max(row.iteration) which under-reported multi-row runs.
func TestFinalDockSummary_StageCountFiltersSubRows(t *testing.T) {
	r := newTestRenderer("zh")
	t0 := time.Now()
	// 4 stage rows (all completed) — the right count.
	for _, stage := range []string{"analyze", "explore", "extract", "finalize"} {
		r.tasks = append(r.tasks, &taskRow{
			stage:     stage,
			endTime:   t0.Add(time.Second),
			toolCount: 2, // 8 tools total
			iteration: 3, // 4 x 3 rounds
		})
	}
	// 2 explore-family sub-node rows (also completed) — same display
	// slot as the explore stage row: no extra stage counted.
	for _, kind := range []string{"probe", "evidence"} {
		r.tasks = append(r.tasks, &taskRow{
			isNodeRow: true,
			nodeKind:  kind,
			endTime:   t0.Add(time.Second),
			toolCount: 1, // +2 -> tools=10
			iteration: 5, // +10 rounds
		})
	}
	// 1 sub-agent row (also completed) — tools/rounds counted, no slot.
	r.tasks = append(r.tasks, &taskRow{
		isSubAgent: true,
		endTime:    t0.Add(time.Second),
		toolCount:  1, // +1 -> tools=11
		iteration:  2, // +2 rounds
	})

	completed, tools, rounds := r.runSummaryTotalsLocked()

	if completed != 4 {
		t.Errorf("completed = %d, want 4 (analyze + explore + extract + finalize display slots; sub-node rows share the explore slot, sub-agent rows contribute none)", completed)
	}
	// 4 stage rows x 2 + 2 sub-node x 1 + 1 sub-agent x 1 = 11.
	if tools != 11 {
		t.Errorf("tools = %d, want 11 (sum across all rows including sub-rows)", tools)
	}
	// Run-cumulative rounds: 4x3 + 2x5 + 2 = 24.
	if rounds != 24 {
		t.Errorf("rounds = %d, want 24 (sum across all rows, not max)", rounds)
	}
}

// TestRunSummaryTotals_ResumeAndHiddenProbeAccumulate pins §29.174
// RUN2AUDIT-1 F3 against the runnable_2.txt witness shape: the
// shutdown footer must keep counting (a) tool calls and LLM rounds
// fired during a HIDDEN probe dispatch (EventTaskNodeStart with a
// NodeID that emitAnalysisReady filtered out of TaskNodes — no row
// exists, r.current stays nil), and (b) rounds across a node
// re-dispatch whose agent iteration counter restarts at 0. The
// witness footer read "1 阶段 · 16 次工具调用 · 2 轮 LLM 对话" for a
// 4-stage / 31-call / ~21-round session.
func TestRunSummaryTotals_ResumeAndHiddenProbeAccumulate(t *testing.T) {
	r := newTestRenderer("zh")
	t0 := time.Now()
	ev := func(e Event) { r.handleEvent(e) }

	// Stage 1: analyze — 1 round, 1 tool.
	ev(Event{Kind: EventStageStart, Stage: "analyze", Agent: "analyzer", Timestamp: t0})
	ev(Event{Kind: EventAgentThinking, Stage: "analyze", Iteration: 0, Timestamp: t0})
	ev(Event{Kind: EventToolCallEnd, Stage: "analyze", ToolName: "emit_analysis", ToolOK: true, Timestamp: t0})
	ev(Event{Kind: EventAnalysisReady, Timestamp: t0, TaskNodes: []TaskNodeInfo{
		{ID: "n1_evidence_t0", Type: "evidence", Objective: "collect"},
		{ID: "n2_validate", Type: "validate", Objective: "check"},
		{ID: "n3_extract", Type: "extract", Objective: "distill"},
		{ID: "n4_finalize", Type: "finalize", Objective: "answer"},
	}})

	// Hidden probe dispatch: NodeID unknown to the row set (probe
	// nodes are filtered out of TaskNodes). 2 rounds + 3 tool calls
	// land with r.current == nil and must NOT vanish.
	ev(Event{Kind: EventTaskNodeStart, NodeID: "n0_probe", NodeKind: "probe", Timestamp: t0})
	ev(Event{Kind: EventAgentThinking, Stage: "explore", Iteration: 0, Timestamp: t0})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "trace_query", ToolOK: true, Timestamp: t0})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "trace_query", ToolOK: true, Timestamp: t0})
	ev(Event{Kind: EventAgentThinking, Stage: "explore", Iteration: 1, Timestamp: t0})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "trace_query", ToolOK: true, Timestamp: t0})

	// Evidence node: first dispatch runs 2 rounds / 2 tools, then a
	// checkpoint resume re-starts the node and the agent iteration
	// counter rewinds to 0 (1 more round / 1 more tool).
	ev(Event{Kind: EventTaskNodeStart, NodeID: "n1_evidence_t0", NodeKind: "evidence", Timestamp: t0})
	ev(Event{Kind: EventAgentThinking, Stage: "explore", Iteration: 0, Timestamp: t0})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "trace_query", ToolOK: true, Timestamp: t0})
	ev(Event{Kind: EventAgentThinking, Stage: "explore", Iteration: 1, Timestamp: t0})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "emit_investigation_complete", ToolOK: true, Timestamp: t0})
	ev(Event{Kind: EventTaskNodeStart, NodeID: "n1_evidence_t0", NodeKind: "evidence", Timestamp: t0.Add(2 * time.Second)})
	ev(Event{Kind: EventAgentThinking, Stage: "explore", Iteration: 0, Timestamp: t0.Add(2 * time.Second)})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "emit_investigation_complete", ToolOK: true, Timestamp: t0.Add(2 * time.Second)})
	ev(Event{Kind: EventTaskNodeEnd, NodeID: "n1_evidence_t0", Timestamp: t0.Add(3 * time.Second)})

	// Resume must not rewind the row's cumulative rounds: 3 rounds
	// total even though the last segment's iteration says "1".
	if row := r.findNodeRow("n1_evidence_t0"); row == nil {
		t.Fatalf("evidence row missing")
	} else if got := rowRoundsForDisplay(row); got != 3 {
		t.Errorf("evidence cumulative rounds = %d, want 3 (2 pre-resume + 1 post-resume; iteration overwrite must not rewind the ledger)", got)
	}

	// Validate node: 1 round / 1 tool.
	ev(Event{Kind: EventTaskNodeStart, NodeID: "n2_validate", NodeKind: "validate", Timestamp: t0.Add(3 * time.Second)})
	ev(Event{Kind: EventAgentThinking, Stage: "explore", Iteration: 0, Timestamp: t0.Add(3 * time.Second)})
	ev(Event{Kind: EventToolCallEnd, Stage: "explore", ToolName: "emit_investigation_complete", ToolOK: true, Timestamp: t0.Add(3 * time.Second)})
	ev(Event{Kind: EventTaskNodeEnd, NodeID: "n2_validate", Timestamp: t0.Add(4 * time.Second)})

	// Extract + finalize nodes complete (finalize: 1 round / 1 tool).
	ev(Event{Kind: EventTaskNodeStart, NodeID: "n3_extract", NodeKind: "extract", Timestamp: t0.Add(4 * time.Second)})
	ev(Event{Kind: EventTaskNodeEnd, NodeID: "n3_extract", Timestamp: t0.Add(4 * time.Second)})
	ev(Event{Kind: EventTaskNodeStart, NodeID: "n4_finalize", NodeKind: "finalize", Timestamp: t0.Add(4 * time.Second)})
	ev(Event{Kind: EventAgentThinking, Stage: "finalize", Iteration: 0, Timestamp: t0.Add(4 * time.Second)})
	ev(Event{Kind: EventToolCallEnd, Stage: "finalize", ToolName: "emit_answer_document", ToolOK: true, Timestamp: t0.Add(5 * time.Second)})
	ev(Event{Kind: EventTaskNodeEnd, NodeID: "n4_finalize", Timestamp: t0.Add(5 * time.Second)})

	completed, tools, rounds := r.runSummaryTotalsLocked()
	// Slots: analyze(1/4) + explore family(2/4) + extract(3/4) +
	// finalize(4/4) = 4 真实阶段.
	if completed != 4 {
		t.Errorf("completed = %d, want 4 (analyze/explore/extract/finalize display slots all ended)", completed)
	}
	// 1 analyze + 3 hidden-probe + 3 evidence + 1 validate + 1 finalize = 9.
	if tools != 9 {
		t.Errorf("tools = %d, want 9 (hidden probe segment's 3 calls must stay on the books)", tools)
	}
	// 1 analyze + 2 hidden-probe + 3 evidence + 1 validate + 1 finalize = 8.
	if rounds != 8 {
		t.Errorf("rounds = %d, want 8 (hidden probe rounds + cross-resume accumulation)", rounds)
	}
}
