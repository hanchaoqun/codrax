package tool

import (
	"encoding/json"
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

func TestCompletionRelationClaimsAreOptionalButRejectDriftAndAcceptTypedRoster(t *testing.T) {
	bus, claims := relationClaimPipelineFixture(t)
	if got, err := validateCompletionRelationClaims(bus, nil); err != nil || len(got) != 0 {
		t.Fatalf("omitted format-only claims should not block closure, got=%+v err=%v", got, err)
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

func TestCompletionRelationClaimsRecoverFromStringEncodedAggregateTail(t *testing.T) {
	bus, claims := relationClaimPipelineFixture(t)
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}
	params, err := json.Marshal(map[string]any{
		"reason":      "typed trace relation handoff complete",
		"confidence":  "high",
		"result_kind": "resolved",
		"aggregate_facts": `[{"kind":"scalar_value","label":"target duration","value":"20.000","unit":"ms"}], "relation_claims": ` +
			string(claimsJSON),
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	res, err := (&EmitInvestigationComplete{}).Execute(bus, params)
	if err != nil || !res.Success {
		t.Fatalf("misplaced typed relation claims should be recovered exactly: res=%+v err=%v", res, err)
	}
	got := bus.Mutable.StableInvestigationRelationClaims()
	if !types.AnswerRelationClaimsEqual(got, claims) {
		t.Fatalf("recovered relation claims mismatch: got=%+v want=%+v", got, claims)
	}
}

func TestAnswerDocumentRelationClaimsAreOptionalButSubmittedClaimsCannotDrift(t *testing.T) {
	bus, claims := relationClaimPipelineFixture(t)
	bus.Mutable.SetInvestigationRelationClaims(claims)
	bus.Mutable.SetInvestigationComplete("model accepted the typed ruler boundaries")
	bus.Mutable.RetainInvestigationRelationClaims()

	missing := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{ID: "s", Kind: types.BlockSummary, Text: "model conclusion"}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(missing), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("already-accepted investigation claims must not create a finalizer copy retry: res=%+v err=%v", res, err)
	}

	wrong := types.CloneAnswerRelationClaims(claims)
	wrong[0].Addition = types.AnswerRelationAdditionForbidden
	invalid := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "s", Kind: types.BlockSummary, Text: "model conclusion", RelationClaims: wrong,
	}}}
	res, err = ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(invalid), nil, time.Now())
	if err != nil || res.Success || !strings.Contains(res.Summary, "do not match typed trace authority") ||
		!strings.Contains(res.Summary, "omit optional relation_claims metadata") {
		t.Fatalf("invalid submitted final claim should remain a precise rejection: res=%+v err=%v", res, err)
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

func TestFinalRelationClaimsDoNotRequireAcceptedSnapshotSupersededByCurrentAuthority(t *testing.T) {
	bus, current := relationClaimPipelineFixture(t)
	stale := types.CloneAnswerRelationClaims(current[:1])
	stale[0].AuthorityID += ":explore-window"
	oldSubtotal := 15.0
	stale[0].SubtotalValue = &oldSubtotal
	bus.Mutable.SetInvestigationRelationClaims(stale)
	bus.Mutable.SetInvestigationComplete("model accepted an earlier typed window")
	bus.Mutable.RetainInvestigationRelationClaims()

	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "s", Kind: types.BlockSummary, Text: "model conclusion updated from final typed inputs",
		RelationClaims: current,
	}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("final current authority was trapped by a superseded accepted snapshot: res=%+v err=%v", res, err)
	}
}

func TestFinalRelationClaimsValidateOptionalAuthorityAddedByDeterministicSupplement(t *testing.T) {
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
	if err != nil || !res.Success {
		t.Fatalf("post-Explorer authority should remain decision context without a copy-only retry: res=%+v err=%v", res, err)
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

func TestTargetStateRelationClaimSurvivesToleratedAnchorEndpointDrift(t *testing.T) {
	ref := types.ObservationSourceRef{
		Kind: types.ObservationSourceRuntimeArtifact, Path: "/tmp/customer.trace",
		ArtifactID: "customer.trace", ArtifactKind: "trace",
	}
	anchorWindow := types.TraceNoteKeySelectedWindow + "=10.000000..10.015000"
	accountWindow := types.TraceNoteKeySelectedWindow + "=10.000000..10.015020"
	mu := types.NewMutableState("trace tolerated relation test")
	mu.AppendDispatchToolResult(types.ToolResult{ToolName: "trace_query", Success: true, Observations: []types.ObservationRecord{
		{
			ID: "trace_query:rank#root", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			ClaimKey: "root_cause_primary:target-7", Subject: "target-7", Predicate: "root_cause_primary",
			Object: "running", Value: "1", Unit: "ms",
			RichNotes: []string{"impact_ms=1.000", "cumulative_impact_ms=1.000", "rank=1", "tier=primary", "chain_relevance=on_chain", "causality=self_wall_clock", anchorWindow},
		},
		{
			ID: "trace_query:states#target_window_states", Origin: types.AnswerEvidenceOriginRuntimeArtifact,
			Producer: "trace_query", GroundingPolicy: types.ClaimGroundingHard, SourceRef: ref,
			ClaimKey: "target_window_states:target-7", Subject: "target-7", Predicate: "target_window_states",
			Object: "state_partition", Value: "15.020", Unit: "ms",
			RichNotes: []string{
				types.TraceNoteKeyRunning + "=1.000", types.TraceNoteKeyRunnable + "=2.000",
				types.TraceNoteKeySleep + "=3.020", types.TraceNoteKeyDState + "=4.000",
				types.TraceNoteKeyIOWait + "=5.000", types.TraceNoteKeyTotal + "=15.020", accountWindow,
			},
		},
	}})
	bus := &types.BusContext{Mutable: mu}

	previewAuthorities := types.CompileTraceAnswerRelationAuthorities(types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{
			Subject: "target-7", RunningMS: 1, RunnableMS: 2, SleepMS: 3.020,
			DStateMS: 4, IOWaitMS: 5, TotalMS: 15.020,
			WindowStartTs: 10, WindowEndTs: 10.015020,
		},
	}}})
	if len(previewAuthorities) != 1 {
		t.Fatalf("preview target-state authority=%+v", previewAuthorities)
	}
	preview := previewAuthorities[0]
	claim := types.AnswerRelationClaim{
		AuthorityID: preview.ID, MemberRefs: append([]string(nil), preview.MemberRefs...),
		PhysicalRelation: preview.PhysicalRelation, Addition: preview.Addition,
		SubtotalValue: preview.SubtotalValue, SubtotalUnit: preview.SubtotalUnit,
	}
	if got, err := validateCompletionRelationClaims(bus, []types.AnswerRelationClaim{claim}); err != nil || len(got) != 1 {
		t.Fatalf("completion rejected a relation admitted within the shared window tolerance: got=%+v err=%v", got, err)
	}
	mu.SetInvestigationRelationClaims([]types.AnswerRelationClaim{claim})
	mu.SetInvestigationComplete("model accepted the typed target-state partition")
	mu.RetainInvestigationRelationClaims()
	doc := &types.AnswerDocumentV2{DocumentModel: "v2", Blocks: []types.AnswerBlock{{
		ID: "s", Kind: types.BlockSummary, Text: "model conclusion", RelationClaims: []types.AnswerRelationClaim{claim},
	}}}
	res, err := ApplyAndPersistMutation(bus, "test_emit", types.NewReplaceAllMutation(doc), nil, time.Now())
	if err != nil || !res.Success {
		t.Fatalf("final answer rejected the same accepted target-state relation: res=%+v err=%v", res, err)
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
