package types

// trace_causal_projection_top_io_inode_test.go — INODE (§28.6, 2026-07-09)
// negative pin: the top_io_inode observation family is a STATISTICAL
// enumeration face, not an attribution face — its rows must never be
// consumed into the causal projection tree (no node in any bucket, no
// anchor-window contribution, no rank/blocking promotion). The projection
// compile is predicate-whitelisted; this pin turns a future "helpful"
// whitelist addition into a reviewed decision instead of a silent drift.

import (
	"encoding/json"
	"strings"
	"testing"
)

func topIOInodeProjectionRecord() ObservationRecord {
	return ObservationRecord{
		ID:              "trace_query:q1#top_io_inode:1",
		Origin:          AnswerEvidenceOriginRuntimeArtifact,
		Producer:        "trace_query",
		Role:            AnswerAggregateRoleSupportingCoverage,
		GroundingPolicy: ClaimGroundingHard,
		SourceRef:       ObservationSourceRef{Kind: ObservationSourceRuntimeArtifact, ArtifactID: "berlin.systrace"},
		Span:            ObservationSpan{LineStart: 100, LineEnd: 120, StartTs: 10.001, EndTs: 10.006},
		ClaimKey:        "top_io_inode:0xaa",
		Subject:         "hot.db",
		Predicate:       "top_io_inode",
		Object:          "259:1",
		Value:           "7",
		Unit:            "events",
		RichNotes: []string{
			"inode=0xaa", "dev=259:1", "name=hot.db", "count=7", "reads=2",
			"writes=1", "max_latency=7.000", "threads=2", "groups_total=2",
		},
		SupportRefs: []string{"berlin.systrace:100-120"},
		Confidence:  0.74,
	}
}

// TestTraceCausalProjectionIgnoresTopIOInodeRows: alone, the statistical row
// compiles to an EMPTY projection; beside a real causal record, the compiled
// tree carries no trace of it in any bucket or path surface.
func TestTraceCausalProjectionIgnoresTopIOInodeRows(t *testing.T) {
	alone := TraceCausalProjectionFromObservationRecords([]ObservationRecord{topIOInodeProjectionRecord()})
	if alone.Active() {
		t.Fatalf("a lone top_io_inode row must compile to an inactive projection: %+v", alone)
	}

	causal := anchorB1PathRecord("path-1", "app-100", "dep-200 -> app-100")
	withCausal := TraceCausalProjectionFromObservationRecords([]ObservationRecord{
		topIOInodeProjectionRecord(),
		causal,
	})
	if !withCausal.Active() {
		t.Fatalf("the causal record must still compile: %+v", withCausal)
	}
	blob, err := json.Marshal(withCausal)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"top_io_inode", "0xaa", "hot.db", "groups_total"} {
		if strings.Contains(string(blob), forbidden) {
			t.Fatalf("statistical enumeration row leaked into the causal projection (%q):\n%s", forbidden, blob)
		}
	}
}
