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

func TestEmitPerfTrace_Execute_AddsHarmonyPrioritySemanticsObservation(t *testing.T) {
	bus := &types.BusContext{
		Mutable:               types.NewMutableState("这是一段 OpenHarmony/鸿蒙 bytrace 文本，分析调度优先级"),
		AttachedHitraceSource: "harmony_hitrace",
		AttachedHitrace: strings.Join([]string{
			"ACCS0-2716 (2519) [000] d..5 168758.662877: sched_waking: comm=Binder:924_3 pid=1200 prio=120 target_cpu=001",
			"HeapTaskDaemon-2532 (2519) [000] d..3 168758.663107: sched_switch: prev_comm=HeapTaskDaemon prev_pid=2532 prev_prio=124 prev_state=R ==> next_comm=rcu_preempt next_pid=7 next_prio=98",
		}, "\n"),
	}
	params, err := json.Marshal(emitPerfTraceParams{
		Meta: emitPerfTraceMeta{
			Source:  "hitrace",
			Summary: "scheduler sample",
		},
		Observations: []emitPerfTraceObservation{{
			Kind:       "line_anchor",
			Subject:    "ACCS0 wakes Binder",
			Summary:    "ACCS0 wakes Binder:924_3 in the sample window",
			Evidence:   "sched_waking",
			LineStart:  1,
			LineEnd:    1,
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
	if bundle == nil || len(bundle.Observations) != 3 {
		t.Fatalf("expected system priority observation plus original observation, got: %+v", bundle)
	}
	got := bundle.Observations[0]
	if got.Kind != "priority_semantics" {
		t.Fatalf("first observation should be priority semantics, got: %+v", got)
	}
	for _, want := range []string{"prio=98/ohos_rt", "prio=120/ohos_rt", "prio=124/ohos_rt"} {
		if !strings.Contains(got.Summary, want) {
			t.Fatalf("priority observation missing %q: %q", want, got.Summary)
		}
	}
	if got.LineStart != 1 || got.LineEnd != 2 || got.Confidence != 1 {
		t.Fatalf("priority observation should carry trace line span/confidence: %+v", got)
	}
	if bundle.Observations[1].Kind != "time_semantics" || !strings.Contains(bundle.Observations[1].Summary, "0.230ms") {
		t.Fatalf("expected system time semantics observation before original observation: %+v", bundle.Observations[1])
	}
	if bundle.Observations[2].Subject != "ACCS0 wakes Binder" {
		t.Fatalf("original observation was not preserved after priority observation: %+v", bundle.Observations)
	}
}

func TestEmitPerfTrace_Execute_NormalizesConflictingHarmonyPriorityObservation(t *testing.T) {
	bus := &types.BusContext{
		Mutable:               types.NewMutableState("鸿蒙 bytrace 调度优先级分析"),
		AttachedHitraceSource: "harmony_hitrace",
		AttachedHitrace: strings.Join([]string{
			"ACCS0-2716 (2519) [000] d..5 168758.662877: sched_waking: comm=Binder:924_3 pid=1200 prio=120 target_cpu=001",
			"HeapTaskDaemon-2532 (2519) [000] d..3 168758.663107: sched_switch: prev_comm=HeapTaskDaemon prev_pid=2532 prev_prio=124 prev_state=R ==> next_comm=rcu_preempt next_pid=7 next_prio=98",
		}, "\n"),
	}
	params, err := json.Marshal(emitPerfTraceParams{
		Meta: emitPerfTraceMeta{Source: "hitrace", Summary: "ACCS0与Binder均为CFS普通优先级prio=120"},
		Observations: []emitPerfTraceObservation{{
			Kind:       "scheduler",
			Subject:    "Priority semantics: HarmonyOS CFS",
			Summary:    "ACCS0(prio=120) and Binder:924_3(prio=120) are CFS baseline nice=0 threads.",
			Evidence:   "prio=120 falls in the CFS range",
			LineStart:  1,
			LineEnd:    2,
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
	if bundle == nil || len(bundle.Observations) < 3 {
		t.Fatalf("perf bundle observations missing: %+v", bundle)
	}
	normalized := bundle.Observations[len(bundle.Observations)-1]
	if normalized.Kind != "priority_semantics_normalized" {
		t.Fatalf("conflicting priority observation was not normalized: %+v", bundle.Observations)
	}
	if normalized.Subject != "HarmonyOS priority semantics" {
		t.Fatalf("normalized subject retained model-authored class label: %q", normalized.Subject)
	}
	if strings.Contains(strings.ToLower(normalized.Summary), "nice=0") || strings.Contains(strings.ToLower(normalized.Summary), "cfs baseline") {
		t.Fatalf("normalized summary retained conflicting prose: %q", normalized.Summary)
	}
	if strings.Contains(strings.ToLower(bundle.Meta.Summary), "cfs baseline") || strings.Contains(strings.ToLower(bundle.Meta.Summary), "120位于cfs") {
		t.Fatalf("meta summary retained conflicting priority prose: %q", bundle.Meta.Summary)
	}
	for _, want := range []string{"prio=98/ohos_rt", "prio=120/ohos_rt", "prio=124/ohos_rt", "1-40=CFS", "41-139=RT"} {
		if !strings.Contains(normalized.Summary, want) {
			t.Fatalf("normalized summary missing %q: %q", want, normalized.Summary)
		}
	}
	if !strings.Contains(bundle.Meta.Summary, "41-139=RT") {
		t.Fatalf("meta summary was not normalized to Harmony priority rule: %q", bundle.Meta.Summary)
	}
}
