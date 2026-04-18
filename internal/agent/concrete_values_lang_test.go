package agent

import (
	"strings"
	"testing"

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
		{"Java/if", repotypes.LangJava,
			`if (request.getMethod().equals("POST")) {`, `request.getMethod().equals("POST")`},
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

// ── Helpers ────────────────────────────────────────────────────────

func splitTestLines(src string) []string {
	return strings.Split(src, "\n")
}

func containsSubstring(s, sub string) bool {
	return strings.Contains(s, sub)
}
