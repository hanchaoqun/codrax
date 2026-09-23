package tracequery

import "testing"

func TestIOInFlightEmptyPopulationPreservesRealDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name     string
		coverage IOInFlightPairingCoverage
		present  bool
	}{
		{"no_observation_or_diagnostic", IOInFlightPairingCoverage{Status: IOInFlightCoverageAvailable}, false},
		{"unresolved_physical_source", IOInFlightPairingCoverage{Status: IOInFlightCoverageUnavailable, UnresolvedSources: 1}, true},
		{"topology_unavailable", IOInFlightPairingCoverage{Status: IOInFlightCoverageUnavailable, Reasons: []string{"pairing_topology_incomplete"}}, true},
		{"quarantined_lane", IOInFlightPairingCoverage{Status: IOInFlightCoveragePartial}, true},
		{"unpaired_done", IOInFlightPairingCoverage{Status: IOInFlightCoverageAvailable, UnpairedDoneCount: 1}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.coverage.Family = "block"
			got := buildIOInFlightStats(Query{TimeStart: 1, TimeEnd: 2}, blockPairingResult{coverage: tc.coverage}, storagePairingResult{})
			if (got != nil) != tc.present {
				t.Fatalf("empty-population disposition mismatch: %+v", got)
			}
			if got != nil && (len(got.Groups) != 0 || len(got.Coverage) != 2 || got.Coverage[0].Status != tc.coverage.Status || got.Coverage[0].UnresolvedSources != tc.coverage.UnresolvedSources) {
				t.Fatalf("real diagnostic was rewritten or promoted to measured zero: %+v", got)
			}
		})
	}
}

func TestIOInFlightCompleteZeroDurationPairIsNotAbsent(t *testing.T) {
	got := buildIOInFlightStats(Query{TimeStart: 1, TimeEnd: 2}, blockPairingResult{census: []IOLatencySummary{{SourcePath: "capture", EndpointFamily: blockEndpointFamilyRQ, IssueTs: 1.5, CompleteTs: 1.5}}}, storagePairingResult{})
	if got == nil || len(got.Groups) != 1 || got.Groups[0].AcceptedPairCount != 1 || got.Groups[0].Values == nil {
		t.Fatalf("admitted zero-duration pair lost measured zero: %+v", got)
	}
	if *got.Groups[0].Values != (IOInFlightValues{}) {
		t.Fatalf("zero-duration pair changed values: %+v", got.Groups[0].Values)
	}
}
