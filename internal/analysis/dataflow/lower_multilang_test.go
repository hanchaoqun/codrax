package dataflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func TestNewLowererRegistry_CoversAllSupportedReadLanguages(t *testing.T) {
	registry := newLowererRegistry()
	for _, lang := range repomap.SupportedReadLanguages() {
		if _, ok := registry[lang]; !ok {
			t.Fatalf("language %q missing from lowerer registry", lang)
		}
	}
}

func TestIsCommentLine_ExtendedLanguages(t *testing.T) {
	cases := []struct {
		lang string
		line string
	}{
		{repomap.LangRuby, "# comment"},
		{repomap.LangLua, "-- comment"},
		{repomap.LangKotlin, "// comment"},
		{repomap.LangSwift, "// comment"},
		{repomap.LangArkTS, "// comment"},
		{repomap.LangCangjie, "// comment"},
	}
	for _, c := range cases {
		if !isCommentLine(c.lang, c.line) {
			t.Fatalf("isCommentLine(%q, %q) = false, want true", c.lang, c.line)
		}
	}
}

// TestDetectGuard_ExtendedLanguages was retired 2026-05-03
// (Phase 6 stage 28). The byte-prefix detectGuard signature
// was replaced with a typed detectGuard(*FileInfo, line, text)
// that reads LineFeatureGuard populated by repomap's AST
// walker. The retired keyword table ({"if ", "if(", "else if",
// "elseif", "elif", "unless", "guard", "switch", "case",
// "when", "match", "until"}) is now AST-detected via the
// closed enum if_statement / case_clause / when_entry /
// guard_statement / unless_statement / etc.
func TestDetectGuard_TypedFeature(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{"swift guard", "guard user != nil else {", "guard user != nil else"},
		{"kotlin when", "when (state) {", "when (state)"},
		{"lua elseif", "elseif ready then", "elseif ready then"},
		{"ruby unless", "unless enabled?", "unless enabled?"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			fi := &repomap.FileInfo{
				LineFeatures: map[int][]repomap.LineFeature{1: {repomap.LineFeatureGuard}},
			}
			if got := detectGuard(fi, 1, c.line); got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// TestDetectUnknownEffect_ExtendedLanguages was retired
// 2026-05-03 (Phase 6 stage 27). The byte-token detectUnknownEffect
// signature was replaced with a typed
// detectUnknownEffect(*FileInfo, line, text) that reads
// LineFeatures populated by repomap's AST walker. The retired
// per-language byte-token tables (Class.forName / public_send /
// Mirror / loadfile / Reflect.get) are now AST-detected via
// dynamicDispatchCallees + typed LineFeatureUnknownEffect.
//
// Replacement: TestDetectUnknownEffect_TypedFeature exercises the
// typed contract — pass a synthetic FileInfo whose LineFeatures
// includes LineFeatureUnknownEffect at the given line and verify
// detectUnknownEffect returns the language's descriptor.
func TestDetectUnknownEffect_TypedFeature(t *testing.T) {
	for _, lang := range repomap.SupportedReadLanguages() {
		fi := &repomap.FileInfo{
			Language:     lang,
			LineFeatures: map[int][]repomap.LineFeature{1: {repomap.LineFeatureUnknownEffect}},
		}
		if got := detectUnknownEffect(fi, 1, "irrelevant"); got == "" {
			t.Errorf("language %s: typed feature should yield descriptor; got empty", lang)
		}
	}
}
