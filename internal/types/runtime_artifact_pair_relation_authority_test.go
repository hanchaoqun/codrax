package types

import "testing"

func TestBuildRuntimeArtifactPairRelationAuthority_LocalIdentityDoesNotProveCrossArtifactRelation(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		runtimeArtifactPairTestRecord("a", "a.systrace", "trace_seconds", "trace_seconds", "identity"),
		runtimeArtifactPairTestRecord("a", "a.systrace", "trace_seconds", "trace_seconds", "identity"),
		runtimeArtifactPairTestRecord("b", "b.systrace", "trace_seconds", "trace_seconds", "identity"),
	}}
	got := BuildRuntimeArtifactPairRelationAuthority(ledger)
	if !got.Active || len(got.Artifacts) != 2 || len(got.Pairs) != 1 {
		t.Fatalf("authority shape = %+v", got)
	}
	pair := got.Pairs[0]
	if pair.SharedClockOrigin != RuntimeArtifactPairRelationUnproven ||
		pair.DirectTimeAlignment != RuntimeArtifactPairRelationUnproven ||
		pair.SharedDevice != RuntimeArtifactPairRelationUnproven ||
		pair.SharedCaptureSession != RuntimeArtifactPairRelationUnproven {
		t.Fatalf("independent artifacts gained relation authority: %+v", pair)
	}
	if !pair.SameTimeDomainLabel || !pair.SameCanonicalDomain || !pair.LocalIdentityOnly {
		t.Fatalf("local endpoint labels were not preserved separately: %+v", pair)
	}
}

func TestBuildRuntimeArtifactPairRelationAuthority_DifferentLabelsStillDoNotProveDifferentOrigin(t *testing.T) {
	ledger := ObservationLedger{Records: []ObservationRecord{
		runtimeArtifactPairTestRecord("a", "a.systrace", "trace_seconds", "trace_seconds", "identity"),
		runtimeArtifactPairTestRecord("b", "b.perftrace", "perf_event_time", "perf_event_time", "identity"),
	}}
	got := BuildRuntimeArtifactPairRelationAuthority(ledger)
	if !got.Active || len(got.Pairs) != 1 {
		t.Fatalf("authority shape = %+v", got)
	}
	pair := got.Pairs[0]
	if pair.SameTimeDomainLabel || pair.SameCanonicalDomain {
		t.Fatalf("different local labels reported equal: %+v", pair)
	}
	if pair.SharedClockOrigin != RuntimeArtifactPairRelationUnproven ||
		pair.DirectTimeAlignment != RuntimeArtifactPairRelationUnproven {
		t.Fatalf("different labels must remain an unproven cross-artifact relation: %+v", pair)
	}
}

func TestBuildRuntimeArtifactPairRelationAuthority_SingleArtifactInactive(t *testing.T) {
	got := BuildRuntimeArtifactPairRelationAuthority(ObservationLedger{Records: []ObservationRecord{
		runtimeArtifactPairTestRecord("a", "a.systrace", "trace_seconds", "trace_seconds", "identity"),
	}})
	if got.Active || len(got.Artifacts) != 0 || len(got.Pairs) != 0 {
		t.Fatalf("single artifact should not publish pair authority: %+v", got)
	}
}

func TestBuildRuntimeArtifactPairRelationAuthority_PathIsPrimaryAcrossIDAliases(t *testing.T) {
	got := BuildRuntimeArtifactPairRelationAuthority(ObservationLedger{Records: []ObservationRecord{
		runtimeArtifactPairTestRecord("attached_trace", `D:\trace\one.sys`, "trace_seconds", "trace_seconds", "identity"),
		runtimeArtifactPairTestRecord("attached_trace.txt", `d:/trace/one.sys`, "trace_seconds", "trace_seconds", "identity"),
	}})
	if got.Active || len(got.Artifacts) != 0 || len(got.Pairs) != 0 {
		t.Fatalf("one canonical physical path with ID aliases must remain one artifact: %+v", got)
	}
}

func TestBuildRuntimeArtifactPairRelationAuthority_CaptureIdentityPrecedesMaterializedPaths(t *testing.T) {
	original := runtimeArtifactPairTestRecord("trace_query", "/repo/frame.systrace", "trace_seconds", "trace_seconds", "identity")
	blob := runtimeArtifactPairTestRecord("attached_trace", "/repo/.codrax/blob/session/attached_trace.txt", "trace_seconds", "trace_seconds", "identity")
	original.SourceRef.CaptureIdentityPath = "frame.systrace"
	blob.SourceRef.CaptureIdentityPath = "frame.systrace"
	got := BuildRuntimeArtifactPairRelationAuthority(ObservationLedger{Records: []ObservationRecord{original, blob}})
	if got.Active || len(got.Artifacts) != 0 || len(got.Pairs) != 0 {
		t.Fatalf("one capture behind two addressable carriers must remain one artifact: %+v", got)
	}
}

func TestBuildRuntimeArtifactPairRelationAuthority_ReusedIDRemainsDistinctByTypedPath(t *testing.T) {
	left := runtimeArtifactPairTestRecord("trace_query", "a.systrace", "trace_seconds", "trace_seconds", "identity")
	right := runtimeArtifactPairTestRecord("trace_query", "b.systrace", "trace_seconds", "trace_seconds", "identity")
	got := BuildRuntimeArtifactPairRelationAuthority(ObservationLedger{Records: []ObservationRecord{left, right}})
	if !got.Active || len(got.Artifacts) != 2 || len(got.Pairs) != 1 {
		t.Fatalf("reused producer IDs must remain distinct by typed source path: %+v", got)
	}
	if got.Artifacts[0].ArtifactID == got.Artifacts[1].ArtifactID {
		t.Fatalf("path fallback identities collided: %+v", got.Artifacts)
	}
}

func runtimeArtifactPairTestRecord(id, path, domain, canonical, alignment string) ObservationRecord {
	return ObservationRecord{
		Origin:   AnswerEvidenceOriginRuntimeArtifact,
		Producer: "trace_query",
		SourceRef: ObservationSourceRef{
			Kind:                ObservationSourceRuntimeArtifact,
			ArtifactID:          id,
			ArtifactKind:        "trace",
			Path:                path,
			TimeDomain:          domain,
			CanonicalTimeDomain: canonical,
			ClockAlignment:      alignment,
		},
	}
}
