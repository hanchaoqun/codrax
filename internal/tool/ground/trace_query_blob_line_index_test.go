package ground

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// Q5-A P1-2 (security root fix, main citation-pollution port): a
// read_file that returned trace_query BLOB bytes carries the typed
// RuntimeArtifactRead marker the tool stamped from the same decision
// that suppressed current-source coverage. buildLineIndex must skip it
// on THAT precise typed signal (not banner-string re-classification), so
// the blob's line_text never enters the Tier-1 citation index and a blob
// line can never become a grounded current-source citation.

// buildRuntimeArtifactReadResult mirrors buildGutterReadResult but stamps
// the typed runtime-artifact marker, modelling a trace_query blob read
// opened through the Q5-A escape lane.
func buildRuntimeArtifactReadResult(path string, startLine int, lines []string, totalLines int) types.ToolResult {
	r := buildGutterReadResult(path, startLine, lines, totalLines)
	r.RuntimeArtifactRead = &types.ToolRuntimeArtifactRead{
		RequestedPath:  path,
		Kind:           "blob",
		TraceQueryBlob: true,
	}
	return r
}

func TestBuildLineIndex_SkipsRuntimeArtifactBlobReads(t *testing.T) {
	blobBasename := "trace_query-ab12cd34.txt"
	history := []types.ToolResult{
		buildRuntimeArtifactReadResult(blobBasename, 10, []string{
			"func Foo() string {", // decoy identifier the model could try to cite
			"\treturn \"x\"",
			"}",
		}, 3),
	}
	idx := buildLineIndex(history, "")
	if len(idx) != 0 {
		t.Fatalf("runtime-artifact blob read must NOT populate the Tier-1 line index, got %+v", idx)
	}

	// GroundItem must therefore fail to ground a citation aimed at that blob.
	gc := &Context{LineIndex: idx}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: blobBasename, LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded {
		t.Fatalf("blob line_text must not become a grounded current-source citation, got status=%q tier=%q", it.GroundingStatus, it.GroundingTier)
	}
}

// Control: an ordinary current-source read (no RuntimeArtifactRead
// marker) still indexes and grounds byte-identically.
func TestBuildLineIndex_OrdinaryReadStillGrounds(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func Foo() string {",
			"\treturn \"x\"",
			"}",
		}, 3),
	}
	idx := buildLineIndex(history, "")
	if got := idx["a.go"][10]; got != "func Foo() string {" {
		t.Fatalf("ordinary current-source read must still index, got %+v", idx)
	}
	gc := &Context{LineIndex: idx}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("ordinary current-source read must still ground, got %q", it.GroundingStatus)
	}
}
