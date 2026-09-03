package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/types"
)

// verification_confidence_witness_census_test.go — V5-1 structural tripwire
// (colleague_merge_audit §40.10 item 4): every VerificationConfidenceRecord
// producer in this package that can mint a satisfied contract-lane
// observation (a literal carrying ContractRefs whose Status is not a literal
// non-satisfied status) MUST stamp WitnessKind, and the stamp must be the one
// witness its literal Category can carry (the types-level table). Producers
// whose Category is not a literal cannot be judged and are reported.

var verificationConfidenceWitnessConstants = map[string]types.WriteBehaviorWitnessKind{
	"WriteBehaviorWitnessVerificationProbe": types.WriteBehaviorWitnessVerificationProbe,
	"WriteBehaviorWitnessProjectTest":       types.WriteBehaviorWitnessProjectTest,
	"WriteBehaviorWitnessSourceText":        types.WriteBehaviorWitnessSourceText,
}

func verificationConfidenceWitnessCensus(files map[string]string) (offenders []string, checked int, err error) {
	fset := token.NewFileSet()
	for name, src := range files {
		file, perr := parser.ParseFile(fset, name, src, 0)
		if perr != nil {
			return nil, 0, perr
		}
		// Elided-type element literals inside []types.VerificationConfidenceRecord{...}
		// are records too: collect them first so the walk below can judge them.
		elided := map[*ast.CompositeLit]bool{}
		ast.Inspect(file, func(n ast.Node) bool {
			outer, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			arr, ok := outer.Type.(*ast.ArrayType)
			if !ok {
				return true
			}
			if sel, ok := arr.Elt.(*ast.SelectorExpr); !ok || sel.Sel.Name != "VerificationConfidenceRecord" {
				return true
			}
			for _, elt := range outer.Elts {
				if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
					elided[inner] = true
				}
			}
			return true
		})
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			if !elided[lit] {
				sel, ok := lit.Type.(*ast.SelectorExpr)
				if !ok || sel.Sel.Name != "VerificationConfidenceRecord" {
					return true
				}
			}
			var statusLit, categoryLit, witnessName string
			var hasRefs, hasStatus, hasCategory, hasWitness bool
			for _, elt := range lit.Elts {
				kv, ok := elt.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok {
					continue
				}
				switch key.Name {
				case "ContractRefs":
					hasRefs = true
				case "Status":
					hasStatus = true
					if b, ok := kv.Value.(*ast.BasicLit); ok && b.Kind == token.STRING {
						statusLit, _ = strconv.Unquote(b.Value)
					}
				case "Category":
					hasCategory = true
					if b, ok := kv.Value.(*ast.BasicLit); ok && b.Kind == token.STRING {
						categoryLit, _ = strconv.Unquote(b.Value)
					}
				case "WitnessKind":
					hasWitness = true
					if s, ok := kv.Value.(*ast.SelectorExpr); ok {
						witnessName = s.Sel.Name
					}
				}
			}
			if !hasStatus || !hasCategory {
				return true
			}
			switch statusLit {
			case "missing", "unavailable", "failed", "error", "advisory":
				return true // never a satisfied observation
			}
			where := name + ":" + strconv.Itoa(fset.Position(lit.Pos()).Line)
			if categoryLit == "" {
				checked++
				offenders = append(offenders, where+" mints a possibly-satisfied record with a non-literal Category (census cannot judge it)")
				return true
			}
			want, known := types.WriteBehaviorWitnessKindForConfidenceCategory(categoryLit)
			if !known {
				if hasRefs {
					checked++
					offenders = append(offenders, where+" mints a possibly-satisfied record with ContractRefs on category "+categoryLit+" which carries no contract witness")
				}
				return true // non-contract lanes (probe_execution, changed symbols, …) carry no witness
			}
			checked++
			if !hasWitness {
				offenders = append(offenders, where+" mints a possibly-satisfied "+categoryLit+" record without WitnessKind")
				return true
			}
			got, ok := verificationConfidenceWitnessConstants[witnessName]
			if !ok || got != want {
				offenders = append(offenders, where+" stamps "+witnessName+" on category "+categoryLit+" (matrix says "+string(want)+")")
			}
			return true
		})
	}
	return offenders, checked, nil
}

func TestSatisfiedContractConfidenceProducersCarryMatrixWitnessKind(t *testing.T) {
	files := map[string]string{}
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		body, rerr := os.ReadFile(e.Name())
		if rerr != nil {
			t.Fatal(rerr)
		}
		files[e.Name()] = string(body)
	}
	offenders, checked, err := verificationConfidenceWitnessCensus(files)
	if err != nil {
		t.Fatalf("census parse failed (a silent green would defeat the tripwire): %v", err)
	}
	if len(offenders) > 0 {
		t.Fatalf("witness matrix violated: %v", offenders)
	}
	if checked < 5 {
		t.Fatalf("census judged only %d possibly-satisfied producers; expected the four probe/project-test producers and the source arm", checked)
	}
	// Self-red: the source arm's satisfied record without its stamp must be flagged.
	const stamped = "ContractRefs: []string{id},\n\t\t\t\tWitnessKind:  types.WriteBehaviorWitnessSourceText,\n\t\t\t\tDetail:       detail,"
	src := files["run_tests_source_contract.go"]
	mutated := strings.Replace(src, stamped, "ContractRefs: []string{id},\n\t\t\t\tDetail:       detail,", 1)
	if mutated == src {
		t.Fatal("self-red fixture did not find the source arm's stamped satisfied record")
	}
	offenders, _, err = verificationConfidenceWitnessCensus(map[string]string{"run_tests_source_contract.go": mutated})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range offenders {
		found = found || strings.Contains(o, "run_tests_source_contract.go") && strings.Contains(o, "without WitnessKind")
	}
	if !found {
		t.Fatalf("census must flag the unstamped source arm, got %v", offenders)
	}
	// Self-red: the package's slice-literal idiom (elided element type) and a
	// producer whose refs are assigned after construction are judged too.
	probe := `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func a(report *types.ChangeReport, refs []string) {
	report.VerificationConfidence = mergeVerificationConfidenceRecords(report.VerificationConfidence, []types.VerificationConfidenceRecord{{
		Source: "lint", Category: "project_test_contract_refs", Status: "satisfied", ContractRefs: refs,
	}})
	rec := types.VerificationConfidenceRecord{Source: "x", Category: "source_contract_refs", Status: "satisfied"}
	rec.ContractRefs = refs
	_ = rec
}
`
	offenders, checked, err = verificationConfidenceWitnessCensus(map[string]string{"probe.go": probe})
	if err != nil {
		t.Fatal(err)
	}
	if checked != 2 || len(offenders) != 2 {
		t.Fatalf("census must judge the elided slice literal and the late-assigned producer: checked=%d offenders=%v", checked, offenders)
	}
}

// R2' same-source teaching: the analyzer schema's kind description is the
// types-level matrix sentence (the skill half is pinned in internal/skill).
func TestWriteAnalysisKindSchemaTeachesTheWitnessMatrixFromTheSameSource(t *testing.T) {
	params := string((&EmitWriteAnalysis{}).Parameters())
	teaching := types.WriteBehaviorContractKindWitnessTeaching()
	if !strings.Contains(params, teaching) {
		t.Fatalf("emit_write_analysis kind description must carry the matrix teaching verbatim:\n%s", teaching)
	}
}
