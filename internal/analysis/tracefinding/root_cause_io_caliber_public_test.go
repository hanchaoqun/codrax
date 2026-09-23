package tracefinding

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

func TestRootCauseIOValueCaliberPublic(t *testing.T) {
	for _, caliber := range []string{types.TraceIOValueCaliberRQResidence, types.TraceIOValueCaliberBIOResidence,
		types.TraceIOValueCaliberIssuerBlocked, types.TraceIOValueCaliberMixed, "", "unrecognized"} {
		for _, lane := range []string{"on_chain", "background"} {
			t.Run(caliber+"/"+lane, func(t *testing.T) {
				node := types.TraceCausalProjectionNode{EvidenceID: "E1", Subject: "worker-12", TypeToken: "io_latency", Rank: 1,
					ChainRelevance: lane, ImpactMS: 31, IOValueCaliber: caliber, ResourceCompletionClosure: true}
				contract, err := CompileCandidateContract(types.ObservationLedger{}, types.TraceCausalProjectionSet{
					Projections: []types.TraceCausalProjection{{RankedSeats: []types.TraceCausalProjectionNode{node}}}}, SeatFrameCausalityAuthority{})
				if err != nil || len(contract.Candidates) != 1 {
					t.Fatalf("compile: %v %+v", err, contract)
				}
				candidate := contract.Candidates[0]
				if candidate.PrimaryEligible != (lane == "on_chain") || candidate.Decision.Magnitude.Value != 31 {
					t.Fatalf("caliber changed eligibility or value: %+v", candidate)
				}
				for _, language := range []string{"zh", "en"} {
					want := types.TraceIOValueCaliberLabel(caliber, language == "zh")
					if got := RootCauseValueDescriptionForLanguage(candidate.Decision, language); !strings.Contains(got, want) {
						t.Errorf("reader value lost ruler: %q want %q", got, want)
					}
				}
				if lane == "background" {
					return
				}
				contract.RootCauseReportEnabled = true
				report, err := BindRootCauseReportSelection(&types.TraceRootCauseReportV2{SchemaVersion: 2,
					RootCauses: []*types.TraceRootCauseItemV2{{CandidateID: candidate.Decision.CandidateID}}}, contract)
				if err != nil {
					t.Fatal(err)
				}
				item := report.RootCauses[0]
				if !strings.Contains(strings.Join(item.Evidence, " "), types.TraceIOValueCaliberLabel(caliber, true)) ||
					item.ImpactSeconds == nil || *item.ImpactSeconds != .031 {
					t.Fatalf("sidecar lost selected ruler/value: %+v", item)
				}
			})
		}
	}
}
