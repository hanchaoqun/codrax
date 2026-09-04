package orchestrator

// ratchet_compliance_message_census_test.go — §40.27 V7-5 / §40.55.
//
// Three hot-file LOC ratchets exist in the repo (single-source table below).
// Their tripwire messages are the only teaching a developer sees at trip
// time, and before this pin all three said "split concern-specific code"
// without saying what is NOT compliance: 4c7a0d0a3 stayed under the
// orchestrator.go ceiling by compressing the §29.60 pendingCompletionReset
// declaration comment and deleting the throat comment (later restored in
// §40.55), and 727366b32 stayed under a 78-LOC source-inventory ceiling by
// rewriting a comment. The ratchet's intent is concern extraction; paying it
// with comments/blank lines/dead-line trimming leaves the god-file the same
// size in code and silently erodes the documentation the ratchet exists to
// protect.
//
// This census pins one shared sentence into every ratchet's failure message.
// It is deliberately a SOFT signal (message wording + docs, §11.8): a
// comment-line floor or comment-density ratchet would hard-gate on a noisy
// count (comments legitimately move with the code they describe), which the
// precise-signals-for-hard-gates rule forbids. The census itself is precise
// and bound by data flow (§40.50): the sentence must occur inside the
// FORMAT-position string literal of a `.Fatalf` / `.Errorf` call — the text
// a developer actually reads when the ratchet trips — never merely somewhere
// in the file. A comment, an unused `var _ = "…"`, or the sentence passed as
// a later argument is dead text for that purpose and stays red (the first
// version of this census took a whole-file substring, which a comment
// satisfied — §40.55 合流复核, G6-ratchet #2).
//
// The tripwire is bound to its RECEIVER as well (§40.55 收编复核再收编, b6f2
// #12): `.Fatalf` / `.Errorf` count only when the receiver is a parameter or
// an explicitly typed local of type `*testing.T` / `testing.TB` — the
// identifier is resolved through the parser's object table, not matched by
// name. A `fmt.Errorf(rule)` (an imported package's function) is not a
// tripwire and never binds; any other receiver shape (a field, an `x := t`
// alias, a `*log.Logger`, an unresolvable name) fails loud as an unrecognized
// receiver rather than reading as either pass or fail (§40.50). A tripwire
// whose enclosing `if` guard is constant-false (`false`, `!true`, either in
// parentheses) is dead text too: the rule bound only there is its own red
// shape. Reachability beyond that literal guard is not modelled — no census
// here does control-flow analysis — so this is the named evasion shape
// added on discovery (§40.50 ①), not a reachability proof.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// ratchetComplianceRule is the sentence every LOC-ratchet tripwire must carry
// verbatim. Edit it here and in each listed test in the same change.
const ratchetComplianceRule = "Comment/blank-line compression and dead-line trimming are NOT ratchet compliance — extract a concern file and lower this ceiling in the same change."

// locRatchetTestFiles is the single-source table of hot-file LOC ratchets
// (paths relative to this package). A new ratchet is registered here so it
// inherits the rule; an unreadable entry fails the census rather than being
// skipped.
var locRatchetTestFiles = []string{
	"ir_delivery_ratchet_test.go",
	filepath.Join("..", "dataquery", "loc_ratchet_test.go"),
	filepath.Join("..", "tool", "source_inventory_convergence_test.go"),
}

// ratchetTripwireMethods is the closed set of testing.TB methods whose FIRST
// argument is the format string a developer reads at trip time.
var ratchetTripwireMethods = map[string]bool{"Fatalf": true, "Errorf": true}

// ratchetMessageShape is the closed set of outcomes ratchetMessageCensus can
// report for one file; ratchetMessageFailure is total over it.
type ratchetMessageShape int

const (
	ratchetMessageBound                  ratchetMessageShape = iota // the rule sits in a live testing.TB tripwire's format literal
	ratchetMessageUnparseable                                       // the file is not parseable Go
	ratchetMessageUnrecognizedReceiver                              // a .Fatalf/.Errorf receiver the census cannot classify (fail loud)
	ratchetMessageRuleOnlyInDeadTripwire                            // the rule sits only in a tripwire under a constant-false guard
	ratchetMessageNoLiteralTripwire                                 // no live testing.TB .Fatalf/.Errorf call has a string-literal format
	ratchetMessageRuleAbsentFromFormat                              // live tripwires exist but none carries the rule in format position
)

// ratchetReceiverKind is the closed classification of a `.Fatalf`/`.Errorf`
// receiver; anything the classifier cannot place is an error, never a kind.
type ratchetReceiverKind int

const (
	ratchetReceiverTestingTB ratchetReceiverKind = iota // *testing.T / testing.TB parameter or typed local
	ratchetReceiverPackage                              // an imported package's function (fmt.Errorf, log.Fatalf)
)

// ratchetMessageCensus parses src and reports whether ratchetComplianceRule
// occurs inside the format-position string literal (a BasicLit, or a `+`
// chain of BasicLits) of at least one live `<recv>.Fatalf(...)` /
// `<recv>.Errorf(...)` call whose receiver is a *testing.T / testing.TB
// parameter or typed local. Placement anywhere else — comments, other
// declarations, non-format arguments, package functions, a tripwire under a
// constant-false guard — does not count. The int is the number of live
// literal-format testing.TB tripwires seen; the error names the offending
// receiver for ratchetMessageUnrecognizedReceiver and the parse failure for
// ratchetMessageUnparseable.
func ratchetMessageCensus(filename string, src []byte) (ratchetMessageShape, int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return ratchetMessageUnparseable, 0, err
	}
	imports := ratchetImportNames(file)
	dead := ratchetConstantFalseBodies(file)
	tripwires, bound, deadBound := 0, false, false
	var unrecognized error
	ast.Inspect(file, func(n ast.Node) bool {
		if unrecognized != nil {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !ratchetTripwireMethods[sel.Sel.Name] {
			return true
		}
		kind, rerr := ratchetTripwireReceiver(sel.X, imports)
		if rerr != nil {
			unrecognized = fmt.Errorf("%s: %w", fset.Position(sel.Pos()), rerr)
			return false
		}
		if kind != ratchetReceiverTestingTB {
			return true
		}
		format, ok := ratchetFormatLiteral(call.Args[0])
		if !ok {
			return true
		}
		carries := strings.Contains(format, ratchetComplianceRule)
		if ratchetInsideAny(call.Pos(), dead) {
			if carries {
				deadBound = true
			}
			return true
		}
		tripwires++
		if carries {
			bound = true
		}
		return true
	})
	switch {
	case unrecognized != nil:
		return ratchetMessageUnrecognizedReceiver, tripwires, unrecognized
	case bound:
		return ratchetMessageBound, tripwires, nil
	case deadBound:
		return ratchetMessageRuleOnlyInDeadTripwire, tripwires, nil
	case tripwires == 0:
		return ratchetMessageNoLiteralTripwire, 0, nil
	}
	return ratchetMessageRuleAbsentFromFormat, tripwires, nil
}

// ratchetImportNames returns the local names the file's imports bind (an
// explicit alias, else the last path segment); blank and dot imports bind no
// selector-usable name.
func ratchetImportNames(file *ast.File) map[string]bool {
	names := make(map[string]bool, len(file.Imports))
	for _, imp := range file.Imports {
		if imp.Name != nil {
			if imp.Name.Name == "_" || imp.Name.Name == "." {
				continue
			}
			names[imp.Name.Name] = true
			continue
		}
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			continue
		}
		names[path.Base(p)] = true
	}
	return names
}

// ratchetTripwireReceiver classifies the receiver expression of a
// `.Fatalf`/`.Errorf` selector. Only a bare identifier is classifiable: one
// that resolves (parser object table) to a function parameter or an
// explicitly typed `var` of type *testing.T / testing.TB is a testing.TB
// receiver; one that names a file import is a package function. Everything
// else — a non-identifier receiver, an `x := t` alias (AssignStmt, no
// declared type), a parameter/local of any other type, or a name that
// resolves nowhere — is an error so the census fails loud instead of
// guessing.
func ratchetTripwireReceiver(recv ast.Expr, imports map[string]bool) (ratchetReceiverKind, error) {
	ident, ok := recv.(*ast.Ident)
	if !ok {
		return 0, fmt.Errorf("unrecognized tripwire receiver shape %T (only a bare *testing.T / testing.TB identifier or an imported package name is classifiable)", recv)
	}
	if ident.Obj == nil {
		if imports[ident.Name] {
			return ratchetReceiverPackage, nil
		}
		return 0, fmt.Errorf("unrecognized tripwire receiver %q: resolves to neither a declaration in this file nor an import", ident.Name)
	}
	var typ ast.Expr
	switch decl := ident.Obj.Decl.(type) {
	case *ast.ImportSpec:
		return ratchetReceiverPackage, nil
	case *ast.Field:
		typ = decl.Type
	case *ast.ValueSpec:
		typ = decl.Type
	default:
		return 0, fmt.Errorf("unrecognized tripwire receiver %q: declared by %T, not a typed parameter or `var`", ident.Name, ident.Obj.Decl)
	}
	if ratchetIsTestingTB(typ) {
		return ratchetReceiverTestingTB, nil
	}
	return 0, fmt.Errorf("unrecognized tripwire receiver %q: declared type is not *testing.T / testing.TB", ident.Name)
}

// ratchetIsTestingTB reports whether typ is exactly `*testing.T` or
// `testing.TB` (the package must be spelled `testing`; an alias is not
// recognized and therefore fails loud upstream).
func ratchetIsTestingTB(typ ast.Expr) bool {
	if star, ok := typ.(*ast.StarExpr); ok {
		return ratchetIsTestingSelector(star.X, "T")
	}
	return ratchetIsTestingSelector(typ, "TB")
}

func ratchetIsTestingSelector(typ ast.Expr, name string) bool {
	sel, ok := typ.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != name {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && pkg.Name == "testing"
}

// ratchetConstantFalseBodies collects the source ranges of every `if` body
// whose condition is literally constant-false (`false`, `!true`, either
// parenthesized); an `else` branch of such an `if` is live and not collected.
func ratchetConstantFalseBodies(file *ast.File) [][2]token.Pos {
	var ranges [][2]token.Pos
	ast.Inspect(file, func(n ast.Node) bool {
		ifs, ok := n.(*ast.IfStmt)
		if ok && ratchetConstantFalse(ifs.Cond) {
			ranges = append(ranges, [2]token.Pos{ifs.Body.Pos(), ifs.Body.End()})
		}
		return true
	})
	return ranges
}

func ratchetConstantFalse(cond ast.Expr) bool {
	switch c := cond.(type) {
	case *ast.ParenExpr:
		return ratchetConstantFalse(c.X)
	case *ast.Ident:
		return c.Name == "false" && c.Obj == nil
	case *ast.UnaryExpr:
		if c.Op != token.NOT {
			return false
		}
		inner := c.X
		if p, ok := inner.(*ast.ParenExpr); ok {
			inner = p.X
		}
		id, ok := inner.(*ast.Ident)
		return ok && id.Name == "true" && id.Obj == nil
	}
	return false
}

func ratchetInsideAny(pos token.Pos, ranges [][2]token.Pos) bool {
	for _, r := range ranges {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}

// ratchetFormatLiteral flattens a string BasicLit or a `+` chain of them into
// the text the developer reads; any other expression shape (an identifier, a
// call, a non-string literal) is not a literal format and reports false.
func ratchetFormatLiteral(expr ast.Expr) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		text, err := strconv.Unquote(e.Value)
		if err != nil {
			return "", false
		}
		return text, true
	case *ast.BinaryExpr:
		if e.Op != token.ADD {
			return "", false
		}
		left, ok := ratchetFormatLiteral(e.X)
		if !ok {
			return "", false
		}
		right, ok := ratchetFormatLiteral(e.Y)
		if !ok {
			return "", false
		}
		return left + right, true
	case *ast.ParenExpr:
		return ratchetFormatLiteral(e.X)
	}
	return "", false
}

// ratchetMessageFailure renders every non-bound shape; an unrecognized shape
// fails loud (§40.50) rather than reading as a pass.
func ratchetMessageFailure(path string, shape ratchetMessageShape, tripwires int, err error) string {
	switch shape {
	case ratchetMessageBound:
		return ""
	case ratchetMessageUnparseable:
		return fmt.Sprintf("%s: ratchet test does not parse as Go: %v", path, err)
	case ratchetMessageUnrecognizedReceiver:
		return fmt.Sprintf("%s: a .Fatalf/.Errorf call has a receiver the census cannot classify — %v (bind the tripwire to a *testing.T / testing.TB parameter or typed local, or extend ratchetTripwireReceiver deliberately)", path, err)
	case ratchetMessageRuleOnlyInDeadTripwire:
		return fmt.Sprintf("%s: the compliance rule sits only in a .Fatalf/.Errorf under a constant-false guard (dead text a developer never reads at trip time); %d live literal-format tripwire(s) lack it", path, tripwires)
	case ratchetMessageNoLiteralTripwire:
		return fmt.Sprintf("%s: no live *testing.T / testing.TB .Fatalf/.Errorf call with a string-literal format to bind the compliance rule to (a ratchet tripwire must state its rule in the format string a developer reads at trip time; fmt.Errorf and friends are not tripwires)", path)
	case ratchetMessageRuleAbsentFromFormat:
		return fmt.Sprintf("%s: none of its %d live literal-format tripwire(s) carries the compliance rule verbatim in format position (a comment, an unused declaration, a non-format argument, a package function such as fmt.Errorf, or a tripwire under `if false` does not count):\n  %s", path, tripwires, ratchetComplianceRule)
	}
	panic(fmt.Sprintf("ratchetMessageCensus produced an unrecognized shape %d — extend ratchetMessageFailure", shape))
}

func TestLOCRatchetMessagesCarryComplianceRule(t *testing.T) {
	t.Parallel()
	if len(locRatchetTestFiles) < 3 {
		t.Fatalf("locRatchetTestFiles lost entries: %v", locRatchetTestFiles)
	}
	for _, path := range locRatchetTestFiles {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read ratchet test %s: %v (a moved or renamed ratchet must update locRatchetTestFiles)", path, err)
		}
		shape, tripwires, perr := ratchetMessageCensus(filepath.Base(path), data)
		if msg := ratchetMessageFailure(path, shape, tripwires, perr); msg != "" {
			t.Error(msg)
		}
	}
}

// TestRatchetMessageCensusShapes is the census's self-red: one synthetic
// source per shape, including every dead-text placement the whole-file
// substring accepted (comment, unused var, non-format argument), the
// receiver / dead-guard evasions of b6f2 #12 (fmt.Errorf, `if false`, an
// alias or foreign-typed receiver), and the live format-position placements
// that must stay green.
func TestRatchetMessageCensusShapes(t *testing.T) {
	t.Parallel()
	rule := strconv.Quote(ratchetComplianceRule)
	// wrap places body inside a TestProbe(t *testing.T) with only "testing"
	// imported; file builds a whole file from an import block and decls.
	file := func(imports, decls string) []byte {
		return []byte("package probe\n\nimport (\n" + imports + ")\n\n" + decls + "\n")
	}
	wrap := func(body string) []byte {
		return file("\t\"testing\"\n", "func TestProbe(t *testing.T) {\n\tlines, max := 2, 1\n\t_ = lines\n\t_ = max\n"+body+"\n}")
	}
	cases := []struct {
		name string
		src  []byte
		want ratchetMessageShape
	}{
		{"green_fatalf_format", wrap(`	if lines > max { t.Fatalf("%d lines over %d. " + ` + rule + `, lines, max) }`), ratchetMessageBound},
		{"green_errorf_format_single_literal", wrap(`	t.Errorf(` + strconv.Quote("over ceiling. "+ratchetComplianceRule) + `)`), ratchetMessageBound},
		{"green_subtest_receiver", wrap(`	t.Run("x", func(tt *testing.T) { tt.Fatalf(("a" + ` + rule + `), 1) })`), ratchetMessageBound},
		{"green_testing_tb_parameter", file("\t\"testing\"\n", `func helper(tb testing.TB) { tb.Fatalf(`+rule+`) }`), ratchetMessageBound},
		{"green_typed_local_testing_tb", wrap(`	var tb testing.TB = t
	tb.Fatalf(` + rule + `)`), ratchetMessageBound},
		{"green_else_branch_of_constant_false_is_live", wrap(`	if false { t.Fatalf("%d lines over %d", lines, max) } else { t.Fatalf(` + rule + `) }`), ratchetMessageBound},
		{"green_fmt_errorf_beside_bound_tripwire", file("\t\"fmt\"\n\t\"testing\"\n", `func TestProbe(t *testing.T) { _ = fmt.Errorf("x"); t.Fatalf(`+rule+`) }`), ratchetMessageBound},
		{"red_rule_in_comment_only", wrap(`	// ` + ratchetComplianceRule + `
	t.Fatalf("%d lines over %d", lines, max)`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_in_unused_var", wrap(`	var _ = ` + rule + `
	t.Fatalf("%d lines over %d", lines, max)`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_as_non_format_argument", wrap(`	t.Fatalf("%d lines over %d: %s", lines, max, ` + rule + `)`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_in_fmt_errorf_tripwire_without", file("\t\"fmt\"\n\t\"testing\"\n", `func TestProbe(t *testing.T) {
	lines, max := 2, 1
	_ = fmt.Errorf(`+rule+`)
	if lines > max { t.Fatalf("%d lines over %d", lines, max) }
}`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_in_aliased_import_errorf", file("\tf \"fmt\"\n\t\"testing\"\n", `func TestProbe(t *testing.T) {
	_ = f.Errorf(`+rule+`)
	t.Fatalf("over")
}`), ratchetMessageRuleAbsentFromFormat},
		{"red_fmt_errorf_only_no_tripwire", file("\t\"fmt\"\n", `func probe() error { return fmt.Errorf(`+rule+`) }`), ratchetMessageNoLiteralTripwire},
		{"red_rule_in_fatal_not_fatalf", wrap(`	t.Fatal(` + rule + `)`), ratchetMessageNoLiteralTripwire},
		{"red_rule_behind_identifier_format", wrap(`	msg := ` + rule + `
	t.Fatalf(msg)`), ratchetMessageNoLiteralTripwire},
		{"red_no_tripwire_at_all", wrap(`	// ` + ratchetComplianceRule), ratchetMessageNoLiteralTripwire},
		{"red_rule_in_constant_false_guard", wrap(`	if false { t.Fatalf(` + rule + `) }
	if lines > max { t.Fatalf("%d lines over %d", lines, max) }`), ratchetMessageRuleOnlyInDeadTripwire},
		{"red_rule_in_not_true_guard_only", wrap(`	if (!true) { t.Fatalf(` + rule + `) }`), ratchetMessageRuleOnlyInDeadTripwire},
		{"red_rule_in_nested_constant_false_guard", wrap(`	if false { if lines > max { t.Fatalf(` + rule + `) } }
	t.Errorf("over")`), ratchetMessageRuleOnlyInDeadTripwire},
		{"red_unrecognized_receiver_assign_alias", wrap(`	tt := t
	tt.Fatalf(` + rule + `)`), ratchetMessageUnrecognizedReceiver},
		{"red_unrecognized_receiver_field", wrap(`	type h struct{ t *testing.T }
	x := h{t: t}
	x.t.Fatalf(` + rule + `)`), ratchetMessageUnrecognizedReceiver},
		{"red_unrecognized_receiver_foreign_type", file("\t\"log\"\n", `func probe(l *log.Logger) { l.Fatalf(`+rule+`) }`), ratchetMessageUnrecognizedReceiver},
		{"red_unrecognized_receiver_unresolved_name", file("", `func probe() { ghost.Errorf(`+rule+`) }`), ratchetMessageUnrecognizedReceiver},
		{"red_unparseable", []byte("package probe\nfunc {"), ratchetMessageUnparseable},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, tripwires, err := ratchetMessageCensus("probe_test.go", tc.src)
			if got != tc.want {
				t.Fatalf("shape = %d (tripwires=%d, err=%v), want %d", got, tripwires, err, tc.want)
			}
			msg := ratchetMessageFailure("probe_test.go", got, tripwires, err)
			if (msg == "") != (got == ratchetMessageBound) {
				t.Fatalf("message/shape disagree: shape %d, message %q", got, msg)
			}
			if got == ratchetMessageUnrecognizedReceiver && (err == nil || !strings.Contains(msg, err.Error())) {
				t.Fatalf("unrecognized receiver must name the offender in the message: err=%v msg=%q", err, msg)
			}
		})
	}
	defer func() {
		if recover() == nil {
			t.Fatalf("ratchetMessageFailure accepted an unrecognized shape silently")
		}
	}()
	_ = ratchetMessageFailure("probe_test.go", ratchetMessageShape(99), 0, nil)
}
