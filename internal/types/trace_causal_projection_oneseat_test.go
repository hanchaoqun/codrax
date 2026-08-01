package types

import (
	"strings"
	"testing"
)

// v5 P1 件① B.2 一段一席 per-carrier engine pins (2026-07-13).
//
// Retirement-contract discipline (§29.56 遗留头位): every display transition
// arm retires only after the ENGINE provably covers its carriers — each pin
// below is one carrier. Red→green protocol: the arm-A/B/C pins were run RED
// against the hook-less aggregation (arms detached from
// traceCausalProjectionAggregateForPresentation) before the hooks landed;
// the R1-coverage pin documents pre-existing engine coverage (勘正①: the
// batch map's "engine misses the equality twin" premise was wrong for the
// VALUED shape — R1 already converged it at HEAD cd2ca239).

func oneSeatSeatCount(projection TraceCausalProjection, subject string) (int, []TraceCausalProjectionNode) {
	key := traceCausalProjectionCanonicalNode(subject)
	seen := map[string]bool{}
	var seats []TraceCausalProjectionNode
	for _, bucket := range [][]TraceCausalProjectionNode{
		projection.PrimaryRootCauses, projection.OnChainCauses, projection.SupportingHops,
		projection.AdjacentCauses, projection.BackgroundCauses,
	} {
		for _, node := range bucket {
			if traceCausalProjectionCanonicalNode(node.Subject) != key {
				continue
			}
			id := traceCausalProjectionCanonicalNode(node.EvidenceID)
			if seen[id] {
				continue // a survivor's own cross-bucket copy is one seat
			}
			seen[id] = true
			seats = append(seats, node)
		}
	}
	return len(seats), seats
}

// --- Carrier: exact cross-publication scheduler-state account (B7-T2).

func stateAccountOneSeatNode(id, predicate, key string, rank, lineStart, lineEnd int) TraceCausalProjectionNode {
	return TraceCausalProjectionNode{
		EvidenceID:        id,
		Subject:           "app-20",
		Predicate:         predicate,
		Object:            "runnable_wait",
		StateKind:         "runnable",
		StateAccountKey:   key,
		Rank:              rank,
		ImpactMS:          5,
		EffectiveImpactMS: 5,
		LineStart:         lineStart,
		LineEnd:           lineEnd,
	}
}

func TestOneSeatExactStateAccountConvergesAcrossDifferentLineEnvelopes(t *testing.T) {
	const key = "state_account:v1:exact-segment-inventory"
	rank := stateAccountOneSeatNode("E-rank", "root_cause_primary", key, 1, 4, 23)
	impact := stateAccountOneSeatNode("E-impact", "wakeup_causal_impact", key, 0, 3, 23)
	projection := TraceCausalProjection{
		PrimaryRootCauses: []TraceCausalProjectionNode{rank},
		OnChainCauses:     []TraceCausalProjectionNode{impact},
	}

	traceCausalProjectionAggregateForPresentation(&projection)
	count, seats := oneSeatSeatCount(projection, "app-20")
	if count != 1 || seats[0].EvidenceID != "E-rank" || seats[0].Rank != 1 {
		t.Fatalf("exact account must keep the rank seat across differing envelopes, got %d: %+v", count, seats)
	}
	if !strings.Contains(strings.Join(seats[0].MergedEvidenceIDs, ","), "E-impact") {
		t.Fatalf("absorbed impact evidence must remain auditable: %+v", seats[0])
	}
}

func TestOneSeatStateAccountNeverFallsBackToEqualValueOrEnvelope(t *testing.T) {
	rank := stateAccountOneSeatNode("E-rank", "root_cause_primary", "state_account:v1:a", 1, 4, 23)
	impact := stateAccountOneSeatNode("E-impact", "wakeup_causal_impact", "state_account:v1:b", 0, 4, 23)
	projection := TraceCausalProjection{
		PrimaryRootCauses: []TraceCausalProjectionNode{rank},
		OnChainCauses:     []TraceCausalProjectionNode{impact},
	}

	traceCausalProjectionConvergeStateAccountPublications(&projection)
	count, seats := oneSeatSeatCount(projection, "app-20")
	if count != 2 {
		t.Fatalf("equal value/envelope with different typed identities must fail open, got %d: %+v", count, seats)
	}
}

func TestOneSeatStateAccountAmbiguityFailsOpen(t *testing.T) {
	const key = "state_account:v1:ambiguous"
	rank := stateAccountOneSeatNode("E-rank", "root_cause_primary", key, 1, 4, 23)
	impactA := stateAccountOneSeatNode("E-impact-a", "wakeup_causal_impact", key, 0, 3, 23)
	impactB := stateAccountOneSeatNode("E-impact-b", "wakeup_causal_impact", key, 0, 2, 23)
	projection := TraceCausalProjection{
		PrimaryRootCauses: []TraceCausalProjectionNode{rank},
		OnChainCauses:     []TraceCausalProjectionNode{impactA, impactB},
	}

	traceCausalProjectionConvergeStateAccountPublications(&projection)
	count, seats := oneSeatSeatCount(projection, "app-20")
	if count != 3 {
		t.Fatalf("one-to-many account publication must remain visible, got %d: %+v", count, seats)
	}
}

// --- Carrier: valued equality twin (14704 archetype) — R1 pre-existing cover.

func oneSeatEqualityRecords(rawValue string) []ObservationRecord {
	return []ObservationRecord{
		{
			ID:              "trace_query:t#wakeup_causal_impact:1",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Span:            ObservationSpan{LineStart: 101, LineEnd: 101, StartTs: 2942.245, EndTs: 2942.300},
			ClaimKey:        "wakeup_causal_impact:[GT]ColdPool#6-36644",
			Subject:         "[GT]ColdPool#6-36644",
			Predicate:       "wakeup_causal_impact",
			Object:          "running",
			Value:           "54.599",
			Unit:            "ms",
			RichNotes: []string{
				"causality=on_wakeup_chain", "dominant_state=running",
				"total=54.599", "target_impact=54.599",
				"selected_window=2942.245..2942.300",
			},
			Confidence: 0.78,
		},
		{
			ID:              "trace_query:t#root_evidence:1",
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			Span:            ObservationSpan{LineStart: 101, LineEnd: 101},
			ClaimKey:        "root_evidence:running",
			Subject:         "[GT]ColdPool#6-36644",
			Predicate:       "running",
			Value:           rawValue,
			Unit:            "ms",
			RichNotes:       []string{"tier=context_only", "effective_impact_ms=0.000"},
			Confidence:      0.75,
		},
	}
}

// 勘正① pin (records level, production-exact emission shapes): the VALUED
// equality twin already converges through R1 — this pin freezes that
// pre-existing coverage so the display equality arm's retirement can cite it.
func TestOneSeatValuedEqualityTwinConvergesViaR1(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords(oneSeatEqualityRecords("54.599"))
	count, seats := oneSeatSeatCount(projection, "[GT]ColdPool#6-36644")
	if count != 1 {
		t.Fatalf("one physical segment must hold one engine seat, got %d: %+v", count, seats)
	}
	joined := strings.Join(seats[0].MergedEvidenceIDs, ",")
	if !strings.Contains(joined, "root_evidence:1") {
		t.Fatalf("the raw copy's evidence id must ride the survivor (信息守恒): %q", joined)
	}
}

// --- Carrier: VALUELESS raw twin (arm A; R1's value-keyed identity cannot
// reach it — this pin was RED before the arm-A hook landed).
func TestOneSeatValuelessRawTwinConverges(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords(oneSeatEqualityRecords("0.000"))
	count, seats := oneSeatSeatCount(projection, "[GT]ColdPool#6-36644")
	if count != 1 {
		t.Fatalf("a valueless raw copy of the same segment must fold (arm A), got %d seats: %+v", count, seats)
	}
	joined := strings.Join(seats[0].MergedEvidenceIDs, ",")
	if !strings.Contains(joined, "root_evidence:1") {
		t.Fatalf("the valueless raw copy's evidence id must ride the survivor: %q", joined)
	}
	if seats[0].ImpactMS != 54.599 {
		t.Fatalf("吸收=佩记号,数值不重计: %v", seats[0].ImpactMS)
	}
}

// --- Carrier: keeper valued on the EFFECTIVE lane only (arm A; R1 keys on
// Impact/Cumulative and misses; RED before the hook).
func TestOneSeatEffOnlyKeeperRawTwinConverges(t *testing.T) {
	records := oneSeatEqualityRecords("54.599")
	records[0].Value = "0.000"
	records[0].RichNotes = []string{
		"causality=on_wakeup_chain", "dominant_state=running",
		"effective_impact_ms=54.599",
		"selected_window=2942.245..2942.300",
	}
	projection := TraceCausalProjectionFromObservationRecords(records)
	count, seats := oneSeatSeatCount(projection, "[GT]ColdPool#6-36644")
	if count != 1 {
		t.Fatalf("an eff-lane-valued keeper must absorb its µs-equal raw twin, got %d seats: %+v", count, seats)
	}
	if seats[0].StateKind != "running" {
		t.Fatalf("the keeper (highest word lane) must hold the seat: %+v", seats[0])
	}
}

// Fail-open control (display-gate mirror): a differing raw value is a
// different account and never folds — 宁两行勿假并.
func TestOneSeatEqualityFailsOpenOnValueMismatch(t *testing.T) {
	projection := TraceCausalProjectionFromObservationRecords(oneSeatEqualityRecords("51.000"))
	count, _ := oneSeatSeatCount(projection, "[GT]ColdPool#6-36644")
	if count != 2 {
		t.Fatalf("value-mismatched lanes must stay two seats, got %d", count)
	}
}

// Fail-open control: a raw whose state token contradicts the keeper's own
// state word is a different view and never folds.
func TestOneSeatEqualityFailsOpenOnStateMismatch(t *testing.T) {
	records := oneSeatEqualityRecords("0.000")
	records[1].Predicate = "sleep_wait"
	records[1].ClaimKey = "root_evidence:sleep_wait"
	projection := TraceCausalProjectionFromObservationRecords(records)
	count, _ := oneSeatSeatCount(projection, "[GT]ColdPool#6-36644")
	if count != 2 {
		t.Fatalf("state-word-contradicting raws must stay seated, got %d", count)
	}
}

// --- Carrier: raw member RE-ISSUES of an ×N seat (arm B, 25846 archetype;
// RED before the hook). Aggregation-level fixture: the ×3 keeper is the R2
// engine mint (SUM + extrema), the raws are production root_evidence shapes.
func oneSeatMemberProjection() TraceCausalProjection {
	self := "T7@ZeusThreadPo-61839"
	raw := func(id string, ms float64, line int) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:" + id,
			Subject: self, Predicate: "sleep_wait",
			Tier: TraceCausalTierContextOnly, ChainRelevance: "on_chain",
			ImpactMS: ms, CumulativeImpactMS: ms, Confidence: 0.75,
			LineStart: line, LineEnd: line + 50,
		}
	}
	return TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#state_drilldown:1",
				Subject: self, Object: "sleep_wait", StateKind: "s_sleep",
				ChainRelevance: "on_chain",
				ImpactMS:       105.794, CumulativeImpactMS: 105.794,
				MergedCount: 3, MergedMinMS: 8.879, MergedMaxMS: 80.751, MergedSumMS: 105.794,
				Confidence: 0.9, LineStart: 100, LineEnd: 900},
			raw("1", 80.751, 200),
			raw("2", 16.164, 400),
			raw("3", 8.879, 600),
		},
	}
}

func TestOneSeatMemberReissuesConverge(t *testing.T) {
	projection := oneSeatMemberProjection()
	traceCausalProjectionAggregateForPresentation(&projection)
	count, seats := oneSeatSeatCount(projection, "T7@ZeusThreadPo-61839")
	if count != 1 {
		t.Fatalf("member re-issues must fold into the ×N seat (arm B), got %d seats: %+v", count, seats)
	}
	keeper := seats[0]
	if keeper.MergedCount != 3 || keeper.ImpactMS != 105.794 {
		t.Fatalf("吸收=佩记号: MergedCount/account must stay byte-identical, got %+v", keeper)
	}
	joined := strings.Join(keeper.MergedEvidenceIDs, ",")
	for _, id := range []string{"root_evidence:1", "root_evidence:2", "root_evidence:3"} {
		if !strings.Contains(joined, id) {
			t.Fatalf("re-issue %s must ride the keeper's evidence union: %q", id, joined)
		}
	}
}

// Fail-open control: a raw whose value matches NO derivable member keeps its
// own honest seat (µs 恒等才算成员).
func TestOneSeatMemberNonMemberValueStaysSeated(t *testing.T) {
	projection := oneSeatMemberProjection()
	projection.OnChainCauses[2].ImpactMS = 17.000
	projection.OnChainCauses[2].CumulativeImpactMS = 17.000
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "T7@ZeusThreadPo-61839")
	if count != 2 {
		t.Fatalf("non-member raw copies never fold, got %d seats", count)
	}
}

// 诚实席红线: missing_wakeup / trace_gap raws carry the ◌/⊘链止 honesty
// semantics and NEVER converge at the engine.
func TestOneSeatHonestySeatsNeverFold(t *testing.T) {
	projection := oneSeatMemberProjection()
	projection.OnChainCauses[1].Predicate = "missing_wakeup"
	projection.OnChainCauses[1].UndrillableReason = "missing_wakeup"
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "T7@ZeusThreadPo-61839")
	if count != 2 {
		t.Fatalf("the ◌ honesty seat must stay (raws 2/3 fold, missing_wakeup stays), got %d seats", count)
	}
}

// --- Carrier: post-R2 merged-twin seats, single-side token absence (arm C,
// WO-D3 witnesses 42729 E9/E15 · 62930 E9/E19; RED before the hook). One
// physical ×3 segment set published on the critical (tokened) lane and the
// raw (token-less) lane — identical member fingerprints. Production shape
// detail (why R1 never catches the pair): the two lanes carve DIFFERENT
// line envelopes for the same segments (that is exactly how the SINGLE rows
// escaped R1 before each lane R2-merged), so the merged twins differ on the
// span while agreeing on subject + count + display + extrema + window.
func oneSeatMergedTwinProjection() TraceCausalProjection {
	subject := "LogThread-42729"
	return TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#critical_blocking:2",
				Subject: subject, Object: "unknown-thread", TypeToken: "d_state_or_io_wait",
				StateKind: "d_sleep", ChainRelevance: "on_chain",
				ImpactMS: 13.418, CumulativeImpactMS: 13.418,
				MergedCount: 3, MergedMinMS: 4.265, MergedMaxMS: 4.884, MergedSumMS: 13.418,
				Confidence: 0.85, LineStart: 300, LineEnd: 890},
		},
		BackgroundCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:9",
				Subject: subject, Object: "",
				ImpactMS: 13.418, CumulativeImpactMS: 13.418, ChainRelevance: "background",
				MergedCount: 3, MergedMinMS: 4.265, MergedMaxMS: 4.884, MergedSumMS: 13.418,
				Confidence: 0.75, LineStart: 312, LineEnd: 905},
		},
	}
}

func TestOneSeatMergedTwinTokenForkConverges(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	traceCausalProjectionAggregateForPresentation(&projection)
	count, seats := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 1 {
		t.Fatalf("one physical segment set must hold one seat (arm C), got %d: %+v", count, seats)
	}
	keeper := seats[0]
	if traceCausalProjectionCanonicalNode(keeper.TypeToken) != "d_state_or_io_wait" {
		t.Fatalf("词位取最高车道: the typed-token side keeps the seat, got %+v", keeper)
	}
	if keeper.MergedCount != 3 {
		t.Fatalf("×N stays ×3, never re-summed into ×6 (vnote 禁令): %d", keeper.MergedCount)
	}
	if keeper.DuplicatePublications < 2 {
		t.Fatalf("the twin publication must ride the existing disclosure lane: %d", keeper.DuplicatePublications)
	}
	if !strings.Contains(strings.Join(keeper.MergedEvidenceIDs, ","), "root_evidence:9") {
		t.Fatalf("the twin's evidence id must ride the survivor: %v", keeper.MergedEvidenceIDs)
	}
}

// Carrier: same-token cross-stanza ×N twins (arm C, S8-TPF 链/▒ 通道漂移
// shape; RED before the hook — the lanes carve different line envelopes, so
// R1 never relates the pair; a SAME-envelope pair is R1's existing coverage).
func TestOneSeatMergedTwinSameTokenCrossStanzaConverges(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	twin := projection.OnChainCauses[0]
	twin.EvidenceID = "trace_query:t2#critical_blocking:5"
	twin.ChainRelevance = "background"
	twin.LineStart, twin.LineEnd = 312, 905
	projection.BackgroundCauses = []TraceCausalProjectionNode{twin}
	traceCausalProjectionAggregateForPresentation(&projection)
	count, seats := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 1 {
		t.Fatalf("same-token cross-stanza twins must converge, got %d: %+v", count, seats)
	}
}

// W-A red line: diverging CUMULATIVE accounts are two account systems
// (§29.50.3 链聚合 vs 状态类互斥) — never folded by any arm.
func TestOneSeatMergedTwinDivergingCumulativeFailsOpen(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	projection.BackgroundCauses[0].CumulativeImpactMS = 20.412
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 2 {
		t.Fatalf("W-A 不同账目绝不折: diverging cumulative scopes stay two seats, got %d", count)
	}
}

// WO-D2 boundary: diverging published EFFECTIVE calibers (the trunk/flat
// 56643 shape) stay two rows — the display D2 fold owns the D4 dual-list
// disclosure until a typed dual-eff carrier exists.
func TestOneSeatMergedTwinDivergingEffFailsOpen(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	projection.OnChainCauses[0].EffectiveImpactMS = 9.612
	projection.BackgroundCauses[0].EffectiveImpactMS = 13.418
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 2 {
		t.Fatalf("diverging eff calibers are two accounts (WO-D2 territory), got %d seats", count)
	}
}

// ⌗ caliber-side guard: a single-side-token-absent pair whose PRESENT token
// resolves no duration-state family (composite/count calibers) fails open —
// the engine never folds across the 两把尺 boundary.
func TestOneSeatMergedTwinCompositeCaliberFailsOpen(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	projection.OnChainCauses[0].TypeToken = "block_io"
	projection.OnChainCauses[0].StateKind = ""
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 2 {
		t.Fatalf("⌗ caliber tokens must not fold with wall-clock lanes, got %d seats", count)
	}
}

// Rank guard: an adjudicated rank seat never re-seats through arm C.
func TestOneSeatMergedTwinRankSeatFailsOpen(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	projection.OnChainCauses[0].Rank = 2
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 2 {
		t.Fatalf("rank seats are adjudicated seating and never fold here, got %d seats", count)
	}
}

// 分区席相容 (§29.50.5): proof-partition sibling seats (cause 席 + 余数席)
// of ONE thread carry different values/identities by construction — the
// value gates exclude them; the partition is a proof partition of one
// segment set, never a duplicate seat.
func TestOneSeatPartitionSeatsNeverFold(t *testing.T) {
	subject := "ThreadPoolForeg-60555"
	projection := TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#root_cause_rank:1",
				Subject: subject, Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
				StateKind: "d_sleep", ChainRelevance: "on_chain", Rank: 1, Tier: "primary",
				ImpactMS: 7.386, CumulativeImpactMS: 7.386,
				BlockedReasonCaller: "fscache_page_wait_o",
				Confidence:          0.9, LineStart: 100, LineEnd: 400},
			{Role: TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#root_cause_rank:2",
				Subject: subject, Object: "d_state_or_io_wait", TypeToken: "d_state_or_io_wait",
				StateKind: "d_sleep", ChainRelevance: "on_chain", Rank: 2, Tier: "secondary",
				ImpactMS: 10.433, CumulativeImpactMS: 10.433,
				DStateCauseUnprovenRemainder: true,
				Confidence:                   0.9, LineStart: 401, LineEnd: 900},
		},
	}
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, subject)
	if count != 2 {
		t.Fatalf("partition seats (cause 席 + 余数席) must both survive, got %d", count)
	}
}

// 件1 (修复轮, 对抗复核 F1, 2026-07-13) — keeper 诚实席门 negative pin
// (复核探针形): a ◌ missing_wakeup honesty seat sharing the raw witness's
// subject + exact span must NOT keep the seat and must NOT get a scheduler
// state word back-filled — the pair stays two honest rows. 突变自检: removing
// the honesty leg of traceCausalProjectionOneSeatKeeperEligible reds this
// (keeper absorbs, StateKind flips to "running").
func TestOneSeatHonestyKeeperNeverAbsorbsRawTwin(t *testing.T) {
	subject := "CompThread_0-2955"
	projection := TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:4",
				Subject: subject, Predicate: "missing_wakeup",
				UndrillableReason: "missing_wakeup",
				Tier:              TraceCausalTierContextOnly, ChainRelevance: "on_chain",
				ImpactMS: 8.793, CumulativeImpactMS: 8.793,
				Confidence: 0.7, LineStart: 300, LineEnd: 340},
			// The valueless raw copy of the same span (the arm-A exclusive
			// shape — R1's value key cannot reach it).
			{Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:9",
				Subject: subject, Predicate: "running",
				Tier: TraceCausalTierContextOnly, ChainRelevance: "on_chain",
				Confidence: 0.75, LineStart: 300, LineEnd: 340},
		},
	}
	traceCausalProjectionAggregateForPresentation(&projection)
	count, seats := oneSeatSeatCount(projection, subject)
	if count != 2 {
		t.Fatalf("the ◌ honesty seat must never absorb a raw twin (件1 F1), got %d seats", count)
	}
	for _, seat := range seats {
		if seat.Undrillable() && seat.StateKind != "" {
			t.Fatalf("the honesty seat must not wear a back-filled state word: %+v", seat)
		}
	}
}

// 件2① (修复轮 F3, 2026-07-13) — 诚实 raw-lane 排除判别力 pin: a VALUELESS
// undrillable raw copy whose Predicate IS a registered state word matching
// the keeper's own — every other arm-A gate passes (twin key / valueless
// lane / state consistency / window), so ONLY the honesty exclusion blocks
// the fold. Valueless deliberately: the valued shape is R1's lane (R1
// carries no honesty gate — pre-batch behavior, see the ledger observation).
// 突变自检: removing the Undrillable() leg from
// traceCausalProjectionRawEvidenceLane reds this.
func TestOneSeatUndrillableRawNeverFolds(t *testing.T) {
	subject := "T7@ZeusThreadPo-61839"
	projection := TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#state_drilldown:1",
				Subject: subject, Object: "sleep_wait", StateKind: "sleep_wait",
				ChainRelevance: "on_chain",
				ImpactMS:       80.751, CumulativeImpactMS: 80.751,
				Confidence: 0.9, LineStart: 200, LineEnd: 250},
			{Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:2",
				Subject: subject, Predicate: "sleep_wait",
				UndrillableReason: "missing_wakeup",
				Tier:              TraceCausalTierContextOnly, ChainRelevance: "on_chain",
				Confidence: 0.7, LineStart: 200, LineEnd: 250},
		},
	}
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, subject)
	if count != 2 {
		t.Fatalf("an undrillable raw copy keeps its honesty seat (件2①), got %d seats", count)
	}
}

// 件2② (修复轮 F3) — 跨窗否决判别力 pin: a valueless raw twin measured in a
// DIFFERENT typed query window than its keeper never folds (two measurement
// scopes). Valueless deliberately — the valued shape would be R1's lane and
// R1 carries no window veto; only arm A's OneSeatWindowsCompatible blocks
// here. 突变自检: disabling the veto reds this.
func TestOneSeatCrossWindowRawTwinFailsOpen(t *testing.T) {
	records := oneSeatEqualityRecords("0.000")
	records[1].RichNotes = append(records[1].RichNotes, "selected_window=2942.100..2942.200")
	projection := TraceCausalProjectionFromObservationRecords(records)
	count, _ := oneSeatSeatCount(projection, "[GT]ColdPool#6-36644")
	if count != 2 {
		t.Fatalf("cross-window twins are two measurements and never one seat (件2②), got %d", count)
	}
}

// 件2③ (修复轮 F3) — arm C ≥3 副本 fail-open pin: THREE ×N seats with the
// identical segment-set fingerprint are ambiguity, not a twin pair — all
// three stay seated. 突变自检: relaxing the len(g)==2 gate reds this.
func TestOneSeatMergedTwinTripleCopyFailsOpen(t *testing.T) {
	projection := oneSeatMergedTwinProjection()
	third := projection.BackgroundCauses[0]
	third.EvidenceID = "trace_query:t3#root_evidence:12"
	third.LineStart, third.LineEnd = 320, 910
	projection.BackgroundCauses = append(projection.BackgroundCauses, third)
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, "LogThread-42729")
	if count != 3 {
		t.Fatalf("≥3 fingerprint copies are ambiguity and fail open (件2③), got %d seats", count)
	}
}

// 件4 佐证 pin (修复轮 F5, 2026-07-13): the display member arm's
// LEGACY-EXCLUSIVE carrier — raw re-issues whose Predicate is an
// UNREGISTERED causal token (d_state_or_io_wait, legacy records without the
// dominant_state note) beside a cross-refinement io keeper — is provably
// NOT foldable by engine arm B (no registered state word on the raw, token
// mismatch across the refinement, and the engine owns no registry copy).
// This is the typed boundary that keeps the display member arm honestly
// retained (its 退役条件 in the tree.go EVOLUTION RECORD).
func TestOneSeatLegacyCausalTokenRawStaysForDisplayArm(t *testing.T) {
	self := "census-thread-1"
	raw := func(id string, ms float64, line int) TraceCausalProjectionNode {
		return TraceCausalProjectionNode{
			Role: TraceCausalRoleCausalHop, EvidenceID: "trace_query:t#root_evidence:" + id,
			Subject: self, Predicate: "d_state_or_io_wait",
			Tier: TraceCausalTierContextOnly, ChainRelevance: "on_chain",
			ImpactMS: ms, CumulativeImpactMS: ms, Confidence: 0.75,
			LineStart: line, LineEnd: line + 40,
		}
	}
	projection := TraceCausalProjection{
		OnChainCauses: []TraceCausalProjectionNode{
			{Role: TraceCausalRoleRootCauseContext, EvidenceID: "trace_query:t#state_drilldown:1",
				Subject: self, Object: "io_wait", TypeToken: "io_wait", StateKind: "io_wait",
				ChainRelevance: "on_chain",
				ImpactMS:       60.000, CumulativeImpactMS: 60.000,
				MergedCount: 3, MergedMinMS: 10.000, MergedMaxMS: 30.000, MergedSumMS: 60.000,
				Confidence: 0.9, LineStart: 100, LineEnd: 900},
			raw("1", 10.000, 200),
			raw("2", 20.000, 400),
			raw("3", 30.000, 600),
		},
	}
	traceCausalProjectionAggregateForPresentation(&projection)
	count, _ := oneSeatSeatCount(projection, self)
	if count != 4 {
		t.Fatalf("the legacy causal-token raw shape must stay un-folded at the engine (display member arm's carrier), got %d seats", count)
	}
}
