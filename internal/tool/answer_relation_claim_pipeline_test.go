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
	if err != nil || res.Success || !strings.Contains(res.Summary, "missing required model-authored relation claim") {
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

func TestFinalRelationClaimsIncludeClosureAuthorityAddedByDeterministicSupplement(t *testing.T) {
	bus, accepted := relationClaimPipelineFixture(t)
	bus.Mutable.SetInvestigationRelationClaims(accepted)
	bus.Mutable.SetInvestigationComplete("model accepted the explorer authorities")
	bus.Mutable.RetainInvestigationRelationClaims()

	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/customer.trace",
		ArtifactID: "customer.trace", ArtifactKind: "trace",
	}
	window := types.TraceNoteKeySelectedWindow + "=10.000000..10.015000"
	bus.Mutable.SetSystemTraceSupplement(types.SystemTraceSupplementMeta{Views: []string{"root_cause_rank"}}, []types.ToolResult{{
		ToolName: "trace_query", Success: true, Summary: "deterministic target state supplement", Observations: []types.ObservationRecord{
			{
				ID: "trace_query:supplement#root", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				ClaimKey: "root_cause_primary:target-7", Subject: "target-7", Predicate: "root_cause_primary",
				Object: "running", Value: "1", Unit: "ms",
				RichNotes: []string{"impact_ms=1.000", "cumulative_impact_ms=1.000", "rank=1", "tier=primary", "chain_relevance=on_chain", "causality=self_wall_clock", window},
			},
			{
				ID: "trace_query:supplement#target_window_states", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
				Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
				ClaimKey: "target_window_states:target-7", Subject: "target-7", Predicate: "target_window_states",
				Object: "state_partition", Value: "15.000", Unit: "ms",
				RichNotes: []string{
					types.TraceNoteKeyRunning + "=1.000", types.TraceNoteKeyRunnable + "=2.000",
					types.TraceNoteKeySleep + "=3.000", types.TraceNoteKeyDState + "=4.000",
					types.TraceNoteKeyIOWait + "=5.000", types.TraceNoteKeyTotal + "=15.000", window,
				},
			},
		},
	}})
	preLedger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	preAuthorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(preLedger)
	if len(preAuthorities) != len(accepted)+1 {
		t.Fatalf("deterministic supplement did not compile its state relation authority: records=%+v authorities=%+v", preLedger.Records, preAuthorities)
	}

	missingSupplement := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "s", Kind: types.BlockSummary, Text: "model conclusion", RelationClaims: accepted,
	}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(missingSupplement), nil, time.Now())
	if err != nil || res.Success || !strings.Contains(res.Summary, "trace:target_state_partition:") {
		t.Fatalf("post-Explorer closure authority must require a model-authored final claim: res=%+v err=%v", res, err)
	}

	ledger := types.CompileObservationLedger(types.ObservationLedgerInputFromBusContext(bus, types.ObservationExtractLedgerEvidenceLimit))
	authorities := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	submitted := types.CloneAnswerRelationClaims(accepted)
	for _, authority := range authorities {
		if !authority.RequiredForClosure || strings.HasPrefix(authority.ID, "trace:self_runnable_two_ruler:") {
			continue
		}
		submitted = append(submitted, types.AnswerRelationClaim{
			AuthorityID: authority.ID, MemberRefs: append([]string(nil), authority.MemberRefs...),
			PhysicalRelation: authority.PhysicalRelation, Addition: authority.Addition,
			SubtotalValue: authority.SubtotalValue, SubtotalUnit: authority.SubtotalUnit,
		})
	}
	if len(submitted) != len(accepted)+1 {
		t.Fatalf("supplement authority roster=%+v submitted=%+v", authorities, submitted)
	}
	valid := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "s", Kind: types.BlockSummary, Text: "model conclusion", RelationClaims: submitted,
	}}}
	res, err = ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(valid), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("model-authored superset preserving Explorer claims was rejected: res=%+v err=%v", res, err)
	}
}

func TestSameSourcePartitionAuthorityIDMatchesHeadAndLedger(t *testing.T) {
	target := tracequery.ThreadRef{Comm: "target", PID: 17267}
	thread := tracequery.ThreadRef{Comm: "CompThread_0", PID: 2955, TGID: 1864}
	rank := &tracequery.RootCauseRankResult{
		Target: target, Window: tracequery.TimeWindow{StartTs: 10, EndTs: 10.23319}, BoardParamsFingerprint: "board-a",
		Items: []tracequery.RootCauseRankItem{
			{Rank: 6, Tier: "tertiary", Type: "d_state_or_io_wait", Thread: thread, DominantState: "d_sleep", DStateMs: 3.598000001, ImpactMs: 3.598000001, CumulativeImpactMs: 3.598000001, EffectiveImpactMs: 3.598000001, LineStart: 2261, LineEnd: 25863, ChainAnchoredMs: 3.598000001, ChainAnchorFullMs: 36.757000002, ChainRelevance: "on_chain", Causality: "on_wakeup_chain"},
			{Rank: 1, Tier: "tertiary", Type: "d_state_or_io_wait", Thread: thread, DominantState: "d_sleep", DStateMs: 33.159000001, ImpactMs: 33.159000001, CumulativeImpactMs: 33.159000001, EffectiveImpactMs: 33.159000001, LineStart: 2261, LineEnd: 25863, ChainAnchoredMs: 3.598000001, ChainAnchorFullMs: 36.757000002, ChainAnchorRemainderSeat: true, ChainRelevance: "adjacent", Causality: "adjacent_to_wakeup_chain"},
		},
	}
	preview := types.CompileTraceAnswerRelationAuthorities(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		RankedSeats: traceQueryRelationAuthorityRankedSeats(rank),
	}}})
	result := tracequery.Result{View: "root_cause_rank", RootCauseRank: rank}
	records := traceQueryTypedObservations(result, "customer.systrace", "payload", "raw", "", time.Unix(1, 0).UTC())
	ledger := types.ObservationLedger{Records: records}
	compiled := types.CompileTraceAnswerRelationAuthoritiesFromLedger(ledger)
	find := func(authorities []types.AnswerRelationAuthority) *types.AnswerRelationAuthority {
		for i := range authorities {
			if authorities[i].Kind == types.AnswerRelationAuthoritySameSourcePartition {
				return &authorities[i]
			}
		}
		return nil
	}
	pre, post := find(preview), find(compiled)
	if pre == nil || post == nil || pre.ID != post.ID || !types.AnswerRelationClaimsEqual(
		[]types.AnswerRelationClaim{{AuthorityID: pre.ID, MemberRefs: pre.MemberRefs, PhysicalRelation: pre.PhysicalRelation, Addition: pre.Addition, SubtotalValue: pre.SubtotalValue, SubtotalUnit: pre.SubtotalUnit}},
		[]types.AnswerRelationClaim{{AuthorityID: post.ID, MemberRefs: post.MemberRefs, PhysicalRelation: post.PhysicalRelation, Addition: post.Addition, SubtotalValue: post.SubtotalValue, SubtotalUnit: post.SubtotalUnit}},
	) {
		t.Fatalf("head/ledger same-source authority drifted: preview=%+v compiled=%+v records=%+v", pre, post, records)
	}
}
