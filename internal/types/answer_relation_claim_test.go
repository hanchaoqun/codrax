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
	copyAccount.WindowEndTs = 10.017
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
