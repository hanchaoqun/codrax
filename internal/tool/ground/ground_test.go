package ground

import (
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// buildGutterReadResult produces a read_file ToolResult with the
// same gutter format the grounder expects (matches
// tool.renderWithLineGutter). Duplicated here to avoid depending on
// internal/agent's test helper (ground is the shared lib; agent
// consumes it).
func buildGutterReadResult(path string, startLine int, lines []string, totalLines int) types.ToolResult {
	body := "[" + path + ": showing lines " + itoa(startLine) + "-" + itoa(startLine+len(lines)-1) + " of " + itoa(totalLines) + "]\n"
	for i, l := range lines {
		body += "  " + itoa(startLine+i) + "│ " + l + "\n"
	}
	return types.ToolResult{ToolName: "read_file", Success: true, Summary: body}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestGroundItem_Tier1LineText locks the primary positive path:
// the read_file gutter contains a line where the AnchorSymbol shows
// up as a whole-word token, so Tier 1 accepts.
func TestGroundItem_Tier1LineText(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func Foo() string {",
			"\treturn \"x\"",
			"}",
		}, 3),
	}
	gc := &Context{LineIndex: buildLineIndex(history)}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	r := GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("status: %q, want grounded", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierLineText {
		t.Errorf("tier: %q, want line_text", it.GroundingTier)
	}
	if r.OriginalLine != 10 || r.AdjustedLine != 10 {
		t.Errorf("lines drifted on grounded: %d→%d", r.OriginalLine, r.AdjustedLine)
	}
}

// TestGroundItem_Tier2SymbolTable_Definition covers the symbol-table
// grounding path: read_file history is empty but the graph has a
// matching Symbol at the cited line. Tier 2 accepts.
func TestGroundItem_Tier2SymbolTable_Definition(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Symbols: []repomap.Symbol{{Name: "Foo", Kind: "function", Line: 10}},
			},
		},
	}
	gc := &Context{Graph: graph}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 10,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("status: %q, want grounded", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierSymbolTable {
		t.Errorf("tier: %q, want symbol_table", it.GroundingTier)
	}
}

// TestGroundItem_RecoveryR1FQNameSameFile reproduces the off-by-N
// case: LLM cites a line number that does NOT contain the anchor
// symbol, but the symbol exists in the same file at a different
// line. R1 rewrites LineStart and marks recovered.
func TestGroundItem_RecoveryR1FQNameSameFile(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Symbols: []repomap.Symbol{{Name: "Foo", Kind: "function", Line: 42}},
			},
		},
	}
	gc := &Context{Graph: graph}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 30,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	r := GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingRecovered {
		t.Fatalf("status: %q, want recovered", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierFQNameSameFile {
		t.Errorf("tier: %q, want fqname_same_file", it.GroundingTier)
	}
	if it.LineStart != 42 {
		t.Errorf("LineStart not rewritten: got %d, want 42", it.LineStart)
	}
	if r.OriginalLine != 30 || r.AdjustedLine != 42 {
		t.Errorf("report lines wrong: original=%d adjusted=%d", r.OriginalLine, r.AdjustedLine)
	}
}

// TestGroundItem_Ungrounded exercises the fail-through path. No
// read_file history, no matching graph entry — every tier fails
// and the item is marked ungrounded with an explanatory note.
// LineStart is preserved so the finalizer still has a human-readable
// reference even though the item cannot be cited.
func TestGroundItem_Ungrounded(t *testing.T) {
	gc := &Context{}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "nowhere.go", LineStart: 99,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Ghost",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("status: %q, want ungrounded", it.GroundingStatus)
	}
	if it.LineStart != 99 {
		t.Errorf("LineStart cleared on ungrounded; must be preserved: got %d", it.LineStart)
	}
	if it.GroundingNote == "" {
		t.Errorf("ungrounded note empty — must explain the miss for LLM repair")
	}
}

// TestGroundItem_Tier2Call confirms the call-site dispatch hits the
// repomap Relations table rather than Symbols. AnchorKind=call.
func TestGroundItem_Tier2Call(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {
				RelPath: "a.go",
				Relations: []repomap.Relation{
					{Kind: "call", File: "a.go", Line: 20,
						ToEP: repomap.RelationEndpoint{Name: "Execute"}},
				},
			},
		},
	}
	gc := &Context{Graph: graph}
	it := &types.EvidenceItem{
		Kind: types.EvidenceRelationship, Source: "a.go", LineStart: 20,
		AnchorKind: types.AnchorCall, AnchorSymbol: "Execute",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("status: %q, want grounded", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierSymbolTable {
		t.Errorf("tier: %q, want symbol_table (call branch)", it.GroundingTier)
	}
}

// TestGroundItem_SnippetFuzzyRecovery locks the R2 path: LLM cites
// line 12 but the actual snippet appears at line 15, snippet is
// provided so the fuzzy search rewrites the line.
func TestGroundItem_SnippetFuzzyRecovery(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"// header",
			"// more header",
			"func doWork() {",
			"\tx := veryDistinctIdentifier(Alpha, Beta, Gamma)",
			"\treturn nil",
			"}",
		}, 6),
	}
	gc := &Context{LineIndex: buildLineIndex(history)}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 12,
		AnchorKind: types.AnchorAssignment, AnchorSymbol: "veryDistinctIdentifier",
		Snippet: "x := veryDistinctIdentifier(Alpha, Beta, Gamma)",
	}
	GroundItem(it, gc)
	// Either T1 accepted at line 13 (neighbour hit) or R2 adjusts to
	// 13 — both are valid grounded-or-recovered outcomes. The test
	// pins that neither ungrounded nor the original-line-missed
	// state leaks.
	if it.GroundingStatus == types.GroundingUngrounded {
		t.Fatalf("snippet match should recover, got ungrounded: note=%q", it.GroundingNote)
	}
}

// TestSplitConversation via GroundItem is implicit — just a smoke
// test that TierLineText's lineCorroborates fallback works when
// AnchorSymbol is empty (legacy concrete_value items from
// deterministic extractor may not set it).
func TestGroundItem_LegacyNoAnchorStillGrounds(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func Orchestrator_Run() {",
		}, 1),
	}
	gc := &Context{LineIndex: buildLineIndex(history)}
	it := &types.EvidenceItem{
		Kind:      types.EvidenceConcrete,
		Source:    "a.go",
		LineStart: 10,
		Subject:   "Orchestrator_Run",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("legacy anchor-less item did not ground: status=%q note=%q",
			it.GroundingStatus, it.GroundingNote)
	}
}
