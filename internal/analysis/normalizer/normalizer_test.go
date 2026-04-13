package normalizer

import (
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// findByID returns the canonical term with the given ID, or nil.
func findByID(tg types.TermGraph, id string) *types.CanonicalTerm {
	for i := range tg.Canonical {
		if tg.Canonical[i].ID == id {
			return &tg.Canonical[i]
		}
	}
	return nil
}

func hasAlias(tg types.TermGraph, source, target, relation string) bool {
	for _, a := range tg.Aliases {
		if a.Source == source && a.Target == target && a.Relation == relation {
			return true
		}
	}
	return false
}

func canonicalIDs(tg types.TermGraph) []string {
	out := make([]string, 0, len(tg.Canonical))
	for _, c := range tg.Canonical {
		out = append(out, c.ID)
	}
	return out
}

func TestNormalize_CamelCaseAndSnakeCaseCollapse(t *testing.T) {
	tg := Normalize("Does TaskList share state with task_list in the same file?", Options{})
	if term := findByID(tg, "code:tasklist"); term == nil {
		t.Fatalf("expected code:tasklist canonical, got IDs=%v", canonicalIDs(tg))
	}
	// Whichever surface was first in the text should be canonical; the
	// other becomes an alias.
	if !hasAlias(tg, "task_list", "code:tasklist", "synonym") &&
		!hasAlias(tg, "TaskList", "code:tasklist", "synonym") {
		t.Fatalf("expected one of TaskList/task_list to be aliased; aliases=%+v", tg.Aliases)
	}
}

func TestNormalize_ChineseEnglishMixed(t *testing.T) {
	tg := Normalize("请解释 explorer 是如何决定 ShouldStop 的", Options{})
	if findByID(tg, "code:shouldstop") == nil {
		t.Fatalf("expected code:shouldstop; got IDs=%v", canonicalIDs(tg))
	}
	if findByID(tg, "en:explorer") == nil {
		t.Fatalf("expected en:explorer; got IDs=%v", canonicalIDs(tg))
	}
	if findByID(tg, "zh:解释") == nil {
		t.Fatalf("expected zh:解释; got IDs=%v", canonicalIDs(tg))
	}
	// 解释 should alias to en:explain (not present here as canonical)
	// → not emitted. But 探索器 is absent from text so no zh:探索器.
	// Just make sure 请 was stripped as a leading stopword.
	for _, id := range canonicalIDs(tg) {
		if strings.Contains(id, "请") {
			t.Fatalf("请 leaked into canonical IDs: %v", canonicalIDs(tg))
		}
	}
}

func TestNormalize_ChineseTranslationAlias(t *testing.T) {
	// When both the zh phrase and its English translation appear, the
	// alias graph must link zh → en.
	tg := Normalize("探索器 explorer 如何工作", Options{})
	if findByID(tg, "en:explorer") == nil {
		t.Fatalf("expected en:explorer canonical; ids=%v", canonicalIDs(tg))
	}
	if !hasAlias(tg, "探索器", "en:explorer", "translation") {
		t.Fatalf("expected translation alias 探索器→en:explorer; aliases=%+v", tg.Aliases)
	}
}

func TestNormalize_FilePathAndConfig(t *testing.T) {
	tg := Normalize("bug in internal/agent/explorer.go around ShouldStop", Options{})
	if findByID(tg, "cfg:internal/agent/explorer.go") == nil {
		t.Fatalf("expected cfg:internal/agent/explorer.go; ids=%v", canonicalIDs(tg))
	}
	if findByID(tg, "code:shouldstop") == nil {
		t.Fatalf("expected code:shouldstop; ids=%v", canonicalIDs(tg))
	}
}

func TestNormalize_CLIFlag(t *testing.T) {
	tg := Normalize("run with --pipeline-max-steps 50 and -repo .", Options{})
	if findByID(tg, "cmd:pipeline-max-steps") == nil {
		t.Fatalf("expected cmd:pipeline-max-steps; ids=%v", canonicalIDs(tg))
	}
	if findByID(tg, "cmd:repo") == nil {
		t.Fatalf("expected cmd:repo; ids=%v", canonicalIDs(tg))
	}
}

func TestNormalize_StopwordsFiltered(t *testing.T) {
	tg := Normalize("How does the explorer decide when to stop?", Options{})
	// "How", "does", "the", "decide", "when" should all be filtered,
	// leaving only "explorer" and "stop" (both ≥4 chars, non-stopword).
	for _, id := range canonicalIDs(tg) {
		if strings.Contains(strings.ToLower(id), ":how") ||
			strings.Contains(strings.ToLower(id), ":does") ||
			strings.Contains(strings.ToLower(id), ":the") {
			t.Fatalf("stopword leaked: %v", canonicalIDs(tg))
		}
	}
	if findByID(tg, "en:explorer") == nil {
		t.Fatalf("expected en:explorer; ids=%v", canonicalIDs(tg))
	}
}

func TestNormalize_QuotedLiteralDroppedByDefault(t *testing.T) {
	tg := Normalize(`answer must include "explorer" literally`, Options{})
	if findByID(tg, "lit:explorer") != nil {
		t.Fatalf("literal should be dropped by default; ids=%v", canonicalIDs(tg))
	}
}

func TestNormalize_QuotedLiteralPreserved(t *testing.T) {
	tg := Normalize(`answer must include "explorer" literally`, Options{PreserveLiterals: true})
	if findByID(tg, "lit:explorer") == nil {
		t.Fatalf("literal should be preserved; ids=%v", canonicalIDs(tg))
	}
}

// fakeResolver simulates a repo symbol table.
type fakeResolver struct {
	hits map[string][]SymbolHit
}

func (f fakeResolver) LookupSymbol(surface string) []SymbolHit {
	return f.hits[surface]
}

func TestNormalize_ResolverUpgradesEnConceptToCodeSymbol(t *testing.T) {
	resolver := fakeResolver{hits: map[string][]SymbolHit{
		"explorer": {{Canonical: "Explorer", Domain: "agent"}},
	}}
	tg := Normalize("how does the explorer stop", Options{Resolver: resolver})
	term := findByID(tg, "code:explorer")
	if term == nil {
		t.Fatalf("expected code:explorer after resolver upgrade; ids=%v", canonicalIDs(tg))
	}
	if term.Domain != "agent" {
		t.Fatalf("expected domain=agent, got %q", term.Domain)
	}
	if term.Confidence < 0.99 {
		t.Fatalf("resolver hit should set confidence=1.0, got %v", term.Confidence)
	}
}

func TestNormalize_EnglishPluralAlias(t *testing.T) {
	tg := Normalize("list the hooks and one hook in the config", Options{})
	// Both "hooks" and "hook" ≥4 chars. "hook" is 4 — ok.
	// Ensure the alias hooks→en:hook exists.
	if findByID(tg, "en:hook") == nil {
		t.Fatalf("expected en:hook; ids=%v", canonicalIDs(tg))
	}
	if !hasAlias(tg, "hooks", "en:hook", "synonym") {
		t.Fatalf("expected plural alias hooks→en:hook; aliases=%+v", tg.Aliases)
	}
}

func TestResolve_ReturnsCanonicalThenAliases(t *testing.T) {
	tg := Normalize("TaskList and task_list", Options{})
	surfaces := Resolve(&tg, "code:tasklist")
	if len(surfaces) < 2 {
		t.Fatalf("expected ≥2 surfaces for code:tasklist; got %v", surfaces)
	}
	if surfaces[0] != "TaskList" {
		t.Fatalf("expected canonical surface first; got %v", surfaces)
	}
}

func TestResolve_NilGraph(t *testing.T) {
	if got := Resolve(nil, "code:anything"); got != nil {
		t.Fatalf("nil graph should return nil; got %v", got)
	}
}

func TestNormalize_Deterministic(t *testing.T) {
	raw := "请解释 ExplorerAgent 是如何 ShouldStop 的, 见 internal/agent/explorer.go"
	a := Normalize(raw, Options{})
	b := Normalize(raw, Options{})
	if len(a.Canonical) != len(b.Canonical) || len(a.Aliases) != len(b.Aliases) {
		t.Fatalf("non-deterministic output: %+v vs %+v", a, b)
	}
	for i := range a.Canonical {
		if a.Canonical[i].ID != b.Canonical[i].ID {
			t.Fatalf("canonical order drift at %d: %v vs %v", i, a.Canonical[i].ID, b.Canonical[i].ID)
		}
	}
	for i := range a.Aliases {
		if a.Aliases[i] != b.Aliases[i] {
			t.Fatalf("alias order drift at %d: %+v vs %+v", i, a.Aliases[i], b.Aliases[i])
		}
	}
}

// Over-fit audit: deletions. Removing the normalizer's knowledge of a
// term must never cause crashes, only degraded recall. This test
// ensures the normalizer degrades gracefully on a fully unknown input.
func TestNormalize_EmptyAndGibberish(t *testing.T) {
	cases := []string{"", "   ", "!!!", "123 456", "...", "???"}
	for _, c := range cases {
		tg := Normalize(c, Options{})
		if len(tg.Canonical) != 0 {
			t.Fatalf("expected empty canonical for %q; got %v", c, canonicalIDs(tg))
		}
		if len(tg.Aliases) != 0 {
			t.Fatalf("expected empty aliases for %q; got %+v", c, tg.Aliases)
		}
	}
}
