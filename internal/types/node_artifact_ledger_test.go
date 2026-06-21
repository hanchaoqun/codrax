package types

import "testing"

func TestNormalizeNodeArtifactRecordsDedupsAndIndexes(t *testing.T) {
	records := []NodeArtifactRecord{
		{
			ProducerNodeID: " explore ",
			ProducerStage:  PipelineStage(" explore "),
			Artifact: RuntimeArtifactRef{
				Kind:      RuntimeArtifactKind("made_up_kind"),
				Path:      " internal/agent/agent.go ",
				LineStart: 5,
				LineEnd:   3,
			},
			EvidenceID: " ev-1 ",
		},
		{
			ProducerNodeID: "explore",
			ProducerStage:  StageExplore,
			Artifact: RuntimeArtifactRef{
				Kind:      RuntimeArtifactUnknown,
				ID:        "internal/agent/agent.go:5",
				Path:      "internal/agent/agent.go",
				LineStart: 5,
				LineEnd:   5,
			},
			EvidenceID: "ev-1",
		},
		{
			ProducerNodeID: "",
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactEvidenceItem, ID: "drop"},
		},
		{
			ProducerNodeID: "explore",
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactSource},
		},
		{
			ProducerNodeID: "extract",
			Artifact: RuntimeArtifactRef{
				Kind: RuntimeArtifactEvidenceItem,
				ID:   "ev-2",
			},
		},
	}

	normalized := NormalizeNodeArtifactRecords(records)
	if len(normalized) != 2 {
		t.Fatalf("normalized records = %+v, want 2 valid deduped records", normalized)
	}
	var exploreRecord NodeArtifactRecord
	for _, record := range normalized {
		if record.ProducerNodeID == "explore" {
			exploreRecord = record
		}
	}
	if exploreRecord.Artifact.Kind != RuntimeArtifactUnknown {
		t.Fatalf("explore normalized record = %+v, want unknown artifact", exploreRecord)
	}
	if exploreRecord.Artifact.ID != "internal/agent/agent.go:5" || exploreRecord.Artifact.LineEnd != 5 {
		t.Fatalf("path-derived artifact identity = %+v", exploreRecord.Artifact)
	}

	ledger := NormalizeNodeArtifactLedger(NodeArtifactLedger{Records: normalized})
	if got := ledger.RecordsByProducer("explore"); len(got) != 1 || got[0].EvidenceID != "ev-1" {
		t.Fatalf("records by producer = %+v", got)
	}
	if got := ledger.RecordsByKind(RuntimeArtifactEvidenceItem); len(got) != 1 || got[0].ProducerNodeID != "extract" {
		t.Fatalf("records by kind = %+v", got)
	}
	if got := ledger.RecordsByKind(RuntimeArtifactKind("unexpected")); len(got) != 1 || got[0].ProducerNodeID != "explore" {
		t.Fatalf("unknown-kind lookup = %+v", got)
	}
}

func TestMergeNodeArtifactLedgersIsStableAndDeduped(t *testing.T) {
	a := NodeArtifactLedger{Records: []NodeArtifactRecord{{
		ProducerNodeID: "n2",
		Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactSource, ID: "b.go"},
	}}}
	b := NodeArtifactLedger{Records: []NodeArtifactRecord{
		{
			ProducerNodeID: "n1",
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactSource, ID: "a.go"},
		},
		{
			ProducerNodeID: "n2",
			Artifact:       RuntimeArtifactRef{Kind: RuntimeArtifactSource, ID: "b.go"},
		},
	}}

	merged := MergeNodeArtifactLedgers(a, b)
	if len(merged.Records) != 2 {
		t.Fatalf("merged records = %+v, want 2", merged.Records)
	}
	if merged.Records[0].ProducerNodeID != "n1" || merged.Records[1].ProducerNodeID != "n2" {
		t.Fatalf("merged records not sorted deterministically: %+v", merged.Records)
	}
}
