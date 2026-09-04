package glossarylint

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// llmImportPath is the package whose ToolSchema composite literals are
// model-facing by construction (Name / Description / Parameters are
// sent verbatim to the provider).
const llmImportPath = "github.com/hanchaoqun/codrax/internal/llm"

// PromptSurface is one shape-bound model-facing text found by
// ScanPromptSurfaces: a package-level `…Prompt` const/var, or one field
// of an llm.ToolSchema composite literal.
type PromptSurface struct {
	Label string // "<file>:<line> <owner>" — owner is the const name or "ToolSchema.<Field>"
	Text  string
}

// ScanPromptSurfaces finds every model-facing text that a package binds
// through one of three precise shapes and scans it against the glossary:
//
//  1. a package-level const or var whose name ends in "SystemPrompt"
//     (the repo's declaration convention for provider system prompts);
//  2. any composite literal of llm.ToolSchema, at package level or
//     inside a function — its Name / Description / Parameters fields;
//  3. any composite literal carrying `Role: "system"` — its Content
//     field, which is how every system prompt reaches the provider
//     (this binds prompts whose const name breaks the convention).
//
// Values must be string-literal shaped: a literal, a concatenation of
// literals, a package-level or same-function const/var of that shape,
// or json.RawMessage(<one of those>). In the Role:"system" lane a call
// operand inside a concatenation (the per-turn language-preference
// tail) is a recognised pass-through: its text is rendered by another
// roster package. Any other value expression — a parameter, a variable
// assigned more than once, a function result elsewhere, a positional
// element — is an unrecognized shape and returns an error naming the
// position, so a new way of building a prompt cannot slip past the
// gate silently (§40.50 ④: census walkers fail loud on unrecognized
// shapes). Packages that build schemas from Tool methods (agent) are
// covered by the full static scan plus the tool roster instead, and do
// not use this lane.
//
// This lane exists for packages that mix operator-facing text with
// model prompts (repl, cmd), where a whole-package literal scan would
// misfire on legitimate operator guidance, and for packages whose
// static Policy skips const values (orchestrator).
func ScanPromptSurfaces(dir string) ([]Hit, []PromptSurface, error) {
	files, err := listGoFiles(dir, false)
	if err != nil {
		return nil, nil, err
	}
	if len(files) == 0 {
		return nil, nil, fmt.Errorf("glossarylint: no non-test .go files under %s", dir)
	}
	fset := token.NewFileSet()
	parsed := make([]*ast.File, 0, len(files))
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return nil, nil, fmt.Errorf("glossarylint: parse %s: %v", path, err)
		}
		parsed = append(parsed, file)
	}
	pkgValues := packageLevelValues(parsed)

	var surfaces []PromptSurface
	var problems []string
	for _, file := range parsed {
		llmAlias := importAlias(file, llmImportPath)
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if !strings.HasSuffix(name.Name, "SystemPrompt") {
						continue
					}
					pos := fset.Position(name.Pos())
					label := fmt.Sprintf("%s:%d %s", pos.Filename, pos.Line, name.Name)
					if i >= len(vs.Values) {
						problems = append(problems, label+": prompt const without a value")
						continue
					}
					text, err := resolveStringExpr(vs.Values[i], pkgValues, false, 0)
					if err != nil {
						problems = append(problems, label+": "+err.Error())
						continue
					}
					surfaces = append(surfaces, PromptSurface{Label: label, Text: text})
				}
			}
		}
		// Composite literals are resolved with the enclosing function's
		// single-assignment locals layered over the package scope.
		for _, decl := range file.Decls {
			scope := pkgValues
			var body ast.Node = decl
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if fd.Body == nil {
					continue
				}
				scope = withLocalValues(pkgValues, fd.Body)
				body = fd.Body
			}
			ast.Inspect(body, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				pos := fset.Position(cl.Pos())
				switch {
				case llmAlias != "" && isSelectorType(cl.Type, llmAlias, "ToolSchema"):
					for _, elt := range cl.Elts {
						kv, ok := elt.(*ast.KeyValueExpr)
						if !ok {
							problems = append(problems, fmt.Sprintf("%s:%d ToolSchema: positional element is an unrecognized shape", pos.Filename, pos.Line))
							continue
						}
						key, ok := kv.Key.(*ast.Ident)
						if !ok {
							problems = append(problems, fmt.Sprintf("%s:%d ToolSchema: non-identifier key is an unrecognized shape", pos.Filename, pos.Line))
							continue
						}
						label := fmt.Sprintf("%s:%d ToolSchema.%s", pos.Filename, pos.Line, key.Name)
						text, err := resolveStringExpr(kv.Value, scope, false, 0)
						if err != nil {
							problems = append(problems, label+": "+err.Error())
							continue
						}
						surfaces = append(surfaces, PromptSurface{Label: label, Text: text})
					}
				case isSystemMessageLiteral(cl):
					label := fmt.Sprintf("%s:%d SystemMessage.Content", pos.Filename, pos.Line)
					content := keyValue(cl, "Content")
					if content == nil {
						problems = append(problems, label+": Role:\"system\" literal without a Content field")
						return true
					}
					text, err := resolveStringExpr(content, scope, true, 0)
					if err != nil {
						problems = append(problems, label+": "+err.Error())
						return true
					}
					surfaces = append(surfaces, PromptSurface{Label: label, Text: text})
				}
				return true
			})
		}
	}
	if len(problems) > 0 {
		sort.Strings(problems)
		return nil, nil, fmt.Errorf("glossarylint: %d unrecognized prompt-surface shape(s) — teach ScanPromptSurfaces the shape or bind the text through a literal-shaped const:\n  %s", len(problems), strings.Join(problems, "\n  "))
	}
	terms := Terms()
	var hits []Hit
	for _, s := range surfaces {
		hits = append(hits, scanWithTerms(s.Label, s.Text, terms)...)
	}
	return hits, surfaces, nil
}

// RunPromptSurfaceScan is the marker for the shape-bound lane: it runs
// ScanPromptSurfaces on dir, fails on unrecognized shapes, requires at
// least one surface (a package that declares none has no business
// calling this marker), and fails the test with every hit listed. It
// returns the surfaces so a package test can pin its expected roster.
func RunPromptSurfaceScan(t testing.TB, dir string) []PromptSurface {
	t.Helper()
	hits, surfaces, err := ScanPromptSurfaces(dir)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if len(surfaces) == 0 {
		t.Fatalf("glossarylint.RunPromptSurfaceScan: %s declares no `…SystemPrompt` const, no llm.ToolSchema literal and no Role:\"system\" message — remove the marker or bind the prompt through a recognized shape", dir)
	}
	reportHits(t, "glossarylint.RunPromptSurfaceScan", hits, "rephrase in user-facing language; never allow-list a term")
	return surfaces
}

// packageLevelValues indexes every package-level const/var name to its
// value expression across the package's files.
func packageLevelValues(files []*ast.File) map[string]ast.Expr {
	out := map[string]ast.Expr{}
	for _, file := range files {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || (gd.Tok != token.CONST && gd.Tok != token.VAR) {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, name := range vs.Names {
					if i < len(vs.Values) {
						out[name.Name] = vs.Values[i]
					}
				}
			}
		}
	}
	return out
}

// resolveStringExpr flattens a string-literal-shaped expression to its
// text. Recognized shapes: string literal; "+" concatenation; parens;
// a const/var in scope of a recognized shape; json.RawMessage(x) where
// x is a recognized shape. With passCalls, any other call operand is a
// pass-through contributing no text (system-message lane only).
// Anything else is an error.
func resolveStringExpr(expr ast.Expr, scope map[string]ast.Expr, passCalls bool, depth int) (string, error) {
	if depth > 16 {
		return "", fmt.Errorf("value resolution exceeded depth 16 (cyclic const?)")
	}
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind != token.STRING {
			return "", fmt.Errorf("non-string literal is an unrecognized shape")
		}
		raw, err := strconv.Unquote(v.Value)
		if err != nil {
			return "", fmt.Errorf("unquote literal: %v", err)
		}
		return raw, nil
	case *ast.ParenExpr:
		return resolveStringExpr(v.X, scope, passCalls, depth+1)
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", fmt.Errorf("binary %s is an unrecognized shape", v.Op)
		}
		left, err := resolveStringExpr(v.X, scope, passCalls, depth+1)
		if err != nil {
			return "", err
		}
		right, err := resolveStringExpr(v.Y, scope, passCalls, depth+1)
		if err != nil {
			return "", err
		}
		return left + right, nil
	case *ast.Ident:
		val, ok := scope[v.Name]
		if !ok {
			return "", fmt.Errorf("identifier %q is not a single-assignment const/var in scope (parameter, reassigned variable, or other package) — unrecognized shape", v.Name)
		}
		if val == nil {
			return "", fmt.Errorf("identifier %q is assigned more than once in this function — unrecognized shape", v.Name)
		}
		return resolveStringExpr(val, scope, passCalls, depth+1)
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "json" && sel.Sel.Name == "RawMessage" && len(v.Args) == 1 {
				return resolveStringExpr(v.Args[0], scope, passCalls, depth+1)
			}
		}
		if passCalls {
			return "", nil
		}
		return "", fmt.Errorf("call %s is an unrecognized shape", exprString(v.Fun))
	}
	return "", fmt.Errorf("%T is an unrecognized shape", expr)
}

// withLocalValues layers the function body's single-assignment locals
// (`x := expr`, `var x = expr`) over the package scope. A name assigned
// more than once maps to nil so resolution fails loud instead of
// guessing which value reached the literal.
func withLocalValues(pkg map[string]ast.Expr, body *ast.BlockStmt) map[string]ast.Expr {
	scope := make(map[string]ast.Expr, len(pkg))
	for k, v := range pkg {
		scope[k] = v
	}
	seen := map[string]int{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				id, ok := lhs.(*ast.Ident)
				if !ok || id.Name == "_" {
					continue
				}
				seen[id.Name]++
				if i < len(v.Rhs) && len(v.Lhs) == len(v.Rhs) {
					scope[id.Name] = v.Rhs[i]
				} else {
					scope[id.Name] = nil
				}
			}
		case *ast.ValueSpec:
			for i, name := range v.Names {
				seen[name.Name]++
				if i < len(v.Values) {
					scope[name.Name] = v.Values[i]
				} else {
					scope[name.Name] = nil
				}
			}
		}
		return true
	})
	for name, n := range seen {
		if n > 1 {
			scope[name] = nil
		}
	}
	return scope
}

// isSystemMessageLiteral reports whether cl carries Role: "system".
func isSystemMessageLiteral(cl *ast.CompositeLit) bool {
	role := keyValue(cl, "Role")
	lit, ok := role.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return false
	}
	raw, err := strconv.Unquote(lit.Value)
	return err == nil && raw == "system"
}

// keyValue returns the value bound to key in a keyed composite literal.
func keyValue(cl *ast.CompositeLit, key string) ast.Expr {
	for _, elt := range cl.Elts {
		kv, ok := elt.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == key {
			return kv.Value
		}
	}
	return nil
}

// importAlias returns the local name under which file imports path
// ("" when it does not). A dot import is reported as "." and never
// matches a selector type.
func importAlias(file *ast.File, path string) string {
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		p, err := strconv.Unquote(imp.Path.Value)
		if err != nil || p != path {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return path[strings.LastIndex(path, "/")+1:]
	}
	return ""
}

func isSelectorType(t ast.Expr, pkg, name string) bool {
	sel, ok := t.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	x, ok := sel.X.(*ast.Ident)
	return ok && x.Name == pkg && sel.Sel.Name == name
}

func exprString(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.SelectorExpr:
		return exprString(v.X) + "." + v.Sel.Name
	case *ast.CallExpr:
		return exprString(v.Fun) + "()"
	case *ast.StarExpr:
		return "*" + exprString(v.X)
	}
	return fmt.Sprintf("%T", e)
}
