package dataworkflow

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/dataquery"
)

// output_contract_declaration_chain_test.go — batch-six fold-in #8
// (colleague_merge_audit §40.56 收编复核再收编): the system-built plan
// builders fold DECLARATIONS only. A Result.OutputContract is an execution
// echo — a batch without assemble_answer adopts the seed answer and the seed
// contract (internal/dataquery/action_runner.go) — so on 381f36cc9 the
// projection builder, the projection-need predicate and the ledger-completion
// base plan resolved (Current, Output, Result.OutputContract) with the echo
// last/highest, and a json_only echo under an explicitly revised
// plain_single_line declaration drove the validator proposal (and through it
// the CLI resume) back to json_only.

func declarationChainContracts() (plain, jsonOnly dataquery.OutputContract) {
	plain = dataquery.OutputContract{
		Format:            dataquery.OutputPlainSingleLine,
		CompleteReference: true,
		ReferencePath:     "targets.csv",
		ReferenceKeyField: "canonical_label",
	}
	jsonOnly = plain
	jsonOnly.Format = dataquery.OutputJSONOnly
	return plain, jsonOnly
}

func declarationChainResult(echo dataquery.OutputContract) dataquery.Result {
	return dataquery.Result{
		Answer:         `{"GroupA":17,"GroupX":4,"GroupC":5}`,
		OutputContract: echo,
		Reconcile: &dataquery.ReconcileReport{
			Status: dataquery.LooseText("pass"),
			Groups: []dataquery.ReconcileGroup{{GroupKey: dataquery.LooseText("GroupA"), Metric: dataquery.LooseText("total_value"), Expected: dataquery.LooseText("17"), Actual: dataquery.LooseText("17")}},
		},
	}
}

// TestSystemBuiltPlansReadTheDeclaredContractNotTheResultEcho: with the
// declared (fold) contract plain_single_line and the latest Result echoing
// json_only at equal specificity, every builder that mints a plan contract
// carries the declaration.
func TestSystemBuiltPlansReadTheDeclaredContractNotTheResultEcho(t *testing.T) {
	plain, jsonOnly := declarationChainContracts()
	current := dataquery.TaskPlan{Status: "complete", Goal: "project per-target totals", OutputContract: plain}
	result := declarationChainResult(jsonOnly)
	coverage := dataquery.CoverageContract{ReconcileRequired: true}
	want := plain.Normalize()

	projection, ok := BuildRequiredOutputProjectionPlan(OutputProjectionPlanInput{
		Current:  current,
		Coverage: coverage,
		Output:   plain,
		Result:   result,
	})
	if !ok {
		t.Fatal("projection plan not built")
	}
	if projection.OutputContract != want {
		t.Fatalf("projection plan contract=%+v, want the declared %+v (Result echo outranked the declaration)", projection.OutputContract, want)
	}
	base := requiredLedgerCompletionBasePlan(RequiredLedgerCompletionPlanInput{
		Current:  current,
		Coverage: coverage,
		Output:   plain,
		Result:   result,
	})
	if base.OutputContract != want {
		t.Fatalf("ledger completion base plan contract=%+v, want the declared %+v", base.OutputContract, want)
	}
	// The need predicate classifies under the declaration: an undeclared
	// chain with only an echo is no declaration at all.
	if ResultNeedsOutputProjection(ResultProjectionNeedInput{Current: dataquery.TaskPlan{}, Coverage: coverage, Result: result}) {
		t.Fatal("projection need was decided from a Result echo with no declaration in the chain")
	}
	if !ResultNeedsOutputProjection(ResultProjectionNeedInput{Current: current, Coverage: coverage, Output: plain, Result: result}) {
		t.Fatal("projection need lost under the declared contract")
	}
}

// TestResolveOutputContractCallersFoldDeclarationsOnly is the structural
// pin: no ResolveOutputContract argument in this package's non-test files
// reaches through a `Result` selector segment. Self-red through the real
// parser on the 381f36cc9 shape.
func TestResolveOutputContractCallersFoldDeclarationsOnly(t *testing.T) {
	fset := token.NewFileSet()
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]*ast.File{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files[name] = file
	}
	problems, calls := resolveOutputContractEchoArguments(files, fset)
	for _, problem := range problems {
		t.Error(problem)
	}
	if calls < 3 {
		t.Fatalf("found only %d ResolveOutputContract calls; the walk drifted", calls)
	}
	probe, err := parser.ParseFile(fset, "zz_probe.go", `package dataworkflow
func zz(input OutputProjectionPlanInput) {
	_ = ResolveOutputContract(input.Current.OutputContract, input.Output, input.Result.OutputContract)
	_ = ResolveOutputContract(input.Current.OutputContract, input.Output)
}
`, 0)
	if err != nil {
		t.Fatal(err)
	}
	problems, _ = resolveOutputContractEchoArguments(map[string]*ast.File{"zz_probe.go": probe}, fset)
	if len(problems) != 1 || problems[0] != "zz_probe.go:3: ResolveOutputContract folds input.Result.OutputContract — a Result.OutputContract is an execution echo, never a declaration" {
		t.Fatalf("self-red: problems=%v", problems)
	}
}

func resolveOutputContractEchoArguments(files map[string]*ast.File, fset *token.FileSet) (problems []string, calls int) {
	for name, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); !ok || ident.Name != "ResolveOutputContract" {
				return true
			}
			calls++
			for _, arg := range call.Args {
				for expr := arg; ; {
					sel, ok := expr.(*ast.SelectorExpr)
					if !ok {
						break
					}
					if sel.Sel.Name == "Result" {
						problems = append(problems, name+":"+strconv.Itoa(fset.Position(call.Pos()).Line)+": ResolveOutputContract folds "+selectorText(arg)+" — a Result.OutputContract is an execution echo, never a declaration")
						break
					}
					expr = sel.X
				}
			}
			return true
		})
	}
	return problems, calls
}

func selectorText(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return selectorText(e.X) + "." + e.Sel.Name
	}
	return "?"
}
