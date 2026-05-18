package render

import (
	"bytes"
	"strings"
	"testing"
	"time"
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
// states freeze the spinner glyph to ⟳ / ✗ instead of cycling
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
	if !strings.Contains(plain, "⟳") {
		t.Errorf("retry state must use frozen ⟳ glyph; got %q", plain)
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
		stageLabel:    "正在探索代码并收集证据",
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
	for _, want := range []string{"并行 2 路", "3 个关注点", "模型 MiniMax-M2.7-highspeed"} {
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
		if strings.Contains(plain, "并行") || strings.Contains(plain, "关注点") {
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
// at least the "总 Xs" segment when total elapsed is non-zero. The
// time row carries the slowest-changing data and is the user's
// participation reference (they look here when other rows go quiet
// to verify the run is still moving).
func TestComposeDockRow3_TimeOnly(t *testing.T) {
	state := dockRowState{
		stageElapsed: "5s",
		totalElapsed: "12s",
		cancelHint:   "Ctrl+C 取消",
		lang:         "zh",
	}
	row := composeDockRow3(state)
	plain := stripAnsiEscapes(row)
	for _, want := range []string{"本 5s", "总 12s", "Ctrl+C 取消"} {
		if !strings.Contains(plain, want) {
			t.Errorf("row 3 must contain %q; got %q", want, plain)
		}
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
	for _, want := range []string{"2/4", "正在探索代码并收集证据"} {
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
	for _, want := range []string{"2/4", "正在探索代码并收集证据"} {
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
	for _, want := range []string{"2/4", "正在探索代码并收集证据"} {
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
	for _, want := range []string{"2/4", "正在探索代码并收集证据"} {
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
	for _, want := range []string{"2/4", "正在探索代码并收集证据"} {
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
		Kind:            EventParallelDispatchStart,
		Timestamp:       t0.Add(20 * time.Millisecond),
		Stage:           "explore",
		Agent:           "explorer",
		ParallelGroupID: "g",
		ParallelTotal:   2,
		Parallelism:     2,
		ParallelUnitIDs: []string{"n1_evidence_t0", "n1_evidence_t1"},
	})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})
	emit(Event{Kind: EventStageStart, Timestamp: t0.Add(40 * time.Millisecond), Stage: "extract", Agent: "extractor"})

	if r.parallel != nil {
		t.Fatal("extract stage start must clear stale explore parallel state")
	}
	rows := r.composeCurrentDockRows()
	row1 := stripAnsiEscapes(rows[0])
	row2 := stripAnsiEscapes(rows[1])
	if strings.Contains(row1, "并行") || strings.Contains(row2, "并行") || strings.Contains(row2, "关注点") {
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
		Kind:            EventParallelDispatchStart,
		Timestamp:       t0.Add(20 * time.Millisecond),
		Stage:           "explore",
		Agent:           "explorer",
		ParallelGroupID: "g",
		ParallelTotal:   2,
		Parallelism:     2,
		ParallelUnitIDs: []string{"n1_evidence_t0", "n1_evidence_t1"},
	})
	emit(Event{Kind: EventTaskNodeEnd, Timestamp: t0.Add(30 * time.Millisecond), NodeID: "n1_evidence_t0"})

	rows := r.composeCurrentDockRows()
	row1 := stripAnsiEscapes(rows[0])
	row2 := stripAnsiEscapes(rows[1])
	if !strings.Contains(row1, "并行") {
		t.Fatalf("active parallel dispatch must own row1 until the dispatch ends; got %q", row1)
	}
	for _, want := range []string{"2/4", "正在探索代码并收集证据", "并行 2 路", "2 个关注点"} {
		if !strings.Contains(row2, want) {
			t.Fatalf("active parallel explore row missing %q: %q", want, row2)
		}
	}
	if strings.Contains(row2, "3/4") || strings.Contains(row2, "提炼关键发现") {
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
	for _, want := range []string{"2/4", "正在探索代码并收集证据", "并行 2 路"} {
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
	if !strings.Contains(row, "4/4") || !strings.Contains(row, "修复中：正在探索代码并收集证据") {
		t.Fatalf("post-finalize backtrack should keep primary progress and name repair sub-stage; got %q", row)
	}
	if strings.Contains(row, "2/4") {
		t.Fatalf("repair backtrack must not make the primary progress regress; got %q", row)
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
// guard for the 2026-05-08 "N 阶段" bug. A bare read-mode question
// (no log / trace attached) walks 4 stages — analyze, explore,
// extract, finalize — but the dock displayed "6 阶段" because the
// completion counter looped over EVERY r.tasks row, including the
// isNodeRow rows EventAnalysisReady inserts for the explore family
// sub-nodes (probe / evidence / validate / chain) and any
// isSubAgent rows EventSubAgentStart appends.
//
// Fix: the loop now skips isNodeRow + isSubAgent before
// incrementing completed. totalTools (sum) and totalIters (max)
// stay unfiltered — those metrics are semantically correct across
// every row.
func TestFinalDockSummary_StageCountFiltersSubRows(t *testing.T) {
	r := newTestRenderer("zh")
	t0 := time.Now()
	// 4 stage rows (all completed) — the right count.
	for _, stage := range []string{"analyze", "explore", "extract", "finalize"} {
		r.tasks = append(r.tasks, &taskRow{
			stage:     stage,
			endTime:   t0.Add(time.Second),
			toolCount: 2, // 8 tools total
			iteration: 3, // max iter = 3
		})
	}
	// 2 explore-family sub-node rows (also completed).
	for _, kind := range []string{"probe", "evidence"} {
		r.tasks = append(r.tasks, &taskRow{
			isNodeRow: true,
			nodeKind:  kind,
			endTime:   t0.Add(time.Second),
			toolCount: 1, // +2 -> totalTools=10
			iteration: 5, // max iter = 5 (sub-node deeper than stage)
		})
	}
	// 1 sub-agent row (also completed).
	r.tasks = append(r.tasks, &taskRow{
		isSubAgent: true,
		endTime:    t0.Add(time.Second),
		toolCount:  1, // +1 -> totalTools=11
		iteration:  2,
	})

	completed := 0
	totalTools := 0
	totalIters := 0
	for _, row := range r.tasks {
		if row == nil {
			continue
		}
		if !row.isNodeRow && !row.isSubAgent && !row.endTime.IsZero() {
			completed++
		}
		totalTools += row.toolCount
		if row.iteration > totalIters {
			totalIters = row.iteration
		}
	}

	if completed != 4 {
		t.Errorf("completed = %d, want 4 (analyze + explore + extract + finalize; sub-node + sub-agent rows must NOT be counted as stages)", completed)
	}
	// totalTools intentionally aggregates across ALL rows (each tool
	// call increments exactly one current-row counter, so the sum is
	// the unique-call count). 4 stage rows x 2 + 2 sub-node x 1 + 1
	// sub-agent x 1 = 11.
	if totalTools != 11 {
		t.Errorf("totalTools = %d, want 11 (sum across all rows including sub-rows)", totalTools)
	}
	// totalIters is max(row.iteration) across ALL rows — sub-node
	// iter (5) is deeper than the stage iters (3), so the deepest
	// ReAct chain is the sub-node's.
	if totalIters != 5 {
		t.Errorf("totalIters = %d, want 5 (max across all rows)", totalIters)
	}
}
