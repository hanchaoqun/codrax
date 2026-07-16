package tracequery

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

func TestIsTraceMarkPayloadParityAndSingleAuthority(t *testing.T) {
	for _, payload := range []string{
		"E",
		"B|42|frame",
		"C|42|queue_depth|3",
		"0xffffffc010123abc: S|42|async|7",
		"hello world",
		"B",
		"X|42|not-a-mark",
	} {
		if got, want := IsTraceMarkPayload(payload), isTraceMarkPayload(payload); got != want {
			t.Fatalf("exported trace-mark classifier parity for %q: got %v want %v", payload, got, want)
		}
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "parse.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "IsTraceMarkPayload" {
			continue
		}
		if len(fn.Body.List) != 1 {
			t.Fatalf("IsTraceMarkPayload must remain one direct delegation, statements=%d", len(fn.Body.List))
		}
		ret, ok := fn.Body.List[0].(*ast.ReturnStmt)
		if !ok || len(ret.Results) != 1 {
			t.Fatalf("IsTraceMarkPayload must return the canonical classifier directly: %#v", fn.Body.List[0])
		}
		call, ok := ret.Results[0].(*ast.CallExpr)
		if !ok || len(call.Args) != 1 {
			t.Fatalf("IsTraceMarkPayload must call the canonical classifier exactly once: %#v", ret.Results[0])
		}
		callee, calleeOK := call.Fun.(*ast.Ident)
		arg, argOK := call.Args[0].(*ast.Ident)
		if !calleeOK || callee.Name != "isTraceMarkPayload" || !argOK || arg.Name != "fields" {
			t.Fatalf("IsTraceMarkPayload copied or bypassed the canonical grammar: %#v", ret.Results[0])
		}
		return
	}
	t.Fatal("IsTraceMarkPayload wrapper is missing")
}
