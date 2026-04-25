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

func TestDetectUnknownEffect_ExtendedLanguages(t *testing.T) {
	cases := []struct {
		lang string
		line string
	}{
		{repomap.LangKotlin, `val k = Class.forName(name)`},
		{repomap.LangRuby, `handler.public_send(method_name)`},
		{repomap.LangSwift, `let t = Mirror(reflecting: value)`},
		{repomap.LangLua, `local fn = loadfile(path)`},
		{repomap.LangArkTS, `const out = Reflect.get(obj, key)`},
	}
	for _, c := range cases {
		if got := detectUnknownEffect(c.lang, c.line); got == "" {
			t.Fatalf("detectUnknownEffect(%q, %q) = empty, want non-empty reason", c.lang, c.line)
		}
	}
}
