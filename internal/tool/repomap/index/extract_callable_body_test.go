package index

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func parseCallableBodyFixture(t *testing.T, language, source string) *types.FileInfo {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	return parseOneFile(FileEntry{AbsPath: path, RelPath: "fixture", Language: language, Size: int64(len(source))})
}

func TestCallableBodyPresenceActualParserLanguageMatrix(t *testing.T) {
	cases := []struct{ language, source, name string }{
		{types.LangGo, "package p\nfunc work() {}\n", "work"},
		{types.LangPython, "def work():\n    pass\n", "work"},
		{types.LangJavaScript, "function work() {}\n", "work"},
		{types.LangTypeScript, "function work(): void {}\n", "work"},
		{types.LangArkTS, "function work(): void {}\n", "work"},
		{types.LangJava, "class P { void work() {} }\n", "work"},
		{types.LangKotlin, "fun work() {}\n", "work"},
		{types.LangSwift, "func work() {}\n", "work"},
		{types.LangC, "void work(void) {}\n", "work"},
		{types.LangCpp, "void work() {}\n", "work"},
		{types.LangRust, "fn work() {}\n", "work"},
		{types.LangRuby, "def work\nend\n", "work"},
		{types.LangLua, "function work()\nend\n", "work"},
		{types.LangCangjie, "package p\nfunc work(): Unit {}\n", "work"},
	}
	for _, tc := range cases {
		t.Run(tc.language, func(t *testing.T) {
			fi := parseCallableBodyFixture(t, tc.language, tc.source)
			for _, sym := range fi.Symbols {
				if sym.Name == tc.name {
					if !sym.HasParserOwnedBody() {
						t.Fatalf("actual parser lost a real (possibly empty) body: %+v; source=%s", sym, tc.source)
					}
					return
				}
			}
			t.Fatalf("actual parser did not publish callable %q: %+v", tc.name, fi.Symbols)
		})
	}
	// Protocol RPCs are navigation declarations, not executable functions.
	fi := parseCallableBodyFixture(t, types.LangProto, "syntax = \"proto3\"; service P { rpc Work (Req) returns (Resp); } message Req {} message Resp {}")
	for _, sym := range fi.Symbols {
		if sym.HasParserOwnedBody() {
			t.Fatalf("protobuf declaration acquired executable authority: %+v", sym)
		}
	}
}

func TestCallableBodyPresenceSeparatesDeclarationsFromImplementations(t *testing.T) {
	cases := []struct{ name, language, source, absent, present string }{
		{"typescript_interface", types.LangTypeScript, "interface Transport { send(): void; }\nclass HttpTransport { send(): void {} }", "Transport.send", "HttpTransport.send"},
		{"arkts_interface", types.LangArkTS, "interface Transport { send(): void; }\nclass HttpTransport { send(): void {} }", "Transport.send", "HttpTransport.send"},
		{"java_interface_default", types.LangJava, "interface P { void empty(); default void actual() {} }", "P.empty", "P.actual"},
		{"java_abstract_native", types.LangJava, "abstract class P { abstract void empty(); native void foreign(); void actual() {} }", "P.empty", "P.actual"},
		{"c_prototype", types.LangC, "void empty(void);\nvoid actual(void) {}", "empty", "actual"},
		{"cpp_pure_virtual", types.LangCpp, "class P { virtual void empty() = 0; void actual() {} };", "P.empty", "P.actual"},
		{"rust_trait_default", types.LangRust, "trait P { fn empty(); fn actual() {} }", "P.empty", "P.actual"},
		{"kotlin_abstract", types.LangKotlin, "abstract class P { abstract fun empty()\nfun actual() {} }", "P.empty", "P.actual"},
		{"swift_protocol", types.LangSwift, "protocol P { func empty() }\nfunc actual() {}", "P.empty", "actual"},
		{"go_external", types.LangGo, "package p\nfunc empty()\nfunc actual() {}", "empty", "actual"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fi := parseCallableBodyFixture(t, tc.language, tc.source)
			foundAbsent, foundPresent := false, false
			for _, sym := range fi.Symbols {
				name := sym.Name
				if sym.Parent != "" {
					name = sym.Parent + "." + name
				}
				if name == tc.absent {
					foundAbsent = true
					if sym.BodyPresence != types.CallableBodyAbsent {
						t.Errorf("declaration state=%q, symbol=%+v; tree=%s", sym.BodyPresence, sym, parseSourceFor(t, tc.language, tc.source).String())
					}
				}
				if name == tc.present {
					foundPresent = true
					if !sym.HasParserOwnedBody() {
						t.Errorf("real body state=%q, symbol=%+v", sym.BodyPresence, sym)
					}
				}
			}
			if !foundAbsent || !foundPresent {
				t.Fatalf("fixture did not exercise both parser surfaces: %+v", fi.Symbols)
			}
		})
	}
}

func TestCallableBodyPresenceParseErrorAndSameLineAmbiguityStayUnknown(t *testing.T) {
	for _, source := range []string{"function work() {", "function work(): void { return ; @@@ }"} {
		fi := parseCallableBodyFixture(t, types.LangTypeScript, source)
		for _, sym := range fi.Symbols {
			if sym.BodyPresence != types.CallableBodyUnknown {
				t.Fatalf("broken callable became precise: %+v", sym)
			}
		}
	}
	source := "function work() {} function work() {}"
	fi := parseCallableBodyFixture(t, types.LangTypeScript, source)
	for _, sym := range fi.Symbols {
		if sym.BodyPresence != types.CallableBodyUnknown {
			t.Fatalf("same name and range could not uniquely own AST body: %+v", sym)
		}
	}
}

func TestCallableBodyPresenceExpressionBodiesAndSignatureOverloads(t *testing.T) {
	for _, tc := range []struct{ language, source string }{
		{types.LangTypeScript, "const work = () => 1;"},
		{types.LangJavaScript, "const work = () => 1;"},
		{types.LangKotlin, "fun work() = 1"},
	} {
		fi := parseCallableBodyFixture(t, tc.language, tc.source)
		if len(fi.Symbols) == 0 {
			t.Fatalf("missing expression callable for %s", tc.language)
		}
		for _, sym := range fi.Symbols {
			if sym.Name == "work" && !sym.HasParserOwnedBody() {
				t.Fatalf("expression body %s: %+v", tc.language, sym)
			}
		}
	}
	// The current TS symbol extractor omits overload signature symbols. Probe
	// the body annotator independently to keep that omission from authorizing
	// an overload signature as executable when symbol extraction expands.
	source := "function work(x: string): string;\nfunction work(x: string) { return x; }"
	root := parseSourceFor(t, types.LangTypeScript, source)
	syms := []types.Symbol{{Name: "work", Kind: "function", Line: 1, EndLine: 1}, {Name: "work", Kind: "function", Line: 2, EndLine: 2}}
	backfillCallableBodyPresence(root, []byte(source), types.LangTypeScript, syms)
	if syms[0].BodyPresence != types.CallableBodyAbsent || !syms[1].HasParserOwnedBody() {
		t.Fatalf("overload signature/implementation conflated: %+v", syms)
	}
}

func TestCallableBodyPresenceArkTSReplacementPreservesOnlyExactParserIdentity(t *testing.T) {
	parsed := types.Symbol{Name: "build", Kind: "function", Line: 4, EndLine: 9, BodyPresence: types.CallableBodyPresent, BodyStartLine: 6, BodyEndLine: 9}
	for _, tc := range []struct {
		name        string
		replacement types.Symbol
		want        types.CallableBodyPresence
	}{
		{"exact", types.Symbol{Name: "build", Kind: "builder", Line: 4, EndLine: 9}, types.CallableBodyPresent},
		{"different_extent", types.Symbol{Name: "build", Kind: "builder", Line: 4, EndLine: 4}, types.CallableBodyUnknown},
		{"different_owner", types.Symbol{Name: "build", Kind: "builder", Parent: "Other", Line: 4, EndLine: 9}, types.CallableBodyUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mergeArkTSSymbols([]types.Symbol{parsed}, []types.Symbol{tc.replacement})
			if got[0].BodyPresence != tc.want || (tc.want == types.CallableBodyPresent && (got[0].BodyStartLine != 6 || got[0].BodyEndLine != 9)) {
				t.Fatalf("replacement=%+v", got)
			}
		})
	}
	symbols, _ := arkTSPostPass([]byte("@Component struct P { build() { Text('hello') } }"), "p.ets")
	for _, sym := range symbols {
		if sym.BodyPresence != types.CallableBodyUnknown {
			t.Fatalf("regex post-pass minted parser presence: %+v", sym)
		}
	}
}

func TestCallableBodyPresenceMultilineParameterExtent(t *testing.T) {
	for _, tc := range []struct {
		language, source string
		start, end       int
	}{
		{types.LangTypeScript, "function work(\n  x: string\n): string {\n  return x;\n}\n", 3, 5},
		{types.LangJava, "class P {\n  String work(\n    String x\n  ) {\n    return x;\n  }\n}\n", 4, 6},
		{types.LangC, "int work(\n  int x\n) {\n  return x;\n}\n", 3, 5},
		{types.LangPython, "def work(\n    x\n):\n    return x\n", 4, 4},
		{types.LangRuby, "def work(\n  x\n)\n  x\nend\n", 4, 4},
		{types.LangLua, "function work(\n  x\n)\n  return x\nend\n", 4, 4},
	} {
		t.Run(tc.language, func(t *testing.T) {
			fi := parseCallableBodyFixture(t, tc.language, tc.source)
			for _, sym := range fi.Symbols {
				if sym.Name != "work" {
					continue
				}
				if !sym.HasParserOwnedBody() || sym.BodyStartLine != tc.start || sym.BodyEndLine != tc.end {
					t.Fatalf("signature/body extent conflated: %+v; tree=%s", sym, parseSourceFor(t, tc.language, tc.source).String())
				}
				return
			}
			t.Fatalf("callable absent: %+v", fi.Symbols)
		})
	}
}
