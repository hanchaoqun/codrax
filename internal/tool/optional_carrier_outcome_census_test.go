package tool

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// optional_carrier_outcome_census_test.go — V2-3 (§40.19 ③) as re-pinned by
// the §40.44 fold-in (E2): every OptionalCarrierOutcome minted during one
// tool call reaches the ToolResult that call returns. The invariant is
// enforced by construction at the tool-result boundary (optionalCarrierLedger
// + finalize choke point), and this census pins the construction on precise
// identifiers — never on log wording:
//
//   1. CREATOR RULE — a function that calls newOptionalCarrierLedger must
//      bind it with `:=`, have a named first result of type types.ToolResult,
//      and the very next statement must be exactly
//      `defer func() { <result> = <ledger>.finalize(<result>) }()`, so every
//      return path (downgrade / accepted / rejection / panic-free early
//      return) leaves through the choke point.
//   2. ONE CONSTRUCTOR — the ledger type is constructed nowhere but inside
//      newOptionalCarrierLedger (no literal, new(), or zero-value var).
//   3. NO ESCAPE — a ledger identifier (creator binding or *optionalCarrierLedger
//      parameter) is only ever the receiver of ignored/mint/finalize/toolName
//      or an argument passed down a call; it is never stored, returned or
//      captured elsewhere, so a per-call registry stays per-call.
//   4. DATA-FLOW TOTALITY — every function that mints (calls .ignored/.mint on a
//      ledger identifier) either creates the ledger or receives it as a
//      parameter, and every caller chain of a receiving function terminates
//      in a creator (the four emit executors are found this way, not listed).
//   5. OWNERS — logOptionalCarrierIgnored is called only inside the ledger's
//      mint; attachOptionalCarrierOutcome only inside finalize; and, repo-wide
//      over every non-test Go file, ToolResult.OptionalCarrierOutcomes is
//      written only by attachOptionalCarrierOutcome.
//
// Each evasion shape has a self-red subtest below (fixture sources run
// through the same audit), and the production run must find the real
// executors (vacuity guards).

const (
	optionalCarrierOwnerFile   = "optional_carrier_outcome.go"
	optionalCarrierLedgerType  = "optionalCarrierLedger"
	optionalCarrierLedgerCtor  = "newOptionalCarrierLedger"
	optionalCarrierLogOwner    = "logOptionalCarrierIgnored"
	optionalCarrierAttachOwner = "attachOptionalCarrierOutcome"
	optionalCarrierResultField = "OptionalCarrierOutcomes"
)

var optionalCarrierMintMethods = map[string]bool{"ignored": true, "mint": true}

type optionalCarrierCensus struct {
	offenders []string
	creators  []string
	minters   []string
	receivers map[string][]string // receiving function → its callers (by name)
	fieldSite int
}

type censusFunc struct {
	file   string
	name   string
	recv   string
	decl   *ast.FuncType
	body   *ast.BlockStmt
	parent *censusFunc
}

// collectFuncs enumerates FuncDecls and every FuncLit nested in them (a
// closure is a function of its own for the creator rule).
func collectFuncs(file string, f *ast.File) []*censusFunc {
	var out []*censusFunc
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		recv := ""
		if fn.Recv != nil && len(fn.Recv.List) > 0 {
			recv = exprTypeName(fn.Recv.List[0].Type)
		}
		top := &censusFunc{file: file, name: fn.Name.Name, recv: recv, decl: fn.Type, body: fn.Body}
		out = append(out, top)
		var walk func(parent *censusFunc, body *ast.BlockStmt)
		walk = func(parent *censusFunc, body *ast.BlockStmt) {
			ast.Inspect(body, func(n ast.Node) bool {
				lit, ok := n.(*ast.FuncLit)
				if !ok {
					return true
				}
				child := &censusFunc{file: file, name: parent.name + ".func", recv: parent.recv, decl: lit.Type, body: lit.Body, parent: parent}
				out = append(out, child)
				walk(child, lit.Body)
				return false
			})
		}
		walk(top, fn.Body)
	}
	return out
}

func exprTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.StarExpr:
		return exprTypeName(t.X)
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		return exprTypeName(t.X) + "." + t.Sel.Name
	}
	return ""
}

// ownStatements visits the statements/expressions of one function body
// without descending into nested FuncLits (those are their own functions).
func ownStatements(body *ast.BlockStmt, visit func(ast.Node) bool) {
	ast.Inspect(body, func(n ast.Node) bool {
		if _, ok := n.(*ast.FuncLit); ok {
			return false
		}
		return visit(n)
	})
}

func isCtorCall(expr ast.Expr) bool {
	call, ok := expr.(*ast.CallExpr)
	if !ok {
		return false
	}
	ident, ok := call.Fun.(*ast.Ident)
	return ok && ident.Name == optionalCarrierLedgerCtor
}

// ledgerIdents returns the identifiers that hold a ledger in this function:
// parameters typed *optionalCarrierLedger and `:=` bindings of the ctor,
// inherited from the enclosing function for closures.
func ledgerIdents(fn *censusFunc) map[string]bool {
	out := map[string]bool{}
	if fn.parent != nil {
		for name := range ledgerIdents(fn.parent) {
			out[name] = true
		}
	}
	if fn.decl != nil && fn.decl.Params != nil {
		for _, field := range fn.decl.Params.List {
			if exprTypeName(field.Type) == optionalCarrierLedgerType {
				for _, name := range field.Names {
					out[name.Name] = true
				}
			}
		}
	}
	ownStatements(fn.body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || assign.Tok != token.DEFINE || len(assign.Rhs) != 1 || !isCtorCall(assign.Rhs[0]) || len(assign.Lhs) != 1 {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok {
			out[ident.Name] = true
		}
		return true
	})
	return out
}

// isFinalizeDefer matches exactly `defer func() { <res> = <ledger>.finalize(<res>) }()`.
func isFinalizeDefer(stmt ast.Stmt, ledger, result string) bool {
	def, ok := stmt.(*ast.DeferStmt)
	if !ok {
		return false
	}
	lit, ok := def.Call.Fun.(*ast.FuncLit)
	if !ok || len(def.Call.Args) != 0 || lit.Body == nil || len(lit.Body.List) != 1 {
		return false
	}
	assign, ok := lit.Body.List[0].(*ast.AssignStmt)
	if !ok || assign.Tok != token.ASSIGN || len(assign.Lhs) != 1 || len(assign.Rhs) != 1 {
		return false
	}
	lhs, ok := assign.Lhs[0].(*ast.Ident)
	if !ok || lhs.Name != result {
		return false
	}
	call, ok := assign.Rhs[0].(*ast.CallExpr)
	if !ok || len(call.Args) != 1 {
		return false
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "finalize" {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	if !ok || recv.Name != ledger {
		return false
	}
	arg, ok := call.Args[0].(*ast.Ident)
	return ok && arg.Name == result
}

func namedToolResult(fn *censusFunc) (string, bool) {
	if fn.decl == nil || fn.decl.Results == nil || len(fn.decl.Results.List) == 0 {
		return "", false
	}
	first := fn.decl.Results.List[0]
	if len(first.Names) == 0 || exprTypeName(first.Type) != "types.ToolResult" {
		return "", false
	}
	return first.Names[0].Name, true
}

func auditOptionalCarrierFiles(fset *token.FileSet, files map[string]*ast.File) optionalCarrierCensus {
	c := optionalCarrierCensus{receivers: map[string][]string{}}
	pos := func(n ast.Node) string { return fset.Position(n.Pos()).String() }
	var funcs []*censusFunc
	names := []string{}
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		funcs = append(funcs, collectFuncs(name, files[name])...)
	}
	isOwner := func(fn *censusFunc) bool { return filepath.Base(fn.file) == optionalCarrierOwnerFile }
	// Receiving functions (a *optionalCarrierLedger parameter), by name.
	receiving := map[string]bool{}
	for _, fn := range funcs {
		if fn.parent != nil || fn.decl == nil || fn.decl.Params == nil {
			continue
		}
		for _, field := range fn.decl.Params.List {
			if exprTypeName(field.Type) == optionalCarrierLedgerType {
				receiving[fn.name] = true
			}
		}
	}
	for _, fn := range funcs {
		idents := ledgerIdents(fn)
		label := fn.name
		if fn.recv != "" {
			label = fn.recv + "." + fn.name
		}
		ownStatements(fn.body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BlockStmt, *ast.CaseClause, *ast.CommClause:
				list := stmtList(node)
				for i, stmt := range list {
					assign, ok := stmt.(*ast.AssignStmt)
					if !ok || len(assign.Rhs) != 1 || !isCtorCall(assign.Rhs[0]) {
						continue
					}
					ledger, ok := assign.Lhs[0].(*ast.Ident)
					if assign.Tok != token.DEFINE || len(assign.Lhs) != 1 || !ok {
						c.offenders = append(c.offenders, pos(stmt)+" "+label+": the ledger must be bound with `ident := "+optionalCarrierLedgerCtor+"(...)`")
						continue
					}
					result, ok := namedToolResult(fn)
					if !ok {
						c.offenders = append(c.offenders, pos(stmt)+" "+label+": a ledger creator must have a NAMED first result of type types.ToolResult so the deferred finalize can rewrite it")
						continue
					}
					if i+1 >= len(list) || !isFinalizeDefer(list[i+1], ledger.Name, result) {
						c.offenders = append(c.offenders, pos(stmt)+" "+label+": the statement right after the ledger creation must be `defer func() { "+result+" = "+ledger.Name+".finalize("+result+") }()` (the tool-result choke point)")
						continue
					}
					c.creators = append(c.creators, label)
				}
			case *ast.CallExpr:
				if isCtorCall(node) {
					// Reached only when the ctor call is NOT the whole rhs of an
					// assignment statement (argument position, expression, …).
					if parentIsAssignRHS(fn.body, node) {
						return true
					}
					c.offenders = append(c.offenders, pos(node)+" "+label+": "+optionalCarrierLedgerCtor+" may only be called as the whole right-hand side of a `:=` binding")
				}
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok {
					if recv, ok := sel.X.(*ast.Ident); ok && idents[recv.Name] && optionalCarrierMintMethods[sel.Sel.Name] {
						c.minters = append(c.minters, label)
					}
				}
				if ident, ok := node.Fun.(*ast.Ident); ok {
					switch ident.Name {
					case optionalCarrierLogOwner:
						if !(isOwner(fn) && fn.recv == optionalCarrierLedgerType && fn.name == "mint") {
							c.offenders = append(c.offenders, pos(node)+" "+label+": "+optionalCarrierLogOwner+" is called only by the ledger's mint (an ignore log without a registered outcome is the log-only shape)")
						}
					case optionalCarrierAttachOwner:
						if !(isOwner(fn) && fn.recv == optionalCarrierLedgerType && fn.name == "finalize") {
							c.offenders = append(c.offenders, pos(node)+" "+label+": "+optionalCarrierAttachOwner+" is called only by the ledger's finalize")
						}
					}
					if receiving[ident.Name] {
						c.receivers[ident.Name] = append(c.receivers[ident.Name], label)
					}
				}
				if sel, ok := node.Fun.(*ast.SelectorExpr); ok && receiving[sel.Sel.Name] {
					c.receivers[sel.Sel.Name] = append(c.receivers[sel.Sel.Name], label)
				}
			case *ast.CompositeLit:
				if exprTypeName(node.Type) == optionalCarrierLedgerType && !(isOwner(fn) && fn.name == optionalCarrierLedgerCtor) {
					c.offenders = append(c.offenders, pos(node)+" "+label+": the ledger is constructed only by "+optionalCarrierLedgerCtor)
				}
			case *ast.ValueSpec:
				if exprTypeName(node.Type) == optionalCarrierLedgerType {
					c.offenders = append(c.offenders, pos(node)+" "+label+": a zero-value ledger bypasses "+optionalCarrierLedgerCtor)
				}
			}
			return true
		})
		// new(optionalCarrierLedger)
		ownStatements(fn.body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == "new" && len(call.Args) == 1 && exprTypeName(call.Args[0]) == optionalCarrierLedgerType {
				c.offenders = append(c.offenders, pos(call)+" "+label+": new("+optionalCarrierLedgerType+") bypasses "+optionalCarrierLedgerCtor)
			}
			return true
		})
		// NO ESCAPE: every use of a ledger identifier is a sanctioned shape.
		if len(idents) > 0 {
			c.offenders = append(c.offenders, ledgerEscapes(fset, fn, idents, label)...)
		}
		// Field writers (repo-wide input): assignment / append / literal key.
		ownStatements(fn.body, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.AssignStmt:
				for _, lhs := range node.Lhs {
					if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == optionalCarrierResultField {
						c.noteFieldWrite(pos(node), label, isOwner(fn) && fn.name == optionalCarrierAttachOwner)
					}
				}
			case *ast.KeyValueExpr:
				if key, ok := node.Key.(*ast.Ident); ok && key.Name == optionalCarrierResultField {
					c.noteFieldWrite(pos(node), label, isOwner(fn) && fn.name == optionalCarrierAttachOwner)
				}
			}
			return true
		})
	}
	// DATA-FLOW TOTALITY: every minter is a creator or a receiver whose caller
	// chain ends in a creator.
	creatorSet := map[string]bool{}
	for _, name := range c.creators {
		creatorSet[name] = true
	}
	for _, minter := range c.minters {
		// A closure mints with the ledger of the function that encloses it.
		for strings.HasSuffix(minter, ".func") {
			minter = strings.TrimSuffix(minter, ".func")
		}
		if creatorSet[minter] {
			continue
		}
		if !c.chainEndsInCreator(minter, creatorSet, map[string]bool{}) {
			c.offenders = append(c.offenders, minter+": mints an outcome but neither creates the ledger nor is reachable only from a ledger creator (no result boundary would finalize its rows)")
		}
	}
	sort.Strings(c.offenders)
	return c
}

func (c *optionalCarrierCensus) noteFieldWrite(at, label string, sanctioned bool) {
	if sanctioned {
		c.fieldSite++
		return
	}
	c.offenders = append(c.offenders, at+" "+label+": ToolResult."+optionalCarrierResultField+" is written only by "+optionalCarrierAttachOwner+" (the ledger finalize)")
}

// chainEndsInCreator walks callers by function name until a creator is
// reached on EVERY path.
func (c *optionalCarrierCensus) chainEndsInCreator(fn string, creators, visiting map[string]bool) bool {
	short := fn
	if i := strings.LastIndex(short, "."); i >= 0 {
		short = short[i+1:]
	}
	if creators[fn] {
		return true
	}
	if visiting[fn] {
		return false
	}
	visiting[fn] = true
	callers := c.receivers[short]
	if len(callers) == 0 {
		return false
	}
	for _, caller := range callers {
		if !c.chainEndsInCreator(caller, creators, visiting) {
			return false
		}
	}
	return true
}

func stmtList(n ast.Node) []ast.Stmt {
	switch node := n.(type) {
	case *ast.BlockStmt:
		return node.List
	case *ast.CaseClause:
		return node.Body
	case *ast.CommClause:
		return node.Body
	}
	return nil
}

func parentIsAssignRHS(body *ast.BlockStmt, target *ast.CallExpr) bool {
	found := false
	ownStatements(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for _, rhs := range assign.Rhs {
			if rhs == target {
				found = true
			}
		}
		return true
	})
	return found
}

// ledgerEscapes flags every use of a ledger identifier that is not one of:
// the `:=` binding itself, the receiver of ignored/mint/finalize/toolName, or
// an argument passed to a call.
func ledgerEscapes(fset *token.FileSet, fn *censusFunc, idents map[string]bool, label string) []string {
	var out []string
	sanctioned := map[ast.Node]bool{}
	ownStatements(fn.body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.AssignStmt:
			if node.Tok == token.DEFINE && len(node.Rhs) == 1 && isCtorCall(node.Rhs[0]) {
				for _, lhs := range node.Lhs {
					sanctioned[lhs] = true
				}
			}
		case *ast.SelectorExpr:
			if recv, ok := node.X.(*ast.Ident); ok && idents[recv.Name] {
				switch node.Sel.Name {
				case "ignored", "mint", "finalize", "toolName":
					sanctioned[recv] = true
				}
			}
		case *ast.CallExpr:
			for _, arg := range node.Args {
				if ident, ok := arg.(*ast.Ident); ok && idents[ident.Name] {
					sanctioned[ident] = true
				}
			}
		case *ast.Field:
			for _, name := range node.Names {
				sanctioned[name] = true
			}
		}
		return true
	})
	ownStatements(fn.body, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if !ok || !idents[ident.Name] || sanctioned[ident] {
			return true
		}
		out = append(out, fset.Position(ident.Pos()).String()+" "+label+": ledger identifier "+ident.Name+" escapes the call (stored, returned or used outside ignored/mint/finalize/argument position)")
		return true
	})
	return out
}

func parseGoFiles(t *testing.T, fset *token.FileSet, paths []string) map[string]*ast.File {
	t.Helper()
	out := map[string]*ast.File{}
	for _, path := range paths {
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		out[path] = f
	}
	return out
}

func repoNonTestGoFiles(t *testing.T) []string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("module root not found at %s: %v", root, err)
	}
	var out []string
	err = filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			name := d.Name()
			if name == "vendor" || name == "testdata" || name == ".git" || name == ".claude" || (strings.HasPrefix(name, ".") && path != root) {
				return filepath.SkipDir
			}
			// Only the module's source trees are executors: root-level
			// files, internal/ and cmd/. Sibling directories such as
			// eval/ hold fixture repos with deliberately broken Go.
			if filepath.Dir(path) == root && name != "internal" && name != "cmd" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(out)
	return out
}

func TestOptionalCarrierOutcomesReachTheToolResultByConstruction(t *testing.T) {
	fset := token.NewFileSet()
	files := parseGoFiles(t, fset, repoNonTestGoFiles(t))
	if len(files) < 100 {
		t.Fatalf("repo scan found only %d non-test Go files — the census scan is broken", len(files))
	}
	c := auditOptionalCarrierFiles(fset, files)
	for _, offender := range c.offenders {
		t.Errorf("optional-carrier disclosure can be lost: %s", offender)
	}
	// Vacuity guards, bound to the live producers by data flow (not a list):
	// the ledger must be created by the executors that mint, and the receiving
	// helpers must be reached from them.
	if len(c.creators) < 4 {
		t.Fatalf("expected the emit executors to create the ledger (>= 4 creators), found %v", c.creators)
	}
	if len(c.minters) < 4 {
		t.Fatalf("expected the minting sites (waivers, selector, raw_request echo), found %v", c.minters)
	}
	if c.fieldSite != 1 {
		t.Fatalf("ToolResult.%s must have exactly one writer (%s), found %d", optionalCarrierResultField, optionalCarrierAttachOwner, c.fieldSite)
	}
	for _, want := range []string{"applyEvidenceFloorWaiverPayload", "resolveTraceRootCauseSelectionForEmit"} {
		if len(c.receivers[want]) == 0 {
			t.Errorf("receiving helper %s has no scanned caller — the data-flow arm is not bound to the live sites", want)
		}
	}
}

// Self-red: each evasion shape must be flagged by the same audit; the
// canonical shape must pass. The fixture set carries a minimal owner file so
// the owner arms resolve exactly as in production.
func TestOptionalCarrierCensusFlagsEachEvasionShape(t *testing.T) {
	const owner = `package tool
import "github.com/hanchaoqun/codrax/internal/types"
type optionalCarrierLedger struct{ toolName string; minted []types.OptionalCarrierOutcome }
func newOptionalCarrierLedger(toolName string) *optionalCarrierLedger { return &optionalCarrierLedger{toolName: toolName} }
func (l *optionalCarrierLedger) ignored(carrier, reason string) types.OptionalCarrierOutcome { return l.mint(types.OptionalCarrierOutcome{Carrier: carrier, Reason: reason}) }
func (l *optionalCarrierLedger) mint(o types.OptionalCarrierOutcome) types.OptionalCarrierOutcome { logOptionalCarrierIgnored(l.toolName, o); l.minted = append(l.minted, o); return o }
func (l *optionalCarrierLedger) finalize(res types.ToolResult) types.ToolResult { for _, o := range l.minted { attachOptionalCarrierOutcome(&res, o) }; return res }
func logOptionalCarrierIgnored(toolName string, o types.OptionalCarrierOutcome) {}
func attachOptionalCarrierOutcome(res *types.ToolResult, o types.OptionalCarrierOutcome) { res.OptionalCarrierOutcomes = append(res.OptionalCarrierOutcomes, o) }
`
	const canonical = `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func helper(ctx int, carriers *optionalCarrierLedger) { carriers.ignored("waiver", "reason") }
func (t *Tool) Execute(params []byte) (result types.ToolResult, err error) {
	carriers := newOptionalCarrierLedger("tool")
	defer func() { result = carriers.finalize(result) }()
	helper(1, carriers)
	if len(params) == 0 { return types.ToolResult{Success: true}, nil }
	carriers.ignored("echo", "reason")
	return types.ToolResult{Success: true}, nil
}
`
	run := func(t *testing.T, src string) optionalCarrierCensus {
		t.Helper()
		fset := token.NewFileSet()
		files := map[string]*ast.File{}
		for name, body := range map[string]string{"optional_carrier_outcome.go": owner, "fixture.go": src} {
			f, err := parser.ParseFile(fset, name, body, 0)
			if err != nil {
				t.Fatalf("fixture parse: %v", err)
			}
			files[name] = f
		}
		return auditOptionalCarrierFiles(fset, files)
	}
	if c := run(t, canonical); len(c.offenders) != 0 || len(c.creators) != 1 || len(c.minters) != 2 || c.fieldSite != 1 {
		t.Fatalf("the canonical shape must pass: %+v", c)
	}
	for _, tc := range []struct{ name, src, want string }{
		{"creator without the finalize defer", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	carriers := newOptionalCarrierLedger("tool")
	carriers.ignored("x", "y")
	return result, nil
}`, "statement right after the ledger creation must be"},
		{"finalize defer not directly after creation", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute(params []byte) (result types.ToolResult, err error) {
	carriers := newOptionalCarrierLedger("tool")
	if len(params) == 0 { return result, nil }
	defer func() { result = carriers.finalize(result) }()
	return result, nil
}`, "statement right after the ledger creation must be"},
		{"finalize defer on a different value", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	carriers := newOptionalCarrierLedger("tool")
	defer func() { other := carriers.finalize(result); _ = other }()
	return result, nil
}`, "statement right after the ledger creation must be"},
		{"creator with unnamed results", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (types.ToolResult, error) {
	carriers := newOptionalCarrierLedger("tool")
	defer func() { carriers.finalize(types.ToolResult{}) }()
	return types.ToolResult{}, nil
}`, "NAMED first result"},
		{"creator inside a closure without the defer", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	run := func() (result types.ToolResult, err error) {
		carriers := newOptionalCarrierLedger("tool")
		carriers.ignored("x", "y")
		return result, nil
	}
	return run()
}`, "statement right after the ledger creation must be"},
		{"ledger built by literal", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	carriers := &optionalCarrierLedger{toolName: "tool"}
	carriers.ignored("x", "y")
	return result, nil
}`, "constructed only by"},
		{"ledger built by new", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	carriers := new(optionalCarrierLedger)
	carriers.ignored("x", "y")
	return result, nil
}`, "bypasses"},
		{"zero-value ledger", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	var carriers optionalCarrierLedger
	carriers.ignored("x", "y")
	return result, nil
}`, "zero-value ledger"},
		{"ledger stashed on a struct (escapes the call)", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	carriers := newOptionalCarrierLedger("tool")
	defer func() { result = carriers.finalize(result) }()
	t.saved = carriers
	return result, nil
}`, "escapes the call"},
		{"ctor call in argument position", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	helper(newOptionalCarrierLedger("tool"))
	return result, nil
}
func helper(carriers *optionalCarrierLedger) { carriers.ignored("x", "y") }`, "whole right-hand side"},
		{"ignore log outside the ledger", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	logOptionalCarrierIgnored("tool", types.OptionalCarrierOutcome{Carrier: "x", Reason: "dropped; not honored"})
	return result, nil
}`, "is called only by the ledger's mint"},
		{"result field written outside the owner", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	result.OptionalCarrierOutcomes = append(result.OptionalCarrierOutcomes, types.OptionalCarrierOutcome{Carrier: "x", Reason: "y"})
	return result, nil
}`, "is written only by"},
		{"result field set in a literal outside the owner", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	return types.ToolResult{OptionalCarrierOutcomes: []types.OptionalCarrierOutcome{{Carrier: "x", Reason: "y"}}}, nil
}`, "is written only by"},
		{"attach helper called outside finalize", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func (t *Tool) Execute() (result types.ToolResult, err error) {
	attachOptionalCarrierOutcome(&result, types.OptionalCarrierOutcome{Carrier: "x", Reason: "y"})
	return result, nil
}`, "is called only by the ledger's finalize"},
		{"receiving helper reachable from a non-creator", `package tool
import "github.com/hanchaoqun/codrax/internal/types"
func helper(carriers *optionalCarrierLedger) { carriers.ignored("x", "y") }
func (t *Tool) Execute(carriers *optionalCarrierLedger) (result types.ToolResult, err error) {
	helper(carriers)
	return result, nil
}`, "no result boundary would finalize its rows"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := run(t, tc.src)
			if len(c.offenders) == 0 {
				t.Fatalf("evasion shape not flagged: %+v", c)
			}
			if !strings.Contains(strings.Join(c.offenders, "\n"), tc.want) {
				t.Fatalf("offender must name the broken rule (%q): %v", tc.want, c.offenders)
			}
		})
	}
}
