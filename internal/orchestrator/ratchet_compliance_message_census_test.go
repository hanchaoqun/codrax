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

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
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
	ratchetMessageBound                ratchetMessageShape = iota // the rule sits in a tripwire's format literal
	ratchetMessageUnparseable                                     // the file is not parseable Go
	ratchetMessageNoLiteralTripwire                               // no .Fatalf/.Errorf call has a string-literal format
	ratchetMessageRuleAbsentFromFormat                            // tripwires exist but none carries the rule in format position
)

// ratchetMessageCensus parses src and reports whether ratchetComplianceRule
// occurs inside the format-position string literal (a BasicLit, or a `+`
// chain of BasicLits) of at least one `<recv>.Fatalf(...)` / `<recv>.Errorf(...)`
// call. Placement anywhere else — comments, other declarations, non-format
// arguments — does not count. The int is the number of literal-format tripwires seen.
func ratchetMessageCensus(filename string, src []byte) (ratchetMessageShape, int, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filename, src, parser.ParseComments)
	if err != nil {
		return ratchetMessageUnparseable, 0, err
	}
	tripwires, bound := 0, false
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !ratchetTripwireMethods[sel.Sel.Name] {
			return true
		}
		format, ok := ratchetFormatLiteral(call.Args[0])
		if !ok {
			return true
		}
		tripwires++
		if strings.Contains(format, ratchetComplianceRule) {
			bound = true
		}
		return true
	})
	switch {
	case tripwires == 0:
		return ratchetMessageNoLiteralTripwire, 0, nil
	case !bound:
		return ratchetMessageRuleAbsentFromFormat, tripwires, nil
	}
	return ratchetMessageBound, tripwires, nil
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
	case ratchetMessageNoLiteralTripwire:
		return fmt.Sprintf("%s: no .Fatalf/.Errorf call with a string-literal format to bind the compliance rule to (a ratchet tripwire must state its rule in the format string a developer reads at trip time)", path)
	case ratchetMessageRuleAbsentFromFormat:
		return fmt.Sprintf("%s: none of its %d literal-format tripwire(s) carries the compliance rule verbatim in format position (a comment, an unused declaration, or a non-format argument does not count):\n  %s", path, tripwires, ratchetComplianceRule)
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
// substring accepted (comment, unused var, non-format argument) and the
// live format-position placements that must stay green.
func TestRatchetMessageCensusShapes(t *testing.T) {
	t.Parallel()
	rule := strconv.Quote(ratchetComplianceRule)
	wrap := func(body string) []byte {
		return []byte("package probe\n\nimport \"testing\"\n\nfunc TestProbe(t *testing.T) {\n\tlines, max := 2, 1\n\t_ = lines\n\t_ = max\n" + body + "\n}\n")
	}
	cases := []struct {
		name string
		src  []byte
		want ratchetMessageShape
	}{
		{"green_fatalf_format", wrap(`	if lines > max { t.Fatalf("%d lines over %d. " + ` + rule + `, lines, max) }`), ratchetMessageBound},
		{"green_errorf_format_single_literal", wrap(`	t.Errorf(` + strconv.Quote("over ceiling. "+ratchetComplianceRule) + `)`), ratchetMessageBound},
		{"green_subtest_receiver", wrap(`	t.Run("x", func(tt *testing.T) { tt.Fatalf(("a" + ` + rule + `), 1) })`), ratchetMessageBound},
		{"red_rule_in_comment_only", wrap(`	// ` + ratchetComplianceRule + `
	t.Fatalf("%d lines over %d", lines, max)`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_in_unused_var", wrap(`	var _ = ` + rule + `
	t.Fatalf("%d lines over %d", lines, max)`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_as_non_format_argument", wrap(`	t.Fatalf("%d lines over %d: %s", lines, max, ` + rule + `)`), ratchetMessageRuleAbsentFromFormat},
		{"red_rule_in_fatal_not_fatalf", wrap(`	t.Fatal(` + rule + `)`), ratchetMessageNoLiteralTripwire},
		{"red_rule_behind_identifier_format", wrap(`	msg := ` + rule + `
	t.Fatalf(msg)`), ratchetMessageNoLiteralTripwire},
		{"red_no_tripwire_at_all", wrap(`	// ` + ratchetComplianceRule), ratchetMessageNoLiteralTripwire},
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
		})
	}
	defer func() {
		if recover() == nil {
			t.Fatalf("ratchetMessageFailure accepted an unrecognized shape silently")
		}
	}()
	_ = ratchetMessageFailure("probe_test.go", ratchetMessageShape(99), 0, nil)
}
