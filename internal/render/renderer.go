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
	"github.com/mattn/go-runewidth"
	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/types"
)

// reAnsi matches ANSI CSI sequences (`ESC [ … final-byte`). Used by
// truncByDisplayWidth to step over style escapes without counting
// them as visible columns.
var reAnsi = regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Renderer consumes pipeline events and produces styled terminal output.
// Uses pterm.Area for in-place updating of task list and status during
// pipeline execution, and Glamour for final markdown rendering.
type Renderer struct {
	glamour *glamour.TermRenderer

	mu            sync.Mutex
	area          *pterm.AreaPrinter
	startTime     time.Time
	agentName  string // current agent name, e.g. "analyzer"
	subRunning int    // number of currently running sub-agents
	detail      string    // tool/sub-agent name for status line
	detailDone  bool      // true if the detail refers to a completed call
	detailStart time.Time // when current detail phase began
	// objective is the single user-question line displayed above the
	// status line. Populated on EventObjectiveStarted and flipped to
	// done=true on EventObjectiveDone.
	objective     string
	objectiveDone bool
	animFrame     int
	animStop      chan struct{}
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
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)

	return &Renderer{glamour: gr}
}

// StartSpinner begins the live status area.
func (r *Renderer) StartSpinner() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.area != nil {
		return
	}
	r.objective = ""
	r.objectiveDone = false
	r.agentName = ""
	r.subRunning = 0
	r.detail = ""
	r.detailDone = false
	r.detailStart = time.Time{}
	r.startTime = time.Now()
	r.animFrame = 0

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
	r.agentName = ""
	r.subRunning = 0
	r.detail = ""
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

		if r.area == nil {
			return
		}

		switch ev.Kind {
		case EventAgentThinking:
			if ev.Iteration == 0 {
				r.detail = "thinking"
			} else {
				r.detail = fmt.Sprintf("thinking (round %d)", ev.Iteration+1)
			}
			r.detailDone = false
			r.detailStart = ev.Timestamp

		case EventStageStart:
			r.agentName = string(ev.Agent)
			r.subRunning = 0
			r.detail = ""
			r.detailDone = false
			r.detailStart = ev.Timestamp

		case EventToolCallStart:
			r.detail = ev.ToolName
			if ev.ToolDetail != "" {
				r.detail += " " + ev.ToolDetail
			}
			r.detailDone = false
			r.detailStart = ev.Timestamp

		case EventToolCallEnd:
			r.detail = ev.ToolName
			if ev.ToolDetail != "" {
				r.detail += " " + ev.ToolDetail
			}
			r.detailDone = true

		case EventTransition:
			r.detail = ""
			r.detailDone = false
			r.detailStart = ev.Timestamp

		case EventSubAgentStart:
			r.subRunning++
			r.detail = ev.SubTaskTitle
			r.detailDone = false
			r.detailStart = ev.Timestamp

		case EventSubAgentEnd:
			if r.subRunning > 0 {
				r.subRunning--
			}
			r.detail = ev.SubTaskTitle
			r.detailDone = true

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

	// Status line: ⠋ stage · detail · 12s
	// Each element has a distinct color for easy scanning. The
	// previously-shown "N calls, last: TOOL" tail was removed because
	// `detail` already names the most recent tool and the count made
	// the line ~30 cols longer for almost no information value — and
	// in 80-col terminals it was the difference between fitting on one
	// row and wrapping, which triggered the刷屏 bug above.
	var statusParts []string
	running := 1 + r.subRunning // 1 main agent + sub-agents
	statusParts = append(statusParts, pterm.FgWhite.Sprint(formatAgent(types.AgentName(r.agentName), running)))
	if r.detail != "" {
		if r.detailDone {
			statusParts = append(statusParts,
				pterm.FgGreen.Sprint("✓")+" "+pterm.FgDarkGray.Sprint(r.detail))
		} else {
			phaseElapsed := time.Since(r.detailStart).Truncate(time.Second)
			detailStr := pterm.FgCyan.Sprint("►") + " " + pterm.FgGray.Sprint(r.detail) +
				" " + pterm.FgDarkGray.Sprint(phaseElapsed)
			statusParts = append(statusParts, detailStr)
		}
	}
	status := strings.Join(statusParts, pterm.FgDarkGray.Sprint(" · "))
	statusLine := fmt.Sprintf("  %s %s %s %s",
		pterm.FgCyan.Sprint(frame),
		status,
		pterm.FgDarkGray.Sprint("·"),
		pterm.FgWhite.Sprint(elapsed))
	b.WriteString(truncByDisplayWidth(statusLine, maxCols))

	r.area.Update(b.String())
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
				rendered := r.renderMarkdown(clean)
				b.WriteString(rendered)
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

