package orchestrator

import (
	"strconv"
	"strings"
	"sync"

	"github.com/hanchaoqun/codrax/internal/logging"
	"github.com/hanchaoqun/codrax/internal/types"
	"github.com/hanchaoqun/codrax/internal/userhint"
)

// TTY-3 steering intake (design ledger
// docs/design/tty_single_stdin_owner_20260729.md §3 T-3): lines typed
// during a TTY Run can steer the CURRENT run instead of waiting for
// post-Run replay. The REPL's run-input window pushes notes through
// PushSteeringNote (thread-safe: the window goroutine writes, the
// scheduler goroutine drains); each read-lane explore window boundary
// consumes pending notes — @path pins become forced reads the explorer
// executes before the LLM sees anything, free text rides the existing
// window-hint lane. Notes accepted but never consumed (run ended
// first, write-mode runs) are returned via TakeUnconsumedSteeringNotes
// so the REPL replays them as follow-up turns: steered or replayed,
// never lost, never run twice.
type steeringIntake struct {
	mu    sync.Mutex
	open  bool
	notes []string
}

func (s *steeringIntake) openIntake() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = true
	s.notes = nil
}

func (s *steeringIntake) closeIntake() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.open = false
}

func (s *steeringIntake) push(note string) bool {
	note = strings.TrimSpace(note)
	if note == "" {
		return false
	}
	// REGFIX-B #10/#15 (sweep): a line the operator typed as a COMMAND
	// is not steering prose. Swallowing "/cancel" into the explorer
	// prompt meant the command never executed while its text polluted
	// the LLM's instructions. Refuse it here so the window queues it
	// for post-run replay through the normal dispatch funnel.
	// SWEEPFIX S4: the refusal mirrors the funnel's own PRECISE
	// predicates — the shell-bang prefix and the alias registry — not
	// a noisy "starts with /" heuristic, which mis-refused routine
	// log-triage prose like "/var/log/app.log 里 10:32 有 panic" and
	// replayed it as a surprise full LLM turn. A path-leading note is
	// prose to the funnel, so it is prose to the intake too. (Prompt
	// TEMPLATE commands are refused one layer up, in the REPL's
	// trySteer wrapper, where the template registry lives.)
	if strings.HasPrefix(note, "!") || types.NormalizeREPLCommandAlias(note) != "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.open {
		return false
	}
	s.notes = append(s.notes, note)
	return true
}

func (s *steeringIntake) drain() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	notes := s.notes
	s.notes = nil
	return notes
}

// PushSteeringNote is the runner-facing intake (REPL run-input window
// → orchestrator). Returns false when no run is accepting steering —
// the caller queues the line for post-Run replay instead.
func (o *Orchestrator) PushSteeringNote(note string) bool {
	if o == nil {
		return false
	}
	return o.steering.push(note)
}

// TakeUnconsumedSteeringNotes returns (and clears) notes that were
// accepted but never reached an explore boundary. The REPL appends
// them to the follow-up replay queue.
func (o *Orchestrator) TakeUnconsumedSteeringNotes() []string {
	if o == nil {
		return nil
	}
	return o.steering.drain()
}

// applySteeringNotesToExploreWindow consumes pending notes at an
// explore window boundary: fs-validated @path pins enqueue as forced
// reads (executed before the explorer LLM sees anything), and the
// note text is appended verbatim to the window hint as data. Returns
// the number of notes consumed.
func (o *Orchestrator) applySteeringNotesToExploreWindow() int {
	consumed, _ := o.takeSteeringNotesForExploreWindow()
	return consumed
}

// takeSteeringNotesForExploreWindow consumes pending notes and returns
// both the count and the composed hint text. REGFIX-B #9 (sweep, high):
// the parallel explore lane composes each worker's hint BEFORE this
// point and each worker then OVERWRITES its bus clone's RetryHint with
// that pre-drain value, so writing only into the parent bus silently
// destroyed the operator's text — echoed as "injected now", reaching no
// prompt, and unreplayable because the intake was already drained. The
// caller must therefore also merge the returned text into the parallel
// per-worker hints.
func (o *Orchestrator) takeSteeringNotesForExploreWindow() (int, string) {
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return 0, ""
	}
	notes := o.steering.drain()
	if len(notes) == 0 {
		return 0, ""
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	// REGFIX-B #11 (sweep): request-time @pins honour the explicit
	// source-exclusion boundary (PIB-5c clause order); mid-run pins must
	// too. An operator who said "don't read source" keeps that boundary
	// even when a stray @token rides a steering note — the note text
	// still reaches the prompt, only the forced read is withheld.
	pinsAllowed := true
	if o.busCtx.AnalysisIR != nil {
		rm := o.busCtx.AnalysisIR.RequestModel
		if rm.ExternalObservationPolicy != nil && rm.ExternalObservationPolicy.ExcludesCurrentSource() {
			pinsAllowed = false
		}
	}
	var hints []string
	for _, note := range notes {
		pins := []string{}
		if pinsAllowed {
			pins = userhint.ExtractPinnedFiles(o.busCtx.RepoRoot, note)
		}
		for _, pin := range pins {
			closure.AddPendingRead(types.PendingRead{
				File:      pin,
				Rationale: "operator steering pin (typed mid-run)",
				Origin:    "explore.user_steering_pin",
				Stage:     string(types.StageExplore),
			})
		}
		// SWEEPFIX S3: the pins clause must tell the truth. Under a
		// source-exclusion boundary (pinsAllowed=false) the forced read
		// is deliberately withheld — claiming the pins "are already
		// queued for reading" was an affirmatively false prompt
		// statement (the dishonest-prompt class the proof-lane red line
		// exists for).
		suffix := "."
		if pinsAllowed && len(pins) > 0 {
			suffix = "; any @file pins in it are already queued for reading."
		} else if !pinsAllowed {
			suffix = ". @file references in it were NOT read (this run excludes current-source reading); treat them as plain text."
		}
		hints = append(hints, "Operator steering note (typed mid-run, verbatim): "+strconv.Quote(note)+
			". Address it within the current exploration"+suffix)
	}
	steeringHint := strings.Join(hints, "\n")
	if existing := strings.TrimSpace(o.busCtx.TaskState.RetryHint); existing != "" {
		o.busCtx.TaskState.RetryHint = existing + "\n\n" + steeringHint
	} else {
		o.busCtx.TaskState.RetryHint = steeringHint
		if o.busCtx.TaskState.RetryHintStage == "" {
			o.busCtx.TaskState.RetryHintStage = types.StageExplore
		}
	}
	logging.Info("[orchestrator] steering: consumed %d mid-run note(s) at explore window boundary", len(notes))
	return len(notes), steeringHint
}

// appendSteeringHintToWindowHint merges the steering text into a
// composed window hint without disturbing the existing body (REGFIX-B
// #9). Empty inputs pass through.
func appendSteeringHintToWindowHint(hint, steering string) string {
	steering = strings.TrimSpace(steering)
	if steering == "" {
		return hint
	}
	if strings.TrimSpace(hint) == "" {
		return steering
	}
	return hint + "\n\n" + steering
}
