package types

import (
	"reflect"
	"testing"
)

func traceRankBoardDisplayRecordFixture() ObservationRecord {
	return ObservationRecord{
		ID:        "trace_query:fixture#root_cause_rank:1",
		SourceRef: ObservationSourceRef{CaptureIdentityPath: "/capture/original.systrace", Path: "/repo/.codrax/blob/materialized.systrace"},
		RichNotes: []string{"selected_window=1.000000..2.000000", "rank_board_target=ui-42", "rank_board_params_fingerprint=abc123"},
	}
}

func TestTraceRankBoardDisplayIdentityReusesCaptureAndRankDomain(t *testing.T) {
	record := traceRankBoardDisplayRecordFixture()
	node := TraceCausalProjectionNode{RankBoardTarget: "ui-42", RankBoardParamsFingerprint: "abc123", QueryWindowStartTs: 1, QueryWindowEndTs: 2}
	projection := TraceCausalProjection{ArtifactPath: "/capture/original.systrace", ArtifactLabel: "original.systrace"}
	fromRecord := TraceRankBoardDisplayIdentityFromRecord(record)
	fromNode := TraceRankBoardDisplayIdentityFromNode(projection, node)
	if !fromRecord.Complete || !reflect.DeepEqual(fromRecord, fromNode) {
		t.Fatalf("record and node must share one capture/window/target/params identity: %+v / %+v", fromRecord, fromNode)
	}
	artifactKey := TraceCausalProjectionRecordArtifactIdentity(record)
	if fromRecord.Key != artifactKey+"\x00"+traceCausalProjectionRankBoardIdentityKey(node) || fromRecord.ArtifactPath != projection.ArtifactPath {
		t.Fatalf("display must reuse existing identity calculations, not materialized path/label: %+v", fromRecord)
	}
	node.QueryWindowStartTs, node.QueryWindowEndTs = 20, 30
	node.RankQueryWindowStartTs, node.RankQueryWindowEndTs = 1, 2
	if got := TraceRankBoardDisplayIdentityFromNode(projection, node); !reflect.DeepEqual(got, fromNode) {
		t.Fatalf("the rank donor's own window must precede merged host window: %+v", got)
	}
	other := projection
	other.ArtifactPath = "/different/original.systrace"
	if TraceRankBoardDisplayIdentityFromNode(other, node).Key == fromNode.Key {
		t.Fatal("same basename cannot merge different physical capture paths")
	}
	for _, tc := range []struct {
		name   string
		mutate func(*TraceCausalProjectionNode)
	}{
		{"target", func(n *TraceCausalProjectionNode) { n.RankBoardTarget = "ui-43" }},
		{"params", func(n *TraceCausalProjectionNode) { n.RankBoardParamsFingerprint = "def456" }},
		{"query window", func(n *TraceCausalProjectionNode) { n.RankQueryWindowEndTs = 2.000020 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			changed := node
			tc.mutate(&changed)
			if got := TraceRankBoardDisplayIdentityFromNode(projection, changed); !got.Complete || got.Key == fromNode.Key {
				t.Fatalf("a changed typed board axis must stay a distinct domain: %+v", got)
			}
		})
	}
}

func TestTraceRankBoardDisplayIdentityUnknownAxesRemainUnmergeable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(*ObservationRecord)
	}{
		{"missing capture", func(r *ObservationRecord) { r.SourceRef = ObservationSourceRef{} }},
		{"lane marker is not capture", func(r *ObservationRecord) { r.SourceRef = ObservationSourceRef{ArtifactID: "attached_trace"} }},
		{"missing target", func(r *ObservationRecord) { r.RichNotes[1] = "rank_board_target=" }},
		{"missing params", func(r *ObservationRecord) { r.RichNotes[2] = "rank_board_params_fingerprint=" }},
		{"missing window", func(r *ObservationRecord) { r.RichNotes[0] = "selected_window=" }},
		{"malformed window", func(r *ObservationRecord) { r.RichNotes[0] = "selected_window=1..NaN" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			record := traceRankBoardDisplayRecordFixture()
			tc.mutate(&record)
			if got := TraceRankBoardDisplayIdentityFromRecord(record); got.Complete || got.Key != "" {
				t.Fatalf("missing identity is not an arbitrary common board: %+v", got)
			}
		})
	}
	record := traceRankBoardDisplayRecordFixture()
	record.SourceRef = ObservationSourceRef{ArtifactID: "capture-id-42"}
	got := TraceRankBoardDisplayIdentityFromRecord(record)
	projection := TraceCausalProjection{ArtifactLabel: "capture-id-42"}
	node := TraceCausalProjectionNode{RankBoardTarget: "ui-42", RankBoardParamsFingerprint: "abc123", QueryWindowStartTs: 1, QueryWindowEndTs: 2}
	if !got.Complete || !reflect.DeepEqual(got, TraceRankBoardDisplayIdentityFromNode(projection, node)) {
		t.Fatalf("the existing typed bare artifact-ID lane must remain valid: %+v", got)
	}
}
