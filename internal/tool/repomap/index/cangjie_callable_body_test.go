package index

import (
	"testing"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

func TestCangjieCallableBodyPresenceFromConsumedSyntax(t *testing.T) {
	tests := []struct {
		name, source, symbol string
		want                 types.CallableBodyPresence
	}{
		{"single-line", "package demo\nfunc run() { return 1 }", "run", types.CallableBodyPresent},
		{"multiline", "package demo\nfunc run(x: Int64): Int64 {\n return x\n}", "run", types.CallableBodyPresent},
		{"generic", "package demo\nfunc run<T>(x: T): T { return x }", "run", types.CallableBodyPresent},
		{"method", "package demo\nclass App { func run() {} }", "run", types.CallableBodyPresent},
		{"ctor", "package demo\nclass App { init() {} }", "init", types.CallableBodyPresent},
		{"operator", "package demo\nclass App { operator func +(x: App): App { return x } }", "operator +", types.CallableBodyPresent},
		{"main", "package demo\nmain(): Int64 { return 0 }", "main", types.CallableBodyPresent},
		{"interface", "package demo\ninterface App { func run(): Int64 }", "run", types.CallableBodyAbsent},
		{"abstract", "package demo\nabstract class App { public abstract func run(): Int64; }", "run", types.CallableBodyAbsent},
		{"foreign", "package demo\nforeign func run(x: Int64): Int64", "run", types.CallableBodyAbsent},
		{"foreign-semi", "package demo\nforeign func run();", "run", types.CallableBodyAbsent},
		{"foreign-body-invalid", "package demo\nforeign func run() {}", "run", types.CallableBodyUnknown},
		{"foreign-return-incomplete", "package demo\nforeign func run(): List<Int64", "run", types.CallableBodyUnknown},
		{"return-empty", "package demo\nfunc run(): {}", "run", types.CallableBodyUnknown},
		{"no-body-unknown", "package demo\nfunc run()", "run", types.CallableBodyUnknown},
		{"unclosed-body", "package demo\nfunc run() {\n return 1", "run", types.CallableBodyUnknown},
		{"unclosed-params", "package demo\nfunc run(x: Int64 { return 1 }", "run", types.CallableBodyUnknown},
		{"missing-params", "package demo\nfunc run { return 1 }", "run", types.CallableBodyUnknown},
		{"unclosed-delimiter", "package demo\nfunc run() { work( }", "run", types.CallableBodyUnknown},
		{"declaration-cannot-borrow-next-body", "package demo\ninterface App { func run(): Int64\n func work() {} }", "run", types.CallableBodyUnknown},
		{"non-callable", "package demo\nclass App {}", "App", types.CallableBodyUnknown},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, symbols, _, _ := parseCangjie([]byte(tt.source), "app.cj")
			for _, sym := range symbols {
				if sym.Name == tt.symbol {
					if sym.BodyPresence != tt.want {
						t.Fatalf("%s: body=%q want=%q, sym=%+v", tt.source, sym.BodyPresence, tt.want, sym)
					}
					if sym.HasParserOwnedBody() != (tt.want == types.CallableBodyPresent) {
						t.Fatalf("body extent mismatch: %+v", sym)
					}
					if tt.want != types.CallableBodyPresent && (sym.BodyStartLine != 0 || sym.BodyEndLine != 0) {
						t.Fatalf("unproved body has extent: %+v", sym)
					}
					return
				}
			}
			t.Fatalf("missing symbol %q: %+v", tt.symbol, symbols)
		})
	}
}

func TestCangjieCallableBodyExtentExcludesMultilineSignature(t *testing.T) {
	_, symbols, _, _ := parseCangjie([]byte("package demo\nfunc run(\n x: Int64\n): Int64\n{\n return x\n}\n"), "app.cj")
	if len(symbols) != 1 {
		t.Fatalf("unexpected symbols: %+v", symbols)
	}
	sym := symbols[0]
	if sym.Line != 2 || sym.EndLine != 7 || sym.BodyStartLine != 5 || sym.BodyEndLine != 7 || !sym.HasParserOwnedBody() {
		t.Fatalf("signature became body: %+v", sym)
	}
}
