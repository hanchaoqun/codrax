package ground

import (
	"fmt"
	"testing"

	repomaptypes "github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func b1628CommentLines(lines ...string) map[int]string {
	out := make(map[int]string, len(lines))
	for i, line := range lines {
		out[i+1] = line
	}
	return out
}

func TestB1628CommentStateObservedSequence(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		lines        []string
		line         int
		want         bool
	}{
		{"python middle", "a.py", []string{"def f():", `    """`, "    prose", "    resolve(kind)", `    """`}, 4, true},
		{"python closed", "a.py", []string{"def f():", `    """`, "    prose", `    """`, "    resolve(kind)"}, 5, false},
		{"python other triple is payload", "a.py", []string{`"""`, `example '''`, `"""`, "resolve(kind)"}, 4, false},
		{"python single triple", "a.py", []string{`'''`, "prose", "resolve(kind)", `'''`}, 3, true},
		{"python hash marker", "a.py", []string{`# """`, "pass", "resolve(kind)"}, 3, false},
		{"python short quoted marker", "a.py", []string{`marker = '"""'`, "pass", "resolve(kind)"}, 3, false},
		{"python escaped triple", "a.py", []string{`"""`, `escaped \""" text`, "resolve(kind)", `"""`}, 3, true},
		{"python same line after close", "a.py", []string{`"""`, "prose", `"""; resolve(kind)`}, 3, false},
		{"python prefixed interpolation opaque", "a.py", []string{`x = f"""`, "{resolve(kind)}", `"""`}, 2, false},
		{"python assigned triple opaque", "a.py", []string{`text = """`, "resolve(kind)", `"""`}, 2, false},
		{"python nested triple opaque", "a.py", []string{`( """`, "resolve(kind)", `""")`}, 2, false},
		{"python continued expression opaque", "a.py", []string{`text = (`, `"""`, "resolve(kind)", `""")`}, 3, false},
		{"c middle", "a.c", []string{"/*", "prose", "resolve(kind);", "*/"}, 3, true},
		{"c nonnested", "a.c", []string{"/* outer /* payload */", "resolve(kind);"}, 2, false},
		{"c quoted opener", "a.c", []string{`char *x = "/*";`, "pass;", "resolve(kind);"}, 3, false},
		{"c line comment opener", "a.c", []string{"// /*", "pass;", "resolve(kind);"}, 3, false},
		{"c same line code after close", "a.c", []string{"/*", "prose", "*/ resolve(kind);"}, 3, false},
		{"c same line block and code", "a.c", []string{"/* prose */ resolve(kind);"}, 1, false},
		{"c pointer not decoration", "a.c", []string{"* target = resolve(kind);"}, 1, false},
		{"c same line multiple blocks", "a.c", []string{"/* a */ /* b", "resolve(kind);", "*/"}, 2, true},
		{"cpp raw literal", "a.cpp", []string{`auto x = R"tag(/*`, "resolve(kind);", `)tag";`, "resolve(kind);"}, 4, false},
		{"rust raw literal", "a.rs", []string{`let x = r##"/*`, "resolve(kind);", `"##;`, "resolve(kind);"}, 4, false},
		{"swift extended raw literal", "a.swift", []string{`let message = #"payload " /* "#`, "resolve(kind)"}, 2, false},
		{"swift interpolation unknown", "a.swift", []string{`let message = "\(fn("/*"))"`, "resolve(kind)"}, 2, false},
		{"swift raw interpolation unknown", "a.swift", []string{`let message = #"\#(fn("/*"))"#`, "resolve(kind)"}, 2, false},
		{"kotlin interpolation unknown", "a.kt", []string{`val message = "${fn("/*")}"`, "resolve(kind)"}, 2, false},
		{"cangjie interpolation unknown", "a.cj", []string{`let message = "${fn("/*")}"`, "resolve(kind)"}, 2, false},
		{"java template unknown", "a.java", []string{`String message = STR."\{fn("/*")}";`, "resolve(kind);"}, 2, false},
		{"python fstring unknown continuation", "a.py", []string{`text = f"{fn("inner")}"`, `"""`, "resolve(kind)", `"""`}, 3, false},
		{"cangjie complex literal unknown", "a.cj", []string{`let message = #"payload " /* "#`, "resolve(kind)"}, 2, false},
		{"go raw literal", "a.go", []string{"x := `/*", "resolve(kind)", "`", "resolve(kind)"}, 4, false},
		{"js regex marker", "a.js", []string{`const x = /[/*]/;`, "noop();", "resolve(kind);"}, 3, false},
		{"js template payload", "a.ts", []string{"const x = `/*", "${resolve(kind)}", "`;", "resolve(kind);"}, 4, false},
		{"js nested template unknown", "a.js", []string{"const message = `outer ${`inner /*`}`;", "resolve(kind);"}, 2, false},
		{"ruby heredoc unknown", "a.rb", []string{"message = <<~DOC", "# resolve(kind)", "DOC"}, 2, false},
		{"ruby percent literal unknown", "a.rb", []string{"message = %q{", "# resolve(kind)", "}"}, 2, false},
		{"lua middle", "a.lua", []string{"--[[", "prose", "resolve(kind)", "]]"}, 3, true},
		{"lua close", "a.lua", []string{"--[[", "prose", "]]", "resolve(kind)"}, 4, false},
		{"lua same line close code", "a.lua", []string{"--[[", "prose", "]] resolve(kind)"}, 3, false},
		{"lua equals comment", "a.lua", []string{"--[=[", "prose ]]", "resolve(kind)", "]=]"}, 3, true},
		{"lua quoted marker", "a.lua", []string{`local x = "--[["`, "noop()", "resolve(kind)"}, 3, false},
		{"lua long string", "a.lua", []string{"local x = [=[", "--[[", "]=]", "resolve(kind)"}, 4, false},
		{"config html middle", "a.xml", []string{"<!--", "prose", "resolve", "-->"}, 3, true},
		{"config close and content", "a.xml", []string{"<!--", "prose", "--> <resolve/>"}, 3, false},
		{"config quoted marker", "a.json", []string{`{"marker":"/*",`, `"resolve": true}`}, 2, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := LineLooksCommentOnly(b1628CommentLines(tc.lines...), tc.line, tc.source); got != tc.want {
				t.Fatalf("%s:%d comment-only=%t want=%t", tc.source, tc.line, got, tc.want)
			}
		})
	}
}

func TestB1628CommentStateLanguageMatrix(t *testing.T) {
	paths := map[string]string{
		repomaptypes.LangGo: "a.go", repomaptypes.LangPython: "a.py", repomaptypes.LangJavaScript: "a.js",
		repomaptypes.LangTypeScript: "a.ts", repomaptypes.LangArkTS: "a.ets", repomaptypes.LangCangjie: "a.cj",
		repomaptypes.LangJava: "a.java", repomaptypes.LangKotlin: "a.kt", repomaptypes.LangRust: "a.rs",
		repomaptypes.LangSwift: "a.swift", repomaptypes.LangProto: "a.proto", repomaptypes.LangC: "a.c",
		repomaptypes.LangCpp: "a.cpp", repomaptypes.LangRuby: "a.rb", repomaptypes.LangLua: "a.lua",
	}
	for _, lang := range repomaptypes.SupportedReadLanguages() {
		t.Run(lang, func(t *testing.T) {
			path, ok := paths[lang]
			if !ok {
				t.Fatalf("language %s needs an explicit syntax fixture", lang)
			}
			marker := "//"
			if hasHashLineComment(lang) {
				marker = "#"
			}
			if hasLuaLineComment(lang) {
				marker = "--"
			}
			if !LineLooksCommentOnly(map[int]string{50: marker + " resolve(kind)"}, 50, path) {
				t.Fatal("standalone line comment lost")
			}
			if hasCFamilyBlockComment(lang) {
				rows := b1628CommentLines("/*", "prose", "resolve(kind)", "*/", "resolve(kind)")
				if !LineLooksCommentOnly(rows, 3, path) || LineLooksCommentOnly(rows, 5, path) {
					t.Fatal("observed block/closed code distinction lost")
				}
				for _, prefix := range []string{`const marker = "/*";`, "// /*"} {
					if LineLooksCommentOnly(b1628CommentLines(prefix, "noop()", "resolve(kind)"), 3, path) {
						t.Fatal("quoted/commented delimiter minted comment")
					}
				}
			}
		})
	}
}

func TestB1628CommentStateUnknownBoundaryDoesNotMintRole(t *testing.T) {
	for _, path := range []string{"a.py", "a.c", "a.lua", "a.xml"} {
		t.Run(path, func(t *testing.T) {
			for _, prefix := range []string{`"""`, "/*", "--[[", "<!--"} {
				rows := map[int]string{10: prefix, 11: "prose", 12: "resolve(kind)"}
				if LineLooksCommentOnly(rows, 12, path) {
					t.Fatal("minimum observed key was treated as file start")
				}
				rows[1] = "header"
				if LineLooksCommentOnly(rows, 12, path) {
					t.Fatal("crossed unseen gap")
				}
			}
		})
	}
	rows := b1628CommentLines(`"""`)
	for i := 2; i <= 202; i++ {
		rows[i] = "prose"
	}
	rows[202] = "resolve(kind)"
	if LineLooksCommentOnly(rows, 202, "a.py") {
		t.Fatal("walk cap was treated as a proven lexical origin")
	}
	if !LineLooksCommentOnly(rows, 201, "a.py") {
		t.Fatal("inclusive 200 prior-line bound lost")
	}
	before := fmt.Sprint(rows)
	_ = LineLooksCommentOnly(rows, 202, "a.py")
	if fmt.Sprint(rows) != before {
		t.Fatal("source lines mutated")
	}
}
