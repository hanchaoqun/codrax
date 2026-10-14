package tool

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"strings"
	"testing"
)

// The existing roster census proves the accessor is frozen and its bound
// local cannot be rebound. This companion pins the one cross-file delegation
// before the Trace early return, and keeps the delegate a read-only consumer.
func runtimeParticipantFrozenDelegationOffenders(gateSource, helperSource string) []string {
	var offenders []string
	fs := token.NewFileSet()
	gate, err := parser.ParseFile(fs, "emit_analysis.go", gateSource, 0)
	if err != nil {
		return []string{err.Error()}
	}
	printed := func(node ast.Node) string {
		var b strings.Builder
		_ = printer.Fprint(&b, fs, node)
		return b.String()
	}
	found := false
	for _, decl := range gate.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "validateRequiredFlowDiagramParticipantProvenance" {
			continue
		}
		found = true
		if fn.Body == nil || len(fn.Body.List) < 3 || printed(fn.Body.List[0]) != "entities := roster.Entities()" {
			offenders = append(offenders, "frozen accessor must precede the runtime delegation")
			continue
		}
		guard, ok := fn.Body.List[1].(*ast.IfStmt)
		if !ok || guard.Init == nil || printed(guard.Init) != "conflict := validateRuntimeParticipantBindingRoster(rm, entities)" ||
			printed(guard.Cond) != `conflict != ""` || len(guard.Body.List) != 1 || printed(guard.Body.List[0]) != "return conflict" {
			offenders = append(offenders, "runtime delegation must consume the frozen copy before any early exit")
		}
	}
	if !found {
		offenders = append(offenders, "frozen gate disappeared")
	}
	helper, err := parser.ParseFile(fs, "emit_analysis_runtime_participant_provenance.go", helperSource, 0)
	if err != nil {
		return append(offenders, err.Error())
	}
	ast.Inspect(helper, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.SelectorExpr:
			switch n.Sel.Name {
			case "AnalyzerHints", "Entities", "PrimaryEntities", "MentionedEntities", "DerivedEntities", "RawRequest":
				offenders = append(offenders, "runtime helper rereads a non-frozen roster or request text")
			}
		case *ast.AssignStmt:
			for _, lhs := range n.Lhs {
				if printed(lhs) == "entities" || strings.HasPrefix(printed(lhs), "entities[") {
					offenders = append(offenders, "runtime helper mutates its frozen copy")
				}
			}
		case *ast.CallExpr:
			for _, arg := range n.Args {
				if printed(arg) == "entities" || printed(arg) == "&entities" {
					offenders = append(offenders, "runtime helper delegates its roster outside the reviewed consumer")
				}
			}
		}
		return true
	})
	return offenders
}

func TestHMC242RuntimeParticipantFrozenDelegationCensus(t *testing.T) {
	gate, err := os.ReadFile("emit_analysis.go")
	if err != nil {
		t.Fatal(err)
	}
	helper, err := os.ReadFile("emit_analysis_runtime_participant_provenance.go")
	if err != nil {
		t.Fatal(err)
	}
	if got := runtimeParticipantFrozenDelegationOffenders(string(gate), string(helper)); len(got) != 0 {
		t.Fatalf("frozen runtime delegation: %v", got)
	}
	for _, field := range []string{"Entities", "PrimaryEntities"} {
		changed := strings.Replace(string(gate), "validateRuntimeParticipantBindingRoster(rm, entities)", "validateRuntimeParticipantBindingRoster(rm, rm.AnalyzerHints."+field+")", 1)
		if changed == string(gate) || len(runtimeParticipantFrozenDelegationOffenders(changed, string(helper))) == 0 {
			t.Fatalf("mutable %s delegate self-red escaped", field)
		}
	}
	for name, mutation := range map[string]string{
		"mutable reread": "entities = rm.AnalyzerHints.Entities",
		"slice mutation": "entities[0] = \"observer\"",
		"escaped roster": "other(entities)",
		"raw request":    "_ = rm.RawRequest",
	} {
		changed := strings.Replace(string(helper), "if rm.DiagramHint == nil", mutation+"\n\tif rm.DiagramHint == nil", 1)
		if changed == string(helper) || len(runtimeParticipantFrozenDelegationOffenders(string(gate), changed)) == 0 {
			t.Fatalf("%s self-red escaped", name)
		}
	}
	late := strings.Replace(string(gate), "entities := roster.Entities()\n\tif conflict := validateRuntimeParticipantBindingRoster", "entities := roster.Entities()\n\tif rm.Intent == types.IntentTrace { return \"\" }\n\tif conflict := validateRuntimeParticipantBindingRoster", 1)
	if late == string(gate) || len(runtimeParticipantFrozenDelegationOffenders(late, string(helper))) == 0 {
		t.Fatal("Trace early-return bypass self-red escaped")
	}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		body, err := os.ReadFile(entry.Name())
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), entry.Name(), body, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok {
					return true
				}
				name, ok := call.Fun.(*ast.Ident)
				if !ok || name.Name != "validateRuntimeParticipantBindingRoster" {
					return true
				}
				calls++
				if entry.Name() != "emit_analysis.go" || fn.Name.Name != "validateRequiredFlowDiagramParticipantProvenance" {
					t.Errorf("unreviewed runtime roster caller: %s:%s", entry.Name(), fn.Name.Name)
				}
				return true
			})
		}
	}
	if calls != 1 {
		t.Fatalf("runtime roster must have exactly one frozen caller: %d", calls)
	}
}
