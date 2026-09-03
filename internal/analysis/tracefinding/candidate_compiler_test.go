package tracefinding

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestCompileCandidateContractBindsTypedRankSeat(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E1"}}}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		ArtifactPath: "traces/a.systrace", ArtifactLabel: "a.systrace",
		WindowStartTs: 1, WindowEndTs: 1.02,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: "ui-7", TotalMS: 20},
		RankedSeats: []types.TraceCausalProjectionNode{{
			EvidenceID: "E1", Subject: "ui-7", TypeToken: "scheduler_latency",
			ChainRelevance: "on_chain",
			SpanName:       "DrawFrame", BlockingKind: "monitor_contention", BlockingHolderSite: "ClassLinker lock",
			Rank: 1, Tier: "A", EffectiveImpactMS: 8.5, EffectiveImpactPublished: true,
			RankBoardParamsFingerprint: "board-1", Confidence: 0.9,
		}},
	}}}
	contract, err := CompileCandidateContract(ledger, set, SeatFrameCausalityAuthority{Applicable: true, Index: SeatFrameCausalityIndex{"E1": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Candidates) != 1 || len(contract.PrimaryCandidateIDs) != 1 {
		t.Fatalf("unexpected candidates: %+v", contract)
	}
	decision := contract.Candidates[0].Decision
	if decision.Token.Token != "scheduler_latency" || decision.SubjectRole != "target_thread" {
		t.Fatalf("unexpected decision snapshot: %+v", decision)
	}
	if decision.SubjectName != "ui-7" || decision.ResourceName != "DrawFrame" || decision.PhaseName != "DrawFrame" || decision.BlockingKind != "monitor_contention" {
		t.Fatalf("concrete trace names were not preserved: %+v", decision)
	}
	if decision.Magnitude == nil || decision.Magnitude.Value != 8.5 || decision.Magnitude.Caliber != "effective_attribution" {
		t.Fatalf("unexpected magnitude: %+v", decision.Magnitude)
	}
	if contract.FindingID == "" || contract.AnalysisKey == "" || contract.ContractHash == "" {
		t.Fatalf("missing stable contract identities: %+v", contract)
	}
}

func TestCompileCandidateContractKeepsContextOutOfPrimaryRoster(t *testing.T) {
	ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "E1"}, {ID: "E2"}, {ID: "E3"}}}
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		RankedSeats: []types.TraceCausalProjectionNode{
			{EvidenceID: "E1", TypeToken: "scheduler_latency", Rank: 1, ImpactMS: 8, ChainRelevance: "on_chain"},
			{EvidenceID: "E2", TypeToken: "scheduler_latency", Rank: 2, ImpactMS: 6, ChainRelevance: "background"},
			{EvidenceID: "E3", TypeToken: "scheduler_latency", Rank: 3, ImpactMS: 4, Tier: types.TraceCausalTierContextOnly},
		},
	}}}
	contract, err := CompileCandidateContract(ledger, set, SeatFrameCausalityAuthority{Applicable: true, Index: SeatFrameCausalityIndex{"E1": true}})
	if err != nil {
		t.Fatal(err)
	}
	if len(contract.Candidates) != 2 {
		t.Fatalf("candidates=%d, want on-chain plus background contributor: %+v", len(contract.Candidates), contract.Candidates)
	}
	if len(contract.PrimaryCandidateIDs) != 1 || len(contract.ContributorCandidateIDs) != 2 {
		t.Fatalf("unexpected eligibility rosters: primary=%v contributor=%v", contract.PrimaryCandidateIDs, contract.ContributorCandidateIDs)
	}
}

func TestCandidateCausalityUnprovenNeverDefaultsToProven(t *testing.T) {
	if candidateCausalityExplicitlyProven("unproven") {
		t.Fatal("unproven must not be matched as proven")
	}
}
