package render

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/ansi"
	"github.com/charmbracelet/glamour/styles"
	"github.com/mattn/go-runewidth"
	"github.com/muesli/termenv"
	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/types"
)

// reAnsi matches ANSI CSI sequences (`ESC [ … final-byte`). Used by
// truncByDisplayWidth to step over style escapes without counting
// them as visible columns.
var reAnsi = regexp.MustCompile(`\x1b\[[0-9;:?]*[A-Za-z]`)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// taskRow is one line of the live task list — either a main-pipeline
// stage dispatch or a sub-agent run. Each Event from the pipeline
// maps onto a taskRow: EventStageStart appends a new row, the
// thinking / tool-call events update its detail in place, and
// EventStageEnd / EventSubAgentEnd mark it completed. This replaces
// the pre-2026-04-17 single-slot {agentName, detail, subRunning}
// representation that collapsed all concurrent work into one line
// and showed only the most-recent event, losing every previous
// stage's progress when the DAG scheduler dispatched multiple
// explore rounds or when the analyzer output multi-topic sub_topics.
type taskRow struct {
	stage       string    // "analyze" / "explore" / "extract" / "finalize"
	agent       string    // concrete agent name
	startTime   time.Time // when EventStageStart / EventSubAgentStart / EventTaskNodeStart fired
	firstStart  time.Time // first start across retries; preserves aggregate stage-slot duration
	endTime     time.Time // zero while running
	okFinished  bool      // true when endTime > 0 and error == ""
	errorMsg    string    // populated by EventStageEnd with non-empty Error
	detail      string    // latest tool/iteration/sub-task description
	detailDone  bool      // true when detail refers to a completed tool call
	detailStart time.Time // for the "current detail elapsed" display
	toolCount   int       // cumulative EventToolCallEnd successes for this row
	iteration   int       // latest ReAct iteration seen on this row

	// activityKind / activityDetail / activityDurMs surface the LLM-
	// interaction sub-state inside the running stage's live bar so
	// the user reads "what's happening RIGHT NOW" (request dispatched
	// / receiving stream) without having to interpret the raw event
	// log. activityPhrase consumes these to build the localized
	// segment.
	activityKind   activityKind
	activityDetail string
	activityDurMs  int

	// streamTail is the rolling 20-30 char tail of the most recent
	// streamed assistant content. Updated on every EventAgentContent
	// (~4 Hz throttle from BaseAgent). Rendered as the final segment
	// of the live bar in a distinct dim style so the user can SEE
	// bytes flowing in even when the activity label is the static
	// "请求模型中". Cleared on EventTransition / EventTaskNodeStart /
	// EventTaskNodeEnd so a stale tail from the previous stage doesn't
	// leak into the next one.
	streamTail string
	isSubAgent bool   // true for rows created by EventSubAgentStart
	subAgentID string // identifier for matching EventSubAgentEnd
	subTitle   string // SubTaskTitle from the event
	subCount   int    // SubTaskCount from the event (batch size)

	// Task-graph node rows (post-EventAnalysisReady). nodeID is the
	// TaskGraph node identifier; nodeKind is the TaskNode.Type string
	// ("evidence" / "validate" / "reconcile" / "finalize"); objective
	// is the node's short objective text rendered as the row label.
	// pending is true when the row has been created but the node has
	// not yet run once — distinguishes "planned" rows from rows that
	// are actively in flight.
	//
	// paused is true when the node was started at some point but the
	// scheduler has moved its active dispatch focus elsewhere (e.g.
	// the node was requeued without an EventTaskNodeEnd because its
	// SuccessCriteria didn't pass, and a different node is now in
	// flight). The orchestrator intentionally does NOT emit
	// EventTaskNodeEnd on requeue — the node is conceptually "still
	// in investigation" — but visually rendering it as `running` next
	// to the actually-active row reads as concurrent execution. paused
	// rows render identically to pending rows (`·` glyph + DarkGray +
	// "待 X" text) so the user reads only ONE active stage at a time.
	isNodeRow            bool
	nodeID               string
	nodeKind             string
	objective            string
	hasInvestigationUnit bool
	investigationUnit    types.InvestigationUnit
	pending              bool
	paused               bool

	// skipped marks a node that completed without dispatching its
	// agent — SkipOnFirstVisit (plan loaded from disk) or
	// --skip-verify short-circuit. The completion line uses a
	// dedicated "已加载" / "已跳过" phrase instead of the regular
	// "已 X" so the user reads honest provenance.
	skipped bool

	// dispatchGen identifies which dispatch window this row's
	// active phase belongs to. Nodes started within the same
	// orchestrator window (the for-loop in orchestrator.go::
	// runReadSchedulerLoop that emitNodeStart's every n in
	// `window` back-to-back) share a generation; the next window
	// increments the generation and parks every older-generation
	// row. Without this distinction, a multi-evidence-topic window
	// (evidence_t0, evidence_t1, evidence_t2 all dispatched by ONE
	// LLM call) would have its first two siblings parked the
	// instant the third fires, even though all three are
	// simultaneously in flight.
	dispatchGen int
}

// Renderer consumes pipeline events and produces styled terminal output.
// Uses pterm.Area for in-place updating of task list and status during
// pipeline execution, and Glamour for final markdown rendering.
//
// Concurrent-work representation: the renderer carries a slice of
// taskRow values in insertion order. Every StageStart / SubAgentStart
// appends; End events mutate the matching row in place. Tool-call and
// thinking events route to `current` (the latest still-running
// non-sub-agent stage row), so a tool call made DURING a sub-agent
// run updates the outer stage's row rather than the sub-agent's.
// This matches the event stream: the orchestrator emits tool-call
// events with the main agent's Agent field even inside sub-agent
// dispatches, so routing by Agent would be ambiguous; routing by
// "latest running stage row" is unambiguous and matches what the
// user sees on screen.
type Renderer struct {
	glamour *glamour.TermRenderer
	out     io.Writer

	mu        sync.Mutex
	dock      *dock // 3-row anchored bottom region (post-2026-04-30 dock redesign)
	startTime time.Time

	// dockEnabled is true when stdout is a TTY AND log_stdout is not
	// requested. When false, every event goes through the
	// handleEventNonTTY path: durable Println-style lines, no live
	// region. Set at StartSpinner time and stays for the lifetime of
	// the spinner.
	dockEnabled bool

	// totalStages controls the K/N denominator on the dock + commit
	// rows. read mode = 6, write mode = 3, chitchat = 0 (renders "—").
	// SetTotalStages installs the value before StartSpinner.
	totalStages int

	// activity is the current row-1 status for the focused row.
	// Updated by handleEvent on every event the dock cares about;
	// read by composeCurrentDockRows. The state machine is "most
	// recent meaningful event wins".
	activity activityState

	// Request telemetry for the latest LLM request observed by the
	// renderer. This is renderer-level rather than task-row-level so
	// row 2 can keep showing the effective model / rough token count
	// across fallback rows, stage notices, direct reviewers, and other
	// control-flow surfaces that are not owned by a BaseAgent row.
	requestModelID               string
	requestContextTokensEstimate int
	requestContextWindowTokens   int

	// streamTail / streamChars carry the live stream preview state.
	// streamTail is the rolling 25-col tail; streamChars is the
	// finalize-only character counter. Both reset on stage / node
	// boundaries so a stale tail can't leak.
	streamTail  string
	streamChars int

	// localStageKey is the stage currently doing orchestrator-local
	// CPU work (contract checks, scheduler bookkeeping, forced-read
	// setup) when no agent / node row is active. It is intentionally
	// a renderer-side stage cursor, not a task row: local phases are
	// often short and should not add durable timeline rows, but the
	// live dock still needs an honest stage label while they run.
	localStageKey string

	// parallel carries the currently active bounded fan-out group,
	// if any. It is render-only telemetry: orchestrator events open
	// and close the group, while agent/tool events tagged with the
	// group/unit IDs update per-worker activity. composeCurrentDockRows
	// folds it into row 1 and row 2 so parallel exploration reads as
	// parallel work, not as the last worker's single-lane status.
	parallel *parallelActivity

	// dockSuppressed forces the dock off even on TTY stdout. Set by
	// SetDockEnabled(false) when --log-stdout is in effect — logger
	// writes to stdout would interleave with paintDock and break the
	// 3-row anchor invariant. Without this, every log line would
	// shift the dock down by 1 row permanently.
	dockSuppressed bool

	// Live task list. Append-only within a Start/Stop cycle; rows
	// are not removed — completed rows stay visible with a
	// done-indicator so the user can read the history of what ran.
	tasks   []*taskRow
	current *taskRow // most recent non-sub-agent row with endTime zero; receives tool / thinking events

	// lastCommittedLine remembers the byte-identical text of the most
	// recent commitLineLocked write so consecutive duplicates (e.g. a
	// soft-notice fired twice in a row by the orchestrator) get
	// suppressed before they paint the scrollback. Cleared implicitly
	// when any DIFFERENT line is committed; nothing reads it outside
	// commitLineLocked, so no external invariant depends on the value.
	lastCommittedLine string

	// analysisReady flips to true on EventAnalysisReady. Once flipped,
	// stage-dispatch events (EventStageStart / EventStageEnd) for
	// explore / extract / finalize are ignored — the task graph's
	// node-lifecycle events (EventTaskNodeStart / EventTaskNodeEnd)
	// own row creation and termination. The analyze stage row stays
	// under stage-dispatch control because it runs BEFORE the task
	// graph exists.
	analysisReady bool

	// objective is the single user-question line displayed above the
	// status list. Populated on EventObjectiveStarted and flipped to
	// done=true on EventObjectiveDone.
	objective     string
	objectiveDone bool
	animFrame     int
	animStop      chan struct{}

	// dispatchGen is the rolling counter for orchestrator dispatch
	// windows. EventTaskNodeStart bumps it whenever the timestamp
	// jumps past dispatchWindowGroupingMs ms from the previous start
	// (signalling a new window). Within-window sibling starts share
	// the current gen so multi-topic evidence dispatches stay all-
	// active together.
	dispatchGen       int
	lastNodeStartAt   time.Time
	lastNodeStartKind string

	// adapterRetryTotal counts EventAdapterRetry events seen during
	// this Run. Surfaced in the dock terminal-summary line so a
	// "✗ Failed · Total elapsed 1m23s (retried 3 times)" makes it
	// obvious that the system tried before failing — distinct from
	// a one-shot failure where N=0. Zero is rendered as no suffix so
	// the success / clean-fail summary stays unchanged.
	//
	// adapterFallbackTotal mirrors EventAdapterFallback so the same
	// summary can name "(model request retried 3 times, switched
	// provider once)" when the LLM fallback chain kicked in.
	adapterRetryTotal    int
	adapterFallbackTotal int

	// answerRewriteTotal counts typed orchestrator notices that re-run or
	// rewrite answer work after a candidate answer exists. It is deliberately
	// separate from adapterRetryTotal: transport backoff and semantic answer
	// repair are different user-visible costs.
	answerRewriteTotal int

	// answerAdvisoryTotal counts typed orchestrator notices where the system
	// proceeds with an answer plus a visible boundary/advisory note instead of
	// triggering another semantic rewrite.
	answerAdvisoryTotal int

	// lang is the user-facing locale code consumed by the status
	// localization layer (status_messages.go). Empty defaults to
	// zh-style output (mirrors the project-wide isZh fallback). Set
	// via SetLang from the orchestrator/REPL after Renderer
	// construction so existing callers stay source-compatible.
	lang string

	// thinkingTruncate controls only durable EventAgentReasoning
	// lines (CLI and REPL share this renderer path). The default is
	// false so model thinking is printed in full; operators can opt
	// back into the legacy 1-2 sentence / 200-char summary via
	// codrax.yaml when terminal noise matters more than traceability.
	thinkingTruncate bool

	// answerDraftPreviewEmitted gates the scrollback copy of the
	// first full emit_answer_document payload. The first payload is
	// initially parked in pendingAnswerDraftPreview: when the same
	// draft is accepted as the final answer, printing it permanently
	// would duplicate the final answer. Commit it only if a tool/gate
	// rejection or a later rewrite proves it is genuinely useful as a
	// first-draft reference.
	answerDraftPreviewEmitted bool
	pendingAnswerDraftPreview []scrollbackLine

	// cancelHint is rendered as a dim trailer line under the task
	// list while the spinner is live. Used by the REPL to surface a
	// "press Ctrl+C to cancel" affordance — without this the spinner
	// silently locks the input box for the duration of the Run with
	// no visible escape hatch. Empty string disables the line so
	// single-shot CLI / non-REPL callers keep their historical
	// byte-identical rendering.
	cancelHint string

	// previewArea / previewRound / previewBuf manage the finalizer's
	// streaming summary preview. previewArea is a separate
	// pterm.AreaPrinter from the spinner area — the first
	// EventLivePreviewChunk after a Clear (or first ever) increments
	// previewRound, stops the spinner, and opens this area. Subsequent
	// chunks call previewArea.Update with the cumulative buffer
	// (header line + Event.PreviewText). EventLivePreviewClear stops
	// the area; PreviewRejected briefly flashes a "已重写" marker
	// before erase so the user knows the just-shown draft was thrown
	// away by the AnswerContract / Tier1 floor gates. Rejected=false
	// is the success-path cleanup before the bordered styled answer
	// prints.
	//
	// Non-TTY short-circuit: when stdout is not a terminal (pipe / >
	// file), Area would print but cannot erase — preview text would
	// leak into the user's piped output and corrupt the file. The
	// chunk handler short-circuits in that case so non-interactive
	// outputs are byte-identical pre/post this feature.
	// previewArea is intentionally NOT a pterm.AreaPrinter — pterm /
	// atomicgo cursor's per-line clear loop (\x1b[2K + \x1b[1A loop)
	// only wipes the row count it tracked from the previous Update's
	// '\n' count. When that count under-estimates the true visible
	// row count (CJK content visually wrapping despite our pre-wrap,
	// off-by-one at the auto-wrap column boundary, or a multiplexer
	// that under-reports terminal width), the difference is left as
	// stale rows ABOVE the cursor that the next Update prints below.
	// The result is the stacking-header bug. ttyPreviewArea uses
	// \x1b[J (clear from cursor to end of screen) instead, so even
	// when our row count is short we still wipe everything below the
	// rewound cursor — the visible-output is robust against width-
	// estimation drift.
	previewArea  *ttyPreviewArea
	previewRound int
	previewBuf   strings.Builder

	// previewLastChunk is the raw preview text from the most recent
	// EventLivePreviewChunk. The shared animation goroutine uses
	// this to refresh the ticker line on every animation tick (so
	// the leading glyph + frame counter animate even when no new
	// chunk arrives). Cleared when previewArea is closed.
	previewLastChunk string

	// routeSummary, when non-nil, overrides the next dock-shutdown
	// summary line. Used by light routes (local / chat) that bypass
	// the full pipeline — the "已结束 · 4 阶段 · 总耗时 7s · …" shape
	// is meaningless for them (no stages / tools / iterations to
	// count) so the dock emits "◇ <label> · <segments> · 总耗时 Xs"
	// instead. Cleared at the end of commitDockShutdownLocked so a
	// subsequent pipeline Run sees the default shape unless the
	// caller re-arms it.
	routeSummary *routeSummary
}

// routeSummary carries the light-route label + detail segments that
// override the next dock shutdown's "已结束 · …" summary line. See
// Renderer.SetRouteSummary / EmitLightRouteSummary.
type routeSummary struct {
	label    string
	segments []string
}

// maxVisibleTasks caps how many rows the live area shows at once.
// When the list exceeds this, the oldest COMPLETED rows are hidden
// first — running rows are always retained so the user can see
// what's in flight. A collapsed-count indicator is rendered in
// their place.
const maxVisibleTasks = 12

// SetTotalStages installs the K/N denominator for the dock's stage
// row. read mode passes 6 (analyze→finalize); write mode passes 3
// (plan→apply→verify); chitchat passes 0 (renders "—" until the
// renderer has stage info). Safe to call any time; subsequent dock
// paints pick the new value up. Default is 6.
func (r *Renderer) SetTotalStages(total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if total < 0 {
		total = 0
	}
	r.totalStages = total
}

// SpinnerActive reports whether the live dock is currently attached.
// REPL routing starts the spinner before turn-policy classification so
// slow classifiers do not leave a blank terminal; light-route handlers
// use this to decide whether to close that pre-armed dock or emit their
// standalone summary line.
func (r *Renderer) SpinnerActive() bool {
	if r == nil {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.dock != nil
}

// SetDockEnabled controls whether the live 3-row dock is drawn. Set
// to false when --log-stdout is active so logger output to stdout
// can't tear the dock's anchor. When false, every event still
// produces durable scrollback via the non-TTY path; only the live
// region is suppressed. Safe to call any time before StartSpinner;
// changing it mid-run has no effect (dock state is captured at
// StartSpinner time).
func (r *Renderer) SetDockEnabled(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.dockSuppressed = !enabled
}

// SetLang installs the user-facing locale code. Used by the status
// localization layer to pick zh / en text for stage labels, thinking
// summaries, tool-call detail, footer and cancel hint. Safe to call
// before or after Start; subsequent redraws pick it up immediately.
func (r *Renderer) SetLang(lang string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lang = lang
}

// SetThinkingTruncate toggles legacy summarisation for durable model
// thinking lines. It affects both TTY and non-TTY rendering because
// CLI single-shot and REPL progress events both pass through
// EventAgentReasoning. The live dock's one-line stream preview is a
// separate fixed-width UI surface and intentionally unaffected.
func (r *Renderer) SetThinkingTruncate(enabled bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.thinkingTruncate = enabled
}

// SetRouteSummary records a light-route label + detail segments that
// will replace the next dock-shutdown summary line. Used by the local
// and chat routes — both run a normal StartSpinner / StopSpinner cycle
// (so wall-clock can be measured) but produce no stages / tool calls /
// iterations to count, leaving the default "已结束 · 0 阶段 · 总耗时 Xs"
// shape misleading. The override emits "◇ <label> · <segments> · 总耗时 Xs"
// instead — same color family as the pipeline summary, hollow diamond
// signalling the lighter-weight path.
//
// Caller is the REPL: invoke BEFORE StopSpinner so commitDockShutdownLocked
// reads the cached value. Cleared automatically after one shutdown so a
// later pipeline Run on the same Renderer sees the default shape.
//
// Nil-safe: SetRouteSummary on a nil receiver is a no-op so the REPL
// caller doesn't have to guard.
// MarkRunFatal flips the dock activity to "已失败" / "failed"
// just before the StopSpinner shutdown. Single repaint so the
// user sees the red ✗ glyph + label for at least one frame, and
// commitDockShutdownLocked picks up the activity to swap the
// "◆ 已结束 · …" success summary for a "✗ 已失败 · 总耗时 Xs"
// failure summary. Reuses the existing fatal palette + glyph;
// no new style is introduced.
//
// Nil-safe; safe to call even when the dock is not active (no-op).
// Callers should invoke this BEFORE StopSpinner so the failure
// state is in place when the shutdown summary commits.
func (r *Renderer) MarkRunFatal() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dock == nil {
		return
	}
	r.activity = activityState{kind: activityErrorFatal}
	r.streamTail = ""
	r.parallel = nil
	r.paintDockLocked()
}

// MarkRunCancelled flips the dock activity to "已取消" /
// "cancelled" before StopSpinner. Mirror of MarkRunFatal — same
// fatal-glyph cluster, distinct label so a Ctrl+C / /cancel run
// never displays as a generic failure.
//
// Nil-safe.
func (r *Renderer) MarkRunCancelled() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dock == nil {
		return
	}
	r.activity = activityState{kind: activityCancelled}
	r.streamTail = ""
	r.parallel = nil
	r.paintDockLocked()
}

func (r *Renderer) SetRouteSummary(label string, segments []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.routeSummary = &routeSummary{label: label, segments: segments}
}

// SetLightRouteActivity switches the live dock into a route-owned activity
// state for REPL flows that do not emit normal pipeline/agent events. It also
// clears request telemetry so stale LLM model/token metadata from a classifier
// or previous pipeline turn does not appear beside local operation progress.
func (r *Renderer) SetLightRouteActivity(label string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activity = activityState{kind: activityLightRoute, detail: strings.TrimSpace(label)}
	r.streamTail = ""
	r.streamChars = 0
	r.requestModelID = ""
	r.requestContextTokensEstimate = 0
	r.requestContextWindowTokens = 0
	r.paintDockLocked()
}

// SetLightRouteActivityDeadline is SetLightRouteActivity plus a live countdown.
// It is used by local operation execution where there is no pipeline task row,
// but the user still needs to see that a long-running command has a bounded
// timeout and how much time remains.
func (r *Renderer) SetLightRouteActivityDeadline(label string, deadline time.Time, timeoutBudget time.Duration) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.activity = activityState{
		kind:          activityLightRoute,
		detail:        strings.TrimSpace(label),
		deadline:      deadline,
		timeoutBudget: timeoutBudget,
	}
	r.streamTail = ""
	r.streamChars = 0
	r.requestModelID = ""
	r.requestContextTokensEstimate = 0
	r.requestContextWindowTokens = 0
	r.paintDockLocked()
}

// EmitLightRouteSummary prints a "◇ <label> · …" scrollback line
// directly, without requiring an active dock cycle. Used by the
// clarify route which never calls StartSpinner (no LLM dispatch).
// When the dock IS live (defensive — clarify shouldn't normally
// hit this path), routes through commitLineLocked so the line lands
// in scrollback without tearing the dock anchor; otherwise writes
// straight to outputWriter and mirrors to the INFO log.
//
// Nil-safe.
func (r *Renderer) EmitLightRouteSummary(label string, segments []string) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	line := FormatLightRouteSummary(label, segments, "", r.lang)
	if r.dock != nil {
		r.commitLineLocked(line)
		return
	}
	fmt.Fprintln(r.outputWriter(), line)
	mirrorDockLineToLog(line)
}

// SetOutput retargets all live renderer output. nil restores stdout.
func (r *Renderer) SetOutput(w io.Writer) {
	if r == nil {
		return
	}
	if w == nil {
		w = os.Stdout
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.out = w
}

func (r *Renderer) outputWriter() io.Writer {
	if r != nil && r.out != nil {
		return r.out
	}
	return os.Stdout
}

// New creates a Renderer.
func New(out interface{}, forceColor bool) *Renderer {
	writer, _ := out.(io.Writer)
	if writer == nil {
		writer = os.Stdout
	}
	color := forceColor
	if !color {
		color = isTTY(writer)
	}
	if !color {
		pterm.DisableColor()
	}

	// Disable glamour word wrap — it counts runes, not display
	// columns, so CJK text overflows. The REPL does its own
	// ANSI-aware, display-width wrapping after rendering.
	gr, _ := glamour.NewTermRenderer(
		glamour.WithStyles(codraxStyleConfig()),
		glamour.WithWordWrap(0),
	)

	return &Renderer{glamour: gr, out: writer}
}

// codraxStyleConfig returns the glamour style used for rendering the
// final answer markdown.
//
// Design intent (post-2026-04-29 audit):
//  1. Hue discipline. Each colour family carries ONE semantic so
//     the eye isn't asked to disambiguate four reds:
//     red    → diagnostics + diff-deleted (only)
//     green  → success + diff-inserted + function name
//     cyan   → inline code + heading family
//     grey   → structure (HR, table grid, comments, operators,
//     dimmed heading levels)
//     yellow → emphasis / built-ins (NOT errors)
//     purple → reserved keywords (class / def / func)
//  2. Heading hierarchy via brightness gradient. H1 = brightest
//     (white), gradient down through cyan→blue→grey→deep grey at
//     H6. Pure prefix-count cues ('## ', '### ') are kept by
//     glamour but no longer the sole signal of level.
//  3. No background colours. Terminal theme owns the background;
//     reintroducing pills/bars has been rejected repeatedly as
//     visually loud across the popular terminal themes.
func codraxStyleConfig() ansi.StyleConfig {
	var cfg ansi.StyleConfig
	dark := termenv.HasDarkBackground()
	if dark {
		cfg = styles.DarkStyleConfig
	} else {
		cfg = styles.LightStyleConfig
	}
	applyHeadingHierarchy(&cfg, dark)
	applyInlineAndStructure(&cfg, dark)
	applyChromaPalette(&cfg, dark)
	return cfg
}

// applyHeadingHierarchy installs a brightness-graded set of heading
// colours so the user perceives heading level by colour AND by '#'
// prefix. Pre-2026-04-29 every heading inherited Heading.Color (cyan
// 39 in dark, blue 27 in light) so H2-H6 were visually identical;
// the 2026-04-30 audit then over-corrected by darkening H3/H4 so
// far ("75" / "67") that the user reported "### xxx 字体颜色就很
// 暗，看着很不协调". This third pass brightens the ladder one full
// step so every active heading (H1-H4) reads as clearly visible
// against a typical dark terminal, while H5/H6 retreat to italic
// grey to mark them as supporting structure.
//
// Hue identity:
//
//	H1 — pure white / black: the "title" tier, neutral. Bold.
//	H2 — bright cyan: dominant section heading. Bold.
//	H3 — light cyan: subsection. Bold; matches the inline-code hue
//	     so heading + code references read as the same family.
//	H4 — mid blue: minor heading. Bold.
//	H5 — light grey: italic, not bold; visibly retreated.
//	H6 — deep grey: italic, not bold; bottommost.
//
// All four active levels (H1-H4) now sit at clearly readable
// luminance on dark backgrounds; the gradient discriminator is
// hue + bold/italic, not "drop the brightness 30% per level".
func applyHeadingHierarchy(cfg *ansi.StyleConfig, dark bool) {
	cfg.H1.BackgroundColor = nil
	if dark {
		// New ladder (post-2026-04-30 redesign): two saturated blues
		// at the top, then pure gray-scale gradient for H3-H6. The
		// "color → gray" shift is what gives the eye an immediate
		// sense of hierarchy without using six different hues. Same
		// principle glow / GitHub-web / Solarized all converged on:
		// brightness, not hue saturation, signals heading depth.
		//
		// 15  — pure white #ffffff       (H1: title, neutral)
		// 39  — DodgerBlue #00afff        (H2: dominant section)
		// 252 — light gray #d0d0d0        (H3: subsection, "step down")
		// 245 — mid gray #8a8a8a          (H4: minor heading)
		// 240 — darker gray #585858       (H5: support structure)
		// 238 — deep gray #444444         (H6: bottommost)
		//
		// H1 stays white — it's the only "title" tier and benefits
		// from neutral max contrast. H2 is the only saturated
		// non-white in the ladder; once past H2 we transition fully
		// to gray so eye does not have to discriminate among four
		// blue/cyan shades the way the pre-2026-04-30 ladder forced.
		cfg.H1.Color = strPtr("15")  // pure white
		cfg.H2.Color = strPtr("39")  // DodgerBlue #00afff
		cfg.H3.Color = strPtr("252") // light gray #d0d0d0
		cfg.H4.Color = strPtr("245") // mid gray #8a8a8a
		cfg.H5.Color = strPtr("240") // darker gray #585858
		cfg.H6.Color = strPtr("238") // deep gray #444444
		// Heading.Color is the cascade fallback for any H level
		// without its own Color; align with H4 (mid gray) so an
		// unexpected H7+ reads as "deep structural" rather than
		// suddenly jumping back into the cyan family.
		cfg.Heading.Color = strPtr("245")
	} else {
		// Light-background mirror: deep blue at top, gray gradient
		// down. Inverted luminance so dark text on light reads
		// bolder up top and fades toward structural mid-gray.
		cfg.H1.Color = strPtr("16")  // black
		cfg.H2.Color = strPtr("21")  // pure blue #0000ff
		cfg.H3.Color = strPtr("240") // dark gray
		cfg.H4.Color = strPtr("244") // mid gray
		cfg.H5.Color = strPtr("247") // mid-light gray
		cfg.H6.Color = strPtr("250") // light gray
		cfg.Heading.Color = strPtr("244")
	}
	cfg.H1.Bold = boolPtr(true)
	cfg.H2.Bold = boolPtr(true)
	cfg.H3.Bold = boolPtr(true)
	cfg.H4.Bold = boolPtr(true)
	// H5 + H6 stay non-bold so the brightness drop carries the
	// level signal alone. Italics also off (user explicitly asked
	// "avoid italic on headings — express hierarchy via colour").
	cfg.H5.Bold = boolPtr(false)
	cfg.H5.Italic = boolPtr(false)
	cfg.H6.Bold = boolPtr(false)
	cfg.H6.Italic = boolPtr(false)
}

// applyInlineAndStructure tunes non-heading prose elements: inline
// code, links, horizontal rule, block quote indent, table grid,
// bold/emphasis runs, and list-item bullets.
//
// Hue discipline (post-2026-04-30 redesign):
//
//	Two saturated families + gray-scale neutral + one warm accent.
//	Every prose element belongs to exactly ONE family so the eye
//	reads "kind of thing" by hue without learning a six-color
//	vocabulary:
//
//	  blue   39 (#00afff)   — interactive: links, list bullets
//	                           (lists + links share "navigable"
//	                           semantic; both come from "follow
//	                           this elsewhere")
//	  amber  215 (#ffaf5f)  — code: inline `code`, BlockQuote
//	                           (cross-family warm color; reads
//	                           as "literal payload from elsewhere")
//	  gray   238/240/245/252 — structure: HR, table grid, headings
//	                           H3-H6, Emph, dim notes
//	  white  15             — Strong + H1 (max contrast peaks)
//
// Strong = pure white + bold (no color override) is the GitHub-web
// contract: bold text is its OWN signal, no need to add color on
// top. Emph = gray italic for the same reason on the muted side.
// Strikethrough, Enumeration, Task are new (post-2026-04-30) so
// the palette covers every glamour element a real answer can carry.
//
// CLI single-shot path is byte-identical because runSingleShot
// emits busCtx.Mutable.Result() directly, never invoking glamour.
// A byte-identity test in renderer_test.go locks the contract.
func applyInlineAndStructure(cfg *ansi.StyleConfig, dark bool) {
	cfg.Code.BackgroundColor = nil
	if dark {
		// Inline code — cross-family warm peach so identifiers in
		// prose ("`cmd/root.go`") read as "this is a literal,
		// distinct from the surrounding heading-blue or gray prose".
		cfg.Code.Color = strPtr("215") // peach #ffaf5f

		// Horizontal rule — deepest gray. Dividers should be
		// visible without competing for attention.
		cfg.HorizontalRule.Color = strPtr("238") // very dim gray

		// Link / LinkText — DodgerBlue + underline. Web/IDE
		// universal contract: blue + underline = clickable. Both
		// the URL part (`Link`) and the visible text (`LinkText`)
		// share the same color so the rendered link reads as one
		// styled unit; the URL receives extra underline.
		cfg.Link.Color = strPtr("39")
		cfg.Link.Underline = boolPtr(true)
		cfg.LinkText.Color = strPtr("39")
		cfg.LinkText.Underline = boolPtr(true)

		// BlockQuote — pale yellow. rich/glow/gh convention:
		// quotes break out of the prose stream into a soft warm
		// hue so the eye registers "this is borrowed text".
		// Cross-family yellow is loud enough to be noticed without
		// stealing focus from headings.
		cfg.BlockQuote.Color = strPtr("222") // pale yellow #ffd787

		// Strong (`**bold**`) — pure white + bold. The bold
		// attribute itself is the dominant signal; layering color
		// on top would create a third heading-like tier and
		// collide with the H1=15 ladder. Pre-redesign this was
		// "pale cyan 159" which was visually indistinguishable
		// from H3 light-cyan 117.
		cfg.Strong.Color = strPtr("15")
		cfg.Strong.Bold = boolPtr(true)

		// Emphasis (`*italic*`) — light gray + italic. Italic is
		// the dominant signal; gray keeps it muted so emphasised
		// prose reads as "softly highlighted" rather than
		// "alternative-color highlighted".
		cfg.Emph.Color = strPtr("252") // light gray
		cfg.Emph.Italic = boolPtr(true)

		// Strikethrough (`~~text~~`) — mid-dim gray + crossed
		// out. Stuck-through text should "fade out" visually so
		// readers see the deletion intent immediately. Color
		// alone is not enough (bat / sublime convention adds the
		// crossed-out attribute).
		cfg.Strikethrough.Color = strPtr("240")
		cfg.Strikethrough.CrossedOut = boolPtr(true)

		// List bullets (`-` / `*`) — DodgerBlue, sharing the
		// link family so all "structured navigation" markers read
		// as one cohort. NOT bold — bullets are markers, not
		// content; Item body inherits the prose default.
		cfg.Item.Color = strPtr("39")

		// Numbered-list markers (`1.` / `2.`) — same blue as
		// bullets so ordered + unordered lists read as siblings.
		// Pre-redesign this was unset and inherited the terminal
		// default, which made ordered lists visually weaker than
		// unordered (the bullet had color but the digit did not).
		cfg.Enumeration.Color = strPtr("39")

		// Task list checkboxes (`[x]` / `[ ]`) — completed in
		// green, pending in mid-gray. The ticked task is "done"
		// and reads positive; the unticked is "todo" and reads
		// neutral. Glamour exposes the two as separate slots
		// inside StyleTask; we set both.
		cfg.Task.Ticked = "[✓]"
		cfg.Task.Unticked = "[ ]"

		// Table cells + grid — leave StylePrimitive.Color UNSET so
		// cell content inherits the terminal foreground (the same
		// rule prose Text uses; see comment below). The previous
		// `Color = strPtr("238")` setting was a real readability
		// bug: glamour's TableCellElement.Render applies
		// Table.StylePrimitive to ALL cell content (table.go:171),
		// not just the grid. On dark terminal backgrounds (#000 /
		// #1e1e1e / #282828) cells were rendered as #444 — barely
		// distinguishable from the background, the user's reported
		// "字体颜色和背景太接近了，看不清楚". Grid runes are written
		// via lipgloss.Border (table.go:104) which does NOT consume
		// StylePrimitive.Color, so removing the setting only fixes
		// cell visibility — grid lines keep their terminal-default
		// rendering exactly as before.
		cfg.Table.CenterSeparator = strPtr("┼")
		cfg.Table.ColumnSeparator = strPtr("│")
		cfg.Table.RowSeparator = strPtr("─")

		// Text (StylePrimitive fallback) — INTENTIONALLY left
		// nil. The user's terminal foreground colour adapts to
		// their theme (dark vs light vs solarized vs custom);
		// pinning Text.Color here would override that and reduce
		// readability on themes we did not anticipate. glow / bat
		// follow the same convention.
		_ = cfg.Text
	} else {
		// Light-mode mirror — same families inverted to dark
		// hues against light backgrounds. Same semantic mapping:
		// blue interactive, amber code/quote, gray structure,
		// black for max-contrast peaks.
		cfg.Code.Color = strPtr("130")           // dark amber #af5f00
		cfg.HorizontalRule.Color = strPtr("250") // light gray (dim divider on white)
		cfg.Link.Color = strPtr("21")            // pure blue
		cfg.Link.Underline = boolPtr(true)
		cfg.LinkText.Color = strPtr("21")
		cfg.LinkText.Underline = boolPtr(true)
		cfg.BlockQuote.Color = strPtr("130") // dark amber (matches code family)
		cfg.Strong.Color = strPtr("16")
		cfg.Strong.Bold = boolPtr(true)
		cfg.Emph.Color = strPtr("244") // mid gray italic
		cfg.Emph.Italic = boolPtr(true)
		cfg.Strikethrough.Color = strPtr("244")
		cfg.Strikethrough.CrossedOut = boolPtr(true)
		cfg.Item.Color = strPtr("21")
		cfg.Enumeration.Color = strPtr("21")
		cfg.Task.Ticked = "[✓]"
		cfg.Task.Unticked = "[ ]"
		// Light-mode mirror: same fix — leave StylePrimitive.Color
		// unset so cells inherit terminal default; the previous
		// `Color = strPtr("250")` (light gray) made cells nearly
		// invisible on white-bg terminals.
		cfg.Table.CenterSeparator = strPtr("┼")
		cfg.Table.ColumnSeparator = strPtr("│")
		cfg.Table.RowSeparator = strPtr("─")
		_ = cfg.Text
	}
}

// applyChromaPalette repaints the syntax-highlighting palette inside
// fenced code blocks. The 2026-04-30 audit (in response to user
// feedback "追求美感，协调，简洁，大方，清晰") consolidates the
// previous 10-hue dracula-derived palette down to a 6-hue family
// where every token has ONE clear semantic. Fewer hues = calmer
// page and zero ambiguity about what each colour means.
//
// Hue assignments (dark mode reference):
//
//	blue   #5fafff — control-flow keywords (if / for / return ...)
//	purple #bd93f9 — declaration keywords (class / def / func) + decorators
//	cyan   #87d7ff — types + classes + namespace (the "shape" family,
//	                 same hue as inline `code` in prose so types read
//	                 consistently across prose and fenced blocks)
//	green  #87d7af — functions + attributes + diff-inserted (the
//	                 "definition / new content" family)
//	yellow #d7d787 — literal strings + builtins (the "data + lib"
//	                 family; warm but desaturated so it doesn't
//	                 fight the cyan headings above)
//	orange #ffaf87 — literal numbers (sole hue, distinct from strings)
//	slate  #6272a4 — comments + preproc (low-contrast, calm)
//	grey   #909090 — operators + subheading (structural neutral)
//	red    #ff5555 — diff-deleted + exceptions (the ONE place red
//	                 carries semantic load)
//
// Punctuation stays uncoloured so it inherits prose default. Pink /
// orange-pink / extra blue shades the dracula palette previously
// scattered across namespaces / escape sequences / preproc are gone.
//
// CodeBlock.Chroma is a pointer into a shared global StyleConfig, so
// shallow-copy before mutating to avoid leaking edits across the
// process.
func applyChromaPalette(cfg *ansi.StyleConfig, dark bool) {
	if cfg.CodeBlock.Chroma == nil {
		return
	}
	chroma := *cfg.CodeBlock.Chroma
	// Backgrounds (fence fill + error highlight) — clear so the
	// terminal theme shows through.
	chroma.Background.BackgroundColor = nil
	chroma.Error.BackgroundColor = nil
	if dark {
		// Keyword family — control-flow blue, declaration purple,
		// "shape" cyan. KeywordNamespace folds into KeywordType
		// because both denote "where this thing lives" in the type
		// graph; one hue keeps the eye scanning naturally.
		chroma.Keyword.Color = strPtr("#5fafff")          // mid blue
		chroma.KeywordReserved.Color = strPtr("#bd93f9")  // soft purple
		chroma.KeywordNamespace.Color = strPtr("#87d7ff") // light cyan (was its own pale blue)
		chroma.KeywordType.Color = strPtr("#87d7ff")      // light cyan (matches namespace + inline-code)
		// Operator + Punctuation: structural neutral. Operator stays
		// grey so high-frequency `=` / `+` / `<` don't blend with
		// diff markers. Punctuation inherits text colour.
		chroma.Operator.Color = strPtr("#909090")
		chroma.Punctuation.Color = nil
		// Names — the "definition" family in green, "type-shape"
		// in cyan. Tag (HTML/XML) folds into the cyan family because
		// it's a structural shape marker, not a keyword.
		chroma.Name.Color = strPtr("#dadada")          // near-white default
		chroma.NameBuiltin.Color = strPtr("#d7d787")   // soft yellow (matches strings)
		chroma.NameTag.Color = strPtr("#87d7ff")       // cyan (was purple — pulled into shape family)
		chroma.NameAttribute.Color = strPtr("#87d7af") // soft green
		chroma.NameClass.Color = strPtr("#87d7ff")     // cyan + bold + underline (kept; was pure white)
		chroma.NameDecorator.Color = strPtr("#bd93f9") // purple (matches reserved)
		chroma.NameFunction.Color = strPtr("#87d7af")  // soft green
		chroma.NameException.Color = strPtr("#ff5555") // red — exception is the ONE alert hue
		// Literals — string vs number unambiguously distinct.
		chroma.LiteralString.Color = strPtr("#d7d787")       // soft yellow
		chroma.LiteralStringEscape.Color = strPtr("#ffaf87") // orange (was pink) — escapes share orange with numbers
		chroma.LiteralNumber.Color = strPtr("#ffaf87")       // soft orange
		// Comments — slate is calm, low-contrast. Preproc folds in
		// because both are "side-channel" context the reader skims.
		chroma.Comment.Color = strPtr("#6272a4")        // dracula slate
		chroma.CommentPreproc.Color = strPtr("#6272a4") // (was pink — folded into comment family)
		// Diff markers — the load-bearing red + green semantics.
		chroma.GenericDeleted.Color = strPtr("#ff5555")
		chroma.GenericInserted.Color = strPtr("#87d7af") // matches function-green so "added" reads as "definition"
		chroma.GenericSubheading.Color = strPtr("#909090")
		// Default text inside fenced blocks
		chroma.Text.Color = strPtr("#dadada")
	} else {
		// Light theme mirror — same hue-discipline, inverted
		// luminance. Strings + builtins share dark-amber, classes +
		// types + namespace share dark-cyan, functions + attributes
		// share dark-green. Comments retreat to a calm slate-grey.
		chroma.Keyword.Color = strPtr("#005faf")          // dark blue
		chroma.KeywordReserved.Color = strPtr("#6f42c1")  // dark purple
		chroma.KeywordNamespace.Color = strPtr("#0087af") // dark cyan (matches type)
		chroma.KeywordType.Color = strPtr("#0087af")
		chroma.Operator.Color = strPtr("#6e7781")
		chroma.Punctuation.Color = nil
		chroma.Name.Color = strPtr("#24292f")
		chroma.NameBuiltin.Color = strPtr("#9a6700")   // dark amber (matches strings; was warm red)
		chroma.NameTag.Color = strPtr("#0087af")       // dark cyan (was dark green)
		chroma.NameAttribute.Color = strPtr("#116329") // dark green
		chroma.NameClass.Color = strPtr("#0087af")     // dark cyan
		chroma.NameDecorator.Color = strPtr("#6f42c1") // dark purple (was orange)
		chroma.NameFunction.Color = strPtr("#116329")  // dark green (was purple)
		chroma.NameException.Color = strPtr("#cf222e")
		chroma.LiteralString.Color = strPtr("#9a6700")       // dark amber
		chroma.LiteralStringEscape.Color = strPtr("#953800") // dark orange (matches numbers)
		chroma.LiteralNumber.Color = strPtr("#953800")
		chroma.Comment.Color = strPtr("#6e7781")
		chroma.CommentPreproc.Color = strPtr("#6e7781") // (was blue — folded into comment family)
		chroma.GenericDeleted.Color = strPtr("#cf222e")
		chroma.GenericInserted.Color = strPtr("#116329")
		chroma.GenericSubheading.Color = strPtr("#6e7781")
		chroma.Text.Color = strPtr("#24292f")
	}
	cfg.CodeBlock.Chroma = &chroma
}

// strPtr / boolPtr are local *string / *bool helpers used by the
// glamour StylePrimitive overrides. The nil check inside cascadeStyle
// requires a concrete pointer — building one inline with `&"117"` is
// not legal Go on string literals, so this trivial helper serves
// both readability and the language constraint.
func strPtr(s string) *string { return &s }
func boolPtr(b bool) *bool    { return &b }

// buildDarkPalette / buildLightPalette are testable entry points
// into codraxStyleConfig() that bypass termenv detection. Production
// code reaches these through codraxStyleConfig() which picks the
// theme via termenv.HasDarkBackground(); tests invoke them directly
// to assert palette invariants without needing a real terminal.
func buildDarkPalette() ansi.StyleConfig {
	cfg := styles.DarkStyleConfig
	applyHeadingHierarchy(&cfg, true)
	applyInlineAndStructure(&cfg, true)
	applyChromaPalette(&cfg, true)
	return cfg
}

func buildLightPalette() ansi.StyleConfig {
	cfg := styles.LightStyleConfig
	applyHeadingHierarchy(&cfg, false)
	applyInlineAndStructure(&cfg, false)
	applyChromaPalette(&cfg, false)
	return cfg
}

// StartSpinner begins the live status area.
func (r *Renderer) StartSpinner() {
	r.startSpinnerWithHint("")
}

// StartSpinnerWithCancelHint is the REPL-aware variant: shows hint
// as a dim trailer line under the task list so the operator sees the
// cancel affordance the moment the input box closes. Single-shot CLI
// callers keep using StartSpinner() and get historical rendering.
func (r *Renderer) StartSpinnerWithCancelHint(hint string) {
	r.startSpinnerWithHint(hint)
}

func (r *Renderer) startSpinnerWithHint(hint string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dock != nil {
		return
	}
	r.objective = ""
	r.objectiveDone = false
	r.tasks = nil
	r.current = nil
	r.analysisReady = false
	r.startTime = time.Now()
	r.animFrame = 0
	r.cancelHint = hint
	r.activity = activityState{kind: activityWaitingPipeline}
	r.streamTail = ""
	r.streamChars = 0
	r.parallel = nil
	r.answerDraftPreviewEmitted = false
	r.pendingAnswerDraftPreview = nil
	if r.totalStages == 0 {
		// Default matches read mode: 4 dispatch boundaries
		// (analyze + explore + extract + finalize). See the
		// rationale on repl.go's read-mode SetTotalStages call.
		r.totalStages = 4
	}
	r.dockEnabled = isTTY(r.outputWriter()) && !r.dockSuppressed
	if r.dockEnabled {
		r.dock = newDock(r.outputWriter())
		r.paintDockLocked()
		r.startAnimGoroutineLocked()
	}
}

// openSpinnerAreaLocked opens a fresh pterm.AreaPrinter and starts
// the animation ticker WITHOUT touching task / objective / analysis
// state. Used by both startSpinnerWithHint (full spinner start) and
// reopenSpinnerAreaLocked (post-finalize-retry recovery), so the
// area + animation goroutine lifecycle has one canonical owner.
//
// Caller MUST hold r.mu.

// totalElapsedString returns the cumulative pipeline wall clock as a
// truncated-second string ("45s"). Empty when the renderer has not
// been started yet. Caller MUST hold r.mu.
func (r *Renderer) totalElapsedString() string {
	if r.startTime.IsZero() {
		return ""
	}
	d := time.Since(r.startTime)
	if d < 0 {
		return ""
	}
	return d.Truncate(time.Second).String()
}

// startAnimGoroutineLocked starts the 100ms animation goroutine
// that ticks animFrame and repaints the dock so the spinner glyph
// rotates between events.
//
// Caller MUST hold r.mu and ensure no prior goroutine is running.
func (r *Renderer) startAnimGoroutineLocked() {
	r.animStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(dockTickInterval)
		defer ticker.Stop()
		for {
			select {
			case <-r.animStop:
				return
			case <-ticker.C:
				r.mu.Lock()
				if r.dock == nil {
					r.mu.Unlock()
					return
				}
				r.animFrame++
				r.paintDockLocked()
				r.mu.Unlock()
			}
		}
	}()
}

// reopenSpinnerAreaLocked restarts the spinner area + animation
// goroutine after they were torn down by handlePreviewChunkLocked
// (the finalize streaming path stops the spinner so the ticker
// can own the screen). When the orchestrator decides to retry a
// rejected finalize round, fresh EventTaskNodeStart / EventStageStart /
// EventAgentThinking events arrive but r.area is nil — without
// reopening, the renderer silently drops every event and the
// terminal "freezes" at whatever was last printed.
//
// Distinct from startSpinnerWithHint: this MUST preserve r.tasks /
// r.current / r.objective / r.analysisReady so the redraw shows
// the full pipeline timeline (with already-done rows visible) the
// moment the retry round picks up. r.cancelHint and r.startTime
// also stay so elapsed-time math is continuous.
//
// Caller MUST hold r.mu.

func (r *Renderer) stopAnimLocked() {
	// Stop animation goroutine first so it can't race the shutdown
	// commit with a stale paintDock.
	if r.animStop != nil {
		select {
		case <-r.animStop:
		default:
			close(r.animStop)
		}
		r.animStop = nil
	}
}

func (r *Renderer) resetSpinnerStateLocked() {
	r.tasks = nil
	r.current = nil
	r.analysisReady = false
	r.objective = ""
	r.objectiveDone = false
	r.activity = activityState{}
	r.streamTail = ""
	r.streamChars = 0
	r.parallel = nil
}

// StopSpinner ends the dispatch ticker. Stage-completion lines have
// already accrued in scrollback as they happened, so StopSpinner
// adds one closing summary line capturing the run's totals before
// wiping the live ticker — without this the user reports "info gone
// after completion". The persistent "✓ 已撰写最终答案" banner is
// printed by RenderResult, which runs immediately after this on the
// bordered-answer path.
func (r *Renderer) StopSpinner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dock == nil {
		// Nothing to stop. Reset state for next Run.
		r.routeSummary = nil
		r.resetSpinnerStateLocked()
		return
	}
	r.stopAnimLocked()
	// Commit closing summary line + clear dock so the next stdout
	// write (RenderResult / REPL prompt) lands cleanly.
	r.commitDockShutdownLocked()
	r.dock = nil
	r.resetSpinnerStateLocked()
}

// StopSpinnerWithoutSummary clears the live dock without committing a
// route or pipeline shutdown summary. It is for local REPL flows that
// immediately render their own durable result panel, such as each
// operation execution round. Normal pipeline completion must continue
// to use StopSpinner so stage summaries remain visible in scrollback.
func (r *Renderer) StopSpinnerWithoutSummary() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dock != nil {
		r.stopAnimLocked()
		r.dock.clearDock()
		r.dock = nil
	}
	r.routeSummary = nil
	r.resetSpinnerStateLocked()
}

// printRunSummaryLocked prints a closing single-line summary of the
// run as a permanent scrollback record. Must be called with r.mu
// held and BEFORE the bar is wiped — the bar.Stop() call in the
// caller emits a clearing newline so the summary lands cleanly above
// it.
//
// Format:
//   ◆ 已结束 · 6 阶段 · 总耗时 45s · 共 N 工具 · M 轮 LLM 对话
//
// Stage count is the number of completed (endTime non-zero) stage /
// node rows; tool count is the cumulative toolCount across all rows;
// iteration total is the sum of all rows' final iteration counters.
// Empty rows yields a minimal "◆ 已结束 · 总耗时 0s" line so the
// user always sees the boundary between this Run and the next prompt.

// Emitter returns an EventEmitter callback bound to this renderer.
func (r *Renderer) Emitter() EventEmitter {
	return r.dockEventEmitter()
}

// findRunningStageRow returns the most recent main-pipeline row
// matching (stage, agent) that has not yet ended, or nil. Used by
// EventStageEnd to terminate the correct row when the same stage
// re-dispatches (DAG explore windows) within one Run.
func (r *Renderer) findRunningStageRow(stage, agent string) *taskRow {
	for i := len(r.tasks) - 1; i >= 0; i-- {
		row := r.tasks[i]
		if row.isSubAgent {
			continue
		}
		if !row.endTime.IsZero() {
			continue
		}
		if row.stage == stage && row.agent == agent {
			return row
		}
	}
	return nil
}

// findSubAgentRow returns the row with the given subAgentID, or nil.
// Sub-agent IDs are assigned by the orchestrator (trace-scoped unique)
// so a direct match is sufficient.
func (r *Renderer) findSubAgentRow(id string) *taskRow {
	if id == "" {
		return nil
	}
	for i := len(r.tasks) - 1; i >= 0; i-- {
		if r.tasks[i].isSubAgent && r.tasks[i].subAgentID == id {
			return r.tasks[i]
		}
	}
	return nil
}

// topicSuffixRe matches compiler.expandEvidenceNodes's per-sub-topic
// node ID shape: the literal prefix ends with `_tN` where N is the
// zero-based sub-topic index. `n1_evidence_t2` → index 2.
// When the node ID does not carry this suffix (single-topic path),
// the match fails and the caller shows the default "[evidence]" tag.
var topicSuffixRe = regexp.MustCompile(`_t(\d+)$`)

func topicIndexFromNodeID(id string) (int, bool) {
	m := topicSuffixRe.FindStringSubmatch(id)
	if m == nil {
		return 0, false
	}
	n := 0
	// Index returned is already non-negative by regex construction;
	// parse manually to avoid pulling in strconv for one call.
	for _, c := range m[1] {
		n = n*10 + int(c-'0')
	}
	return n, true
}

// findNodeRow returns the task-graph node row with the given nodeID,
// or nil. Task-graph rows are appended once per AnalysisIR node at
// EventAnalysisReady time and stay in place for the remainder of the
// run; the scan walks in insertion order because multiple rows may
// share a short id prefix.
func (r *Renderer) findNodeRow(id string) *taskRow {
	if id == "" {
		return nil
	}
	for _, row := range r.tasks {
		if row.isNodeRow && row.nodeID == id {
			return row
		}
	}
	return nil
}

// redraw rebuilds the area content from current state. Must be called
// with r.mu held.
//
// Every line written here is hard-truncated to the terminal's display
// width before being handed to pterm.Area. The reason: pterm.Area
// tracks how many rows to overwrite by counting the `\n`s it wrote
// last frame, NOT by asking the terminal how many rows the previous
// content actually occupied. If a line was longer than the terminal
// width and the terminal wrapped it onto a second visual row,
// pterm.Area's cursor-up arithmetic is off by one and each new frame
// lands BELOW the old one — visible to the user as the task list and
// spinner "刷屏" / scrolling forever down the screen. The fix is to
// guarantee no line ever wraps, by clamping every emitted line to
// (terminal_width - small margin) display columns.

// previewStreamMeta returns the streamMeta payload for the
// finalize-streaming line when the previewArea is in flight.
// Empty when no streaming is happening.
//
// Must be called with r.mu held.

// composeStatusFrame builds the same content redraw() sends to
// pterm.Area but returns it as a string instead of pushing to the
// area. Used by the preview-handover path: when the first
// EventLivePreviewChunk fires, we want to FREEZE the spinner area's
// last-rendered state as static text above the ticker line so the
// user keeps seeing the validate / reconcile / extract status
// history during retry rounds. Without freeze the area's
// WithRemoveWhenDone(true) erases everything, leaving the ticker
// alone with no context.
//
// Must be called with r.mu held (same as redraw).

// composeStatusFrameWithFilter renders the status frame with an
// optional "drop the still-running finalize row" filter. The freeze-
// for-preview path passes hideRunningFinalize=true so the snapshot
// printed to scrollback shows only the COMPLETED upstream history;
// the live ticker below carries the finalize state. Without this
// filter the frozen line reads "⠙ 正在生成最终答案 …" while the
// ticker right below it animates the same stage — the user reads
// two contradictory live indicators for one row, then later a third
// "✓ 已生成最终答案" persists below both. Filtering eliminates the
// duplicate so the screen reads as one timeline.

// visibleRows returns the task rows to draw this frame. Running rows
// are always retained; when the running + recent-done count exceeds
// maxVisibleTasks the oldest DONE rows are collapsed into a single
// "(+N earlier done)" marker row inserted at the top.

// formatTaskLine: removed in 2026-04-30. The taskRow → screen path
// now goes through buildStatusBlocks + renderStatusBlock so all
// localization and styling lives in one place. Pre-removal callers
// in redraw() were the only consumers.

// pluralS returns "s" when n != 1, "" otherwise. Renderer-local
// plurality helper used by formatTaskLine's tool-count rendering.
func pluralS(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// printAboveArea temporarily clears the pterm.Area, prints text to
// stdout, then redraws the area beneath it. The printed text scrolls
// up and stays visible; the spinner continues below. Must be called
// with r.mu held.

// reasoningMaxChars caps the legacy opt-in reasoning summary shown to the user.
const reasoningMaxChars = 200

type scrollbackLine struct {
	text   string
	visual bool
}

// stripMarkdown removes markdown syntax that clutters a single-line
// thinking trace: headers (###), bold (**), bullets (- / *), code
// fences (```), and leading/trailing whitespace around them.
func stripMarkdown(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		line = stripMarkdownLine(line)
		if IsMarkdownFenceLine(line) {
			continue
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, " ")
}

func stripMarkdownLine(line string) string {
	// Strip header prefixes: "### Title" → "Title".
	for strings.HasPrefix(line, "#") {
		line = strings.TrimLeft(line, "#")
	}
	line = strings.TrimSpace(line)
	// Strip bold markers.
	line = strings.ReplaceAll(line, "**", "")
	// Strip leading bullet.
	if strings.HasPrefix(line, "- ") {
		line = line[2:]
	} else if strings.HasPrefix(line, "* ") {
		line = line[2:]
	}
	return line
}

func IsMarkdownFenceLine(line string) bool {
	line = strings.TrimSpace(line)
	return strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~")
}

func IsRenderedCodeBlockHeaderLine(plain string) bool {
	if plain == "" {
		return false
	}
	return strings.HasPrefix(plain, "─── ") && strings.HasSuffix(plain, " ───")
}

func IsRenderedCodeBlockContinuation(s string) bool {
	if strings.TrimSpace(stripAnsiEscapes(s)) == "" {
		return true
	}
	plain := stripAnsiEscapes(s)
	return strings.HasPrefix(plain, "    ") || strings.HasPrefix(plain, "\t")
}

func StripANSIOnly(s string) string {
	return stripAnsiEscapes(s)
}

func DisplayWidth(s string) int {
	return runewidth.StringWidth(StripANSIOnly(s))
}

func legacyReasoningSummary(text string, truncate bool) string {
	text = stripMarkdown(text)
	if text == "" {
		return ""
	}
	summary := text
	if !truncate {
		return summary
	}
	// Take roughly 1-2 sentences worth.
	if idx := strings.Index(summary, ". "); idx > 0 && idx < reasoningMaxChars-20 {
		// Try to find a second sentence boundary.
		if idx2 := strings.Index(summary[idx+2:], ". "); idx2 > 0 && idx+2+idx2 < reasoningMaxChars {
			summary = summary[:idx+2+idx2+1]
		}
	}
	if len(summary) > reasoningMaxChars {
		cut := reasoningMaxChars - 3
		for cut > 0 && summary[cut] != ' ' {
			cut--
		}
		if cut == 0 {
			cut = reasoningMaxChars - 3
		}
		summary = summary[:cut] + "..."
	}
	return summary
}

// formatReasoning formats LLM reasoning as a quiet line with a
// user-facing stage/round label. Internal agent ids stay in debug logs;
// scrollback uses labels such as "探索 · 第 3 轮" / "Explore · Round 3".
// The body uses statusReasoningBody, which keeps thinking below
// final-answer prose but readable on dark terminal backgrounds. Markdown
// is stripped from prose lines and leading blank lines are skipped so the
// display is clean plain text. Visual rows (tables, box drawings, Mermaid
// source, fenced blocks) stay line-shaped so the REPL does not turn useful
// model-produced diagrams into a single wrapped paragraph. When truncate is
// true the legacy 1-2 sentence / 200-char summary is used; the default caller
// path passes false so CLI and REPL expose the full model thinking trace.
func formatReasoning(agent string, stage types.PipelineStage, iteration int, text string, truncate bool, lang string) string {
	lines := formatReasoningLines(agent, stage, iteration, text, truncate, lang)
	if len(lines) == 0 {
		return ""
	}
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, line.text)
	}
	return strings.Join(out, "\n")
}

func formatReasoningLines(agent string, stage types.PipelineStage, iteration int, text string, truncate bool, lang string) []scrollbackLine {
	return formatReasoningLinesWithParallel(agent, stage, iteration, text, truncate, lang, "")
}

func formatReasoningLinesWithParallel(agent string, stage types.PipelineStage, iteration int, text string, truncate bool, lang string, parallelUnitLabel string) []scrollbackLine {
	trace := activityTraceLabelWithParallel(agent, stage, iteration, lang, parallelUnitLabel)
	prefix := "  " + statusReasoningGlyph.Sprint(string(glyphReasoning)) + " " + statusMeta.Sprint(trace) + " "
	if truncate {
		summary := legacyReasoningSummary(text, true)
		if summary == "" {
			return nil
		}
		return []scrollbackLine{{text: prefix + statusReasoningBody.Sprint(summary)}}
	}
	bodyLines := reasoningDisplayLines(text)
	if len(bodyLines) == 0 {
		return nil
	}
	if len(bodyLines) == 1 && !bodyLines[0].visual {
		return []scrollbackLine{{text: prefix + statusReasoningBody.Sprint(bodyLines[0].text)}}
	}
	out := make([]scrollbackLine, 0, len(bodyLines))
	for i, line := range bodyLines {
		linePrefix := "    "
		if i == 0 {
			linePrefix = prefix
		}
		out = append(out, scrollbackLine{
			text:   linePrefix + statusReasoningBody.Sprint(line.text),
			visual: line.visual,
		})
	}
	return out
}

func reasoningDisplayLines(text string) []scrollbackLine {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = expandInlineReasoningVisualRows(text)
	raw := strings.Split(text, "\n")
	tableLines := MarkdownTableLineMask(raw)
	lines := make([]scrollbackLine, 0, len(raw))
	inFence := false
	hasVisual := false
	prevBlank := false
	for i, rawLine := range raw {
		plain := strings.TrimSpace(stripAnsiEscapes(rawLine))
		fenceLine := IsMarkdownFenceLine(plain)
		visual := inFence || fenceLine || tableLines[i] || ShouldPreserveVisualLine(rawLine)
		clean := rawLine
		if visual {
			clean = strings.TrimRight(clean, " \r")
			hasVisual = true
		} else {
			clean = stripMarkdownLine(clean)
		}
		blank := strings.TrimSpace(stripAnsiEscapes(clean)) == ""
		if blank && !visual {
			if prevBlank {
				continue
			}
			prevBlank = true
			continue
		}
		prevBlank = blank && !visual
		if clean != "" || visual {
			lines = append(lines, scrollbackLine{text: clean, visual: visual})
		}
		if fenceLine {
			inFence = !inFence
			continue
		}
	}
	if !hasVisual {
		summary := legacyReasoningSummary(text, false)
		if summary == "" {
			return nil
		}
		return []scrollbackLine{{text: summary}}
	}
	for len(lines) > 0 && strings.TrimSpace(stripAnsiEscapes(lines[0].text)) == "" && !lines[0].visual {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(stripAnsiEscapes(lines[len(lines)-1].text)) == "" && !lines[len(lines)-1].visual {
		lines = lines[:len(lines)-1]
	}
	return lines
}

func expandInlineReasoningVisualRows(text string) string {
	if !strings.ContainsAny(text, "┌├└│") {
		return text
	}
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	inFence := false
	for _, line := range lines {
		plain := strings.TrimSpace(stripAnsiEscapes(line))
		fenceLine := IsMarkdownFenceLine(plain)
		if inFence || fenceLine || shouldPreserveReasoningVisualRow(line) {
			out = append(out, line)
			if fenceLine {
				inFence = !inFence
			}
			continue
		}
		out = append(out, expandInlineReasoningVisualLine(line))
	}
	return strings.Join(out, "\n")
}

func shouldPreserveReasoningVisualRow(line string) bool {
	if !ShouldPreserveVisualLine(line) {
		return false
	}
	plain := strings.TrimLeftFunc(stripAnsiEscapes(line), isUnicodeSpace)
	if plain == "" {
		return false
	}
	r, _ := utf8.DecodeRuneInString(plain)
	switch {
	case r >= 0x2500 && r <= 0x257F:
		return true
	case r == '|' || r == '+':
		return true
	default:
		return false
	}
}

func expandInlineReasoningVisualLine(line string) string {
	var out []rune
	out = make([]rune, 0, len(line))
	var prevNonSpace rune
	lineHasNonSpace := false
	for _, r := range line {
		split := false
		switch r {
		case '┌', '├', '└':
			split = lineHasNonSpace
		case '│':
			split = lineHasNonSpace && isReasoningBoxRowBoundary(prevNonSpace)
		}
		if split {
			out = append(out, '\n')
			prevNonSpace = 0
			lineHasNonSpace = false
		}
		out = append(out, r)
		if !isUnicodeSpace(r) {
			prevNonSpace = r
			lineHasNonSpace = true
		}
	}
	return string(out)
}

func isReasoningBoxRowBoundary(r rune) bool {
	switch r {
	case '┐', '┤', '┘', '│':
		return true
	default:
		return false
	}
}

func isUnicodeSpace(r rune) bool {
	switch r {
	case ' ', '\t', '\r', '\v', '\f':
		return true
	default:
		return false
	}
}

func MarkdownTableLineMask(raw []string) []bool {
	mask := make([]bool, len(raw))
	for i := 0; i+1 < len(raw); i++ {
		if !looksLikeMarkdownTableDataLine(raw[i]) || !looksLikeMarkdownTableSeparatorLine(raw[i+1]) {
			continue
		}
		mask[i] = true
		mask[i+1] = true
		for j := i + 2; j < len(raw); j++ {
			if !looksLikeMarkdownTableDataLine(raw[j]) {
				break
			}
			mask[j] = true
		}
	}
	return mask
}

func looksLikeMarkdownTableDataLine(s string) bool {
	plain := strings.TrimSpace(stripAnsiEscapes(s))
	if plain == "" || strings.Count(plain, "|") < 1 {
		return false
	}
	return !looksLikeMarkdownTableSeparatorLine(plain)
}

func looksLikeMarkdownTableSeparatorLine(s string) bool {
	plain := strings.TrimSpace(stripAnsiEscapes(s))
	if plain == "" || !strings.Contains(plain, "|") {
		return false
	}
	plain = strings.Trim(plain, "| ")
	if plain == "" {
		return false
	}
	hasDash := false
	for _, r := range plain {
		switch r {
		case '-', ':', '|', ' ':
			if r == '-' {
				hasDash = true
			}
		default:
			return false
		}
	}
	return hasDash
}

func ShouldPreserveVisualLine(s string) bool {
	plain := strings.TrimSpace(stripAnsiEscapes(s))
	if plain == "" {
		return false
	}
	return looksLikePipeTableLine(plain) ||
		looksLikeBoxDrawingLine(plain) ||
		looksLikeASCIITableRule(plain) ||
		looksLikeDiagramSyntaxLine(plain)
}

func looksLikePipeTableLine(s string) bool {
	if strings.Count(s, "|") >= 2 && strings.HasPrefix(s, "|") && strings.HasSuffix(s, "|") {
		return true
	}
	return strings.Count(s, "│") >= 2 && strings.HasPrefix(s, "│") && strings.HasSuffix(s, "│")
}

func looksLikeBoxDrawingLine(s string) bool {
	box := 0
	for _, r := range s {
		if r >= 0x2500 && r <= 0x257F {
			box++
		}
	}
	if box == 0 {
		return false
	}
	first, last := firstLastVisualRune(s)
	if first >= 0x2500 && first <= 0x257F || last >= 0x2500 && last <= 0x257F {
		return box >= 2 || strings.ContainsAny(s, "─━═")
	}
	return box >= 4
}

func firstLastVisualRune(s string) (rune, rune) {
	var first rune
	var last rune
	for i, r := range s {
		if i == 0 {
			first = r
		}
		last = r
	}
	return first, last
}

func looksLikeASCIITableRule(s string) bool {
	if !(strings.HasPrefix(s, "+") && strings.HasSuffix(s, "+")) {
		return false
	}
	if strings.Count(s, "+") < 2 {
		return false
	}
	rule := 0
	for _, r := range s {
		switch r {
		case '+', '-', '=', ':', ' ':
			rule++
		default:
			return false
		}
	}
	return rule == len([]rune(s))
}

func looksLikeDiagramSyntaxLine(s string) bool {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return false
	}
	for _, prefix := range []string{
		"flowchart ", "graph ", "sequencediagram", "participant ", "actor ",
		"subgraph ", "classdiagram", "statediagram", "erdiagram",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	for _, token := range []string{
		"-->", "-.->", "==>", "->>", "-->>", "<--", "<->", "<|--", "--|>",
	} {
		if strings.Contains(s, token) {
			return true
		}
	}
	return false
}

func formatToolCallBatch(agent string, stage types.PipelineStage, iteration int, names []string, count int, firstName, firstDetail, lang string) string {
	return formatToolCallBatchWithParallel(agent, stage, iteration, names, count, firstName, firstDetail, lang, "")
}

func formatToolCallBatchWithParallel(agent string, stage types.PipelineStage, iteration int, names []string, count int, firstName, firstDetail, lang string, parallelUnitLabel string) string {
	return formatToolCallBatchWithAvailability(agent, stage, iteration, names, count, firstName, firstDetail, lang, parallelUnitLabel, nil)
}

func formatUnavailableToolCallBatchWithParallel(agent string, stage types.PipelineStage, iteration int, names []string, count int, firstName, firstDetail, lang string, parallelUnitLabel string, unavailable []string) string {
	return formatToolCallBatchWithAvailability(agent, stage, iteration, names, count, firstName, firstDetail, lang, parallelUnitLabel, unavailable)
}

func formatToolCallBatchWithAvailability(agent string, stage types.PipelineStage, iteration int, names []string, count int, firstName, firstDetail, lang string, parallelUnitLabel string, unavailable []string) string {
	body := toolCallBatchBody(names, count, firstName, firstDetail, lang)
	if len(unavailable) > 0 {
		body = unavailableToolCallBatchBody(unavailable, count, lang)
	}
	if body == "" {
		return ""
	}
	trace := activityTraceLabelWithParallel(agent, stage, iteration, lang, parallelUnitLabel)
	// Tool-call batches are model activity, but they are actions
	// rather than reasoning prose. Brighten only the dispatch marker
	// so scrollback distinguishes it from thinking without making the
	// tag or body compete with answer prose.
	return "  " + statusToolGlyph.Sprint(string(glyphToolCall)) + " " + statusMeta.Sprint(trace) + " " + statusReasoningBody.Sprint(body)
}

func activityTraceLabel(agent string, stage types.PipelineStage, iteration int, lang string) string {
	return activityTraceLabelWithParallel(agent, stage, iteration, lang, "")
}

func activityTraceLabelWithParallel(agent string, stage types.PipelineStage, iteration int, lang string, parallelUnitLabel string) string {
	label := activityDisplayLabel(agent, stage, lang)
	round := iteration + 1
	parallelUnitLabel = strings.TrimSpace(parallelUnitLabel)
	if isZh(lang) {
		if parallelUnitLabel != "" {
			return fmt.Sprintf("%s · %s · 第 %d 轮", label, parallelUnitLabel, round)
		}
		return fmt.Sprintf("%s · 第 %d 轮", label, round)
	}
	if parallelUnitLabel != "" {
		return fmt.Sprintf("%s · %s · Round %d", label, parallelUnitLabel, round)
	}
	return fmt.Sprintf("%s · Round %d", label, round)
}

func activityDisplayLabel(agent string, stage types.PipelineStage, lang string) string {
	zh := isZh(lang)
	if label := activityStageLabel(stage, zh); label != "" {
		return label
	}
	if label := activityAgentLabel(agent, zh); label != "" {
		return label
	}
	if zh {
		return "模型"
	}
	return "Model"
}

func activityStageLabel(stage types.PipelineStage, zh bool) string {
	switch stage {
	case types.StageLogTriage:
		if zh {
			return "日志诊断"
		}
		return "Log Triage"
	case types.StagePerfTriage:
		if zh {
			return "性能诊断"
		}
		return "Perf Triage"
	case types.StageAnalyze:
		if zh {
			return "分析"
		}
		return "Analyze"
	case types.StageExplore:
		if zh {
			return "探索"
		}
		return "Explore"
	case types.StageExtract:
		if zh {
			return "提炼"
		}
		return "Extract"
	case types.StageFinalize:
		if zh {
			return "成文"
		}
		return "Compose"
	case types.StageWriteAnalyze:
		if zh {
			return "写前分析"
		}
		return "Write Analysis"
	case types.StagePlan:
		if zh {
			return "规划"
		}
		return "Plan"
	case types.StageApply:
		if zh {
			return "修改"
		}
		return "Edit"
	case types.StageVerify:
		if zh {
			return "验证"
		}
		return "Verify"
	}
	return ""
}

func activityAgentLabel(agent string, zh bool) string {
	switch types.AgentName(strings.TrimSpace(agent)) {
	case types.AgentAnalyzer:
		if zh {
			return "分析"
		}
		return "Analyze"
	case types.AgentExplorer:
		if zh {
			return "探索"
		}
		return "Explore"
	case types.AgentExtractor:
		if zh {
			return "提炼"
		}
		return "Extract"
	case types.AgentFinalizer:
		if zh {
			return "成文"
		}
		return "Compose"
	case types.AgentLogTriager:
		if zh {
			return "日志诊断"
		}
		return "Log Triage"
	case types.AgentPerfTriager:
		if zh {
			return "性能诊断"
		}
		return "Perf Triage"
	case types.AgentWriteAnalyzer:
		if zh {
			return "写前分析"
		}
		return "Write Analysis"
	case types.AgentPlanner:
		if zh {
			return "规划"
		}
		return "Plan"
	case types.AgentCoder:
		if zh {
			return "修改"
		}
		return "Edit"
	case types.AgentVerifier:
		if zh {
			return "验证"
		}
		return "Verify"
	}
	return ""
}

func toolCallBatchBody(names []string, count int, firstName, firstDetail, lang string) string {
	if count <= 0 {
		count = len(names)
	}
	firstName = strings.TrimSpace(firstName)
	if firstName == "" && len(names) > 0 {
		firstName = strings.TrimSpace(names[0])
	}
	if count <= 0 && firstName != "" {
		count = 1
	}
	if count <= 0 {
		return ""
	}
	zh := isZh(lang)
	if count == 1 {
		if firstName == "" {
			if zh {
				return "调用工具"
			}
			return "calling tool"
		}
		if firstDetail = strings.TrimSpace(firstDetail); firstDetail != "" {
			if isMCPReadResourceToolName(firstName) {
				if zh {
					return fmt.Sprintf("读取 MCP 资源 %s", trimMCPResourceDetail(firstDetail))
				}
				return fmt.Sprintf("reading MCP resource %s", trimMCPResourceDetail(firstDetail))
			}
			if isNamespacedMCPToolName(firstName) {
				if zh {
					return fmt.Sprintf("调用 MCP %s %s", prettyMCPToolName(firstName), firstDetail)
				}
				return fmt.Sprintf("calling MCP %s %s", prettyMCPToolName(firstName), firstDetail)
			}
			if zh {
				return fmt.Sprintf("调用工具 %s %s", firstName, firstDetail)
			}
			return fmt.Sprintf("calling tool %s %s", firstName, firstDetail)
		}
		if isMCPReadResourceToolName(firstName) {
			if zh {
				return "读取 MCP 资源"
			}
			return "reading MCP resource"
		}
		if isNamespacedMCPToolName(firstName) {
			if zh {
				return "调用 MCP " + prettyMCPToolName(firstName)
			}
			return "calling MCP " + prettyMCPToolName(firstName)
		}
		if zh {
			return "调用工具 " + firstName
		}
		return "calling tool " + firstName
	}
	allMCP := allMCPToolNames(names, count, firstName)
	list := compactToolNameList(names, 4)
	if allMCP {
		list = compactToolNameList(prettyMCPToolNames(names), 4)
	}
	if list == "" && firstName != "" {
		list = firstName
		if allMCP {
			list = prettyMCPToolName(firstName)
		}
	}
	if zh {
		if list == "" {
			return fmt.Sprintf("调用 %d 个工具", count)
		}
		if allMCP {
			return fmt.Sprintf("调用 %d 个 MCP 工具 %s", count, list)
		}
		return fmt.Sprintf("调用 %d 个工具 %s", count, list)
	}
	if list == "" {
		return fmt.Sprintf("calling %d tools", count)
	}
	if allMCP {
		return fmt.Sprintf("calling %d MCP tools %s", count, list)
	}
	return fmt.Sprintf("calling %d tools %s", count, list)
}

func isMCPReadResourceToolName(name string) bool {
	return strings.TrimSpace(name) == "mcp_read_resource"
}

func isNamespacedMCPToolName(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" || isMCPReadResourceToolName(name) {
		return false
	}
	parts := strings.Split(name, "__")
	return len(parts) == 2 && strings.TrimSpace(parts[0]) != "" && strings.TrimSpace(parts[1]) != ""
}

func isMCPToolCallName(name string) bool {
	return isMCPReadResourceToolName(name) || isNamespacedMCPToolName(name)
}

func prettyMCPToolName(name string) string {
	name = strings.TrimSpace(name)
	if isMCPReadResourceToolName(name) {
		return "mcp_read_resource"
	}
	parts := strings.Split(name, "__")
	if len(parts) != 2 {
		return name
	}
	server := strings.TrimSpace(parts[0])
	tool := strings.TrimSpace(parts[1])
	if server == "" || tool == "" {
		return name
	}
	return server + "." + tool
}

func prettyMCPToolNames(names []string) []string {
	if len(names) == 0 {
		return nil
	}
	out := make([]string, 0, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			continue
		}
		out = append(out, prettyMCPToolName(name))
	}
	return out
}

func allMCPToolNames(names []string, count int, firstName string) bool {
	if count <= 0 {
		count = len(names)
	}
	if count <= 0 {
		return false
	}
	if len(names) == 0 {
		return isMCPToolCallName(firstName)
	}
	for _, name := range names {
		if !isMCPToolCallName(name) {
			return false
		}
	}
	return true
}

func trimMCPResourceDetail(detail string) string {
	detail = strings.TrimSpace(detail)
	detail = strings.TrimPrefix(detail, "uri=")
	return detail
}

func unavailableToolCallBatchBody(unavailable []string, totalCount int, lang string) string {
	list := compactToolNameList(unavailable, 4)
	if list == "" {
		return ""
	}
	zh := isZh(lang)
	if len(unavailable) == 1 {
		if totalCount > 1 {
			if zh {
				return fmt.Sprintf("尝试不可用工具 %s（本轮共 %d 个工具调用）", list, totalCount)
			}
			return fmt.Sprintf("attempted unavailable tool %s (%d tool calls this turn)", list, totalCount)
		}
		if zh {
			return "尝试不可用工具 " + list
		}
		return "attempted unavailable tool " + list
	}
	if zh {
		return fmt.Sprintf("尝试 %d 个不可用工具 %s", len(unavailable), list)
	}
	return fmt.Sprintf("attempted %d unavailable tools %s", len(unavailable), list)
}

func compactToolNameList(names []string, maxKinds int) string {
	if maxKinds <= 0 {
		maxKinds = 4
	}
	type entry struct {
		name  string
		count int
	}
	index := make(map[string]int)
	var ordered []entry
	for _, raw := range names {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if pos, ok := index[name]; ok {
			ordered[pos].count++
			continue
		}
		index[name] = len(ordered)
		ordered = append(ordered, entry{name: name, count: 1})
	}
	if len(ordered) == 0 {
		return ""
	}
	limit := len(ordered)
	if limit > maxKinds {
		limit = maxKinds
	}
	parts := make([]string, 0, limit+1)
	for _, ent := range ordered[:limit] {
		if ent.count > 1 {
			parts = append(parts, fmt.Sprintf("%s x%d", ent.name, ent.count))
		} else {
			parts = append(parts, ent.name)
		}
	}
	if omitted := len(ordered) - limit; omitted > 0 {
		parts = append(parts, fmt.Sprintf("+%d", omitted))
	}
	return strings.Join(parts, ", ")
}

// truncByDisplayWidth clamps a styled (ANSI-CSI-bearing) line to at
// most maxCols display columns. ANSI escapes are passed through
// without consuming columns, runes count via runewidth (so CJK takes
// 2 cols and ambient combining marks take 0). When truncation
// happens, the result ends with a "…" marker followed by an SGR
// reset so any active colour doesn't bleed into the next line.
func truncByDisplayWidth(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	var b strings.Builder
	w := 0
	i := 0
	truncated := false
	for i < len(s) {
		if s[i] == 0x1b {
			loc := reAnsi.FindStringIndex(s[i:])
			if loc != nil && loc[0] == 0 {
				b.WriteString(s[i : i+loc[1]])
				i += loc[1]
				continue
			}
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		// Reserve 1 col for the ellipsis if we're about to overflow
		// AND there's still more content after this rune.
		if w+rw > maxCols {
			truncated = true
			break
		}
		b.WriteRune(r)
		w += rw
		i += size
	}
	if truncated {
		// Walk back until we have room for "…" plus reset.
		out := b.String()
		// Strip trailing partial cells if we're now at the cap.
		for runewidth.StringWidth(stripAnsiEscapes(out)) > maxCols-1 && len(out) > 0 {
			// Drop one trailing rune (skip ANSI sequences while
			// walking back is overkill; truncByDisplayWidth never
			// places escapes after content without a rune in
			// between, so trimming runes only is safe in practice).
			_, size := utf8.DecodeLastRuneInString(out)
			out = out[:len(out)-size]
		}
		return out + "…\x1b[0m"
	}
	return b.String()
}

// wrapByDisplayWidth inserts '\n' into s so no visible line exceeds
// maxCols display columns. ANSI CSI escapes pass through with zero
// width; runes count via runewidth so CJK takes 2 cols and combining
// marks take 0. Existing '\n' in the input start a new line and
// reset the column counter — pre-wrapped paragraphs keep their
// shape.
//
// Why this exists: pterm.AreaPrinter.Update tracks how many rows it
// printed by counting '\n' in its argument. If the rendered content
// visually wraps in the terminal (long CJK sentence narrower than
// content), Area's cursor-up math goes up fewer lines than the
// terminal actually drew, leaving leftover rows that subsequent
// Updates print BELOW. The stacking-header bug the streaming
// preview hit pre-2026-04-29 is the canonical instance. Inserting
// our own wraps keeps Area's line count aligned with the terminal's
// drawn rows so Update / Stop erase exactly what was painted.
//
// Cross-platform: pure Go (utf8 + runewidth + regexp). No syscalls,
// no platform branches. CJK width is computed from the unconditional
// East Asian Width tables (Han / Hiragana / Katakana / Hangul are
// always wide on Windows/macOS/Linux). '\r' from CRLF line endings
// is a zero-width control rune (runewidth returns -1, clamped to 0
// here), so a "\r\n" sequence wraps as a single line break. Tabs
// pass through as zero-width — the terminal expands them on draw.
//
// Edge cases: maxCols < 1 → return s untouched (caller's problem).
// A single rune wider than maxCols (rare; combining sequences) is
// emitted on its own line — better than an infinite loop on the
// wrap boundary.
func wrapByDisplayWidth(s string, maxCols int) string {
	if maxCols < 1 || s == "" {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) + len(s)/16)
	col := 0
	i := 0
	for i < len(s) {
		// Pass through ANSI CSI escapes unchanged.
		if s[i] == 0x1b {
			loc := reAnsi.FindStringIndex(s[i:])
			if loc != nil && loc[0] == 0 {
				b.WriteString(s[i : i+loc[1]])
				i += loc[1]
				continue
			}
		}
		// Explicit newline — reset column.
		if s[i] == '\n' {
			b.WriteByte('\n')
			col = 0
			i++
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		rw := runewidth.RuneWidth(r)
		// Negative or zero-width runes don't trigger a wrap on their own.
		if rw < 0 {
			rw = 0
		}
		if rw > 0 && col+rw > maxCols {
			b.WriteByte('\n')
			col = 0
		}
		b.WriteRune(r)
		col += rw
		i += size
	}
	return b.String()
}

// stripAnsiEscapes removes CSI sequences from s. Used by
// truncByDisplayWidth's tail-shrink loop.
func stripAnsiEscapes(s string) string {
	return reAnsi.ReplaceAllString(s, "")
}

// RenderResult formats a completed BusContext into styled terminal output.
func (r *Renderer) RenderResult(busCtx *types.BusContext) string {
	if busCtx == nil {
		return "(no result)\n"
	}
	var b strings.Builder

	// Suppress the standalone "  error: …" line when the stage hook
	// has already authored a complete user-facing diagnostic into
	// Mutable.Result. ResultIsPlain is the structural signal that
	// the bare-dir authorization gate / forced-finalize / similar
	// hook has surfaced an error explanation that ALSO got mirrored
	// onto LastError for telemetry. Pre-2026-04-30 both surfaces
	// emitted, so the bordered output printed the same paragraph
	// twice (#1 reported by user). The plain result IS the answer
	// surface; LastError is internal state.
	suppressErrorLine := busCtx.Mutable != nil &&
		busCtx.Mutable.ResultIsPlain() &&
		strings.TrimSpace(busCtx.Mutable.Result()) != ""
	if busCtx.TaskState.LastError != "" && !suppressErrorLine {
		b.WriteString(fmt.Sprintf("\n  error: %s\n", busCtx.TaskState.LastError))
	}

	if busCtx.Mutable != nil {
		if result := busCtx.Mutable.Result(); result != "" {
			clean := stripAgentLabels(result)
			if clean != "" {
				// Plain-text fail-loud messages (set via
				// SetResultPlain by stage hooks) bypass glamour
				// markdown rendering. chroma's syntax highlighter
				// splits identifier-like tokens (e.g. emit_change_plan)
				// into ANSI-coloured fragments which look broken on
				// hook diagnostic prose. Real LLM answers stay on
				// the markdown path so styled headings / code
				// blocks / lists render as expected.
				if busCtx.Mutable.ResultIsPlain() {
					// Plain (fail-loud) path: NO success banner.
					// The result is an error / diagnostic message —
					// printing "✓ 已生成最终答案" above an error
					// would be self-contradicting. Skip glamour
					// because chroma's underscore-aware tokenizer
					// fragments identifier tokens (e.g.
					// `emit_change_plan`, `--auto-init-repo`)
					// inside the body, breaking the visual line.
					b.WriteString(clean)
				} else {
					// Real LLM answer path: print the persistent
					// "✓ 已生成最终答案" line DIRECTLY to stdout
					// (NOT into the response buffer) so the line
					// lands ABOVE the bordered answer the REPL
					// emits via renderBordered, aligned with the
					// upstream done rows ("  ✓ 已整理结论 …") in
					// both indent and colour. Pre-2026-04-30 it was
					// concatenated into the response string, which
					// renderBordered then wrapped with `│ ` — the
					// banner ended up INSIDE the bordered answer
					// area, mis-aligned, and rendered with a green
					// text body that didn't match the gray-family
					// done rows above it.
					r.printFinalizeBanner()
					// Convert ```mermaid``` fences to aligned ASCII
					// grids before glamour styles the markdown. The
					// mermaid library lives in this package and only
					// fires when mermaidRenderingEnabled is true (the
					// default for REPL — set false by single-shot CLI
					// callers that want raw source). Pre-2026-05-06
					// this call was missing: the doc comment on
					// repl.renderRichResponse claimed "the pipeline
					// path gets this via RenderResult", but RenderResult
					// only ran RenderMarkdown — so REPL pipeline
					// answers shipped raw ```mermaid``` fences that
					// glamour rendered as plain code blocks. CLI single-
					// shot bypasses RenderResult entirely (cmd/root.go
					// prints Mutable.Result() directly with its own
					// optional --mermaid-render hook), so this addition
					// affects ONLY the REPL path it was supposed to
					// already cover.
					clean = RenderMermaidBlocks(clean)
					b.WriteString(r.renderMarkdown(clean))
				}
				b.WriteString("\n")
			}
		}
	}

	if b.Len() == 0 {
		b.WriteString("(no result)\n")
	}
	return b.String()
}

// ---------- helpers ----------

// Patterns to strip agent-internal labels from user-facing output.
// Handles all bold/colon arrangements: "Answer:", "**Answer:**", "**Answer**:"
var reAgentLabels = regexp.MustCompile(`(?m)^(\*\*)?(Answer|Evidence|Caveat)(:\*\*|\*\*:|:)\s*$`)
var reAgentLabelInline = regexp.MustCompile(`(?m)^(\*\*)?(Answer|Evidence|Caveat)(:\*\*|\*\*:|:)\s*`)

// stripAgentLabels removes "Answer:", "Evidence:", "Caveat:" labels
// (plain or **bold** markdown) from agent output.
func stripAgentLabels(s string) string {
	// Remove standalone label lines first (e.g. "Evidence:\n").
	s = reAgentLabels.ReplaceAllString(s, "")
	// Remove inline label prefix (e.g. "Answer: some text").
	s = reAgentLabelInline.ReplaceAllString(s, "")
	s = strings.TrimSpace(s)
	// Clean up residual bold markers at start/end (e.g. "** text **").
	s = strings.TrimLeft(s, "* ")
	s = strings.TrimRight(s, "* ")
	s = strings.TrimSpace(s)
	return s
}

func (r *Renderer) renderMarkdown(text string) string {
	return r.RenderMarkdown(text)
}

// RenderMarkdown is the public entry point so callers outside this
// package (e.g. internal/repl's local + chitchat dispatch paths,
// which used to print raw text bypassing glamour) can reuse the
// glamour-backed markdown pipeline without re-wiring it. Nil
// receiver returns input unchanged so test fixtures that pass
// Renderer=nil keep working byte-identically. Glamour failure
// (rare) also falls back to the original text — no regression
// risk for any caller.
//
// Pre-2026-04-30 the lowercase renderMarkdown was the only entry,
// which forced internal/repl/repl.go's renderBordered consumers
// to choose between (a) calling RenderResult against a synthetic
// BusContext (heavy) or (b) printing raw markdown (the bug
// users reported as "图表/表格未渲染"). Exposing a thin public
// wrapper is the minimal generalisation: same input, same
// output, callable without a BusContext.
func (r *Renderer) RenderMarkdown(text string) string {
	if r == nil {
		return text
	}
	if r.glamour != nil {
		rendered, err := r.glamour.Render(text)
		if err == nil {
			return strings.TrimRight(rendered, "\n")
		}
	}
	return text
}

// formatAgent formats an agent name with call count, e.g. "AnalyzerAgent(2)".
// The count is the total number of agent calls (main + sub) so far.
func formatAgent(name types.AgentName, count int) string {
	s := string(name)
	if s == "" {
		s = "agent"
	}
	// Capitalize first letter + append "Agent" suffix.
	s = strings.ToUpper(s[:1]) + s[1:] + "Agent"
	return fmt.Sprintf("%s(%d)", s, count)
}

// previewIsTTY reports whether stdout is a terminal. The renderer
// short-circuits live preview events on non-TTY outputs (pipes / >
// file / CI) so the preview never leaks into the user's piped
// stream. isTTY is the same predicate the diff colorizer uses, so
// the preview / diff color paths agree on what counts as
// interactive.
func (r *Renderer) previewIsTTY() bool {
	return isTTY(r.outputWriter())
}

// previewLineMaxCols caps the ticker line width. Single-line ticker
// design — every Update fits in one terminal row, never wraps. The
// margin protects against off-by-one auto-wrap at the right edge.
// Capped above at 160 cols so the line stays readable on ultra-wide
// monitors; floored at 40 cols so degenerate split panes still get
// useful content.
func previewLineMaxCols() int {
	w := pterm.GetTerminalWidth() - 2
	if w > 160 {
		w = 160
	}
	if w < 40 {
		w = 40
	}
	return w
}

// previewLineLastSnippetCols is how many display columns of the
// most-recent stream tail we surface inside the ticker line. The
// rest of the line is the round + char-count prefix. Tuned so the
// snippet is informative without dominating: a 140-col terminal
// dedicates ~35 cols to prefix and ~105 cols to snippet.
func previewLineLastSnippetCols(total int) int {
	tail := total - 36
	if tail < 8 {
		tail = 8
	}
	return tail
}

// handlePreviewChunkLocked is called with r.mu held. With the
// post-2026-04-30 redesign, finalize streaming is just another
// state of the SAME liveBar — composeLiveBar's previewStreamMeta
// path picks up r.previewLastChunk and renders it as the meta
// segment with the same animated spinner glyph as every other
// running stage. No separate previewArea, no frozen-frame print,
// no animation-goroutine swap.

// renderPreviewLineLocked composes and pushes the streaming-finalize
// ticker line. Called both from handlePreviewChunkLocked (per-chunk
// arrival) and refreshPreviewLineLocked (per-animation-tick when no
// new chunk has fired). The leading glyph cycles through
// spinnerFrames so the ticker visibly animates between chunks
// instead of staring at a static `⟳`.
//
// Caller MUST hold r.mu.

// refreshPreviewLineLocked re-renders the ticker line with the
// CURRENT spinner frame using the cached previewLastChunk text. The
// shared animation goroutine calls this every 100 ms so the leading
// glyph animates between chunk arrivals.
//
// Caller MUST hold r.mu.

// stylePreviewLine applies the per-segment colour scheme to the
// already-truncated ticker line. The split logic recognises:
//   - leading "  " indent (uncoloured)
//   - the ⟳ glyph (statusRecoverable)
//   - the primary running phrase (statusPrimary)
//   - the meta segment between the two " · " separators (statusMeta)
//   - the trailing snippet (statusDetail)
//
// Splitting on visible substrings keeps the styling lossless when
// truncation clips the snippet — the un-clipped prefix segments
// always survive because truncByTerminalWidth fits the prefix
// width budget before snipping the tail.

// handlePreviewClearLocked is called with r.mu held. Clears the
// streaming-mode sentinel so subsequent redraw()s drop the
// streamMeta segment and the bar returns to "正在撰写最终答案"
// (without the "已收到 N 字 · 回复中:" suffix). On rejection
// (PreviewRejected=true), print a "草稿被规则丢弃" line to
// scrollback as a permanent record of the rewrite.

// printFinalizeBanner emits the persistent "✓ 已生成最终答案" /
// "✓ Final answer ready" line directly to stdout. The line MUST
// match the visual shape of the upstream done status rows
// rendered by renderStatusLine — same 2-space indent, same green
// glyph (statusSuccessMuted), same gray text (statusPrimaryDone) —
// so the user reads the timeline as one continuous family:
//
//	✓ 已校核分析结论 · 5s · 1 次工具调用
//	✓ 已整理结论 · 5s
//	✓ 已生成最终答案
//	┌─ Final answer ─┐
//
// Pre-2026-04-30 the line was sprinted with statusSuccessMuted
// across the WHOLE string (icon + text both green), and the user
// reported it as visually inconsistent with the gray-text done
// rows above. Splitting the styles per token mirrors the row
// layout exactly.
func (r *Renderer) printFinalizeBanner() {
	icon := statusSuccessMuted.Sprint(string(glyphSuccess))
	text := statusPrimaryDone.Sprint(finalizeDoneText(r.lang))
	fmt.Fprintf(r.outputWriter(), "  %s %s\n\n", icon, text)
}

func finalizeDoneText(lang string) string {
	if isZh(lang) {
		return "已撰写最终答案"
	}
	return "Final answer composed"
}

// tailByDisplayWidth returns the suffix of s whose display width
// fits within maxCols. Used by the streaming ticker to surface the
// most-recent N columns of the cumulative summary so the user
// always sees what is being written *right now* rather than the
// long-stale opening sentence.
func tailByDisplayWidth(s string, maxCols int) string {
	if maxCols <= 0 {
		return ""
	}
	if runewidth.StringWidth(s) <= maxCols {
		return s
	}
	runes := []rune(s)
	col := 0
	for i := len(runes) - 1; i >= 0; i-- {
		rw := runewidth.RuneWidth(runes[i])
		if rw < 0 {
			rw = 0
		}
		if col+rw > maxCols {
			return string(runes[i+1:])
		}
		col += rw
	}
	return s
}
