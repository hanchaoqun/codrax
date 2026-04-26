package ground

import (
	"testing"

	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestIsLineComment_ExtendedLanguageMarkers(t *testing.T) {
	cases := []struct {
		source string
		lines  map[int]string
	}{
		{source: "a.kt", lines: map[int]string{1: "// kotlin comment"}},
		{source: "a.ets", lines: map[int]string{1: "// arkts comment"}},
		{source: "a.cj", lines: map[int]string{1: "// cangjie comment"}},
		{source: "a.swift", lines: map[int]string{1: "// swift comment"}},
		{source: "a.proto", lines: map[int]string{1: "// proto comment"}},
		{source: "a.rb", lines: map[int]string{1: "# ruby comment"}},
		{source: "a.lua", lines: map[int]string{1: "-- lua comment"}},
	}
	for _, c := range cases {
		if !isLineComment(c.lines, 1, c.source) {
			t.Fatalf("isLineComment(%q) = false, want true", c.source)
		}
	}
}

func TestCommentLanguageHelpers_ExtendedSets(t *testing.T) {
	for _, lang := range []string{
		repomaptypes.LangKotlin,
		repomaptypes.LangArkTS,
		repomaptypes.LangCangjie,
		repomaptypes.LangSwift,
		repomaptypes.LangProto,
	} {
		if !hasCFamilyLineComment(lang) {
			t.Fatalf("hasCFamilyLineComment(%q) = false, want true", lang)
		}
	}
	if !hasHashLineComment(repomaptypes.LangRuby) {
		t.Fatal("hasHashLineComment(ruby) = false, want true")
	}
	if !hasLuaLineComment(repomaptypes.LangLua) {
		t.Fatal("hasLuaLineComment(lua) = false, want true")
	}
	if !hasLuaBlockComment(repomaptypes.LangLua) {
		t.Fatal("hasLuaBlockComment(lua) = false, want true")
	}
}

func TestIsInsideBlockComment_LuaLongComment(t *testing.T) {
	lines := map[int]string{
		1: "--[[",
		2: "comment body",
		3: "]]",
	}
	if !isLineComment(lines, 2, "a.lua") {
		t.Fatal("lua long comment body should be treated as comment")
	}
}
