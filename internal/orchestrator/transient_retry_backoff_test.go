package orchestrator

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// transientBackoffCapture installs the transientRetrySleepHook and
// records every delay the L4 lanes request instead of really sleeping.
type transientBackoffCapture struct {
	mu     sync.Mutex
	delays []time.Duration
}

func installTransientBackoffCapture(t *testing.T) *transientBackoffCapture {
	t.Helper()
	c := &transientBackoffCapture{}
	transientRetrySleepHook = func(d time.Duration) {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.delays = append(c.delays, d)
	}
	t.Cleanup(func() { transientRetrySleepHook = nil })
	return c
}

func (c *transientBackoffCapture) snapshot() []time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]time.Duration, len(c.delays))
	copy(out, c.delays)
	return out
}

func (c *transientBackoffCapture) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.delays = nil
}

func newTransientBackoffTestOrchestrator(events *[]render.Event) (*Orchestrator, *graphState) {
	mut := types.NewMutableState("transient backoff test")
	bus := &types.BusContext{Mode: types.ModeRead, Mutable: mut}
	o := &Orchestrator{
		busCtx:               bus,
		transientRetryBudget: 3,
		emit: func(ev render.Event) {
			*events = append(*events, ev)
		},
	}
	state := newGraphState(types.TaskGraph{Nodes: []types.TaskNode{{ID: "finalize"}}})
	return o, state
}

// TestRetryReadStageDispatchError_SharedBackoffSchedule pins §29.92.1
// 件3: the L4 transient requeue lane must wait the shared
// llm.NextRetryDelay backoff before re-dispatching (mutation guard: a
// requeue that no longer routes through sleepTransientRetryBackoff
// leaves the capture hook silent and this test red), and the
// user-facing retry notice must carry the wait duration.
func TestRetryReadStageDispatchError_SharedBackoffSchedule(t *testing.T) {
	capture := installTransientBackoffCapture(t)

	t.Run("stream_shape_waits_jittered_backoff_and_notice_carries_duration", func(t *testing.T) {
		capture.reset()
		var events []render.Event
		o, state := newTransientBackoffTestOrchestrator(&events)
		fin := &types.TaskNode{ID: "finalize"}

		if !o.retryReadStageDispatchError(state, types.StageFinalize, nil, fin, io.EOF) {
			t.Fatalf("stream-level EOF within budget must requeue")
		}
		delays := capture.snapshot()
		if len(delays) != 1 {
			t.Fatalf("requeue must wait the shared backoff exactly once, hook saw %v", delays)
		}
		if delays[0] <= 0 || delays[0] > time.Second {
			t.Fatalf("attempt-0 stream backoff must be full-jitter in (0, 1s], got %v", delays[0])
		}
		if len(events) != 1 || events[0].NoticeKind != render.NoticeRetry {
			t.Fatalf("expected exactly one retry notice, got %+v", events)
		}
		wantClause := transportRetryDelayClause(delays[0], false)
		if !strings.Contains(events[0].Reasoning, wantClause) {
			t.Fatalf("retry notice must carry the backoff duration clause %q, got %q", wantClause, events[0].Reasoning)
		}
	})

	t.Run("deadline_class_form_a_requeues_immediately_without_extra_sleep", func(t *testing.T) {
		capture.reset()
		var events []render.Event
		o, state := newTransientBackoffTestOrchestrator(&events)
		fin := &types.TaskNode{ID: "finalize"}
		// 形A: the request already burned its full configured window
		// (analyzer terminal emit-only ctx kill surfaces exactly this
		// shape). Extra sleep on top only extends dead air.
		err := fmt.Errorf("analyze dispatch: %w", context.DeadlineExceeded)

		if !o.retryReadStageDispatchError(state, types.StageFinalize, nil, fin, err) {
			t.Fatalf("deadline-class error within budget must requeue")
		}
		if delays := capture.snapshot(); len(delays) != 0 {
			t.Fatalf("deadline-class 形A must keep zero extra backoff, hook saw %v", delays)
		}
		if len(events) != 1 || !strings.Contains(events[0].Reasoning, "immediately") {
			t.Fatalf("deadline-class retry notice must read immediate retry, got %+v", events)
		}
	})

	t.Run("standalone_extract_lane_shares_the_same_backoff", func(t *testing.T) {
		capture.reset()
		var events []render.Event
		o, state := newTransientBackoffTestOrchestrator(&events)

		if !o.retryReadStandaloneDispatchError(state, types.StageExtract, io.EOF) {
			t.Fatalf("standalone stream-level EOF within budget must retry")
		}
		delays := capture.snapshot()
		if len(delays) != 1 || delays[0] <= 0 || delays[0] > time.Second {
			t.Fatalf("standalone lane must wait the shared attempt-0 jitter in (0, 1s], hook saw %v", delays)
		}
		if len(events) != 1 || !strings.Contains(events[0].Reasoning, transportRetryDelayClause(delays[0], false)) {
			t.Fatalf("standalone retry notice must carry the backoff duration, got %+v", events)
		}
	})

	t.Run("attempt_index_grows_the_jitter_ceiling", func(t *testing.T) {
		capture.reset()
		var events []render.Event
		o, state := newTransientBackoffTestOrchestrator(&events)
		fin := &types.TaskNode{ID: "finalize"}

		for attempt := 0; attempt < 3; attempt++ {
			if !o.retryReadStageDispatchError(state, types.StageFinalize, nil, fin, io.EOF) {
				t.Fatalf("attempt %d within budget must requeue", attempt)
			}
		}
		delays := capture.snapshot()
		if len(delays) != 3 {
			t.Fatalf("three requeues must wait three times, hook saw %v", delays)
		}
		for attempt, d := range delays {
			ceil := time.Second << uint(attempt)
			if d <= 0 || d > ceil {
				t.Fatalf("attempt %d backoff = %v, want in (0, %v]", attempt, d, ceil)
			}
		}
	})
}

// TestSleepTransientRetryBackoff_CancellationAware pins the wait
// implementation itself: zero/negative delays return without touching
// the hook or a timer, and a canceled run context unblocks the real
// timer wait immediately instead of stranding the scheduler.
func TestSleepTransientRetryBackoff_CancellationAware(t *testing.T) {
	if transientRetrySleepHook != nil {
		t.Fatalf("test requires the real timer path; hook must be nil")
	}
	mut := types.NewMutableState("cancel-aware backoff")
	ctx, cancel := context.WithCancel(context.Background())
	bus := &types.BusContext{Mode: types.ModeRead, Mutable: mut, Ctx: ctx}
	o := &Orchestrator{busCtx: bus, emit: func(render.Event) {}}

	cancel()
	start := time.Now()
	o.sleepTransientRetryBackoff(10 * time.Second)
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("canceled context must unblock the backoff wait immediately, took %v", elapsed)
	}

	start = time.Now()
	o.sleepTransientRetryBackoff(0)
	o.sleepTransientRetryBackoff(-time.Second)
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("non-positive delays must return immediately, took %v", elapsed)
	}
}
