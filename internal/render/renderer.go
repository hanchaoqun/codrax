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
	isNodeRow bool
	nodeID    string
	nodeKind  string
	objective string
	pending   bool
	paused    bool

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

	// dispatchGen is the rolling counter for orchestrator dispatch
	// windows. EventTaskNodeStart bumps it whenever the timestamp
	// jumps past dispatchWindowGroupingMs ms from the previous start
	// (signalling a new window). Within-window sibling starts share
	// the current gen so multi-topic evidence dispatches stay all-
	// active together.
	dispatchGen     int
	lastNodeStartAt time.Time

	// lang is the user-facing locale code consumed by the status
	// localization layer (status_messages.go). Empty defaults to
	// zh-style output (mirrors the project-wide isZh fallback). Set
	// via SetLang from the orchestrator/REPL after Renderer
	// construction so existing callers stay source-compatible.
	lang string

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
}

// maxVisibleTasks caps how many rows the live area shows at once.
// When the list exceeds this, the oldest COMPLETED rows are hidden
// first — running rows are always retained so the user can see
// what's in flight. A collapsed-count indicator is rendered in
// their place.
const maxVisibleTasks = 12

// SetLang installs the user-facing locale code. Used by the status
// localization layer to pick zh / en text for stage labels, thinking
// summaries, tool-call detail, footer and cancel hint. Safe to call
// before or after Start; subsequent redraws pick it up immediately.
func (r *Renderer) SetLang(lang string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lang = lang
}

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
//   H1 — pure white / black: the "title" tier, neutral. Bold.
//   H2 — bright cyan: dominant section heading. Bold.
//   H3 — light cyan: subsection. Bold; matches the inline-code hue
//        so heading + code references read as the same family.
//   H4 — mid blue: minor heading. Bold.
//   H5 — light grey: italic, not bold; visibly retreated.
//   H6 — deep grey: italic, not bold; bottommost.
//
// All four active levels (H1-H4) now sit at clearly readable
// luminance on dark backgrounds; the gradient discriminator is
// hue + bold/italic, not "drop the brightness 30% per level".
func applyHeadingHierarchy(cfg *ansi.StyleConfig, dark bool) {
	cfg.H1.BackgroundColor = nil
	if dark {
		cfg.H1.Color = strPtr("15")  // pure white
		cfg.H2.Color = strPtr("51")  // bright cyan #00ffff
		cfg.H3.Color = strPtr("117") // light cyan #87d7ff (matches inline code)
		cfg.H4.Color = strPtr("75")  // mid blue #5fafff
		cfg.H5.Color = strPtr("250") // light grey #bcbcbc — readable, not loud
		cfg.H6.Color = strPtr("245") // mid grey #8a8a8a
		// Heading.Color cascades into any H without its own Color;
		// keep light cyan as the family default for unexpected H levels.
		cfg.Heading.Color = strPtr("117")
	} else {
		// Light backgrounds: invert the brightness ladder. Darker hues
		// at the top because dark text on light reads bolder; lift the
		// lower levels into mid-grey territory so they remain visible
		// without becoming heavier than the structural prose.
		cfg.H1.Color = strPtr("16")  // black
		cfg.H2.Color = strPtr("21")  // pure blue #0000ff
		cfg.H3.Color = strPtr("25")  // dark blue #005faf
		cfg.H4.Color = strPtr("31")  // mid blue #0087af
		cfg.H5.Color = strPtr("241") // dark grey
		cfg.H6.Color = strPtr("244") // mid grey
		cfg.Heading.Color = strPtr("25")
	}
	cfg.H1.Bold = boolPtr(true)
	cfg.H2.Bold = boolPtr(true)
	cfg.H3.Bold = boolPtr(true)
	cfg.H4.Bold = boolPtr(true)
	// H5 + H6 stay non-bold; the user's session-37+1 follow-up
	// asked to keep heading hierarchy expressed via colour alone
	// ("标题什么的,最好避免用斜体字，主要通过颜色来统一美感"),
	// so italics are explicitly disabled here. The brightness drop
	// from 75 (H4) → 250 (H5) → 245 (H6) carries the level signal.
	cfg.H5.Bold = boolPtr(false)
	cfg.H5.Italic = boolPtr(false)
	cfg.H6.Bold = boolPtr(false)
	cfg.H6.Italic = boolPtr(false)
}

// applyInlineAndStructure tunes non-heading prose elements: inline
// code, links, horizontal rule, block quote indent, table grid,
// bold/emphasis runs, and list-item bullets.
//
// Hue discipline ladder used here (dark-mode reference, light mode
// is the symmetric inverted-luminance counterpart):
//
//   115 (#87d7af) — soft mint, used for emphasis (italic prose)
//   117 (#87d7ff) — light cyan, anchor / inline-code / heading family
//   151 (#afd7af) — pale green, list bullet markers
//   244 (#808080) — neutral grey, structural dividers
//
// Pre-2026-04-30 Strong / Emphasis inherited bare bold/italic from
// glamour DarkStyleConfig — no explicit Color — which on dark
// terminals rendered as the terminal-default foreground (often a
// dim "off-white"). Inline-bold tokens like "**关键锚点**：" or
// "**Key anchors:**" therefore read as visually quieter than the
// surrounding prose, which is exactly the user's "字体颜色就很暗，
// 看着很不协调" complaint applied to the Strong run. Pinning
// Strong + Emphasis + Item to explicit colours keeps every prose
// surface in the heading-family hue space without re-introducing
// the rainbow per token.
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
		// Strong (`**bold**`) — used by all the answer-doc section
		// labels ("**Key anchors:**", "**关键锚点**：") and by the
		// snippet headers ("📄 **`file:line`**"). Pale cyan + bold
		// keeps Strong in the heading-cyan cohort (so the label
		// reads as "structural marker") while staying distinct from
		// H1 (pure white) AND from inline code (light cyan 117).
		// The brightness ladder is H1=15 > Strong=159 > Code=117 so
		// the eye reads Section ⊃ Label ⊃ identifier without any
		// hue switch.
		cfg.Strong.Color = strPtr("159") // #afffff very pale cyan
		cfg.Strong.Bold = boolPtr(true)
		// Emphasis (`*italic*`) — pale green-cyan so italics read
		// as a distinct accent without leaning into the heading-
		// cyan family or competing with inline code.
		cfg.Emph.Color = strPtr("115") // #87d7af pale green-cyan
		cfg.Emph.Italic = boolPtr(true)
		// List items — bullet markers in pale green so structured
		// lists (steps, key anchors, citations) read as clearly
		// itemised. Body text inherits the prose default.
		cfg.Item.Color = strPtr("151") // #afd7af
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
		// Light-mode Strong / Emphasis / Item — symmetric to the
		// dark palette but anchored at a darker luminance so prose
		// stays readable on white-ish terminal backgrounds.
		cfg.Strong.Color = strPtr("16") // black
		cfg.Strong.Bold = boolPtr(true)
		cfg.Emph.Color = strPtr("28") // dark green
		cfg.Emph.Italic = boolPtr(true)
		cfg.Item.Color = strPtr("28")
		cfg.Table.CenterSeparator = strPtr("┼")
		cfg.Table.ColumnSeparator = strPtr("│")
		cfg.Table.RowSeparator = strPtr("─")
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
//   blue   #5fafff — control-flow keywords (if / for / return ...)
//   purple #bd93f9 — declaration keywords (class / def / func) + decorators
//   cyan   #87d7ff — types + classes + namespace (the "shape" family,
//                    same hue as inline `code` in prose so types read
//                    consistently across prose and fenced blocks)
//   green  #87d7af — functions + attributes + diff-inserted (the
//                    "definition / new content" family)
//   yellow #d7d787 — literal strings + builtins (the "data + lib"
//                    family; warm but desaturated so it doesn't
//                    fight the cyan headings above)
//   orange #ffaf87 — literal numbers (sole hue, distinct from strings)
//   slate  #6272a4 — comments + preproc (low-contrast, calm)
//   grey   #909090 — operators + subheading (structural neutral)
//   red    #ff5555 — diff-deleted + exceptions (the ONE place red
//                    carries semantic load)
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
		chroma.Keyword.Color = strPtr("#5fafff")           // mid blue
		chroma.KeywordReserved.Color = strPtr("#bd93f9")   // soft purple
		chroma.KeywordNamespace.Color = strPtr("#87d7ff")  // light cyan (was its own pale blue)
		chroma.KeywordType.Color = strPtr("#87d7ff")       // light cyan (matches namespace + inline-code)
		// Operator + Punctuation: structural neutral. Operator stays
		// grey so high-frequency `=` / `+` / `<` don't blend with
		// diff markers. Punctuation inherits text colour.
		chroma.Operator.Color = strPtr("#909090")
		chroma.Punctuation.Color = nil
		// Names — the "definition" family in green, "type-shape"
		// in cyan. Tag (HTML/XML) folds into the cyan family because
		// it's a structural shape marker, not a keyword.
		chroma.Name.Color = strPtr("#dadada")              // near-white default
		chroma.NameBuiltin.Color = strPtr("#d7d787")       // soft yellow (matches strings)
		chroma.NameTag.Color = strPtr("#87d7ff")           // cyan (was purple — pulled into shape family)
		chroma.NameAttribute.Color = strPtr("#87d7af")     // soft green
		chroma.NameClass.Color = strPtr("#87d7ff")         // cyan + bold + underline (kept; was pure white)
		chroma.NameDecorator.Color = strPtr("#bd93f9")     // purple (matches reserved)
		chroma.NameFunction.Color = strPtr("#87d7af")      // soft green
		chroma.NameException.Color = strPtr("#ff5555")     // red — exception is the ONE alert hue
		// Literals — string vs number unambiguously distinct.
		chroma.LiteralString.Color = strPtr("#d7d787")          // soft yellow
		chroma.LiteralStringEscape.Color = strPtr("#ffaf87")    // orange (was pink) — escapes share orange with numbers
		chroma.LiteralNumber.Color = strPtr("#ffaf87")          // soft orange
		// Comments — slate is calm, low-contrast. Preproc folds in
		// because both are "side-channel" context the reader skims.
		chroma.Comment.Color = strPtr("#6272a4")           // dracula slate
		chroma.CommentPreproc.Color = strPtr("#6272a4")    // (was pink — folded into comment family)
		// Diff markers — the load-bearing red + green semantics.
		chroma.GenericDeleted.Color = strPtr("#ff5555")
		chroma.GenericInserted.Color = strPtr("#87d7af")   // matches function-green so "added" reads as "definition"
		chroma.GenericSubheading.Color = strPtr("#909090")
		// Default text inside fenced blocks
		chroma.Text.Color = strPtr("#dadada")
	} else {
		// Light theme mirror — same hue-discipline, inverted
		// luminance. Strings + builtins share dark-amber, classes +
		// types + namespace share dark-cyan, functions + attributes
		// share dark-green. Comments retreat to a calm slate-grey.
		chroma.Keyword.Color = strPtr("#005faf")           // dark blue
		chroma.KeywordReserved.Color = strPtr("#6f42c1")   // dark purple
		chroma.KeywordNamespace.Color = strPtr("#0087af")  // dark cyan (matches type)
		chroma.KeywordType.Color = strPtr("#0087af")
		chroma.Operator.Color = strPtr("#6e7781")
		chroma.Punctuation.Color = nil
		chroma.Name.Color = strPtr("#24292f")
		chroma.NameBuiltin.Color = strPtr("#9a6700")       // dark amber (matches strings; was warm red)
		chroma.NameTag.Color = strPtr("#0087af")           // dark cyan (was dark green)
		chroma.NameAttribute.Color = strPtr("#116329")     // dark green
		chroma.NameClass.Color = strPtr("#0087af")         // dark cyan
		chroma.NameDecorator.Color = strPtr("#6f42c1")     // dark purple (was orange)
		chroma.NameFunction.Color = strPtr("#116329")      // dark green (was purple)
		chroma.NameException.Color = strPtr("#cf222e")
		chroma.LiteralString.Color = strPtr("#9a6700")     // dark amber
		chroma.LiteralStringEscape.Color = strPtr("#953800") // dark orange (matches numbers)
		chroma.LiteralNumber.Color = strPtr("#953800")
		chroma.Comment.Color = strPtr("#6e7781")
		chroma.CommentPreproc.Color = strPtr("#6e7781")    // (was blue — folded into comment family)
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
	r.openSpinnerAreaLocked()
}

// openSpinnerAreaLocked opens a fresh pterm.AreaPrinter and starts
// the animation ticker WITHOUT touching task / objective / analysis
// state. Used by both startSpinnerWithHint (full spinner start) and
// reopenSpinnerAreaLocked (post-finalize-retry recovery), so the
// area + animation goroutine lifecycle has one canonical owner.
//
// Caller MUST hold r.mu.
func (r *Renderer) openSpinnerAreaLocked() {
	a, _ := pterm.DefaultArea.WithRemoveWhenDone(true).Start()
	r.area = a
	r.redraw()

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
				// The single animation goroutine drives BOTH the
				// spinner area (when active) AND the streaming
				// preview ticker (when active). Pre-2026-04-30 the
				// goroutine returned the moment r.area became nil
				// — but handlePreviewChunkLocked nulls r.area so
				// the preview ticker can own the screen, leaving
				// the previewArea's leading "⟳" glyph static
				// between chunks. Continuing to tick lets the
				// preview line refresh its glyph + frame counter
				// even when no new chunk arrives.
				if r.area == nil && r.previewArea == nil {
					r.mu.Unlock()
					return
				}
				r.animFrame++
				if r.area != nil {
					r.redraw()
				}
				if r.previewArea != nil {
					r.refreshPreviewLineLocked()
				}
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
func (r *Renderer) reopenSpinnerAreaLocked() {
	if r.area != nil {
		return
	}
	if r.previewArea != nil {
		// Streaming preview is currently rendering — leave the
		// ticker alone, it owns the screen until handlePreviewClearLocked.
		return
	}
	r.openSpinnerAreaLocked()
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
			// Spinner area was torn down (typically by
			// handlePreviewChunkLocked freezing the spinner before
			// the finalize streaming preview opened its ticker, or
			// after the ticker closed and the bordered answer
			// printed). When events that signal "the pipeline is
			// still active" arrive AFTER teardown — e.g. an
			// orchestrator retry round re-dispatching a stage —
			// reopen the spinner area so the user sees state
			// updates instead of a frozen terminal. Read-only
			// events (EventTaskNodeEnd / EventStageEnd /
			// EventToolCallEnd / etc.) trigger a reopen too because
			// after them the loop typically continues with a fresh
			// thinking/dispatch cycle.
			switch ev.Kind {
			case EventTaskNodeStart,
				EventTaskNodeEnd,
				EventStageStart,
				EventStageEnd,
				EventAgentThinking,
				EventAgentContent,
				EventToolCallStart,
				EventToolCallEnd,
				EventTransition,
				EventSubAgentStart,
				EventSubAgentEnd,
				EventObjectiveStarted,
				EventAnalysisReady:
				r.reopenSpinnerAreaLocked()
			}
			if r.area == nil {
				return
			}
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
				// Dispatch-window detection: orchestrator emits every
				// node in a window back-to-back inside one for-loop,
				// so consecutive EventTaskNodeStart timestamps are
				// microseconds apart for siblings of the SAME dispatch
				// (e.g. evidence_t0 + evidence_t1 + evidence_t2 all
				// being dispatched together). Cross-window starts are
				// separated by the prior window's actual LLM call —
				// at minimum hundreds of milliseconds, typically
				// seconds. dispatchWindowGroupingMs threshold groups
				// rows by dispatch identity so parking only fires on
				// genuine focus shifts (new window started a
				// different node), not on same-window siblings.
				const dispatchWindowGroupingMs = 750
				sameWindow := !r.lastNodeStartAt.IsZero() &&
					ev.Timestamp.Sub(r.lastNodeStartAt) <= dispatchWindowGroupingMs*time.Millisecond
				if !sameWindow {
					// New dispatch window: park every older-gen row
					// that's still in flight (started but not ended
					// by EventTaskNodeEnd — the orchestrator's "no
					// end on requeue" contract leaves these visible
					// as running). Pre-2026-04-30 traces showed
					// validate's window-2 start while window-1's
					// evidence_tN rows still had endTime=0; without
					// generation gating they all rendered as "正在
					// X" simultaneously.
					r.dispatchGen++
					for _, other := range r.tasks {
						if other == row {
							continue
						}
						if !other.isNodeRow {
							continue
						}
						if other.pending || !other.endTime.IsZero() {
							continue
						}
						if other.dispatchGen < r.dispatchGen {
							other.paused = true
						}
					}
				}
				row.pending = false
				row.paused = false
				row.startTime = ev.Timestamp
				row.detailStart = ev.Timestamp
				row.endTime = time.Time{}
				row.okFinished = false
				row.errorMsg = ""
				row.dispatchGen = r.dispatchGen
				r.lastNodeStartAt = ev.Timestamp
				r.current = row
			}

		case EventTaskNodeEnd:
			if row := r.findNodeRow(ev.NodeID); row != nil {
				row.pending = false
				row.paused = false
				row.endTime = ev.Timestamp
				row.errorMsg = ev.Error
				row.okFinished = ev.Error == ""
				if row == r.current {
					r.current = nil
				}
			}

		case EventAgentThinking:
			if r.current != nil {
				// Store iteration as a 1-based round number so meta's
				// metaRoundPhrase renders the same "第 N 轮" / "round N"
				// the user expects to read elsewhere. Pre-2026-04-30
				// iteration was 0-based AND detail also carried a
				// "(round N+1)" suffix — same logical round shown as
				// two different numbers ("第 2 轮" + "第 1 轮") on the
				// same row. Single 1-based source eliminates the
				// off-by-one AND the redundancy.
				r.current.iteration = ev.Iteration + 1
				// Detail no longer carries the round; meta is the
				// single surface for it.
				r.current.detail = "thinking"
				r.current.detailDone = false
				r.current.detailStart = ev.Timestamp
				// Defense-in-depth parking: EventAgentThinking ONLY
				// fires for the actively-dispatched agent (BaseAgent
				// emits it from inside one Chat call), so any OTHER
				// in-flight NodeRow is by definition not the current
				// dispatch's focus and must be parked. This catches
				// scheduler-level edge cases where the dispatch-window
				// timestamp grouping mis-classifies sibling vs cross-
				// window starts (e.g. two nodes started <750 ms apart
				// but only one is actually dispatched, the other was
				// in scheduler "running" state from a prior window).
				// Parking is independent of gen/timestamp because
				// EventAgentThinking is a strict signal: this row
				// owns the current LLM call.
				if r.current.isNodeRow {
					for _, other := range r.tasks {
						if other == r.current {
							continue
						}
						if !other.isNodeRow {
							continue
						}
						if other.pending || !other.endTime.IsZero() {
							continue
						}
						other.paused = true
					}
				}
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
	r.area.Update(r.composeStatusFrame())
}

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
func (r *Renderer) composeStatusFrame() string {
	return r.composeStatusFrameWithFilter(false)
}

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
func (r *Renderer) composeStatusFrameWithFilter(hideRunningFinalize bool) string {
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
		line := composeObjectiveLine(r.objective, r.objectiveDone)
		b.WriteString(truncByDisplayWidth(line, maxCols))
		b.WriteByte('\n')
		b.WriteString("\n")
	}

	rows := r.visibleRows()
	if hideRunningFinalize {
		filtered := make([]*taskRow, 0, len(rows))
		for _, row := range rows {
			if !row.endTime.IsZero() {
				filtered = append(filtered, row)
				continue
			}
			// Running row — drop iff it's finalize. Use the same
			// stage-key resolver renderStatusLine uses so we don't
			// drift on alias names ("finalizer", "finaliser").
			if stageKeyFor(row) == "finalize" {
				continue
			}
			filtered = append(filtered, row)
		}
		rows = filtered
	}
	now := time.Now()
	blocks := r.buildStatusBlocks(rows, frame, now)
	for bi, blk := range blocks {
		blockLines := renderStatusBlock(blk, r.lang)
		for li, line := range blockLines {
			b.WriteString(truncByDisplayWidth(line, maxCols))
			if li < len(blockLines)-1 || bi < len(blocks)-1 {
				b.WriteByte('\n')
			}
		}
	}

	// Footer line (frame + elapsed + cancel hint) is suppressed in
	// the freeze snapshot — the live ticker below carries that info,
	// duplicating it stains the static printout with a stale spinner
	// that never updates.
	if len(blocks) > 0 && !hideRunningFinalize {
		b.WriteByte('\n')
		footer := composeFooter(frame, elapsed.String(), r.cancelHint, r.lang)
		b.WriteString(truncByDisplayWidth(footer, maxCols))
	}

	return b.String()
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
					// Plain (fail-loud) path: NO success banner.
					// The result is an error / diagnostic message —
					// printing "✓ 已生成最终答案" above an error
					// would be self-contradicting.
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
		// Stop the spinner area BUT preserve its last-rendered
		// state as static text above the ticker. Without this
		// freeze, the validate / reconcile / extract status
		// history disappears the moment the first preview chunk
		// arrives — and if a retry fires mid-finalize, the user
		// sees only the ticker with no context.
		//
		// Sequence:
		//   1. Stop the animation goroutine (no more redraws)
		//   2. Capture the current frame as a string
		//   3. Stop pterm.Area (WithRemoveWhenDone(true) erases)
		//   4. Print the captured frame to stdout — terminal
		//      keeps it as scrollback above the cursor
		//   5. Ticker writes its single line below the frozen text
		if r.area != nil {
			// Note: we deliberately do NOT close r.animStop here.
			// The single animation goroutine is shared between the
			// spinner area and the streaming preview ticker; closing
			// it would freeze the previewArea's `⟳` glyph between
			// chunks. The goroutine itself notices r.area==nil
			// below and switches to driving previewArea via
			// refreshPreviewLineLocked.
			// Frozen-frame print suppression on retry rounds: only
			// the FIRST round's frame is printed to scrollback. On
			// rounds 2+ the prior round already left a frozen frame
			// + bordered answer (or fail-loud notice) above; printing
			// another frame would duplicate every "✓ 已 X" row in
			// the user's terminal, exactly the visual stutter the
			// user reported in the HiTraceAnalyzer trace where the
			// whole task list re-printed on every retry round.
			//
			// hideRunningFinalize=true: the live ticker below this
			// snapshot owns the finalize state, so don't print a
			// stale "⠙ 正在生成最终答案" line in scrollback that
			// would later contradict the persistent "✓ 已生成最终
			// 答案" line RenderResult prepends to the bordered
			// answer.
			r.area.Stop()
			r.area = nil
			if r.previewRound == 0 {
				frozen := r.composeStatusFrameWithFilter(true)
				if frozen != "" {
					// Trim trailing newlines so the spacer below
					// (added before the ticker line) is the canonical
					// gap; doubled newlines in pterm output would
					// stack visually.
					fmt.Fprintln(os.Stdout, strings.TrimRight(frozen, "\n"))
				}
			}
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
	// Build the ticker line piece-by-piece so each segment carries
	// a colour matching its semantic role — same brightness ladder
	// as renderStatusLine for a regular running row, so the user
	// reads the ticker as "another running stage" rather than a
	// dim/done line. Pre-2026-04-30 the whole line was sprinted in
	// pterm.FgDarkGray, which made "正在生成最终答案" (running text)
	// render in the done-row colour — directly contradicting the
	// state the text described.
	//
	// Layout (mirrors renderStatusLine: "  <icon> <primary> · <meta> · <detail>"):
	//   "  "                    indent (2 cols, aligns with status rows)
	//   ⟳                       glyph in statusRecoverable (animating cue)
	//   " 正在生成最终答案"     primary running text in statusPrimary
	//   " · #N · 已收到 N 字"   meta in statusMeta (round + char count)
	//   " · <tail snippet>"     detail in statusDetail (gray)
	r.previewLastChunk = ev.PreviewText
	r.renderPreviewLineLocked(ev.PreviewText, chars, roundLabel, maxCols)
}

// renderPreviewLineLocked composes and pushes the streaming-finalize
// ticker line. Called both from handlePreviewChunkLocked (per-chunk
// arrival) and refreshPreviewLineLocked (per-animation-tick when no
// new chunk has fired). The leading glyph cycles through
// spinnerFrames so the ticker visibly animates between chunks
// instead of staring at a static `⟳`.
//
// Caller MUST hold r.mu.
func (r *Renderer) renderPreviewLineLocked(previewText string, chars int, roundLabel string, maxCols int) {
	if r.previewArea == nil {
		return
	}
	frame := spinnerFrames[r.animFrame%len(spinnerFrames)]
	primaryText := "正在撰写最终答案"
	metaText := fmt.Sprintf("%s· 已收到 %d 字", roundLabel, chars)
	prefixPlain := fmt.Sprintf("  %s %s · %s · ", frame, primaryText, metaText)
	prefixCols := runewidth.StringWidth(prefixPlain)
	tailCols := maxCols - prefixCols
	if tailCols < 8 {
		tailCols = previewLineLastSnippetCols(maxCols)
	}
	flat := flattenToOneLine(previewText)
	snippetRaw := tailByDisplayWidth(flat, tailCols)
	plain := truncByTerminalWidth(prefixPlain+snippetRaw, maxCols)
	r.previewArea.Update(stylePreviewLine(plain, primaryText, metaText, frame))
}

// refreshPreviewLineLocked re-renders the ticker line with the
// CURRENT spinner frame using the cached previewLastChunk text. The
// shared animation goroutine calls this every 100 ms so the leading
// glyph animates between chunk arrivals.
//
// Caller MUST hold r.mu.
func (r *Renderer) refreshPreviewLineLocked() {
	if r.previewArea == nil {
		return
	}
	chars := utf8.RuneCountInString(r.previewLastChunk)
	maxCols := previewLineMaxCols()
	roundLabel := ""
	if r.previewRound > 1 {
		roundLabel = fmt.Sprintf("#%d ", r.previewRound)
	}
	r.renderPreviewLineLocked(r.previewLastChunk, chars, roundLabel, maxCols)
}

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
func stylePreviewLine(plain, primaryText, metaText, glyph string) string {
	const indent = "  "
	const dotSep = " · "
	rest := strings.TrimPrefix(plain, indent)
	// Glyph segment: caller-supplied (animated spinner frame in
	// production; whatever was put in the plain assembly). Falls
	// through to the glyphRecoverable static character when the
	// caller passes empty for backward compatibility.
	if glyph == "" {
		glyph = string(glyphRecoverable)
	}
	if !strings.HasPrefix(rest, glyph) {
		// Truncation collapsed the whole line; render plain as a
		// single statusDetail-coloured run so the row at least
		// stays legible.
		return statusDetail.Sprint(plain)
	}
	rest = strings.TrimPrefix(rest, glyph)
	rest = strings.TrimPrefix(rest, " ")
	// Primary segment.
	if !strings.HasPrefix(rest, primaryText) {
		return statusDetail.Sprint(plain)
	}
	rest = strings.TrimPrefix(rest, primaryText)
	// Meta segment between two `dotSep` separators.
	var metaSegment, tail string
	if strings.HasPrefix(rest, dotSep) && strings.HasPrefix(rest[len(dotSep):], metaText) {
		afterMeta := rest[len(dotSep)+len(metaText):]
		metaSegment = metaText
		// Tail (snippet) follows the next dotSep, when present.
		switch {
		case strings.HasPrefix(afterMeta, dotSep):
			tail = afterMeta[len(dotSep):]
		default:
			tail = strings.TrimPrefix(afterMeta, " ")
		}
	}
	var b strings.Builder
	b.WriteString(indent)
	// Spinner frame uses statusSpinner (the same dim-grey style the
	// regular spinner-area row uses) so the animated glyph reads
	// as a "live ticker" cue without competing with the
	// statusPrimary primary text. statusRecoverable was used pre-
	// 2026-04-30 when the glyph was a static `⟳`; the animated
	// braille frames need a calmer hue so the eye locks on the
	// text body, not the cycling glyph.
	b.WriteString(statusSpinner.Sprint(glyph))
	b.WriteString(" ")
	b.WriteString(statusPrimary.Sprint(primaryText))
	if metaSegment != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint(metaSegment))
	}
	if tail != "" {
		b.WriteString(" ")
		b.WriteString(statusMeta.Sprint("·"))
		b.WriteString(" ")
		b.WriteString(statusDetail.Sprint(tail))
	}
	return b.String()
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
		r.previewArea.Stop()
		r.previewArea = nil
		r.previewLastChunk = ""
		return
	}
	// Success path: just wipe the ticker row. The persistent
	// "✓ 已生成最终答案" line is printed by RenderResult so both
	// streaming-finalize (ticker was open) and non-streaming
	// (ticker never opened) paths surface the same confirmation
	// — single source of truth, no duplicate printing.
	r.previewArea.Stop()
	r.previewArea = nil
	r.previewLastChunk = ""
}

// printFinalizeBanner emits the persistent "✓ 已生成最终答案" /
// "✓ Final answer ready" line directly to stdout. The line MUST
// match the visual shape of the upstream done status rows
// rendered by renderStatusLine — same 2-space indent, same green
// glyph (statusSuccessMuted), same gray text (statusPrimaryDone) —
// so the user reads the timeline as one continuous family:
//
//   ✓ 已校核分析结论 · 5s · 1 次工具调用
//   ✓ 已整理结论 · 5s
//   ✓ 已生成最终答案
//   ┌─ Final answer ─┐
//
// Pre-2026-04-30 the line was sprinted with statusSuccessMuted
// across the WHOLE string (icon + text both green), and the user
// reported it as visually inconsistent with the gray-text done
// rows above. Splitting the styles per token mirrors the row
// layout exactly.
func (r *Renderer) printFinalizeBanner() {
	icon := statusSuccessMuted.Sprint(string(glyphSuccess))
	text := statusPrimaryDone.Sprint(finalizeDoneText(r.lang))
	fmt.Fprintf(os.Stdout, "  %s %s\n\n", icon, text)
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
