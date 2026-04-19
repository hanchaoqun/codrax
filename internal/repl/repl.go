// Package repl implements the interactive multi-turn loop for codrax.
//
// When the binary is launched with no --request, main.go hands control
// to this package. Each user line is dispatched as a fresh
// orchestrator.Run, with prior conversation injected into the request
// string via memory.Store.BuildContext. Slash commands manipulate the
// store directly without going through the orchestrator.
//
// Multi-line input is supported: end a line with \ to continue on the
// next line. The continuation lines are joined with newlines.
package repl

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
	"golang.org/x/term"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/pterm/pterm"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/memory"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// Runner is the orchestrator-shaped surface the REPL needs. Defined
// here as an interface so tests can stub it without pulling in the
// full pipeline.
type Runner interface {
	Run(request, repoRoot, branch string) (*types.BusContext, error)
}

// ResultRenderer turns a finished BusContext into the user-facing
// response text. main.go owns the canonical implementation.
type ResultRenderer func(*types.BusContext) string

// Config holds all dependencies for constructing a REPL. Using a
// struct keeps the constructor readable as the field count grows and
// lets tests inject an io.Reader for scripted input.
type Config struct {
	Runner   Runner
	Store    *memory.Store
	Render   ResultRenderer
	Renderer *render.Renderer
	RepoRoot string
	Branch   string
	In       io.Reader // nil → interactive (huh); non-nil → line-oriented
	Out      io.Writer

	// UI customization (used by line-oriented mode; ignored for huh).
	Prompt     string // primary prompt, e.g. ">"
	PromptCont string // continuation prompt, e.g. "."
	Banner     string // printed once at start; empty → default badge
}

// REPL drives the interactive prompt.
type REPL struct {
	runner     Runner
	store      *memory.Store
	render     ResultRenderer
	renderer   *render.Renderer
	repoRoot   string
	branch     string
	in         io.Reader
	out        io.Writer
	prompt     string
	promptCont string
	bannerText string
	scanner    *bufio.Scanner // lazy-init for line-oriented mode
}

// New constructs a REPL from a Config.
func New(cfg Config) *REPL {
	return &REPL{
		runner:     cfg.Runner,
		store:      cfg.Store,
		render:     cfg.Render,
		renderer:   cfg.Renderer,
		repoRoot:   cfg.RepoRoot,
		branch:     cfg.Branch,
		in:         cfg.In,
		out:        cfg.Out,
		prompt:     cfg.Prompt,
		promptCont: cfg.PromptCont,
		bannerText: cfg.Banner,
	}
}

// interactive returns true when the REPL is attached to an
// interactive terminal (huh-based input), false when driven by a
// scripted io.Reader.
func (r *REPL) interactive() bool { return r.in == nil }

// info prints an informational message. In interactive mode it uses
// pterm styling; in line-oriented mode it writes plain text to r.out
// so tests can capture it.
func (r *REPL) info(msg string) {
	if r.interactive() {
		pterm.Info.Println(msg)
	} else {
		fmt.Fprintln(r.out, msg)
	}
}

// success prints a success message.
func (r *REPL) success(msg string) {
	if r.interactive() {
		pterm.Success.Println(msg)
	} else {
		fmt.Fprintln(r.out, msg)
	}
}

// errorf prints an error message.
func (r *REPL) errorf(format string, args ...interface{}) {
	if r.interactive() {
		pterm.Error.Printf(format, args...)
	} else {
		fmt.Fprintf(r.out, format, args...)
	}
}

// warn prints a warning message.
func (r *REPL) warn(format string, args ...interface{}) {
	if r.interactive() {
		pterm.Warning.Printf(format, args...)
	} else {
		fmt.Fprintf(r.out, format, args...)
	}
}

// Loop runs the prompt until /exit, /quit, or EOF.
func (r *REPL) Loop() error {
	r.banner()
	for {
		line, err := r.readInput("❯❯")
		if err != nil {
			fmt.Fprintln(r.out)
			fmt.Fprintln(r.out, "  Goodbye!")
			return nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if r.interactive() {
			// Echo user input so it's visible after huh clears.
			pterm.FgLightCyan.Printf("  > %s\n", line)
			pterm.FgDarkGray.Println("  ─────────────────────────────────────")
		}
		if strings.HasPrefix(line, "/") {
			if quit := r.handleSlash(line); quit {
				return nil
			}
			continue
		}
		r.dispatch(line)
	}
}

func (r *REPL) banner() {
	if r.bannerText != "" {
		fmt.Fprintln(r.out, r.bannerText)
		return
	}
	badge := pterm.NewStyle(pterm.BgBlue, pterm.FgWhite, pterm.Bold).Sprint(" CODRAX ")
	hint := pterm.FgDarkGray.Sprint("/help · /exit")
	fmt.Fprintf(r.out, "\n  %s  %s\n", badge, hint)
	if summary := r.memorySummaryLine(); summary != "" {
		fmt.Fprintf(r.out, "  %s\n", summary)
	}
	fmt.Fprintln(r.out)
}

// memorySummaryLine returns a one-line dim-gray digest of the memory
// store's current state, or "" when the store is empty or absent.
// Format: "memory: <recent> recent turn(s) + <idx> compacted, <N> B total".
//
// Rendered in the banner instead of dumping the full history: the
// turns are already replayed into every dispatch via
// Store.BuildContext, so showing them verbatim in the banner would
// be duplicate noise. A short stat line lets the user see "yes, my
// previous N turns are still in play" without scrolling.
func (r *REPL) memorySummaryLine() string {
	if r.store == nil {
		return ""
	}
	recent := r.store.Recent()
	idx := r.store.Index()
	if len(recent) == 0 && len(idx) == 0 {
		return ""
	}
	bytes := 0
	for _, t := range recent {
		bytes += len(t.Request) + len(t.Response)
	}
	for _, e := range idx {
		bytes += len(e.Topic) + len(e.Summary)
	}
	return pterm.FgDarkGray.Sprintf("Memory: %d recent + %d compacted, %s",
		len(recent), len(idx), humanByteSize(bytes))
}

// humanByteSize renders a byte count in the smallest unit that keeps
// the integer part ≤ 3 digits. "1023 B", "9.4 KB", "2.1 MB".
func humanByteSize(n int) string {
	const (
		kb = 1024
		mb = 1024 * 1024
	)
	switch {
	case n < kb:
		return fmt.Sprintf("%d B", n)
	case n < mb:
		return fmt.Sprintf("%.1f KB", float64(n)/float64(kb))
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/float64(mb))
	}
}

// inputTheme returns a minimal huh theme — no borders, no decorations,
// just a clean prompt and cursor.
func inputTheme() *huh.Theme {
	t := huh.ThemeBase()

	// Clean cursor and prompt styling.
	t.Focused.TextInput.Cursor = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")) // bright cyan
	t.Focused.TextInput.Prompt = lipgloss.NewStyle().
		Foreground(lipgloss.Color("51")).Bold(true) // bright cyan bold
	t.Focused.TextInput.Text = lipgloss.NewStyle()
	t.Focused.TextInput.Placeholder = lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")) // dim gray

	// Remove all borders and decorations.
	t.Focused.Base = lipgloss.NewStyle().PaddingLeft(1)
	t.Focused.Title = lipgloss.NewStyle().Width(0).MaxWidth(0)
	t.Focused.Description = lipgloss.NewStyle().Width(0).MaxWidth(0)

	t.Blurred = t.Focused
	return t
}

// readInput reads a (possibly multi-line) request from the user.
// In interactive mode it delegates to charmbracelet/huh; when driven
// by an io.Reader (tests, pipes) it reads lines via bufio.Scanner.
func (r *REPL) readInput(prompt string) (string, error) {
	if r.interactive() {
		return r.readInputHuh(prompt)
	}
	return r.readInputLines()
}

// readInputHuh uses charmbracelet/huh for interactive terminal input.
func (r *REPL) readInputHuh(prompt string) (string, error) {
	theme := inputTheme()
	var parts []string
	cur := prompt
	for {
		var val string
		err := huh.NewInput().
			Prompt(cur + " ").
			Value(&val).
			WithTheme(theme).
			Run()
		if err != nil {
			return "", err
		}
		// Empty input on first line → return empty so Loop skips it.
		if val == "" && len(parts) == 0 {
			return "", nil
		}
		if !strings.HasSuffix(val, "\\") {
			parts = append(parts, val)
			return strings.TrimSpace(strings.Join(parts, "\n")), nil
		}
		parts = append(parts, strings.TrimSuffix(val, "\\"))
		cur = "…"
	}
}

// readInputLines reads from r.in using a scanner. Supports multi-line
// continuation (trailing \) and prints prompt/promptCont to r.out so
// test assertions can verify continuation behavior.
func (r *REPL) readInputLines() (string, error) {
	if r.scanner == nil {
		r.scanner = bufio.NewScanner(r.in)
	}
	prompt := r.prompt
	if prompt != "" {
		fmt.Fprint(r.out, prompt)
	}
	var parts []string
	for r.scanner.Scan() {
		line := r.scanner.Text()
		if strings.HasSuffix(line, "\\") {
			parts = append(parts, strings.TrimSuffix(line, "\\"))
			// Print continuation prompt.
			if r.promptCont != "" {
				fmt.Fprint(r.out, r.promptCont)
			}
			continue
		}
		parts = append(parts, line)
		return strings.TrimSpace(strings.Join(parts, "\n")), nil
	}
	if err := r.scanner.Err(); err != nil {
		return "", err
	}
	return "", io.EOF
}

// dispatch runs one user request through the orchestrator and prints
// the result, then records the turn in memory.
func (r *REPL) dispatch(line string) {
	prior := r.store.BuildContext(line)
	effective := line
	if prior != "" {
		effective = "## Prior conversation\n" + prior + "\n\n## Current request\n" + line
	}

	logging.Info("[repl] dispatching request: %s", oneLine(line))

	if r.renderer != nil {
		r.renderer.StartSpinner()
	}

	busCtx, err := r.runner.Run(effective, r.repoRoot, r.branch)

	if r.renderer != nil {
		// Stop spinner BEFORE printing response so task list comes first.
		r.renderer.StopSpinner()
	}

	if err != nil {
		logging.Error("[repl] orchestrator error: %v", err)
		r.errorf("error: %v\n", err)
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	logging.Info("[repl] final answer:\n%s", response)

	// Decide what to persist in memory. When the pipeline ended with
	// a TaskState.LastError, the rendered response includes an
	// "error: ..." prefix carrying the internal error text (task UUIDs,
	// oscillation guard messages, OpenAI 400 payloads, etc.). Persisting
	// that verbatim pollutes every subsequent REPL turn's
	// "### Recent conversation" block — the LLM sees historical errors
	// in its prior-conversation context and may be swayed into thinking
	// those failures are current state. The user still sees the full
	// error in their terminal via the rendering below; only what
	// reaches memory is sanitized.
	//
	// See memory/project_repl_memory_error_pollution.md for the
	// diagnostic trail.
	memResponse := response
	if busCtx != nil && busCtx.TaskState.LastError != "" {
		memResponse = "(previous attempt ended in error — details omitted from memory)"
	}

	// Skip rendering if no meaningful content.
	if response == "" || response == "(no result)" {
		fmt.Fprintln(r.out, "  ??")
		r.recordTurn(line, memResponse)
		return
	}

	// Render model output with a continuous left border. Strip trailing
	// ANSI escapes and whitespace so the bar aligns cleanly.
	raw := strings.Split(response, "\n")
	var lines []string
	prevBlank := false
	for _, ln := range raw {
		clean := stripTrailing(ln)
		blank := stripANSI(clean) == ""
		if blank && prevBlank {
			continue
		}
		prevBlank = blank
		lines = append(lines, clean)
	}
	for len(lines) > 0 && stripANSI(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && stripANSI(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	bar := pterm.FgWhite.Sprint("│")
	// Wrap each line to fit the terminal, accounting for the
	// "  │ " prefix (4 display cols + 2 margin).
	maxContent := 60
	if w, _, werr := term.GetSize(int(os.Stdout.Fd())); werr == nil && w > 10 {
		maxContent = w - 6
	}
	fmt.Fprintf(r.out, "  %s\n", bar)
	for _, ln := range lines {
		wrapped := wrapByWidth(ln, maxContent)
		for _, wl := range wrapped {
			fmt.Fprintf(r.out, "  %s %s\n", bar, wl)
		}
	}
	fmt.Fprintf(r.out, "  %s\n\n", bar)

	r.recordTurn(line, memResponse)
}

func (r *REPL) recordTurn(request, response string) {
	turn := memory.Turn{
		ID:        fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		Request:   request,
		Response:  response,
		Timestamp: time.Now(),
	}
	if err := r.store.Append(turn); err != nil {
		logging.Warning("[repl] memory append failed: %v", err)
	}
}

// handleSlash returns true if the loop should exit.
func (r *REPL) handleSlash(line string) bool {
	cmd := strings.Fields(line)[0]
	switch cmd {
	case "/exit", "/quit":
		fmt.Fprintln(r.out, "  Goodbye!")
		return true
	case "/clear":
		peers, perr := r.store.LivePeerCount()
		if perr != nil {
			logging.Warning("[repl] live peer count failed: %v", perr)
		}
		msg := "/clear wipes this conversation memory (MEMORY.md + turns/)."
		if peers == 1 {
			msg += " 1 other live codrax instance shares this directory."
		} else if peers > 1 {
			msg += fmt.Sprintf(" %d other live codrax instances share this directory.", peers)
		}
		confirmed := false
		if r.interactive() {
			err := huh.NewConfirm().
				Title(msg).
				Affirmative("Yes").
				Negative("No").
				Value(&confirmed).
				Run()
			if err != nil {
				confirmed = false
			}
		} else {
			// Line-oriented mode: print message and read y/n.
			fmt.Fprintln(r.out, msg)
			line, err := r.readInputLines()
			if err == nil {
				confirmed = strings.TrimSpace(strings.ToLower(line)) == "y"
			}
		}
		if !confirmed {
			r.info("clear cancelled")
			break
		}
		if err := r.store.Clear(); err != nil {
			r.errorf("clear failed: %v\n", err)
		} else {
			r.success("conversation memory cleared.")
		}
	case "/history":
		recent := r.store.Recent()
		idx := r.store.Index()
		if len(recent) == 0 && len(idx) == 0 {
			r.info("(empty)")
			return false
		}
		if len(idx) > 0 {
			fmt.Fprintln(r.out, "  compacted index:")
			for _, e := range idx {
				fmt.Fprintf(r.out, "    - [%s] %s — keywords: %s\n", e.ID, e.Topic, strings.Join(e.Keywords, ", "))
			}
		}
		if len(recent) > 0 {
			fmt.Fprintln(r.out, "  recent turns:")
			for _, t := range recent {
				fmt.Fprintf(r.out, "    - [%s] %s\n", t.Timestamp.Format("15:04:05"), oneLine(t.Request))
			}
		}
	case "/compact":
		if err := r.store.Compact(); err != nil {
			r.errorf("compact failed: %v\n", err)
		} else {
			r.success(fmt.Sprintf("compaction done. recent=%d index=%d", len(r.store.Recent()), len(r.store.Index())))
		}
	case "/help":
		r.info("commands: /exit /quit /clear /history /compact /help")
		r.info("tip: end a line with \\ for multi-line input")
	default:
		r.warn("unknown command %q — try /help\n", cmd)
	}
	return false
}

var reANSI = regexp.MustCompile(`\x1b\[[0-9;]*m`)
var reTrailing = regexp.MustCompile(`(\s|\x1b\[[0-9;]*m)+$`)

// stripTrailing removes all trailing whitespace and ANSI sequences.
func stripTrailing(s string) string {
	return reTrailing.ReplaceAllString(s, "")
}

// stripANSI removes all ANSI escape sequences to get plain text.
func stripANSI(s string) string {
	return strings.TrimSpace(reANSI.ReplaceAllString(s, ""))
}

// wrapByWidth breaks a line into multiple lines that each fit within
// maxCols display columns. ANSI escape sequences are preserved: they
// are skipped for width calculation, and active SGR styles are replayed
// at the start of each continuation line so colours/bold survive wraps.
func wrapByWidth(s string, maxCols int) []string {
	if s == "" {
		return []string{""}
	}
	var result []string
	var activeSeqs []string // SGR sequences active at current position

	for len(s) > 0 {
		var buf strings.Builder
		// Replay active styles on continuation lines.
		if len(result) > 0 {
			for _, seq := range activeSeqs {
				buf.WriteString(seq)
			}
		}

		w := 0
		i := 0
		hasVisible := false

		for i < len(s) {
			// Detect ANSI CSI sequence: ESC [ <params> <final byte>
			if s[i] == '\x1b' && i+1 < len(s) && s[i+1] == '[' {
				j := i + 2
				for j < len(s) && ((s[j] >= '0' && s[j] <= '9') || s[j] == ';' || s[j] == ':') {
					j++
				}
				if j < len(s) {
					j++ // final byte (m, H, K, …)
				}
				seq := s[i:j]
				buf.WriteString(seq)
				// Track SGR (Select Graphic Rendition) sequences for
				// style continuation across wrapped lines.
				if len(seq) > 0 && seq[len(seq)-1] == 'm' {
					if seq == "\x1b[0m" || seq == "\x1b[m" {
						activeSeqs = nil
					} else {
						activeSeqs = append(activeSeqs, seq)
					}
				}
				i = j
				continue
			}

			ch, size := utf8.DecodeRuneInString(s[i:])
			rw := runewidth.RuneWidth(ch)
			if w+rw > maxCols && hasVisible {
				break
			}
			buf.WriteString(s[i : i+size])
			w += rw
			i += size
			hasVisible = true
		}

		s = s[i:]
		// Reset styles at end of wrapped line so they don't bleed
		// into the "│" prefix on the next line.
		if len(s) > 0 && len(activeSeqs) > 0 {
			buf.WriteString("\x1b[0m")
		}
		result = append(result, buf.String())
	}
	return result
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// ensure huh.ErrUserAborted is treated as EOF for graceful shutdown.
func init() {
	_ = errors.Is(huh.ErrUserAborted, io.EOF) // compile-time check that the type exists
}
