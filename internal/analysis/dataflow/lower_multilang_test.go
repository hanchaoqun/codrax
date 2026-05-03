package dataflow

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap"
)

func TestNewLowererRegistry_CoversAllSupportedReadLanguages(t *testing.T) {
	registry := newLowererRegistry()
	for _, lang := range []string{
		repomap.LangGo,
		repomap.LangPython,
		repomap.LangJavaScript,
		repomap.LangTypeScript,
		repomap.LangArkTS,
		repomap.LangCangjie,
		repomap.LangJava,
		repomap.LangKotlin,
		repomap.LangRust,
		repomap.LangRuby,
		repomap.LangSwift,
		repomap.LangLua,
		repomap.LangProto,
		repomap.LangC,
		repomap.LangCpp,
	} {
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

func TestDetectGuard_ExtendedLanguages(t *testing.T) {
	cases := []struct {
		lang string
		line string
		want string
	}{
		{repomap.LangSwift, "guard user != nil else {", "guard user != nil else"},
		{repomap.LangKotlin, "when (state) {", "when (state)"},
		{repomap.LangLua, "elseif ready then", "elseif ready then"},
		{repomap.LangRuby, "unless enabled?", "unless enabled?"},
	}
	for _, c := range cases {
		if got := detectGuard(c.lang, c.line); got != c.want {
			t.Fatalf("detectGuard(%q, %q) = %q, want %q", c.lang, c.line, got, c.want)
		}
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
	cases := []struct {
		lang string
	}{
		{repomap.LangKotlin}, {repomap.LangRuby}, {repomap.LangSwift},
		{repomap.LangLua}, {repomap.LangArkTS}, {repomap.LangGo},
		{repomap.LangPython}, {repomap.LangJavaScript}, {repomap.LangTypeScript},
		{repomap.LangRust}, {repomap.LangC}, {repomap.LangCpp},
		{repomap.LangJava}, {repomap.LangCangjie},
	}
	for _, c := range cases {
		fi := &repomap.FileInfo{
			Language:     c.lang,
			LineFeatures: map[int][]repomap.LineFeature{1: {repomap.LineFeatureUnknownEffect}},
		}
		if got := detectUnknownEffect(fi, 1, "irrelevant"); got == "" {
			t.Errorf("language %s: typed feature should yield descriptor; got empty", c.lang)
		}
	}
}
