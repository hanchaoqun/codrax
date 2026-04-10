// Package repl implements the interactive multi-turn loop for codrax.
//
// When the binary is launched with no -request, main.go hands control
// to this package. Each user line is dispatched as a fresh
// orchestrator.Run, with prior conversation injected into the request
// string via memory.Store.BuildContext. Slash commands manipulate the
// store directly without going through the orchestrator.
//
// Multi-line input is supported: end a line with \ to continue on the
// next line. The continuation lines are joined with newlines.
package repl

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

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

// REPL drives the interactive prompt.
type REPL struct {
	runner   Runner
	store    *memory.Store
	render   ResultRenderer
	renderer *render.Renderer
	repoRoot string
	branch   string
	out      io.Writer
}

// New constructs a REPL.
func New(runner Runner, store *memory.Store, renderFn ResultRenderer, renderer *render.Renderer, repoRoot, branch string, out io.Writer) *REPL {
	return &REPL{
		runner:   runner,
		store:    store,
		render:   renderFn,
		renderer: renderer,
		repoRoot: repoRoot,
		branch:   branch,
		out:      out,
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
		// Echo user input so it's visible after huh clears.
		pterm.FgLightCyan.Printf("  > %s\n", line)
		pterm.FgDarkGray.Println("  ─────────────────────────────────────")
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
	badge := pterm.NewStyle(pterm.BgBlue, pterm.FgWhite, pterm.Bold).Sprint(" CODRAX ")
	hint := pterm.FgDarkGray.Sprint("/help · /exit")
	fmt.Fprintf(r.out, "\n  %s  %s\n\n", badge, hint)
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

// readInput uses charmbracelet/huh for interactive input. Handles
// multi-line continuation when a line ends with \.
func (r *REPL) readInput(prompt string) (string, error) {
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
		if !strings.HasSuffix(val, "\\") {
			parts = append(parts, val)
			return strings.TrimSpace(strings.Join(parts, "\n")), nil
		}
		parts = append(parts, strings.TrimSuffix(val, "\\"))
		cur = "…"
	}
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

	r.renderer.StartSpinner()

	busCtx, err := r.runner.Run(effective, r.repoRoot, r.branch)

	// Stop spinner BEFORE printing response so task list comes first.
	r.renderer.StopSpinner()

	if err != nil {
		logging.Error("[repl] orchestrator error: %v", err)
		pterm.Error.Printf("error: %v\n", err)
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	logging.Info("[repl] final answer:\n%s", response)

	// Skip rendering if no meaningful content.
	if response == "" || response == "(no result)" {
		pterm.FgDarkGray.Println("  ??")
		r.recordTurn(line, response)
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
	fmt.Fprintf(r.out, "  %s\n", bar)
	for _, ln := range lines {
		fmt.Fprintf(r.out, "  %s %s\n", bar, ln)
	}
	fmt.Fprintf(r.out, "  %s\n\n", bar)

	r.recordTurn(line, response)
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
		var confirmed bool
		err := huh.NewConfirm().
			Title(msg).
			Affirmative("Yes").
			Negative("No").
			Value(&confirmed).
			Run()
		if err != nil || !confirmed {
			pterm.Info.Println("clear cancelled")
			break
		}
		if err := r.store.Clear(); err != nil {
			pterm.Error.Printf("clear failed: %v\n", err)
		} else {
			pterm.Success.Println("conversation memory cleared.")
		}
	case "/history":
		recent := r.store.Recent()
		idx := r.store.Index()
		if len(recent) == 0 && len(idx) == 0 {
			pterm.Info.Println("(empty)")
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
			pterm.Error.Printf("compact failed: %v\n", err)
		} else {
			pterm.Success.Printf("compaction done. recent=%d index=%d\n", len(r.store.Recent()), len(r.store.Index()))
		}
	case "/help":
		pterm.Info.Println("commands: /exit /quit /clear /history /compact /help")
		pterm.Info.Println("tip: end a line with \\ for multi-line input")
	default:
		pterm.Warning.Printf("unknown command %q — try /help\n", cmd)
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
