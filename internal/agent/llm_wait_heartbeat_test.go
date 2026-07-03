package agent

import (
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/llm"
	"github.com/hanchaoqun/codrax/internal/render"
	"github.com/hanchaoqun/codrax/internal/types"
)

// TestEmitLLMWaitHeartbeat_CarriesTypedTickAndElapsed pins the render
// event the LLM-request watchdog emits per slow tick. The renderer's
// power-of-two durable-line gate keys on WaitTick, so the 1-based tick
// counter and the identifying fields (agent / stage / iteration /
// model) must all survive the projection — losing any of them either
// breaks the rate limit or renders an unattributable "waiting" line.
// 2026-07-03 review WF3 extension: the parallel attribution trio
// (ParallelGroupID / ParallelUnitID / DispatchKind) must follow the
// same ctx fill pattern as the other per-request emits — without it a
// fan-out worker's heartbeat renders with no unit label, and two
// units waiting on the same model at the same elapsed second collapse
// into one line under the renderer's byte-equality dedupe.
func TestEmitLLMWaitHeartbeat_CarriesTypedTickAndElapsed(t *testing.T) {
	var got []render.Event
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{Emit: func(ev render.Event) { got = append(got, ev) }},
	}
	ctx := &types.AgentContext{
		AgentName:           types.AgentExplorer,
		Stage:               types.StageExplore,
		ParallelGroupID:     "grp-7",
		ExploreDispatchKey:  "unit-2",
		ExploreDispatchKind: types.NodeProbe,
	}

	base.emitLLMWaitHeartbeat(ctx, 12, 5, llm.RequestTelemetry{ModelID: "m-slow"}, 150*time.Second)

	if len(got) != 1 {
		t.Fatalf("expected exactly 1 event, got %d", len(got))
	}
	ev := got[0]
	if ev.Kind != render.EventAgentLLMWaiting ||
		ev.Agent != types.AgentExplorer ||
		ev.Stage != types.StageExplore ||
		ev.Iteration != 12 ||
		ev.WaitTick != 5 ||
		ev.WaitElapsed != 150*time.Second ||
		ev.ModelID != "m-slow" {
		t.Fatalf("unexpected heartbeat event: %+v", ev)
	}
	if ev.ParallelGroupID != "grp-7" ||
		ev.ParallelUnitID != "unit-2" ||
		ev.DispatchKind != string(types.NodeProbe) {
		t.Fatalf("parallel attribution fields lost: %+v", ev)
	}
	if ev.Timestamp.IsZero() {
		t.Fatalf("heartbeat event must carry a timestamp")
	}
}

// TestEmitLLMWaitHeartbeat_NilSafe pins the guard rails: agents built
// without an Emit dependency (unit-test fixtures, headless embedders)
// and watchdogs started with a nil AgentContext must not panic — the
// heartbeat is best-effort UI telemetry, never load-bearing.
func TestEmitLLMWaitHeartbeat_NilSafe(t *testing.T) {
	noEmit := &BaseAgent{name: types.AgentExplorer, deps: &Dependencies{}}
	noEmit.emitLLMWaitHeartbeat(nil, 0, 1, llm.RequestTelemetry{}, time.Second) // must not panic

	var got []render.Event
	base := &BaseAgent{
		name: types.AgentExplorer,
		deps: &Dependencies{Emit: func(ev render.Event) { got = append(got, ev) }},
	}
	base.emitLLMWaitHeartbeat(nil, 3, 2, llm.RequestTelemetry{}, time.Minute)
	if len(got) != 1 {
		t.Fatalf("nil ctx must still emit, got %d events", len(got))
	}
	if got[0].Stage != "" {
		t.Fatalf("nil ctx must project an empty stage, got %q", got[0].Stage)
	}
	if got[0].ParallelGroupID != "" || got[0].ParallelUnitID != "" || got[0].DispatchKind != "" {
		t.Fatalf("nil ctx must project empty parallel attribution, got %+v", got[0])
	}
	if got[0].WaitTick != 2 || got[0].Iteration != 3 {
		t.Fatalf("tick/iteration lost on nil-ctx path: %+v", got[0])
	}
}
