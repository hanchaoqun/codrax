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

// attachedLogSetter is the optional capability the REPL probes on
// its Runner to propagate a sticky /log attachment before dispatch.
// Real orchestrators implement SetAttachedLog; test stubs that omit
// it simply don't see log-triage attachments — the REPL falls back
// to the pre-flag behaviour.
type attachedLogSetter interface {
	SetAttachedLog(string)
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
	In       io.Reader // nil → interactive (bubbletea); non-nil → line-oriented
	Out      io.Writer

	// UI customization (used by line-oriented mode).
	Prompt     string // primary prompt, e.g. ">"
	PromptCont string // continuation prompt, e.g. "."
	Banner     string // printed once at start; empty → default badge

	// PasteFoldMinChars is the rune-count threshold above which a
	// single-line paste gets folded into a placeholder. Multi-line
	// pastes always fold regardless of length. Zero or negative →
	// DefaultPasteFoldMinChars. Surfaces
	// codrax.yaml :: repl_paste_fold_min_chars.
	PasteFoldMinChars int
}

// REPL drives the interactive prompt.
type REPL struct {
	runner            Runner
	store             *memory.Store
	render            ResultRenderer
	renderer          *render.Renderer
	repoRoot          string
	branch            string
	in                io.Reader
	out               io.Writer
	prompt            string
	promptCont        string
	bannerText        string
	scanner           *bufio.Scanner // lazy-init for line-oriented mode
	pasteFoldMinChars int            // per-session paste-fold threshold (runes)

	// attachedLog holds the runtime log excerpt the user attached via
	// /log or `--log`. Sticky across turns until /log clear — users
	// typically investigate the same panic from several angles
	// ("root cause?" → "safe fix?" → "regression risk?"). Propagated
	// to the runner via attachedLogSetter before each dispatch.
	attachedLog string

	// pendingPaste is the line-oriented fallback for terminals /
	// tmux configurations that swallow bracketed paste (most common
	// in SSH → tmux environments). `/paste` captures lines via a
	// plain bufio.Scanner until `/end`; the captured content is
	// stashed here, and the next interactive readInput call injects
	// it into the new Bubble Tea model as a placeholder seed. Single-
	// use; cleared on consumption or on abort of the seeded turn.
	pendingPaste string
}

// New constructs a REPL from a Config.
func New(cfg Config) *REPL {
	r := &REPL{
		runner:            cfg.Runner,
		store:             cfg.Store,
		render:            cfg.Render,
		renderer:          cfg.Renderer,
		repoRoot:          cfg.RepoRoot,
		branch:            cfg.Branch,
		in:                cfg.In,
		out:               cfg.Out,
		prompt:            cfg.Prompt,
		promptCont:        cfg.PromptCont,
		bannerText:        cfg.Banner,
		pasteFoldMinChars: cfg.PasteFoldMinChars,
	}
	// Seed sticky log from whatever the runner already has (CLI set
	// `--log` before handing off to the REPL). Keeps the invariant
	// "REPL.attachedLog is the single source of truth" from the
	// first /log show onward.
	if getter, ok := cfg.Runner.(interface{ AttachedLog() string }); ok {
		r.attachedLog = getter.AttachedLog()
	}
	return r
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
//
// The interactive readInput session renders its own echo + divider
// above the inline viewport via tea.Printf, so this function only
// owns echo in the line-oriented (scripted/piped) branch.
func (r *REPL) Loop() error {
	r.banner()
	for {
		line, display, err := r.readInputPair("❯❯")
		if err != nil {
			fmt.Fprintln(r.out)
			fmt.Fprintln(r.out, "  Goodbye!")
			return nil
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if !r.interactive() {
			// Scanner mode: preserve the old echo so piped test output
			// still contains a "> …" marker for visual assertions.
			fmt.Fprintf(r.out, "  > %s\n", display)
		}
		if cmd := types.NormalizeREPLCommandAlias(line); cmd != "" {
			if quit := r.handleSlash(cmd); quit {
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
// In interactive mode it delegates to a Bubble Tea session
// (readInputInteractive) that carries paste-folding, history, and
// slash-command autocomplete. When driven by an io.Reader (tests,
// pipes) it reads lines via bufio.Scanner unchanged.
//
// Returned display/expanded split: expanded is what reaches the
// orchestrator (placeholders inlined to their original pasted
// payload); display is what the user actually saw on their input line
// (placeholders kept compact). The Bubble Tea session echoes display
// itself via tea.Printf, so Loop() no longer needs to echo in the
// interactive branch.
func (r *REPL) readInput(prompt string) (string, error) {
	expanded, _, err := r.readInputPair(prompt)
	return expanded, err
}

// readInputPair returns (expanded, display, error). In line-oriented
// mode expanded and display are identical.
func (r *REPL) readInputPair(prompt string) (string, string, error) {
	if r.interactive() {
		return r.readInputInteractive(prompt)
	}
	s, err := r.readInputLines()
	return s, s, err
}

// readInputInteractive runs the Bubble Tea input session, looping for
// trailing-"\" continuation. Each invocation can fold its own pastes
// into independent placeholder slots.
func (r *REPL) readInputInteractive(prompt string) (string, string, error) {
	cur := prompt
	var expandedParts, displayParts []string
	for {
		res, err := r.readInputBubble(cur, len(expandedParts) > 0)
		if err != nil {
			return "", "", err
		}
		if res.aborted {
			return "", "", io.EOF
		}
		expandedParts = append(expandedParts, res.expanded)
		displayParts = append(displayParts, res.display)
		if !res.continues {
			expanded := strings.TrimSpace(strings.Join(expandedParts, "\n"))
			display := strings.TrimSpace(strings.Join(displayParts, "\n"))
			return expanded, display, nil
		}
		cur = "…"
	}
}

// scriptedScanner returns the single long-lived bufio.Scanner that
// reads from r.in in scripted mode. Two callers share it:
// readInputLines for normal lines and handle{Log,Paste}Cmd for the
// multi-line capture loops. Creating a fresh Scanner inside the
// capture helpers would conflict with readInputLines' prior read-
// ahead buffering (the capture's Scan() would see EOF because the
// outer scanner has already consumed bytes into its internal buffer).
// Caller must check r.in != nil first.
func (r *REPL) scriptedScanner() *bufio.Scanner {
	if r.scanner == nil {
		r.scanner = bufio.NewScanner(r.in)
		// Lift the token cap from bufio's 64 KiB default so a single
		// long pasted line (a minified JSON blob, a flattened stack
		// trace) doesn't silently truncate — matches the paste/log
		// capture ceiling enforced elsewhere.
		r.scanner.Buffer(make([]byte, 64*1024), maxREPLAttachedLogBytes+1)
	}
	return r.scanner
}

// captureScanner is the bufio.Scanner used by /log paste and /paste
// for multi-line capture. In scripted mode it reuses r.scanner so
// reads pick up where readInputLines left off (a second scanner would
// miss any bytes the first has already buffered ahead). In
// interactive mode the Bubble Tea session has already quit and
// restored cooked mode, so a fresh os.Stdin scanner is fine — nothing
// else is reading from stdin during capture.
func (r *REPL) captureScanner() *bufio.Scanner {
	if r.in != nil {
		return r.scriptedScanner()
	}
	s := bufio.NewScanner(os.Stdin)
	s.Buffer(make([]byte, 64*1024), maxREPLAttachedLogBytes+1)
	return s
}

// readInputLines reads from r.in using a scanner. Supports multi-line
// continuation (trailing \) and prints prompt/promptCont to r.out so
// test assertions can verify continuation behavior.
func (r *REPL) readInputLines() (string, error) {
	r.scriptedScanner()
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
	// T1.1: auto-route pasted log content into AttachedLog so the
	// log_triage pre-stage parses it with the LLM instead of letting
	// the analyzer's TermGraph / keyword_search be poisoned by log
	// timestamps and stack-frame literals. Only fires when the user
	// has NOT already set an explicit log via /log — explicit > auto.
	if r.attachedLog == "" {
		cleaned, detected := splitPastedLog(line)
		if detected != "" {
			if len(detected) > maxREPLAttachedLogBytes {
				r.warn("auto-detected log hit %d-byte cap; truncating\n", maxREPLAttachedLogBytes)
				detected = detected[:maxREPLAttachedLogBytes]
			}
			r.attachedLog = detected
			r.info(fmt.Sprintf("auto-attached log: %d bytes (/log clear to remove)", len(detected)))
			line = cleaned
			if strings.TrimSpace(line) == "" {
				// All the user typed was the log. Supply a minimal
				// placeholder so the analyzer still has a request
				// string; log_triage will drive the intent.
				line = "分析附带的日志"
			}
		}
	}

	prior := r.store.BuildContext(line)
	effective := line
	if prior != "" {
		effective = "## Prior conversation\n" + prior + "\n\n## Current request\n" + line
	}

	logging.Info("[repl] dispatching request: %s", oneLine(line))

	// Propagate sticky attached-log to the runner. Runners without
	// SetAttachedLog simply skip this step (tests).
	if setter, ok := r.runner.(attachedLogSetter); ok {
		setter.SetAttachedLog(r.attachedLog)
	}

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

// maxREPLAttachedLogBytes caps the /log paste-mode payload so a
// runaway paste can't balloon memory. Matches cmd.maxAttachedLogBytes.
const maxREPLAttachedLogBytes = 1 << 20 // 1 MB

// handleSlash returns true if the loop should exit.
func (r *REPL) handleSlash(line string) bool {
	cmd := strings.Fields(line)[0]
	switch cmd {
	case "/log":
		r.handleLogCmd(line)
		return false
	case "/paste":
		r.handlePasteCmd()
		return false
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
		r.info("commands: /exit /quit /clear /history /compact /log /paste /help")
		r.info("tip: end a line with \\ for multi-line input")
		r.info("log: /log <path> | /log (paste then /end) | /log clear | /log show")
		r.info("paste: /paste (terminal-independent fallback when bracketed paste is stripped)")
	default:
		r.warn("unknown command %q — try /help\n", cmd)
	}
	return false
}

// handleLogCmd dispatches the `/log` subcommands. Recognised forms:
//
//	/log <path>   — load a log file from disk (replaces any current)
//	/log clear    — drop the current attached log
//	/log show     — print the first 20 lines + total byte count
//	/log          — enter paste mode; subsequent lines form the log
//	                until a lone `/end` line terminates capture
//
// Sticky semantics: an attachment survives across turns and across
// /clear (which only wipes conversation memory). Only /log clear or
// a new /log path/paste replaces it.
func (r *REPL) handleLogCmd(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/log"))
	switch {
	case rest == "":
		r.handleLogPaste()
	case rest == "clear":
		if r.attachedLog == "" {
			r.info("no log attached")
			return
		}
		r.attachedLog = ""
		if setter, ok := r.runner.(attachedLogSetter); ok {
			setter.SetAttachedLog("")
		}
		r.success("attached log cleared")
	case rest == "show":
		r.handleLogShow()
	default:
		r.handleLogLoad(rest)
	}
}

// handleLogLoad reads a log file from disk into the sticky buffer.
// Replaces any existing attachment (a `/log append` variant is a
// future add; users can cat files together themselves for now).
func (r *REPL) handleLogLoad(path string) {
	data, err := os.ReadFile(path)
	if err != nil {
		r.errorf("load log: %v\n", err)
		return
	}
	if len(data) > maxREPLAttachedLogBytes {
		r.warn("log truncated: %d → %d bytes\n", len(data), maxREPLAttachedLogBytes)
		data = data[:maxREPLAttachedLogBytes]
	}
	r.attachedLog = string(data)
	r.success(fmt.Sprintf("attached log loaded: %s (%d bytes)", path, len(data)))
}

// handleLogPaste enters a multi-line capture mode. Every subsequent
// line is appended to the buffer until a sole `/end` line (or EOF)
// terminates capture. Empty capture leaves the prior attachment
// unchanged.
//
// Interactive capture deliberately bypasses the Bubble Tea input
// session: placeholder folding would replace a large paste with a
// token instead of storing the raw log content that log-triage
// actually needs. We read straight from the caller's input stream
// (os.Stdin in production, r.in in tests) one line at a time.
func (r *REPL) handleLogPaste() {
	r.info("paste log, terminate with a lone /end line")
	scanner := r.captureScanner()
	var buf strings.Builder
	for {
		if r.interactive() {
			fmt.Fprint(r.out, "  log> ")
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if trim == "/end" || trim == "\\end" {
			break
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if buf.Len() > maxREPLAttachedLogBytes {
			r.warn("log paste hit %d-byte cap; stopping capture\n", maxREPLAttachedLogBytes)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		logging.Warning("[repl] log paste scan error: %v", err)
	}
	if buf.Len() == 0 {
		r.info("no input captured; attached log unchanged")
		return
	}
	r.attachedLog = buf.String()
	r.success(fmt.Sprintf("attached log captured: %d bytes", buf.Len()))
}

// handlePasteCmd is the terminal-independent fallback for the main
// input. bracketed paste is stripped in common SSH/tmux setups; when
// that happens an auto-fold never fires because the paste arrives as
// individual KeyRunes instead of one tea.KeyMsg{Paste:true}. This
// command reads lines straight from the caller's input stream until
// a lone `/end`, stores the collected bytes in r.pendingPaste, and
// lets the next readInputBubble inject it as a pre-seeded placeholder
// token. User can then edit around the token and submit normally.
func (r *REPL) handlePasteCmd() {
	r.info("paste content, terminate with a lone /end line; press Enter to cancel an empty capture")
	scanner := r.captureScanner()
	var buf strings.Builder
	for {
		if r.interactive() {
			fmt.Fprint(r.out, "  paste> ")
		}
		if !scanner.Scan() {
			break
		}
		line := scanner.Text()
		trim := strings.TrimSpace(line)
		if trim == "/end" || trim == "\\end" {
			break
		}
		buf.WriteString(line)
		buf.WriteByte('\n')
		if buf.Len() > maxREPLAttachedLogBytes {
			r.warn("paste hit %d-byte cap; stopping capture\n", maxREPLAttachedLogBytes)
			break
		}
	}
	if err := scanner.Err(); err != nil {
		logging.Warning("[repl] /paste scan error: %v", err)
	}
	captured := strings.TrimRight(buf.String(), "\n")
	if captured == "" {
		r.info("no input captured")
		return
	}
	r.pendingPaste = captured
	chars := 0
	for range captured {
		chars++
	}
	lines := strings.Count(captured, "\n") + 1
	r.success(fmt.Sprintf("captured %d lines, %d chars → placed as [Pasted text #0] in next prompt", lines, chars))
}

// handleLogShow prints a preview of the currently-attached log.
func (r *REPL) handleLogShow() {
	if r.attachedLog == "" {
		r.info("no log attached")
		return
	}
	lines := strings.Split(r.attachedLog, "\n")
	head := lines
	if len(head) > 20 {
		head = lines[:20]
	}
	fmt.Fprintf(r.out, "  attached log: %d bytes, %d lines\n", len(r.attachedLog), len(lines))
	for _, ln := range head {
		fmt.Fprintf(r.out, "    | %s\n", ln)
	}
	if len(lines) > 20 {
		fmt.Fprintf(r.out, "    | ... [%d more lines]\n", len(lines)-20)
	}
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
