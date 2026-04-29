package render

import (
	"fmt"
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
var reAnsi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

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
	endTime     time.Time // zero while running
	okFinished  bool      // true when endTime > 0 and error == ""
	errorMsg    string    // populated by EventStageEnd with non-empty Error
	detail      string    // latest tool/iteration/sub-task description
	detailDone  bool      // true when detail refers to a completed tool call
	detailStart time.Time // for the "current detail elapsed" display
	toolCount   int       // cumulative EventToolCallEnd successes for this row
	iteration   int       // latest ReAct iteration seen on this row
	isSubAgent  bool      // true for rows created by EventSubAgentStart
	subAgentID  string    // identifier for matching EventSubAgentEnd
	subTitle    string    // SubTaskTitle from the event
	subCount    int       // SubTaskCount from the event (batch size)

	// Task-graph node rows (post-EventAnalysisReady). nodeID is the
	// TaskGraph node identifier; nodeKind is the TaskNode.Type string
	// ("evidence" / "validate" / "reconcile" / "finalize"); objective
	// is the node's short objective text rendered as the row label.
	// pending is true when the row has been created but the node has
	// not yet run once — distinguishes "planned" rows from rows that
	// are actively in flight.
	isNodeRow bool
	nodeID    string
	nodeKind  string
	objective string
	pending   bool
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

	mu        sync.Mutex
	area      *pterm.AreaPrinter
	startTime time.Time

	// Live task list. Append-only within a Start/Stop cycle; rows
	// are not removed — completed rows stay visible with a
	// done-indicator so the user can read the history of what ran.
	tasks   []*taskRow
	current *taskRow // most recent non-sub-agent row with endTime zero; receives tool / thinking events

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
}

// maxVisibleTasks caps how many rows the live area shows at once.
// When the list exceeds this, the oldest COMPLETED rows are hidden
// first — running rows are always retained so the user can see
// what's in flight. A collapsed-count indicator is rendered in
// their place.
const maxVisibleTasks = 12

// New creates a Renderer.
func New(_ /* out */ interface{}, forceColor bool) *Renderer {
	color := forceColor
	if !color {
		info, err := os.Stdout.Stat()
		if err == nil {
			color = (info.Mode() & os.ModeCharDevice) != 0
		}
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

	return &Renderer{glamour: gr}
}

// codraxStyleConfig returns the glamour style used for rendering the
// final answer markdown.
//
// Design intent (post-2026-04-29 audit):
//   1. Hue discipline. Each colour family carries ONE semantic so
//      the eye isn't asked to disambiguate four reds:
//        red    → diagnostics + diff-deleted (only)
//        green  → success + diff-inserted + function name
//        cyan   → inline code + heading family
//        grey   → structure (HR, table grid, comments, operators,
//                 dimmed heading levels)
//        yellow → emphasis / built-ins (NOT errors)
//        purple → reserved keywords (class / def / func)
//   2. Heading hierarchy via brightness gradient. H1 = brightest
//      (white), gradient down through cyan→blue→grey→deep grey at
//      H6. Pure prefix-count cues ('## ', '### ') are kept by
//      glamour but no longer the sole signal of level.
//   3. No background colours. Terminal theme owns the background;
//      reintroducing pills/bars has been rejected repeatedly as
//      visually loud across the popular terminal themes.
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
// 39 in dark, blue 27 in light) so H2-H6 were visually identical.
func applyHeadingHierarchy(cfg *ansi.StyleConfig, dark bool) {
	cfg.H1.BackgroundColor = nil
	if dark {
		// Dark backgrounds: brightest at top, descending to deep grey.
		cfg.H1.Color = strPtr("15")  // white
		cfg.H2.Color = strPtr("117") // light cyan #87d7ff (matches inline code family)
		cfg.H3.Color = strPtr("75")  // mid blue #5fafff
		cfg.H4.Color = strPtr("67")  // dark blue #5f87af
		cfg.H5.Color = strPtr("246") // mid grey #949494
		cfg.H6.Color = strPtr("240") // deep grey #585858
		// Heading.Color cascades into any H without its own Color;
		// keep cyan as the family default for unexpected H levels.
		cfg.Heading.Color = strPtr("117")
	} else {
		// Light backgrounds: darkest at top, ascending to medium grey.
		cfg.H1.Color = strPtr("16")  // black
		cfg.H2.Color = strPtr("25")  // dark blue #005faf
		cfg.H3.Color = strPtr("31")  // mid blue #0087af
		cfg.H4.Color = strPtr("67")  // mid blue-grey
		cfg.H5.Color = strPtr("242") // dark grey
		cfg.H6.Color = strPtr("245") // mid grey
		cfg.Heading.Color = strPtr("25")
	}
	cfg.H1.Bold = boolPtr(true)
	cfg.H2.Bold = boolPtr(true)
	cfg.H3.Bold = boolPtr(true)
	cfg.H4.Bold = boolPtr(true)
	cfg.H5.Bold = boolPtr(true)
	cfg.H6.Bold = boolPtr(false) // visually retreats — bottom of hierarchy
}

// applyInlineAndStructure tunes non-heading prose elements: inline
// code, links, horizontal rule, block quote indent, table grid.
// Inline code's red→cyan move is the original codrax customisation
// (red triggered alert fatigue on identifier-heavy answers); other
// surfaces follow the same hue-discipline philosophy.
func applyInlineAndStructure(cfg *ansi.StyleConfig, dark bool) {
	cfg.Code.BackgroundColor = nil
	if dark {
		cfg.Code.Color = strPtr("117") // light cyan #87d7ff
		// HR: 240 was almost invisible on dim themes; 244 reads as a
		// subtle but clearly-present divider.
		cfg.HorizontalRule.Color = strPtr("244") // #808080
		// Link URL was 30 (#008787) — too dim. 67 reads as a calm,
		// readable blue against dark backgrounds.
		cfg.Link.Color = strPtr("67") // #5f87af
		// Link text shares inline-code's cyan so links read as
		// "interactive code reference". Underline retained as the
		// universal affordance signal.
		cfg.LinkText.Color = strPtr("117")
		cfg.LinkText.Bold = boolPtr(true)
		// Block quote: colour the `│ ` indent token so quotes are
		// visually delimited. Use cyan to match the heading family.
		cfg.BlockQuote.Color = strPtr("117")
		// Tables: fill in grid characters at glamour's medium-grey
		// neutral so the structure reads without being noisy. The
		// runes themselves are the standard ASCII set glamour ships.
		cfg.Table.CenterSeparator = strPtr("┼")
		cfg.Table.ColumnSeparator = strPtr("│")
		cfg.Table.RowSeparator = strPtr("─")
	} else {
		cfg.Code.Color = strPtr("33") // xterm blue #0087ff
		cfg.HorizontalRule.Color = strPtr("245")
		cfg.Link.Color = strPtr("25")
		cfg.LinkText.Color = strPtr("25")
		cfg.LinkText.Bold = boolPtr(true)
		cfg.BlockQuote.Color = strPtr("25")
		cfg.Table.CenterSeparator = strPtr("┼")
		cfg.Table.ColumnSeparator = strPtr("│")
		cfg.Table.RowSeparator = strPtr("─")
	}
}

// applyChromaPalette repaints the syntax-highlighting palette inside
// fenced code blocks. The default (glamour DarkStyleConfig) packs
// five different reds/pinks (#FF5FD2 / #FF5F87 / #EF8080 / #FF8EC7 /
// #FD5B5B) into keywords / namespace / operator / built-in / diff-
// deleted, all of which read as the same "warm-red alarm" hue.
// We reserve red for the ONE place it carries semantic load (diff-
// deleted + error) and pull every other token into a distinct family.
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
		// keyword / keyword.reserved / keyword.namespace / keyword.type:
		// ONE blue family + a distinct purple for "reserved" so the
		// reader can tell `if` apart from `class`.
		chroma.Keyword.Color = strPtr("#5fafff")           // mid blue
		chroma.KeywordReserved.Color = strPtr("#bd93f9")   // dracula purple
		chroma.KeywordNamespace.Color = strPtr("#87afff")  // pale blue
		chroma.KeywordType.Color = strPtr("#8be9fd")       // dracula cyan
		// Operator → neutral grey so high-frequency `=` / `+` / `<`
		// stop screaming. Pre-fix used #EF8080 light red which
		// visually merged with diff-deleted lines.
		chroma.Operator.Color = strPtr("#909090")
		// Punctuation → no colour (inherits text). Pale yellow
		// (#E8E8A8) was distracting.
		chroma.Punctuation.Color = nil
		// Identifiers
		chroma.Name.Color = strPtr("#dadada")              // near-white default
		chroma.NameBuiltin.Color = strPtr("#f1fa8c")       // dracula yellow (was hot pink)
		chroma.NameTag.Color = strPtr("#bd93f9")           // purple (HTML/XML tags)
		chroma.NameAttribute.Color = strPtr("#50fa7b")     // green attribute name
		chroma.NameClass.Color = strPtr("#ffffff")         // white + bold + underline (kept)
		chroma.NameDecorator.Color = strPtr("#f1fa8c")     // yellow (decorators ~ marker)
		chroma.NameFunction.Color = strPtr("#50fa7b")      // dracula green
		chroma.NameException.Color = strPtr("#ff5555")     // red (semantic: errors)
		// Literals
		chroma.LiteralString.Color = strPtr("#f1fa8c")     // yellow string (was warm orange — kept warm hue)
		chroma.LiteralStringEscape.Color = strPtr("#ff79c6") // dracula pink for \n / \t markers
		chroma.LiteralNumber.Color = strPtr("#ffb86c")     // dracula orange (was green — collided with function)
		// Comments: keep dim grey but slightly lift for readability.
		chroma.Comment.Color = strPtr("#6272a4")           // dracula slate
		chroma.CommentPreproc.Color = strPtr("#ff79c6")    // pink
		// Diff markers — the ONE place red and green carry meaning.
		chroma.GenericDeleted.Color = strPtr("#ff5555")
		chroma.GenericInserted.Color = strPtr("#50fa7b")
		chroma.GenericSubheading.Color = strPtr("#909090")
		// Default text inside fenced blocks
		chroma.Text.Color = strPtr("#dadada")
	} else {
		// Light theme mirror — same hue-discipline, inverted brightness.
		chroma.Keyword.Color = strPtr("#005faf")           // dark blue
		chroma.KeywordReserved.Color = strPtr("#6f42c1")   // GitHub-light purple
		chroma.KeywordNamespace.Color = strPtr("#0087af")
		chroma.KeywordType.Color = strPtr("#0087af")
		chroma.Operator.Color = strPtr("#6e7781")          // GitHub-light grey
		chroma.Punctuation.Color = nil
		chroma.Name.Color = strPtr("#24292f")
		chroma.NameBuiltin.Color = strPtr("#cf222e")       // dark red but reserved for builtins; light-theme builtins are conventionally red (e.g. GitHub)
		chroma.NameTag.Color = strPtr("#116329")           // dark green (HTML/XML)
		chroma.NameAttribute.Color = strPtr("#116329")
		chroma.NameClass.Color = strPtr("#24292f")
		chroma.NameDecorator.Color = strPtr("#953800")
		chroma.NameFunction.Color = strPtr("#8250df")      // GitHub-light purple
		chroma.NameException.Color = strPtr("#cf222e")
		chroma.LiteralString.Color = strPtr("#0a3069")     // dark navy string
		chroma.LiteralStringEscape.Color = strPtr("#cf222e")
		chroma.LiteralNumber.Color = strPtr("#0550ae")
		chroma.Comment.Color = strPtr("#6e7781")
		chroma.CommentPreproc.Color = strPtr("#0550ae")
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
	if r.area != nil {
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

	a, _ := pterm.DefaultArea.WithRemoveWhenDone(true).Start()
	r.area = a
	r.redraw()

	// Start animation ticker.
	r.animStop = make(chan struct{})
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-r.animStop:
				return
			case <-ticker.C:
				r.mu.Lock()
				if r.area == nil {
					r.mu.Unlock()
					return
				}
				r.animFrame++
				r.redraw()
				r.mu.Unlock()
			}
		}
	}()
}

// StopSpinner stops the live area and prints the final task list.
func (r *Renderer) StopSpinner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.area == nil {
		return
	}

	// Stop animation.
	close(r.animStop)

	// Stop area (removes its content due to RemoveWhenDone).
	r.area.Stop()
	r.area = nil

	// Print final objective — green checkmark, dimmed title.
	if r.objective != "" {
		fmt.Println()
		fmt.Printf("  %s %s\n",
			pterm.FgGreen.Sprint("✓"),
			pterm.FgDarkGray.Sprint(r.objective))
		fmt.Println()
	}

	r.objective = ""
	r.objectiveDone = false
	r.tasks = nil
	r.current = nil
	r.analysisReady = false
}

// Emitter returns an EventEmitter callback bound to this renderer.
func (r *Renderer) Emitter() EventEmitter {
	return func(ev Event) {
		r.mu.Lock()
		defer r.mu.Unlock()

		// Reasoning events are printed regardless of whether the
		// spinner area is active (REPL) or nil (single-shot). In
		// single-shot mode this is the user's only window into what
		// the pipeline is doing during the 30+ second wait.
		if ev.Kind == EventAgentReasoning && ev.Reasoning != "" {
			line := formatReasoning(string(ev.Agent), ev.Iteration, ev.Reasoning)
			if r.area != nil {
				r.printAboveArea(line)
			} else {
				fmt.Fprintln(os.Stderr, line)
			}
			return
		}

		// Live preview events are independent of the spinner Area
		// state — they manage their OWN previewArea so the chunk
		// arrival can stop the spinner and open the preview without
		// requiring an active spinner.
		switch ev.Kind {
		case EventLivePreviewChunk:
			r.handlePreviewChunkLocked(ev)
			return
		case EventLivePreviewClear:
			r.handlePreviewClearLocked(ev)
			return
		}

		if r.area == nil {
			return
		}

		switch ev.Kind {
		case EventStageStart:
			// Post-analysisReady the task graph's node rows own the
			// display; suppress stage-dispatch row creation for the
			// downstream stages (explore / extract / finalize) so the
			// user does not see an extra "ExplorerAgent(explore)" line
			// below the actual task list. The analyze stage starts
			// BEFORE the task graph exists, so its stage row is
			// always accepted.
			if r.analysisReady && ev.Stage != "" && string(ev.Stage) != "analyze" {
				break
			}
			row := &taskRow{
				stage:       string(ev.Stage),
				agent:       string(ev.Agent),
				startTime:   ev.Timestamp,
				detailStart: ev.Timestamp,
			}
			r.tasks = append(r.tasks, row)
			r.current = row

		case EventStageEnd:
			// Same gating as EventStageStart: ignore downstream stage
			// ends once the task graph is driving the display.
			if r.analysisReady && ev.Stage != "" && string(ev.Stage) != "analyze" {
				break
			}
			// Terminate the most recent matching (stage, agent) still-
			// running row. Matching by (stage, agent) rather than just
			// stage supports DAG windows dispatching the same stage
			// multiple times with different agents, and sub-agent
			// rows are exempt (isSubAgent flag).
			if row := r.findRunningStageRow(string(ev.Stage), string(ev.Agent)); row != nil {
				row.endTime = ev.Timestamp
				row.errorMsg = ev.Error
				row.okFinished = ev.Error == ""
				if row == r.current {
					r.current = nil
				}
			}

		case EventAnalysisReady:
			// Analyze phase complete: mark any still-running analyze
			// row done and append one "pending" row per TaskNodeInfo.
			// Rows are keyed by nodeID so subsequent EventTaskNodeStart
			// / EventTaskNodeEnd can find them without rebuilding the
			// list. Ordering is preserved from the orchestrator (which
			// itself preserved TaskGraph.Nodes declaration order).
			for _, row := range r.tasks {
				if row.isSubAgent || row.isNodeRow {
					continue
				}
				if row.stage == "analyze" && row.endTime.IsZero() {
					row.endTime = ev.Timestamp
					row.okFinished = true
					if row == r.current {
						r.current = nil
					}
				}
			}
			for _, n := range ev.TaskNodes {
				r.tasks = append(r.tasks, &taskRow{
					isNodeRow: true,
					nodeID:    n.ID,
					nodeKind:  n.Type,
					objective: n.Objective,
					pending:   true,
				})
			}
			r.analysisReady = true

		case EventTaskNodeStart:
			if row := r.findNodeRow(ev.NodeID); row != nil {
				row.pending = false
				row.startTime = ev.Timestamp
				row.detailStart = ev.Timestamp
				row.endTime = time.Time{}
				row.okFinished = false
				row.errorMsg = ""
				r.current = row
			}

		case EventTaskNodeEnd:
			if row := r.findNodeRow(ev.NodeID); row != nil {
				row.pending = false
				row.endTime = ev.Timestamp
				row.errorMsg = ev.Error
				row.okFinished = ev.Error == ""
				if row == r.current {
					r.current = nil
				}
			}

		case EventAgentThinking:
			if r.current != nil {
				r.current.iteration = ev.Iteration
				if ev.Iteration == 0 {
					r.current.detail = "thinking"
				} else {
					r.current.detail = fmt.Sprintf("thinking (round %d)", ev.Iteration+1)
				}
				r.current.detailDone = false
				r.current.detailStart = ev.Timestamp
			}

		case EventAgentContent:
			// Streaming content preview: update the row's detail line
			// in place with the latest sliver of assistant text so the
			// user sees the reply being typed while the LLM is still
			// responding. Never prints a new line above the area —
			// that would stack dozens of deltas per second.
			if r.current != nil && ev.Reasoning != "" {
				preview := stripMarkdown(ev.Reasoning)
				// Take the tail: users want the most recent thought,
				// not the whole message.
				const previewMax = 80
				if len(preview) > previewMax {
					preview = "…" + preview[len(preview)-previewMax:]
				}
				// Collapse newlines / repeated spaces so the row stays
				// on one visual line even if the delta contains `\n`.
				preview = strings.Join(strings.Fields(preview), " ")
				r.current.detail = "thinking: " + preview
				r.current.detailDone = false
				r.current.detailStart = ev.Timestamp
			}

		case EventToolCallStart:
			if r.current != nil {
				r.current.detail = ev.ToolName
				if ev.ToolDetail != "" {
					r.current.detail += " " + ev.ToolDetail
				}
				r.current.detailDone = false
				r.current.detailStart = ev.Timestamp
			}

		case EventToolCallEnd:
			if r.current != nil {
				r.current.detail = ev.ToolName
				if ev.ToolDetail != "" {
					r.current.detail += " " + ev.ToolDetail
				}
				r.current.detailDone = true
				if ev.ToolOK {
					r.current.toolCount++
				}
			}

		case EventTransition:
			if r.current != nil {
				r.current.detail = ""
				r.current.detailDone = false
				r.current.detailStart = ev.Timestamp
			}

		case EventSubAgentStart:
			row := &taskRow{
				stage:       string(ev.Stage),
				agent:       ev.SubAgentName,
				startTime:   ev.Timestamp,
				detailStart: ev.Timestamp,
				isSubAgent:  true,
				subAgentID:  ev.SubAgentID,
				subTitle:    ev.SubTaskTitle,
				subCount:    ev.SubTaskCount,
				detail:      ev.SubTaskTitle,
			}
			r.tasks = append(r.tasks, row)

		case EventSubAgentEnd:
			if row := r.findSubAgentRow(ev.SubAgentID); row != nil {
				row.endTime = ev.Timestamp
				row.errorMsg = ev.Error
				row.okFinished = ev.Error == ""
			}

		case EventObjectiveStarted:
			if ev.Objective != "" {
				r.objective = ev.Objective
				r.objectiveDone = false
			}

		case EventObjectiveDone:
			r.objectiveDone = true
		}

		r.redraw()
	}
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
func (r *Renderer) redraw() {
	var b strings.Builder
	elapsed := time.Since(r.startTime).Truncate(time.Second)
	frame := spinnerFrames[r.animFrame%len(spinnerFrames)]

	// 4 = 2-col left margin + 2-col safety pad. Below 20 we just give
	// up on truncation; the user has bigger problems.
	maxCols := pterm.GetTerminalWidth() - 4
	if maxCols < 20 {
		maxCols = 20
	}

	// Objective line (single-task wrapper collapsed into one row).
	if r.objective != "" {
		var icon string
		var color pterm.Style
		if r.objectiveDone {
			icon = "✓"
			color = *pterm.NewStyle(pterm.FgGreen)
		} else {
			icon = "►"
			color = *pterm.NewStyle(pterm.FgCyan)
		}
		line := fmt.Sprintf("  %s %s", color.Sprint(icon), r.objective)
		b.WriteString(truncByDisplayWidth(line, maxCols))
		b.WriteByte('\n')
		b.WriteString("\n")
	}

	// Task list: one line per stage dispatch + sub-agent run, in
	// event order. Completed rows stay visible with a done-indicator.
	// When the list grows past maxVisibleTasks we collapse the oldest
	// completed entries into a summary row — running rows are never
	// hidden.
	rows := r.visibleRows()
	for i, row := range rows {
		line := r.formatTaskLine(row, frame)
		b.WriteString(truncByDisplayWidth(line, maxCols))
		if i < len(rows)-1 {
			b.WriteByte('\n')
		}
	}

	// Footer: total elapsed for the whole pipeline. Rendered only
	// when at least one row is present so a pre-event "just started"
	// state shows a bare objective. The leading cyan spinner mirrors
	// the running-row glyph so the footer visibly animates — makes
	// it obvious the pipeline is still live even when every task row
	// has already entered a stable state between dispatches.
	if len(rows) > 0 {
		b.WriteByte('\n')
		footer := fmt.Sprintf("  %s %s %s",
			pterm.FgCyan.Sprint(frame),
			pterm.FgDarkGray.Sprint("Total"),
			pterm.FgWhite.Sprint(elapsed))
		b.WriteString(truncByDisplayWidth(footer, maxCols))
		// Cancel hint — surfaced ONLY while the spinner is live and
		// only when the REPL set a hint via StartSpinnerWithCancelHint.
		// Empty cancelHint preserves the historical single-shot CLI
		// rendering byte-identically.
		if r.cancelHint != "" {
			b.WriteString("  ")
			b.WriteString(pterm.FgDarkGray.Sprint(r.cancelHint))
		}
	}

	r.area.Update(b.String())
}

// visibleRows returns the task rows to draw this frame. Running rows
// are always retained; when the running + recent-done count exceeds
// maxVisibleTasks the oldest DONE rows are collapsed into a single
// "(+N earlier done)" marker row inserted at the top.
func (r *Renderer) visibleRows() []*taskRow {
	if len(r.tasks) <= maxVisibleTasks {
		return r.tasks
	}
	// Split into done vs running. Running is rare (usually 1-3 rows);
	// done is everything else.
	var done, running []*taskRow
	for _, row := range r.tasks {
		if row.endTime.IsZero() {
			running = append(running, row)
		} else {
			done = append(done, row)
		}
	}
	// Keep at most (maxVisibleTasks - 1 - len(running)) recent done
	// rows so we have room for the collapsed marker and every running
	// row. If running alone exceeds the cap, drop the collapsed
	// marker and show all running rows.
	budget := maxVisibleTasks - len(running)
	if budget < 1 {
		return running
	}
	budget-- // reserve one slot for the marker
	keepDone := done
	hidden := 0
	if len(done) > budget {
		hidden = len(done) - budget
		keepDone = done[len(done)-budget:]
	}
	out := make([]*taskRow, 0, len(keepDone)+len(running)+1)
	if hidden > 0 {
		marker := &taskRow{
			stage:      fmt.Sprintf("+%d earlier done", hidden),
			okFinished: true,
			endTime:    time.Now(), // treat marker as completed for rendering
		}
		out = append(out, marker)
	}
	out = append(out, keepDone...)
	out = append(out, running...)
	return out
}

// formatTaskLine renders one task row as a single terminal line with
// colour codes and the current animation frame. Running rows get the
// spinner frame; completed rows get ✓ (green) or ✗ (red). Layout:
//
//	  ⠋ ExplorerAgent(explore) · ► grep agent.go 2s · 3s
//	  ✓ AnalyzerAgent(analyze) · finished · 1s
//	  ⠋ subexplorer [sub-topic A] · ► read_file 1s
func (r *Renderer) formatTaskLine(row *taskRow, frame string) string {
	// Leading status icon. Node rows in the "pending" state (plan
	// published but this node has not run yet) get a dim bullet so
	// they read as "queued" rather than "in flight".
	var icon string
	switch {
	case row.isNodeRow && row.pending:
		icon = pterm.FgDarkGray.Sprint("·")
	case !row.endTime.IsZero() && row.okFinished:
		icon = pterm.FgGreen.Sprint("✓")
	case !row.endTime.IsZero() && !row.okFinished:
		icon = pterm.FgRed.Sprint("✗")
	default:
		icon = pterm.FgCyan.Sprint(frame)
	}

	// Label: distinguishes main-stage rows from sub-agent rows.
	var label string
	switch {
	case row.isNodeRow:
		// Task-graph node row: show "[kind] objective". The kind tag
		// is dimmed so the objective — the part the user actually
		// cares about — dominates. finalize is special-cased to a
		// friendlier label since the canonical id ("finalize") is
		// not informative as a summary. Multi-subtopic evidence rows
		// get "[topic N]" instead of "[evidence]" so a user seeing
		// three "[evidence]" lines in a row can tell at a glance
		// which is which — the tag comes from the node ID suffix
		// (`_tN`) that compiler.expandEvidenceNodes writes when
		// len(rm.SubTopics) > 1.
		kindTag := row.nodeKind
		if kindTag == "" {
			kindTag = "task"
		}
		if row.nodeKind == "evidence" {
			if idx, ok := topicIndexFromNodeID(row.nodeID); ok {
				kindTag = fmt.Sprintf("topic %d", idx+1)
			}
		}
		text := row.objective
		if text == "" {
			if row.nodeKind == "finalize" {
				text = "Synthesise final answer"
			} else {
				text = row.nodeID
			}
		}
		label = pterm.FgDarkGray.Sprintf("[%s] ", kindTag) + pterm.FgWhite.Sprint(text)
	case row.agent == "" && row.stage != "":
		// Collapsed marker row — render just the stage field as the label.
		label = pterm.FgDarkGray.Sprint(row.stage)
	case row.isSubAgent:
		name := row.agent
		if name == "" {
			name = "subagent"
		}
		// Include the SubTaskCount when the batch has >1 sub-tasks,
		// so a multi-sub-topic dispatch visibly shows "2 sub-tasks".
		if row.subCount > 1 {
			label = pterm.FgMagenta.Sprintf("%s[×%d]", name, row.subCount)
		} else {
			label = pterm.FgMagenta.Sprint(name)
		}
	default:
		// Main stage row: "AnalyzerAgent(analyze)" style. Stage name
		// in parentheses makes it obvious even when the same agent
		// is dispatched in different stages (rare but possible).
		agentLabel := formatAgent(types.AgentName(row.agent), 1)
		// Strip the "(1)" count formatAgent appends — the per-row
		// view doesn't need a count; the batch size is shown on
		// sub-agent rows via [×N].
		if idx := strings.LastIndex(agentLabel, "("); idx > 0 {
			agentLabel = agentLabel[:idx]
		}
		label = pterm.FgWhite.Sprintf("%s(%s)", agentLabel, row.stage)
	}

	// Detail segment. For running rows with a live detail, show the
	// per-phase elapsed; for completed rows, show total row elapsed.
	var detailSeg string
	switch {
	case row.endTime.IsZero() && row.detail != "":
		if row.detailDone {
			detailSeg = pterm.FgGreen.Sprint("✓") + " " + pterm.FgDarkGray.Sprint(row.detail)
		} else {
			phaseElapsed := time.Since(row.detailStart).Truncate(time.Second)
			detailSeg = pterm.FgCyan.Sprint("►") + " " + pterm.FgGray.Sprint(row.detail) +
				" " + pterm.FgDarkGray.Sprint(phaseElapsed.String())
		}
	case !row.endTime.IsZero():
		// Completed row: show total elapsed + tool count when > 0.
		elapsed := row.endTime.Sub(row.startTime).Truncate(time.Second)
		parts := []string{pterm.FgDarkGray.Sprint(elapsed.String())}
		if row.toolCount > 0 {
			parts = append(parts, pterm.FgDarkGray.Sprintf("%d tool%s", row.toolCount, pluralS(row.toolCount)))
		}
		if row.errorMsg != "" {
			parts = append(parts, pterm.FgRed.Sprintf("error: %s", row.errorMsg))
		}
		detailSeg = strings.Join(parts, pterm.FgDarkGray.Sprint(" · "))
	}

	// Skip the trailing separator when the row has no agent (collapsed
	// marker) or no detail segment.
	if detailSeg == "" {
		return fmt.Sprintf("  %s %s", icon, label)
	}
	return fmt.Sprintf("  %s %s %s %s",
		icon, label, pterm.FgDarkGray.Sprint("·"), detailSeg)
}

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
func (r *Renderer) printAboveArea(text string) {
	if r.area == nil || text == "" {
		return
	}
	// Clear the area's current content so the new text doesn't
	// interleave with the spinner line.
	r.area.Update("")
	fmt.Println(text)
	r.redraw()
}

// reasoningMaxChars caps the reasoning summary shown to the user.
const reasoningMaxChars = 200

// stripMarkdown removes markdown syntax that clutters a single-line
// thinking trace: headers (###), bold (**), bullets (- / *), code
// fences (```), and leading/trailing whitespace around them.
func stripMarkdown(s string) string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		// Strip header prefixes: "### Title" → "Title"
		for strings.HasPrefix(line, "#") {
			line = strings.TrimLeft(line, "#")
		}
		line = strings.TrimSpace(line)
		// Strip bold markers
		line = strings.ReplaceAll(line, "**", "")
		// Strip leading bullet
		if strings.HasPrefix(line, "- ") {
			line = line[2:]
		} else if strings.HasPrefix(line, "* ") {
			line = line[2:]
		}
		// Strip code fences
		if strings.HasPrefix(line, "```") {
			continue
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, " ")
}

// formatReasoning extracts the first 1-2 sentences from the LLM's
// reasoning text and formats them as a dimmed line with an
// [agent-iteration] tag. Markdown is stripped and leading blank
// lines are skipped so the display is clean plain text.
func formatReasoning(agent string, iteration int, text string) string {
	text = stripMarkdown(text)
	if text == "" {
		return ""
	}
	summary := text
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
	tag := fmt.Sprintf("[%s-%d]", agent, iteration+1)
	return "  " + pterm.FgDarkGray.Sprint("💭 "+tag+" "+summary)
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

	if busCtx.TaskState.LastError != "" {
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
					b.WriteString(clean)
				} else {
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
func previewIsTTY() bool {
	return isTTY(os.Stdout)
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

// handlePreviewChunkLocked is called with r.mu held. Renders the
// streaming-summary progress as a single-line ticker via
// ttyPreviewArea (`\r` + `\x1b[2K` + content). Single-line is the
// load-bearing simplification: no multi-line cursor positioning,
// no row-count tracking, no possibility of leftover wrap rows
// staining the screen when the final answer prints.
func (r *Renderer) handlePreviewChunkLocked(ev Event) {
	if !previewIsTTY() {
		return
	}
	if ev.PreviewText == "" {
		return
	}
	if r.previewArea == nil {
		// Stop the spinner area if it is still active so the ticker
		// owns the bottom row of the terminal cleanly.
		if r.area != nil {
			if r.animStop != nil {
				select {
				case <-r.animStop:
				default:
					close(r.animStop)
				}
				r.animStop = nil
			}
			r.area.Stop()
			r.area = nil
		}
		r.previewArea = newTTYPreviewArea(os.Stdout)
		r.previewRound++
	}
	chars := utf8.RuneCountInString(ev.PreviewText)
	maxCols := previewLineMaxCols()
	roundLabel := ""
	if r.previewRound > 1 {
		roundLabel = fmt.Sprintf("#%d ", r.previewRound)
	}
	prefix := fmt.Sprintf("⟳ 正在生成最终答案 %s· 已收到 %d 字 · ", roundLabel, chars)
	prefixCols := runewidth.StringWidth(prefix)
	tailCols := maxCols - prefixCols
	if tailCols < 8 {
		tailCols = previewLineLastSnippetCols(maxCols)
	}
	// Show the most-recent slice of streamed content, not the head:
	// the user's interest is "what is the model writing now". A
	// rolling tail-window also makes the line visibly change every
	// chunk so the eye perceives progress.
	flat := flattenToOneLine(ev.PreviewText)
	snippetRaw := tailByDisplayWidth(flat, tailCols)
	line := truncByTerminalWidth(prefix+snippetRaw, maxCols)
	// pterm.FgDarkGray dims the ticker line so it visually
	// recedes — the bordered final answer is what the user reads,
	// the ticker is just a "still working" signal.
	r.previewArea.Update(pterm.FgDarkGray.Sprint(line))
}

// handlePreviewClearLocked is called with r.mu held. Tears down
// r.previewArea. PreviewRejected=true briefly flashes a "[已重写]"
// notice on the same single line so the user knows the just-shown
// draft was rejected by the AnswerContract / Tier1 floor gates;
// PreviewRejected=false (success path) just wipes the line so the
// styled final answer prints on a clean canvas.
func (r *Renderer) handlePreviewClearLocked(ev Event) {
	if r.previewArea == nil {
		return
	}
	if ev.PreviewRejected {
		notice := truncByTerminalWidth("✗ 上一份草稿已被规则丢弃,正在重写", previewLineMaxCols())
		r.previewArea.Update(pterm.FgYellow.Sprint(notice))
		// Release the lock during the brief flash so other events
		// (reasoning, next chunk) can not deadlock waiting on us.
		r.mu.Unlock()
		time.Sleep(300 * time.Millisecond)
		r.mu.Lock()
	}
	if r.previewArea != nil {
		r.previewArea.Stop()
		r.previewArea = nil
	}
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
