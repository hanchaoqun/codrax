package types

import (
	"strings"
	"testing"
)

func testTraceAnswerRelationAuthorities(t *testing.T) []AnswerRelationAuthority {
	t.Helper()
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
		SelfRunnableTwoRulerAccountings: []TraceCausalProjectionSelfRunnableTwoRuler{{
			Subject: "target-7", WallRanks: []int{4, 13}, WallEffsMS: []float64{3.956, 1.193},
			WallSubtotalMS: 5.149, EdgeRanks: []int{10}, EdgeEffsMS: []float64{1.648}, EdgeSubtotalMS: 1.648,
		}},
		TargetStateAccount: &TraceCausalProjectionTargetStateAccount{Subject: "target-7", RunningMS: 1, RunnableMS: 2, SleepMS: 3, DStateMS: 4, IOWaitMS: 5, TotalMS: 15},
	}}}
	authorities := CompileTraceAnswerRelationAuthorities(set)
	if len(authorities) != 4 {
		t.Fatalf("authority count=%d, want 4: %+v", len(authorities), authorities)
	}
	return authorities
}

func claimsFromRequiredAuthorities(authorities []AnswerRelationAuthority) []AnswerRelationClaim {
	var claims []AnswerRelationClaim
	for _, authority := range authorities {
		if !authority.RequiredForClosure {
			continue
		}
		members := authority.MemberRefs
		if authority.Kind == AnswerRelationAuthorityCrossRulerBoundary {
			members = append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)
		}
		claims = append(claims, AnswerRelationClaim{
			AuthorityID: authority.ID, MemberRefs: members,
			PhysicalRelation: authority.PhysicalRelation, Addition: authority.Addition,
			SubtotalValue: authority.SubtotalValue, SubtotalUnit: authority.SubtotalUnit,
		})
	}
	return claims
}

func TestValidateAnswerRelationClaimsPinsTwoRulerArithmeticWithoutProse(t *testing.T) {
	authorities := testTraceAnswerRelationAuthorities(t)
	claims := claimsFromRequiredAuthorities(authorities)
	if err := ValidateAnswerRelationClaims(claims, authorities, true); err != nil {
		t.Fatalf("valid claims rejected: %v", err)
	}
	if len(claims) != 4 {
		t.Fatalf("required claims=%d, want 4", len(claims))
	}

	wrong := CloneAnswerRelationClaims(claims)
	for i := range wrong {
		if strings.HasSuffix(wrong[i].AuthorityID, ":cross") {
			wrong[i].Addition = AnswerRelationAdditionAuthorized
			value := 5.604
			wrong[i].SubtotalValue = &value
			wrong[i].SubtotalUnit = "ms"
		}
	}
	if err := ValidateAnswerRelationClaims(wrong, authorities, true); err == nil || !strings.Contains(err.Error(), "requires \"forbidden\"") {
		t.Fatalf("cross-ruler addition must be rejected precisely, got %v", err)
	}

	if err := ValidateAnswerRelationClaims(claims[:2], authorities, true); err == nil || !strings.Contains(err.Error(), "missing required") {
		t.Fatalf("omitted cross authority must block closure, got %v", err)
	}
	duplicate := CloneAnswerRelationClaims(claims)
	duplicate[0].MemberRefs = append(duplicate[0].MemberRefs, duplicate[0].MemberRefs[0])
	if err := ValidateAnswerRelationClaims(duplicate, authorities, true); err == nil || !strings.Contains(err.Error(), "exact member set") {
		t.Fatalf("duplicate member refs must not be silently normalized, got %v", err)
	}
}

func TestValidateAnswerRelationClaimsReportsIndependentViolationsInOneReject(t *testing.T) {
	authorities := testTraceAnswerRelationAuthorities(t)
	claims := claimsFromRequiredAuthorities(authorities)

	claims[0].PhysicalRelation = AnswerPhysicalRelationOverlap
	claims[0].Addition = AnswerRelationAdditionForbidden
	claims[0].MemberRefs = []string{"not-a-member"}
	claims[0].SubtotalValue = nil
	claims[0].SubtotalUnit = "us"

	claims[2].Addition = AnswerRelationAdditionAuthorized
	crossSubtotal := 6.797
	claims[2].SubtotalValue = &crossSubtotal
	claims[2].SubtotalUnit = "ms"

	claims = append(claims[:3], AnswerRelationClaim{AuthorityID: "trace:unknown"})
	err := ValidateAnswerRelationClaims(claims, authorities, true)
	if err == nil {
		t.Fatal("multiple invalid claims unexpectedly accepted")
	}
	message := err.Error()
	for _, want := range []string{
		"failed with 9 violation(s)",
		"physical_relation=\"overlap\"; typed authority requires \"unresolved\"",
		"addition=\"forbidden\"; typed authority requires \"authorized_to_published_subtotal\"",
		"member_refs=[not-a-member]; typed authority requires the exact member set [#4 #13]",
		"subtotal_value=<absent>; typed published subtotal is 5.149000",
		"subtotal_unit=\"us\"; typed authority requires \"ms\"",
		"subtotal_value=6.797000 subtotal_unit=\"ms\"; typed cross-ruler authority requires subtotal_value=<absent> subtotal_unit=\"\"",
		"authority_id=\"trace:unknown\" has no typed relation authority",
		"missing required model-authored relation claim for authority_id=\"trace:target_state_partition:",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("aggregate reject missing %q:\n%s", want, message)
		}
	}
}

func TestPartitionAnswerRelationClaimsByCurrentAuthoritiesWithdrawsStaleSnapshot(t *testing.T) {
	subtotal := 10.0
	authorities := []AnswerRelationAuthority{{
		ID: "trace:target_state_partition:current", Kind: AnswerRelationAuthorityClosedPartition,
		MemberRefs:       []string{"running", "runnable", "sleep", "d_state", "io_wait"},
		PhysicalRelation: AnswerPhysicalRelationMutuallyExclusive,
		Addition:         AnswerRelationAdditionAuthorized, SubtotalValue: &subtotal,
		SubtotalUnit: "ms", RequiredForClosure: true,
	}}
	current := AnswerRelationClaim{
		AuthorityID: authorities[0].ID, MemberRefs: append([]string(nil), authorities[0].MemberRefs...),
		PhysicalRelation: authorities[0].PhysicalRelation, Addition: authorities[0].Addition,
		SubtotalValue: authorities[0].SubtotalValue, SubtotalUnit: authorities[0].SubtotalUnit,
	}
	oldSubtotal := 15.0
	stale := current
	stale.AuthorityID = "trace:target_state_partition:explore-window"
	stale.SubtotalValue = &oldSubtotal

	kept, withdrawn := PartitionAnswerRelationClaimsByCurrentAuthorities(
		[]AnswerRelationClaim{stale, current}, authorities,
	)
	if !AnswerRelationClaimsEqual(kept, []AnswerRelationClaim{current}) {
		t.Fatalf("current claim partition mismatch: %+v", kept)
	}
	if !AnswerRelationClaimsEqual(withdrawn, []AnswerRelationClaim{stale}) {
		t.Fatalf("superseded claim partition mismatch: %+v", withdrawn)
	}
}

func TestTraceTargetStatePartitionRequiresExactClosedFiveLaneAccount(t *testing.T) {
	projection := TraceCausalProjection{TargetStateAccount: &TraceCausalProjectionTargetStateAccount{
		Subject: "target-7", RunningMS: 1, RunnableMS: 2, SleepMS: 3, DStateMS: 4, IOWaitMS: 5, TotalMS: 15,
	}}
	got := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{projection}})
	if len(got) != 1 || !got[0].RequiredForClosure || got[0].Kind != AnswerRelationAuthorityClosedPartition ||
		got[0].PhysicalRelation != AnswerPhysicalRelationMutuallyExclusive || got[0].Addition != AnswerRelationAdditionAuthorized {
		t.Fatalf("closed target partition did not become required dual-axis authority: %+v", got)
	}
	bad := projection
	copyAccount := *projection.TargetStateAccount
	copyAccount.TotalMS = 14
	bad.TargetStateAccount = &copyAccount
	if invalid := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{bad}}); len(invalid) != 0 {
		t.Fatalf("imbalanced target state account must fail closed: %+v", invalid)
	}
	windowed := projection
	windowed.WindowStartTs, windowed.WindowEndTs = 10, 10.015
	copyAccount = *projection.TargetStateAccount
	copyAccount.WindowStartTs, copyAccount.WindowEndTs = 10, 10.015
	windowed.TargetStateAccount = &copyAccount
	if exact := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{windowed}}); len(exact) != 1 {
		t.Fatalf("exact selected-window partition rejected: %+v", exact)
	}
	// The projection compiler admits a typed account whose endpoints are within
	// the shared F-2 ±1ms same-window tolerance. The relation compiler must not
	// then compare the account subtotal against the slightly different anchor
	// duration and retire an authority that was valid in the compact preview.
	// This is the production shape where an end sched_switch extends the account
	// by 20µs beyond the originally requested anchor.
	tolerated := windowed
	copyAccount = *projection.TargetStateAccount
	copyAccount.WindowStartTs, copyAccount.WindowEndTs = 10, 10.015020
	copyAccount.TotalMS = 15.020
	copyAccount.SleepMS = 3.020
	tolerated.TargetStateAccount = &copyAccount
	toleratedAuthority := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{tolerated}})
	if len(toleratedAuthority) != 1 {
		t.Fatalf("same-window tolerated account must retain its closed-partition authority: %+v", toleratedAuthority)
	}
	preview := TraceCausalProjection{TargetStateAccount: &copyAccount}
	previewAuthority := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{preview}})
	if len(previewAuthority) != 1 || previewAuthority[0].ID != toleratedAuthority[0].ID {
		t.Fatalf("preview/final authority identity drifted: preview=%+v final=%+v", previewAuthority, toleratedAuthority)
	}
	copyAccount.WindowEndTs = 10.017
	copyAccount.TotalMS = 17
	copyAccount.SleepMS = 5
	windowed.TargetStateAccount = &copyAccount
	if drift := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{windowed}}); len(drift) != 0 {
		t.Fatalf("different-window target partition must fail closed: %+v", drift)
	}
}

func TestCompileTraceAnswerRelationAuthoritiesUsesContentStableIDs(t *testing.T) {
	first := testTraceAnswerRelationAuthorities(t)
	second := testTraceAnswerRelationAuthorities(t)
	for i := range first {
		if first[i].ID != second[i].ID {
			t.Fatalf("authority id drift at %d: %q != %q", i, first[i].ID, second[i].ID)
		}
	}
	if !strings.HasPrefix(first[0].ID, "trace:self_runnable_two_ruler:") || !strings.HasSuffix(first[0].ID, ":wall") {
		t.Fatalf("unexpected content-stable authority id %q", first[0].ID)
	}
}

func TestTraceCausalProjectionSelfRunnableTwoRulerRecordFeedsRelationAuthority(t *testing.T) {
	record := ObservationRecord{
		Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard,
		Subject: "target-7", Predicate: "self_runnable_two_ruler",
		RichNotes: []string{
			TraceNoteKeySelfTwoRulerWallEffs + "=3.956,1.193",
			TraceNoteKeySelfTwoRulerWallRanks + "=4,13",
			TraceNoteKeySelfTwoRulerWallSubtotal + "=5.149",
			TraceNoteKeySelfTwoRulerEdgeEffs + "=1.648",
			TraceNoteKeySelfTwoRulerEdgeRanks + "=10",
			TraceNoteKeySelfTwoRulerEdgeSubtotal + "=1.648",
		},
	}
	accounting, ok := traceCausalProjectionSelfRunnableTwoRulerFromRecord(record)
	if !ok {
		t.Fatalf("valid producer record did not parse: %+v", record)
	}
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{SelfRunnableTwoRulerAccountings: []TraceCausalProjectionSelfRunnableTwoRuler{accounting}}}}
	if got := CompileTraceAnswerRelationAuthorities(set); len(got) != 3 {
		t.Fatalf("record authority count=%d, want 3: %+v", len(got), got)
	}
}

func TestTraceSameSourcePartitionRequiresExactVisibleTypedPair(t *testing.T) {
	base := TraceCausalProjectionNode{
		Subject: "CompThread_0-2955", Object: "d_state_or_io_wait",
		LineStart: 2261, LineEnd: 25863, RankBoardTarget: "target-17267",
		RankBoardParamsFingerprint: "board-a", QueryWindowStartTs: 10, QueryWindowEndTs: 10.2,
		ChainAnchoredMS: 3.598000001, ChainAnchorFullMS: 36.757000002,
		EffectiveImpactPublished: true,
	}
	anchored := base
	anchored.Rank, anchored.ChainRelevance, anchored.EffectiveImpactMS = 6, "on_chain", 3.598000001
	remainder := base
	remainder.Rank, remainder.ChainRelevance, remainder.EffectiveImpactMS = 1, "adjacent", 33.159000001
	remainder.ChainAnchorRemainderSeat = true
	set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{RankedSeats: []TraceCausalProjectionNode{anchored, remainder}}}}
	got := CompileTraceAnswerRelationAuthorities(set)
	if len(got) != 1 || got[0].Kind != AnswerRelationAuthoritySameSourcePartition ||
		got[0].PhysicalRelation != AnswerPhysicalRelationMutuallyExclusive ||
		got[0].Addition != AnswerRelationAdditionAuthorized || !got[0].RequiredForClosure ||
		got[0].SubtotalValue == nil || *got[0].SubtotalValue != 36.757 || len(got[0].MemberRefs) != 2 {
		t.Fatalf("exact same-source split did not become relation authority: %+v", got)
	}
	claims := claimsFromRequiredAuthorities(got)
	if err := ValidateAnswerRelationClaims(claims, got, true); err != nil {
		t.Fatalf("valid same-source split claim rejected: %v", err)
	}

	for name, mutate := range map[string]func(*TraceCausalProjectionNode){
		"missing twin":              func(node *TraceCausalProjectionNode) {},
		"wrong published remainder": func(node *TraceCausalProjectionNode) { node.EffectiveImpactMS = 30 },
		"ownership divergent":       func(node *TraceCausalProjectionNode) { node.ChainAnchorOwnershipDivergent = true },
		"cross board":               func(node *TraceCausalProjectionNode) { node.RankBoardParamsFingerprint = "board-b" },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := remainder
			mutate(&candidate)
			seats := []TraceCausalProjectionNode{anchored, candidate}
			if name == "missing twin" {
				seats = seats[:1]
			}
			invalid := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{{RankedSeats: seats}}})
			if len(invalid) != 0 {
				t.Fatalf("invalid pair must fail closed: %+v", invalid)
			}
		})
	}
}

func TestTraceOverlappingMembersCompileOneTypedOptionalBoundary(t *testing.T) {
	inside := true
	base := TraceCausalProjectionNode{
		Object: "runnable_wait", EffectiveImpactPublished: true,
		FixDirection: "lock_priority", ChainRelevance: "on_chain",
		QueryWindowStartTs: 10, QueryWindowEndTs: 10.2,
		RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.2,
		RankBoardTarget: "target-7", RankBoardParamsFingerprint: "board-a",
		WithinRequestedWindow: &inside,
	}
	left := base
	left.Rank, left.Subject, left.EffectiveImpactMS = 1, "worker-a-11", 23.9944
	left.StartTs, left.EndTs, left.LineStart, left.LineEnd = 10.010, 10.110, 10, 20
	right := base
	right.Rank, right.Subject, right.EffectiveImpactMS = 2, "worker-b-12", 19.0414
	right.StartTs, right.EndTs, right.LineStart, right.LineEnd = 10.020, 10.120, 30, 40
	differentDirection := base
	differentDirection.Rank, differentDirection.Subject, differentDirection.EffectiveImpactMS = 3, "worker-c-13", 7
	differentDirection.StartTs, differentDirection.EndTs = 10.030, 10.090
	differentDirection.FixDirection = "io_dependency"
	disjoint := base
	disjoint.Rank, disjoint.Subject, disjoint.EffectiveImpactMS = 4, "worker-d-14", 6
	disjoint.StartTs, disjoint.EndTs = 10.130, 10.150
	differentLane := base
	differentLane.Rank, differentLane.Subject, differentLane.EffectiveImpactMS = 5, "worker-e-15", 5
	differentLane.StartTs, differentLane.EndTs = 10.030, 10.080
	differentLane.ChainRelevance = "adjacent"

	projection := TraceCausalProjection{
		WindowStartTs: 10, WindowEndTs: 10.2,
		RankedSeats: []TraceCausalProjectionNode{left, right, differentDirection, disjoint, differentLane},
	}
	authorities := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{projection}})
	var got []AnswerRelationAuthority
	for _, authority := range authorities {
		if authority.Kind == AnswerRelationAuthorityOverlappingMembers {
			got = append(got, authority)
		}
	}
	if len(got) != 1 {
		t.Fatalf("overlap authorities=%d, want 1: %+v", len(got), got)
	}
	authority := got[0]
	if authority.RequiredForClosure || authority.PhysicalRelation != AnswerPhysicalRelationOverlap ||
		authority.Addition != AnswerRelationAdditionForbidden || len(authority.MemberRefs) != 2 ||
		len(authority.MemberValuesMS) != 2 || authority.MemberValuesMS[0] != 23.994 || authority.MemberValuesMS[1] != 19.041 ||
		authority.MeasuredOverlapMS == nil || *authority.MeasuredOverlapMS != 90 ||
		authority.ComparisonValueMS == nil || *authority.ComparisonValueMS != 23.994 ||
		authority.ComparisonRule != "max_member_only_no_subtotal" ||
		authority.FixDirection != "lock_priority" || authority.ChainLane != "on_chain" {
		t.Fatalf("overlap authority drifted: %+v", authority)
	}
	claim := AnswerRelationClaim{
		AuthorityID: authority.ID, MemberRefs: append([]string(nil), authority.MemberRefs...),
		PhysicalRelation: authority.PhysicalRelation, Addition: authority.Addition,
	}
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claim}, authorities, false); err != nil {
		t.Fatalf("exact optional overlap claim rejected: %v", err)
	}
	claim.Addition = AnswerRelationAdditionAuthorized
	if err := ValidateAnswerRelationClaims([]AnswerRelationClaim{claim}, authorities, false); err == nil {
		t.Fatal("overlap addition drift was accepted")
	}
	if err := ValidateAnswerRelationClaims(nil, authorities, false); err != nil {
		t.Fatalf("omitted optional overlap JSON must not trigger a format retry: %v", err)
	}
}

func TestTraceOverlappingMembersFailClosedWithoutExactPairIdentity(t *testing.T) {
	base := TraceCausalProjectionNode{
		Object: "runnable_wait", Rank: 1, Subject: "worker-a-11",
		EffectiveImpactMS: 10, EffectiveImpactPublished: true,
		FixDirection: "lock_priority", ChainRelevance: "on_chain",
		StartTs: 10.010, EndTs: 10.110,
		QueryWindowStartTs: 10, QueryWindowEndTs: 10.2,
		RankQueryWindowStartTs: 10, RankQueryWindowEndTs: 10.2,
		RankBoardTarget: "target-7", RankBoardParamsFingerprint: "board-a",
	}
	peer := base
	peer.Rank, peer.Subject, peer.StartTs, peer.EndTs = 2, "worker-b-12", 10.020, 10.120
	for name, mutate := range map[string]func(*TraceCausalProjectionNode){
		"missing board params": func(node *TraceCausalProjectionNode) { node.RankBoardParamsFingerprint = "" },
		"different board":      func(node *TraceCausalProjectionNode) { node.RankBoardParamsFingerprint = "board-b" },
		"missing direction":    func(node *TraceCausalProjectionNode) { node.FixDirection = "" },
		"different lane":       func(node *TraceCausalProjectionNode) { node.ChainRelevance = "adjacent" },
		"touch only":           func(node *TraceCausalProjectionNode) { node.StartTs, node.EndTs = 10.110, 10.120 },
		"merged envelope":      func(node *TraceCausalProjectionNode) { node.MergedCount = 2 },
		"unpublished value":    func(node *TraceCausalProjectionNode) { node.EffectiveImpactPublished = false },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := peer
			mutate(&candidate)
			got := CompileTraceAnswerRelationAuthorities(TraceCausalProjectionSet{Projections: []TraceCausalProjection{{
				WindowStartTs: 10, WindowEndTs: 10.2, RankedSeats: []TraceCausalProjectionNode{base, candidate},
			}}})
			for _, authority := range got {
				if authority.Kind == AnswerRelationAuthorityOverlappingMembers {
					t.Fatalf("inexact pair minted overlap authority: %+v", authority)
				}
			}
		})
	}
}
