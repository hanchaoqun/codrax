// Package repl implements the interactive multi-turn loop for codrax.
//
// When the binary is launched with no -request, main.go hands control
// to this package. Each user line is dispatched as a fresh
// orchestrator.Run, with prior conversation injected into the request
// string via memory.Store.BuildContext. Slash commands manipulate the
// store directly without going through the orchestrator.
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

// REPL drives the interactive prompt.
type REPL struct {
	runner   Runner
	store    *memory.Store
	render   ResultRenderer
	repoRoot string
	branch   string
	in       *bufio.Reader
	out      io.Writer
}

// New constructs a REPL.
func New(runner Runner, store *memory.Store, render ResultRenderer, repoRoot, branch string, in io.Reader, out io.Writer) *REPL {
	return &REPL{
		runner:   runner,
		store:    store,
		render:   render,
		repoRoot: repoRoot,
		branch:   branch,
		in:       bufio.NewReader(in),
		out:      out,
	}
}

// Loop runs the prompt until /exit, /quit, or EOF.
func (r *REPL) Loop() error {
	r.banner()
	for {
		fmt.Fprint(r.out, "You> ")
		line, err := r.in.ReadString('\n')
		if errors.Is(err, io.EOF) {
			fmt.Fprintln(r.out)
			fmt.Fprintln(r.out, "Codrax> bye")
			return nil
		}
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
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
	fmt.Fprintln(r.out, "codrax interactive mode — type /help for commands, /exit to quit.")
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
	busCtx, err := r.runner.Run(effective, r.repoRoot, r.branch)
	if err != nil {
		logging.Error("[repl] orchestrator error: %v", err)
		fmt.Fprintf(r.out, "\nCodrax> error: %v\n\n", err)
		return
	}

	response := strings.TrimSpace(r.render(busCtx))
	// Record the final user-visible answer in the log file so audits
	// and post-mortems do not have to scrape stdout. Info level so it
	// survives the default log level without needing -log-level debug.
	logging.Info("[repl] final answer:\n%s", response)
	fmt.Fprintf(r.out, "\nCodrax> %s\n\n", response)

	turn := memory.Turn{
		ID:        fmt.Sprintf("turn-%d", time.Now().UnixNano()),
		Request:   line,
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
		fmt.Fprintln(r.out, "Codrax> bye")
		return true
	case "/clear":
		// /clear wipes MEMORY.md and turns/, which are *shared* with
		// any sibling codrax instances pointed at the same memory
		// directory (per-target-repo namespacing means "same target
		// repo" by default). Surface the impact before doing it.
		// LivePeerCount looks at sidecar marker files dropped by each
		// Store at NewStore time; the count excludes us and any
		// crashed peers whose PID is no longer alive.
		peers, perr := r.store.LivePeerCount()
		if perr != nil {
			logging.Warning("[repl] live peer count failed: %v", perr)
		}
		switch {
		case peers == 0:
			fmt.Fprint(r.out, "Codrax> /clear wipes this conversation memory (MEMORY.md + turns/). Confirm? [y/N]: ")
		case peers == 1:
			fmt.Fprint(r.out, "Codrax> /clear wipes shared memory (MEMORY.md + turns/) — 1 other live codrax instance is also using this directory and will see the wipe on its next read. Confirm? [y/N]: ")
		default:
			fmt.Fprintf(r.out, "Codrax> /clear wipes shared memory (MEMORY.md + turns/) — %d other live codrax instances are also using this directory and will see the wipe on their next read. Confirm? [y/N]: ", peers)
		}
		answer, err := r.in.ReadString('\n')
		if err != nil {
			// EOF or read error → treat as "no" so a stray /clear at
			// end-of-input never silently nukes memory.
			fmt.Fprintln(r.out, "Codrax> clear cancelled")
			break
		}
		answer = strings.TrimSpace(strings.ToLower(answer))
		if answer != "y" && answer != "yes" {
			fmt.Fprintln(r.out, "Codrax> clear cancelled")
			break
		}
		if err := r.store.Clear(); err != nil {
			fmt.Fprintf(r.out, "Codrax> clear failed: %v\n", err)
		} else {
			fmt.Fprintln(r.out, "Codrax> conversation memory cleared.")
		}
	case "/history":
		recent := r.store.Recent()
		idx := r.store.Index()
		if len(recent) == 0 && len(idx) == 0 {
			fmt.Fprintln(r.out, "Codrax> (empty)")
			return false
		}
		if len(idx) > 0 {
			fmt.Fprintln(r.out, "Codrax> compacted index:")
			for _, e := range idx {
				fmt.Fprintf(r.out, "  - [%s] %s — keywords: %s\n", e.ID, e.Topic, strings.Join(e.Keywords, ", "))
			}
		}
		if len(recent) > 0 {
			fmt.Fprintln(r.out, "Codrax> recent turns:")
			for _, t := range recent {
				fmt.Fprintf(r.out, "  - [%s] %s\n", t.Timestamp.Format("15:04:05"), oneLine(t.Request))
			}
		}
	case "/compact":
		if err := r.store.Compact(); err != nil {
			fmt.Fprintf(r.out, "Codrax> compact failed: %v\n", err)
		} else {
			fmt.Fprintf(r.out, "Codrax> compaction done. recent=%d index=%d\n", len(r.store.Recent()), len(r.store.Index()))
		}
	case "/help":
		fmt.Fprintln(r.out, "Codrax> commands: /exit /quit /clear /history /compact /help")
	default:
		fmt.Fprintf(r.out, "Codrax> unknown command %q. Try /help.\n", cmd)
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
