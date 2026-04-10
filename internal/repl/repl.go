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
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/memory"
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

// Config holds all REPL construction parameters.
type Config struct {
	Runner       Runner
	Store        *memory.Store
	Render       ResultRenderer
	RepoRoot     string
	Branch       string
	In           io.Reader
	Out          io.Writer
	Prompt       string // styled input prompt (e.g. "❯")
	PromptCont   string // styled continuation prompt (e.g. "…")
	Banner       string // styled welcome banner
	BorderTop    string // styled top border for input box
	BorderBottom string // styled bottom border for input box
}

// REPL drives the interactive prompt.
type REPL struct {
	runner       Runner
	store        *memory.Store
	render       ResultRenderer
	repoRoot     string
	branch       string
	in           *bufio.Reader
	out          io.Writer
	prompt       string
	promptCont   string
	banner       string
	borderTop    string
	borderBottom string
}

// New constructs a REPL from the given Config.
func New(cfg Config) *REPL {
	return &REPL{
		runner:       cfg.Runner,
		store:        cfg.Store,
		render:       cfg.Render,
		repoRoot:     cfg.RepoRoot,
		branch:       cfg.Branch,
		in:           bufio.NewReader(cfg.In),
		out:          cfg.Out,
		prompt:       cfg.Prompt,
		promptCont:   cfg.PromptCont,
		banner:       cfg.Banner,
		borderTop:    cfg.BorderTop,
		borderBottom: cfg.BorderBottom,
	}
}

// Loop runs the prompt until /exit, /quit, or EOF.
func (r *REPL) Loop() error {
	r.printBanner()
	for {
		input, err := r.readInput()
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(r.out)
			fmt.Fprintf(r.out, "  Goodbye!\n")
			return nil
		}
		if err != nil {
			return err
		}
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			if quit := r.handleSlash(input); quit {
				return nil
			}
			continue
		}
		r.dispatch(input)
	}
}

func (r *REPL) printBanner() {
	fmt.Fprintf(r.out, "%s\n", r.banner)
}

// readInput reads one (possibly multi-line) user input, enclosed in
// a ╭───╮ / ╰───╯ input box.
func (r *REPL) readInput() (string, error) {
	// Print top border + prompt.
	fmt.Fprintf(r.out, "\n%s\n  %s ", r.borderTop, r.prompt)

	line, err := r.in.ReadString('\n')
	if errors.Is(err, io.EOF) {
		line = strings.TrimRight(line, "\r\n")
		if line != "" {
			fmt.Fprintf(r.out, "%s\n", r.borderBottom)
			return strings.TrimSpace(line), nil
		}
		return "", err
	}
	if err != nil {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")

	if !strings.HasSuffix(line, "\\") {
		fmt.Fprintf(r.out, "%s\n", r.borderBottom)
		return strings.TrimSpace(line), nil
	}

	// Multi-line continuation: collect lines ending with \.
	var parts []string
	for strings.HasSuffix(line, "\\") {
		parts = append(parts, strings.TrimSuffix(line, "\\"))
		fmt.Fprintf(r.out, "  %s ", r.promptCont)
		line, err = r.in.ReadString('\n')
		if errors.Is(err, io.EOF) {
			line = strings.TrimRight(line, "\r\n")
			parts = append(parts, line)
			fmt.Fprintf(r.out, "%s\n", r.borderBottom)
			return strings.TrimSpace(strings.Join(parts, "\n")), nil
		}
		if err != nil {
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
	}
	parts = append(parts, line)
	fmt.Fprintf(r.out, "%s\n", r.borderBottom)
	return strings.TrimSpace(strings.Join(parts, "\n")), nil
}

// dispatch runs one user request through the orchestrator and prints
// the result, then records the turn in memory.
func (r *REPL) dispatch(input string) {
	prior := r.store.BuildContext(input)
	effective := input
	if prior != "" {
		effective = "## Prior conversation\n" + prior + "\n\n## Current request\n" + input
	}

	logging.Info("[repl] dispatching request: %s", oneLine(input))
	busCtx, err := r.runner.Run(effective, r.repoRoot, r.branch)
	if err != nil {
		logging.Error("[repl] orchestrator error: %v", err)
		fmt.Fprintf(r.out, "\n  error: %v\n", err)
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	logging.Info("[repl] final answer:\n%s", response)
	fmt.Fprintf(r.out, "\n%s\n", response)

	turn := memory.Turn{
		ID:        fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		Request:   input,
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
		fmt.Fprintf(r.out, "  Goodbye!\n")
		return true
	case "/clear":
		peers, perr := r.store.LivePeerCount()
		if perr != nil {
			logging.Warning("[repl] live peer count failed: %v", perr)
		}
		switch {
		case peers == 0:
			fmt.Fprintf(r.out, "  /clear wipes this conversation memory (MEMORY.md + turns/). Confirm? [y/N]: ")
		case peers == 1:
			fmt.Fprintf(r.out, "  /clear wipes shared memory (MEMORY.md + turns/) — 1 other live codrax instance is also using this directory and will see the wipe on its next read. Confirm? [y/N]: ")
		default:
			fmt.Fprintf(r.out, "  /clear wipes shared memory (MEMORY.md + turns/) — %d other live codrax instances are also using this directory and will see the wipe on their next read. Confirm? [y/N]: ", peers)
		}
		answer, err := r.in.ReadString('\n')
		if err != nil {
			fmt.Fprintf(r.out, "  clear cancelled\n")
			break
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintf(r.out, "  clear cancelled\n")
			break
		}
		if err := r.store.Clear(); err != nil {
			fmt.Fprintf(r.out, "  clear failed: %v\n", err)
		} else {
			fmt.Fprintf(r.out, "  conversation memory cleared.\n")
		}
	case "/history":
		recent := r.store.Recent()
		idx := r.store.Index()
		if len(recent) == 0 && len(idx) == 0 {
			fmt.Fprintf(r.out, "  (empty)\n")
			return false
		}
		if len(idx) > 0 {
			fmt.Fprintf(r.out, "  compacted index:\n")
			for _, e := range idx {
				fmt.Fprintf(r.out, "    - [%s] %s — keywords: %s\n", e.ID, e.Topic, strings.Join(e.Keywords, ", "))
			}
		}
		if len(recent) > 0 {
			fmt.Fprintf(r.out, "  recent turns:\n")
			for _, t := range recent {
				fmt.Fprintf(r.out, "    - [%s] %s\n", t.Timestamp.Format("15:04:05"), oneLine(t.Request))
			}
		}
	case "/compact":
		if err := r.store.Compact(); err != nil {
			fmt.Fprintf(r.out, "  compact failed: %v\n", err)
		} else {
			fmt.Fprintf(r.out, "  compaction done. recent=%d index=%d\n", len(r.store.Recent()), len(r.store.Index()))
		}
	case "/help":
		fmt.Fprintf(r.out, "  commands: /exit /quit /clear /history /compact /help\n")
		fmt.Fprintf(r.out, "  tip: end a line with \\ for multi-line input\n")
	default:
		fmt.Fprintf(r.out, "  unknown command %q — try /help\n", cmd)
	}
	return false
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}
