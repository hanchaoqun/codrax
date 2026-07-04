package tool

// trace_note_keys_display_pin_test.go — NKR display-layer consumer pin
// (tool-side twin of internal/types/trace_note_keys_consumer_pin_test.go):
// the display/coverage helpers that re-parse trace_query rich notes must
// receive their key argument as a types.TraceNoteKey* constant, never as a
// bare string literal. A bare literal is exactly the wire-typo shape that
// silently drops a metric snapshot / next-step line (§7.4 / F-2 failure
// class). Test files keep verbatim literals on purpose.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

var traceNoteKeyDisplayParseHelpers = map[string][]int{
	"runtimeTraceObservationRichNoteValue": {1},
	"traceObservationNoteValue":            {1},
	"runtimeTraceProjSupplyNoteFloat":      {1},
	// Evaluator-side prefix consumer: the key argument is a "<key>=" prefix,
	// built as types.TraceNoteKey* + "=" (a BinaryExpr — the pin only rejects
	// a bare string literal in that slot).
	"traceQueryObservationHasRichNotePrefix": {1},
}

// traceNoteKeyDisplayPinFiles: every display-layer consumer file that
// re-parses trace_query rich notes, including the evaluator supplement lane
// in internal/agent (paths are relative to this package directory).
var traceNoteKeyDisplayPinFiles = []string{
	"answer_document_mutation_runtime.go",
	"emit_investigation_complete.go",
	"../agent/answer_document_evaluator.go",
}

func TestTraceNoteKeyDisplayCallSitesUseConstants(t *testing.T) {
	fset := token.NewFileSet()
	for _, name := range traceNoteKeyDisplayPinFiles {
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		matched := 0
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			fn, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			keyArgs, ok := traceNoteKeyDisplayParseHelpers[fn.Name]
			if !ok {
				return true
			}
			matched++
			for _, idx := range keyArgs {
				if idx >= len(call.Args) {
					continue
				}
				if lit, isLit := call.Args[idx].(*ast.BasicLit); isLit {
					t.Errorf("%s: %s called with bare key literal %s — use the types.TraceNoteKey* constant (trace_note_keys.go change protocol)",
						fset.Position(lit.Pos()), fn.Name, lit.Value)
				}
			}
			return true
		})
		if matched == 0 {
			t.Fatalf("%s: no note-parse helper call sites matched — the pin is checking nothing; update traceNoteKeyDisplayParseHelpers alongside the refactor", name)
		}
	}
}
