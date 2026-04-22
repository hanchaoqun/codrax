package agent

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/hanchaoqun/codrax/internal/skill"
)

// TestNoInternalTermsInHints scans non-test .go files under
// internal/agent/ for string literals that (a) contain a
// skill.InternalTermsBlocklist token AND (b) are not direct arguments
// to a logging-package call.
//
// The dominant concern is text that ends up in the LLM's prompt
// context — LoopSignal.Hint, BuildInitialInstruction returns, retry
// hints, mid-loop re-prompts, StageOutput.Error / .StageReport
// (orchestrator renders these into the next dispatch's Retry
// Directive). Internal logger messages that mention the same tokens
// (e.g. `logging.Debug("[CGEC] I1 ...")`) are noise — they never
// leave the host process, so they are filtered out.
//
// Filter heuristic: a string literal is skipped when it is a direct
// argument to a CallExpr whose receiver/function name matches the
// logging helper pattern (logging.Debug/Info/Warning/Error/Trace,
// log.Printf, fmt.Fprintf(os.Stderr,…), etc.). String literals that
// are concatenated inside expressions are still flagged because the
// concatenated parts may later be rendered to LLM text; the Contains
// check is on each literal in isolation.
//
// Comment-only mentions (Go line / block comments) are not
// BasicLit(STRING) nodes, so they are implicitly ignored.
//
// Batch 1 policy: report-only (t.Log). Batch 2B/4B promote cleaned
// categories to t.Errorf.
func TestNoInternalTermsInHints(t *testing.T) {
	files, err := listAgentGoFiles(".")
	if err != nil {
		t.Fatalf("listAgentGoFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no non-test .go files found under internal/agent/")
	}

	fset := token.NewFileSet()
	var hits []string
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		// Collect the positions of every STRING literal that is a
		// direct argument to a logging helper, or an ImportSpec
		// path. Those positions are excluded from the scan below.
		excluded := collectLoggedStringPositions(file)
		for _, imp := range file.Imports {
			if imp.Path != nil {
				excluded[imp.Path.Pos()] = true
			}
		}

		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if excluded[lit.Pos()] {
				return true
			}
			raw, err := strconv.Unquote(lit.Value)
			if err != nil {
				return true
			}
			for _, term := range skill.InternalTermsBlocklist {
				if term == "" {
					continue
				}
				idx := findTermMatch(raw, term)
				if idx < 0 {
					continue
				}
				pos := fset.Position(lit.Pos())
				hits = append(hits, formatHintHit(pos.Filename, pos.Line, term, raw, idx))
			}
			return true
		})
	}

	if len(hits) == 0 {
		return
	}

	t.Errorf("TestNoInternalTermsInHints found %d violation(s) (logger-arg literals excluded); see docs/prompt_glossary.md for replacements:", len(hits))
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
}

// collectLoggedStringPositions walks the AST once and returns the
// token positions of every STRING literal that appears as a direct
// argument to a logging helper call. The set is checked in the
// subsequent scan so those literals are not flagged as LLM-facing.
//
// The match list is intentionally narrow — only calls whose package
// selector matches a known logging surface. This avoids false
// positives where a non-logger function happens to take a string
// argument with the same shape.
func collectLoggedStringPositions(file *ast.File) map[token.Pos]bool {
	out := make(map[token.Pos]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isLoggingCall(call.Fun) {
			return true
		}
		for _, arg := range call.Args {
			if lit, ok := arg.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				out[lit.Pos()] = true
			}
		}
		return true
	})
	return out
}

// isLoggingCall returns true when the callee is a logger function
// whose first (format or message) string argument is not LLM-facing.
//
//   - logging.Debug / Info / Warning / Error / Trace / Fatalf   (package logger)
//   - log.Printf / Println / Print / Fatalf / Panicf            (stdlib)
//   - fmt.Fprintf / Fprintln                                    (when first arg is a Writer, not LLM)
//
// fmt.Sprintf / fmt.Errorf intentionally NOT skipped — their return
// values may flow to LLM-facing fields (e.g. `Error: fmt.Errorf(...)`).
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
		// Fprintf/Fprintln are typical logger surfaces when the
		// destination is os.Stderr / log file. Sprintf / Errorf are
		// NOT filtered because their results often flow to LLM.
		switch name {
		case "Fprintf", "Fprintln", "Fprint":
			return true
		}
	}
	return false
}

// listAgentGoFiles returns every .go file directly under dir that is
// not a test file. The lint does not recurse; sub-packages would be
// audited with their own test targets.
func listAgentGoFiles(dir string) ([]string, error) {
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
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out, nil
}

// findTermMatch locates term inside body. For short uppercase
// acronyms (≤4 letters, all uppercase) it requires word boundaries
// on both sides so "ERM" does not match inside "TERMINAL". For all
// other terms — which are longer phrases or mixed-case identifiers —
// plain substring matching is used.
func findTermMatch(body, term string) int {
	if isShortUpperAcronym(term) {
		return findWholeWord(body, term)
	}
	return strings.Index(body, term)
}

func isShortUpperAcronym(s string) bool {
	if len(s) > 4 {
		return false
	}
	for _, r := range s {
		if r < 'A' || r > 'Z' {
			return false
		}
	}
	return len(s) >= 2
}

func findWholeWord(body, term string) int {
	from := 0
	for {
		i := strings.Index(body[from:], term)
		if i < 0 {
			return -1
		}
		pos := from + i
		left := pos == 0 || !isWordChar(body[pos-1])
		end := pos + len(term)
		right := end == len(body) || !isWordChar(body[end])
		if left && right {
			return pos
		}
		from = pos + 1
		if from >= len(body) {
			return -1
		}
	}
}

func isWordChar(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z') || (b >= '0' && b <= '9') || b == '_'
}

func formatHintHit(file string, line int, term, body string, offset int) string {
	const window = 40
	from := offset - window
	if from < 0 {
		from = 0
	}
	to := offset + len(term) + window
	if to > len(body) {
		to = len(body)
	}
	snippet := body[from:to]
	snippet = strings.ReplaceAll(snippet, "\n", " / ")
	snippet = strings.ReplaceAll(snippet, "\t", " ")
	return file + ":" + strconv.Itoa(line) + ": token=" + term + " preview=…" + snippet + "…"
}
