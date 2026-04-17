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

// TestGroundCitation_GutterHitQuoteMatches is the strongest happy
// path: the file was read, the line is in the gutter, the Quote
// tokens overlap with the line text → Valid + QuoteMatched.
func TestGroundCitation_GutterHitQuoteMatches(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func MyFunction() int {",
			"\treturn 42",
			"}",
		}, 3),
	}
	gc := &Context{LineIndex: buildLineIndex(history)}
	c := types.Citation{File: "a.go", Line: 10, Quote: "func MyFunction() int"}
	r := GroundCitation(c, gc)
	if !r.Valid {
		t.Fatalf("valid: %v (reason=%q)", r.Valid, r.Reason)
	}
	if !r.QuoteMatched {
		t.Errorf("quote should be corroborated: %+v", r)
	}
}

// TestGroundCitation_GutterHitQuoteFabricated catches the primary
// bug motivating this change: the file:line is real but the Quote is
// prose the LLM wrote, not text from the cited line. Valid stays
// true (the anchor is usable) but QuoteMatched is false so the
// caller clears it.
func TestGroundCitation_GutterHitQuoteFabricated(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func MyFunction() int {",
		}, 1),
	}
	gc := &Context{LineIndex: buildLineIndex(history)}
	c := types.Citation{
		File: "a.go", Line: 10,
		Quote: "stated that the module is used for creating navigation indexes",
	}
	r := GroundCitation(c, gc)
	if !r.Valid {
		t.Fatalf("file:line is real, must be Valid: %+v", r)
	}
	if r.QuoteMatched {
		t.Errorf("prose quote should NOT corroborate; got QuoteMatched=true")
	}
}

// TestGroundCitation_FileAbsentEverywhere is the drop path: file not
// in gutter and not in graph. Valid=false with a reason the LLM can
// read and fix on the next turn.
func TestGroundCitation_FileAbsentEverywhere(t *testing.T) {
	gc := &Context{LineIndex: map[string]map[int]string{"b.go": {1: "pkg foo"}}}
	c := types.Citation{File: "nowhere.go", Line: 42}
	r := GroundCitation(c, gc)
	if r.Valid {
		t.Fatalf("absent file must not be Valid: %+v", r)
	}
	if r.Reason == "" {
		t.Errorf("Valid=false must carry a Reason so the LLM knows what to fix")
	}
}

// TestGroundCitation_GraphFallbackAcceptsAnyLineInIndexedFile covers
// the Tier 2 fallback: the file is indexed by repomap (so we know
// it's a real source file in this repo), the cited line is accepted
// even when it falls outside every symbol range. This is the
// package-doc / import / top-level-const case that matters for real
// code — requiring symbol-range coverage rejected every `facade.go:1`
// cite in the first REPL run.
func TestGroundCitation_GraphFallbackAcceptsAnyLineInIndexedFile(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {RelPath: "a.go", Symbols: []repomap.Symbol{
				{Name: "Foo", Kind: "function", Line: 10, EndLine: 30},
			}},
		},
	}
	gc := &Context{Graph: graph}
	// Line 1 (package doc comment) — outside every symbol range but
	// file is indexed → Valid.
	r1 := GroundCitation(types.Citation{File: "a.go", Line: 1, Quote: "// Package foo"}, gc)
	if !r1.Valid {
		t.Errorf("line 1 in indexed file must be Valid (package doc): %+v", r1)
	}
	if r1.QuoteMatched {
		t.Errorf("Tier 2 cannot corroborate quotes without the gutter; QuoteMatched must be false")
	}
	// Line 20 inside Foo — still valid.
	r2 := GroundCitation(types.Citation{File: "a.go", Line: 20}, gc)
	if !r2.Valid {
		t.Errorf("line 20 (inside symbol) must be Valid: %+v", r2)
	}
}

// TestRecoveryR3_PackageSymbolGatedByAnchorKind pins the bug fix
// where an AnchorKind=condition item citing an if-statement call site
// (agent.go:900 "if _, err := b.deps.SubAgents.Get(...) {") was being
// cross-file rewritten to registry.go:30 (the Registry.Get method
// definition) because both carry a "Get" symbol. Only definition and
// import anchors have cross-file package-wide semantics; every other
// kind is file-local and must NOT trigger R3's Source rewrite.
func TestRecoveryR3_PackageSymbolGatedByAnchorKind(t *testing.T) {
	// A repo where `Get` is defined in registry.go (line 30) but the
	// LLM cites a call site in agent.go (line 900) inside a condition.
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/agent.go": {
				RelPath: "internal/agent/agent.go",
				Package: "agent",
			},
			"internal/agent/registry.go": {
				RelPath: "internal/agent/registry.go",
				Package: "agent",
				Symbols: []repomap.Symbol{
					{Name: "Get", Kind: "method", Line: 30},
				},
			},
		},
		SymbolDefs: map[string][]*repomap.Symbol{
			"Get": {{Name: "Get", File: "internal/agent/registry.go", Line: 30}},
		},
	}

	// Condition anchor — file-local semantic. R3 must NOT fire.
	condItem := &types.EvidenceItem{
		Kind:         types.EvidenceConditional,
		Source:       "internal/agent/agent.go",
		LineStart:    900,
		AnchorKind:   types.AnchorCondition,
		AnchorSymbol: "SubAgents.Get",
	}
	if newSource, newLine, ok := recoverPackageSymbol(condItem, &Context{Graph: graph}); ok {
		t.Errorf("condition anchor MUST NOT trigger R3 (got rewrite to %s:%d)", newSource, newLine)
	}

	// Call anchor — also file-local. R3 must NOT fire (R4 handles it).
	callItem := &types.EvidenceItem{
		Kind:         types.EvidenceRelationship,
		Source:       "internal/agent/agent.go",
		LineStart:    900,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "SubAgents.Get",
	}
	if newSource, newLine, ok := recoverPackageSymbol(callItem, &Context{Graph: graph}); ok {
		t.Errorf("call anchor MUST NOT trigger R3 (got rewrite to %s:%d)", newSource, newLine)
	}

	// Definition anchor on a file lacking the symbol — R3 MUST fire
	// (this is the legitimate "pasted neighbour file" case).
	defItem := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/agent/agent.go",
		LineStart:    1,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "Get",
	}
	newSource, newLine, ok := recoverPackageSymbol(defItem, &Context{Graph: graph})
	if !ok {
		t.Fatalf("definition anchor with no same-file match MUST trigger R3")
	}
	if newSource != "internal/agent/registry.go" || newLine != 30 {
		t.Errorf("R3 rewrite wrong: got %s:%d, want registry.go:30", newSource, newLine)
	}

	// Empty AnchorKind (legacy item) — R3 must NOT fire because we
	// cannot tell whether the original anchor was a definition.
	legacyItem := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/agent/agent.go",
		LineStart:    900,
		AnchorSymbol: "Get",
	}
	if newSource, newLine, ok := recoverPackageSymbol(legacyItem, &Context{Graph: graph}); ok {
		t.Errorf("empty AnchorKind MUST NOT trigger R3 (got rewrite to %s:%d)", newSource, newLine)
	}
}

// TestGroundCitation_EmptyContextSkipsGrounding locks the unit-test
// backdoor: when the context carries no ground-truth sources, the
// grounder returns Valid=true without looking. Required so tests
// that don't bother constructing a read_file history still exercise
// the downstream citation-handling paths.
func TestGroundCitation_EmptyContextSkipsGrounding(t *testing.T) {
	c := types.Citation{File: "a.go", Line: 1, Quote: "whatever"}
	if r := GroundCitation(c, nil); !r.Valid {
		t.Errorf("nil context must skip grounding, got %+v", r)
	}
	if r := GroundCitation(c, &Context{}); !r.Valid {
		t.Errorf("empty context must skip grounding, got %+v", r)
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
