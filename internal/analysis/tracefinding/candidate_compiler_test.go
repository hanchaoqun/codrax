package tracefinding

import (
	"strings"
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

// Candidates that share a rank are ordered by what the trace says about them.
// The candidate id hashes the artifact path and the evidence coordinates, so a
// directory or a digest must never decide the order, and with it the scope the
// contract takes from its first candidate.
func TestCompileCandidateContractSameRankOrderIgnoresLocationIdentity(t *testing.T) {
	digestA, digestB := strings.Repeat("a", 64), strings.Repeat("b", 64)
	compile := func(artifactPath, strongDigest, weakDigest string) *types.TraceFindingContract {
		t.Helper()
		strong, weak := "trace_query:publication-"+strongDigest+"#root_cause_rank:1", "trace_query:publication-"+weakDigest+"#root_cause_rank:2"
		ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: strong}, {ID: weak}}}
		set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
			ArtifactPath: artifactPath, ArtifactLabel: "capture.systrace",
			RankedSeats: []types.TraceCausalProjectionNode{
				{EvidenceID: weak, Subject: "worker-2", TypeToken: "scheduler_latency", Rank: 2, ImpactMS: 6, ChainRelevance: "on_chain"},
				{EvidenceID: strong, Subject: "worker-1", TypeToken: "scheduler_latency", Rank: 2, ImpactMS: 8, ChainRelevance: "on_chain", ChainDepth: 1},
			},
		}}}
		contract, err := CompileCandidateContract(ledger, set, SeatFrameCausalityAuthority{})
		if err != nil {
			t.Fatal(err)
		}
		return contract
	}
	for _, tc := range []struct{ path, strong, weak string }{
		{"/a/capture.systrace", digestA, digestB},
		{"/a/capture.systrace", digestB, digestA},
		{"/zz/other/dir/capture.systrace", digestA, digestB},
		{"/zz/other/dir/capture.systrace", digestB, digestA},
	} {
		contract := compile(tc.path, tc.strong, tc.weak)
		if len(contract.Candidates) != 2 {
			t.Fatalf("%s: control: both seats must compile, got %d", tc.path, len(contract.Candidates))
		}
		if first := contract.Candidates[0].Decision; first.SubjectName != "worker-1" || first.Magnitude.Value != 8 {
			t.Fatalf("%s: the larger account leads a shared rank, got %s %.3f", tc.path, first.SubjectName, first.Magnitude.Value)
		}
		if contract.Scope.Phase != "pre_wakeup_dependency" {
			t.Fatalf("%s: the scope comes from the leading candidate, got %q", tc.path, contract.Scope.Phase)
		}
	}
}
