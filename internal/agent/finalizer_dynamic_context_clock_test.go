package agent

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tool"
	"github.com/hanchaoqun/codrax/internal/types"
)

// The dynamic-context budget of a finalizer dispatch is read through the
// dispatch's own clock. Production leaves the clock unset and reads the wall
// clock; a caller that must reproduce a recorded dispatch supplies the
// recorded position instead, so host load never decides which units a
// reproduced request carries.

func dynamicContextClockRequest(t *testing.T, clock DynamicContextClock) (dynamic string, calls int) {
	t.Helper()
	ctx, path := hmc320BusinessFinalizer(t)
	hmc320BusinessQuery(t, ctx, path, map[string]any{"pid": 700})
	capture := &hmc326BudgetCapture{traceTeachingCaptureLLM: traceTeachingCaptureLLM{stop: errors.New("captured first request")}, window: 2000000}
	registry := tool.NewRegistry()
	registry.Register(&tool.EmitAnswerDocument{})
	registry.Register(&tool.EmitAnswerDocumentPatch{})
	finalizer := NewFinalizerAgent(&Dependencies{LLM: capture, Tools: registry, MaxIterations: 1,
		AgentSettings:                types.AgentSettings{FinalizerRequestBytePolicy: types.FinalizerRequestByteEnforce},
		FinalizerDynamicContextClock: clock})
	if _, err := finalizer.Execute(ctx, traceTeachingSkill(t, "answer-document-skill")); capture.calls != 1 || !errors.Is(err, capture.stop) {
		t.Fatalf("first request was not captured: calls=%d err=%v", capture.calls, err)
	}
	return hmc326DynamicRequestMessage(t, capture), capture.calls
}

const dynamicContextBudgetNote = "## Dynamic Prompt Budget\n"

func TestFinalizerDynamicContextClockDecidesWhichUnitsAreBuilt(t *testing.T) {
	var seen []DynamicContextCheckpoint
	complete, _ := dynamicContextClockRequest(t, func(at DynamicContextCheckpoint) time.Duration {
		seen = append(seen, at)
		return 0
	})
	if strings.Contains(complete, dynamicContextBudgetNote) {
		t.Fatal("a clock that never passes the budget still omitted a unit")
	}
	// Both passes reach the ledger: the first pass defers it, the second
	// renders it. The second-pass checkpoint is the only one a deferred group
	// has, so it must be distinguishable from the first-pass one.
	first, second := -1, -1
	for i, at := range seen {
		if at.Unit != "observation_ledger" {
			continue
		}
		if at.Deferred && second < 0 {
			second = i
		}
		if !at.Deferred && first < 0 {
			first = i
		}
	}
	if first < 0 || second < first {
		t.Fatalf("checkpoints do not name the ledger in both passes in build order: first=%d second=%d of %d", first, second, len(seen))
	}
	for _, at := range seen[second:] {
		if !at.Deferred {
			t.Fatalf("a first-pass checkpoint %q followed the deferred pass", at.Unit)
		}
	}

	// The budget passes when the deferred pass reaches the ledger: every
	// first-pass unit is still built, the ledger is not, and the request says so.
	expired := false
	withoutLedger, _ := dynamicContextClockRequest(t, func(at DynamicContextCheckpoint) time.Duration {
		if at.Unit == "observation_ledger" && at.Deferred {
			expired = true
		}
		if expired {
			return answerDocDynamicSoftBudget + time.Nanosecond
		}
		return 0
	})
	if strings.Count(withoutLedger, dynamicContextBudgetNote) != 1 || len(withoutLedger) >= len(complete) {
		t.Fatalf("the deferred ledger was not omitted at its own checkpoint: bytes=%d complete=%d notes=%d",
			len(withoutLedger), len(complete), strings.Count(withoutLedger, dynamicContextBudgetNote))
	}
	tail := complete[len(complete)-200:]
	if !strings.HasSuffix(withoutLedger, tail) {
		t.Fatal("first-pass units after the ledger's position were lost with it")
	}

	// The same unit name in the first pass is a different position: passing
	// the budget there stops the first pass, so everything after it is absent.
	expired = false
	stoppedEarly, _ := dynamicContextClockRequest(t, func(at DynamicContextCheckpoint) time.Duration {
		if at.Unit == "observation_ledger" && !at.Deferred {
			expired = true
		}
		if expired {
			return answerDocDynamicSoftBudget + time.Nanosecond
		}
		return 0
	})
	if strings.HasSuffix(stoppedEarly, tail) || len(stoppedEarly) >= len(withoutLedger) {
		t.Fatalf("a first-pass stop at the ledger kept later units: bytes=%d deferred-stop=%d", len(stoppedEarly), len(withoutLedger))
	}
}

func TestFinalizerDynamicContextBudgetIsTwentySecondsOfWallClockByDefault(t *testing.T) {
	if answerDocDynamicSoftBudget != 20*time.Second {
		t.Fatalf("dynamic-context budget = %s, want 20s", answerDocDynamicSoftBudget)
	}
	// No injected clock: the wall clock since the dispatch began building.
	for _, c := range []struct {
		age  time.Duration
		stop bool
	}{{19 * time.Second, false}, {21 * time.Second, true}} {
		trace := newAnswerDocDynamicTrace(&types.AgentContext{})
		trace.started = time.Now().Add(-c.age)
		if got := trace.shouldStop(nil, "unit"); got != c.stop {
			t.Errorf("wall clock, building for %s: stop=%v want %v", c.age, got, c.stop)
		}
	}
	// An injected clock is the only authority of its dispatch, in both
	// directions, and exactly at the budget nothing is omitted yet.
	for _, c := range []struct {
		age, reported time.Duration
		stop          bool
	}{
		{time.Hour, 0, false},
		{time.Hour, 20 * time.Second, false},
		{0, 20*time.Second + time.Nanosecond, true},
	} {
		trace := newAnswerDocDynamicTrace(&types.AgentContext{})
		trace.started = time.Now().Add(-c.age)
		trace.clock = func(DynamicContextCheckpoint) time.Duration { return c.reported }
		if got := trace.shouldStop(nil, "unit"); got != c.stop {
			t.Errorf("injected clock reports %s after %s of wall clock: stop=%v want %v", c.reported, c.age, got, c.stop)
		}
	}
}
