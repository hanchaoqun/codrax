package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

// Source-member proof qualifies the member, not the model's unrelated runtime
// scope. Exercise the real aggregate compiler, not a hand-stamped authority.
func TestRuntimeArtifactSourceClaimBoundaryIndependentMemberIsNotCapture(t *testing.T) {
	fact, rm, input := b1700SourceMemberFixture()
	fact.Provenance = "trace_query:source-member-review"
	fact.Dimensions = []AnswerAggregateDimension{{Name: "scope", Value: "worker_pid=41"}}
	input.AggregateFacts, input.RequestModel = []AnswerAggregateFact{fact}, &rm
	native := runtimeArtifactPairTestRecord("capture", "/captures/one.ftrace", "trace_seconds", "trace_seconds", "identity")
	native.ID, native.ClaimAuthority = "native-one", ObservationClaimAuthorityDirectObservation
	input.ToolResults = []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{native}}}
	before, _ := json.Marshal(input)
	ledger := CompileObservationLedger(input)
	var aggregate *ObservationRecord
	for i := range ledger.Records {
		if ledger.Records[i].ID == "aggregate:0#runtime_artifact" {
			aggregate = &ledger.Records[i]
		}
	}
	if aggregate == nil || aggregate.ClaimAuthority != ObservationClaimAuthorityIndependentlyProven ||
		aggregate.SourceRef.ArtifactID != "worker_pid=41" || aggregate.Value != "1" {
		t.Fatalf("fixture must retain independently proven member with model-owned scope: %+v", aggregate)
	}
	for name, candidate := range runtimeArtifactSourceClaimBoundaryRoundTrips(t, ledger) {
		if candidate.RuntimeArtifactSources == nil || len(candidate.RuntimeArtifactSources.Sources) != 1 {
			t.Fatalf("%s: source-member proof entered the physical-source receipt: %+v", name, candidate.RuntimeArtifactSources)
		}
		if got := BuildRuntimeArtifactPairRelationAuthority(candidate); got.Active || len(got.Pairs) != 0 {
			t.Errorf("%s: SOURCE_MEMBER_IS_NOT_CAPTURE: source proof minted an unrelated runtime endpoint: %+v", name, got)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("identity compilation changed source evidence or model facts")
	}
}

// A merged claim may become independently_proven, while a native observation
// already established the exact source tuple. Global claim-strength merging
// must neither mint a new identity nor erase that native source witness.
func TestRuntimeArtifactSourceClaimBoundaryMergePreservesNativeCapture(t *testing.T) {
	fact, rm, input := b1700SourceMemberFixture()
	fact.Provenance = "trace_query:source-member-review"
	fact.Label = "Worker"
	fact.Dimensions = []AnswerAggregateDimension{
		{Name: "artifact_id", Value: "capture-a"},
		{Name: "artifact_kind", Value: "trace"},
		{Name: "artifact_path", Value: "/captures/a.ftrace"},
		{Name: "payload_ref", Value: "/payload/a.json"},
		{Name: "tool_call_id", Value: "trace_query[0]"},
	}
	input.AggregateFacts, input.RequestModel = []AnswerAggregateFact{fact}, &rm
	// Derive the exact legacy-compatible source tuple through the aggregate
	// compiler; no model text, note, or filename inference proves the capture.
	aggregateLedger := CompileObservationLedger(input)
	var native ObservationRecord
	for _, record := range aggregateLedger.Records {
		if record.ID == "aggregate:0#runtime_artifact" {
			native = record
		}
	}
	if native.ClaimAuthority != ObservationClaimAuthorityIndependentlyProven {
		t.Fatalf("source member did not acquire its legitimate proof: %+v", native)
	}
	native.ID, native.Producer, native.ClaimAuthority = "native-a", "trace_query", ObservationClaimAuthorityDirectObservation
	native.ModelNotes, native.RichNotes = nil, nil
	other := runtimeArtifactPairTestRecord("capture-b", "/captures/b.ftrace", "", "", "")
	other.ID, other.ClaimAuthority = "native-b", ObservationClaimAuthorityDirectObservation
	input.ToolResults = []ToolResult{{ToolName: "trace_query", Success: true, Observations: []ObservationRecord{native, other}}}
	before, _ := json.Marshal(input)
	ledger := CompileObservationLedger(input)
	matching := 0
	for _, record := range ledger.Records {
		if record.Origin == AnswerEvidenceOriginRuntimeArtifact && record.SourceRef.Path == native.SourceRef.Path {
			matching++
			if !reflect.DeepEqual(record.SourceRef, native.SourceRef) || record.ClaimAuthority != ObservationClaimAuthorityIndependentlyProven {
				t.Fatalf("global claim-strength semantics changed: %+v", record)
			}
		}
	}
	if matching != 1 {
		t.Fatalf("fixture must actually merge the exact source tuples, got %d", matching)
	}
	for name, candidate := range runtimeArtifactSourceClaimBoundaryRoundTrips(t, ledger) {
		if candidate.RuntimeArtifactSources == nil || len(candidate.RuntimeArtifactSources.Sources) != 2 {
			t.Fatalf("%s: pre-merge physical-source receipts lost: %+v", name, candidate.RuntimeArtifactSources)
		}
		got := BuildRuntimeArtifactPairRelationAuthority(candidate)
		if !got.Active || len(got.Artifacts) != 2 || len(got.Pairs) != 1 {
			t.Errorf("%s: NATIVE_CAPTURE_ERASED_BY_CLAIM_MERGE: %+v", name, got)
			continue
		}
		paths := map[string]bool{}
		for _, artifact := range got.Artifacts {
			paths[artifact.Path] = true
		}
		if !paths["/captures/a.ftrace"] || !paths["/captures/b.ftrace"] {
			t.Errorf("%s: capture identities changed: %+v", name, got)
		}
		// Deliberately discard the new receipt to model an old ledger. A
		// direct-only post-merge scan cannot recover capture-a from the stronger
		// independently-proven claim; this is why the pre-merge snapshot matters.
		legacy := candidate
		legacy.RuntimeArtifactSources = nil
		if legacyView := BuildRuntimeArtifactPairRelationAuthority(legacy); legacyView.Active || len(legacyView.Pairs) != 0 {
			t.Errorf("%s: fixture did not demonstrate the post-merge direct-only identity loss: %+v", name, legacyView)
		}
	}
	after, _ := json.Marshal(input)
	if string(before) != string(after) {
		t.Fatal("identity compilation changed native/source inputs")
	}
}

func runtimeArtifactSourceClaimBoundaryRoundTrips(t *testing.T, ledger ObservationLedger) map[string]ObservationLedger {
	t.Helper()
	raw, err := json.Marshal(ledger)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ObservationLedger
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	return map[string]ObservationLedger{"compiled": ledger, "json_round_trip": decoded}
}
