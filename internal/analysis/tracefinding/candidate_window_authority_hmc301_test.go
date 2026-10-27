package tracefinding

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestHMC301CandidatePrincipalWindowAuthority(t *testing.T) {
	for _, token := range []string{"io_wait", "scheduler_latency", "priority_inversion_candidate"} {
		for _, tc := range []struct {
			name string
			edit func(*types.TraceCausalProjection)
			want int
		}{
			{"known_expansion_inside_occurrence", nil, 1},
			{"wakeup_authority", func(p *types.TraceCausalProjection) {
				p.TargetStateAccount = nil
				p.WakeupPathQueryWindowStartTs, p.WakeupPathQueryWindowEndTs = 0, .020
			}, 1},
			{"display_ruler_only_multi_board", func(p *types.TraceCausalProjection) { p.TargetStateAccount = nil }, 2},
			{"other_account_is_not_authority", func(p *types.TraceCausalProjection) { p.TargetStateAccount.WindowEndTs = .021 }, 2},
			{"legacy_account_unknown_window", func(p *types.TraceCausalProjection) { p.TargetStateAccount.WindowEndTs = 0 }, 2},
			{"unknown_projection_window", func(p *types.TraceCausalProjection) { p.WindowEndTs = 0 }, 2},
			{"unknown_node_window", func(p *types.TraceCausalProjection) { p.RankedSeats[1].QueryWindowEndTs = 0 }, 2},
			{"rank_donor_exact_over_wide_seed", func(p *types.TraceCausalProjection) { p.RankedSeats[1].RankQueryWindowEndTs = .020 }, 2},
			{"rank_donor_wide_over_exact_seed", func(p *types.TraceCausalProjection) {
				p.RankedSeats[1].QueryWindowEndTs = .020
				p.RankedSeats[1].RankQueryWindowEndTs = .021
			}, 1},
			{"within_existing_two_microsecond_tolerance", func(p *types.TraceCausalProjection) { p.RankedSeats[1].QueryWindowEndTs = .020001 }, 2},
			{"outside_existing_two_microsecond_tolerance", func(p *types.TraceCausalProjection) { p.RankedSeats[1].QueryWindowEndTs = .020003 }, 1},
			{"same_window_independent_boards", func(p *types.TraceCausalProjection) { p.RankedSeats[1].QueryWindowEndTs = .020 }, 2},
		} {
			t.Run(token+"/"+tc.name, func(t *testing.T) {
				p := types.TraceCausalProjection{
					ArtifactPath: "traces/a.trace", ArtifactLabel: "a.trace", WindowEndTs: .020,
					TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{Subject: "app-100", WindowEndTs: .020, TotalMS: 20},
				}
				for i, id := range []string{"exact", "probe"} {
					p.RankedSeats = append(p.RankedSeats, types.TraceCausalProjectionNode{
						EvidenceID: id, Subject: "worker-7", TypeToken: token, ChainRelevance: "on_chain",
						Rank: i + 1, ImpactMS: 8, StartTs: .003, EndTs: .014,
						QueryWindowEndTs: .020 + float64(i)*.001, RankBoardParamsFingerprint: "board-" + id,
					})
				}
				if tc.edit != nil {
					tc.edit(&p)
				}
				set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{p}}
				before, _ := json.Marshal(set)
				ledger := types.ObservationLedger{Records: []types.ObservationRecord{{ID: "exact"}, {ID: "probe"}}}
				contract, err := CompileCandidateContract(ledger, set, SeatFrameCausalityAuthority{})
				if err != nil {
					t.Fatal(err)
				}
				if len(contract.Candidates) != tc.want || len(contract.PrimaryCandidateIDs) != tc.want || len(contract.ContributorCandidateIDs) != tc.want {
					t.Fatalf("selected-board eligibility: want %d, got %+v", tc.want, contract)
				}
				if !reflect.DeepEqual(contract.AcceptedEvidenceIDs, []string{"exact", "probe"}) {
					t.Fatalf("exploratory evidence erased: %v", contract.AcceptedEvidenceIDs)
				}
				after, _ := json.Marshal(set)
				if !reflect.DeepEqual(before, after) {
					t.Fatal("compilation mutated measurements")
				}
				for _, candidate := range contract.Candidates {
					if candidate.Decision.Magnitude == nil || candidate.Decision.Magnitude.Value != 8 || candidate.Decision.Token.Token != token {
						t.Fatalf("candidate caliber/token changed: %+v", candidate)
					}
					if tc.want == 1 && !reflect.DeepEqual(candidate.Decision.EvidenceRefs, []string{"exact"}) {
						t.Fatalf("wrong board retained: %+v", candidate)
					}
				}
			})
		}
	}
}

func TestHMC301CandidateScopeDoesNotPromoteBackgroundOrOutside(t *testing.T) {
	outside := false
	set := types.TraceCausalProjectionSet{Projections: []types.TraceCausalProjection{{
		WindowEndTs:        .020,
		TargetStateAccount: &types.TraceCausalProjectionTargetStateAccount{WindowEndTs: .020},
		RankedSeats: []types.TraceCausalProjectionNode{
			{EvidenceID: "background", TypeToken: "scheduler_latency", ChainRelevance: "background", Rank: 1, ImpactMS: 3, QueryWindowEndTs: .020},
			{EvidenceID: "outside", TypeToken: "scheduler_latency", ChainRelevance: "on_chain", Rank: 2, ImpactMS: 4, QueryWindowEndTs: .020, WithinRequestedWindow: &outside},
		},
	}}}
	contract, err := CompileCandidateContract(types.ObservationLedger{}, set, SeatFrameCausalityAuthority{})
	if err != nil || len(contract.Candidates) != 1 || len(contract.PrimaryCandidateIDs) != 0 || len(contract.ContributorCandidateIDs) != 1 || contract.Candidates[0].Decision.EvidenceRefs[0] != "background" {
		t.Fatalf("existing background/outside boundary changed: %v %+v", err, contract)
	}
}
