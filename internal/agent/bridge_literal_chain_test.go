package agent

import (
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
	got := extractBridgeLiteralChains(graph, root, nil)
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
			foundReal = true
		}
	}
	if !foundReal {
		t.Fatalf("expected chain containing RegisterDefaultSubAgents/SubExplorer/\"explorer\"; got: %+v", got)
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
		"reg.go":    {{Name: "RegisterOpaque", Kind: "function", File: "reg.go", Line: 3, EndLine: 5}},
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
		"reg.go":   {{Name: "RegisterAll", Kind: "function", File: "reg.go", Line: 3, EndLine: 6}},
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
		{"Agent", "Agent"},   // already singular
		{"s", "s"},           // too short to strip
		{"", ""},             // empty
		{"Kubernetes", "Kubernete"}, // Over-applies — known limitation
	}
	for _, c := range cases {
		if got := singularize(c.in); got != c.want {
			t.Errorf("singularize(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
