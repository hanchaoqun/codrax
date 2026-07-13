package tracequery

// rank_family_recon_case1_test.go — CASE-1 G1 recon SECOND adjudicated pair
// (§29.52 独立立案 CASE-1, smr_audit_report_20260712; witnesses SMR-S7 +
// S4-TPF + E9↔E15, reports 31693/42729/45903; v5 P1 批, 2026-07-13).
//
// ONE batch of source events — a thread's per-(thread,cpu) D-state / iowait
// ledger groups (stats.DStateTop / stats.IOWaitTop) — is published through
// TWO lanes of the SAME result:
//
//   rank lane   — rootCauseDIOStateFamilyItems mints ONE family contender per
//                 thread (members = the per-(thread,cpu) ledger groups; Type
//                 d_state_or_io_wait, or io_wait when dStateMs==0);
//   chain lane  — buildCriticalBlockingCallsFromStats mints one
//                 critical_blocking candidate per THE SAME ledger group.
//
// Adjudicated universe extension: {d_state_or_io_wait, io_wait}² joins
// {io_latency↔io_latency} (walked through the TestG1ReconTypeUniversePinned
// authority channel — see its EVOLUTION RECORD).
//
// Membership arm split (the CASE-1 technical caveat, S4-TPF vnote): a D/IO
// ledger group's [StartTs,EndTs] is a segment HULL (io_wait cpu=3 held 10
// discontinuous segments in the 42729 witness) — G1 explicitly rejects hull
// containment. The D/IO pairs therefore use the SAME-SOURCE identity ONLY:
// exact float interval equality AND exact float value equality against the
// family's validated member inventory (both lanes copy the same
// ThreadDuration floats). The io_latency pair keeps its original two-pass
// membership (exact member, then union containment) byte-identically.
//
// MUTATION self-checks:
//   - drop gap (a) (critical D/IO rows without StartTs/EndTs)
//                       → TestCase1EngineAbsorbsPureDStateGroups reds
//                         (no typed interval → membership unprovable);
//   - drop gap (b) (family without familyMemberIntervals)
//                       → same test reds (eligibility gate);
//   - drop gap (c) (io_wait family token missing from the map)
//                       → TestCase1EngineAbsorbsPureIOWaitGroups reds;
//   - loosen the same-source arm to union containment
//                       → TestCase1SubIntervalContainmentNeverAbsorbs reds;
//   - drop the value-identity dimension
//                       → TestCase1ValueMismatchNeverAbsorbs reds;
//   - widen the window gate → TestCase1CrossWindowNegative reds.

import (
	"math"
	"strings"
	"testing"
)

// case1PureDTrace: worker-200 blocks in D twice — the two blocking
// sched_switch rows land on DIFFERENT cpus (002 then 003), so the D ledger
// holds TWO per-(thread,cpu) groups (20ms + 30ms) and the family folds
// MemberCount=2. No sched_blocked_reason markers → pure non-IO D form.
const case1PureDTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// case1PureIOTrace: the same two-group geometry with BOTH wakeups carrying an
// iowait=1 sched_blocked_reason marker → both segments ride the IOWaitTop
// ledger, dStateMs==0 and the family mints the io_wait token (gap (c)).
const case1PureIOTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.030000: sched_blocked_reason: pid=200 iowait=1 caller=submit_bio_wait+0x74/0x160[kernel] delay=1000
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=200 iowait=1 caller=submit_bio_wait+0x74/0x160[kernel] delay=1000
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

// case1MixedTrace: first segment stays non-IO D (cpu 002 group), second
// carries iowait=1 (cpu 003 group → IOWaitTop). The ONE mixed family keeps
// the d_state_or_io_wait token while the chain lane mints one
// d_state_or_io_wait row AND one io_wait row — the cross-token combos of the
// {d_state_or_io_wait, io_wait}² universe. The iowait marker's caller is a
// pure hex address (parse collapses it to "unknown") so the §29.50.5 proof
// partition never carves the io fragment out — the mixed family stays ONE
// seat by construction.
const case1MixedTrace = `
        app-100 (100) [001] .... 3.000000: sched_switch: prev_comm=app prev_pid=100 prev_prio=52 prev_state=S ==> next_comm=idle/1 next_pid=0 next_prio=120
     worker-200 (200) [002] .... 3.001000: sched_switch: prev_comm=idle/2 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [002] .... 3.010000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/2 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.030000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
     worker-200 (200) [003] .... 3.031000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.040000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=D ==> next_comm=idle/3 next_pid=0 next_prio=120
       peer-300 (300) [003] .... 3.070000: sched_wakeup: comm=worker pid=200 prio=20 target_cpu=003
       peer-300 (300) [003] .... 3.070000: sched_blocked_reason: pid=200 iowait=1 caller=0xffffffc010123456 delay=1000
     worker-200 (200) [003] .... 3.071000: sched_switch: prev_comm=idle/3 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=worker next_pid=200 next_prio=20
     worker-200 (200) [003] .... 3.072000: sched_switch: prev_comm=worker prev_pid=200 prev_prio=20 prev_state=S ==> next_comm=idle/3 next_pid=0 next_prio=120
        app-100 (100) [001] .... 3.120000: sched_switch: prev_comm=idle/1 prev_pid=0 prev_prio=120 prev_state=R ==> next_comm=app next_pid=100 next_prio=52
`

func case1BuildBundle(t *testing.T, content string) FrameRootCauseBundle {
	t.Helper()
	idx := buildTraceIndex(t, "case1_recon.systrace", content)
	bundle := BuildFrameRootCauseBundle(idx, Query{PID: 100, TimeStart: 3.0, TimeEnd: 3.2,
		MaxDepth: 4, MinDurationMs: 0.05, TraceFlavorHint: TraceFlavorHarmonyHitrace, Limit: 16})
	if bundle.RootCauseRank == nil || bundle.CriticalBlocking == nil {
		t.Fatal("bundle must carry both lanes")
	}
	return bundle
}

func case1FamilyRow(t *testing.T, rank RootCauseRankResult, typ string) *RootCauseRankItem {
	t.Helper()
	var fam *RootCauseRankItem
	for i := range rank.Items {
		item := &rank.Items[i]
		if item.Thread.PID == 200 && item.Type == typ && strings.HasPrefix(item.Source, "window_stats") {
			if fam != nil {
				t.Fatalf("expected ONE window_stats %s family row for worker-200, found a second: %+v", typ, *item)
			}
			fam = item
		}
	}
	if fam == nil {
		t.Fatalf("no window_stats %s family row minted for worker-200; rank items: %+v", typ, rank.Items)
	}
	return fam
}

func case1BlockingRows(t *testing.T, blocking CriticalBlockingResult, types ...string) []*CriticalBlockingCandidate {
	t.Helper()
	want := map[string]bool{}
	for _, typ := range types {
		want[typ] = true
	}
	var rows []*CriticalBlockingCandidate
	for i := range blocking.Items {
		item := &blocking.Items[i]
		if item.Thread.PID == 200 && want[item.Type] {
			rows = append(rows, item)
		}
	}
	if len(rows) == 0 {
		t.Fatalf("no critical_blocking %v rows minted for worker-200: %+v", types, blocking.Items)
	}
	return rows
}

// TestCase1EngineAbsorbsPureDStateGroups — the pure-D two-group form: the
// family carries the validated member inventory (gap b), the chain-lane rows
// carry their typed group intervals (gap a), and every chain row absorbs into
// the ONE family seat by same-source identity.
func TestCase1EngineAbsorbsPureDStateGroups(t *testing.T) {
	bundle := case1BuildBundle(t, case1PureDTrace)
	fam := case1FamilyRow(t, *bundle.RootCauseRank, "d_state_or_io_wait")
	if fam.MemberCount != 2 {
		t.Fatalf("pure-D form must fold ×2 (two cpu groups), got member_count=%d", fam.MemberCount)
	}
	// Gap (b): the validated member inventory with same-source values.
	if len(fam.familyMemberIntervals) != 2 {
		t.Fatalf("family must carry the 2-member validated inventory, got %d", len(fam.familyMemberIntervals))
	}
	values := map[int]bool{}
	for _, iv := range fam.familyMemberIntervals {
		if iv.end <= iv.start {
			t.Fatalf("member inventory interval must be typed, got %+v", iv)
		}
		if iv.valueMs <= 0 {
			t.Fatalf("member inventory must carry the same-source value, got %+v", iv)
		}
		values[int(math.Round(iv.valueMs))] = true
	}
	if !values[20] || !values[30] {
		t.Fatalf("member inventory values must be the 20/30ms group accounts, got %+v", fam.familyMemberIntervals)
	}
	rows := case1BlockingRows(t, *bundle.CriticalBlocking, "d_state_or_io_wait")
	if len(rows) != 2 {
		t.Fatalf("chain lane must mint one candidate per ledger group, got %d", len(rows))
	}
	for _, row := range rows {
		// Gap (a): the chain-lane candidate carries its typed group interval
		// ENGINE-INTERNALLY (修复轮 h1 ∿ 回归: the hull stays OFF the
		// published StartTs/EndTs wire — a hull is not an occurrence segment
		// and span-overlap fold arms must never fire on it).
		if start, end := criticalBlockingReconInterval(*row); start <= 0 || end <= start {
			t.Fatalf("chain-lane D/IO candidate must carry the recon interval, got %+v", *row)
		}
		if row.StartTs != 0 || row.EndTs != 0 {
			t.Fatalf("the hull must stay off the published wire, got %+v", *row)
		}
		if !row.AbsorbedByRankFamily {
			t.Fatalf("chain-lane row must absorb into the family seat, got %+v", *row)
		}
		if row.AbsorbedIntoFamily != fam.RankFamilyKey || fam.RankFamilyKey == "" {
			t.Fatalf("absorption key mismatch: row=%q fam=%q", row.AbsorbedIntoFamily, fam.RankFamilyKey)
		}
	}
	if fam.AbsorbedChainRows != 2 {
		t.Fatalf("family must count exactly its two absorbed chain rows, got %d", fam.AbsorbedChainRows)
	}
	// 观测照发不删: absorption stamps markers only — the chain rows stay
	// published on the blocking result.
	if len(rows) != 2 {
		t.Fatalf("absorbed rows must remain published, got %d", len(rows))
	}
}

// TestCase1EngineAbsorbsPureIOWaitGroups — gap (c): dStateMs==0 mints the
// io_wait family token; the io_wait↔io_wait combo of the adjudicated square
// must absorb.
func TestCase1EngineAbsorbsPureIOWaitGroups(t *testing.T) {
	bundle := case1BuildBundle(t, case1PureIOTrace)
	fam := case1FamilyRow(t, *bundle.RootCauseRank, "io_wait")
	if fam.MemberCount != 2 {
		t.Fatalf("pure-IO form must fold ×2, got member_count=%d", fam.MemberCount)
	}
	if len(fam.familyMemberIntervals) != 2 {
		t.Fatalf("family must carry the 2-member validated inventory, got %d", len(fam.familyMemberIntervals))
	}
	rows := case1BlockingRows(t, *bundle.CriticalBlocking, "io_wait")
	if len(rows) != 2 {
		t.Fatalf("chain lane must mint one io_wait candidate per ledger group, got %d", len(rows))
	}
	for _, row := range rows {
		if !row.AbsorbedByRankFamily || row.AbsorbedIntoFamily != fam.RankFamilyKey {
			t.Fatalf("io_wait chain row must absorb into the io_wait family, got %+v", *row)
		}
	}
	if fam.AbsorbedChainRows != 2 {
		t.Fatalf("family must count its two absorbed chain rows, got %d", fam.AbsorbedChainRows)
	}
}

// TestCase1EngineAbsorbsMixedFamilyCrossToken — the cross-token combos: ONE
// mixed d_state_or_io_wait family absorbs BOTH the d_state_or_io_wait chain
// row and the io_wait chain row (io_wait↔d_state_or_io_wait arm).
func TestCase1EngineAbsorbsMixedFamilyCrossToken(t *testing.T) {
	bundle := case1BuildBundle(t, case1MixedTrace)
	fam := case1FamilyRow(t, *bundle.RootCauseRank, "d_state_or_io_wait")
	if fam.MemberCount != 2 || fam.DStateMs <= 0 || fam.IOWaitMs <= 0 {
		t.Fatalf("mixed form must fold one D + one IO member, got %+v", *fam)
	}
	rows := case1BlockingRows(t, *bundle.CriticalBlocking, "d_state_or_io_wait", "io_wait")
	if len(rows) != 2 {
		t.Fatalf("chain lane must mint one row per ledger group across both tokens, got %d", len(rows))
	}
	seenTypes := map[string]bool{}
	for _, row := range rows {
		seenTypes[row.Type] = true
		if !row.AbsorbedByRankFamily || row.AbsorbedIntoFamily != fam.RankFamilyKey {
			t.Fatalf("%s chain row must absorb into the ONE mixed family, got %+v", row.Type, *row)
		}
	}
	if !seenTypes["d_state_or_io_wait"] || !seenTypes["io_wait"] {
		t.Fatalf("fixture drifted: expected one row per token, got %+v", seenTypes)
	}
	if fam.AbsorbedChainRows != 2 {
		t.Fatalf("mixed family must absorb both chain rows, got %d", fam.AbsorbedChainRows)
	}
}

// case1UnitFamily builds the unit-form family/blocking pair for the negative
// arms (same style as the B4 unit fixtures).
func case1UnitFamily() (RootCauseRankResult, CriticalBlockingResult) {
	fam := RootCauseRankItem{
		Type:               "d_state_or_io_wait",
		Thread:             ThreadRef{Comm: "worker", PID: 200},
		MemberCount:        2,
		StatsWindowStartTs: 3.0,
		StatsWindowEndTs:   3.2,
	}
	fam.familyMemberIntervals = []foldInterval{
		{start: 3.010, end: 3.030, valueMs: 20.0},
		{start: 3.040, end: 3.070, valueMs: 30.0},
	}
	rank := RootCauseRankResult{Items: []RootCauseRankItem{fam}}
	blocking := CriticalBlockingResult{
		Window: TimeWindow{StartTs: 3.0, EndTs: 3.2},
		Items: []CriticalBlockingCandidate{{
			Type:       "d_state_or_io_wait",
			Thread:     ThreadRef{Comm: "worker", PID: 200},
			DurationMs: 20.0,
			StartTs:    3.010,
			EndTs:      3.030,
		}},
	}
	return rank, blocking
}

// TestCase1SubIntervalContainmentNeverAbsorbs — the hull/containment veto:
// a chain row whose interval sits strictly INSIDE a family member interval
// (union containment — the io_latency pass-1 arm) must NOT absorb on the
// D/IO pairs; the group interval is a segment hull and containment would
// admit non-members (S4-TPF vnote: G1 明拒 hull).
func TestCase1SubIntervalContainmentNeverAbsorbs(t *testing.T) {
	rank, blocking := case1UnitFamily()
	blocking.Items[0].StartTs = 3.012
	blocking.Items[0].EndTs = 3.028
	blocking.Items[0].DurationMs = 16.0
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("sub-interval containment must never absorb on the D/IO pair: %+v", blocking.Items[0])
	}
	if rank.Items[0].AbsorbedChainRows != 0 {
		t.Fatalf("family must not count a rejected row, got %d", rank.Items[0].AbsorbedChainRows)
	}
}

// TestCase1ValueMismatchNeverAbsorbs — the µs value-identity dimension: an
// exact interval match whose published value diverges from the member account
// is NOT the same source measurement (fail open, honest duplicate beats
// wrong absorption).
func TestCase1ValueMismatchNeverAbsorbs(t *testing.T) {
	rank, blocking := case1UnitFamily()
	blocking.Items[0].DurationMs = 19.0 // interval matches member 1, value does not
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("value mismatch must never absorb on the D/IO pair: %+v", blocking.Items[0])
	}
}

// TestCase1ValuelessInventoryNeverAbsorbs — a member inventory without the
// same-source value carrier (foreign/legacy inventories) is unprovable for
// the D/IO arm and must fail open.
func TestCase1ValuelessInventoryNeverAbsorbs(t *testing.T) {
	rank, blocking := case1UnitFamily()
	for i := range rank.Items[0].familyMemberIntervals {
		rank.Items[0].familyMemberIntervals[i].valueMs = 0
	}
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("valueless inventory must never absorb on the D/IO pair: %+v", blocking.Items[0])
	}
}

// TestCase1CrossWindowNegative — 跨窗不对账 holds on the new pairs.
func TestCase1CrossWindowNegative(t *testing.T) {
	rank, blocking := case1UnitFamily()
	rank.Items[0].StatsWindowStartTs = 2.9
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("cross-window absorption is forbidden: %+v", blocking.Items[0])
	}
}

// TestCase1ExactMemberIdentityAbsorbs — the unit-form positive: exact
// same-source interval + value identity absorbs and mints matching keys.
func TestCase1ExactMemberIdentityAbsorbs(t *testing.T) {
	rank, blocking := case1UnitFamily()
	reconcileCriticalBlockingWithRankFamilies(&rank, &blocking)
	if !blocking.Items[0].AbsorbedByRankFamily {
		t.Fatalf("exact member identity must absorb: %+v", blocking.Items[0])
	}
	if blocking.Items[0].AbsorbedIntoFamily == "" ||
		blocking.Items[0].AbsorbedIntoFamily != rank.Items[0].RankFamilyKey {
		t.Fatalf("keys must join verbatim: row=%q fam=%q",
			blocking.Items[0].AbsorbedIntoFamily, rank.Items[0].RankFamilyKey)
	}
	if rank.Items[0].AbsorbedChainRows != 1 {
		t.Fatalf("family must count exactly one absorbed row, got %d", rank.Items[0].AbsorbedChainRows)
	}
}
