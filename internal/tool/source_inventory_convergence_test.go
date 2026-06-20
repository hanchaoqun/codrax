package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// source_inventory_convergence_test.go is the STRUCTURAL CONVERGENCE TRIPWIRE
// for the source-inventory subsystem (2026-06-20 convergence-target spec). The
// subsystem had become a per-SHAPE-not-per-CLASS hot-spot: each eval failure
// was patched by appending another guard to the 5,236-LOC reconcile.go god-file
// instead of consolidating onto the existing kernels, so the same CLASS of
// failure re-opened under a new label every sweep. These ratchets convert that
// treadmill into a monotonic ratchet: the build fails when a change GROWS the
// god-file, ADDS an unbudgeted kernel bypass, or RE-DERIVES a source-class
// taxonomy instead of using types.ClassifySourcePathRole. Each clause reads a
// PRECISE structural signal (line counts, typed AST node shapes) — never a
// noisy heuristic — so it cannot false-fire on a structurally fine change,
// consistent with the project's precise-signals-for-hard-gates red line.

// sourceInventoryFileLOCCeiling pins each cluster file at its current size.
// Ceilings may only RATCHET DOWN as the subsystem is decomposed (the
// convergence target is reconcile.go <= 1500). A file that grows past its
// ceiling fails the build: extract code into a concern sub-file / the bounded
// execution kernel instead of accreting here. A NEW source_inventory_*.go file
// gets the strict default ceiling so the cluster cannot grow a fresh god-file.
var sourceInventoryFileLOCCeiling = map[string]int{
	"source_inventory_reconcile.go":         5236,
	"source_inventory_observation.go":       645,
	"source_inventory_universe_coverage.go": 644,
	"source_inventory_profile.go":           229,
	"source_inventory_advisory.go":          223,
	"source_inventory_exec_budget.go":       141,
	"source_inventory_scope.go":             117,
}

const sourceInventoryNewFileLOCCeiling = 1500

func sourceInventoryClusterFiles(t *testing.T) []string {
	t.Helper()
	var files []string
	for _, dir := range []string{".", "../types"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("readdir %s: %v", dir, err)
		}
		for _, e := range entries {
			n := e.Name()
			if strings.HasPrefix(n, "source_inventory") && strings.HasSuffix(n, ".go") && !strings.HasSuffix(n, "_test.go") {
				files = append(files, filepath.Join(dir, n))
			}
		}
	}
	return files
}

// Clause A — LOC-ceiling ratchet. Stops the +423/commit god-file accretion.
func TestSourceInventoryConvergence_LOCCeilingRatchet(t *testing.T) {
	for _, path := range sourceInventoryClusterFiles(t) {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		loc := strings.Count(string(data), "\n") // newline count == wc -l for newline-terminated Go files
		base := filepath.Base(path)
		ceiling, known := sourceInventoryFileLOCCeiling[base]
		if !known {
			ceiling = sourceInventoryNewFileLOCCeiling
		}
		if loc > ceiling {
			t.Errorf("%s is %d LOC, over its convergence ceiling %d. The source-inventory convergence target is to SHRINK this cluster (reconcile.go -> <=1500): extract code into a concern sub-file or the bounded execution kernel instead of growing it. If a growth is genuinely unavoidable, raise the ceiling here DELIBERATELY (never silently) — it is a ratchet, meant to fall.", path, loc, ceiling)
		}
	}
}

// Clause B — kernel-bypass ratchet. A zero-value sourceInventoryExecBudget{}
// bypasses the bounded execution kernel (unbudgeted candidate construction).
// The count may only ratchet DOWN to 0 as the kernel becomes load-bearing.
const sourceInventoryZeroBudgetLiteralCeiling = 2

func TestSourceInventoryConvergence_NoNewKernelBypass(t *testing.T) {
	fset := token.NewFileSet()
	count := 0
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	for _, e := range entries {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		f, perr := parser.ParseFile(fset, n, nil, 0)
		if perr != nil {
			continue
		}
		count += countZeroValueExecBudgetLiterals(f)
	}
	if count > sourceInventoryZeroBudgetLiteralCeiling {
		t.Errorf("found %d zero-value sourceInventoryExecBudget{} literal(s) (unbudgeted kernel bypass), over the ratchet ceiling %d. Route candidate construction through the bounded execution kernel (sourceInventoryExecBudgetForLens); do NOT add a new unbudgeted bypass — the target is 0.", count, sourceInventoryZeroBudgetLiteralCeiling)
	}
}

func countZeroValueExecBudgetLiterals(f *ast.File) int {
	n := 0
	ast.Inspect(f, func(node ast.Node) bool {
		cl, ok := node.(*ast.CompositeLit)
		if !ok {
			return true
		}
		if id, ok := cl.Type.(*ast.Ident); ok && id.Name == "sourceInventoryExecBudget" && len(cl.Elts) == 0 {
			n++
		}
		return true
	})
	return n
}

// Clause C — no parallel source-class taxonomy. Role classification must route
// through types.ClassifySourcePathRole; a switch/if-chain with >=2 source-class
// string-literal cases is a re-derived taxonomy (the RNE-C53 recurrence root).
var sourceClassTaxonomyTokens = map[string]bool{
	"test": true, "tests": true, "testdata": true, "fixture": true, "fixtures": true,
	"corpus": true, "corpora": true, "example": true, "examples": true,
	"generated": true, "promptsupport": true,
}

func TestSourceInventoryConvergence_NoParallelSourceClassTaxonomy(t *testing.T) {
	fset := token.NewFileSet()
	var violations []string
	for _, path := range sourceInventoryClusterFiles(t) {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		violations = append(violations, findParallelSourceClassTaxonomy(f, fset, path)...)
	}
	if len(violations) > 0 {
		t.Fatalf("parallel source-class taxonomy (>=2 source-class string tokens in one switch) — route role classification through types.ClassifySourcePathRole instead of re-deriving it:\n  %s", strings.Join(violations, "\n  "))
	}
}

func findParallelSourceClassTaxonomy(f *ast.File, fset *token.FileSet, path string) []string {
	var out []string
	ast.Inspect(f, func(node ast.Node) bool {
		sw, ok := node.(*ast.SwitchStmt)
		if !ok {
			return true
		}
		tokens := 0
		for _, stmt := range sw.Body.List {
			cc, ok := stmt.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range cc.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				if v, err := strconv.Unquote(lit.Value); err == nil && sourceClassTaxonomyTokens[strings.ToLower(v)] {
					tokens++
				}
			}
		}
		if tokens >= 2 {
			out = append(out, path+":"+strconv.Itoa(fset.Position(sw.Pos()).Line))
		}
		return true
	})
	return out
}

// TestSourceInventoryConvergence_DetectorsSelfValidate proves the AST detectors
// fire on synthetic violations and stay quiet on legitimate code, so the
// tripwire cannot silently become a no-op.
func TestSourceInventoryConvergence_DetectorsSelfValidate(t *testing.T) {
	fset := token.NewFileSet()

	bypass := `package p
type sourceInventoryExecBudget struct{ n int }
func a() { _ = sourceInventoryExecBudget{} }          // zero-value bypass -> BAN
func b() { _ = sourceInventoryExecBudget{n: 1} }      // budgeted -> OK
`
	f, err := parser.ParseFile(fset, "bypass.go", bypass, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got := countZeroValueExecBudgetLiterals(f); got != 1 {
		t.Fatalf("bypass detector: got %d, want 1 (zero-value only)", got)
	}

	taxonomy := `package p
func cls(s string) string { switch s { case "test", "fixture": return "nonprod" }; return "prod" } // 2 class tokens -> BAN
func skip(s string) bool { switch s { case ".git", "node_modules", "vendor": return true }; return false } // dir-skip, 0 class tokens -> OK
`
	f2, err := parser.ParseFile(fset, "tax.go", taxonomy, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	v := findParallelSourceClassTaxonomy(f2, fset, "tax.go")
	if len(v) != 1 {
		t.Fatalf("taxonomy detector: got %d violations, want 1 (the >=2 class-token switch; the dir-skip switch must NOT fire): %v", len(v), v)
	}
}
