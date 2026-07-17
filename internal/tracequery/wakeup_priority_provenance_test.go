package tracequery

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWakeupPriorityProvenanceCannotMintHardPriorityEvidence(t *testing.T) {
	interner := newStringInterner()
	exact, ok := ParseLine(1, "waker-200 (200) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001", interner)
	if !ok {
		t.Fatal("exact wakeup did not parse")
	}
	inferred, ok := ParseLine(2, "waker-200 (200) [002] .... 5.002000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=inferred_next_sched_slice", interner)
	if !ok {
		t.Fatal("inferred wakeup did not parse")
	}
	unknown, ok := ParseLine(3, "waker-200 (200) [002] .... 5.003000: sched_wakeup: comm=app pid=100 target_cpu=001 codrax_prio_source=unknown", interner)
	if !ok {
		t.Fatal("field-level unknown wakeup edge did not parse")
	}
	if exact.WakeePrioritySource() != "" || eventWakeePriorityForHardUse(exact) != 159 {
		t.Fatalf("original trace-event priority lost authority: %+v", exact)
	}
	if inferred.WakeePrio != 159 || inferred.WakeePrioritySource() != WakeePrioritySourceInferredNextSchedSlice || eventWakeePriorityForHardUse(inferred) != 0 {
		t.Fatalf("schedule-time inference regained wakeup-time authority: %+v", inferred)
	}
	if unknown.WakeePrio != 0 || unknown.WakeePrioritySource() != WakeePrioritySourceUnknown || eventWakeePriorityForHardUse(unknown) != 0 {
		t.Fatalf("unknown priority did not preserve the edge as field-level unknown: %+v", unknown)
	}
	if flavored := applyPriorityFlavor(inferred, TraceFlavorHarmonyHitrace); flavored.WakeePrioClass != "" {
		t.Fatalf("inferred priority was classified as hard Harmony priority: %+v", flavored)
	}
	idx := &Index{Events: []Event{inferred}}
	cache := newChainQueryCache(idx, nil)
	authority := cache.priorityAuthorityFor(Query{TraceFlavor: TraceFlavorHarmonyHitrace})
	point, pointOK := authority.pointForEvent(inferred)
	if !pointOK {
		t.Fatal("inferred wakeup row lost its physical point")
	}
	if got := authority.pointVerdict(100, point, priorityPointAt); got.Caliber != priorityCaliberAdvisoryNearest && got.Caliber != priorityCaliberUnknown {
		t.Fatalf("inferred wakeup priority entered point-authoritative evidence: %+v", got)
	}
	if priority, class := threadPriorityNear(idx, TraceFlavorHarmonyHitrace, ThreadRef{PID: 100, Comm: "app"}, inferred.Ts); priority != 0 || class != "" {
		t.Fatalf("inferred wakeup priority entered nearest exact priority lookup: priority=%d class=%q", priority, class)
	}
}

func TestWakeupPriorityUnknownProvenanceIsFailClosed(t *testing.T) {
	interner := newStringInterner()
	event, ok := ParseLine(1, "waker-200 (200) [002] .... 5.001000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=future_untrusted_source", interner)
	if !ok {
		t.Fatal("wakeup with unknown provenance token should retain its exact edge")
	}
	if event.WakeePrioritySource() != "untrusted" || eventWakeePriorityForHardUse(event) != 0 {
		t.Fatalf("unknown provenance token reopened hard priority authority: %+v", event)
	}
}

func TestWakeupPriorityProvenanceSurvivesChainWithoutMintingInversion(t *testing.T) {
	const prefix = `
        app-100 (100) [001] .... 5.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 5.000100: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
`
	const suffix = `
     worker-200 (200) [002] .... 5.005100: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/2 next_pid=0 next_prio=120
        app-100 (100) [001] .... 5.006000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=159
        app-100 (100) [001] .... 5.007000: sched_switch: prev_comm=app prev_pid=100 prev_prio=159 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
`
	query := Query{PID: 100, TimeStart: 5.0, TimeEnd: 5.007, MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace}
	findEdge := func(t *testing.T, wakeupLine string) WakeupEdge {
		t.Helper()
		idx := buildTraceIndex(t, "priority-provenance.systrace", prefix+wakeupLine+suffix)
		chain := BuildWakeupChain(idx, query)
		for _, edge := range chain.Edges {
			if edge.Waker.PID == 200 && edge.Wakee.PID == 100 {
				return edge
			}
		}
		t.Fatalf("field-level priority provenance suppressed the exact wakeup edge: %+v", chain)
		return WakeupEdge{}
	}

	for name, wakeupLine := range map[string]string{
		"unknown":  "     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 target_cpu=001 codrax_prio_source=unknown",
		"inferred": "     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001 codrax_prio_source=inferred_next_sched_slice",
	} {
		t.Run(name, func(t *testing.T) {
			edge := findEdge(t, wakeupLine)
			if edge.WakeePriority != 0 || edge.WakeePriorityClass != "" || edge.PriorityRelation != "" || edge.PriorityInversionCandidate {
				t.Fatalf("non-exact wakeup priority minted hard chain semantics: %+v", edge)
			}
			if edge.WakeePrioritySource != name && !(name == "inferred" && edge.WakeePrioritySource == WakeePrioritySourceInferredNextSchedSlice) {
				t.Fatalf("priority source was not carried to the wakeup edge: %+v", edge)
			}
		})
	}

	exact := findEdge(t, "     worker-200 (200) [002] .... 5.005000: sched_wakeup: comm=app pid=100 prio=159 target_cpu=001")
	if exact.WakeePriority != 159 || exact.WakeePrioritySource != "" || exact.PriorityRelation != "lower_priority_waker" || !exact.PriorityInversionCandidate {
		t.Fatalf("native exact wakeup priority behavior regressed: %+v", exact)
	}
	payload, err := json.Marshal(exact)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "wakee_priority_source") {
		t.Fatalf("native exact edge changed its JSON compatibility surface: %s", payload)
	}
}
