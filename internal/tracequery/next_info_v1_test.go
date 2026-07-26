package tracequery

import (
	"encoding/json"
	"strings"
	"testing"
)

// next_info_v1_test.go — NEXTINFO-V1 (unfrozen 2026-07-26, 硬伤A value
// channel): the packed next_info field 3 is ices_boost (前台加速, foreground
// acceleration) per the customer semantics doc — the legacy "restricted"
// reading inverted an ACCELERATION into a RESTRICTION. These pins retire
// every restriction-inference consumer: a boosted thread must never mint a
// cpu_affinity_or_cpuset restriction claim, the lying restricted=%t display
// token retires (ices_boost=%t is the truthful face), and the JSON surface
// drops next_info_restricted. Admission/decisive census arms retarget onto
// the SEMANTIC boost flag (Known-gated: out-of-doc raw values prove nothing).

func TestNextInfoV1_RenderPolicyRetiresRestrictedToken(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(1, `        app-20   (   20) [001] .... 1.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53 next_info=e,0,1,1,0,1 cg=top-app`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	got := renderNextInfoPolicy(ev)
	if strings.Contains(got, "restricted=") {
		t.Fatalf("restricted token must retire from the policy face (V1): %q", got)
	}
	if !strings.Contains(got, "ices_boost=true") {
		t.Fatalf("boost=1 must speak the truthful ices_boost token: %q", got)
	}
}

func TestNextInfoV1_OutOfDocBoostClaimsNothing(t *testing.T) {
	// boost raw=3 is outside the documented {0,1} set: BoostKnown=false, and
	// under V1 the face makes NO claim at all (the legacy fill used to print
	// restricted=true for exactly this shape).
	intern := newStringInterner()
	ev, ok := ParseLine(1, `        app-22   (   22) [001] .... 1.140000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app3 next_pid=22 next_prio=53 next_info=f,10,1,3,0 cg=top-app`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	got := renderNextInfoPolicy(ev)
	if strings.Contains(got, "restricted=") || strings.Contains(got, "ices_boost=") {
		t.Fatalf("out-of-doc boost value must claim nothing on the boost lane: %q", got)
	}
}

func TestNextInfoV1_ConstraintAdmissionReadsSemanticBoost(t *testing.T) {
	// A semantically-boosted sched_switch (no affinity/allowed evidence) IS
	// policy evidence for the constraint census — via the typed boost flag,
	// not the retired restricted fill.
	boosted := Event{Type: EventSchedSwitch, NextInfoBoostKnown: true, NextInfoBoost: true}
	if !isCPUConstraintEvidence(boosted) {
		t.Fatal("semantic boost must admit the event as constraint-census evidence")
	}
	// Known boost=false (and the unknown lane) admit nothing by themselves.
	unboosted := Event{Type: EventSchedSwitch, NextInfoBoostKnown: true, NextInfoBoost: false}
	if isCPUConstraintEvidence(unboosted) {
		t.Fatal("boost=false alone must not admit")
	}
	unknown := Event{Type: EventSchedSwitch}
	if isCPUConstraintEvidence(unknown) {
		t.Fatal("no next_info evidence must not admit")
	}
	// The Known gate itself: a raw Boost bit without BoostKnown (out-of-doc
	// or malformed lane) proves nothing and must not admit.
	unknownBoost := Event{Type: EventSchedSwitch, NextInfoBoost: true}
	if isCPUConstraintEvidence(unknownBoost) {
		t.Fatal("Boost without BoostKnown must not admit (Known gate)")
	}
}

func TestNextInfoV1_RestrictsExecutionNeedsRealRestriction(t *testing.T) {
	cpus := []CPUStats{{CPU: 0}, {CPU: 1}}
	// Boost is an acceleration — never restriction evidence.
	boostOnly := CPUConstraintSummary{Policy: "next_info affinity=f ices_boost=true"}
	if cpuConstraintRestrictsExecution(boostOnly, cpus) {
		t.Fatal("a boost-only policy must not count as execution restriction")
	}
	// The retired legacy token must stay dead even if the literal appears
	// (arm-resurrection guard: no producer mints it, and no consumer may
	// sniff it back into a restriction claim).
	legacyLiteral := CPUConstraintSummary{Policy: "next_info affinity=f restricted=true"}
	if cpuConstraintRestrictsExecution(legacyLiteral, cpus) {
		t.Fatal("the retired restricted=true sniff must not resurrect a restriction claim")
	}
	// Real restriction evidence keeps its arms — with BINDING provenance.
	withSet := CPUConstraintSummary{CPUSet: "background", CPUSetIsBinding: true}
	if !cpuConstraintRestrictsExecution(withSet, cpus) {
		t.Fatal("a cpuset binding is real restriction evidence")
	}
	// Dual-review P2: the sched_switch cg= suffix stamps a cgroup NAME into
	// CPUSet as display context — WITHOUT binding provenance it must never
	// claim restriction (an unrestricted foreground thread with cg=top-app
	// used to mint a 受限 seat through this arm).
	cgroupProxy := CPUConstraintSummary{CPUSet: "top-app"}
	if cpuConstraintRestrictsExecution(cgroupProxy, cpus) {
		t.Fatal("a cgroup-name proxy CPUSet must not claim restriction")
	}
}

func TestNextInfoV1_RankConfidenceNeedsBindingProvenance(t *testing.T) {
	// The 0.72 bump reads the SAME precise signal as the restriction gate:
	// binding provenance. The cgroup proxy and the retired boost token stay
	// at the 0.64 base.
	if got := cpuConstraintRankConfidence(CPUConstraintSummary{CPUSet: "top-app"}); got != 0.64 {
		t.Fatalf("cgroup-proxy CPUSet must not inflate confidence: %v", got)
	}
	if got := cpuConstraintRankConfidence(CPUConstraintSummary{Policy: "next_info affinity=f ices_boost=true"}); got != 0.64 {
		t.Fatalf("boost policy must not inflate confidence: %v", got)
	}
	if got := cpuConstraintRankConfidence(CPUConstraintSummary{CPUSet: "background", CPUSetIsBinding: true}); got != 0.72 {
		t.Fatalf("a real binding keeps the 0.72 arm: %v", got)
	}
}

func TestNextInfoV1_CensusProvenanceAndDecisiveBoost(t *testing.T) {
	// One thread, three sched_switch lines: cg= proxy fills CPUSet WITHOUT
	// binding provenance; the boost flag's FIRST appearance (line 3) is
	// decisive and advances the seat's line span; a later real
	// sched_setaffinity binding upgrades provenance and overrides the proxy.
	idx := buildTraceIndex(t, "nextinfo_v1_census.systrace", `
       idle/0-0   (    0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=f,4,1,0,0 cg=top-app
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
       idle/0-0   (    0) [000] .... 1.020000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=f,4,1,1,0 cg=top-app
        app-100   (  100) [000] .... 1.030000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.030, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	var seat *CPUConstraintSummary
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			seat = &stats.CPUConstraints[i]
			break
		}
	}
	if seat == nil {
		t.Fatalf("expected a constraint seat for app-100: %+v", stats.CPUConstraints)
	}
	if seat.CPUSet != "top-app" || seat.CPUSetIsBinding {
		t.Fatalf("cg= proxy must fill CPUSet WITHOUT binding provenance: %+v", seat)
	}
	if !strings.Contains(seat.Policy, "ices_boost=true") {
		t.Fatalf("the boost flag's first appearance must reach the policy face: %+v", seat)
	}
	if seat.LineEnd < 3 {
		t.Fatalf("boost first appearance (line 3) is decisive and must advance the seat span: %+v", seat)
	}
}

func TestNextInfoV1_BindingEventUpgradesProvenance(t *testing.T) {
	idx := buildTraceIndex(t, "nextinfo_v1_binding.systrace", `
       idle/0-0   (    0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=f,4,1,1,0 cg=top-app
       ctrl-300   (  900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0x3 cpuset=bg-restrict target_cpu=0 policy=bind
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.010, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	var seat *CPUConstraintSummary
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			seat = &stats.CPUConstraints[i]
			break
		}
	}
	if seat == nil {
		t.Fatalf("expected a constraint seat for app-100: %+v", stats.CPUConstraints)
	}
	if seat.CPUSet != "bg-restrict" || !seat.CPUSetIsBinding {
		t.Fatalf("a real binding event must override the cg= proxy and carry provenance: %+v", seat)
	}
	// Provenance-verify D2: the basis face must attribute the binding-
	// provenance verdict to the binding lane, not the proxy label.
	if seat.Kind != "sched_setaffinity" {
		t.Fatalf("binding upgrade must promote the basis kind off the proxy label: %+v", seat)
	}
}

func TestNextInfoV1_SameNameBindingEventIsDecisive(t *testing.T) {
	// Provenance-verify D1: the common platform shape — the binding event's
	// cpuset name EQUALS the earlier cg= proxy fill (cgroup and cpuset
	// controller share group names). The provenance flip alone must make the
	// binding event decisive: the seat's evidence span must include the
	// binding line (it is the SOLE witness the restriction gate fires on).
	idx := buildTraceIndex(t, "nextinfo_v1_samename.systrace", `
       idle/0-0   (    0) [000] .... 1.000000: sched_switch: prev_comm=idle/0 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52 next_info=f,4,1,1,0 cg=top-app
       ctrl-300   (  900) [001] .... 1.000500: sched_setaffinity: comm=app pid=100 mask=0xf cpuset=top-app target_cpu=0 policy=bind
        app-100   (  100) [000] .... 1.010000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/0 next_pid=0 next_prio=120
	`)
	q := Query{PID: 100, TimeStart: 1.000, TimeEnd: 1.010, TraceFlavorHint: TraceFlavorHarmonyHitrace, MinDurationMs: 0.05, Limit: 8}
	stats := ComputeWindowStats(idx, q)
	var seat *CPUConstraintSummary
	for i := range stats.CPUConstraints {
		if stats.CPUConstraints[i].Thread.PID == 100 {
			seat = &stats.CPUConstraints[i]
			break
		}
	}
	if seat == nil {
		t.Fatalf("expected a constraint seat for app-100: %+v", stats.CPUConstraints)
	}
	if seat.CPUSet != "top-app" || !seat.CPUSetIsBinding {
		t.Fatalf("same-name binding must keep the name AND carry provenance: %+v", seat)
	}
	if seat.LineEnd < 2 {
		t.Fatalf("the binding witness (line 2) must enter the evidence span: %+v", seat)
	}
	if seat.Kind != "sched_setaffinity" {
		t.Fatalf("same-name binding upgrade must promote the basis kind: %+v", seat)
	}
}

func TestNextInfoV1_EventJSONNoRestrictedKey(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(1, `        app-20   (   20) [001] .... 1.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53 next_info=e,0,1,1,0,1 cg=top-app`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if strings.Contains(string(raw), "next_info_restricted") {
		t.Fatalf("next_info_restricted must leave the JSON surface (V1): %s", raw)
	}
	if !strings.Contains(string(raw), "next_info_boost") {
		t.Fatalf("the typed boost lane must stay on the JSON surface: %s", raw)
	}
}
