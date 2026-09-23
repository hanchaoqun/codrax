package types

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestRuntimeArtifactSourceSnapshotOwnsMetadata(t *testing.T) {
	record := runtimeArtifactPairTestRecord("a", "/captures/a", "trace_seconds", "trace_seconds", "identity")
	record.ClaimAuthority = ObservationClaimAuthorityDirectObservation
	offset, slope := 3.0, 1.0
	record.SourceRef.ClockOffsetSec, record.SourceRef.ClockSlope = &offset, &slope
	record.SourceRef.TraceExcerptScope = &PerfObservationSourceScope{}
	second := record
	otherOffset, otherSlope := offset, slope
	second.SourceRef.ClockOffsetSec, second.SourceRef.ClockSlope = &otherOffset, &otherSlope
	second.SourceRef.TraceExcerptScope = &PerfObservationSourceScope{}
	got := compileRuntimeArtifactSourceIdentities([]ObservationRecord{record, second})
	if len(got.Sources) != 1 {
		t.Fatalf("same values at different addresses not deduplicated: %+v", got)
	}
	ref := &got.Sources[0]
	if !reflect.DeepEqual(*ref, record.SourceRef) || ref.ClockOffsetSec == record.SourceRef.ClockOffsetSec ||
		ref.ClockSlope == record.SourceRef.ClockSlope || ref.TraceExcerptScope == record.SourceRef.TraceExcerptScope {
		t.Fatal("source metadata not independently preserved")
	}
	*ref.ClockOffsetSec, *ref.ClockSlope = 4, 2
	if offset != 3 || slope != 1 {
		t.Fatal("snapshot modified original source")
	}
	offset, slope = 5, 3
	if *ref.ClockOffsetSec != 4 || *ref.ClockSlope != 2 {
		t.Fatal("original source modified snapshot")
	}
}

func TestRuntimeArtifactSourceSnapshotEmptyIsNotLegacy(t *testing.T) {
	records := []ObservationRecord{
		runtimeArtifactPairTestRecord("a", "/a", "trace_seconds", "trace_seconds", "identity"),
		runtimeArtifactPairTestRecord("b", "/b", "trace_seconds", "trace_seconds", "identity"),
	}
	for _, snapshot := range []*RuntimeArtifactSourceIdentitySet{nil, {}} {
		ledger := ObservationLedger{Records: records, RuntimeArtifactSources: snapshot}
		data, err := json.Marshal(ledger)
		if err != nil {
			t.Fatal(err)
		}
		var restored ObservationLedger
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if got := BuildRuntimeArtifactPairRelationAuthority(restored); got.Active != (snapshot == nil) {
			t.Fatalf("empty source receipt fell back to claim rows, or legacy lost: %+v", got)
		}
	}
	compiled := CompileObservationLedger(ObservationLedgerInput{})
	if compiled.RuntimeArtifactSources == nil || len(compiled.RuntimeArtifactSources.Sources) != 0 {
		t.Fatal("new empty ledger must carry authoritative empty receipt")
	}
}
