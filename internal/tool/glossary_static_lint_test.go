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

	"github.com/hanchaoqun/codrax/internal/skill"
)

// TestNoInternalTermsInToolSourceLiterals is the AST-level twin of
// internal/agent/glossary_lint_test.go's TestNoInternalTermsInHints,
// scoped to internal/tool/. Tool source files carry LLM-facing text
// in places the schema-only gate (TestNoInternalTermsInToolSchemas)
// does not see: ToolResult.Summary error strings, per-tool emitFixHint
// Reason fields, mid-loop hint builders. Any blocklist hit in a
// non-logger string literal there is the same R6 leak as one in a
// skill Workflow item.
//
// Same logger-filter heuristic as the agent gate: string literals
// passed directly to logging.Debug/Info/Warning/Error/Trace,
// log.Printf, fmt.Fprintf are excluded because they never reach the
// LLM. The scan covers non-test .go files DIRECTLY under
// internal/tool/ — no recursion, matching the agent gate's scope.
func TestNoInternalTermsInToolSourceLiterals(t *testing.T) {
	files, err := listToolGoFiles(".")
	if err != nil {
		t.Fatalf("listToolGoFiles: %v", err)
	}
	if len(files) == 0 {
		t.Fatalf("no non-test .go files found under internal/tool/")
	}

	fset := token.NewFileSet()
	var hits []string
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			t.Errorf("parse %s: %v", path, err)
			continue
		}
		excluded := collectLoggedStringPositionsTool(file)
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
				idx := findTermMatchLocal(raw, term)
				if idx < 0 {
					continue
				}
				pos := fset.Position(lit.Pos())
				hits = append(hits, formatToolStaticHit(pos.Filename, pos.Line, term, raw, idx))
			}
			for _, term := range skill.ProjectSpecificIdentifierBlocklist {
				if term == "" {
					continue
				}
				idx := findTermMatchLocal(raw, term)
				if idx < 0 {
					continue
				}
				pos := fset.Position(lit.Pos())
				hits = append(hits, formatToolStaticHit(pos.Filename, pos.Line, term, raw, idx))
			}
			return true
		})
	}
	if len(hits) == 0 {
		return
	}
	for _, h := range hits {
		t.Errorf("  %s", h)
	}
	t.Fatalf("TestNoInternalTermsInToolSourceLiterals found %d violation(s) (logger-arg literals excluded); rephrase in user-facing language or extend internal/skill/glossary.go :: InternalTermsBlocklist", len(hits))
}

// collectLoggedStringPositionsTool walks the AST once and returns the
// positions of every STRING literal that appears as a direct argument
// to a logging-package call. The set is checked in the subsequent
// scan so those literals are not flagged as LLM-facing.
func collectLoggedStringPositionsTool(file *ast.File) map[token.Pos]bool {
	out := make(map[token.Pos]bool)
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if !isLoggingCallTool(call.Fun) {
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

// isLoggingCallTool returns true when the callee is a logger function
// whose string argument is not LLM-facing. fmt.Sprintf / fmt.Errorf
// are intentionally NOT skipped — their return values often flow to
// LLM-facing fields (e.g. ToolResult.Summary).
func isLoggingCallTool(fn ast.Expr) bool {
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

// toolStaticLintSkipFiles names .go files in internal/tool/ that carry
// load-bearing repo-path data tables (matched as data, not rendered as
// fixture prose). The static gate exempts them so the literal paths the
// runtime depends on do not have to be obfuscated into the data.
var toolStaticLintSkipFiles = map[string]bool{
	"wiring_anchors.go": true,
}

func listToolGoFiles(dir string) ([]string, error) {
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
		if toolStaticLintSkipFiles[name] {
			continue
		}
		out = append(out, filepath.Join(dir, name))
	}
	return out, nil
}

func formatToolStaticHit(file string, line int, term, body string, offset int) string {
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
