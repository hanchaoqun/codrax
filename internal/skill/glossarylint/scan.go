// Package glossarylint is the single scanner behind every "no internal
// jargon in model-facing text" lint in the repository. The vocabulary
// lives in internal/skill/glossary.go (InternalTermsBlocklist +
// ProjectSpecificIdentifierBlocklist); this package owns the matcher,
// the Go-source exclusion policy, and the shape-bound prompt-surface
// scan so that every renderer package runs the same gate instead of a
// hand-mirrored copy (§40.52: four matcher copies and one 22-entry term
// mirror were retired into this package).
//
// The package is imported by tests only. It deliberately depends on
// nothing but internal/skill (whose own dependency set is closed), so
// every renderer package — including packages that skill's consumers
// depend on — can import it from a test without an import cycle.
package glossarylint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

// Hit is one glossary token found in one scanned string (alias of the
// matcher's own type so both packages report the same shape).
type Hit = skill.GlossaryHit

// Terms returns the full glossary vocabulary (both blocklists).
func Terms() []string { return skill.GlossaryTerms() }

// Match locates term inside body (skill.MatchGlossaryTerm: whole-word
// for 2–4 letter uppercase acronyms, case-sensitive substring otherwise).
func Match(body, term string) int { return skill.MatchGlossaryTerm(body, term) }

// ScanText returns one Hit per glossary token found in s.
func ScanText(label, s string) []Hit { return skill.ScanText(label, s) }

// ScanTextWith scans s against the glossary PLUS per-surface extras.
func ScanTextWith(label, s string, extra ...string) []Hit {
	return skill.ScanTextWith(label, s, extra...)
}

// scanWithTerms scans s against an explicit term list with the shared
// matcher (used by the Go-source lanes, which resolve the list once).
func scanWithTerms(label, s string, terms []string) []Hit {
	if s == "" {
		return nil
	}
	var out []Hit
	for _, term := range terms {
		idx := Match(s, term)
		if idx < 0 {
			continue
		}
		out = append(out, Hit{Label: label, Term: term, Preview: preview(s, idx, len(term))})
	}
	return out
}

// preview returns a short single-line snippet of s centred on the match.
func preview(s string, start, length int) string {
	const window = 40
	from := start - window
	if from < 0 {
		from = 0
	}
	to := start + length + window
	if to > len(s) {
		to = len(s)
	}
	snippet := s[from:to]
	snippet = strings.ReplaceAll(snippet, "\n", " / ")
	snippet = strings.ReplaceAll(snippet, "\t", " ")
	return "…" + snippet + "…"
}

// ExemptReason is the closed set of reasons a source file may be left
// out of a package's static literal scan. A reason outside this set —
// or a row naming a file that no longer exists — fails the scan, so an
// exemption can never silently outlive the file it justified.
type ExemptReason string

const (
	// ExemptDataTable: the file is a repo-path data table matched as
	// data by the runtime (never rendered as prose to a model).
	ExemptDataTable ExemptReason = "data_table"
	// ExemptOperatorFacing: the file renders bilingual CLI/REPL
	// guidance to the human operator (config knob names, verbs); the
	// text never enters a model prompt.
	ExemptOperatorFacing ExemptReason = "operator_facing"
)

// Exemption names one file (basename, directly under the scanned dir)
// and the closed-set reason it is skipped.
type Exemption struct {
	File   string
	Reason ExemptReason
}

// Policy declares which structural literal positions a package's static
// scan skips. The always-on exclusions (direct logger arguments, import
// paths, struct field tags) are not options because no package has a
// model-facing string in those positions. The two optional exclusions
// exist for the orchestrator, whose const enum values and fail-loud
// *Error assignments are wire identifiers / operator diagnostics; a
// renderer package MUST NOT enable them casually — prose consts and
// StageOutput.Error assignments there do reach the next prompt.
type Policy struct {
	// SkipConstRHS excludes string literals that are values of const
	// declarations (typed enum constants such as "finalizer_only").
	SkipConstRHS bool
	// SkipErrorTargets excludes literals assigned to identifiers or
	// selectors whose name ends in "Error" (TaskState.LastError).
	SkipErrorTargets bool
	// Exempt lists files skipped entirely, each with a closed-set reason.
	Exempt []Exemption
}

// ScanGoDir statically scans every non-test .go file directly under dir
// (no recursion — sub-packages carry their own marker test) and returns
// one Hit per glossary token found in a string literal that the Policy
// does not exclude.
func ScanGoDir(dir string, p Policy) ([]Hit, error) {
	files, err := listGoFiles(dir, false)
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("glossarylint: no non-test .go files under %s", dir)
	}
	exempt := map[string]ExemptReason{}
	for _, e := range p.Exempt {
		switch e.Reason {
		case ExemptDataTable, ExemptOperatorFacing:
		default:
			return nil, fmt.Errorf("glossarylint: exemption %q has reason %q outside the closed set", e.File, e.Reason)
		}
		if _, statErr := os.Stat(filepath.Join(dir, e.File)); statErr != nil {
			return nil, fmt.Errorf("glossarylint: stale exemption row %q (%s): %v", e.File, e.Reason, statErr)
		}
		exempt[e.File] = e.Reason
	}
	terms := Terms()
	fset := token.NewFileSet()
	var hits []Hit
	for _, path := range files {
		if _, skip := exempt[filepath.Base(path)]; skip {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("glossarylint: parse %s: %v", path, err)
		}
		excluded := excludedLiteralPositions(file, p)
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || excluded[lit.Pos()] {
				return true
			}
			raw, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			pos := fset.Position(lit.Pos())
			hits = append(hits, scanWithTerms(pos.Filename+":"+strconv.Itoa(pos.Line), raw, terms)...)
			return true
		})
	}
	return hits, nil
}

// RunPackageScan is the marker every renderer package's lint test calls:
// it runs ScanGoDir and fails the test with every hit listed. The
// repo-wide roster census (TestRendererRosterTotality) recognises a
// package as covered by the presence of this call in one of its tests.
func RunPackageScan(t testing.TB, dir string, p Policy) {
	t.Helper()
	hits, err := ScanGoDir(dir, p)
	if err != nil {
		t.Fatalf("%v", err)
	}
	reportHits(t, "glossarylint.RunPackageScan", hits, "rephrase in user-facing language, or — for a wire identifier / operator diagnostic — extend the structural Policy; never allow-list a term")
}

func reportHits(t testing.TB, gate string, hits []Hit, teach string) {
	t.Helper()
	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("%s found %d violation(s); %s (vocabulary: internal/skill/glossary.go)", gate, len(hits), teach)
}

// excludedLiteralPositions marks the string literals the Policy skips.
func excludedLiteralPositions(file *ast.File, p Policy) map[token.Pos]bool {
	out := map[token.Pos]bool{}
	for _, imp := range file.Imports {
		if imp.Path != nil {
			out[imp.Path.Pos()] = true
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			if isLoggingCall(v.Fun) {
				for _, arg := range v.Args {
					markStringLits(arg, out)
				}
			}
		case *ast.Field:
			if v.Tag != nil {
				out[v.Tag.Pos()] = true
			}
		case *ast.GenDecl:
			if p.SkipConstRHS && v.Tok == token.CONST {
				for _, spec := range v.Specs {
					if vs, ok := spec.(*ast.ValueSpec); ok {
						for _, val := range vs.Values {
							markStringLits(val, out)
						}
					}
				}
			}
		case *ast.AssignStmt:
			if p.SkipErrorTargets {
				for i, lhs := range v.Lhs {
					if isErrorTargetExpr(lhs) && i < len(v.Rhs) {
						markStringLits(v.Rhs[i], out)
					}
				}
			}
		}
		return true
	})
	return out
}

// markStringLits flags every STRING literal directly inside expr,
// descending through parentheses and string concatenation only — never
// across a function-call boundary.
func markStringLits(expr ast.Expr, out map[token.Pos]bool) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			out[v.Pos()] = true
		}
	case *ast.ParenExpr:
		markStringLits(v.X, out)
	case *ast.BinaryExpr:
		markStringLits(v.X, out)
		markStringLits(v.Y, out)
	}
}

func isErrorTargetExpr(expr ast.Expr) bool {
	switch v := expr.(type) {
	case *ast.SelectorExpr:
		return strings.HasSuffix(v.Sel.Name, "Error")
	case *ast.Ident:
		return strings.HasSuffix(v.Name, "Error")
	}
	return false
}

// isLoggingCall reports whether the callee is a logger whose string
// arguments never leave the host process: logging.* (package logger),
// log.* (stdlib) and fmt.Fprint* (writer-directed). fmt.Sprintf /
// fmt.Errorf are deliberately NOT excluded — their results flow into
// model-facing fields.
func isLoggingCall(fn ast.Expr) bool {
	sel, ok := fn.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	if !ok {
		return false
	}
	name := sel.Sel.Name
	switch pkg.Name {
	case "logging":
		switch name {
		case "Debug", "Info", "Warning", "Warn", "Error", "Trace", "Fatalf", "Fatal", "Panicf":
			return true
		}
	case "log":
		switch name {
		case "Printf", "Println", "Print", "Fatalf", "Fatal", "Panicf", "Panic":
			return true
		}
	case "fmt":
		switch name {
		case "Fprintf", "Fprintln", "Fprint":
			return true
		}
	}
	return false
}

// listGoFiles returns the .go files directly under dir, sorted. With
// tests=false only non-test files are returned; with tests=true only
// _test.go files.
func listGoFiles(dir string, tests bool) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		if strings.HasSuffix(name, "_test.go") != tests {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	sort.Strings(out)
	return out, nil
}
