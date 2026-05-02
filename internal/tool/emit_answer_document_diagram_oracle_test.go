package tool

import (
	"strings"
	"testing"

	repomap "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

// makeBusContextWithGraph wires a fresh BusContext that exposes
// the supplied graph via Mutable.SearchGraph(); the diagram
// oracle reads the graph through the same accessor as production.
func makeBusContextWithGraph(g *repomap.Graph) *types.BusContext {
	mu := types.NewMutableState("test")
	if g != nil {
		mu.SetSearchGraph(g)
	}
	return &types.BusContext{Mutable: mu}
}

// graphWithSymbols builds a minimal Graph populating SymbolDefs +
// FileIndex for the provided (name, tier) pairs. Each entry is
// stored at file "<name>.go".
func graphWithSymbols(symTiers map[string]int) *repomap.Graph {
	g := &repomap.Graph{
		SymbolDefs: map[string][]*repomap.Symbol{},
		FileIndex:  map[string]*repomap.FileInfo{},
	}
	for name, tier := range symTiers {
		path := name + ".go"
		fi := &repomap.FileInfo{RelPath: path, Language: "go", ParseTier: tier}
		g.FileIndex[path] = fi
		g.SymbolDefs[name] = []*repomap.Symbol{{Name: name, File: path}}
	}
	return g
}

// countDiagramViolations is the test helper for asserting how
// many ViolDiagramIdentifier entries landed in the closure.
func countDiagramViolations(ctx *types.BusContext) int {
	if ctx == nil || ctx.Mutable == nil {
		return 0
	}
	closure := ctx.Mutable.EvidenceClosure()
	if closure == nil {
		return 0
	}
	count := 0
	for _, v := range closure.Violations() {
		if v.Kind == types.ViolDiagramIdentifier {
			count++
		}
	}
	return count
}

// TestDiagramOracle_NilContextNoOp pins the back-compat contract:
// missing ctx / mutable / graph = silent skip, no panic.
func TestDiagramOracle_NilContextNoOp(t *testing.T) {
	maybeRunDiagramIdentifierOracle("```mermaid\nA[FakeFunc] --> B\n```", nil, nil)
	// no panic = pass
	ctx := &types.BusContext{}
	maybeRunDiagramIdentifierOracle("```mermaid\nA --> B\n```", nil, ctx)
	// nil graph, also fine
	ctx = makeBusContextWithGraph(nil)
	maybeRunDiagramIdentifierOracle("```mermaid\nA[handleRequest] --> B\n```", nil, ctx)
	if got := countDiagramViolations(ctx); got != 0 {
		t.Errorf("nil graph should yield zero violations; got %d", got)
	}
}

// TestDiagramOracle_KnownIdentifierPasses pins the happy path:
// a CamelCase identifier the graph knows at Tier 1 is silently
// accepted.
func TestDiagramOracle_KnownIdentifierPasses(t *testing.T) {
	g := graphWithSymbols(map[string]int{"handleRequest": 1, "GetUserToken": 1})
	ctx := makeBusContextWithGraph(g)
	summary := "```mermaid\nA[handleRequest] --> B[GetUserToken]\n```"
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 0 {
		t.Errorf("known Tier-1 identifiers should pass; got %d violations", got)
	}
}

// TestDiagramOracle_UnknownIdentifierViolates pins the reject
// path: an LLM-hallucinated CamelCase name lands as a violation.
func TestDiagramOracle_UnknownIdentifierViolates(t *testing.T) {
	g := graphWithSymbols(map[string]int{"handleRequest": 1})
	ctx := makeBusContextWithGraph(g)
	summary := "```mermaid\nA[handleRequest] --> B[completelyMadeUpFunc]\n```"
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 1 {
		t.Errorf("unknown identifier should produce 1 violation; got %d", got)
	}
}

// TestDiagramOracle_SingleEnglishWordIgnored pins the regex
// filter: bare words (User / Action) don't even match the
// identifier shape, so they silently pass without the oracle
// being consulted.
func TestDiagramOracle_SingleEnglishWordIgnored(t *testing.T) {
	g := graphWithSymbols(map[string]int{}) // empty graph
	ctx := makeBusContextWithGraph(g)
	summary := "```mermaid\nA[User] --> B[Action]\nC[Service] --> D[Login]\n```"
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 0 {
		t.Errorf("single-word labels should not trigger oracle; got %d violations", got)
	}
}

// TestDiagramOracle_WhitelistSuppresses pins the operator-
// override path: a hallucinated-looking identifier that is in the
// whitelist passes silently.
func TestDiagramOracle_WhitelistSuppresses(t *testing.T) {
	t.Cleanup(func() { SetDiagramIdentifierWhitelist(nil) })
	SetDiagramIdentifierWhitelist([]string{"HttpRequest"})
	g := graphWithSymbols(map[string]int{}) // empty graph
	ctx := makeBusContextWithGraph(g)
	summary := "```mermaid\nA[HttpRequest] --> B\n```"
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 0 {
		t.Errorf("whitelisted identifier should pass; got %d violations", got)
	}
}

// TestDiagramOracle_Tier3PlusRejected pins the tier-floor: even
// when the identifier resolves, a low-confidence (Tier 3+) match
// is treated as not-validated and lands a violation.
func TestDiagramOracle_Tier3PlusRejected(t *testing.T) {
	g := graphWithSymbols(map[string]int{"weakMatch": 4})
	ctx := makeBusContextWithGraph(g)
	summary := "```mermaid\nA[weakMatch] --> B\n```"
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 1 {
		t.Errorf("Tier-4 match should yield 1 violation; got %d", got)
	}
}

// TestDiagramOracle_NonMermaidFenceIgnored pins fence-scoping:
// a bash / json / generic fenced block is NOT validated because
// node labels there can legitimately mention any identifier
// (CLI args, JSON keys, etc.).
func TestDiagramOracle_NonMermaidFenceIgnored(t *testing.T) {
	g := graphWithSymbols(map[string]int{}) // empty graph
	ctx := makeBusContextWithGraph(g)
	summary := strings.Join([]string{
		"```bash",
		"my_function --arg=value",
		"```",
		"```json",
		"{\"keyName\": \"foo\"}",
		"```",
	}, "\n")
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 0 {
		t.Errorf("non-mermaid fences should not be validated; got %d violations", got)
	}
}

// TestDiagramOracle_LabelGroundedByCitation pins the per-label
// citation-grounding short-circuit added 2026-05-02 after Run 2 of
// the audit triggered 4 false-positive violations on labels that
// named local variables and field tags inside otherwise grounded
// nodes. The label
//
//	for i < maxIter (internal/agent/agent.go:925)
//
// has `internal/agent/agent.go` in citations[]; the bare `maxIter`
// inside the same label inherits that grounding because the WHOLE
// label is the cited evidence.
func TestDiagramOracle_LabelGroundedByCitation(t *testing.T) {
	g := graphWithSymbols(map[string]int{})
	ctx := makeBusContextWithGraph(g)
	citations := []types.Citation{
		{File: "internal/agent/agent.go", Line: 925},
		{File: "internal/types/config.go", Line: 256},
	}
	summary := strings.Join([]string{
		"```mermaid",
		"flowchart TD",
		"    A[\"for i < maxIter (internal/agent/agent.go:925)\"]",
		"    B[\"max_iterations field (internal/types/config.go:256)\"]",
		"    A --> B",
		"```",
	}, "\n")
	maybeRunDiagramIdentifierOracle(summary, citations, ctx)
	if got := countDiagramViolations(ctx); got != 0 {
		t.Errorf("labels containing cited file paths should be grounded; got %d violations", got)
	}
}

// TestDiagramOracle_LabelWithoutCitationStillFlags pins that the
// citation-grounding bypass is per-label, not global: a separate
// label without a cited path is still validated normally.
func TestDiagramOracle_LabelWithoutCitationStillFlags(t *testing.T) {
	g := graphWithSymbols(map[string]int{})
	ctx := makeBusContextWithGraph(g)
	citations := []types.Citation{
		{File: "internal/agent/agent.go", Line: 925},
	}
	summary := strings.Join([]string{
		"```mermaid",
		"flowchart TD",
		"    A[\"for i < maxIter (internal/agent/agent.go:925)\"]",
		"    B[\"hallucinatedFunc orchestrator hop\"]",
		"    A --> B",
		"```",
	}, "\n")
	maybeRunDiagramIdentifierOracle(summary, citations, ctx)
	// Label A is grounded by citation; label B has no cited path
	// so its `hallucinatedFunc` token must still produce a
	// violation.
	if got := countDiagramViolations(ctx); got != 1 {
		t.Errorf("ungrounded label must still flag bare identifier; got %d violations", got)
	}
}

// TestDiagramOracle_PathLikeButNotCited pins that any file-shaped
// token inside a label is checked against the actual citations[]
// set — a path that LOOKS like a real file but isn't cited cannot
// be used to launder ungrounded identifiers.
func TestDiagramOracle_PathLikeButNotCited(t *testing.T) {
	g := graphWithSymbols(map[string]int{})
	ctx := makeBusContextWithGraph(g)
	citations := []types.Citation{
		{File: "internal/agent/agent.go", Line: 925},
	}
	summary := strings.Join([]string{
		"```mermaid",
		"flowchart TD",
		"    A[\"hallucinatedFunc (made/up/path.go:99)\"]",
		"```",
	}, "\n")
	maybeRunDiagramIdentifierOracle(summary, citations, ctx)
	if got := countDiagramViolations(ctx); got != 1 {
		t.Errorf("path that isn't in citations[] must not exempt label; got %d violations", got)
	}
}

// TestDiagramOracle_DedupsAcrossLabels pins per-Run dedup: the
// same hallucinated identifier appearing in multiple boxes lands
// ONE violation, not N.
func TestDiagramOracle_DedupsAcrossLabels(t *testing.T) {
	g := graphWithSymbols(map[string]int{}) // empty graph
	ctx := makeBusContextWithGraph(g)
	summary := strings.Join([]string{
		"```mermaid",
		"A[fakeHandler] --> B[fakeHandler]",
		"C[fakeHandler] --> D[fakeHandler]",
		"```",
	}, "\n")
	maybeRunDiagramIdentifierOracle(summary, nil, ctx)
	if got := countDiagramViolations(ctx); got != 1 {
		t.Errorf("repeated identifier should dedup to 1 violation; got %d", got)
	}
}
