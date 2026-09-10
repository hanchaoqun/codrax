package types

import (
	"reflect"
	"testing"
)

func TestB1638B3AbsorbersRetainOriginsWithoutChangingExistingFields(t *testing.T) {
	for _, mode := range []string{"same_fact", "duplicate_publication", "dedupe_twin", "one_seat_evidence", "one_seat_merged_twin"} {
		for _, legacy := range []string{"neither", "keeper", "loser", "both"} {
			t.Run(mode+"/legacy_"+legacy, func(t *testing.T) {
				a, b := b1638B3OriginRow(0), b1638B3OriginRow(1)
				if legacy == "keeper" || legacy == "both" {
					a.MeasurementOrigins = nil
				}
				if legacy == "loser" || legacy == "both" {
					b.MeasurementOrigins = nil
				}
				b.EffectiveImpactMS, b.EffectiveImpactPublished = 1.5, true
				b.TargetImpactMS, b.BlockedReasonCaller = .5, "observed_caller"
				absorb := func(keeper, loser TraceCausalProjectionNode) TraceCausalProjectionNode {
					switch mode {
					case "same_fact":
						traceCausalProjectionAbsorbSameFact(&keeper, loser, map[string]bool{keeper.EvidenceID: true}, nil)
					case "duplicate_publication":
						traceCausalProjectionAbsorbDuplicatePublication(&keeper, loser)
					case "dedupe_twin":
						traceCausalProjectionAbsorbDedupeTwinEvidence(&keeper, loser)
					case "one_seat_evidence", "one_seat_merged_twin":
						out := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{keeper}}
						if mode == "one_seat_evidence" {
							traceCausalProjectionOneSeatAbsorbEvidence(&out, keeper.EvidenceID, loser, "S")
						} else {
							traceCausalProjectionOneSeatAbsorbMergedTwin(&out, keeper, loser)
						}
						keeper = out.OnChainCauses[0]
					}
					return keeper
				}
				var want []TraceSchedulerMeasurementOrigin
				for _, row := range []TraceCausalProjectionNode{a, b} {
					if len(row.MeasurementOrigins) == 0 {
						want = append(want, TraceSchedulerMeasurementOrigin{})
					} else {
						want = append(want, CloneTraceSchedulerMeasurementOrigins(row.MeasurementOrigins)...)
					}
				}
				plainA, plainB := a, b
				plainA.MeasurementOrigins, plainB.MeasurementOrigins = nil, nil
				wantFields := absorb(plainA, plainB)
				got := absorb(a, b)
				if !reflect.DeepEqual(got.MeasurementOrigins, want) {
					t.Fatalf("absorber dropped an exact or unknown contributor: got %d want %d", len(got.MeasurementOrigins), len(want))
				}
				beforeA, beforeB := CloneTraceSchedulerMeasurementOrigins(a.MeasurementOrigins), CloneTraceSchedulerMeasurementOrigins(b.MeasurementOrigins)
				for i := range got.MeasurementOrigins {
					o := &got.MeasurementOrigins[i]
					if o.MeasurementSources != nil {
						o.MeasurementSources.Domains[0].PartitionID = "changed"
						*o.SourceRef.ClockOffsetSec = 5
						*o.SourceRef.ClockSlope = 6
					}
				}
				if !reflect.DeepEqual(a.MeasurementOrigins, beforeA) || !reflect.DeepEqual(b.MeasurementOrigins, beforeB) {
					t.Fatal("absorbed sources alias input memory")
				}
				got.MeasurementOrigins, wantFields.MeasurementOrigins = nil, nil
				if !reflect.DeepEqual(got, wantFields) {
					t.Fatal("source metadata changed an existing value, identity, or disclosure field")
				}
			})
		}
	}
}

func TestB1638B3OriginMetadataDoesNotChangeExactStateAccountOneSeatGate(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "same_complete_key", false: "different_complete_key"}[sameKey], func(t *testing.T) {
			rank, impact := b1638B3OriginRow(0), b1638B3OriginRow(1)
			rank.Predicate, rank.Rank, rank.StateAccountKey = "root_cause_rank", 1, "full-native-account-A"
			impact.StateAccountKey = rank.StateAccountKey
			if !sameKey {
				impact.StateAccountKey = "full-native-account-B"
			}
			out := TraceCausalProjection{OnChainCauses: []TraceCausalProjectionNode{rank, impact}}
			traceCausalProjectionConvergeStateAccountPublications(&out)
			if !sameKey {
				if !reflect.DeepEqual(out.OnChainCauses, []TraceCausalProjectionNode{rank, impact}) {
					t.Fatal("source metadata changed the existing exact account gate")
				}
				return
			}
			if len(out.OnChainCauses) != 1 || out.OnChainCauses[0].EvidenceID != rank.EvidenceID {
				t.Fatal("exact cross-view account no longer converged")
			}
			want := append(CloneTraceSchedulerMeasurementOrigins(rank.MeasurementOrigins), impact.MeasurementOrigins...)
			if !reflect.DeepEqual(out.OnChainCauses[0].MeasurementOrigins, want) {
				t.Fatal("exact account convergence erased the absorbed view's source")
			}
			if out.OnChainCauses[0].ImpactMS != rank.ImpactMS || out.OnChainCauses[0].Rank != rank.Rank {
				t.Fatal("source propagation changed exact account value or rank")
			}
		})
	}
}
