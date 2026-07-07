package tool

// answer_document_projection_p0a2_coverage_test.go — P0-A2 pins (coordinator
// lane ruling 2026-07-07) for the two §12.3-4 / §18.C rulings on self-heavy /
// hop-only projections. Every numerator/disclosure pin drives the REAL builder
// buildRuntimeTraceProjTreeModel — the admission gate reads model.SelfRows
// (where the builder consumes the 🎯 target's own blocked-wait rows), not the
// depthless stray lane (which the builder empties of target-subject rows before
// it is reached — the dead-code trap the first cut fell into).
//
//  Item 1 (§12.3-4 裁定4②):
//    the target's OWN lock/binder blocked-wait row (subject==target, resolved
//    peer Object, EffectiveImpactMS>0) is admitted into the coverage numerator
//    under MAX discipline; an unresolved-counterpart row (Object is a
//    state/type token — F6) or a bare state row stays residual + disclosed.
//  Item 1 disclosure (§12.3-4 裁定4①): the honest "未计入的链上行" count/max.

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// p0a2SelfLockProjection builds a q6/q8-shaped projection whose 🎯 target
// (app_main-100) blocked in a lock/binder wait on a NAMED holder (peer with a
// pid tail). The wait row is a causal_hop self view carrying EffectiveImpactMS —
// exactly the SelfRows shape the numerator lane must admit. `peer` chooses the
// Object identity (resolved thread label vs a bare state token — F6). Additional
// self hops let the MAX pin (F2) and the disclosure pin exercise the real
// builder without hand-built models.
func p0a2SelfLockProjection(peer string, effMS float64, extraHops ...types.TraceCausalProjectionNode) types.TraceCausalProjection {
	hops := []types.TraceCausalProjectionNode{{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-lock",
		Subject: "app_main-100", Object: peer, StateKind: "s_sleep",
		Predicate: "critical_blocking", TypeToken: "monitor_contention",
		ImpactMS: effMS, EffectiveImpactMS: effMS, Confidence: 0.9,
	}}
	hops = append(hops, extraHops...)
	return types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app_main-100"},
		WindowStartTs: 100.0, WindowEndTs: 100.151,
		SupportingHops: hops,
		// A shallow resolved chain lane (small running row) so the depth-only
		// numerator would otherwise dominate the coverage — the self lock row
		// must LIFT it.
		OnChainCauses: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-chain",
			Subject: "waker-1", Object: "running", ChainRelevance: "on_chain",
			ChainDepth: 1, ImpactMS: 0.051, CumulativeImpactMS: 0.051, Confidence: 0.7,
		}},
	}
}

func TestP0A2NumeratorAdmitsTargetSelfResolvedCounterpartRow(t *testing.T) {
	// q6/q8 lock case: target blocked 4.577ms on OS_IPC-6543 (resolved peer).
	// The self lock row must lift the numerator above the 0.051 running lane.
	projection := p0a2SelfLockProjection("OS_IPC-6543", 4.577)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if len(model.SelfRows) == 0 {
		t.Fatalf("fixture drifted: expected the target lock row in SelfRows, got none")
	}
	if got := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model); got != 4.577 {
		t.Fatalf("resolved-peer self lock row must be admitted: got %.3f", got)
	}
	if got := runtimeTraceProjDepth1Cumulative(model); got != 4.577 {
		t.Fatalf("numerator must be MAX(depth 0.051, admitted 4.577) = 4.577: got %.3f", got)
	}
}

func TestP0A2NumeratorMAXNotSumAcrossSelfRows(t *testing.T) {
	// F2 pin: two admitted self rows → the numerator is the MAX, never the Σ.
	extra := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-lock2",
		Subject: "app_main-100", Object: "OS_IPC-7000", StateKind: "s_sleep",
		Predicate: "critical_blocking", TypeToken: "monitor_contention",
		ImpactMS: 2.100, EffectiveImpactMS: 2.100, Confidence: 0.9,
	}
	projection := p0a2SelfLockProjection("OS_IPC-6543", 4.577, extra)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if got := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model); got != 4.577 {
		t.Fatalf("two admitted rows must take the MAX (4.577), not the Σ (6.677): got %.3f", got)
	}
}

func TestP0A2NumeratorRejectsStateTokenCounterpart(t *testing.T) {
	// F6 pin: Object is a scheduler-state token, not a resolved thread — the row
	// is NOT admitted (禁把状态词当对端) and instead feeds the disclosure.
	for _, stateToken := range []string{"sleep_wait", "d_state_or_io_wait", "lock_contention", "monitor_contention"} {
		projection := p0a2SelfLockProjection(stateToken, 9.0)
		model := buildRuntimeTraceProjTreeModel(projection, nil, true)
		if got := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model); got != 0 {
			t.Fatalf("state-token counterpart %q must not be admitted: numerator got %.3f", stateToken, got)
		}
		if got := runtimeTraceProjDepth1Cumulative(model); got != 0.051 {
			t.Fatalf("unresolved-counterpart row must leave the numerator at the depth lane: got %.3f", got)
		}
	}
	// Positive/negative controls for the F6 identity predicate itself.
	for _, ok := range []string{"OS_IPC-6543", "pid=6543", "42067"} {
		if !runtimeTraceProjResolvedCounterpartObject(ok) {
			t.Fatalf("%q must count as a resolved counterpart", ok)
		}
	}
	for _, bad := range []string{"", "unknown-thread", "sleep_wait", "lock_contention", "monitor_contention"} {
		if runtimeTraceProjResolvedCounterpartObject(bad) {
			t.Fatalf("%q must not count as a resolved counterpart", bad)
		}
	}
}

func TestP0A2DisclosureCountsUnresolvedSelfRow(t *testing.T) {
	// One admitted lock row (OS_IPC) + one unresolved-counterpart self row
	// (Object=lock_contention) → numerator counts the admitted; disclosure
	// counts the 1 unadmitted self row.
	extra := types.TraceCausalProjectionNode{
		Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-unres",
		Subject: "app_main-100", Object: "lock_contention", StateKind: "s_sleep",
		Predicate: "critical_blocking", TypeToken: "monitor_contention",
		ImpactMS: 2.770, EffectiveImpactMS: 2.770, Confidence: 0.8,
	}
	projection := p0a2SelfLockProjection("OS_IPC-6543", 4.577, extra)
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	n, x, folded := runtimeTraceProjUnadmittedOnChainDisclosure(model)
	if n != 1 || x != 2.770 || folded {
		t.Fatalf("disclosure must count the 1 unadmitted self row (max 2.770, unfolded): n=%d x=%.3f folded=%v", n, x, folded)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "另有 1 项未计入的链上行(单项最大 2.770ms,墙钟不可加和,见明细表/树)未纳入本覆盖分子。") {
		t.Fatalf("zh disclosure sentence missing/incorrect:\n%s", line)
	}
	en := runtimeTraceProjWindowLine(projection, model, false)
	if !strings.Contains(en, "A further 1 on-chain row(s) not counted here (single largest 2.770ms, wall clock not summable; see the detail table/tree) are outside this coverage numerator.") {
		t.Fatalf("EN disclosure sentence missing/incorrect:\n%s", en)
	}
	// F8 wording: no internal 行话 ("深度未解析/降背景") on the panel.
	if strings.Contains(line, "深度未解析") || strings.Contains(line, "降背景") {
		t.Fatalf("disclosure must not leak internal 行话:\n%s", line)
	}
}

func TestP0A2DisclosureFiresEvenWhenNumeratorZero(t *testing.T) {
	// F7 pin: a self-heavy shape whose whole numerator is 0 (the ONLY self rows
	// are unresolved-counterpart) renders no coverage sentence — yet it is the
	// shape most in need of the disclosure, which must still appear.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app_main-100"},
		WindowStartTs: 100.0, WindowEndTs: 100.151,
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-unres",
			Subject: "app_main-100", Object: "lock_contention", StateKind: "s_sleep",
			Predicate: "critical_blocking", TypeToken: "monitor_contention",
			ImpactMS: 5.500, EffectiveImpactMS: 5.500, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if got := runtimeTraceProjDepth1Cumulative(model); got != 0 {
		t.Fatalf("numerator must be 0 for an unresolved-only self-heavy shape: got %.3f", got)
	}
	line := runtimeTraceProjWindowLine(projection, model, true)
	if !strings.Contains(line, "另有 1 项未计入的链上行") {
		t.Fatalf("disclosure must fire even with a zero numerator (F7):\n%s", line)
	}
}

func TestP0A2OwnEdgeRowNeverAdmittedOrDoubleDisclosed(t *testing.T) {
	// F4 pin: an own-process IO caliber self row (Edge==own, stamped by the real
	// builder's runtimeTraceProjOwnProcessIONode) is excluded from BOTH the
	// numerator AND the disclosure — never two contradictory exclusion reasons
	// for the same row.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"waker-1", "app_main-100"},
		WindowStartTs: 100.0, WindowEndTs: 100.151,
		OnChainCauses: []types.TraceCausalProjectionNode{{
			// Same pid (100) as the target, different comm label + IO caliber
			// token → runtimeTraceProjOwnProcessIONode stamps Edge==own.
			Role: types.TraceCausalRoleRootCauseContext, EvidenceID: "e-own",
			Subject: "app_worker-100", Object: "OS_IPC-6543", TypeToken: "io_burst_episode",
			ChainRelevance: "on_chain", ImpactMS: 232.0, EffectiveImpactMS: 232.0, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	// Locate the own-edge row wherever the builder seated it.
	ownSeen := false
	for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
		for _, row := range rows {
			if row.Edge == runtimeTraceProjTreeEdgeOwn {
				ownSeen = true
			}
		}
	}
	if !ownSeen {
		t.Skipf("fixture drifted: expected an own-edge row to be stamped")
	}
	if got := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model); got != 0 {
		t.Fatalf("own-process IO caliber must never be admitted to the numerator: got %.3f", got)
	}
	// Whatever else the disclosure counts, it must not count the own-edge row.
	nBase, _, _ := runtimeTraceProjUnadmittedOnChainDisclosure(model)
	for _, row := range model.SelfRows {
		if row.Edge == runtimeTraceProjTreeEdgeOwn && nBase != 0 {
			t.Fatalf("own-edge self row must be excluded from the disclosure count: n=%d", nBase)
		}
	}
}

func TestP0A2FlatModelDoesNotMintSubjectlessNumerator(t *testing.T) {
	// F5 pin: a flat model (no ≥2-node wakeup path → model.Target=="") must not
	// admit a subjectless self node via empty-canonical equality.
	model := runtimeTraceProjTreeModel{
		Target: "",
		SelfRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "", Object: "OS_IPC-6543", EffectiveImpactMS: 50.0}},
		},
	}
	if got := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model); got != 0 {
		t.Fatalf("flat model (empty target) must not mint a subjectless numerator: got %.3f", got)
	}
}

// --- P0-A1 byte-preservation guard ----------------------------------------------

func TestP0A2DisclosureAbsentForP0A1Witness(t *testing.T) {
	// P0-A1 byte-preservation: the depthless-free / self-state-only witness model
	// has no unadmitted on-chain row and no admitted self lock row, so the
	// coverage line is byte-identical and the numerator stays 112.223.
	model := p0a1CoverageModel(112.175, 112.223)
	if n, _, _ := runtimeTraceProjUnadmittedOnChainDisclosure(model); n != 0 {
		t.Fatalf("P0-A1 witness must have zero unadmitted rows: got %d", n)
	}
	if got := runtimeTraceProjAdmittedTargetSelfNumeratorMS(model); got != 0 {
		t.Fatalf("P0-A1 witness self rows carry no attribution caliber → not admitted: got %.3f", got)
	}
	line := runtimeTraceProjWindowLine(p0a1Projection, model, true)
	if strings.Contains(line, "未纳入本覆盖分子") {
		t.Fatalf("no disclosure may appear for a fully-countable P0-A1 coverage line:\n%s", line)
	}
	if got := runtimeTraceProjDepth1Cumulative(model); got != 112.223 {
		t.Fatalf("P0-A1 numerator must be byte-preserved: got %.3f", got)
	}
}

// --- Item 2: target symptom cell hop-only fallback ------------------------------

func TestP0A2TargetSymptomCellHopOnlyCaliberAnnotation(t *testing.T) {
	// q9 shape via the REAL builder: the target's only self row is a causal_hop
	// sleep view → the F1 state-segment aggregate is 0 but a hop-view sleep
	// exists; the cell shows the value WITH its view-caliber annotation.
	projection := types.TraceCausalProjection{
		WakeupPath:    []string{"dep-2", "self-1"},
		WindowStartTs: 100.0, WindowEndTs: 102.0,
		SupportingHops: []types.TraceCausalProjectionNode{{
			Role: types.TraceCausalRoleCausalHop, EvidenceID: "e-self",
			Subject: "self-1", Object: "sleep_wait", StateKind: "s_sleep",
			ImpactMS: 1300.441, Confidence: 0.8,
		}},
	}
	model := buildRuntimeTraceProjTreeModel(projection, nil, true)
	if got := runtimeTraceProjTargetSymptomMS(model); got != 0 {
		t.Fatalf("F1 state-segment aggregate must stay 0 for a hop-only target: got %.3f", got)
	}
	cell := runtimeTraceProjCompareTargetSymptomCell(model, true)
	if cell != "1300.441ms(唤醒链视图目标睡眠,非状态段聚合)" {
		t.Fatalf("zh hop-only cell must carry the view-caliber annotation: %q", cell)
	}
	en := runtimeTraceProjCompareTargetSymptomCell(model, false)
	if en != "1300.441ms (wakeup-chain-view target sleep, not a state-segment aggregate)" {
		t.Fatalf("EN hop-only cell must carry the view-caliber annotation: %q", en)
	}
}

func TestP0A2TargetSymptomCellStateViewUnannotated(t *testing.T) {
	model := runtimeTraceProjTreeModel{
		Target:   "self-1",
		WindowMS: 2000.0,
		SelfRows: []runtimeTraceProjTreeRow{
			{Kind: runtimeTraceProjTreeRowSelf, HasData: true,
				Node: types.TraceCausalProjectionNode{Subject: "self-1", StateKind: "s_sleep", ImpactMS: 800.0}},
		},
	}
	if cell := runtimeTraceProjCompareTargetSymptomCell(model, true); cell != "800.000ms" {
		t.Fatalf("a state-view symptom cell stays the bare F1 caliber: %q", cell)
	}
}

func TestP0A2TargetSymptomCellDashWhenNoSleep(t *testing.T) {
	model := runtimeTraceProjTreeModel{Target: "self-1", WindowMS: 2000.0}
	if cell := runtimeTraceProjCompareTargetSymptomCell(model, true); cell != "—" {
		t.Fatalf("no symptom and no hop-only sleep must render the dash: %q", cell)
	}
}
