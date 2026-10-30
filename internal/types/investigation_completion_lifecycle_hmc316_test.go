package types

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestHMC316DocumentationCannotBorrowRetainedGeneration(t *testing.T) {
	m, rm := documentationCompletionState()
	m.AppendDispatchToolResult(m.StampToolDocumentationResult(documentationCompletionResult(t, "waits", 0)))
	sealDocumentationCompletion(t, m, &rm)
	fork := m.ForkForExploreDispatch()
	sealDocumentationCompletion(t, fork, &rm)
	nested := fork.ForkForExploreDispatch()
	sealDocumentationCompletion(t, nested, &rm)
	nested.ResetInvestigationComplete()
	fork.MergeExploreFork(nested)
	if fork.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("retained documentation borrowed an inherited live flag")
	}
	fork.AcceptToolDocumentationCompletion(&rm)
	if fork.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("stale live acceptance resealed documentation at the retained generation")
	}
	m.MergeExploreFork(fork)
	if m.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("nested merge revived documentation from a different live generation")
	}
	if !m.ToolDocumentationReady() {
		t.Fatal("retiring completion authority discarded published documentation")
	}
	m.ResetInvestigationComplete()
	if m.HasAcceptedToolDocumentationCompletion(&rm) {
		t.Fatal("documentation gained native handoff's retained-reset semantics")
	}
}

func TestHMC316LiveCompletionGenerationIsPrivateAndDecisionScoped(t *testing.T) {
	field, ok := reflect.TypeOf(MutableState{}).FieldByName("investigationLiveCompletionGeneration")
	if !ok || field.PkgPath == "" || field.Type.Kind() != reflect.Uint64 {
		t.Fatal("live ownership must remain a private exact generation")
	}
	m := NewMutableState("lifecycle")
	m.SetInvestigationComplete("live")
	if m.investigationLiveCompletionGeneration != 1 || m.InvestigationCompleteGeneration() != 1 {
		t.Fatal("live acceptance did not stamp its own decision")
	}
	fork := m.ForkForExploreDispatch()
	if fork.investigationLiveCompletionGeneration != 1 || fork.exploreForkCompletionGenerationBase != 1 {
		t.Fatal("fork did not preserve the live owner and acceptance baseline")
	}
	m.RecordExploreBacktrackExhausted("typed retained exhaustion")
	if m.InvestigationCompleteGeneration() != 2 || m.investigationLiveCompletionGeneration != 1 {
		t.Fatal("retained system exhaustion manufactured a live decision")
	}
	m.ResetInvestigationComplete()
	if m.InvestigationCompleteGeneration() != 2 || m.investigationLiveCompletionGeneration != 0 {
		t.Fatal("reset failed to separate accepted history from its live flag")
	}
	if !m.AcceptInvestigationCompleteWithBusinessSpanRef("next accepted model tail", nil) || m.investigationLiveCompletionGeneration != 3 {
		t.Fatal("accepted model tail failed to stamp its live decision")
	}
	b, err := json.Marshal(m)
	if err != nil || strings.Contains(string(b), "investigationLiveCompletionGeneration") {
		t.Fatalf("private ownership leaked: %s %v", b, err)
	}
	var replay MutableState
	if err := json.Unmarshal([]byte(`{"investigationLiveCompletionGeneration":99,"investigationCompleteGeneration":99}`), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.investigationLiveCompletionGeneration != 0 || replay.InvestigationCompleteGeneration() != 0 {
		t.Fatal("JSON restored completion ownership")
	}
}

func TestHMC316LiveCompletionGenerationWritersAreExplicit(t *testing.T) {
	const field = "investigationLiveCompletionGeneration"
	want := map[string]int{
		"context.go/ForkForExploreDispatch":                                      1,
		"context.go/MergeExploreFork":                                            1,
		"context.go/SetInvestigationComplete":                                    1,
		"context.go/ResetInvestigationComplete":                                  1,
		"trace_business_focus.go/AcceptInvestigationCompleteWithBusinessSpanRef": 1,
	}
	got := map[string]int{}
	files, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range files {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			key := entry.Name() + "/" + fn.Name.Name
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				switch node := n.(type) {
				case *ast.AssignStmt:
					for _, lhs := range node.Lhs {
						if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == field {
							got[key]++
						}
					}
				case *ast.IncDecStmt:
					if sel, ok := node.X.(*ast.SelectorExpr); ok && sel.Sel.Name == field {
						got[key]++
					}
				case *ast.KeyValueExpr:
					if id, ok := node.Key.(*ast.Ident); ok && id.Name == field {
						got[key]++
					}
				}
				return true
			})
		}
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("live ownership writers changed; classify the new lifecycle explicitly: got=%v want=%v", got, want)
	}
}
