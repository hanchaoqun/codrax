package ground

import (
	"os"
	"path/filepath"
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

// TestGroundItem_CallAnchorAcceptsCallExpressionFromLineFeatures
// pins the architectural fix layer: when tree-sitter has populated
// FileInfo.LineFeatures with LineFeatureCallExpression at the cited
// line, lineCorroboratesCallSite returns true and the regex-shape
// classifier's "definition-shaped" verdict is overridden.
//
// This is the precise typed-signal path — AST evidence beats regex
// heuristic for hard-gate decisions. The fallback regex prefix list
// in sourceLineStartsWithControlKeyword catches the same patterns
// (yield* / await / yield from) when the graph is unavailable, but
// the AST signal is the authoritative source when present.
func TestGroundItem_CallAnchorAcceptsCallExpressionFromLineFeatures(t *testing.T) {
	// Simulated: TS Effect.gen call `yield* assertExternalDirectoryEffect(ctx, filePath)`.
	// The regex shape classifier would mis-tag this as a C-family
	// type-prefixed definition (`yield*` head + `assert...(args)`
	// body); LineFeatures from tree-sitter tags it as a call.
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"opencode/packages/opencode/src/tool/apply_patch.ts": {
				RelPath:  "opencode/packages/opencode/src/tool/apply_patch.ts",
				Language: "typescript",
				LineFeatures: map[int][]repomap.LineFeature{
					74: {repomap.LineFeatureCallExpression},
				},
			},
		},
	}
	history := []types.ToolResult{
		buildGutterReadResult(
			"opencode/packages/opencode/src/tool/apply_patch.ts",
			70,
			[]string{
				"      const ctx = yield* PatchCtx.context",
				"      for (const filePath of files) {",
				"        const before = readFile(filePath)",
				"        // line 74 is the call below",
				"        yield* assertExternalDirectoryEffect(ctx, filePath)",
				"        applyHunk(filePath, hunk)",
			},
			80,
		),
	}
	gc := &Context{
		LineIndex: buildLineIndex(history, ""),
		Graph:     graph,
	}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceRelationship,
		Source:       "opencode/packages/opencode/src/tool/apply_patch.ts",
		LineStart:    74,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "assertExternalDirectoryEffect",
		Subject:      "ApplyPatchTool.run",
		Object:       "assertExternalDirectoryEffect",
		Snippet:      "yield* assertExternalDirectoryEffect(ctx, filePath)",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingUngrounded &&
		strings.Contains(it.GroundingNote, "definition-shaped") {
		t.Fatalf("LineFeatureCallExpression at line 74 must override regex shape: status=%q note=%q",
			it.GroundingStatus, it.GroundingNote)
	}
}

// TestGroundItem_CallAnchorAcceptsJSExpressionPrefixedCallSites pins
// the 2026-05-17 grounder fix: TypeScript Effect.gen / async patterns
// like `yield* funcName(args)` and `await funcName(args)` are CALL
// SITES, not definitions. Before the fix, cFamilyDefinitionLineRe
// matched these as type-prefixed C-style function declarations
// (`yield*` slotted into the "type-token + space" regex head because
// asterisk is in the type-token char class for pointer return types).
// sourceLineStartsWithControlKeyword now short-circuits these
// expression operators before the definition regex runs.
func TestGroundItem_CallAnchorAcceptsJSExpressionPrefixedCallSites(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		line   string
		symbol string
	}{
		{
			name:   "ts_yield_star_generator_delegation",
			path:   "opencode/packages/opencode/src/tool/apply_patch.ts",
			line:   "        yield* assertExternalDirectoryEffect(ctx, filePath)",
			symbol: "assertExternalDirectoryEffect",
		},
		{
			name:   "ts_yield_star_short_call",
			path:   "opencode/packages/opencode/src/tool/shell.ts",
			line:   "      yield* ask(ctx, scan)",
			symbol: "ask",
		},
		{
			name:   "ts_await_async_call",
			path:   "opencode/packages/opencode/src/session/session.ts",
			line:   "      await fetchUserData(userId)",
			symbol: "fetchUserData",
		},
		{
			name:   "python_yield_from_delegation",
			path:   "service.py",
			line:   "      yield from compute_results(seed)",
			symbol: "compute_results",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []types.ToolResult{
				buildGutterReadResult(tc.path, 70, []string{tc.line, "      return value", "    }"}, 80),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceRelationship,
				Source:       tc.path,
				LineStart:    70,
				AnchorKind:   types.AnchorCall,
				AnchorSymbol: tc.symbol,
				Subject:      tc.symbol,
				Snippet:      tc.line,
			}
			GroundItem(it, gc)
			if it.GroundingStatus == types.GroundingUngrounded &&
				strings.Contains(it.GroundingNote, "definition-shaped") {
				t.Fatalf("%s: call-site falsely rejected as definition-shaped — note=%q",
					tc.name, it.GroundingNote)
			}
		})
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

func TestGroundItem_CallAnchorRejectsGoMethodDefinitionLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/agent.go", 859, []string{
			"func (b *BaseAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {",
			"\ttoolSchemas := b.buildToolSchemas(sk, ctx)",
		}, 2284),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceRelationship,
		Source:       "internal/agent/agent.go",
		LineStart:    859,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "Execute",
		Subject:      "Execute",
		Snippet:      "func (b *BaseAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("Go method definition must not ground as a callsite: status=%q tier=%q line=%d note=%q",
			it.GroundingStatus, it.GroundingTier, it.LineStart, it.GroundingNote)
	}
	if !strings.Contains(it.GroundingNote, "definition-shaped source line") {
		t.Fatalf("repair note should explain anchor_kind mismatch, got %q", it.GroundingNote)
	}
}

func TestGroundItem_CallAnchorRejectsSupportedLanguageDefinitionLines(t *testing.T) {
	cases := []struct {
		name   string
		lang   string
		path   string
		line   string
		symbol string
	}{
		{"go_function", repomap.LangGo, "svc.go", "func Execute(ctx context.Context) error {", "Execute"},
		{"go_method", repomap.LangGo, "svc.go", "func (s *Service) Execute(ctx context.Context) error {", "Execute"},
		{"python", repomap.LangPython, "svc.py", "async def execute(request):", "execute"},
		{"python_lambda_assignment", repomap.LangPython, "svc.py", "execute = lambda request: request.ok", "execute"},
		{"javascript", repomap.LangJavaScript, "svc.js", "export function execute(request) {", "execute"},
		{"javascript_arrow_assignment", repomap.LangJavaScript, "svc.js", "const execute = async (request) => {", "execute"},
		{"typescript_method", repomap.LangTypeScript, "svc.ts", "public execute(request: Request): Response {", "execute"},
		{"typescript_arrow_assignment", repomap.LangTypeScript, "svc.ts", "const execute: Handler = (request: Request): Response => {", "execute"},
		{"arkts_function", repomap.LangArkTS, "svc.ets", "@Builder function Execute() {", "Execute"},
		{"arkts_build_method", repomap.LangArkTS, "svc.ets", "build() {", "build"},
		{"java", repomap.LangJava, "Svc.java", "public Result execute(Request request) {", "execute"},
		{"kotlin", repomap.LangKotlin, "Svc.kt", "override fun execute(request: Request): Response {", "execute"},
		{"rust", repomap.LangRust, "svc.rs", "pub fn execute(request: Request) -> Response {", "execute"},
		{"c", repomap.LangC, "svc.c", "int execute(struct request *request) {", "execute"},
		{"cpp", repomap.LangCpp, "svc.cpp", "Result Service::execute(const Request& request) {", "execute"},
		{"cuda_cpp_remap", repomap.LangCpp, "kernel.cu", "__global__ void execute(Request request) {", "execute"},
		{"objective_c_remap", repomap.LangC, "Svc.m", "- (void)execute:(Request *)request {", "execute"},
		{"ruby", repomap.LangRuby, "svc.rb", "def execute(request)", "execute"},
		{"swift", repomap.LangSwift, "Svc.swift", "public func execute(_ request: Request) -> Response {", "execute"},
		{"lua", repomap.LangLua, "svc.lua", "local function execute(request)", "execute"},
		{"proto", repomap.LangProto, "svc.proto", "rpc Execute (Request) returns (Response);", "Execute"},
		{"cangjie", repomap.LangCangjie, "svc.cj", "public func execute(request: Request): Response {", "execute"},
		{"cangjie_foreign", repomap.LangCangjie, "svc.cj", "foreign func native_add(a: Int64, b: Int64): Int64", "native_add"},
		{"cangjie_operator", repomap.LangCangjie, "svc.cj", "public operator func +(other: V): V {", "+"},
	}
	coveredLanguages := map[string]bool{}
	for _, tc := range cases {
		coveredLanguages[tc.lang] = true
	}
	for _, lang := range repomap.SupportedReadLanguages() {
		if !coveredLanguages[lang] {
			t.Fatalf("definition-shape call-anchor guard lacks coverage for supported language %q", lang)
		}
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []types.ToolResult{
				buildGutterReadResult(tc.path, 10, []string{tc.line}, 100),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceRelationship,
				Source:       tc.path,
				LineStart:    10,
				AnchorKind:   types.AnchorCall,
				AnchorSymbol: tc.symbol,
				Subject:      tc.symbol,
				Snippet:      tc.line,
			}
			GroundItem(it, gc)
			if it.GroundingStatus != types.GroundingUngrounded {
				t.Fatalf("%s definition line must not ground as callsite: status=%q tier=%q line=%d note=%q",
					tc.name, it.GroundingStatus, it.GroundingTier, it.LineStart, it.GroundingNote)
			}
			if !strings.Contains(it.GroundingNote, "definition-shaped") {
				t.Fatalf("%s repair note should explain definition/call mismatch, got %q", tc.name, it.GroundingNote)
			}
		})
	}
}

func TestGroundItem_CallRecoveryDoesNotJumpFromDefinitionToFarSameNameCall(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/agent/agent.go", 859, []string{
			"func (b *BaseAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {",
			"\ttoolSchemas := b.buildToolSchemas(sk, ctx)",
		}, 2284),
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"internal/agent/agent.go": {
				RelPath: "internal/agent/agent.go",
				Symbols: []repomap.Symbol{
					{Name: "BaseAgent.Execute", Kind: "method", Line: 859},
				},
				Relations: []repomap.Relation{
					{Kind: "call", File: "internal/agent/agent.go", Line: 1842,
						ToEP: repomap.RelationEndpoint{Name: "Execute"}},
				},
			},
		},
	}
	gc := &Context{LineIndex: buildLineIndex(history, ""), Graph: graph}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceRelationship,
		Source:       "internal/agent/agent.go",
		LineStart:    859,
		AnchorKind:   types.AnchorCall,
		AnchorSymbol: "Execute",
		Subject:      "Execute",
		Snippet:      "func (b *BaseAgent) Execute(ctx *types.AgentContext, sk *skill.Config) (*StageOutput, error) {",
	}
	GroundItem(it, gc)
	if it.LineStart != 859 {
		t.Fatalf("definition-shaped call anchor must not jump to far same-name callsite; got line %d", it.LineStart)
	}
	if it.GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("mismatched anchor should be ungrounded, got status=%q tier=%q note=%q",
			it.GroundingStatus, it.GroundingTier, it.GroundingNote)
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

func TestGroundClaimCitation_RejectsDriftWithSameFileCandidate(t *testing.T) {
	lines := make([]string, 55)
	for i := range lines {
		lines[i] = "// unrelated"
	}
	lines[100-98] = "func AllMainStages() []PipelineStage {"
	lines[146-98] = "func AllAgentNames() []AgentName {"
	lines[147-98] = "\treturn []AgentName{AgentAnalyzer, AgentExplorer}"
	history := []types.ToolResult{
		buildGutterReadResult("internal/types/enums.go", 98, lines, 200),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}

	report := GroundClaimCitation(
		types.Citation{File: "internal/types/enums.go", Line: 100},
		"AllAgentNames() returns AgentAnalyzer and AgentExplorer entries",
		gc,
	)
	if !report.Valid {
		t.Fatalf("line exists and should be structurally valid: %+v", report)
	}
	if report.Corroborated {
		t.Fatalf("drifted claim citation should not be corroborated: %+v", report)
	}
	if got := FormatClaimCitationCandidates(report.Candidates); !strings.Contains(got, "internal/types/enums.go:146") {
		t.Fatalf("expected precise same-file candidate, got %q", got)
	}
}

func TestGroundClaimCitation_AcceptsIdentifierInReadWindow(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/types/enums.go", 146, []string{
			"func AllAgentNames() []AgentName {",
			"\treturn []AgentName{AgentAnalyzer, AgentExplorer}",
			"}",
		}, 200),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}

	report := GroundClaimCitation(
		types.Citation{File: "internal/types/enums.go", Line: 146},
		"AllAgentNames() returns AgentAnalyzer and AgentExplorer entries",
		gc,
	)
	if !report.Valid || !report.Corroborated {
		t.Fatalf("grounded claim citation should pass: %+v", report)
	}
}

func TestGroundClaimCitation_RejectsUnreadRangeWhenReadHistoryExists(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("a.go", 10, []string{
			"func AlreadyRead() {}",
			"",
			"var Seen = true",
		}, 120),
	}
	graph := &repomap.Graph{
		FileIndex: map[string]*repomap.FileInfo{
			"a.go": {RelPath: "a.go", Symbols: []repomap.Symbol{
				{Name: "Foo", Kind: "function", Line: 100, EndLine: 110},
			}},
		},
	}
	gc := &Context{LineIndex: buildLineIndex(history, ""), Graph: graph}

	report := GroundClaimCitation(
		types.Citation{File: "a.go", Line: 100, LineEnd: 102},
		"Foo returns the agent list",
		gc,
	)
	if report.Valid {
		t.Fatalf("claim citation should reject ranges outside read_file history despite graph fallback: %+v", report)
	}
	if !strings.Contains(report.Reason, "read_file history") || !strings.Contains(report.Reason, "10-12") {
		t.Fatalf("expected visible read range hint, got %q", report.Reason)
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

func TestBuildContext_ObservedLineIndexIncludesGrepSourceLines(t *testing.T) {
	grepResult := types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 2 matching lines]",
			"[grep params: pattern=extractSubAgentProposal path=internal/orchestrator/orchestrator.go context_lines=1]",
			"internal/orchestrator/orchestrator.go-6064-\t// dispatch sub-agent proposals after the main agent response",
			"internal/orchestrator/orchestrator.go:6065:\tif proposal := extractSubAgentProposal(output, agentName); proposal != nil {",
			"--",
		}, "\n"),
	}

	mut := types.NewMutableState("irrelevant")
	mut.AppendDispatchToolResult(grepResult)
	gc := BuildContext(&types.BusContext{Mutable: mut})

	if _, ok := gc.LineIndex["internal/orchestrator/orchestrator.go"]; ok {
		t.Fatalf("grep output must not be promoted into strict read_file LineIndex: %+v", gc.LineIndex)
	}
	observed := gc.ObservedLineIndex["internal/orchestrator/orchestrator.go"]
	if got := observed[6065]; !strings.Contains(got, "extractSubAgentProposal") {
		t.Fatalf("grep match line missing from ObservedLineIndex: %q", got)
	}
	if got := observed[6064]; !strings.Contains(got, "dispatch sub-agent proposals") {
		t.Fatalf("grep context line missing from ObservedLineIndex: %q", got)
	}
}

func TestGroundItem_UngroundedHintDistinguishesNavigationObservedLine(t *testing.T) {
	grepResult := types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary: strings.Join([]string{
			"[grep: 1 matching line]",
			"kernel/bpf/arraymap.c:811:\t.map_update_elem = array_map_update_elem,",
		}, "\n"),
	}

	mut := types.NewMutableState("irrelevant")
	mut.AppendDispatchToolResult(grepResult)
	gc := BuildContext(&types.BusContext{Mutable: mut})
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Scope:        types.ScopeLine,
		Source:       "kernel/bpf/arraymap.c",
		LineStart:    811,
		AnchorKind:   types.AnchorInitializer,
		AnchorSymbol: "array_map_update_elem",
	}

	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("grep-observed line must not become strict grounded evidence: status=%q note=%q", it.GroundingStatus, it.GroundingNote)
	}
	for _, want := range []string{"navigation/search output", "read_file gutter"} {
		if !strings.Contains(it.GroundingNote, want) {
			t.Fatalf("expected tool-history-aware hint %q, got %q", want, it.GroundingNote)
		}
	}
}

func TestBuildContext_ObservedGrepParserPrefersExistingHyphenatedPath(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "internal", "fixtures"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "internal", "fixtures", "foo-2024-bar.go"), []byte("package fixtures\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	grepResult := types.ToolResult{
		ToolName: "grep",
		Success:  true,
		Summary:  "internal/fixtures/foo-2024-bar.go-7-content includes -99- in text\n",
	}

	mut := types.NewMutableState("irrelevant")
	mut.AppendDispatchToolResult(grepResult)
	gc := BuildContext(&types.BusContext{Mutable: mut, RepoRoot: tmp})

	observed := gc.ObservedLineIndex["internal/fixtures/foo-2024-bar.go"]
	if got := observed[7]; !strings.Contains(got, "content includes -99- in text") {
		t.Fatalf("hyphenated grep context line parsed incorrectly: observed=%+v", observed)
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

func TestGroundItem_LineLocalAnchorUsesExactSnippetWhenSymbolIsDescriptive(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/orchestrator/orchestrator.go", 2268, []string{
			"\tfor attempt := 0; attempt < max; {",
		}, 2300),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceConditional,
		Source:       "internal/orchestrator/orchestrator.go",
		LineStart:    2268,
		AnchorKind:   types.AnchorCondition,
		AnchorSymbol: "for loop",
		Condition:    "attempt < max",
		Snippet:      "for attempt := 0; attempt < max; {",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
		t.Fatalf("line-local exact snippet should ground despite descriptive anchor_symbol: status=%s tier=%s note=%q",
			it.GroundingStatus, it.GroundingTier, it.GroundingNote)
	}
}

func TestGroundItem_LineLocalAnchorDoesNotUseSummaryOnly(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/orchestrator/orchestrator.go", 2268, []string{
			"\tfor attempt := 0; attempt < max; {",
		}, 2300),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceConditional,
		Source:       "internal/orchestrator/orchestrator.go",
		LineStart:    2268,
		AnchorKind:   types.AnchorCondition,
		AnchorSymbol: "retry loop",
		Summary:      "重试循环上界由 dynamicAnalyzeRetries 决定",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded && it.GroundingTier == types.TierLineText {
		t.Fatalf("summary-only line-local evidence must not bypass anchor_symbol grounding")
	}
}

func TestGroundItem_TextReferenceAcceptsCommentLine(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/types/analysis_ir.go", 1235, []string{
			"\t// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
		}, 1300),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/types/analysis_ir.go",
		LineStart:    1235,
		AnchorKind:   types.AnchorTextReference,
		AnchorSymbol: "ShapeValue",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
		t.Fatalf("text_reference comment evidence status=%s tier=%s note=%q, want grounded line_text",
			it.GroundingStatus, it.GroundingTier, it.GroundingNote)
	}
}

func TestGroundItem_TextReferenceAcceptsVisibleCommentPhrase(t *testing.T) {
	cases := []struct {
		name   string
		anchor string
	}{
		{name: "phrase with punctuation", anchor: "Turn B (extractor) evaluator"},
		{name: "filename with dot", anchor: "extractor.go"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []types.ToolResult{
				buildGutterReadResult("internal/agent/extractor.go", 3, []string{
					"// extractor.go — Turn B (extractor) evaluator.",
				}, 40),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceDirect,
				Source:       "internal/agent/extractor.go",
				LineStart:    3,
				AnchorKind:   types.AnchorTextReference,
				AnchorSymbol: tc.anchor,
			}
			GroundItem(it, gc)
			if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
				t.Fatalf("text_reference phrase status=%s tier=%s note=%q, want grounded line_text",
					it.GroundingStatus, it.GroundingTier, it.GroundingNote)
			}
		})
	}
}

func TestGroundItem_StringLiteralAcceptsCrossLanguageLiteralLines(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		line   string
		anchor string
	}{
		{"go", "tool.go", `func (t *Tool) Name() string { return "emit_answer_document" }`, "emit_answer_document"},
		{"python", "tool.py", `return "emit_answer_document"`, "emit_answer_document"},
		{"javascript", "tool.js", `return 'emit_answer_document';`, "emit_answer_document"},
		{"typescript", "tool.ts", `const tool = "emit_answer_document";`, "emit_answer_document"},
		{"arkts", "tool.ets", `let tool: string = "emit_answer_document"`, "emit_answer_document"},
		{"java", "Tool.java", `return "emit_answer_document";`, "emit_answer_document"},
		{"kotlin", "Tool.kt", `return "emit_answer_document"`, "emit_answer_document"},
		{"rust raw string", "tool.rs", `const TOOL: &str = r#"emit_answer_document"#;`, "emit_answer_document"},
		{"cpp", "tool.cpp", `const char* tool = "emit_answer_document";`, "emit_answer_document"},
		{"swift", "Tool.swift", `return "emit_answer_document"`, "emit_answer_document"},
		{"ruby", "tool.rb", `return 'emit_answer_document'`, "emit_answer_document"},
		{"lua", "tool.lua", `return "emit_answer_document"`, "emit_answer_document"},
		{"proto", "tool.proto", `option (tool_name) = "emit_answer_document";`, "emit_answer_document"},
		{"cangjie", "tool.cj", `return "emit_answer_document"`, "emit_answer_document"},
		{"route punctuation", "routes.ts", `router.get("/v1/health-check", handler)`, "/v1/health-check"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []types.ToolResult{
				buildGutterReadResult(tc.path, 20, []string{tc.line}, 40),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceDirect,
				Source:       tc.path,
				LineStart:    20,
				Scope:        types.ScopeLine,
				AnchorKind:   types.AnchorStringLiteral,
				AnchorSymbol: tc.anchor,
			}
			GroundItem(it, gc)
			if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
				t.Fatalf("string literal status=%s tier=%s note=%q, want grounded line_text",
					it.GroundingStatus, it.GroundingTier, it.GroundingNote)
			}
			if it.Snippet != tc.line {
				t.Fatalf("snippet=%q want %q", it.Snippet, tc.line)
			}
		})
	}
}

func TestGroundItem_StringLiteralAcceptsVisibleFragmentInLongerLiteral(t *testing.T) {
	cases := []struct {
		name   string
		line   string
		anchor string
	}{
		{
			name:   "markdown heading prefix before escaped newlines",
			line:   `b.WriteString("## Breadth Scan\n\n")`,
			anchor: "## Breadth Scan",
		},
		{
			name:   "multi-token label inside longer literal",
			line:   `b.WriteString("## Breadth Scan\n\n")`,
			anchor: "Breadth Scan",
		},
		{
			name:   "collapsed escaped whitespace",
			line:   `const label = "Phase 0:\nBreadth Scan"`,
			anchor: "Phase 0: Breadth Scan",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const path = "internal/agent/explorer.go"
			history := []types.ToolResult{
				buildGutterReadResult(path, 508, []string{tc.line}, 520),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceDirect,
				Source:       path,
				LineStart:    508,
				Scope:        types.ScopeLine,
				AnchorKind:   types.AnchorStringLiteral,
				AnchorSymbol: tc.anchor,
			}
			GroundItem(it, gc)
			if it.GroundingStatus != types.GroundingGrounded || it.GroundingTier != types.TierLineText {
				t.Fatalf("string literal fragment status=%s tier=%s note=%q, want grounded line_text",
					it.GroundingStatus, it.GroundingTier, it.GroundingNote)
			}
			if it.Snippet != tc.line {
				t.Fatalf("snippet=%q want %q", it.Snippet, tc.line)
			}
		})
	}
}

func TestGroundItem_StringLiteralFragmentRejectsIdentifierPrefix(t *testing.T) {
	const path = "tool.go"
	history := []types.ToolResult{
		buildGutterReadResult(path, 20, []string{`return "emit_answer_document_patch"`}, 40),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       path,
		LineStart:    20,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorStringLiteral,
		AnchorSymbol: "emit_answer_document",
	}
	GroundItem(it, gc)
	if it.GroundingStatus == types.GroundingGrounded {
		t.Fatalf("identifier prefix inside longer string literal grounded unexpectedly: tier=%s note=%q",
			it.GroundingTier, it.GroundingNote)
	}
}

func TestGroundItem_StringLiteralRejectsIdentifierOrCommentOnlyMentions(t *testing.T) {
	cases := []struct {
		name string
		line string
	}{
		{"identifier only", `emit_answer_document := true`},
		{"comment literal", `// tool name is "emit_answer_document"`},
		{"inline comment literal", `ok := true // "emit_answer_document"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			const path = "tool.go"
			history := []types.ToolResult{
				buildGutterReadResult(path, 30, []string{tc.line}, 50),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceDirect,
				Source:       path,
				LineStart:    30,
				Scope:        types.ScopeLine,
				AnchorKind:   types.AnchorStringLiteral,
				AnchorSymbol: "emit_answer_document",
			}
			GroundItem(it, gc)
			if it.GroundingStatus == types.GroundingGrounded {
				t.Fatalf("non-literal/comment line grounded unexpectedly: tier=%s note=%q", it.GroundingTier, it.GroundingNote)
			}
		})
	}
}

func TestGroundItem_SnippetFuzzyRejectsCommentOnlyCodeAnchor(t *testing.T) {
	history := []types.ToolResult{
		buildGutterReadResult("internal/types/analysis_ir.go", 1235, []string{
			"\t// ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue /",
		}, 1300),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       "internal/types/analysis_ir.go",
		LineStart:    1234,
		AnchorKind:   types.AnchorInitializer,
		AnchorSymbol: "ShapeValue",
		Snippet:      "ShapeStepList / ShapeValue / ShapeBoolean / ShapeConfigValue",
	}
	GroundItem(it, gc)
	if it.GroundingStatus != types.GroundingUngrounded {
		t.Fatalf("comment-only initializer recovered unexpectedly: status=%s tier=%s note=%q",
			it.GroundingStatus, it.GroundingTier, it.GroundingNote)
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

func TestGroundItem_AssignmentAcceptsCrossLanguageMemberInitializers(t *testing.T) {
	cases := []struct {
		name string
		path string
		line string
	}{
		{
			name: "go composite literal field",
			path: "internal/agent/explorer.go",
			line: "\tEntryConditions: critEntry(\"has_enough_facts\"),",
		},
		{
			name: "arkts object literal member",
			path: "entry/src/main/ets/pages/Index.ets",
			line: "\tentryConditions: buildEntry(\"ready\"),",
		},
		{
			name: "typescript quoted object key",
			path: "src/config.ts",
			line: "\t\"entryConditions\": buildEntry(\"ready\"),",
		},
		{
			name: "cpp designated initializer",
			path: "src/config.cpp",
			line: "\t.entry_conditions = build_entry(),",
		},
		{
			name: "cangjie named member fallback",
			path: "src/main.cj",
			line: "\tentryConditions: buildEntry(\"ready\"),",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []types.ToolResult{
				buildGutterReadResult(tc.path, 40, []string{tc.line}, 100),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceDirect,
				Source:       tc.path,
				LineStart:    40,
				Scope:        types.ScopeLine,
				AnchorKind:   types.AnchorAssignment,
				AnchorSymbol: "OwnerSymbolNotOnInitializerLine",
				Subject:      "OwnerSymbolNotOnInitializerLine",
			}
			rep := GroundItem(it, gc)
			if rep.Status != types.GroundingGrounded {
				t.Fatalf("assignment initializer grounding status=%s note=%q, want grounded", rep.Status, rep.Note)
			}
			if it.GroundingTier != types.TierSymbolTable {
				t.Fatalf("assignment initializer tier=%s, want symbol_table fallback", it.GroundingTier)
			}
		})
	}
}

func TestGroundItem_Tier1AcceptsPunctuationPrefixedMemberInitializerTokensAcrossLanguages(t *testing.T) {
	cases := []struct {
		name   string
		path   string
		line   string
		anchor string
	}{
		{
			name:   "c designated initializer field",
			path:   "kernel/bpf/arraymap.c",
			line:   "\t.map_update_elem = array_map_update_elem,",
			anchor: "map_update_elem",
		},
		{
			name:   "c designated initializer value",
			path:   "kernel/bpf/arraymap.c",
			line:   "\t.map_update_elem = array_map_update_elem,",
			anchor: "array_map_update_elem",
		},
		{
			name:   "go composite literal field",
			path:   "internal/agent/explorer.go",
			line:   "\tUpdateElem: arrayMapUpdateElem,",
			anchor: "UpdateElem",
		},
		{
			name:   "typescript quoted object key",
			path:   "src/config.ts",
			line:   "\t\"updateElem\": arrayMapUpdateElem,",
			anchor: "updateElem",
		},
		{
			name:   "arkts object member",
			path:   "entry/src/main/ets/pages/Index.ets",
			line:   "\tupdateElem: arrayMapUpdateElem,",
			anchor: "updateElem",
		},
		{
			name:   "cangjie named member",
			path:   "src/main.cj",
			line:   "\tupdateElem: arrayMapUpdateElem,",
			anchor: "updateElem",
		},
		{
			name:   "kotlin named argument",
			path:   "src/Main.kt",
			line:   "\tupdateElem = arrayMapUpdateElem,",
			anchor: "updateElem",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			history := []types.ToolResult{
				buildGutterReadResult(tc.path, 40, []string{tc.line}, 100),
			}
			gc := &Context{LineIndex: buildLineIndex(history, "")}
			it := &types.EvidenceItem{
				Kind:         types.EvidenceRegistration,
				Source:       tc.path,
				LineStart:    40,
				Scope:        types.ScopeLine,
				AnchorKind:   types.AnchorInitializer,
				AnchorSymbol: tc.anchor,
				Summary:      "member initializer is visible on the cited line",
			}
			rep := GroundItem(it, gc)
			if rep.Status != types.GroundingGrounded {
				t.Fatalf("initializer token grounding status=%s note=%q, want grounded", rep.Status, rep.Note)
			}
			if it.GroundingTier != types.TierLineText {
				t.Fatalf("initializer token tier=%s, want line_text", it.GroundingTier)
			}
		})
	}
}

func TestGroundItem_AssignmentConsumesRepomapLineFeature(t *testing.T) {
	const path = "src/widget.ets"
	history := []types.ToolResult{
		buildGutterReadResult(path, 12, []string{
			"\tentryConditions buildEntry(\"ready\")",
		}, 40),
	}
	gc := &Context{
		LineIndex: buildLineIndex(history, ""),
		Graph: &repomap.Graph{FileIndex: map[string]*repomap.FileInfo{
			path: {
				RelPath:  path,
				Language: repomap.LangArkTS,
				LineFeatures: map[int][]repomap.LineFeature{
					12: {repomap.LineFeatureMemberInitializer},
				},
			},
		}},
	}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       path,
		LineStart:    12,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorAssignment,
		AnchorSymbol: "OwnerSymbolNotOnInitializerLine",
		Subject:      "OwnerSymbolNotOnInitializerLine",
	}
	rep := GroundItem(it, gc)
	if rep.Status != types.GroundingGrounded {
		t.Fatalf("repomap member-initializer feature grounding status=%s note=%q, want grounded", rep.Status, rep.Note)
	}
}

func TestGroundItem_AssignmentRejectsCommentOnlyMemberInitializer(t *testing.T) {
	const path = "internal/agent/explorer.go"
	history := []types.ToolResult{
		buildGutterReadResult(path, 40, []string{
			"\t// EntryConditions: critEntry(\"has_enough_facts\"),",
		}, 100),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       path,
		LineStart:    40,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorAssignment,
		AnchorSymbol: "OwnerSymbolNotOnInitializerLine",
		Subject:      "OwnerSymbolNotOnInitializerLine",
	}
	rep := GroundItem(it, gc)
	if rep.Status == types.GroundingGrounded {
		t.Fatalf("comment-only member initializer grounded unexpectedly: tier=%s note=%q", it.GroundingTier, it.GroundingNote)
	}
}

func TestGroundItem_AssignmentRejectsPureTypeAnnotationMember(t *testing.T) {
	const path = "src/config.ts"
	history := []types.ToolResult{
		buildGutterReadResult(path, 18, []string{
			"\tentryConditions: EntryCondition[];",
		}, 40),
	}
	gc := &Context{LineIndex: buildLineIndex(history, "")}
	it := &types.EvidenceItem{
		Kind:         types.EvidenceDirect,
		Source:       path,
		LineStart:    18,
		Scope:        types.ScopeLine,
		AnchorKind:   types.AnchorAssignment,
		AnchorSymbol: "OwnerSymbolNotOnInitializerLine",
		Subject:      "OwnerSymbolNotOnInitializerLine",
	}
	rep := GroundItem(it, gc)
	if rep.Status == types.GroundingGrounded {
		t.Fatalf("pure type annotation grounded as assignment unexpectedly: tier=%s note=%q", it.GroundingTier, it.GroundingNote)
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
