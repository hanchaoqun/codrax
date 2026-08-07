package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
	"github.com/hanchaoqun/codrax/internal/types"
)

func callChainFrontierContext(source string) *types.AgentContext {
	return &types.AgentContext{AnalysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
		Intent:        types.IntentTrace,
		PredicateAxis: types.AxisCall,
		AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqCallChain)},
		CallChainEndpointProfile: &types.CallChainEndpointProfile{
			Source: source, Sink: "Terminal.run", SinkMode: types.CallChainSinkResolutionExact,
		},
	}}}
}

func TestRenderExplorerCallChainDirectCallFrontier_PublishesBoundedASTNavigationWithoutMintingEvidence(t *testing.T) {
	file := &repomap.FileInfo{
		RelPath: "internal/agent/analyzer.go", Language: repomap.LangGo, Package: "agent",
		Symbols: []repomap.Symbol{
			{Name: "buildAnalysisIR", Kind: "function", File: "internal/agent/analyzer.go", Line: 10, EndLine: 100},
			{Name: "detectLanguage", Kind: "function", File: "internal/agent/analyzer.go", Line: 110, EndLine: 115},
			{Name: "analyzerGraphForNormalize", Kind: "function", File: "internal/agent/analyzer.go", Line: 120, EndLine: 125},
			{Name: "nested", Kind: "function", File: "internal/agent/analyzer.go", Line: 40, EndLine: 50},
		},
		Relations: []repomap.Relation{
			{Kind: "call", File: "internal/agent/analyzer.go", Line: 14, ToEP: repomap.RelationEndpoint{Name: "detectLanguage"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call"},
			{Kind: "call", File: "internal/agent/analyzer.go", Line: 20, ToEP: repomap.RelationEndpoint{Name: "analyzerGraphForNormalize"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call"},
			{Kind: "call", File: "internal/agent/analyzer.go", Line: 45, ToEP: repomap.RelationEndpoint{Name: "insideNested"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_call"},
			{Kind: "call", File: "internal/agent/analyzer.go", Line: 60, ToEP: repomap.RelationEndpoint{Name: "regexOnly"}, Confidence: repotypes.ConfidenceRegexSalvage, Provenance: repotypes.ProvenanceRegexFallback, ResolvedBy: "regex"},
			{Kind: "call", File: "internal/agent/analyzer.go", Line: 90, ToEP: repomap.RelationEndpoint{Name: "Run", Receiver: "dynamicGate"}, Confidence: repotypes.ConfidenceAST, Provenance: repotypes.ProvenanceTreeSitter, ResolvedBy: "go_ast_selector_call"},
		},
	}
	graph := repomap.BuildGraph(t.TempDir(), []*repomap.FileInfo{file})
	got := renderExplorerCallChainDirectCallFrontier(callChainFrontierContext("buildAnalysisIR"), graph)
	for _, want := range []string{
		"Typed Direct-call Frontier (advisory)",
		"not answer evidence and not a required member list",
		"select only calls that are load-bearing",
		"sibling edges from the same caller are not concurrent",
		"analyzerGraphForNormalize",
		"dynamicGate.Run",
		"unresolved syntax surface; inspect the line",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("direct-call frontier missing %q:\n%s", want, got)
		}
	}
	for _, forbidden := range []string{"insideNested", "regexOnly", "emit_evidence items"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("frontier admitted forbidden/non-direct authority %q:\n%s", forbidden, got)
		}
	}

	// Raw objective text is not an input to the typed frontier selector.
	other := callChainFrontierContext("buildAnalysisIR")
	other.Objective = "completely different prose including analyzerGraphForNormalize"
	if gotOther := renderExplorerCallChainDirectCallFrontier(other, graph); gotOther != got {
		t.Fatalf("frontier must be independent of raw objective prose:\nA=%s\nB=%s", got, gotOther)
	}
}

func TestExplorerUniqueCallChainSourceDefinition_AllReadLanguagesUseSharedQualifiedResolution(t *testing.T) {
	cases := []struct {
		name     string
		language string
		endpoint string
		owner    string
		parent   bool
	}{
		{name: "go", language: repomap.LangGo, endpoint: "run"},
		{name: "python", language: repomap.LangPython, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "javascript", language: repomap.LangJavaScript, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "typescript", language: repomap.LangTypeScript, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "java", language: repomap.LangJava, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "kotlin", language: repomap.LangKotlin, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "rust", language: repomap.LangRust, endpoint: "pipeline::run", owner: "pipeline"},
		{name: "c", language: repomap.LangC, endpoint: "run"},
		{name: "cpp", language: repomap.LangCpp, endpoint: "Pipeline::run", owner: "Pipeline"},
		{name: "ruby", language: repomap.LangRuby, endpoint: "Pipeline#run", owner: "Pipeline"},
		{name: "swift", language: repomap.LangSwift, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "lua", language: repomap.LangLua, endpoint: "Pipeline.run", owner: "Pipeline"},
		{name: "proto", language: repomap.LangProto, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "arkts", language: repomap.LangArkTS, endpoint: "Pipeline.run", owner: "Pipeline", parent: true},
		{name: "cangjie", language: repomap.LangCangjie, endpoint: "pipeline::run", owner: "pipeline"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sym := repomap.Symbol{Name: "run", Kind: "function", File: "src/pipeline/main.src", Line: 10, EndLine: 20}
			if tc.owner != "" {
				sym.Kind = "method"
				if tc.parent {
					sym.Parent = tc.owner
				} else {
					sym.Receiver = tc.owner
				}
			}
			fi := &repomap.FileInfo{RelPath: sym.File, Language: tc.language, Package: "pipeline", Symbols: []repomap.Symbol{sym}}
			graph := repomap.BuildGraph(t.TempDir(), []*repomap.FileInfo{fi})
			_, got, ok := explorerUniqueCallChainSourceDefinition(graph, tc.endpoint)
			if !ok || got == nil || got.Line != 10 {
				t.Fatalf("shared qualified resolver did not resolve %s endpoint %q: ok=%t sym=%+v", tc.name, tc.endpoint, ok, got)
			}
		})
	}
}

func TestRenderExplorerCallChainDirectCallFrontier_FailsOpenOnAmbiguityAndBypassesRuntimeTrace(t *testing.T) {
	files := []*repomap.FileInfo{
		{RelPath: "a/run.go", Language: repomap.LangGo, Package: "a", Symbols: []repomap.Symbol{{Name: "run", Kind: "function", File: "a/run.go", Line: 1, EndLine: 4}}},
		{RelPath: "b/run.go", Language: repomap.LangGo, Package: "b", Symbols: []repomap.Symbol{{Name: "run", Kind: "function", File: "b/run.go", Line: 1, EndLine: 4}}},
	}
	graph := repomap.BuildGraph(t.TempDir(), files)
	if got := renderExplorerCallChainDirectCallFrontier(callChainFrontierContext("run"), graph); got != "" {
		t.Fatalf("ambiguous source definition must not produce a frontier: %s", got)
	}

	single := repomap.BuildGraph(t.TempDir(), files[:1])
	ctx := callChainFrontierContext("run")
	ctx.AttachedHitrace = "customer.trace"
	if got := renderExplorerCallChainDirectCallFrontier(ctx, single); got != "" {
		t.Fatalf("runtime trace context must bypass source direct-call frontier: %s", got)
	}
}

func TestBuildInitialInstruction_WiresTypedDirectCallFrontier(t *testing.T) {
	source, err := os.ReadFile("explorer.go")
	if err != nil {
		t.Fatal(err)
	}
	needle := "b.WriteString(renderExplorerCallChainDirectCallFrontier(ctx, sr.Graph))"
	if !strings.Contains(string(source), needle) {
		t.Fatalf("production BuildInitialInstruction wiring missing %q", needle)
	}
}

func TestRenderExplorerCallChainDirectCallFrontier_RepositoryGraphPublishesEarlyHelper(t *testing.T) {
	packageDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	repoRoot := filepath.Clean(filepath.Join(packageDir, "..", ".."))
	graph, err := repomap.BuildOrLoadGraph(repoRoot, "buildAnalysisIR")
	if err != nil {
		t.Fatal(err)
	}
	ctx := callChainFrontierContext("buildAnalysisIR")
	ctx.AnalysisIR.RequestModel.CallChainEndpointProfile.Sink = "gate.Run"
	got := renderExplorerCallChainDirectCallFrontier(ctx, graph)
	for _, want := range []string{
		"Typed Direct-call Frontier (advisory)",
		"analyzerGraphForNormalize",
		"gate.RunWith",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("repository graph frontier missing %q:\n%s", want, got)
		}
	}
}

func TestExplorerEndpointVicinityScore_PrioritizesTypedEndpointWithoutEquatingUnrelatedOwners(t *testing.T) {
	for _, candidate := range []string{"gate.RunWith", "gate::RunWith", "gate->RunWith", "gate#RunWith"} {
		if got := explorerEndpointVicinityScore(candidate, "gate.Run"); got <= 0 {
			t.Fatalf("same-owner typed sibling vicinity should be navigable for %q, score=%d", candidate, got)
		}
	}
	if got := explorerEndpointVicinityScore("other.RunWith", "gate.Run"); got != 0 {
		t.Fatalf("different owner must not gain sibling vicinity, score=%d", got)
	}
	if got := explorerEndpointVicinityScore("gate.Run", "gate.Run"); got <= explorerEndpointVicinityScore("gate.RunWith", "gate.Run") {
		t.Fatalf("exact endpoint must outrank sibling vicinity, exact=%d sibling=%d", got, explorerEndpointVicinityScore("gate.RunWith", "gate.Run"))
	}
}
