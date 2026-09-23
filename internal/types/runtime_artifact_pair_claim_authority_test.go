package types

import (
	"encoding/json"
	"testing"
)

func TestRuntimeArtifactPairSourceAuthority(t *testing.T) {
	for _, tc := range []struct {
		name      string
		authority ObservationClaimAuthority
		lane      ObservationProvenanceLane
		wantPair  bool
	}{
		{"native", ObservationClaimAuthorityDirectObservation, ObservationProvenanceUnknown, true},
		{"independently_proven", ObservationClaimAuthorityIndependentlyProven, ObservationProvenanceUnknown, false},
		{"legacy", ObservationClaimAuthorityUnknown, ObservationProvenanceUnknown, true},
		{"model", ObservationClaimAuthorityModelInference, ObservationProvenanceUnknown, false},
		{"legacy_possibility", ObservationClaimAuthorityUnknown, ObservationProvenanceInferredUpstreamPossibility, false},
		{"invalid", ObservationClaimAuthority("future_unknown_authority"), ObservationProvenanceUnknown, false},
	} {
		for _, path := range []string{"", "/captures/second.trace"} {
			t.Run(tc.name+"/"+path, func(t *testing.T) {
				first := runtimeArtifactPairTestRecord("first", "/captures/first.trace", "trace_seconds", "trace_seconds", "identity")
				second := runtimeArtifactPairTestRecord("scope-41", path, "trace_seconds", "trace_seconds", "identity")
				second.Producer = "trace_query:thread_timeline+window_stats"
				second.ClaimAuthority, second.ProvenanceLane = tc.authority, tc.lane
				ledger := ObservationLedger{Records: []ObservationRecord{first, second}}
				before, _ := json.Marshal(ledger)
				got := BuildRuntimeArtifactPairRelationAuthority(ledger)
				if got.Active != tc.wantPair {
					t.Fatalf("source authority=%q path=%q: active=%v want=%v: %+v", tc.authority, path, got.Active, tc.wantPair, got)
				}
				if tc.wantPair && (len(got.Pairs) != 1 || got.Pairs[0].SharedClockOrigin != RuntimeArtifactPairRelationUnproven) {
					t.Fatalf("independent sources lost or gained clock authority: %+v", got)
				}
				after, _ := json.Marshal(ledger)
				if string(before) != string(after) {
					t.Fatal("pair projection mutated retained observations")
				}
			})
		}
	}
}

func TestRuntimeArtifactPairModelCannotOwnDerivedCarriers(t *testing.T) {
	for _, field := range []string{"payload", "raw", "rowset", "page"} {
		for _, reverse := range []bool{false, true} {
			t.Run(field+map[bool]string{false: "/forward", true: "/reverse"}[reverse], func(t *testing.T) {
				left := runtimeArtifactPairTestRecord("a", "/captures/a.trace", "trace_seconds", "trace_seconds", "identity")
				right := runtimeArtifactPairTestRecord("b", "/captures/b.trace", "trace_seconds", "trace_seconds", "identity")
				left.SourceRef.PayloadRef = "/derived/query.json"
				carrier := runtimeArtifactPairTestRecord("query", left.SourceRef.PayloadRef, "trace_seconds", "trace_seconds", "identity")
				forged := right
				forged.ClaimAuthority = ObservationClaimAuthorityModelInference
				switch field {
				case "payload":
					forged.SourceRef.PayloadRef = carrier.SourceRef.Path
				case "raw":
					forged.SourceRef.RawRef = carrier.SourceRef.Path
				case "rowset":
					forged.SourceRef.RowSetRef = carrier.SourceRef.Path
				case "page":
					forged.SourceRef.PageRef = carrier.SourceRef.Path
				}
				rows := []ObservationRecord{left, right, carrier, forged}
				if reverse {
					for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
						rows[i], rows[j] = rows[j], rows[i]
					}
				}
				got := BuildRuntimeArtifactPairRelationAuthority(ObservationLedger{Records: rows})
				if len(got.Artifacts) != 2 || len(got.Pairs) != 1 {
					t.Fatalf("model owner poisoned unique native ownership: %+v", got)
				}
			})
		}
	}
}

func TestRuntimeArtifactPairModelCannotBridgeDerivedOwnership(t *testing.T) {
	first := runtimeArtifactPairTestRecord("a", "/captures/a.trace", "trace_seconds", "trace_seconds", "identity")
	first.SourceRef.PayloadRef = "/derived/query.json"
	bridge := runtimeArtifactPairTestRecord("bridge", first.SourceRef.PayloadRef, "trace_seconds", "trace_seconds", "identity")
	bridge.ClaimAuthority = ObservationClaimAuthorityModelInference
	bridge.SourceRef.PayloadRef = "/captures/b.trace"
	second := runtimeArtifactPairTestRecord("b", bridge.SourceRef.PayloadRef, "trace_seconds", "trace_seconds", "identity")
	got := BuildRuntimeArtifactPairRelationAuthority(ObservationLedger{Records: []ObservationRecord{first, bridge, second}})
	if len(got.Artifacts) != 2 || len(got.Pairs) != 1 {
		t.Fatalf("model link collapsed two independent native captures: %+v", got)
	}
}
