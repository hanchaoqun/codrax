package ground

import (
	"strings"
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
	gc := &Context{LineIndex: buildLineIndex(history, "")}
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
	if it.Snippet != "func Foo() string {" {
		t.Errorf("snippet not attached from grounded line: got %q", it.Snippet)
	}
}

func TestGroundItem_ConfigSurfaceCommentAllowsLooseCorroboration(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("codrax.yaml.example", 20, []string{
			"# Precedence (lowest wins last):",
			"#",
			"#   code defaults  <  <exeDir>/codrax.yaml  <  command-line flags",
		}, 24),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:                 types.EvidenceDirect,
		Source:               "codrax.yaml.example",
		LineStart:            22,
		AnchorKind:           types.AnchorDefinition,
		AnchorSymbol:         "exeDir",
		Subject:              "codrax.yaml",
		Object:               "command-line flags",
		Summary:              "code defaults < codrax.yaml < command-line flags",
		DiagramRole:          types.EvidenceDiagramRoleConfig,
		RequestedDiagramRole: types.EvidenceDiagramRoleConfig,
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("status: %q, want grounded", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierLineText {
		t.Fatalf("tier: %q, want line_text", it.GroundingTier)
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

func TestGroundItem_GraphDefinitionLineCanonicalizesDocCommentToDefinition(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/orchestrator/orchestrator.go", 3620, []string{
			"// runReadSchedulerLoop owns the read-mode task graph.",
			"// It schedules analyzer, explorer, extractor, and finalizer.",
			"// The comment is useful context but not the definition.",
			"//",
			"// More doc text.",
			"func (o *Orchestrator) runReadSchedulerLoop(state *graphState) (*agent.StageOutput, string, bool) {",
		}, 3700),
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/orchestrator/orchestrator.go": {
				RelPath: "internal/orchestrator/orchestrator.go",
				Symbols: []repomap.Symbol{{
					Name: "runReadSchedulerLoop",
					Kind: "method",
					Line: 3620,
				}},
			},
		},
	}
	gc := &Context{LineIndex: buildLineIndex(history, ""), Graph: graph}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/orchestrator/orchestrator.go",
		LineStart:    3620,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "runReadSchedulerLoop",
	}
	r := GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Fatalf("status: %q, want grounded", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierSymbolTable {
		t.Fatalf("tier: %q, want symbol_table", it.GroundingTier)
	}
	if it.LineStart != 3625 {
		t.Fatalf("LineStart = %d, want canonical definition line 3625", it.LineStart)
	}
	if r.OriginalLine != 3620 || r.AdjustedLine != 3625 {
		t.Fatalf("report lines = %d→%d, want 3620→3625", r.OriginalLine, r.AdjustedLine)
	}
	if !strings.Contains(it.Snippet, "func (o *Orchestrator) runReadSchedulerLoop") {
		t.Fatalf("snippet = %q, want function definition line", it.Snippet)
	}
}

func TestGroundItem_RecoveryR1CanonicalizesGraphDocCommentLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/orchestrator/orchestrator.go", 3620, []string{
			"// runReadSchedulerLoop owns the read-mode task graph.",
			"// It schedules analyzer, explorer, extractor, and finalizer.",
			"// The comment is useful context but not the definition.",
			"//",
			"// More doc text.",
			"func (o *Orchestrator) runReadSchedulerLoop(state *graphState) (*agent.StageOutput, string, bool) {",
		}, 3700),
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/orchestrator/orchestrator.go": {
				RelPath: "internal/orchestrator/orchestrator.go",
				Symbols: []repomap.Symbol{{
					Name: "runReadSchedulerLoop",
					Kind: "method",
					Line: 3620,
				}},
			},
		},
	}
	gc := &Context{LineIndex: buildLineIndex(history, ""), Graph: graph}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/orchestrator/orchestrator.go",
		LineStart:    3608,
		AnchorKind:   types.AnchorDefinition,
		AnchorSymbol: "runReadSchedulerLoop",
	}
	r := GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingRecovered {
		t.Fatalf("status: %q, want recovered", it.GroundingStatus)
	}
	if it.GroundingTier != types.TierFQNameSameFile {
		t.Fatalf("tier: %q, want fqname_same_file", it.GroundingTier)
	}
	if it.LineStart != 3625 {
		t.Fatalf("LineStart = %d, want canonical definition line 3625", it.LineStart)
	}
	if r.OriginalLine != 3608 || r.AdjustedLine != 3625 {
		t.Fatalf("report lines = %d→%d, want 3608→3625", r.OriginalLine, r.AdjustedLine)
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

func TestGroundItem_Tier1CallAcceptsRealCallsiteWithoutGraph(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 20, []string{
			"func ParseOutput(ctx *AgentContext) error {",
			"\tir, buildErr := buildAnalysisIR(ctx)",
			"\treturn buildErr",
		}, 30),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:       types.EvidenceRelationship,
		Source:     "a.go",
		LineStart:  21,
		AnchorKind: types.AnchorCall,
		// LLM-facing contract says call anchors should use the callee
		// name as anchor_symbol; Tier 1 should still accept the real
		// callsite without needing a graph.
		AnchorSymbol: "buildAnalysisIR",
		Subject:      "ParseOutput",
		Object:       "buildAnalysisIR",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
		t.Fatalf("real callsite should Tier-1 ground without graph: status=%q tier=%q note=%q", it.GroundingStatus, it.GroundingTier, it.GroundingNote)
	}
}

func TestGroundItem_Tier1CallRejectsFunctionDefinitionLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func ParseOutput(ctx *AgentContext) error {",
			"\treturn nil",
			"}",
		}, 20),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceRelationship,
		Source:       "a.go",
		LineStart:    10,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "ParseOutput", // caller-shaped, not callee-shaped
		Subject:      "ParseOutput",
		Object:       "buildAnalysisIR",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded && it.GroundingTier == types.TierLineText {
		t.Fatalf("function definition line must not Tier-1 ground as a callsite")
	}
}

func TestGroundItem_CallAnchorUsesCalleeForRecovery(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/analyzer.go", 10, []string{
			"func (e *analyzerEvaluator) ParseOutput(ctx *types.AgentContext, messages []llm.Message, toolResults []types.ToolResult, _ []types.MCPResponse) (*StageOutput, error) {",
			"\treturn nil, nil",
			"}",
			"",
			"",
			"",
			"",
			"",
			"",
			"func another() {}",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"",
			"\tir, buildErr := buildAnalysisIR(ctx)",
		}, 40),
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/analyzer.go": {
				RelPath: "internal/agent/analyzer.go",
				Relations: []repomap.Relation{
					{Kind: "call", File: "internal/agent/analyzer.go", Line: 29, ToEP: repomap.RelationEndpoint{Name: "buildAnalysisIR"}},
				},
			},
		},
	}
	gc := &Context{LineIndex: buildLineIndex(history, ""), Graph: graph}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceRelationship,
		Source:       "internal/agent/analyzer.go",
		LineStart:    10,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "ParseOutput", // caller-shaped anchor, common LLM mistake
		Subject:      "ParseOutput",
		Object:       "buildAnalysisIR",
		Snippet:      "ir, buildErr := buildAnalysisIR(ctx)",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingUngrounded {
		t.Fatalf("call anchor should recover via callee-backed grounding, got ungrounded: note=%q", it.GroundingNote)
	}
	if it.LineStart != 29 {
		t.Fatalf("call anchor recovered line = %d, want 29", it.LineStart)
	}
	if got, want := it.Snippet, "ir, buildErr := buildAnalysisIR(ctx)"; got != want {
		t.Fatalf("recovered callsite snippet=%q, want %q", got, want)
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
	gc := &Context{LineIndex: buildLineIndex(history, "")}
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
	if it.Snippet != "x := veryDistinctIdentifier(Alpha, Beta, Gamma)" {
		t.Fatalf("recovered item should keep the grounded line snippet, got %q", it.Snippet)
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
	gc := &Context{LineIndex: buildLineIndex(history, "")}
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
	gc := &Context{LineIndex: buildLineIndex(history, "")}
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

// TestGroundCitation_Tier2RejectsLineOutsideStructuralRegion locks the
// 2026-04-17 tightening: a citation to an indexed file whose line
// falls in a dead zone (between two far-apart symbols, beyond the doc
// radius of either) is rejected so the LLM cannot fabricate line
// numbers in files it never read. Legitimate doc-block cites
// (within tier2DocRadius of a symbol) and prologue cites are still
// accepted.
func TestGroundCitation_Tier2RejectsLineOutsideStructuralRegion(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {RelPath: "a.go", Symbols: []repomap.Symbol{
				{Name: "Foo", Kind: "function", Line: 10, EndLine: 30},
				{Name: "Bar", Kind: "function", Line: 100, EndLine: 120},
			}},
		},
	}
	gc := &Context{Graph: graph}

	// Line 50 — mid-file between Foo (ends 30) and Bar's doc radius
	// (starts 100-10=90). Outside both symbols' windows. Rejected.
	r := GroundCitation(types.Citation{File: "a.go", Line: 50, Quote: "fabricated"}, gc)
	if r.Valid {
		t.Errorf("line 50 is a dead zone between symbols; must be rejected, got Valid=true tier=%q", r.Tier)
	}
	if r.Reason == "" {
		t.Errorf("rejection must carry a human-readable reason")
	}

	// Line 27 — inside Foo [10, 30]. Accepted with Tier=symbol_table.
	r2 := GroundCitation(types.Citation{File: "a.go", Line: 27}, gc)
	if !r2.Valid || r2.Tier != types.TierSymbolTable {
		t.Errorf("line 27 inside Foo body must be Valid+symbol_table: %+v", r2)
	}

	// Line 92 — inside Bar's doc radius [90, 120]. Accepted.
	r3 := GroundCitation(types.Citation{File: "a.go", Line: 92}, gc)
	if !r3.Valid || r3.Tier != types.TierSymbolTable {
		t.Errorf("line 92 in Bar's doc block must be Valid+symbol_table: %+v", r3)
	}

	// Line 2 — prologue before Foo (firstSymbolLine=10). Line
	// 2 ∈ [docStart=1, EndLine=30] via Foo's window.
	r4 := GroundCitation(types.Citation{File: "a.go", Line: 2}, gc)
	if !r4.Valid {
		t.Errorf("line 2 (prologue / Foo doc window) must be Valid: %+v", r4)
	}
}

// TestGroundCitation_TierFieldPopulated verifies that the Tier field
// of CitationReport is always set when Valid, so emit_answer_document
// can enforce the "at least one Tier 1 proven peer in pool" rule.
func TestGroundCitation_TierFieldPopulated(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{"func Foo() string {", "\treturn \"x\"", "}"}, 3),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	r := GroundCitation(types.Citation{File: "a.go", Line: 10}, gc)
	if r.Tier != types.TierLineText {
		t.Errorf("gutter-hit citation must have Tier=line_text, got %q", r.Tier)
	}

	graph := &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
		"b.go": {RelPath: "b.go", Symbols: []repomap.Symbol{{Name: "Bar", Line: 5, EndLine: 20}}},
	}}
	gc2 := &Context{Graph: graph}
	r2 := GroundCitation(types.Citation{File: "b.go", Line: 10}, gc2)
	if r2.Tier != types.TierSymbolTable {
		t.Errorf("graph-only citation must have Tier=symbol_table, got %q", r2.Tier)
	}

	// No-context path: empty Tier so emit_answer_document can tell
	// this is a test environment and skip the pool-level rule.
	r3 := GroundCitation(types.Citation{File: "x.go", Line: 1}, &Context{})
	if r3.Tier != "" {
		t.Errorf("no-context citation must have empty Tier, got %q", r3.Tier)
	}
	if !r3.Valid {
		t.Errorf("no-context citation must stay Valid for test compatibility")
	}
}

// TestBuildContext_PicksUpDispatchToolResults pins the fix for the
// "grounder doesn't see read_file history" bug. BaseAgent.executeTool
// now mirrors every tool result into Mutable.DispatchToolResults; the
// grounder's BuildContext reads that buffer ALONGSIDE
// ctx.ToolResults so emit_evidence called later in the SAME ReAct
// loop can corroborate lines the LLM read earlier in the same loop.
func TestBuildContext_PicksUpDispatchToolResults(t *testing.T) {
	// Simulate a read_file done earlier in the same dispatch (not yet
	// flushed to bus.ToolResults by applyStageOutput).
	readResult := buildGutterReadResult("a.go", 100, []string{
		"func Foo() {",
		"    bar()",
		"}",
	}, 3)

	mut := types.NewMutableState("irrelevant")
	mut.AppendDispatchToolResult(readResult)
	bus := &types.BusContext{Mutable: mut}

	gc := BuildContext(bus)
	if _, ok := gc.LineIndex["a.go"]; !ok {
		t.Fatalf("a.go missing from line index built from dispatch buffer: %+v", gc.LineIndex)
	}
	// Tier 1 grounding against the dispatch-buffered read.
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.go", LineStart: 100,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Errorf("in-dispatch read_file must ground: status=%q note=%q",
			it.GroundingStatus, it.GroundingNote)
	}
}

// TestBuildContext_ForcedReadPrefixedSummaryStillGrounds pins the
// load-bearing fix for the finalizer-grounder × forced-read death
// corner. runForcedReads (orchestrator/cgec_enforcers.go) tags
// synthesised reads with `[forced_read] ` or `[forced_read surgical] `
// for trace inspection; without StripForcedReadPrefix, the LineIndex
// builder split on the FIRST `: ` of the prefix, indexing the
// gutter lines under a malformed key. The grounder then reported
// "ungrounded" for any citation pointing into surgical-read content
// even though the LLM had received it.
//
// Test: append a forced-read-tagged ToolResult into the dispatch
// buffer, build LineIndex, and ground a citation against the
// surgical-read content. Must succeed.
func TestBuildContext_ForcedReadPrefixedSummaryStillGrounds(t *testing.T) {
	clean := buildGutterReadResult("internal/repl/repl.go", 100, []string{
		"func Foo() {",
		"    bar()",
		"}",
	}, 4368)
	tagged := clean
	tagged.Summary = "[forced_read surgical] " + clean.Summary

	mut := types.NewMutableState("irrelevant")
	mut.AppendDispatchToolResult(tagged)
	bus := &types.BusContext{Mutable: mut}

	gc := BuildContext(bus)
	if _, ok := gc.LineIndex["internal/repl/repl.go"]; !ok {
		t.Fatalf("forced-read-tagged Summary must still register in LineIndex; got %+v", gc.LineIndex)
	}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "internal/repl/repl.go", LineStart: 100,
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded {
		t.Errorf("forced-read content must ground at the cited line; status=%q note=%q",
			it.GroundingStatus, it.GroundingNote)
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

// TestGroundItem_Tier1RejectsCommentLine reproduces the 2026-04-18
// "explorer → subagent" bug. The LLM emitted an evidence item anchored
// at a line whose entire body is a `//` comment that happens to mention
// the anchor_symbol. The Tier 1 matcher used to accept this because
// the token substring appeared in the line; the comment-line gate now
// forces fall-through so Tier 2 / recovery can find the real
// definition (or leave the item ungrounded).
func TestGroundItem_Tier1RejectsCommentLine(t *testing.T) {
	// Line 321 is a pure comment mentioning the anchor; line 319-323
	// are a comment block. Real definition lives elsewhere.
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/explorer.go", 319, []string{
			"\t\t\t// English nouns (\"count\",\"agents\",\"that\",\"call\") to the",
			"\t\t\t// entity set, inflating registration req count from 2 to 8",
			"\t\t\t// and flipping answer_chain[0] from the canonical",
			"\t\t\t// `RegisterDefaultSubAgents → SubExplorer` chain to the",
			"\t\t\t// spurious `RegisterDefaults → GrepTool.Description` chain",
		}, 400),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "internal/agent/explorer.go",
		LineStart:  322, // claim on the same comment block
		AnchorKind: types.AnchorCall, AnchorSymbol: "RegisterDefaultSubAgents",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded &&
		it.GroundingTier == types.TierLineText {
		t.Fatalf("comment-only line was grounded at Tier 1 — gate did not fire (status=%q tier=%q)",
			it.GroundingStatus, it.GroundingTier)
	}
}

func TestGroundItem_AttachSnippetPreservesStatementLocalLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/analyzer.go", 977, []string{
			"func buildAnalysisIR(ctx *types.AgentContext) (*types.AnalysisIR, error) {",
			"if ctx == nil || ctx.Mutable == nil { return nil, errors.New(\"analyzer: missing AgentContext.Mutable\") }",
			"raw := ctx.Mutable.RequestModel()",
		}, 1200),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}

	cond := &types.EvidenceItem{
		Kind:         types.EvidenceConditional,
		Source:       "internal/agent/analyzer.go",
		LineStart:    978,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorCondition,
		AnchorSymbol: "buildAnalysisIR",
		OwnerSymbol:  "buildAnalysisIR",
		Condition:    "ctx == nil || ctx.Mutable == nil",
		Snippet:      "if ctx == nil || ctx.Mutable == nil { return nil, errors.New(\"analyzer: missing AgentContext.Mutable\") }",
	}
	rep := GroundItem(cond, gc)
	if rep.Status != types.GroundingGrounded {
		t.Fatalf("condition grounding status=%s, want grounded", rep.Status)
	}
	if got, want := cond.Snippet, "if ctx == nil || ctx.Mutable == nil { return nil, errors.New(\"analyzer: missing AgentContext.Mutable\") }"; got != want {
		t.Fatalf("condition snippet=%q, want %q", got, want)
	}

	assign := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/agent/analyzer.go",
		LineStart:    979,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorAssignment,
		AnchorSymbol: "buildAnalysisIR",
		OwnerSymbol:  "buildAnalysisIR",
		Subject:      "buildAnalysisIR",
		Snippet:      "raw := ctx.Mutable.RequestModel()",
	}
	rep = GroundItem(assign, gc)
	if rep.Status != types.GroundingGrounded {
		t.Fatalf("assignment grounding status=%s, want grounded", rep.Status)
	}
	if got, want := assign.Snippet, "raw := ctx.Mutable.RequestModel()"; got != want {
		t.Fatalf("assignment snippet=%q, want %q", got, want)
	}
}

// TestGroundItem_Tier1AcceptsRealCodeLine is the positive control:
// when the anchor actually lands on a code line (not a comment), Tier 1
// should still accept it. Guards against the comment-exclusion gate
// over-rejecting legitimate matches.
func TestGroundItem_Tier1AcceptsRealCodeLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/subagent.go", 62, []string{
			"// RegisterDefaultSubAgents registers the default set of SubAgent implementations.",
			"func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {",
			"\tr.Register(NewSubExplorer(deps))",
			"}",
		}, 100),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind: types.EvidenceRegistration, Source: "internal/agent/subagent.go",
		LineStart:  63, // the actual func declaration line
		AnchorKind: types.AnchorDefinition, AnchorSymbol: "RegisterDefaultSubAgents",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
		t.Fatalf("real code line was not Tier-1 grounded: status=%q tier=%q note=%q",
			it.GroundingStatus, it.GroundingTier, it.GroundingNote)
	}
}

// TestGroundItem_Tier1RejectsBlockComment verifies the `/* ... */`
// block-comment detector walks back through LineIndex and refuses to
// Tier-1 ground a line sitting inside an unclosed block opener.
func TestGroundItem_Tier1RejectsBlockComment(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.c", 10, []string{
			"/*",
			" * Block comment mentioning Foo here",
			" * still in the block",
			" */",
			"int Foo(void) { return 0; }",
		}, 20),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "a.c",
		LineStart: 11, AnchorKind: types.AnchorDefinition, AnchorSymbol: "Foo",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded && it.GroundingTier == types.TierLineText {
		t.Fatalf("anchor inside /* */ block was Tier-1 grounded — block gate did not fire")
	}
}

// TestGroundItem_Tier1RejectsPythonDocstring verifies Python triple-
// quoted docstring detection: a line mentioning the anchor inside a
// `""" ... """` block must not ground at Tier 1.
func TestGroundItem_Tier1RejectsPythonDocstring(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("mod.py", 5, []string{
			`def foo():`,
			`    """`,
			`    Calls bar() under certain conditions.`,
			`    """`,
			`    return None`,
		}, 10),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind: types.EvidenceDirect, Source: "mod.py",
		LineStart: 7, AnchorKind: types.AnchorCall, AnchorSymbol: "bar",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded && it.GroundingTier == types.TierLineText {
		t.Fatalf("anchor inside Python docstring was Tier-1 grounded — docstring gate did not fire")
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
	gc := &Context{LineIndex: buildLineIndex(history, "")}
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

// TestGroundCitation_NearestSymbolsHintInRejection pins C+ (session-8):
// when Tier 2 rejects a citation, the Reason text lists the 3 closest
// symbols with their line ranges so the LLM can re-cite without
// another read_file round-trip.
func TestGroundCitation_NearestSymbolsHintInRejection(t *testing.T) {
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {RelPath: "a.go", Symbols: []repomap.Symbol{
				{Name: "Alpha", Kind: "function", Line: 10, EndLine: 20},
				{Name: "Beta", Kind: "function", Line: 200, EndLine: 240},
				{Name: "Gamma", Kind: "function", Line: 300, EndLine: 340},
				{Name: "Delta", Kind: "function", Line: 500, EndLine: 520},
			}},
		},
	}
	gc := &Context{Graph: graph}
	// Line 100 is in a dead zone between Alpha (20) and Beta's doc
	// radius (190). Rejection must list Beta / Alpha / Gamma
	// (ordered by distance: 100 nearest Beta=100, Alpha=90, Gamma=200).
	r := GroundCitation(types.Citation{File: "a.go", Line: 100}, gc)
	if r.Valid {
		t.Fatalf("line 100 should be rejected: %+v", r)
	}
	if !strings.Contains(r.Reason, "Candidate symbols nearby:") {
		t.Errorf("rejection must carry candidate-symbols hint: %q", r.Reason)
	}
	// Nearest: Alpha (dist 90), Beta (dist 100), Gamma (dist 200).
	// Delta (dist 400) should NOT appear — only top 3.
	if !strings.Contains(r.Reason, "Alpha") || !strings.Contains(r.Reason, "lines 10-20") {
		t.Errorf("nearest Alpha missing: %q", r.Reason)
	}
	if !strings.Contains(r.Reason, "Beta") || !strings.Contains(r.Reason, "lines 200-240") {
		t.Errorf("nearest Beta missing: %q", r.Reason)
	}
	if !strings.Contains(r.Reason, "Gamma") {
		t.Errorf("third-nearest Gamma missing: %q", r.Reason)
	}
	if strings.Contains(r.Reason, "Delta") {
		t.Errorf("beyond-top-3 Delta leaked into hint: %q", r.Reason)
	}
}

// TestGroundCitation_NoSymbolsNoHint — when the file has zero indexed
// symbols (e.g. YAML/JSON config), rejection text has no candidate
// section. (Actually such files accept any positive line under the
// structural-region rule, so this case rarely hits; the test
// documents the graceful degrade.)
func TestGroundCitation_NearestSymbolsHint_EmptyWhenNoSymbols(t *testing.T) {
	fi := &repomap.FileInfo{RelPath: "a.yaml"}
	got := nearestSymbolsHint(fi, 50)
	if got != "" {
		t.Errorf("empty-symbols file must produce empty hint, got %q", got)
	}
}

// TestBuildContext_PicksUpTurnAArtifactsToolResults pins the
// session-8 fix for trace 1776455705131728812: explorer's read_file
// results live only on Mutable.TurnAArtifacts.ToolResults after
// the stage ends (StageOutput.ToolResults is empty). Citation
// grounding at finalizer-time must still see that history or every
// cite falls through to Tier 2, and the Tier-1-peer pool rule
// drops the whole pool with "LLM never read any cited file".
func TestBuildContext_PicksUpTurnAArtifactsToolResults(t *testing.T) {
	readResult := buildGutterReadResult("a.go", 10, []string{
		"func Foo() {",
		"    bar()",
		"}",
	}, 3)

	mut := types.NewMutableState("q")
	mut.SetTurnAArtifacts(types.TurnAArtifacts{
		ReadFiles:   []string{"a.go"},
		ToolResults: []types.ToolResult{readResult},
	})
	bus := &types.BusContext{Mutable: mut}

	gc := BuildContext(bus)
	if _, ok := gc.LineIndex["a.go"]; !ok {
		t.Fatalf("a.go missing from line index built from TurnA snapshot: %+v", gc.LineIndex)
	}

	// Tier 1 citation grounding must now succeed against the
	// snapshot-sourced gutter.
	rep := GroundCitation(types.Citation{File: "a.go", Line: 10, Quote: "func Foo"}, gc)
	if !rep.Valid || rep.Tier != types.TierLineText {
		t.Errorf("expected Tier 1 grounding via TurnA snapshot, got valid=%v tier=%q",
			rep.Valid, rep.Tier)
	}
}

// TestConditionKeywords_CoversAllSupportedLanguages locks the
// recoverNearestCondition / graphMatchCondition lexical fallback
// against every language repomap supports. For each language we
// build a synthetic 1-line fixture using that language's canonical
// conditional construct and assert lineContainsAnyKeyword catches
// it via the conditionKeywords table.
//
// 2026-05-08 audit: Rust / Python 3.10+ / Cangjie all use `match`
// for pattern-matching conditionals — previously absent from the
// table. Go's channel `select { ... }` likewise missing. This test
// pins the gap closures so a future trim of the keyword list
// cannot silently regress recovery for those languages.
func TestConditionKeywords_CoversAllSupportedLanguages(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		// Universal `if` / `if(`.
		{"go-if", "if x > 0 {"},
		{"java-if", "if (x > 0) {"},
		{"python-if", "if x > 0:"},
		{"rust-if", "if x > 0 {"},
		{"c-if", "if (x > 0) {"},
		{"swift-if", "if x > 0 {"},
		{"ruby-if", "if x > 0"},
		{"lua-if", "if x > 0 then"},
		{"kotlin-if", "if (x > 0) {"},
		{"cangjie-if", "if (x > 0) {"},
		{"arkts-if", "if (x > 0) {"},
		// switch / case (Go, C, Java, JS, TS, Swift, Obj-C, CUDA).
		{"go-switch", "switch kind {"},
		{"c-switch", "switch (kind) {"},
		{"java-case", "case 1:"},
		{"swift-case", "case .ready:"},
		// when (Kotlin), unless (Ruby), guard (Swift).
		{"kotlin-when", "when (state) {"},
		{"ruby-when", "when 1"},
		{"ruby-unless", "unless x.nil?"},
		{"swift-guard", "guard let v = optional else { return }"},
		// 2026-05-08 add: match (Rust / Python 3.10+ / Cangjie).
		{"rust-match", "match value {"},
		{"python-match", "match command:"},
		{"cangjie-match", "match (kind) {"},
		// 2026-05-08 add: Go select.
		{"go-select", "select {"},
		{"go-select-tight", "select{"},
		// 2026-05-08 add: exception-handling forms.
		{"java-try", "try {"},
		{"python-try", "try:"},
		{"swift-try-tight", "try{"},
		{"java-catch-paren", "catch (Exception e) {"},
		{"java-catch-paren-tight", "catch(Exception e) {"},
		{"swift-catch-brace", "} catch {"},
		{"python-except", "except ValueError as e:"},
		{"python-except-bare", "except:"},
		{"ruby-rescue", "rescue StandardError => e"},
		{"java-finally", "finally {"},
		{"python-finally", "finally:"},
		{"swift-finally-tight", "finally{"},
		{"ruby-ensure", "ensure cleanup"},
		{"java-throw", "throw new IllegalArgumentException(msg);"},
		{"python-raise", "raise ValueError('bad input')"},
	}
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"fixture.txt": {},
		},
	}
	for i, c := range cases {
		gc.LineIndex["fixture.txt"][i+1] = c.line
		it := &types.EvidenceItem{Source: "fixture.txt", LineStart: i + 1}
		if !lineContainsAnyKeyword(it, gc, conditionKeywords) {
			t.Errorf("%s: line %q must match conditionKeywords (line=%d)", c.name, c.line, i+1)
		}
	}
}

// TestConditionKeywords_RejectsUnrelatedLines guards against false-
// positive matches the 2026-05-08 expansion might have introduced.
// Lines that do not contain a true conditional construct must NOT
// match — otherwise the recovery snaps to a noise line and the
// citation lands somewhere semantically irrelevant.
func TestConditionKeywords_RejectsUnrelatedLines(t *testing.T) {
	negatives := []struct {
		name string
		line string
	}{
		// SQL prose in a doc-string (the `select {` constraint
		// avoids matching bare uppercase SQL SELECT).
		{"sql-uppercase", "// SELECT * FROM users"},
		// `select` without the `{` it would have in Go is also
		// not a conditional anchor.
		{"select-bare", "select * from users where id = 1"},
	}
	gc := &Context{
		LineIndex: map[string]map[int]string{
			"fixture.txt": {},
		},
	}
	for i, c := range negatives {
		gc.LineIndex["fixture.txt"][i+1] = c.line
		it := &types.EvidenceItem{Source: "fixture.txt", LineStart: i + 1}
		if lineContainsAnyKeyword(it, gc, conditionKeywords) {
			t.Errorf("%s: line %q must NOT match conditionKeywords (line=%d)", c.name, c.line, i+1)
		}
	}
}
