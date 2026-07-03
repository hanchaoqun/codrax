package types

import "testing"

// CMP-B pins for the exported record→artifact attribution helpers
// (TraceCausalProjectionRecordMatchesArtifact /
// TraceCausalProjectionRecordArtifactIdentity): same typed identity lanes as
// the partitioner, including the F5a suffix-alias relation; no-identity
// records and provenance-less projections never match.
func TestTraceCausalProjectionRecordMatchesArtifact(t *testing.T) {
	pathRecord := func(path string) ObservationRecord {
		return ObservationRecord{
			Origin:          AnswerEvidenceOriginRuntimeArtifact,
			Producer:        "trace_query",
			GroundingPolicy: ClaimGroundingHard,
			SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, Path: path},
		}
	}
	pathProjection := TraceCausalProjection{ArtifactPath: "traces/one.trace", ArtifactLabel: "one.trace"}
	if !TraceCausalProjectionRecordMatchesArtifact(pathRecord("traces/one.trace"), pathProjection) {
		t.Fatal("canonical path equality must match")
	}
	if !TraceCausalProjectionRecordMatchesArtifact(pathRecord("/repo/traces/one.trace"), pathProjection) {
		t.Fatal("F5a suffix-alias spelling (absolute vs relative) must match")
	}
	if TraceCausalProjectionRecordMatchesArtifact(pathRecord("other/two.trace"), pathProjection) {
		t.Fatal("a different artifact path must not match")
	}
	if TraceCausalProjectionRecordMatchesArtifact(pathRecord("one.trace"), pathProjection) {
		t.Fatal("a bare basename (1-segment) is the dir1/x-vs-dir2/x collision and must not alias-match")
	}

	idRecord := ObservationRecord{
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		GroundingPolicy: ClaimGroundingHard,
		SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "seven.systrace"},
	}
	idProjection := TraceCausalProjection{ArtifactLabel: "seven.systrace"}
	if !TraceCausalProjectionRecordMatchesArtifact(idRecord, idProjection) {
		t.Fatal("id-lane identity must match on verbatim label equality")
	}
	if TraceCausalProjectionRecordMatchesArtifact(idRecord, pathProjection) {
		t.Fatal("an id-lane record must not match a path-lane projection")
	}

	none := ObservationRecord{Origin: AnswerEvidenceOriginRuntimeArtifact, Producer: "trace_query", GroundingPolicy: ClaimGroundingHard}
	if got := TraceCausalProjectionRecordArtifactIdentity(none); got != "" {
		t.Fatalf("identity-less record must report an empty key, got %q", got)
	}
	if TraceCausalProjectionRecordMatchesArtifact(none, pathProjection) ||
		TraceCausalProjectionRecordMatchesArtifact(none, TraceCausalProjection{}) {
		t.Fatal("identity-less records and provenance-less projections never match")
	}
	if got := TraceCausalProjectionRecordArtifactIdentity(idRecord); got == "" {
		t.Fatal("id-lane record must report a non-empty identity key")
	}
}
