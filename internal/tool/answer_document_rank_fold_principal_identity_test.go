package tool

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hanchaoqun/codrax/internal/analysis/tracefinding"
	"github.com/hanchaoqun/codrax/internal/tracequery"
	"github.com/hanchaoqun/codrax/internal/types"
)

func rankFoldPrincipalIdentityPair() (types.TraceCausalProjectionNode, types.TraceCausalProjectionNode) {
	rank := types.TraceCausalProjectionNode{
		EvidenceID: "rank", Subject: "worker-200", Predicate: "root_cause_primary",
		Object: "priority_inversion_candidate", Rank: 3, Tier: "primary",
		ChainRelevance: "on_chain", ChainCredentialCensus: "interval_proven",
		PriorityInversionCandidate: true, EffectiveImpactMS: 7.405, CumulativeImpactMS: 8.403,
		ImpactMS: 7.405, LineStart: 100, LineEnd: 200,
		QueryWindowStartTs: 10.000708, QueryWindowEndTs: 10.233898,
		RankBoardTarget: "app-100", RankBoardParamsFingerprint: "same-rank-knobs",
	}
	chain := rank
	chain.EvidenceID, chain.Predicate, chain.Object = "chain", "wakeup_causal_impact", "running"
	chain.Rank, chain.Tier, chain.RankBoardTarget, chain.RankBoardParamsFingerprint = 0, "", "", ""
	chain.StateKind, chain.ImpactMS = "running", 8.294
	chain.QueryWindowStartTs, chain.QueryWindowEndTs = 10, 10.234
	return rank, chain
}

func TestRankFoldCarriesOneCompleteDonorIdentity(t *testing.T) {
	for _, preserved := range []bool{false, true} {
		t.Run(map[bool]string{false: "source_query_pair", true: "preserved_rank_pair"}[preserved], func(t *testing.T) {
			rank, chain := rankFoldPrincipalIdentityPair()
			wantStart, wantEnd := rank.QueryWindowStartTs, rank.QueryWindowEndTs
			if preserved {
				rank.RankQueryWindowStartTs, rank.RankQueryWindowEndTs = wantStart, wantEnd
				rank.QueryWindowStartTs, rank.QueryWindowEndTs = 10, 10.234
			}
			// The host's provisional annotation is not the adopted ordinal's tier.
			chain.Tier = "secondary"
			kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain})
			if len(kept) != 1 || len(peers) != 1 {
				t.Fatalf("same accounted interval must still fold: kept=%+v peers=%+v", kept, peers)
			}
			want := chain
			want.Rank, want.Tier = rank.Rank, rank.Tier
			want.RankBoardTarget, want.RankBoardParamsFingerprint = rank.RankBoardTarget, rank.RankBoardParamsFingerprint
			want.RankQueryWindowStartTs, want.RankQueryWindowEndTs = wantStart, wantEnd
			if !reflect.DeepEqual(kept[0], want) {
				t.Fatalf("only the complete rank identity may move; source state/account/query must remain unchanged:\ngot=%+v\nwant=%+v", kept[0], want)
			}
			if !types.TraceCausalProjectionNodeMatchesPrincipalWindow(kept[0], wantStart, wantEnd) {
				t.Fatal("folded receipt disappeared from its own exact principal window")
			}
			if types.TraceCausalProjectionNodeMatchesPrincipalWindow(kept[0], wantStart, wantEnd+.000020) {
				t.Fatal("identity carriage must not widen the principal-value window gate")
			}
			if !reflect.DeepEqual(peers[runtimeTraceCausalProjectionNodeKey(kept[0])], []types.TraceCausalProjectionNode{rank}) {
				t.Fatal("the original rank receipt must remain lossless in the fold peer")
			}
		})
	}
}

func TestRankFoldKnownBoardConflictsRemainSeparate(t *testing.T) {
	for name, mutate := range map[string]func(*types.TraceCausalProjectionNode){
		"different_target":       func(n *types.TraceCausalProjectionNode) { n.RankBoardTarget = "other-300" },
		"different_params":       func(n *types.TraceCausalProjectionNode) { n.RankBoardParamsFingerprint = "other-knobs" },
		"different_query_window": func(n *types.TraceCausalProjectionNode) { n.QueryWindowEndTs += .010 },
		"different_rank_window": func(n *types.TraceCausalProjectionNode) {
			n.Rank = 4
			n.RankQueryWindowStartTs, n.RankQueryWindowEndTs = 20, 20.234
		},
		"host_symptom_exclusion": func(n *types.TraceCausalProjectionNode) { n.Tier = "target_self_state" },
		"host_context_exclusion": func(n *types.TraceCausalProjectionNode) { n.Tier = types.TraceCausalTierContextOnly },
	} {
		t.Run(name, func(t *testing.T) {
			rank, chain := rankFoldPrincipalIdentityPair()
			mutate(&chain)
			in := []types.TraceCausalProjectionNode{rank, chain}
			kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins(in)
			if !reflect.DeepEqual(kept, in) || len(peers) != 0 {
				t.Fatalf("known-different rank domains must remain two lossless rows: %+v peers=%+v", kept, peers)
			}
		})
	}
}

func TestRankFoldKeepsAnAlreadySeatedHostsCompleteIdentity(t *testing.T) {
	rank, chain := rankFoldPrincipalIdentityPair()
	chain.Rank, chain.Tier = 4, "tertiary"
	chain.RankBoardTarget, chain.RankBoardParamsFingerprint = rank.RankBoardTarget, rank.RankBoardParamsFingerprint
	chain.RankQueryWindowStartTs, chain.RankQueryWindowEndTs = rank.QueryWindowStartTs, rank.QueryWindowEndTs
	kept, peers := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || len(peers) != 1 || !reflect.DeepEqual(kept[0], chain) {
		t.Fatalf("an existing seat must not acquire parts of a different donor's identity: %+v", kept)
	}
}

func TestRankFoldIdentityDoesNotInventWindowOrChainCredential(t *testing.T) {
	rank, chain := rankFoldPrincipalIdentityPair()
	rank.QueryWindowStartTs, rank.QueryWindowEndTs = 0, 0
	rank.RankBoardTarget, rank.RankBoardParamsFingerprint = "", ""
	chain.RankBoardTarget, chain.RankBoardParamsFingerprint = "host-only-target", "host-only-knobs"
	// A donor with no window must not acquire one from its display host.
	kept, _ := runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || kept[0].RankQueryWindowStartTs != 0 || kept[0].RankQueryWindowEndTs != 0 ||
		kept[0].RankBoardTarget != "" || kept[0].RankBoardParamsFingerprint != "" {
		t.Fatalf("legacy missing donor identity must stay absent, never filled from the host: %+v", kept)
	}
	// An incomplete preserved window must not mix one endpoint with the
	// source-query pair. Only that complete source-query pair may travel.
	rank, chain = rankFoldPrincipalIdentityPair()
	rank.RankQueryWindowStartTs = 1
	kept, _ = runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 || kept[0].RankQueryWindowStartTs != rank.QueryWindowStartTs || kept[0].RankQueryWindowEndTs != rank.QueryWindowEndTs {
		t.Fatalf("rank window endpoints must be transferred as one complete pair: %+v", kept)
	}
	rank, chain = rankFoldPrincipalIdentityPair()
	rank.ChainCredentialCensus, chain.ChainCredentialCensus = "none", "none"
	kept, _ = runtimeTraceProjFoldSameSegmentLaneTwins([]types.TraceCausalProjectionNode{rank, chain})
	if len(kept) != 1 {
		t.Fatal("credential test requires the existing display fold to run")
	}
	row := runtimeTraceProjTreeRow{Node: kept[0], HasData: true, Kind: runtimeTraceProjTreeRowCause}
	if _, valid := runtimeTraceProjRowValidSeat(row); valid || runtimeTraceProjRowSeatBadgeOrdinal(row) != 0 {
		t.Fatal("a rank-window receipt is not a chain credential and cannot mint a badge/crown")
	}
}

// B1568 production witness: a rounded explorer wakeup window and an exact
// system-supplement rank window describe the same scheduler segments. The
// retained chain host must not replace the exact rank receipt with its own
// rounded window. This traverses the actual engine -> observation -> merge ->
// tree/overview, and compares the independent programmatic selection surface.
func TestRankFoldRoundedProbeMatchesExactRankInTreeOverviewAndJSON(t *testing.T) {
	if testing.Short() {
		t.Skip("real-trace witness")
	}
	if _, err := os.Stat(elimSemanticDonghuTrace); err != nil {
		t.Skipf("golden fixture not present: %v", err)
	}
	idx, err := tracequery.BuildIndex(context.Background(), elimSemanticDonghuTrace)
	if err != nil {
		t.Fatal(err)
	}
	start, end := 13762.791708, 13763.024898
	var records []types.ObservationRecord
	for _, q := range []tracequery.Query{
		{View: "wakeup_chain", PID: 17267, TimeStart: 13762.791, TimeEnd: 13763.025},
		{View: "root_cause_rank", PID: 17267, TimeStart: start, TimeEnd: end},
	} {
		q.MaxDepth, q.MinDurationMs, q.Limit = 4, .5, 12
		q.TraceFlavorHint = tracequery.TraceFlavorHarmonyHitrace
		result := tracequery.Run(idx, q)
		// Distinct chronological artifact IDs retain the earlier rounded
		// chain receipt when the identical per-segment publications dedupe.
		resultID := "probe-0"
		if q.View == "root_cause_rank" {
			resultID = "supplement-1"
		}
		observations := traceQueryTypedObservations(result, elimSemanticDonghuTrace, resultID, "r", "", time.Unix(1, 0))
		for i := range observations {
			observations[i].SystemSupplement = q.View == "root_cause_rank"
		}
		records = append(records, observations...)
	}
	ledger := types.ObservationLedger{
		Records: records, AnchorUserEntities: []types.AnchorUserEntity{{Value: "17267", TypedLane: true}},
		RuntimeArtifactScopeProfile: &types.RuntimeArtifactScopeProfile{
			RequestedScope: types.RuntimeArtifactScopeExplicitWindow, TimeStart: &start, TimeEnd: &end,
			SourceQuote: "13762.791708..13763.024898",
		},
	}
	set := types.CompileTraceCausalProjectionSet(ledger)
	if len(set.Projections) != 1 {
		t.Fatalf("expected one trace partition, got %d", len(set.Projections))
	}
	projection := set.Projections[0]
	if projection.WindowStartTs != start || projection.WindowEndTs != end || !types.TraceCausalProjectionPrincipalWindowAuthoritative(projection) {
		t.Fatalf("fixture must elect the exact principal scope: %.6f..%.6f", projection.WindowStartTs, projection.WindowEndTs)
	}
	contract, err := tracefinding.CompileCandidateContract(ledger, set, tracefinding.SeatFrameCausalityAuthority{})
	if err != nil {
		t.Fatal(err)
	}
	contract.RootCauseReportEnabled = true
	selection := &types.TraceRootCauseReportV2{SchemaVersion: types.TraceRootCauseReportSchemaVersion}
	wants := map[string]float64{"CompThread_0-2955": 7.405, "JankManager-9655": 4.710}
	for _, candidate := range contract.Candidates {
		if _, ok := wants[candidate.Decision.SubjectName]; ok && candidate.Decision.Token.Token == "priority_inversion_candidate" {
			selection.RootCauses = append(selection.RootCauses, &types.TraceRootCauseItemV2{CandidateID: candidate.Decision.CandidateID})
		}
	}
	bound, err := tracefinding.BindRootCauseReportSelection(selection, contract)
	if err != nil || bound == nil || len(bound.RootCauses) != 2 {
		t.Fatalf("both original rank receipts must remain selectable: %v %+v", err, bound)
	}
	wire, err := json.Marshal(bound)
	if err != nil {
		t.Fatal(err)
	}
	var public types.TraceRootCauseReportV2
	if err := json.Unmarshal(wire, &public); err != nil {
		t.Fatal(err)
	}
	model := buildRuntimeTraceProjTreeModel(projection, newRuntimeTraceCausalProjectionEvidenceIndex(), true)
	tree, overview := runtimeTraceProjTreeFence(model, true), runtimeTraceProjElimOverviewFence(projection, model, true)
	overviewCandidates := map[string]bool{}
	for _, entry := range runtimeTraceProjElimBoard(model) {
		if entry.row.Node.PriorityInversionCandidate && entry.row.Node.Rank > 0 {
			overviewCandidates[entry.row.Node.Subject] = true
		}
	}
	for _, cause := range public.RootCauses {
		want, ok := wants[cause.ThreadName]
		if !ok || cause.ImpactSeconds == nil || math.Abs(*cause.ImpactSeconds*1000-want) > .00051 {
			t.Fatalf("public amount changed: %+v", cause)
		}
		if !strings.Contains(tree, cause.ThreadName) || !strings.Contains(overview, cause.ThreadName) || !overviewCandidates[cause.ThreadName] {
			t.Errorf("same proven seat must remain visible in tree AND overview: %s\n%s", cause.ThreadName, overview)
		}
		foundFolded := false
		for _, rows := range [][]runtimeTraceProjTreeRow{model.SelfRows, model.TreeRows} {
			for _, row := range rows {
				if row.Node.Subject == cause.ThreadName && row.Node.Rank > 0 && len(row.RankFoldPeers) > 0 {
					foundFolded = true
					if row.Node.RankQueryWindowStartTs != start || row.Node.RankQueryWindowEndTs != end ||
						!types.TraceCausalProjectionNodeMatchesPrincipalWindow(row.Node, start, end) {
						t.Errorf("fold host lost the exact rank donor identity: subject=%s rank=%d query=%.6f..%.6f rank_query=%.6f..%.6f",
							row.Node.Subject, row.Node.Rank, row.Node.QueryWindowStartTs, row.Node.QueryWindowEndTs,
							row.Node.RankQueryWindowStartTs, row.Node.RankQueryWindowEndTs)
					}
				}
			}
		}
		if !foundFolded {
			t.Errorf("production witness no longer traverses the rank fold: %s", cause.ThreadName)
		}
	}
}
