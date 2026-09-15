package index

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// The JSON projection deliberately exercises ParseFiles without requiring a
// not-yet-published Go field; the original implementation fails behaviorally.
type b1692ReturnRecord struct {
	CallableName     string `json:"callable_name"`
	CallableReceiver string `json:"callable_receiver"`
	CallableParent   string `json:"callable_parent"`
	CallableLine     int    `json:"callable_line"`
	CallableEndLine  int    `json:"callable_end_line"`
	Kind             string `json:"kind"`
	Expression       string `json:"expression"`
	LineStart        int    `json:"line_start"`
	LineEnd          int    `json:"line_end"`
	StartByte        uint32 `json:"start_byte"`
	EndByte          uint32 `json:"end_byte"`
	Provenance       string `json:"provenance"`
	ResolvedBy       string `json:"resolved_by"`
}

func TestB1692PublicParserParallelFilesKeepSourceOwners(t *testing.T) {
	dir := t.TempDir()
	var entries []FileEntry
	sources := map[string][]byte{}
	for i := 0; i < 16; i++ {
		rel := fmt.Sprintf("scope%d/work.go", i)
		source := []byte(fmt.Sprintf("package p\nfunc work() string { return \"value%d\" }\n", i))
		path := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, source, 0600); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, FileEntry{AbsPath: path, RelPath: rel, Language: types.LangGo, Size: int64(len(source))})
		sources[rel] = source
	}
	files := ParseFiles(entries, dir)
	if len(files) != len(entries) {
		t.Fatal("public batch parse missing files")
	}
	for i, fi := range files {
		if fi.RelPath != entries[i].RelPath || len(fi.Symbols) != 1 {
			t.Fatal("parser source-order/owner mismatch")
		}
		got := types.NewCallableReturnReader(fi, sources[fi.RelPath]).For(fi.Symbols[0])
		if len(got) != 1 || got[0].Expression != fmt.Sprintf("\"value%d\"", i) {
			t.Fatalf("parallel file return mixed: %+v", got)
		}
	}
}

func b1692ParsePublic(t *testing.T, language, source string) (*types.FileInfo, []b1692ReturnRecord) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "fixture")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	files := ParseFiles([]FileEntry{{AbsPath: path, RelPath: "fixture", Language: language, Size: int64(len(source))}}, dir)
	if len(files) != 1 || len(files[0].Symbols) == 0 {
		t.Fatalf("public parser missing prerequisite symbols: %+v", files)
	}
	b, err := json.Marshal(files[0])
	if err != nil {
		t.Fatal(err)
	}
	var published struct {
		Returns []b1692ReturnRecord `json:"callable_return_expressions"`
	}
	if err := json.Unmarshal(b, &published); err != nil {
		t.Fatal(err)
	}
	for _, r := range published.Returns {
		matches := 0
		for _, s := range files[0].Symbols {
			if s.Name == r.CallableName && s.Receiver == r.CallableReceiver && s.Parent == r.CallableParent && s.Line == r.CallableLine && s.EndLine == r.CallableEndLine {
				matches++
			}
		}
		if matches != 1 || r.LineStart < r.CallableLine || r.LineEnd > r.CallableEndLine || r.LineEnd < r.LineStart || r.EndByte <= r.StartByte || int(r.EndByte) > len(source) || source[r.StartByte:r.EndByte] != r.Expression || r.Provenance == "" || r.ResolvedBy == "" {
			t.Fatalf("return lost exact owner/expression provenance: %+v, symbols=%+v", r, files[0].Symbols)
		}
	}
	return files[0], published.Returns
}

func TestB1692PublicParserExplicitReturnLanguageMatrix(t *testing.T) {
	cases := []struct{ language, source string }{
		{types.LangGo, "package p\nfunc work() string {\n return \"value\"\n}\n"},
		{types.LangPython, "def work():\n    return \"value\"\n"},
		{types.LangJavaScript, "function work() {\n return \"value\";\n}\n"},
		{types.LangTypeScript, "function work(): string {\n return \"value\";\n}\n"},
		{types.LangArkTS, "function work(): string {\n return \"value\";\n}\n"},
		{types.LangJava, "class P { String work() {\n return \"value\";\n} }\n"},
		{types.LangKotlin, "fun work(): String {\n return \"value\"\n}\n"},
		{types.LangRust, "fn work() -> &'static str {\n return \"value\";\n}\n"},
		{types.LangC, "const char* work(void) {\n return \"value\";\n}\n"},
		{types.LangCpp, "const char* work() {\n return \"value\";\n}\n"},
		{types.LangRuby, "def work\n return \"value\"\nend\n"},
		{types.LangSwift, "func work() -> String {\n return \"value\"\n}\n"},
		{types.LangLua, "function work()\n return \"value\"\nend\n"},
		{types.LangCangjie, "package p\nfunc work(): String {\n return \"value\"\n}\n"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			_, rows := b1692ParsePublic(t, tc.language, tc.source)
			if len(rows) != 1 || rows[0].CallableName != "work" || rows[0].Expression != `"value"` || rows[0].Kind != "explicit_return" {
				if tc.language != types.LangCangjie {
					t.Logf("AST: %s", parseSourceFor(t, tc.language, tc.source).String())
				}
				t.Fatalf("public parser lost actual explicit return: %+v", rows)
			}
		})
	}
}

func TestB1692PublicParserExpressionAndOwnership(t *testing.T) {
	cases := []struct {
		name, language, source string
		want                   map[string][]string
		kind                   string
	}{
		{"js_arrow", types.LangJavaScript, "const work = () => ({ value: \"arrow\" });", map[string][]string{"work": {`({ value: "arrow" })`}}, "expression_body"},
		{"ts_multiline_arrow", types.LangTypeScript, "const work = () =>\n  (\"arrow\");", map[string][]string{"work": {`("arrow")`}}, "expression_body"},
		{"arkts_arrow", types.LangArkTS, "const work = () => \"arrow\";", map[string][]string{"work": {`"arrow"`}}, "expression_body"},
		{"rust_tail", types.LangRust, "fn work() -> &'static str {\n \"tail\"\n}", map[string][]string{"work": {`"tail"`}}, "implicit_tail"},
		{"rust_tail_match", types.LangRust, "fn work(x: bool) -> &'static str {\n match x { true => \"yes\", false => \"no\" }\n}", map[string][]string{"work": {`match x { true => "yes", false => "no" }`}}, "implicit_tail"},
		{"rust_assignment_match_not_return", types.LangRust, "fn work(x: bool) {\n let mut output = \"\";\n match x { true => output = \"yes\", false => output = \"no\" };\n}", map[string][]string{}, ""},
		{"rust_mid_literal_not_tail", types.LangRust, "fn work() {\n \"not_return\";\n finish();\n}", map[string][]string{}, ""},
		{"js_closure_same_line", types.LangJavaScript, "function work() { const inner = () => { return \"hidden\"; }; return \"outer\"; }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"go_closure", types.LangGo, "package p\nfunc work() string {\n inner := func() string { return \"hidden\" }; _ = inner\n return \"outer\"\n}", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"rust_closure_async_item", types.LangRust, "fn work() -> &'static str {\n let inner = || { return \"hidden\"; };\n let pending = async { return \"pending\"; };\n fn nested() -> &'static str { return \"nested\"; }\n return \"outer\";\n}", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"python_nested", types.LangPython, "def work():\n    def inner():\n        return \"hidden\"\n    return \"outer\"\n", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"adjacent_callables", types.LangJavaScript, "function first() { return \"first\"; } function second() { return \"second\"; }", map[string][]string{"first": {`"first"`}, "second": {`"second"`}}, "explicit_return"},
		{"string_tokens_not_code", types.LangJavaScript, "function work() { const note = ' return \"hidden\"; => false'; return \"outer\"; }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"typescript_block_arrow", types.LangTypeScript, "const work = () => { const hidden = () => \"nested\"; return \"outer\"; };", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"js_generator_boundary", types.LangJavaScript, "function work() { const hidden = function* () { return \"hidden\"; }; return \"outer\"; }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"java_lambda_boundary", types.LangJava, "class P { String work() { Object hidden = () -> { return \"hidden\"; }; return \"outer\"; } }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"cpp_lambda_boundary", types.LangCpp, "const char* work() { auto hidden = []() { return \"hidden\"; }; return \"outer\"; }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"kotlin_anonymous_boundary", types.LangKotlin, "fun work(): String { val hidden = fun(): String { return \"hidden\" }; return \"outer\" }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"swift_closure_boundary", types.LangSwift, "func work() -> String { let hidden = { () -> String in return \"hidden\" }; return \"outer\" }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"swift_computed_property_boundary", types.LangSwift, "func work() -> String { var hidden: String { return \"hidden\" }; return \"outer\" }", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"ruby_lambda_boundary", types.LangRuby, "def work\n hidden = -> { return \"hidden\" }\n return \"outer\"\nend", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"lua_anonymous_boundary", types.LangLua, "function work()\n local hidden = function() return \"hidden\" end\n return \"outer\"\nend", map[string][]string{"work": {`"outer"`}}, "explicit_return"},
		{"rust_match_assigned", types.LangRust, "fn work(x: bool) -> &'static str { let selected = match x { true => \"yes\", false => \"no\" }; return selected; }", map[string][]string{"work": {`selected`}}, "explicit_return"},
		{"go_multiline_values", types.LangGo, "package p\nfunc work() (string, error) {\n return (\n \"first\"), nil\n}\n", map[string][]string{"work": {"(\n \"first\"), nil"}}, "explicit_return"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, rows := b1692ParsePublic(t, tc.language, tc.source)
			got := make(map[string][]string)
			for _, r := range rows {
				got[r.CallableName] = append(got[r.CallableName], r.Expression)
				if tc.kind != "" && r.Kind != tc.kind {
					t.Errorf("kind=%s", r.Kind)
				}
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Logf("AST: %s", parseSourceFor(t, tc.language, tc.source).String())
				t.Fatalf("returns=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestB1692PublicCangjieReturnsHaveBoundedConsumedSyntax(t *testing.T) {
	for _, tc := range []struct {
		name, source string
		want         []string
	}{
		{"direct_call", "package p\nfunc work(): String { return transform(\"value\") }", []string{`transform("value")`}},
		{"nested_closure", "package p\nfunc work(): String { let hidden = { => return \"hidden\" }; return \"outer\" }", []string{`"outer"`}},
		{"comment_string", "package p\nfunc work(): String {\n let note = \"return hidden\"\n // return \"comment\"\n return \"outer\"\n}", []string{`"outer"`}},
		{"nested_branch_unavailable", "package p\nfunc work(): String { if (ready) { return \"nested\" } }", nil},
		{"bad_expression_group", "package p\nfunc work(): String { return broken) }", nil},
		{"continued_statement_unavailable", "package p\nfunc work(): String {\n return first\n + second\n}", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, rows := b1692ParsePublic(t, types.LangCangjie, tc.source)
			var got []string
			for _, r := range rows {
				got = append(got, r.Expression)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Cangjie receipts=%v want=%v", got, tc.want)
			}
		})
	}
}

func TestB1692PublicParserNoReceiptForDeclarationsAndAmbiguousOwners(t *testing.T) {
	cases := []struct{ lang, source string }{
		{types.LangJavaScript, "function same() { return 1; } function same() { return 2; }"},
		{types.LangTypeScript, "interface P { work(): string; }"},
		{types.LangProto, `syntax = "proto3"; service P { rpc Work (Req) returns (Resp); } message Req {} message Resp {}`},
		{types.LangRust, "fn work() -> i32 { return ; broken( }"},
	}
	for _, tc := range cases {
		t.Run(tc.lang, func(t *testing.T) {
			_, rows := b1692ParsePublic(t, tc.lang, tc.source)
			if len(rows) != 0 {
				t.Fatalf("unproved callable acquired return receipt: %+v", rows)
			}
		})
	}
}
