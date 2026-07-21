package tracequery

// cluster_stream_test.go — CLUSTERSTREAM-1 (§29.193/§29.193.1, user
// authorization 2026-07-21; CLUSTERDIAG dossier §5) pin family: the streaming
// pairwise witness criterion, its clustering rules (pro floor + con one-vote
// veto + sameEmission second lane), and the Index-level single-derivation
// memo. The pair-level truth-table pins live in cluster_freq_share_cap3_test
// (evolved in place); THIS file pins the clustering/veto layer, the real
// donghu witness account, and the case1-scale heal.
//
// 渐进单向 (batch red line): today's already-judged faces regress nowhere —
// TestR6DonghuClusterGroundTruth (donghu judged 3-cluster truth) and
// TestR6TiebaClusterDerivation (tieba honest single_cluster) run unchanged
// beside this file and ARE the zero-regression pins.

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
)

// streamTL builds a transition timeline: base announcement then one
// transition per (ts, khz) pair, with a per-cpu member skew.
func streamTL(skew float64, points ...[2]float64) []freqSample {
	out := make([]freqSample, 0, len(points))
	for _, p := range points {
		out = append(out, freqSample{ts: p[0] + skew, khz: int64(p[1])})
	}
	return out
}

// --- donghu real-capture witness baseline (件5, 主会话实测对账) --------------

// The committed donghu capture under the witness criterion: three clusters
// (ground truth {0-3}/{4-11}/{12,13}), ZERO contradiction witnesses on every
// pair, ZERO cross-cluster pro witnesses, and the uniform per-pair pro
// accounts (every same-cluster pair witnesses every cluster transition:
// small 25 / middle 55 / big 63 co-witnessed transitions — the big account
// includes the §29.172 multi-value 齐动 burst as pro, never con). Caliber
// note: the ledger's burst-level figure (§29.193.1 「181 见证/零反例」 = 182
// pooled co-emission bursts of which 1 is the multi-value 齐动 form, DHM-C1
// census 182/1/0) states the SAME facts on the burst caliber; this pin
// accounts on the pairwise-transition caliber the criterion actually
// consumes. Fixture red line: real capture — measured numbers, never edit
// targets.
func TestClusterStreamDonghuWitnessBaseline(t *testing.T) {
	idx, err := BuildIndex(context.Background(), "../../eval/fixtures/real_traces/donghu.ftrace")
	if err != nil {
		t.Fatalf("BuildIndex(donghu): %v", err)
	}
	tls := indexFreqSampleTimelines(idx)
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 3 {
		t.Fatalf("donghu must derive three clusters, got %d (%+v)", d.groupCount, d.members)
	}
	var sampled []int
	for cpu, tl := range tls {
		if len(tl) > 0 {
			sampled = append(sampled, cpu)
		}
	}
	sort.Ints(sampled)
	wantPro := map[string]int{"derived_c0": 25, "derived_c1": 55, "derived_c2": 63}
	totalIntra := 0
	for i := 0; i < len(sampled); i++ {
		for j := i + 1; j < len(sampled); j++ {
			w := freqWitnessScanPair(tls[sampled[i]], tls[sampled[j]])
			if w.con != 0 {
				t.Fatalf("donghu 零反例 drifted: pair(%d,%d) con=%d — a real contradiction witness appeared; take it to the ledger, never re-base silently", sampled[i], sampled[j], w.con)
			}
			sameCluster := d.byCPU[sampled[i]] == d.byCPU[sampled[j]]
			if !sameCluster {
				if w.pro != 0 {
					t.Fatalf("cross-cluster pair(%d,%d) must accumulate zero pro, got %d", sampled[i], sampled[j], w.pro)
				}
				continue
			}
			totalIntra += w.pro
			if want := wantPro[d.byCPU[sampled[i]]]; w.pro != want {
				t.Fatalf("pair(%d,%d) pro=%d, want %d (uniform per-cluster witness account)", sampled[i], sampled[j], w.pro, want)
			}
		}
	}
	// 6 small pairs ×25 + 28 middle pairs ×55 + 1 big pair ×63.
	if totalIntra != 1753 {
		t.Fatalf("donghu intra-cluster pro total drifted: %d want 1753", totalIntra)
	}
}

// --- case1-scale heal + honest split (件5 case1 形合成 fixture) ---------------

// The fleet case1 shape at scale (dossier §2.2: 2517 频点行, ONE mid-stream
// unpaired point killed the whole cluster under the retired trimmed
// criterion): hundreds of co-witnessed transitions, one lost member row —
// the pair stays merged and the capability JUDGES (freq_only 不死). The
// second arm keeps the honesty direction: the same fixture with a PERSISTENT
// true divergence (repeated same-window different-value transitions) keeps
// minting con and honestly splits — and the fmax tie between the split twins
// fails loud with the conflict-arm split audit.
func TestClusterStreamCase1ScaleHealAndHonestSplit(t *testing.T) {
	const n = 300
	vals := []float64{1430000, 1530000, 1652000, 1930000}
	mk := func(skew float64, dropAt int, divergeEvery int) []freqSample {
		tl := []freqSample{{ts: 1.0 + skew, khz: 1090000}}
		for k := 0; k < n; k++ {
			ts := 1.0 + float64(k+1)*0.010 + skew
			khz := int64(vals[k%len(vals)])
			if divergeEvery > 0 && k%divergeEvery == 0 {
				khz += 50000 // true divergence: different value, same window
			}
			if k == dropAt {
				continue // the lost member row (case1 single unpaired point)
			}
			tl = append(tl, freqSample{ts: ts, khz: khz})
		}
		return tl
	}
	// Big cluster far away so the judgment has a second class.
	big := []freqSample{{ts: 1.0, khz: 2400000}, {ts: 2.5001, khz: 2750000}, {ts: 3.7002, khz: 2400000}, {ts: 4.9003, khz: 2750000}}

	// Arm 1 (heal): cpu0 full stream, cpu1 lost row k=150 → pro≈n-1, con=0.
	heal := map[int][]freqSample{
		0: mk(0, -1, 0),
		1: mk(1e-6, 150, 0),
		7: big,
	}
	d := deriveClusterFreqDomains(heal)
	if d.groupCount != 2 || d.byCPU[0] != d.byCPU[1] {
		t.Fatalf("case1 heal: one lost row among %d witnessed transitions must not split, got %+v", n, d.members)
	}
	capability := resolveCoreCapability(d, heal)
	if capability.source != CoreCapabilitySourceDefault || capability.freqOnlyReason != "" {
		t.Fatalf("case1 heal must JUDGE (freq_only 不死), got %q/%q", capability.source, capability.freqOnlyReason)
	}

	// Arm 2 (honest split): every 10th transition truly diverges on cpu1 —
	// con accumulates, the pair splits, the same-fmax twins tie and the
	// verdict fails loud with the conflict-arm audit (件3 败因因子: both
	// sides' kHz + skew).
	diverge := map[int][]freqSample{
		0: mk(0, -1, 0),
		1: mk(1e-6, -1, 10),
		7: big,
	}
	d = deriveClusterFreqDomains(diverge)
	if d.byCPU[0] == d.byCPU[1] {
		t.Fatalf("persistent divergence must split (con one-vote veto), got %+v", d.members)
	}
	capability = resolveCoreCapability(d, diverge)
	if capability.source != CoreCapabilitySourceFreqOnly || capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("split twins must fail loud on the fmax tie, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	for _, needle := range []string{"cpu0↔cpu1", "判定臂=" + freqCoMoveSplitArmConflict, "kHz vs ", "偏斜"} {
		if !strings.Contains(capability.freqOnlySplitAudit, needle) {
			t.Fatalf("conflict audit missing %q: %q", needle, capability.freqOnlySplitAudit)
		}
	}
}

// --- con one-vote veto across components (件2 传递安全) -----------------------

// Transitive-safety veto: A merges B (pro), B would merge C (pro), but A↔C
// carries a contradiction witness — the union A∪B may never absorb C
// (矛盾守卫传递安全). Deterministic outcome: {A,B} and {C} with ascending
// labels.
func TestClusterStreamConVetoBlocksTransitiveUnion(t *testing.T) {
	// A(cpu0): transitions at t=2/3/4 (shared with B) plus a lone transition
	// at t=10 (1800000) that B never witnesses.
	a := streamTL(0, [2]float64{1, 1000000}, [2]float64{2, 1100000}, [2]float64{3, 1200000},
		[2]float64{4, 1300000}, [2]float64{10, 1800000})
	// B(cpu1): pro(A,B)=3 via t=2/3/4; additionally transitions at t=5/6
	// (shared with C); nothing at t=10.
	b := streamTL(1e-6, [2]float64{1, 1000000}, [2]float64{2, 1100000}, [2]float64{3, 1200000},
		[2]float64{4, 1300000}, [2]float64{5, 1400000}, [2]float64{6, 1500000})
	// C(cpu2): pro(B,C)=2 via t=5/6, and a CONTRADICTION with A inside the
	// t=10 window (1850000 vs A's 1800000).
	c := streamTL(2e-6, [2]float64{1, 1000000}, [2]float64{5, 1400000}, [2]float64{6, 1500000},
		[2]float64{10, 1850000})
	tls := map[int][]freqSample{0: a, 1: b, 2: c}
	// Sanity: the pair verdicts really are pro(0,1)>=2, pro(1,2)>=2, con(0,2)>0.
	if w := freqWitnessScanPair(a, b); w.pro < 2 || w.con != 0 {
		t.Fatalf("fixture: pair(0,1) must be a pro edge, got %+v", w)
	}
	if w := freqWitnessScanPair(b, c); w.pro < 2 || w.con != 0 {
		t.Fatalf("fixture: pair(1,2) must be a pro edge, got %+v", w)
	}
	if w := freqWitnessScanPair(a, c); w.con == 0 {
		t.Fatalf("fixture: pair(0,2) must carry a con witness, got %+v", w)
	}
	d := deriveClusterFreqDomains(tls)
	if d.byCPU[0] != d.byCPU[1] {
		t.Fatalf("pro edge (0,1) must merge, got %+v", d.members)
	}
	if d.byCPU[2] == d.byCPU[0] {
		t.Fatalf("con(0,2) must veto the transitive union of cpu2 into {0,1} (一票否决), got %+v", d.members)
	}
	if d.groupCount != 2 || d.byCPU[0] != "derived_c0" || d.byCPU[2] != "derived_c1" {
		t.Fatalf("labels must stay ascending-deterministic, got %+v", d.byCPU)
	}
	// The same-pair self-veto twin: pro above the floor AND con on ONE pair
	// never merges (con outranks any pro count).
	if w := freqWitnessScanPair(a, c); w.pro >= clusterFreqCoWitnessFloor {
		t.Fatalf("fixture drift: pair(0,2) was meant to stay below the floor beside its con, got %+v", w)
	}
	selfVeto := map[int][]freqSample{
		0: streamTL(0, [2]float64{1, 1000000}, [2]float64{2, 1100000}, [2]float64{3, 1200000}, [2]float64{4, 1250000}),
		1: streamTL(1e-6, [2]float64{1, 1000000}, [2]float64{2, 1100000}, [2]float64{3, 1200000}, [2]float64{4, 1300000}),
	}
	if w := freqWitnessScanPair(selfVeto[0], selfVeto[1]); w.pro < clusterFreqCoWitnessFloor || w.con == 0 {
		t.Fatalf("fixture: self-veto pair must carry pro≥floor AND con, got %+v", w)
	}
	if d := deriveClusterFreqDomains(selfVeto); d.groupCount != 2 {
		t.Fatalf("a pair with any con must never merge regardless of pro, got %+v", d.members)
	}
}

// --- floor constant + announcement discipline (件2/件5) -----------------------

// The witness floor is the FIXED ruled constant 2 (§29.129 既裁③: zero
// adaptive behavior; §28.5 复核 P1 lineage) — a mutant loosening it to 1
// reds here (pro=1 地板臂), and announcements never count toward it
// (公告不铸见证: the §28.5 all-policy announcement sweep mints pro=0).
func TestClusterStreamFloorConstantAndAnnouncementDiscipline(t *testing.T) {
	if clusterFreqCoWitnessFloor != 2 {
		t.Fatalf("witness floor drifted: %d want 2 (§29.193.1, same constant lineage as §28.5 复核 P1)", clusterFreqCoWitnessFloor)
	}
	// pro=1: one co-witnessed transition — never a merge.
	one := map[int][]freqSample{
		0: streamTL(0, [2]float64{1, 1000000}, [2]float64{2, 1100000}),
		1: streamTL(1e-6, [2]float64{1.5, 1000000}, [2]float64{2, 1100000}),
	}
	if w := freqWitnessScanPair(one[0], one[1]); w.pro != 1 || w.con != 0 {
		t.Fatalf("fixture: want pro=1 con=0, got %+v", w)
	}
	if d := deriveClusterFreqDomains(one); d.groupCount != 2 {
		t.Fatalf("pro=1 must split (地板臂), got %+v", d.members)
	}
	// §28.5 复核 P1 announcement sweep: two parked clusters, first rows in
	// one skew window, different cadence — zero transitions, zero pro.
	sweep := map[int][]freqSample{
		0: {{ts: 1.0, khz: 1430000}, {ts: 2.0, khz: 1430000}, {ts: 3.0, khz: 1430000}},
		4: {{ts: 1.000005, khz: 1430000}, {ts: 1.8, khz: 1430000}},
	}
	if w := freqWitnessScanPair(sweep[0], sweep[4]); w.pro != 0 {
		t.Fatalf("公告不铸见证: the announcement sweep must mint zero pro, got %+v", w)
	}
	if d := deriveClusterFreqDomains(sweep); d.groupCount != 2 {
		t.Fatalf("parked announcement twins must stay split, got %+v", d.members)
	}
}

// --- Index-level single derivation memo (件1 复用纪律) ------------------------

// One derivation per Index: repeated indexed resolutions share the SAME
// member maps (pointer identity), the per-query ignored-input flag stays
// per-call, and an explicit topology bypasses the memo outright.
func TestClusterStreamIndexDerivationMemo(t *testing.T) {
	idx := buildTraceIndex(t, "clusterstream_memo.systrace",
		"  tppmgr-sched-in-5850  (    2) [001] .... 1.000000: cpu_frequency: state=1000000 cpu_id=0\n"+
			"  tppmgr-sched-in-5850  (    2) [001] .... 1.000001: cpu_frequency: state=1000000 cpu_id=1\n"+
			"  tppmgr-sched-in-5850  (    2) [001] .... 2.000000: cpu_frequency: state=1200000 cpu_id=0\n"+
			"  tppmgr-sched-in-5850  (    2) [001] .... 2.000001: cpu_frequency: state=1200000 cpu_id=1\n"+
			"  tppmgr-sched-in-5850  (    2) [001] .... 3.000000: cpu_frequency: state=1000000 cpu_id=0\n"+
			"  tppmgr-sched-in-5850  (    2) [001] .... 3.000001: cpu_frequency: state=1000000 cpu_id=1\n")
	d1 := resolveClusterFreqDomainsIndexed("", idx)
	d2 := resolveClusterFreqDomainsIndexed("garbage-topology", idx)
	if d1.source != ClusterFreqSourceDerived || d1.groupCount != 1 {
		t.Fatalf("memoized derivation wrong: %+v", d1)
	}
	if fmt.Sprintf("%p", d1.byCPU) != fmt.Sprintf("%p", d2.byCPU) {
		t.Fatalf("件1 复用: repeated indexed resolutions must share ONE derivation (map identity)")
	}
	if d1.explicitInputIgnored || !d2.explicitInputIgnored {
		t.Fatalf("P3-4 ignored-input flag must stay per-call: %v/%v", d1.explicitInputIgnored, d2.explicitInputIgnored)
	}
	if d := resolveClusterFreqDomainsIndexed("small=0-1;big=2-3", idx); d.source != ClusterFreqSourceExplicit {
		t.Fatalf("explicit topology must bypass the memo, got %q", d.source)
	}
}

// --- con-veto record feeds the split audit (复核 F1, 2026-07-21) -------------

// A real veto split whose audited-fragment REPRESENTATIVE pair itself
// co-moves: {0,1}|{2,3} where the vetoing con edges ride the
// non-representative member cpu1 — con(1,2) and con(1,3) — while the
// representative pair (0,2) holds pro≥floor ∧ con==0. Pre-fix the audit
// re-diagnosed only (0,2), took the "unexpectedly co-moves" branch and
// rendered EMPTY (fmax_tie verdict with zero 败因因子 — the self-diagnosis
// gap's residual form). The derivation now records every union-refusing con
// edge and the audit discloses the recorded edge with the
// transition_conflict factors verbatim; zero matching record keeps the
// honest empty return.
func TestClusterStreamConVetoRecordFeedsSplitAudit(t *testing.T) {
	tls := map[int][]freqSample{
		// Shared transitions @20/@30 give every cross pair pro=2; cpu0's lone
		// 1800000 @50 sits outside every skew window (no con, lifts c0 fmax).
		0: streamTL(0, [2]float64{10, 1000000}, [2]float64{20, 1430000}, [2]float64{30, 1200000}, [2]float64{50, 1800000}),
		// cpu1 @40.000001→1800000 contradicts cpu2 @40.000005→1700000 (4µs)
		// and cpu3 @40.000008→1700000 (7µs) inside one skew window.
		1: streamTL(1e-6, [2]float64{10, 1000000}, [2]float64{20, 1430000}, [2]float64{30, 1200000}, [2]float64{40, 1800000}),
		2: streamTL(5e-6, [2]float64{10, 1000000}, [2]float64{20, 1430000}, [2]float64{30, 1200000}, [2]float64{40, 1700000}),
		// cpu3's lone 1800000 @60.000008 ties c1's fmax with c0 (audit fires).
		3: streamTL(8e-6, [2]float64{10, 1000000}, [2]float64{20, 1430000}, [2]float64{30, 1200000}, [2]float64{40, 1700000}, [2]float64{60, 1800000}),
	}
	// Fixture sanity — the representative pair (0,2) really is the empty
	// shape's precondition: pro≥floor, con=0 (it would merge on its own).
	if w := freqWitnessScanPair(tls[0], tls[2]); w.pro < clusterFreqCoWitnessFloor || w.con != 0 {
		t.Fatalf("fixture: representative pair (0,2) must co-move on its own, got %+v", w)
	}
	for _, pair := range [][2]int{{1, 2}, {1, 3}} {
		if w := freqWitnessScanPair(tls[pair[0]], tls[pair[1]]); w.con == 0 {
			t.Fatalf("fixture: pair %v must carry the con witness, got %+v", pair, w)
		}
	}
	d := deriveClusterFreqDomains(tls)
	if d.groupCount != 2 || d.byCPU[0] != d.byCPU[1] || d.byCPU[2] != d.byCPU[3] || d.byCPU[0] == d.byCPU[2] {
		t.Fatalf("veto split must land {0,1}|{2,3}, got %+v", d.members)
	}
	// Derive-side record: both union-refusing edges, ascending scan order,
	// factors from the same witness scan that minted the con edges.
	if len(d.conVetoes) != 2 {
		t.Fatalf("want both refused unions recorded, got %+v", d.conVetoes)
	}
	first := d.conVetoes[0]
	if first.cpuA != 1 || first.cpuB != 2 || first.khzA != 1800000 || first.khzB != 1700000 {
		t.Fatalf("first veto record must be the con(1,2) edge with both targets, got %+v", first)
	}
	if d.conVetoes[1].cpuA != 1 || d.conVetoes[1].cpuB != 3 {
		t.Fatalf("second veto record must be the con(1,3) edge, got %+v", d.conVetoes[1])
	}
	// End-to-end: fmax tie (both clusters peak 1800000) → freq_only + the
	// audit seat discloses the recorded veto edge, factors verbatim.
	capability := resolveCoreCapability(d, tls)
	if capability.source != CoreCapabilitySourceFreqOnly || capability.freqOnlyReason != CoreCapabilityFreqOnlyReasonFmaxTie {
		t.Fatalf("fixture must land the fmax_tie freq_only form, got %q/%q", capability.source, capability.freqOnlyReason)
	}
	audit := capability.freqOnlySplitAudit
	for _, needle := range []string{"cpu1↔cpu2", "@40.000001", "判定臂=" + freqCoMoveSplitArmConflict,
		freqCoMoveSplitArmZH(freqCoMoveSplitArmConflict), "1800000kHz vs 1700000kHz", "偏斜4.0µs"} {
		if !strings.Contains(audit, needle) {
			t.Fatalf("veto-record audit missing %q: %q", needle, audit)
		}
	}
	// Zero matching record keeps the honest empty return (the pre-fix
	// post-rail-refinement fallback stays byte-identical).
	stripped := d
	stripped.conVetoes = nil
	if got := capabilityFreqOnlySplitAudit(stripped, tls, d.byCPU[0], d.byCPU[2]); got != "" {
		t.Fatalf("record-less co-moving representatives must keep the empty audit, got %q", got)
	}
}
