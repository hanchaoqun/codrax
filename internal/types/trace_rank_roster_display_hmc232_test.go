package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestHMC232RankRosterCaliberPropagationDoesNotChangeAuthority(t *testing.T) {
	for _, channel := range []string{"on_chain", "adjacent", "background", ""} {
		for _, caliber := range []string{TraceIOValueCaliberRQResidence, TraceIOValueCaliberBIOResidence, TraceIOValueCaliberIssuerBlocked, TraceIOValueCaliberMixed, "", "future_unknown"} {
			n := traceRankRosterTestNode("record-1", "target", "params", 1, 2, 1, "io_latency", "issuer", 35)
			n.ChainRelevance = channel
			set := TraceCausalProjectionSet{Projections: []TraceCausalProjection{{ArtifactLabel: "trace", RankedSeats: []TraceCausalProjectionNode{n}}}}
			before := BuildTraceRankRosterAuthorities(set)
			set.Projections[0].RankedSeats[0].IOValueCaliber = caliber
			got := BuildTraceRankRosterAuthorities(set)
			if len(got) != 1 || len(got[0].Seats) != 1 || got[0].Seats[0].IOValueCaliber != caliber {
				t.Fatalf("ruler lost: %+v", got)
			}
			// The new scalar is descriptive, not a reconstructable credential.
			data, err := json.Marshal(got)
			if err != nil {
				t.Fatal(err)
			}
			var roundtrip []TraceRankRosterAuthority
			if err := json.Unmarshal(data, &roundtrip); err != nil || !reflect.DeepEqual(got, roundtrip) {
				t.Fatal("scalar copy lost roster fields")
			}
			got[0].Seats[0].IOValueCaliber = ""
			if !reflect.DeepEqual(before, got) {
				t.Fatal("ruler changed rank/board/channel/qualification")
			}
		}
	}
}
