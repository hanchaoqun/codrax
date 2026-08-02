package tool

import (
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func relationClaimPipelineFixture(t *testing.T) (*types.BusContext, []types.AnswerRelationClaim) {
	t.Helper()
	target := tracequery.ThreadRef{Comm: "target", PID: 7}
	rank := &tracequery.RootCauseRankResult{SelfRunnableTwoRuler: &tracequery.SelfRunnableTwoRulerAccounting{
		Thread:         target,
		WallSeats:      []tracequery.SelfRunnableTwoRulerSeat{{Rank: 4, EffMs: 3.956}, {Rank: 13, EffMs: 1.193}},
		EdgeSeats:      []tracequery.SelfRunnableTwoRulerSeat{{Rank: 10, EffMs: 1.648}},
		WallSubtotalMs: 5.149, EdgeSubtotalMs: 1.648,
	}}
	observations := traceQueryTypedSelfRunnableTwoRulerObservations(rank, types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/customer.trace", ArtifactID: "customer.trace", ArtifactKind: "trace",
	}, "customer.trace", "now")
	if len(observations) != 1 {
		t.Fatalf("fixture observations=%d", len(observations))
	}
	mu := types.NewMutableState("trace relation test")
	mu.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Observations: observations})
	bus := &types.BusContext{Mutable: mu}
	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	authorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	var claims []types.AnswerRelationClaim
	for _, authority := range authorities {
		if !authority.RequiredForClosure {
			continue
		}
		members := authority.MemberRefs
		if authority.Kind == types.AnswerRelationAuthorityCrossRulerBoundary {
			members = append(append([]string(nil), authority.LeftMemberRefs...), authority.RightMemberRefs...)
		}
		claims = append(claims, types.AnswerRelationClaim{
			AuthorityID: authority.ID, MemberRefs: members, PhysicalRelation: authority.PhysicalRelation,
			Addition: authority.Addition, SubtotalValue: authority.SubtotalValue, SubtotalUnit: authority.SubtotalUnit,
		})
	}
	if len(claims) != 3 {
		t.Fatalf("fixture relation claims=%d, authorities=%+v", len(claims), authorities)
	}
	return bus, claims
}

func TestCompletionRelationClaimsRejectCrossRulerTotalAndAcceptTypedRoster(t *testing.T) {
	bus, claims := relationClaimPipelineFixture(t)
	if got, err := validateCompletionRelationClaims(bus, nil); err == nil || len(got) != 0 || !strings.Contains(err.Error(), "missing required") {
		t.Fatalf("missing required claims should reject, got=%+v err=%v", got, err)
	}
	wrong := types.CloneAnswerRelationClaims(claims)
	wrong[2].Addition = types.AnswerRelationAdditionAuthorized
	value := 5.604
	wrong[2].SubtotalValue = &value
	wrong[2].SubtotalUnit = "ms"
	if _, err := validateCompletionRelationClaims(bus, wrong); err == nil || !strings.Contains(err.Error(), "requires \"forbidden\"") {
		t.Fatalf("wrong cross-ruler claim should reject, got %v", err)
	}
	got, err := validateCompletionRelationClaims(bus, claims)
	if err != nil || !types.AnswerRelationClaimsEqual(got, claims) {
		t.Fatalf("valid relation claims rejected: got=%+v err=%v", got, err)
	}
}

func TestAnswerDocumentRelationClaimsRemainModelOwnedAndCannotDrift(t *testing.T) {
	bus, claims := relationClaimPipelineFixture(t)
	bus.Mutable.SetInvestigationRelationClaims(claims)
	bus.Mutable.SetInvestigationComplete("model accepted the typed ruler boundaries")
	bus.Mutable.RetainInvestigationRelationClaims()

	missing := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s", Kind: types.BlockSummary, Text: "model conclusion"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(missing), nil, time.Now())
	if err != nil || res.Success || !strings.Contains(res.Summary, "must preserve every accepted") {
		t.Fatalf("missing final claims should be retryable rejection: res=%+v err=%v", res, err)
	}

	valid := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "s", Kind: types.BlockSummary, Text: "model conclusion", RelationClaims: claims,
	}}}
	res, err = ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(valid), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("valid model-owned relation claims rejected: res=%+v err=%v", res, err)
	}
	persisted := bus.Mutable.AnswerDocumentV2()
	if persisted == nil || len(persisted.Blocks) == 0 || !types.AnswerRelationClaimsEqual(persisted.Blocks[0].RelationClaims, claims) {
		t.Fatalf("relation claims did not survive persist: %+v", persisted)
	}
}
