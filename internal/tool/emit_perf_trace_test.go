package tool

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestEmitPerfTrace_Execute_TagsRuntimeArtifactOrigin(t *testing.T) {
	bus := &types.BusContext{
		Mutable:         types.NewMutableState("test"),
		AttachedHitrace: "Frame 1 duration=24.5ms",
	}
	params, err := json.Marshal(emitPerfTraceParams{
		Meta: emitPerfTraceMeta{
			Source:     "hitrace",
			DurationMs: 24.5,
			Signals:    []string{"jank"},
			Summary:    "one slow frame",
		},
		Frames: []emitPerfTraceFrame{{
			FrameNo:    1,
			DurationMs: 24.5,
			Janky:      true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	tool := &EmitPerfTrace{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false, summary=%s", res.Summary)
	}
	if !strings.Contains(res.Summary, "evidence_origin=runtime_artifact") {
		t.Fatalf("summary should tag runtime artifact origin: %q", res.Summary)
	}
	if bundle := bus.Mutable.PerfTrace(); bundle == nil || len(bundle.Frames) != 1 {
		t.Fatalf("perf bundle missing frame: %+v", bundle)
	}
}

func TestEmitPerfTrace_Execute_AcceptsTraceObservationsOnly(t *testing.T) {
	bus := &types.BusContext{
		Mutable:         types.NewMutableState("trace line lookup"),
		AttachedHitrace: "1│ tracing_mark_write: B|1000|H:GC:Collect\n2│ tracing_mark_write: E|1000\n",
	}
	params, err := json.Marshal(emitPerfTraceParams{
		Meta: emitPerfTraceMeta{
			Source:  "hitrace",
			Summary: "GC span line/duration lookup",
		},
		Observations: []emitPerfTraceObservation{{
			Kind:       "line_anchor",
			Subject:    "GC span start",
			Summary:    "GC:Collect begins on attached trace line 1 and lasts 8ms; no GC span exceeds 50ms",
			Evidence:   "B|1000|H:GC:Collect",
			LineStart:  1,
			LineEnd:    2,
			DurationMs: 8,
			Tags:       []string{"H:GC:Collect"},
			Confidence: 1,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}

	tool := &EmitPerfTrace{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("success=false, summary=%s", res.Summary)
	}
	bundle := bus.Mutable.PerfTrace()
	if bundle == nil || len(bundle.Observations) != 1 {
		t.Fatalf("perf observation bundle missing: %+v", bundle)
	}
	if !bundle.IsExternalSource() {
		t.Fatalf("observation-only trace should be external runtime evidence: %+v", bundle)
	}
	if !strings.Contains(res.Summary, "observations=1") {
		t.Fatalf("summary should report observations count: %q", res.Summary)
	}
}

func TestEmitPerfTrace_Execute_StructuredPayloadCompatRepairsStringWrappedArrays(t *testing.T) {
	bus := &types.BusContext{
		Mutable:         types.NewMutableState("trace json compatibility"),
		AttachedHitrace: "1│ B|1000|H:RenderService:DoFrame\n2│ E|1000\n",
	}
	params := json.RawMessage(`{
		"meta": {
			"source": "hitrace",
			"signals": "[\"jank\"]",
			"summary": "one frame observation"
		},
		"observations": "[{\"kind\":\"line_anchor\",\"subject\":\"DoFrame\",\"summary\":\"DoFrame span is observed on trace lines 1-2\",\"evidence\":\"H:RenderService:DoFrame\",\"line_start\":\"1\",\"line_end\":\"2\",\"tags\":\"[\\\"render\\\"]\"}]"
	}`)

	tool := &EmitPerfTrace{}
	res, err := tool.Execute(bus, params)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Success {
		t.Fatalf("expected schema-compatible payload to be accepted, got: %s", res.Summary)
	}
	bundle := bus.Mutable.PerfTrace()
	if bundle == nil || len(bundle.Observations) != 1 {
		t.Fatalf("perf observation bundle missing: %+v", bundle)
	}
	got := bundle.Observations[0]
	if got.LineStart != 1 || got.LineEnd != 2 || got.Subject != "DoFrame" {
		t.Fatalf("observation was not preserved after compat repair: %+v", got)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "render" {
		t.Fatalf("nested string-wrapped tags were not repaired: %+v", got.Tags)
	}
	if len(bundle.Meta.Signals) == 0 || bundle.Meta.Signals[0] != "jank" {
		t.Fatalf("meta signals were not repaired: %+v", bundle.Meta.Signals)
	}
}
