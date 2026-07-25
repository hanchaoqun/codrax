package tracequery

import (
	"strings"
	"testing"
)

// next_info_p1_test.go — NEXTINFO P1 pins (客户 next_info 语义文档, 2026-07-25):
// field 3 is ices_boost (NOT a restriction), every 0 has closed-set meaning
// (load=0 power hint, expel=0 expellee, cgid=0 SP_DEFAULT), the format is
// incremental (extra tail fields survive), and a malformed field fails open
// to Known=false instead of collapsing to a fake 0-meaning.

func TestParseHarmonyNextInfoCustomerSixFieldShape(t *testing.T) {
	// Customer doc example: next_info=e,166,3,0,0,1
	info, ok := parseHarmonyNextInfo("e,166,3,0,0,1")
	if !ok {
		t.Fatal("customer 6-field shape must parse")
	}
	if info.affinity != "e" || strings.Join(intsToStrings(info.allowedCPUs), ",") != "1,2,3" {
		t.Fatalf("affinity e should expand to CPUs 1-3: %+v", info)
	}
	if !info.loadKnown || info.load != 166 {
		t.Fatalf("load=166 lost: %+v", info)
	}
	if !info.groupKnown || info.group != 3 {
		t.Fatalf("group=3 (capacity) lost: %+v", info)
	}
	if !info.boostKnown || info.boost || info.restricted {
		t.Fatalf("boost=0 must read known-false boost (and bug-compat restricted=false): %+v", info)
	}
	if !info.expelKnown || info.expel != 0 {
		t.Fatalf("expel=0 (expellee) must stay KNOWN zero: %+v", info)
	}
	if !info.cgidKnown || info.cgid != 1 {
		t.Fatalf("cgid=1 (SP_BACKGROUND) lost: %+v", info)
	}
	if info.fieldCount != 6 || len(info.extra) != 0 {
		t.Fatalf("field census wrong: %+v", info)
	}
}

func TestParseHarmonyNextInfoIncrementalTailAndBoost(t *testing.T) {
	// 7+ fields: tail preserved verbatim; boost=1 keeps the bug-compatible
	// restricted fill until NEXTINFO-V1 retires its consumers.
	info, ok := parseHarmonyNextInfo("f,0,1,1,2,4,17,3")
	if !ok {
		t.Fatal("incremental 8-field shape must parse")
	}
	if !info.loadKnown || info.load != 0 {
		t.Fatalf("load=0 is the power-domain hint and must stay KNOWN: %+v", info)
	}
	if !info.boostKnown || !info.boost || !info.restricted {
		t.Fatalf("boost=1 must set boost AND bug-compat restricted: %+v", info)
	}
	if info.fieldCount != 8 || len(info.extra) != 2 || info.extra[0] != "17" || info.extra[1] != "3" {
		t.Fatalf("incremental tail must survive verbatim: %+v", info)
	}
}

func TestParseHarmonyNextInfoMalformedFieldFailsOpen(t *testing.T) {
	info, ok := parseHarmonyNextInfo("f,10,x,1,3")
	if !ok {
		t.Fatal("one malformed field must not reject the whole payload")
	}
	if info.groupKnown || info.group != 0 {
		t.Fatalf("malformed group must fail open to known=false, not fake 0: %+v", info)
	}
	if !info.loadKnown || !info.boostKnown || !info.expelKnown {
		t.Fatalf("well-formed siblings must stay known: %+v", info)
	}
	if info.cgidKnown {
		t.Fatalf("5-field shape has no cgid claim: %+v", info)
	}
}

func TestNextInfoLexiconClosedSets(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{NextInfoSchedGroupWord(0, true), "no_group"},
		{NextInfoSchedGroupWord(1, true), "power_group"},
		{NextInfoSchedGroupWord(2, true), "energy_group"},
		{NextInfoSchedGroupWord(3, true), "capacity_group"},
		{NextInfoSchedGroupWord(7, true), "unknown_group_7"},
		{NextInfoSchedGroupWord(2, false), "unknown"},
		{NextInfoSMTExpelWord(0, true), "expellee"},
		{NextInfoSMTExpelWord(1, true), "util_expeller"},
		{NextInfoSMTExpelWord(2, true), "expeller"},
		{NextInfoSMTExpelWord(3, true), "force_expeller"},
		{NextInfoSMTExpelWord(4, true), "force_expeller_long"},
		{NextInfoSMTExpelWord(9, true), "unknown_expel_9"},
		{NextInfoSPCGroupName(0, true), "SP_DEFAULT"},
		{NextInfoSPCGroupName(1, true), "SP_BACKGROUND"},
		{NextInfoSPCGroupName(4, true), "SP_TOP_APP"},
		{NextInfoSPCGroupName(15, true), "SP_LOW_BACKGROUND"},
		{NextInfoSPCGroupName(16, true), "unknown_cgroup_16"},
		{NextInfoSPCGroupName(3, false), "unknown"},
	} {
		if tc.got != tc.want {
			t.Fatalf("lexicon word drifted: got %q want %q", tc.got, tc.want)
		}
	}
}

func TestRenderNextInfoPolicyKnownGatedTokens(t *testing.T) {
	intern := newStringInterner()
	ev, ok := ParseLine(1, `        app-20   (   20) [001] .... 1.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=20 next_prio=53 next_info=e,0,1,1,0,1 cg=top-app`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	got := renderNextInfoPolicy(ev)
	// Legacy tokens stay byte-stable (the "restricted=true" substring gates
	// depend on them until NEXTINFO-V1).
	for _, want := range []string{
		"restricted=true",
		"load=0",
		"ices_boost=true",
		"sched_group=power_group",
		"smt_expel=expellee",
		"cgroup_name=SP_BACKGROUND",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("renderNextInfoPolicy missing %q: %q", want, got)
		}
	}
	// A malformed field renders NO closed-set word for that lane.
	ev2, ok := ParseLine(2, `        app-21   (   21) [001] .... 1.130000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app2 next_pid=21 next_prio=53 next_info=f,10,zz,0,3 cg=top-app`, intern)
	if !ok {
		t.Fatal("line 2 must parse")
	}
	got2 := renderNextInfoPolicy(ev2)
	if strings.Contains(got2, "sched_group=") {
		t.Fatalf("malformed group must not mint a closed-set word: %q", got2)
	}
	if !strings.Contains(got2, "smt_expel=force_expeller") {
		t.Fatalf("well-formed expel lane must still speak: %q", got2)
	}
}

// TestRenderNextInfoPolicyLegacyFacesKnownGated — AUD-05(2) (§14.6,
// 2026-07-25): the LEGACY numeric tokens are Known-gated too — a malformed
// field no longer prints a pseudo-measured group=0/restricted=false, while
// every well-formed payload keeps its bytes and the out-of-doc boost=2 keeps
// the bug-compatible restricted=true gate token (V1 frozen) without the
// semantic ices_boost claim.
func TestRenderNextInfoPolicyLegacyFacesKnownGated(t *testing.T) {
	intern := newStringInterner()
	malformed, ok := ParseLine(1, `        app-22   (   22) [001] .... 1.140000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app3 next_pid=22 next_prio=53 next_info=f,10,zz,zz,3 cg=top-app`, intern)
	if !ok {
		t.Fatal("line must parse")
	}
	got := renderNextInfoPolicy(malformed)
	for _, banned := range []string{"group=0", "restricted=false"} {
		if strings.Contains(got, banned) {
			t.Fatalf("malformed fields must not print pseudo-measured %q: %q", banned, got)
		}
	}
	if !strings.Contains(got, "load=10") || !strings.Contains(got, "expel=3") {
		t.Fatalf("well-formed siblings keep their bytes: %q", got)
	}
	boost2, ok := ParseLine(2, `        app-23   (   23) [001] .... 1.150000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app4 next_pid=23 next_prio=53 next_info=f,10,2,2,3 cg=top-app`, intern)
	if !ok {
		t.Fatal("boost2 line must parse")
	}
	got2 := renderNextInfoPolicy(boost2)
	if !strings.Contains(got2, "restricted=true") {
		t.Fatalf("boost=2 keeps the bug-compatible gate token: %q", got2)
	}
	if strings.Contains(got2, "ices_boost=") {
		t.Fatalf("boost=2 is outside the doc closed set — no semantic claim: %q", got2)
	}
	wellFormed, ok := ParseLine(3, `        app-24   (   24) [001] .... 1.160000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app5 next_pid=24 next_prio=53 next_info=f,10,2,1,3,6 cg=top-app`, intern)
	if !ok {
		t.Fatal("well-formed line must parse")
	}
	got3 := renderNextInfoPolicy(wellFormed)
	for _, want := range []string{"load=10", "group=2", "restricted=true", "expel=3", "cgid=6"} {
		if !strings.Contains(got3, want) {
			t.Fatalf("well-formed payload keeps every legacy byte %q: %q", want, got3)
		}
	}
}
