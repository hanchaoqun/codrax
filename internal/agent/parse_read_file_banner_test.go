package agent

import (
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// The banner format is load-bearing for three consumers in explorer.go
// (extractFileCoverage, detectTruncatedUngrepped,
// detectPartiallyReadSymbols). This test suite locks the shapes the
// read_file tool emits in internal/tool/builtin.go so a future banner
// change that breaks parsing is caught at CI time rather than at
// "citations look right but CGEC dropped them all" time.

func TestParseReadFileBanner_ShowingLines(t *testing.T) {
	summary := "[a.go: showing lines 10-25 of 100 total]\n  10│ func Foo() {\n"
	path, rng, total, ok := parseReadFileBanner(summary)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if path != "a.go" {
		t.Errorf("path: got %q, want a.go", path)
	}
	if !reflect.DeepEqual(rng, types.LineRange{Start: 10, End: 25}) {
		t.Errorf("range: got %+v, want {10, 25}", rng)
	}
	if total != 100 {
		t.Errorf("total: got %d, want 100", total)
	}
}

func TestParseReadFileBanner_ShowingAll(t *testing.T) {
	// The "showing all" shape is what the read_file tool emits when
	// the file fits under MaxInlineBytes and Limit expands to full
	// file. Without this branch, inline-expanded reads were invisible
	// to range-aware coverage.
	summary := "[config.yaml: showing all 42 lines (1234 bytes); limit=200 expanded to full file (inline-sized; pass offset>0 for explicit paging)]\n"
	path, rng, total, ok := parseReadFileBanner(summary)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if path != "config.yaml" {
		t.Errorf("path: got %q, want config.yaml", path)
	}
	if !reflect.DeepEqual(rng, types.LineRange{Start: 1, End: 42}) {
		t.Errorf("range: got %+v, want {1, 42}", rng)
	}
	if total != 42 {
		t.Errorf("total: got %d, want 42", total)
	}
}

func TestParseReadFileBanner_PathWithSlash(t *testing.T) {
	summary := "[internal/agent/explorer.go: showing lines 1-200 of 6500 total]\n"
	path, rng, total, ok := parseReadFileBanner(summary)
	if !ok {
		t.Fatalf("expected ok")
	}
	if path != "internal/agent/explorer.go" {
		t.Errorf("path: got %q", path)
	}
	if rng.Start != 1 || rng.End != 200 || total != 6500 {
		t.Errorf("parsed: rng=%+v total=%d", rng, total)
	}
}

func TestParseReadFileBanner_NoBanner(t *testing.T) {
	if _, _, _, ok := parseReadFileBanner("just raw content, no banner"); ok {
		t.Error("expected ok=false for raw content")
	}
	if _, _, _, ok := parseReadFileBanner(""); ok {
		t.Error("expected ok=false for empty summary")
	}
}

func TestParseReadFileBanner_MalformedShapes(t *testing.T) {
	cases := []string{
		"[a.go: showing lines 10-5 of 100 total]\n",   // End < Start
		"[a.go: showing lines NaN-25 of 100 total]\n", // non-numeric Start
		"[a.go: showing lines 10-NaN of 100 total]\n", // non-numeric End
		"[a.go: showing stuff 10-25 of 100 total]\n",  // wrong marker
		"[a.go: showing all zero lines (0 bytes)]\n",  // zero lines
		"[: showing lines 10-25 of 100 total]\n",      // empty path (colonIdx == 1 → fails colonIdx > 1)
		"[a.go: showing lines 10 of 100 total]\n",     // no dash
		"[a.go: showing lines 10-25 total]\n",         // no "of"
		"[a.go: showing lines 10-25 of total]\n",      // non-numeric total (total optional so this is ok) — actually this should NOT be malformed because total is optional
		"[a.go: showing all abc lines (1 bytes)]\n",   // non-numeric N in "showing all"
		"[a.go: showing all -5 lines (1 bytes)]\n",    // negative N
	}
	for i, s := range cases {
		path, rng, _, ok := parseReadFileBanner(s)
		if i == 8 {
			// "of total" with non-numeric total — total is optional,
			// so parser returns ok=true with total=0. Verify that.
			if !ok {
				t.Errorf("case %d %q: expected ok=true (total optional)", i, s)
			}
			continue
		}
		if ok {
			t.Errorf("case %d %q: expected ok=false, got path=%q rng=%+v", i, s, path, rng)
		}
	}
}

func TestExtractFileCoverage_PopulatesRanges(t *testing.T) {
	history := []types.ToolResult{
		{ToolName: "read_file", Success: true,
			Summary: "[a.go: showing lines 1-50 of 500 total]\n  1│ pkg foo\n"},
		{ToolName: "read_file", Success: true,
			Summary: "[a.go: showing lines 200-300 of 500 total]\n"},
		{ToolName: "read_file", Success: true,
			Summary: "[b.go: showing all 30 lines (600 bytes); limit=200 expanded to full file]\n"},
		{ToolName: "read_file", Success: false, // failed read is ignored
			Summary: "[c.go: showing lines 1-10 of 10 total]\n"},
	}
	_, readSet, ranges := extractFileCoverage(history, "")
	if !readSet["a.go"] || !readSet["b.go"] {
		t.Fatalf("readSet missing entries: %v", readSet)
	}
	if readSet["c.go"] {
		t.Error("failed read must be excluded from readSet")
	}
	aRanges := ranges["a.go"]
	// Two separate reads → two ranges (pre-merge, in insertion order).
	if len(aRanges) != 2 {
		t.Errorf("a.go ranges: got %d, want 2 (%v)", len(aRanges), aRanges)
	}
	bRanges := ranges["b.go"]
	if len(bRanges) != 1 || bRanges[0].Start != 1 || bRanges[0].End != 30 {
		t.Errorf("b.go ranges: got %+v, want [{1, 30}]", bRanges)
	}
}

func TestExtractFileCoverage_CanonicalizesWindowsReadFilePaths(t *testing.T) {
	history := []types.ToolResult{
		{
			ToolName: "read_file",
			Success:  true,
			Summary:  "[.\\internal\\tool\\repomap\\tool.go: showing lines 1-25 of 200 total]\n",
		},
	}

	_, readSet, ranges := extractFileCoverage(history, "")
	if !readSet["internal/tool/repomap/tool.go"] {
		t.Fatalf("readSet should store canonical slash path, got %v", readSet)
	}
	if got := ranges["internal/tool/repomap/tool.go"]; len(got) != 1 || got[0].Start != 1 || got[0].End != 25 {
		t.Fatalf("ranges should follow the canonical path, got %v", got)
	}
}
