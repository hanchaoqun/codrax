package types

import "testing"

func TestEvidenceClosureIngestRoundReadCoverageAndAcceptedEvidence(t *testing.T) {
	c := NewEvidenceClosure("")
	results := []ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[./a.go: showing lines 2-5 of 10 total]\ncode",
		},
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[forced_read] [b.go: showing all 3 lines (90 bytes); limit=1 expanded]\ncode",
		},
		{
			ToolName: "emit_evidence",
			Success:  true,
			Handoff: &ToolHandoffCarrier{
				Version:          ToolHandoffCarrierVersion,
				ToolName:         "emit_evidence",
				AcceptedEvidence: []AcceptedEvidenceRef{{ID: "ev-1", Source: "a.go", LineStart: 2}},
			},
		},
	}

	delta := c.IngestRound(results, "")
	if delta.Empty() {
		t.Fatal("expected non-empty delta")
	}
	if !delta.ReadSet["a.go"] || !delta.ReadSet["b.go"] {
		t.Fatalf("delta read set = %+v", delta.ReadSet)
	}
	if got := c.ReadRanges("a.go"); len(got) != 1 || got[0].Start != 2 || got[0].End != 5 {
		t.Fatalf("a.go ranges = %+v", got)
	}
	if got := c.FileTotalLines("b.go"); got != 3 {
		t.Fatalf("b.go total = %d, want 3", got)
	}
	if refs := c.AcceptedEvidenceRefs(); len(refs) != 1 || refs[0].ID != "ev-1" {
		t.Fatalf("accepted evidence refs = %+v", refs)
	}
}

func TestEvidenceRoundDeltaMatchesExtractReadCoverage(t *testing.T) {
	results := []ToolResult{
		{ToolName: "read_file", Success: true, Summary: "[a.go: showing lines 1-2 of 5 total]\n"},
		{ToolName: "read_file", Success: false, Summary: "[ignored.go: showing lines 1-2 of 5 total]\n"},
	}
	readSet, readRanges, totals := ExtractReadCoverage(results, "")
	delta := EvidenceRoundDeltaFromToolResults(results, "")
	if len(delta.ReadSet) != len(readSet) || !delta.ReadSet["a.go"] {
		t.Fatalf("delta read set = %+v, want %+v", delta.ReadSet, readSet)
	}
	if len(delta.ReadRanges["a.go"]) != len(readRanges["a.go"]) ||
		delta.ReadRanges["a.go"][0] != readRanges["a.go"][0] {
		t.Fatalf("delta ranges = %+v, want %+v", delta.ReadRanges, readRanges)
	}
	if delta.FileTotalLines["a.go"] != totals["a.go"] {
		t.Fatalf("delta totals = %+v, want %+v", delta.FileTotalLines, totals)
	}
}
