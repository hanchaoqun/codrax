package agent

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
	repotypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// concrete_values_lang_test.go — cross-language tests for the 4 new
// concrete-value kinds (P0.1–P0.4). Each kind has sub-tests per
// language so a failure immediately tells you WHICH language broke.

// ── P0.1: conditional ──────────────────────────────────────────────

func TestScanConditionalPatterns(t *testing.T) {
	cases := []struct {
		name string
		lang string
		src  string
		want string // substring expected in the first result's value
	}{
		{"Go/if", repotypes.LangGo,
			`if len(proposals) == 0 {`, "len(proposals) == 0"},
		{"Go/switch", repotypes.LangGo,
			`switch req.Kind {`, "switch req.Kind"},
		{"Go/if-let-skip-trivial", repotypes.LangGo,
			`if err != nil {`, ""}, // trivial → no result
		{"Python/if", repotypes.LangPython,
			`if self.phase == 0:`, "self.phase == 0"},
		{"Python/elif", repotypes.LangPython,
			`elif count > threshold:`, "count > threshold"},
		{"Python/match", repotypes.LangPython,
			`match command:`, "match command"},
		{"JS/if-parens", repotypes.LangJavaScript,
			`if (user.isAdmin()) {`, "user.isAdmin()"},
		{"TS/switch", repotypes.LangTypeScript,
			`switch (action.type) {`, "switch action.type"},
		{"Kotlin/when", repotypes.LangKotlin,
			`when (state) {`, "when state"},
		{"Java/if", repotypes.LangJava,
			`if (request.getMethod().equals("POST")) {`, `request.getMethod().equals("POST")`},
		{"Swift/guard", repotypes.LangSwift,
			`guard route != nil else {`, "guard route != nil"},
		{"Ruby/unless", repotypes.LangRuby,
			`unless cache_hit then`, "unless cache_hit"},
		{"Lua/if-then", repotypes.LangLua,
			`if route.enabled then`, "route.enabled"},
		{"Rust/if", repotypes.LangRust,
			`if self.budget > 0 {`, "self.budget > 0"},
		{"Rust/match", repotypes.LangRust,
			`match self.state {`, "match self.state"},
		{"Rust/if-let", repotypes.LangRust,
			`if let Some(val) = map.get(key) {`, "let Some(val) = map.get(key)"},
		{"C/if", repotypes.LangC,
			`if (ptr != NULL) {`, "ptr != NULL"},
		{"Cpp/if", repotypes.LangCpp,
			`if (auto it = map.find(key); it != map.end()) {`, "auto it = map.find(key); it != map.end()"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := splitTestLines(tc.src)
			got := scanConditionalPatterns(lines, tc.lang)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("expected no results (trivial), got %v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("expected result containing %q, got nothing", tc.want)
			}
			if !containsSubstring(got[0].value, tc.want) {
				t.Errorf("value = %q, want substring %q", got[0].value, tc.want)
			}
			if got[0].kind != "conditional" {
				t.Errorf("kind = %q, want conditional", got[0].kind)
			}
		})
	}
}

// ── P0.2: embeds ───────────────────────────────────────────────────

func TestScanEmbedsPatterns(t *testing.T) {
	cases := []struct {
		name  string
		lang  string
		src   string
		want  string
		count int
	}{
		{"Go/struct-embed", repotypes.LangGo,
			"ReadOnly\n*Foo\npkg.Bar",
			"embeds ReadOnly", 3},
		{"Go/skip-named-field", repotypes.LangGo,
			"name string\nage int",
			"", 0},
		{"Python/single-base", repotypes.LangPython,
			`class Explorer(BaseAgent):`,
			"inherits BaseAgent", 1},
		{"Python/multi-base", repotypes.LangPython,
			`class Foo(Base1, Base2, object):`,
			"inherits Base1", 2}, // object filtered out
		{"JS/extends", repotypes.LangJavaScript,
			`class UserController extends BaseController {`,
			"extends BaseController", 1},
		{"TS/extends", repotypes.LangTypeScript,
			`class Handler extends EventEmitter {`,
			"extends EventEmitter", 1},
		{"TS/interface-extends", repotypes.LangTypeScript,
			`interface Serializable extends Readable, Writable {`,
			"extends Readable", 2},
		{"Java/extends", repotypes.LangJava,
			`public class UserService extends AbstractService {`,
			"extends AbstractService", 1},
		{"Ruby/class-inherits", repotypes.LangRuby,
			`class Greeter < BaseGreeter`,
			"inherits BaseGreeter", 1},
		{"Swift/class-inherits", repotypes.LangSwift,
			`final class Greeter: NSObject, Greetable {`,
			"inherits NSObject", 1},
		{"Swift/protocol-inherits", repotypes.LangSwift,
			`protocol Child: ParentA, ParentB {`,
			"inherits ParentA", 2},
		{"Lua/metatable-index", repotypes.LangLua,
			`local Child = setmetatable({}, { __index = BaseHandler })`,
			"inherits BaseHandler", 1},
		{"Rust/supertrait", repotypes.LangRust,
			`pub trait SubAgent: Agent + Send + Sync {`,
			"supertrait Agent", 3},
		{"Cpp/inheritance", repotypes.LangCpp,
			`class Derived : public Base, protected Mixin {`,
			"inherits Base", 2},
		{"C/struct-embed", repotypes.LangC,
			`struct widget { struct base_object base; int flags; };`,
			"embeds struct base_object", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := splitTestLines(tc.src)
			got := scanEmbedsPatterns(lines, tc.lang)
			if tc.count == 0 {
				if len(got) != 0 {
					t.Errorf("expected no results, got %v", got)
				}
				return
			}
			if len(got) != tc.count {
				t.Fatalf("count = %d, want %d; got %v", len(got), tc.count, got)
			}
			if !containsSubstring(got[0].value, tc.want) {
				t.Errorf("value = %q, want substring %q", got[0].value, tc.want)
			}
			if got[0].kind != "embeds" {
				t.Errorf("kind = %q, want embeds", got[0].kind)
			}
		})
	}
}

// ── P0.3: implements ───────────────────────────────────────────────

func TestScanImplementsPatterns(t *testing.T) {
	cases := []struct {
		name  string
		lang  string
		src   string
		want  string
		count int
	}{
		{"Go/var-assert", repotypes.LangGo,
			`var _ SubAgent = (*SubExplorer)(nil)`,
			"implements SubAgent", 1},
		{"Go/var-assert-ref", repotypes.LangGo,
			`var _ io.Writer = &Buffer{}`,
			"implements io.Writer", 1},
		{"TS/implements", repotypes.LangTypeScript,
			`class Handler implements EventListener, Serializable {`,
			"implements EventListener", 2},
		{"Java/implements", repotypes.LangJava,
			`public class UserService implements Repository, Closeable {`,
			"implements Repository", 2},
		{"Swift/class-conformance", repotypes.LangSwift,
			`final class Greeter: NSObject, Greetable, Sendable {`,
			"implements Greetable", 2},
		{"Swift/struct-conformance", repotypes.LangSwift,
			`struct User: Codable, Sendable {`,
			"implements Codable", 2},
		{"Ruby/include", repotypes.LangRuby,
			`include Enumerable, Comparable`,
			"implements Enumerable", 2},
		{"Rust/impl-for", repotypes.LangRust,
			`impl SubAgent for SubExplorer {`,
			"SubExplorer implements SubAgent", 1},
		{"Rust/pub-impl-for", repotypes.LangRust,
			`pub impl Display for MyType {`,
			"MyType implements Display", 1},
		{"JS/no-concept", repotypes.LangJavaScript,
			`class Foo extends Bar {`, "", 0},
		{"Python/no-concept", repotypes.LangPython,
			`class Foo(Protocol):`, "", 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := splitTestLines(tc.src)
			got := scanImplementsPatterns(lines, tc.lang)
			if tc.count == 0 {
				if len(got) != 0 {
					t.Errorf("expected no results, got %v", got)
				}
				return
			}
			if len(got) != tc.count {
				t.Fatalf("count = %d, want %d; got %v", len(got), tc.count, got)
			}
			if !containsSubstring(got[0].value, tc.want) {
				t.Errorf("value = %q, want substring %q", got[0].value, tc.want)
			}
			if got[0].kind != "implements" {
				t.Errorf("kind = %q, want implements", got[0].kind)
			}
		})
	}
}

// ── P0.4: errors ───────────────────────────────────────────────────

func TestScanErrorsPatterns(t *testing.T) {
	cases := []struct {
		name string
		lang string
		src  string
		want string
	}{
		{"Go/Errorf", repotypes.LangGo,
			`return fmt.Errorf("analyze failed: %w", err)`,
			`"analyze failed: %w"`},
		{"Go/errors.New", repotypes.LangGo,
			`return errors.New("not found")`,
			`"not found"`},
		{"Go/panic", repotypes.LangGo,
			`panic("unreachable")`,
			`"unreachable"`},
		{"Python/raise", repotypes.LangPython,
			`raise ValueError("invalid input")`,
			"raises ValueError"},
		{"JS/throw", repotypes.LangJavaScript,
			`throw new TypeError("expected string");`,
			"throws new TypeError"},
		{"TS/throw", repotypes.LangTypeScript,
			`throw new Error("connection refused");`,
			"throws new Error"},
		{"Java/throw", repotypes.LangJava,
			`throw new IllegalArgumentException("bad param");`,
			"throws new IllegalArgumentException"},
		{"Cangjie/throw", repotypes.LangCangjie,
			`throw IllegalStateException("bad state")`,
			"throws IllegalStateException"},
		{"Ruby/raise", repotypes.LangRuby,
			`raise ConfigError, "missing route"`,
			"raises ConfigError"},
		{"Swift/throw", repotypes.LangSwift,
			`throw RoutingError.missingHandler`,
			"throws RoutingError.missingHandler"},
		{"Lua/error", repotypes.LangLua,
			`error("route missing")`,
			`"route missing"`},
		{"Rust/Err", repotypes.LangRust,
			`return Err(anyhow!("timeout"))`,
			"Err"},
		{"Rust/panic", repotypes.LangRust,
			`panic!("assertion failed: x > 0")`,
			`"assertion failed: x > 0"`},
		{"Cpp/throw", repotypes.LangCpp,
			`throw std::runtime_error("overflow");`,
			"throws std::runtime_error"},
		{"C/no-pattern", repotypes.LangC,
			`return -1;`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			lines := splitTestLines(tc.src)
			got := scanErrorsPatterns(lines, tc.lang)
			if tc.want == "" {
				if len(got) != 0 {
					t.Errorf("expected no results, got %v", got)
				}
				return
			}
			if len(got) == 0 {
				t.Fatalf("expected result containing %q, got nothing", tc.want)
			}
			if !containsSubstring(got[0].value, tc.want) {
				t.Errorf("value = %q, want substring %q", got[0].value, tc.want)
			}
			if got[0].kind != "errors" {
				t.Errorf("kind = %q, want errors", got[0].kind)
			}
		})
	}
}

// ── End-to-end: extractConcreteValues with lang ────────────────────

func TestExtractConcreteValues_LanguageAwareKinds(t *testing.T) {
	// Go source with all 4 new kinds present.
	src := `type ExecCommand struct {
	ReadOnly
	NonEvidenceTool
}

var _ Tool = (*ExecCommand)(nil)

func (t *ExecCommand) Execute(ctx *BusContext) error {
	if ctx.Mutable == nil {
		return fmt.Errorf("mutable is nil")
	}
	switch ctx.Stage {
	case StageAnalyze:
		return nil
	}
	panic("unreachable")
}`

	entries := extractConcreteValues(src, "go")
	kinds := map[string]bool{}
	for _, e := range entries {
		kinds[e.kind] = true
	}

	for _, want := range []string{"embeds", "implements", "conditional", "errors"} {
		if !kinds[want] {
			var ks []string
			for k := range kinds {
				ks = append(ks, k)
			}
			t.Errorf("expected kind %q in results, got kinds=%v", want, ks)
		}
	}
}

func TestExtractConcreteValues_NoLangSkipsNewKinds(t *testing.T) {
	// Same Go source but lang="" — should NOT emit new kinds.
	src := `type ExecCommand struct {
	ReadOnly
}
var _ Tool = (*ExecCommand)(nil)
func (t *ExecCommand) Execute() error {
	if x > 0 {
		return fmt.Errorf("bad")
	}
	return nil
}`
	entries := extractConcreteValues(src, "")
	for _, e := range entries {
		switch e.kind {
		case "embeds", "implements", "conditional", "errors":
			t.Errorf("lang=\"\" should not emit %q, got %+v", e.kind, e)
		}
	}
}

func TestExtractDeclarationConcreteValues(t *testing.T) {
	t.Run("ruby class ancestry and mixins", func(t *testing.T) {
		src := `class Greeter < BaseGreeter
  include Enumerable
  prepend Trackable
end`
		got := extractDeclarationConcreteValues(src, repotypes.LangRuby)
		want := []string{
			"embeds inherits BaseGreeter",
			"implements implements Enumerable",
			"implements implements Trackable",
		}
		for _, needle := range want {
			found := false
			for _, item := range got {
				if item.kind+" "+item.value == needle {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("missing %q in %+v", needle, got)
			}
		}
	})

	t.Run("swift inheritance and conformance", func(t *testing.T) {
		src := `final class Greeter: NSObject, Greetable, Sendable {
  func greet() -> String { "hi" }
}`
		got := extractDeclarationConcreteValues(src, repotypes.LangSwift)
		if len(got) != 3 {
			t.Fatalf("expected 3 declaration values, got %+v", got)
		}
		if got[0].kind != "embeds" || !strings.Contains(got[0].value, "NSObject") {
			t.Fatalf("expected embeds NSObject first, got %+v", got)
		}
	})

	t.Run("proto rpc contract", func(t *testing.T) {
		src := `rpc Hello(Greeting) returns (google.protobuf.Empty);`
		got := extractDeclarationConcreteValues(src, repotypes.LangProto)
		if len(got) != 2 {
			t.Fatalf("expected request+response rows, got %+v", got)
		}
		if got[0].kind != "maps" || got[0].value != "request => Greeting" {
			t.Fatalf("unexpected request row: %+v", got[0])
		}
		if got[1].kind != "returns" || got[1].value != "google.protobuf.Empty" {
			t.Fatalf("unexpected response row: %+v", got[1])
		}
	})
}

func TestConcreteValueSymbolKindCoverage(t *testing.T) {
	for _, kind := range []string{
		"function", "method", "ctor", "operator", "foreign-func",
		"builder", "styles", "ui-entry", "suspend-function",
		"extension-function", "extend",
	} {
		if !isConcreteValueBodySymbolKind(kind) {
			t.Fatalf("body kind %q should be supported", kind)
		}
	}
	for _, kind := range []string{
		"class", "protocol", "module", "rpc", "service", "component",
		"actor", "struct", "enum", "object", "companion-object",
		"data-class", "sealed-class", "annotation",
	} {
		if !isConcreteValueDeclarationSymbolKind(kind) {
			t.Fatalf("declaration kind %q should be supported", kind)
		}
	}
	if isConcreteValueBodySymbolKind("class") {
		t.Fatal("class should not be treated as body-scanned")
	}
}

func TestConcreteValueMatchesFocus_OwnerOnly(t *testing.T) {
	focus := map[string]bool{"greeter": true}
	sym := &repomap.Symbol{Name: "Hello", Parent: "Greeter"}
	if !concreteValueMatchesFocus(sym, focus) {
		t.Fatal("owner-only focus should include service/class members")
	}
}

// ── Helpers ────────────────────────────────────────────────────────

func splitTestLines(src string) []string {
	return strings.Split(src, "\n")
}

func containsSubstring(s, sub string) bool {
	return strings.Contains(s, sub)
}
