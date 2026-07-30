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
	if o == nil || o.busCtx == nil || o.busCtx.Mutable == nil {
		return 0
	}
	notes := o.steering.drain()
	if len(notes) == 0 {
		return 0
	}
	closure := o.busCtx.Mutable.EvidenceClosure()
	var hints []string
	for _, note := range notes {
		for _, pin := range userhint.ExtractPinnedFiles(o.busCtx.RepoRoot, note) {
			closure.AddPendingRead(types.PendingRead{
				File:      pin,
				Rationale: "operator steering pin (typed mid-run)",
				Origin:    "explore.user_steering_pin",
				Stage:     string(types.StageExplore),
			})
		}
		hints = append(hints, "Operator steering note (typed mid-run, verbatim): "+strconv.Quote(note)+
			". Address it within the current exploration; any @file pins in it are already queued for reading.")
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
	return len(notes)
}
