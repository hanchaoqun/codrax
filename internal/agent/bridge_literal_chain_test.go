package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	"github.com/hanchaoqun/codrax/internal/types"
)

// buildFakeGraph constructs an in-memory repomap.Graph pointing at
// real files written into a temp dir. We only populate the fields
// extractBridgeLiteralChains actually consults: graph.Files with
// file symbols that carry Name/Kind/Receiver/Line/EndLine. It avoids
// the real tree-sitter extractor so tests are deterministic.
func buildFakeGraph(t *testing.T, files map[string][]repomap.Symbol, contents map[string]string) (*repomap.Graph, string) {
	t.Helper()
	root := t.TempDir()
	for rel, body := range contents {
		abs := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
			t.Fatalf("mkdirall: %v", err)
		}
		if err := os.WriteFile(abs, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	g := &repomap.Graph{
		Root:       root,
		FileIndex:  make(map[string]*repomap.FileInfo),
		SymbolDefs: make(map[string][]*repomap.Symbol),
	}
	for rel, syms := range files {
		fi := &repomap.FileInfo{RelPath: rel, Symbols: syms}
		g.Files = append(g.Files, fi)
		g.FileIndex[rel] = fi
		for i := range fi.Symbols {
			s := &fi.Symbols[i]
			g.SymbolDefs[s.Name] = append(g.SymbolDefs[s.Name], s)
		}
	}
	return g, root
}

// TestExtractBridgeLiteralChains_GoFixture validates the canonical
// Go register+identity-literal case: RegisterDefaultSubAgents binds
// NewSubExplorer, SubExplorer.Name() returns "explorer".
func TestExtractBridgeLiteralChains_GoFixture(t *testing.T) {
	subagent := `package agent

func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {
	r.Register(NewSubExplorer(deps))
}
`
	subExplorer := `package agent

type SubExplorer struct{ base *BaseAgent }

func NewSubExplorer(deps *Dependencies) *SubExplorer {
	return &SubExplorer{base: nil}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}
`
	files := map[string][]repomap.Symbol{
		"subagent.go": {
			{Name: "RegisterDefaultSubAgents", Kind: "function", File: "subagent.go", Line: 3, EndLine: 5},
		},
		"sub_explorer.go": {
			{Name: "SubExplorer", Kind: "type", File: "sub_explorer.go", Line: 3, EndLine: 3},
			{Name: "NewSubExplorer", Kind: "function", File: "sub_explorer.go", Line: 5, EndLine: 7},
			{Name: "Name", Kind: "method", File: "sub_explorer.go", Line: 9, EndLine: 11, Receiver: "SubExplorer"},
		},
	}
	contents := map[string]string{
		"subagent.go":     subagent,
		"sub_explorer.go": subExplorer,
	}
	graph, root := buildFakeGraph(t, files, contents)
	extraction := extractBridgeLiteralEvidence(graph, root, nil)
	got := extraction.chains
	if len(got) == 0 {
		t.Fatalf("expected at least one bridge chain, got 0")
	}
	foundReal := false
	for _, it := range got {
		if it.Producer != "bridge_literal" {
			t.Errorf("producer=%s, want bridge_literal", it.Producer)
		}
		if it.Kind != types.EvidenceDataflowPath {
			t.Errorf("kind=%s, want dataflow_path", it.Kind)
		}
		if it.Predicate != "resolution_chain" {
			t.Errorf("predicate=%s, want resolution_chain", it.Predicate)
		}
		if strings.Contains(it.Summary, "RegisterDefaultSubAgents") &&
			strings.Contains(it.Summary, "SubExplorer") &&
			strings.Contains(it.Summary, `"explorer"`) {
			if it.OwnerSymbol != "RegisterDefaultSubAgents" ||
				it.AnchorSymbol != "SubExplorer.Name" ||
				it.Object != `"explorer"` ||
				len(it.DerivedFrom) != 1 || strings.TrimSpace(it.DerivedFrom[0]) == "" {
				t.Fatalf("bridge chain must preserve the structured binding/identity join: %+v", it)
			}
			foundReal = true
		}
	}
	if !foundReal {
		t.Fatalf("expected chain containing RegisterDefaultSubAgents/SubExplorer/\"explorer\"; got: %+v", got)
	}
	foundTerminal := false
	var terminalID string
	for _, it := range extraction.terminalReturns {
		if it.Subject == "SubExplorer.Name" &&
			it.Predicate == "returns" &&
			it.Object == `"explorer"` &&
			it.Source == "sub_explorer.go" &&
			it.LineStart == 10 &&
			it.AnchorKind == types.AnchorReturn {
			foundTerminal = true
			terminalID = it.ID
		}
	}
	if !foundTerminal {
		t.Fatalf("expected terminal return companion for SubExplorer.Name at sub_explorer.go:10, got %+v", extraction.terminalReturns)
	}
	for _, it := range got {
		if it.OwnerSymbol == "RegisterDefaultSubAgents" && it.AnchorSymbol == "SubExplorer.Name" &&
			(len(it.DerivedFrom) != 1 || it.DerivedFrom[0] != terminalID) {
			t.Fatalf("bridge chain must derive from the exact terminal companion %q: %+v", terminalID, it)
		}
	}
}

func TestExtractBridgeLiteralChains_ExactResolvedRegistrationOwner(t *testing.T) {
	registry := `package agent

type SubAgentRegistry struct{}

func (r *SubAgentRegistry) Register(sa any) {}

func RegisterDefaultSubAgents(r *SubAgentRegistry) {
	r.Register(NewSubExplorer())
}
`
	explorer := `package agent

type SubExplorer struct{}
func NewSubExplorer() *SubExplorer { return &SubExplorer{} }
func (s *SubExplorer) Name() string { return "explorer" }
`
	files := map[string][]repomap.Symbol{
		"registry.go": {
			{Name: "SubAgentRegistry", Kind: "type", File: "registry.go", Line: 3, EndLine: 3},
			{Name: "Register", Kind: "method", File: "registry.go", Line: 5, EndLine: 5, Receiver: "SubAgentRegistry", Arity: 1},
			{Name: "RegisterDefaultSubAgents", Kind: "function", File: "registry.go", Line: 7, EndLine: 9, Arity: 1},
		},
		"explorer.go": {
			{Name: "SubExplorer", Kind: "type", File: "explorer.go", Line: 3, EndLine: 3},
			{Name: "NewSubExplorer", Kind: "function", File: "explorer.go", Line: 4, EndLine: 4},
			{Name: "Name", Kind: "method", File: "explorer.go", Line: 5, EndLine: 5, Receiver: "SubExplorer"},
		},
	}
	graph, root := buildFakeGraph(t, files, map[string]string{"registry.go": registry, "explorer.go": explorer})
	registerID := repomap.MakeSymbolID(repomap.LangGo, "agent", "SubAgentRegistry", "Register", 1)
	registerSym := &graph.FileIndex["registry.go"].Symbols[1]
	registerSym.ID = registerID
	graph.SymbolByID = map[repomap.SymbolID]*repomap.Symbol{registerID: registerSym}
	graph.FileIndex["registry.go"].Relations = []repomap.Relation{{
		Kind: "call", File: "registry.go", Line: 8,
		ToEP:       repomap.RelationEndpoint{ID: registerID, Name: "Register", Receiver: "r", File: "registry.go", Line: 5},
		Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_ast_selector_call",
	}}
	got := extractBridgeLiteralEvidence(graph, root, nil).chains
	for _, item := range got {
		if item.OwnerSymbol == "RegisterDefaultSubAgents" && item.Object == `"explorer"` {
			if item.DeclaredOwner != "SubAgentRegistry" {
				t.Fatalf("exact resolved registration owner not carried: %+v", item)
			}
			return
		}
	}
	t.Fatalf("registration bridge missing: %+v", got)
}

func TestExtractBridgeLiteralChains_ProductionGoGraphResolvesRegistryOwner(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"registry.go": `package agent

type SubAgentRegistry struct{}
func (r *SubAgentRegistry) Register(sa any) {
}
func RegisterDefaultSubAgents(r *SubAgentRegistry) {
	r.Register(NewSubExplorer())
}
`,
		"explorer.go": `package agent

type SubExplorer struct{}
func NewSubExplorer() *SubExplorer {
	return &SubExplorer{}
}
func (s *SubExplorer) Name() string {
	return "explorer"
}
`,
	}
	for rel, body := range files {
		if err := os.WriteFile(filepath.Join(root, rel), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
	entries, err := repomap.ScanFiles(root)
	if err != nil {
		t.Fatalf("scan fixture: %v", err)
	}
	graph := repomap.BuildGraph(root, repomap.ParseFiles(entries, root))
	for _, item := range extractBridgeLiteralEvidence(graph, root, nil).chains {
		if item.OwnerSymbol == "RegisterDefaultSubAgents" && item.Object == `"explorer"` {
			if item.DeclaredOwner != "SubAgentRegistry" {
				t.Fatalf("production graph did not resolve registry owner: item=%+v relations=%+v symbol_ids=%+v", item, graph.FileIndex["registry.go"].Relations, graph.SymbolByID)
			}
			return
		}
	}
	t.Fatalf("production graph registration bridge missing")
}

func TestExtractBridgeLiteralChains_AmbiguousRegistrationOwnersFailClosed(t *testing.T) {
	registry := `package agent

func RegisterDefaults(a *ARegistry, b *BRegistry) {
	a.Register(NewWorker()); b.Register(NewWorker())
}
`
	worker := `package agent
type Worker struct{}
func NewWorker() *Worker { return &Worker{} }
func (w *Worker) Name() string { return "worker" }
`
	files := map[string][]repomap.Symbol{
		"registry.go": {{Name: "RegisterDefaults", Kind: "function", File: "registry.go", Line: 3, EndLine: 5}},
		"worker.go": {
			{Name: "Worker", Kind: "type", File: "worker.go", Line: 2, EndLine: 2},
			{Name: "NewWorker", Kind: "function", File: "worker.go", Line: 3, EndLine: 3},
			{Name: "Name", Kind: "method", File: "worker.go", Line: 4, EndLine: 4, Receiver: "Worker"},
		},
	}
	graph, root := buildFakeGraph(t, files, map[string]string{"registry.go": registry, "worker.go": worker})
	graph.SymbolByID = map[repomap.SymbolID]*repomap.Symbol{}
	for _, owner := range []string{"ARegistry", "BRegistry"} {
		id := repomap.MakeSymbolID(repomap.LangGo, "agent", owner, "Register", 1)
		sym := &repomap.Symbol{Name: "Register", Kind: "method", Receiver: owner, ID: id, Arity: 1}
		graph.SymbolByID[id] = sym
		graph.FileIndex["registry.go"].Relations = append(graph.FileIndex["registry.go"].Relations, repomap.Relation{
			Kind: "call", File: "registry.go", Line: 4,
			ToEP:       repomap.RelationEndpoint{ID: id, Name: "Register"},
			Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_ast_selector_call",
		})
	}
	for _, item := range extractBridgeLiteralEvidence(graph, root, nil).chains {
		if item.OwnerSymbol == "RegisterDefaults" && item.DeclaredOwner != "" {
			t.Fatalf("ambiguous same-line receivers must not mint one registry endpoint: %+v", item)
		}
	}
}

func TestExactResolvedRegistrationOwnerAtLine_AllSupportedLanguagesShareTypedEndpoint(t *testing.T) {
	for _, language := range repomap.SupportedReadLanguages() {
		t.Run(language, func(t *testing.T) {
			id := repomap.MakeSymbolID(language, "pkg", "Registry", "Register", 1)
			target := &repomap.Symbol{Name: "Register", Kind: "method", Receiver: "Registry", ID: id, Arity: 1}
			provenance := repomap.ProvenanceTreeSitter
			if language == repomap.LangCangjie {
				provenance = repomap.ProvenanceCangjieParser
			}
			file := &repomap.FileInfo{RelPath: "registry.src", Language: language, Relations: []repomap.Relation{{
				Kind: "call", File: "registry.src", Line: 7,
				ToEP:       repomap.RelationEndpoint{ID: id, Name: "Register"},
				Confidence: 1, Provenance: provenance, ResolvedBy: "typed_call",
			}}}
			graph := &repomap.Graph{SymbolByID: map[repomap.SymbolID]*repomap.Symbol{id: target}}
			got := exactResolvedRegistrationOwnerAtLine(graph, file, 7, func(name string) bool {
				return strings.EqualFold(name, "register")
			})
			if got != "Registry" {
				t.Fatalf("language %s lost shared typed registration endpoint: %q", language, got)
			}
		})
	}
}

func TestExactResolvedRegistrationOwnerAtLine_RegexFallbackCannotMintEndpoint(t *testing.T) {
	id := repomap.MakeSymbolID(repomap.LangGo, "pkg", "Registry", "Register", 1)
	target := &repomap.Symbol{Name: "Register", Kind: "method", Receiver: "Registry", ID: id, Arity: 1}
	file := &repomap.FileInfo{RelPath: "registry.go", Language: repomap.LangGo, Relations: []repomap.Relation{{
		Kind: "call", File: "registry.go", Line: 7,
		ToEP:       repomap.RelationEndpoint{ID: id, Name: "Register"},
		Confidence: 0.6, Provenance: "regex_fallback", ResolvedBy: "regex_call",
	}}}
	graph := &repomap.Graph{SymbolByID: map[repomap.SymbolID]*repomap.Symbol{id: target}}
	if got := exactResolvedRegistrationOwnerAtLine(graph, file, 7, func(string) bool { return true }); got != "" {
		t.Fatalf("regex fallback must not mint a hard registration endpoint: %q", got)
	}
}

func TestExactResolvedRegistrationOwnerAtLine_CrossPackageSameOwnerIsAmbiguous(t *testing.T) {
	left := &repomap.Symbol{Name: "Register", Kind: "method", Receiver: "Registry", File: "left/registry.go", Arity: 1}
	right := &repomap.Symbol{Name: "Register", Kind: "method", Receiver: "Registry", File: "right/registry.go", Arity: 1}
	file := &repomap.FileInfo{RelPath: "caller.go", Package: "caller", Language: repomap.LangGo, Relations: []repomap.Relation{{
		Kind: "call", File: "caller.go", Line: 9,
		ToEP:       repomap.RelationEndpoint{Name: "Register", Receiver: "Registry"},
		Confidence: 1, Provenance: repomap.ProvenanceTreeSitter, ResolvedBy: "go_ast_selector_call",
	}}}
	graph := &repomap.Graph{
		SymbolDefs: map[string][]*repomap.Symbol{"Register": {left, right}},
		MethodIndex: map[repomap.MethodKey]*repomap.Symbol{
			{Pkg: "left", Receiver: "Registry", Name: "Register"}:  left,
			{Pkg: "right", Receiver: "Registry", Name: "Register"}: right,
		},
	}
	if got := exactResolvedRegistrationOwnerAtLine(graph, file, 9, func(string) bool { return true }); got != "" {
		t.Fatalf("cross-package same-owner ambiguity must fail closed: %q", got)
	}
}

func TestExplorerParseOutputRefreshesAcceptedRelationAuthorityAfterBridgeEvidence(t *testing.T) {
	subagent := `package agent

func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {
	r.Register(NewSubExplorer(deps))
}
`
	subExplorer := `package agent

type SubExplorer struct{ base *BaseAgent }

func NewSubExplorer(deps *Dependencies) *SubExplorer {
	return &SubExplorer{base: nil}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}
`
	files := map[string][]repomap.Symbol{
		"subagent.go": {
			{Name: "RegisterDefaultSubAgents", Kind: "function", File: "subagent.go", Line: 3, EndLine: 5},
		},
		"sub_explorer.go": {
			{Name: "SubExplorer", Kind: "type", File: "sub_explorer.go", Line: 3, EndLine: 3},
			{Name: "NewSubExplorer", Kind: "function", File: "sub_explorer.go", Line: 5, EndLine: 7},
			{Name: "Name", Kind: "method", File: "sub_explorer.go", Line: 9, EndLine: 11, Receiver: "SubExplorer"},
		},
	}
	graph, root := buildFakeGraph(t, files, map[string]string{
		"subagent.go": subagent, "sub_explorer.go": subExplorer,
	})
	registerID := repomap.MakeSymbolID(repomap.LangGo, "agent", "SubAgentRegistry", "Register", 1)
	registerSym := &repomap.Symbol{Name: "Register", Kind: "method", Receiver: "SubAgentRegistry", ID: registerID, Arity: 1}
	graph.SymbolByID = map[repomap.SymbolID]*repomap.Symbol{registerID: registerSym}
	graph.FileIndex["subagent.go"].Relations = []repomap.Relation{{
		Kind: "call", File: "subagent.go", Line: 4,
		ToEP:       repomap.RelationEndpoint{ID: registerID, Name: "Register", Receiver: "r"},
		Confidence: 1, Provenance: "tree_sitter", ResolvedBy: "go_ast_selector_call",
	}}
	rm := types.RequestModel{
		Intent:        types.IntentEnumerate,
		PredicateAxis: types.AxisRegister,
		Predicates: types.SemanticPredicates{
			IsCategoryEnumeration: true,
			// Production r519 emitted the exact register axis while leaving
			// the redundant generic relation boolean false. Exact typed relation
			// authority must not disappear merely because those two analyzer
			// carriers diverge.
			IsRelationalLookup: false,
		},
		AnalyzerHints: types.AnalyzerHints{
			Kind:              string(types.ReqEnumeration),
			ExactTargets:      []string{"SubAgentRegistry"},
			MentionedEntities: []string{"SubAgentRegistry", "Name", "Names"},
			PrimaryEntities:   []string{"SubAgentRegistry", "Name", "Names"},
		},
		CompletenessObligation: &types.CompletenessObligation{Required: true},
	}
	mut := types.NewMutableState("default registered subagent names")
	mut.SetInvestigationAggregateFacts([]types.AnswerAggregateFact{{
		Kind: types.AnswerAggregateMemberSet, Label: "default registered subagent names", Value: "1",
		Members: []string{"explorer"}, SupportRefs: []string{"explorer: sub_explorer.go:10"},
	}})
	mut.SetInvestigationComplete("model accepted the exact registry member set")
	mut.RetainInvestigationAggregateFacts()
	ctx := &types.AgentContext{
		Objective: "list the default registered subagent names",
		RepoRoot:  root,
		Mutable:   mut,
		AnalysisIR: &types.AnalysisIR{
			RequestModel: rm,
		},
		SearchGraph: graph,
	}
	eval := &explorerEvaluator{
		userQuestion: "list the default registered subagent names",
		analysisIR:   ctx.AnalysisIR,
		mutable:      mut,
		searchResult: &keywordSearchResult{Graph: graph},
		requiredFiles: []string{
			"subagent.go", "sub_explorer.go",
		},
		structuredEvidence: []types.EvidenceItem{
			pinnedEvidenceItem("subagent.go", "RegisterDefaultSubAgents", 3),
			pinnedEvidenceItem("sub_explorer.go", "SubExplorer.Name", 9),
		},
		investigationNotes: []string{
			"- [REGISTRATION] RegisterDefaultSubAgents line 3: binds NewSubExplorer",
			"- [DIRECT] SubExplorer.Name line 9: returns explorer",
		},
		isEnumerationQuery: true,
	}
	toolResults := []types.ToolResult{
		{ToolName: "grep", Success: true, Summary: "subagent.go:3\nsub_explorer.go:9"},
		readFileCoverageResult("subagent.go", 1, 5, 5),
		readFileCoverageResult("sub_explorer.go", 1, 11, 11),
	}
	out, err := eval.ParseOutput(ctx, nil, toolResults, nil)
	if err != nil {
		t.Fatalf("ParseOutput error: %v", err)
	}
	got := mut.StableInvestigationAggregateFacts()
	if len(got) != 1 || !types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(got[0]) {
		t.Fatalf("ParseOutput did not refresh the accepted aggregate authority after deterministic evidence: facts=%+v evidence=%+v", got, out.EvidenceItems)
	}
	handoff := mut.TurnAArtifacts()
	if handoff == nil || len(handoff.AcceptedAggregateFacts) != 1 ||
		!types.AnswerAggregateFactHasTypedRelationPrincipalAuthority(handoff.AcceptedAggregateFacts[0]) {
		t.Fatalf("Turn-A handoff did not receive refreshed authority: %+v", handoff)
	}
	foundBridge := false
	for _, item := range out.EvidenceItems {
		if item.Producer == "bridge_literal" && item.OwnerSymbol == "RegisterDefaultSubAgents" &&
			item.DeclaredOwner == "SubAgentRegistry" && item.Object == `"explorer"` {
			foundBridge = true
			break
		}
	}
	if !foundBridge {
		t.Fatalf("ParseOutput did not preserve the exact bridge evidence: %+v", out.EvidenceItems)
	}
}

// TestExtractBridgeLiteralChains_IgnoresComments proves Step 1's
// comment-filter composes with Step 2: a comment-text NewSubExplorer
// reference inside a register-named function body must NOT emit a
// chain.
func TestExtractBridgeLiteralChains_IgnoresComments(t *testing.T) {
	// registerSomething has a REAL binding (NewHandler) plus a
	// phantom NewPhantomThing mentioned only inside a comment.
	reg := `package agent

func RegisterHandlers(r *R) {
	// legacy: r.Register(NewPhantomThing(deps))
	r.Register(NewHandler(deps))
}
`
	handler := `package agent

type Handler struct{}

func NewHandler(deps *Dependencies) *Handler { return &Handler{} }

func (h *Handler) Name() string { return "real" }
`
	// A stray type that only exists in comment text — if the filter
	// leaks, we'd see a phantom chain for it.
	phantom := `package agent

type PhantomThing struct{}

func (p *PhantomThing) Name() string { return "phantom" }
`
	files := map[string][]repomap.Symbol{
		"reg.go": {
			{Name: "RegisterHandlers", Kind: "function", File: "reg.go", Line: 3, EndLine: 6},
		},
		"handler.go": {
			{Name: "Handler", Kind: "type", File: "handler.go", Line: 3, EndLine: 3},
			{Name: "NewHandler", Kind: "function", File: "handler.go", Line: 5, EndLine: 5},
			{Name: "Name", Kind: "method", File: "handler.go", Line: 7, EndLine: 7, Receiver: "Handler"},
		},
		"phantom.go": {
			{Name: "PhantomThing", Kind: "type", File: "phantom.go", Line: 3, EndLine: 3},
			{Name: "Name", Kind: "method", File: "phantom.go", Line: 5, EndLine: 5, Receiver: "PhantomThing"},
		},
	}
	contents := map[string]string{
		"reg.go":     reg,
		"handler.go": handler,
		"phantom.go": phantom,
	}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	foundReal := false
	for _, it := range got {
		if strings.Contains(it.Summary, "PhantomThing") {
			t.Fatalf("phantom chain leaked through comment filter: %s", it.Summary)
		}
		if strings.Contains(it.Summary, "Handler") && strings.Contains(it.Summary, `"real"`) {
			foundReal = true
		}
	}
	if !foundReal {
		t.Fatalf("expected real Handler chain; got: %+v", got)
	}
}

// TestExtractBridgeLiteralChains_NoBait verifies rule 4 of the
// over-fit audit: a register call with no identity method on the
// target class must NOT emit a bridge chain.
func TestExtractBridgeLiteralChains_NoBait(t *testing.T) {
	reg := `package agent

func RegisterOpaque(r *R) {
	r.Register(NewOpaque())
}
`
	opaque := `package agent

type Opaque struct{}

func NewOpaque() *Opaque { return &Opaque{} }

func (o *Opaque) Do() {}
`
	files := map[string][]repomap.Symbol{
		"reg.go": {{Name: "RegisterOpaque", Kind: "function", File: "reg.go", Line: 3, EndLine: 5}},
		"opaque.go": {
			{Name: "Opaque", Kind: "type", File: "opaque.go", Line: 3, EndLine: 3},
			{Name: "NewOpaque", Kind: "function", File: "opaque.go", Line: 5, EndLine: 5},
			{Name: "Do", Kind: "method", File: "opaque.go", Line: 7, EndLine: 7, Receiver: "Opaque"},
		},
	}
	contents := map[string]string{"reg.go": reg, "opaque.go": opaque}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	if len(got) != 0 {
		t.Fatalf("expected no bridge chain (no identity method on Opaque); got: %+v", got)
	}
}

// TestExtractBridgeLiteralChains_NoBinding verifies the identity-only
// half does not fire alone — a class with a Name() literal but no
// register caller must NOT emit a chain.
func TestExtractBridgeLiteralChains_NoBinding(t *testing.T) {
	lone := `package agent

type Lone struct{}

func (l *Lone) Name() string { return "lonely" }
`
	files := map[string][]repomap.Symbol{
		"lone.go": {
			{Name: "Lone", Kind: "type", File: "lone.go", Line: 3, EndLine: 3},
			{Name: "Name", Kind: "method", File: "lone.go", Line: 5, EndLine: 5, Receiver: "Lone"},
		},
	}
	contents := map[string]string{"lone.go": lone}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	if len(got) != 0 {
		t.Fatalf("expected no bridge chain (no register caller); got: %+v", got)
	}
}

// TestExtractBridgeLiteralChains_MultipleTargets validates aggregated
// bindings split into separate chains. Also exercises the Slug/Key
// method-name variants.
func TestExtractBridgeLiteralChains_MultipleTargets(t *testing.T) {
	reg := `package agent

func RegisterAll(r *R) {
	r.Register(NewAlpha())
	r.Register(NewBeta())
}
`
	alpha := `package agent

type Alpha struct{}

func NewAlpha() *Alpha { return &Alpha{} }

func (a *Alpha) Slug() string { return "alpha-slug" }
`
	beta := `package agent

type Beta struct{}

func NewBeta() *Beta { return &Beta{} }

func (b *Beta) Key() string { return "beta-key" }
`
	files := map[string][]repomap.Symbol{
		"reg.go": {{Name: "RegisterAll", Kind: "function", File: "reg.go", Line: 3, EndLine: 6}},
		"alpha.go": {
			{Name: "Alpha", Kind: "type", File: "alpha.go", Line: 3, EndLine: 3},
			{Name: "NewAlpha", Kind: "function", File: "alpha.go", Line: 5, EndLine: 5},
			{Name: "Slug", Kind: "method", File: "alpha.go", Line: 7, EndLine: 7, Receiver: "Alpha"},
		},
		"beta.go": {
			{Name: "Beta", Kind: "type", File: "beta.go", Line: 3, EndLine: 3},
			{Name: "NewBeta", Kind: "function", File: "beta.go", Line: 5, EndLine: 5},
			{Name: "Key", Kind: "method", File: "beta.go", Line: 7, EndLine: 7, Receiver: "Beta"},
		},
	}
	contents := map[string]string{"reg.go": reg, "alpha.go": alpha, "beta.go": beta}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	if len(got) < 2 {
		t.Fatalf("expected ≥2 chains (alpha, beta); got %d: %+v", len(got), got)
	}
	foundAlpha, foundBeta := false, false
	for _, it := range got {
		if strings.Contains(it.Summary, "Alpha") && strings.Contains(it.Summary, `"alpha-slug"`) {
			foundAlpha = true
		}
		if strings.Contains(it.Summary, "Beta") && strings.Contains(it.Summary, `"beta-key"`) {
			foundBeta = true
		}
	}
	if !foundAlpha || !foundBeta {
		t.Fatalf("missing chains (alpha=%v beta=%v): %+v", foundAlpha, foundBeta, got)
	}
}

// TestExtractBridgeLiteralChains_PythonFixture exercises Python
// idioms: classes use sym.Parent (not Receiver), class instantiation
// is bare `Xxx()` instead of `NewXxx()`, method names are
// lowercase/snake_case, and binding calls often use `.register(...)`
// or `.add_handler(...)`.
func TestExtractBridgeLiteralChains_PythonFixture(t *testing.T) {
	registry := `
def setup(app):
    app.register(UserHandler())
`
	handler := `
class UserHandler:
    def name(self):
        return "user"
`
	files := map[string][]repomap.Symbol{
		"registry.py": {
			{Name: "setup", Kind: "function", File: "registry.py", Line: 2, EndLine: 3},
		},
		"handler.py": {
			{Name: "UserHandler", Kind: "class", File: "handler.py", Line: 2, EndLine: 5},
			{Name: "name", Kind: "method", File: "handler.py", Line: 3, EndLine: 4, Parent: "UserHandler"},
		},
	}
	contents := map[string]string{
		"registry.py": registry,
		"handler.py":  handler,
	}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	found := false
	for _, it := range got {
		if strings.Contains(it.Summary, "UserHandler") && strings.Contains(it.Summary, `"user"`) &&
			strings.Contains(it.Summary, "setup") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected UserHandler chain from Python fixture; got: %+v", got)
	}
}

// TestExtractBridgeLiteralChains_JavaFixture exercises Java idioms:
// `new Xxx()` constructor, `Parent`-based class membership, getName()
// identity method with `public String getName()` signature.
func TestExtractBridgeLiteralChains_JavaFixture(t *testing.T) {
	config := `
public class Config {
    void registerBeans(Registry r) {
        r.bind(new UserController());
    }
}
`
	controller := `
public class UserController {
    public String getName() {
        return "users";
    }
}
`
	files := map[string][]repomap.Symbol{
		"Config.java": {
			{Name: "Config", Kind: "class", File: "Config.java", Line: 2, EndLine: 6},
			{Name: "registerBeans", Kind: "method", File: "Config.java", Line: 3, EndLine: 5, Parent: "Config"},
		},
		"UserController.java": {
			{Name: "UserController", Kind: "class", File: "UserController.java", Line: 2, EndLine: 6},
			{Name: "getName", Kind: "method", File: "UserController.java", Line: 3, EndLine: 5, Parent: "UserController"},
		},
	}
	contents := map[string]string{"Config.java": config, "UserController.java": controller}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	found := false
	for _, it := range got {
		if strings.Contains(it.Summary, "UserController") && strings.Contains(it.Summary, `"users"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected UserController chain from Java fixture; got: %+v", got)
	}
}

// TestExtractBridgeLiteralChains_RustFixture exercises Rust idioms:
// `fn name(&self) -> &str { "x" }` identity method with Parent set
// from impl block, Registry.register(Box::new(Handler::new()))-style
// binding. Rust's `Box::new(Xxx::new())` won't match our constructor
// detection directly, so we test a simpler shape that the scanner can
// handle: `register(Handler::new())`. The parser recognizes
// capitalized bare class calls.
func TestExtractBridgeLiteralChains_RustFixture(t *testing.T) {
	registry := `
pub fn register_all(r: &mut Registry) {
    r.register(Handler::new());
}
`
	handler := `
pub struct Handler;

impl Handler {
    pub fn name(&self) -> &str {
        "rust-handler"
    }
}
`
	files := map[string][]repomap.Symbol{
		"registry.rs": {
			{Name: "register_all", Kind: "function", File: "registry.rs", Line: 2, EndLine: 4},
		},
		"handler.rs": {
			{Name: "Handler", Kind: "type", File: "handler.rs", Line: 2, EndLine: 2},
			{Name: "name", Kind: "method", File: "handler.rs", Line: 5, EndLine: 7, Parent: "Handler"},
		},
	}
	contents := map[string]string{"registry.rs": registry, "handler.rs": handler}
	graph, root := buildFakeGraph(t, files, contents)
	got := extractBridgeLiteralChains(graph, root, nil)
	found := false
	for _, it := range got {
		if strings.Contains(it.Summary, "Handler") && strings.Contains(it.Summary, `"rust-handler"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected Handler chain from Rust fixture; got: %+v", got)
	}
}

// TestParseTargetClassFromBinding verifies the token parser across
// common shapes.
func TestParseTargetClassFromBinding(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"NewSubExplorer(deps)", "SubExplorer"},
		{" NewHandler() ", "Handler"},
		{"new Handler()", "Handler"},
		{"&Config{}", "Config"},
		{"UserHandler", "UserHandler"},
		{"pkg.NewThing(d)", "Thing"},
		{"pkg.UserHandler", "UserHandler"},
		{"nil", ""},
		{"", ""},
		{"lowercase", ""},
		{"(NewFoo(d))", "Foo"},
		{"Handler::new()", "Handler"}, // Rust path
		{"a.b.UserHandler", "UserHandler"},
		{"Handler<T>", "Handler"}, // generic
	}
	for _, c := range cases {
		got := parseTargetClassFromBinding(c.in)
		if got != c.want {
			t.Errorf("parseTargetClassFromBinding(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestExtractBridgeLiteralChains_ConsumerGateJoin validates Pass D:
// a consumer concrete_value whose Object shows <Field>.Get(<key>) is
// joined with the producer binding+identity chain when the field
// name shares a stem with the binding function name. This is the
// join that answers questions like "how many agents can call
// subagent" — the consumer (buildToolSchemas gate) plus the
// registry population chain (RegisterDefaultSubAgents binds
// NewSubExplorer, Name()="explorer") together imply "only the agent
// named 'explorer' passes the gate → 1 agent".
func TestExtractBridgeLiteralChains_ConsumerGateJoin(t *testing.T) {
	// Producer-side: RegisterDefaultSubAgents populates a registry.
	subagent := `package agent

func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {
	r.Register(NewSubExplorer(deps))
}
`
	subExplorer := `package agent

type SubExplorer struct{ base *BaseAgent }

func NewSubExplorer(deps *Dependencies) *SubExplorer {
	return &SubExplorer{base: nil}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}
`
	files := map[string][]repomap.Symbol{
		"subagent.go": {
			{Name: "RegisterDefaultSubAgents", Kind: "function", File: "subagent.go", Line: 3, EndLine: 5},
		},
		"sub_explorer.go": {
			{Name: "SubExplorer", Kind: "type", File: "sub_explorer.go", Line: 3, EndLine: 3},
			{Name: "NewSubExplorer", Kind: "function", File: "sub_explorer.go", Line: 5, EndLine: 7},
			{Name: "Name", Kind: "method", File: "sub_explorer.go", Line: 9, EndLine: 11, Receiver: "SubExplorer"},
		},
	}
	contents := map[string]string{
		"subagent.go":     subagent,
		"sub_explorer.go": subExplorer,
	}
	graph, root := buildFakeGraph(t, files, contents)

	// Consumer-side: BaseAgent.buildToolSchemas has an assignment
	// concrete_value that gates on SubAgents.Get(name).
	consumerValues := []concreteValue{
		{
			file:   "internal/agent/agent.go",
			method: "BaseAgent.buildToolSchemas",
			kind:   "assigns",
			value:  `err := b.deps.SubAgents.Get(string(b.name)); err == nil {`,
			line:   901,
		},
	}

	got := extractBridgeLiteralChains(graph, root, consumerValues)

	// Pass C should still produce the bridge chain.
	foundBridge, foundConsumerGate := false, false
	for _, it := range got {
		switch it.Producer {
		case "bridge_literal":
			if strings.Contains(it.Summary, "RegisterDefaultSubAgents") &&
				strings.Contains(it.Summary, `"explorer"`) {
				foundBridge = true
			}
		case "consumer_gate":
			if !strings.Contains(it.Summary, "BaseAgent.buildToolSchemas") {
				t.Errorf("consumer_gate chain missing consumer subject: %s", it.Summary)
			}
			if !strings.Contains(it.Summary, "gates on SubAgents.Get") {
				t.Errorf("consumer_gate chain missing gate clause: %s", it.Summary)
			}
			if !strings.Contains(it.Summary, "RegisterDefaultSubAgents") {
				t.Errorf("consumer_gate chain missing producer name: %s", it.Summary)
			}
			if !strings.Contains(it.Summary, `"explorer"`) {
				t.Errorf("consumer_gate chain missing identity literal: %s", it.Summary)
			}
			if it.Source != "internal/agent/agent.go" || it.LineStart != 901 {
				t.Errorf("consumer_gate chain source/line wrong: %s:%d", it.Source, it.LineStart)
			}
			foundConsumerGate = true
		}
	}
	if !foundBridge {
		t.Errorf("Pass C bridge_literal chain missing; got: %+v", got)
	}
	if !foundConsumerGate {
		t.Fatalf("Pass D consumer_gate chain missing; got: %+v", got)
	}
}

func TestRelationConsumerGateValues_ScansSameDirectoryGateForRegistration(t *testing.T) {
	subagent := `package agent

func RegisterDefaultSubAgents(r *SubAgentRegistry, deps *Dependencies) {
	r.Register(NewSubExplorer(deps))
}
`
	subExplorer := `package agent

type SubExplorer struct{ base *BaseAgent }

func NewSubExplorer(deps *Dependencies) *SubExplorer {
	return &SubExplorer{base: nil}
}

func (s *SubExplorer) Name() string {
	return "explorer"
}
`
	agentFile := `package agent

type BaseAgent struct{ deps *Dependencies; name string }
type Dependencies struct{ SubAgents *SubAgentRegistry }

func (b *BaseAgent) buildToolSchemas() {
	if _, err := b.deps.SubAgents.Get(string(b.name)); err == nil {
		_ = err
	}
}
`
	files := map[string][]repomap.Symbol{
		"subagent.go": {
			{Name: "RegisterDefaultSubAgents", Kind: "function", File: "subagent.go", Line: 3, EndLine: 5},
		},
		"sub_explorer.go": {
			{Name: "SubExplorer", Kind: "type", File: "sub_explorer.go", Line: 3, EndLine: 3},
			{Name: "NewSubExplorer", Kind: "function", File: "sub_explorer.go", Line: 5, EndLine: 7},
			{Name: "Name", Kind: "method", File: "sub_explorer.go", Line: 9, EndLine: 11, Receiver: "SubExplorer"},
		},
		"agent.go": {
			{Name: "BaseAgent", Kind: "type", File: "agent.go", Line: 3, EndLine: 3},
			{Name: "buildToolSchemas", Kind: "method", File: "agent.go", Line: 6, EndLine: 10, Receiver: "BaseAgent"},
		},
	}
	contents := map[string]string{
		"subagent.go":     subagent,
		"sub_explorer.go": subExplorer,
		"agent.go":        agentFile,
	}
	graph, root := buildFakeGraph(t, files, contents)
	e := &explorerEvaluator{
		analysisIR: &types.AnalysisIR{RequestModel: types.RequestModel{
			Intent:        types.IntentTrace,
			PredicateAxis: types.AxisRegister,
			AnalyzerHints: types.AnalyzerHints{Kind: string(types.ReqRegistration)},
		}},
		requiredFiles: []string{"subagent.go"},
	}

	values := e.relationConsumerGateValues(context.Background(), graph, root, map[string]bool{"subagent.go": true})
	foundGate := false
	for _, v := range values {
		if v.file == "agent.go" &&
			v.method == "BaseAgent.buildToolSchemas" &&
			strings.Contains(v.value, "SubAgents.Get") {
			foundGate = true
		}
	}
	if !foundGate {
		t.Fatalf("expected relation consumer gate from agent.go, got %+v", values)
	}

	joined := extractBridgeLiteralEvidence(graph, root, values)
	foundConsumerGate := false
	for _, it := range joined.chains {
		if it.Producer == "consumer_gate" &&
			it.Source == "agent.go" &&
			strings.Contains(it.Summary, "BaseAgent.buildToolSchemas") &&
			strings.Contains(it.Summary, `"explorer"`) {
			foundConsumerGate = true
		}
	}
	if !foundConsumerGate {
		t.Fatalf("expected joined consumer_gate evidence, got %+v", joined.chains)
	}
}

// TestExtractBridgeLiteralChains_ConsumerGate_StemGate guards against
// over-eager joining when the consumer's field stem is too generic
// or its first letter is lowercase. Also verifies that non-assign
// concrete values are ignored even if their text contains .Get(.
func TestExtractBridgeLiteralChains_ConsumerGate_StemGate(t *testing.T) {
	// A realistic bridge: RegisterPlugins binds NewAnalyzer,
	// Analyzer.Name()="analyzer".
	reg := `package agent

func RegisterPlugins(r *PluginRegistry, deps *Dependencies) {
	r.Register(NewAnalyzer(deps))
}
`
	analyzer := `package agent

type Analyzer struct{}

func NewAnalyzer(deps *Dependencies) *Analyzer { return &Analyzer{} }

func (a *Analyzer) Name() string { return "analyzer" }
`
	files := map[string][]repomap.Symbol{
		"reg.go": {
			{Name: "RegisterPlugins", Kind: "function", File: "reg.go", Line: 3, EndLine: 5},
		},
		"analyzer.go": {
			{Name: "Analyzer", Kind: "type", File: "analyzer.go", Line: 3, EndLine: 3},
			{Name: "NewAnalyzer", Kind: "function", File: "analyzer.go", Line: 5, EndLine: 5},
			{Name: "Name", Kind: "method", File: "analyzer.go", Line: 7, EndLine: 7, Receiver: "Analyzer"},
		},
	}
	contents := map[string]string{"reg.go": reg, "analyzer.go": analyzer}
	graph, root := buildFakeGraph(t, files, contents)

	consumerValues := []concreteValue{
		// Too-short stem after singularization: "Tool" → len=4, rejected.
		{file: "x.go", method: "X.A", kind: "assigns",
			value: `err := b.Tools.Get("propose_sub_agents"); err == nil {`, line: 10},
		// Lowercase field (`deps.plugins`) — not a capitalised identifier
		// in the position expected for registry-typed fields.
		{file: "y.go", method: "Y.B", kind: "assigns",
			value: `err := deps.plugins.Get("x"); err == nil {`, line: 11},
		// Non-assign kind — even if text contains .Get(, ignored.
		{file: "z.go", method: "Z.C", kind: "returns",
			value: `b.Plugins.Get("x")`, line: 12},
		// Valid case: Plugins stem "Plugin" is ≥5 chars, substring of
		// "RegisterPlugins" → join succeeds.
		{file: "valid.go", method: "Valid.D", kind: "assigns",
			value: `err := b.deps.Plugins.Get(id); err == nil {`, line: 42},
	}

	got := extractBridgeLiteralChains(graph, root, consumerValues)

	consumerGateCount := 0
	validFound := false
	for _, it := range got {
		if it.Producer != "consumer_gate" {
			continue
		}
		consumerGateCount++
		if it.Source == "valid.go" && it.LineStart == 42 {
			validFound = true
		}
	}
	if !validFound {
		t.Errorf("expected valid-case consumer_gate chain to fire; got: %+v", got)
	}
	if consumerGateCount != 1 {
		t.Errorf("want exactly 1 consumer_gate (the valid case), got %d", consumerGateCount)
	}
}

// TestParseConsumerGateField exercises the parser across shapes.
func TestParseConsumerGateField(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"canonical deps chain", `err := b.deps.SubAgents.Get(string(b.name)); err == nil {`, "SubAgents"},
		{"simple field", `err := registry.Plugins.Get(id); err == nil {`, "Plugins"},
		{"two-arg Get", `ok := store.Handlers.Get(key, ver); ok {`, "Handlers"},
		{"lowercase — reject (not registry-style)", `x := list.get(i)`, ""},
		{"no Get pattern", `y := foo.Bar()`, ""},
		{"Get without dot prefix", `val := Get("x")`, ""},
		{"identifier touching Get", `err := SubAgents.Get(name)`, "SubAgents"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := parseConsumerGateField(c.in)
			if got != c.want {
				t.Errorf("parseConsumerGateField(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// TestSingularize covers the plural-stripping heuristic.
func TestSingularize(t *testing.T) {
	cases := []struct{ in, want string }{
		{"SubAgents", "SubAgent"},
		{"Handlers", "Handler"},
		{"Agent", "Agent"},          // already singular
		{"s", "s"},                  // too short to strip
		{"", ""},                    // empty
		{"Kubernetes", "Kubernete"}, // Over-applies — known limitation
	}
	for _, c := range cases {
		if got := singularize(c.in); got != c.want {
			t.Errorf("singularize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
