package render

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// TestDock_FirstPaintNoRewind locks the load-bearing first-paint
// invariant: the very first paintDock MUST NOT issue \x1b[3A. There
// is no prior dock to rewind over; doing so would scroll terminal
// content above the cursor up off-screen and corrupt the user's
// scrollback before the dock has even appeared.
func TestDock_FirstPaintNoRewind(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"row1", "row2", "row3"})
	out := buf.String()
	if strings.Contains(out, "\x1b[3A") {
		t.Errorf("first paintDock must NOT emit \\x1b[3A; got: %q", out)
	}
	if want := "row1\nrow2\nrow3\n"; out != want {
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
// emits the \x1b[3A\x1b[J rewind+clear sequence so the previous
// dock visual is wiped before the new rows are drawn. Without this
// every paint would stack a fresh 3 rows below the previous, which
// is the exact "drift" symptom the dock redesign aimed to eliminate.
func TestDock_SubsequentPaintRewinds(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"a", "b", "c"})
	buf.Reset()
	d.paintDock([dockRowCount]string{"d", "e", "f"})
	out := buf.String()
	if !strings.HasPrefix(out, "\x1b[3A\x1b[J") {
		t.Errorf("subsequent paintDock must start with \\x1b[3A\\x1b[J; got: %q", out)
	}
	if want := "\x1b[3A\x1b[Jd\ne\nf\n"; out != want {
		t.Errorf("subsequent paint output = %q, want %q", out, want)
	}
}

// TestDock_CommitToScrollbackOrder pins the byte sequence of a
// commit while the dock is dirty: rewind → body → '\n' → 3 rows →
// '\n' each. Order matters because the body lands ABOVE the
// re-painted dock, which is what gives the user the illusion of
// "scrollback grows under a fixed dock".
func TestDock_CommitToScrollbackOrder(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.paintDock([dockRowCount]string{"r1", "r2", "r3"})
	buf.Reset()
	d.commitToScrollback("✓ stage done\n", [dockRowCount]string{"r1b", "r2b", "r3b"})
	out := buf.String()
	want := "\x1b[3A\x1b[J✓ stage done\nr1b\nr2b\nr3b\n"
	if out != want {
		t.Errorf("commit byte sequence = %q, want %q", out, want)
	}
}

// TestDock_CommitWhenInactiveSkipsRewind verifies that when the
// dock has not been painted yet (first commit before first paint),
// commitToScrollback writes the body directly + paints fresh —
// without an erroneous \x1b[3A that would scroll real terminal
// content off-screen.
func TestDock_CommitWhenInactiveSkipsRewind(t *testing.T) {
	var buf bytes.Buffer
	d := newDock(&buf)
	d.commitToScrollback("first line\n", [dockRowCount]string{"a", "b", "c"})
	out := buf.String()
	if strings.Contains(out, "\x1b[3A") {
		t.Errorf("commit when dock inactive must NOT rewind; got: %q", out)
	}
	if !strings.HasPrefix(out, "first line\n") {
		t.Errorf("commit body must come first; got: %q", out)
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
	if !strings.HasPrefix(buf.String(), "\x1b[3A\x1b[J") {
		t.Errorf("clearDock must emit \\x1b[3A\\x1b[J; got: %q", buf.String())
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
	emit(Event{Kind: EventAgentThinking, Timestamp: t0.Add(20 * time.Millisecond), Iteration: 0})
	if r.activity.kind != activityRequesting {
		t.Errorf("EventAgentThinking must flip activity to requesting; got %v", r.activity.kind)
	}
}
