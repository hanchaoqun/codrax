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
// sent verbatim to the provider) and whose Message literals carry every
// prompt to the provider.
const llmImportPath = "github.com/hanchaoqun/codrax/internal/llm"

// PromptSurface is one shape-bound model-facing text found by
// ScanPromptSurfaces: a package-level `…SystemPrompt` const/var, one
// field of an llm.ToolSchema composite literal, or the Content of an
// llm.Message literal.
type PromptSurface struct {
	Label string // "<file>:<line> <owner>" — owner is the const name, "ToolSchema.<Field>" or "<Role>Message.Content"
	Text  string
	// parts are the literal positions bound through the instruction
	// walker (Role:"user" / "assistant" / "tool" Content and same-package
	// builder calls inside a system message); each is scanned under its
	// own file:line so a hit names the literal, not the message site.
	parts []surfacePart
}

// surfacePart is one string literal bound to a surface by text flow.
type surfacePart struct {
	pos  string
	text string
}

// ScanPromptSurfaces finds every model-facing text that a package binds
// through one of three precise shapes and scans it against the glossary:
//
//  1. a package-level const or var whose name ends in "SystemPrompt"
//     (the repo's declaration convention for provider system prompts);
//  2. any composite literal of llm.ToolSchema, at package level or
//     inside a function — its Name / Description / Parameters fields;
//  3. any llm.Message composite literal (typed `llm.Message{…}` or an
//     element of a `[]llm.Message{…}` literal) — its Content field, which
//     is how every prompt reaches the provider.
//
// Shapes 1–2 and a Role:"system" message resolve the text exactly: a
// literal, a concatenation of literals, a package-level or same-function
// single-assignment const/var of that shape, or json.RawMessage(<one of
// those>). Inside a system message a call operand of a concatenation
// that targets another package (the per-turn language-preference tail,
// rendered by another roster package) is a recognised pass-through; a
// call to a same-package function — as an operand or as the whole
// Content — binds that function as an instruction builder (below). Any
// other value expression — a parameter, a variable assigned more than
// once, another package's function result as the whole Content, a
// positional element — is an unrecognized shape and returns an error
// naming the position.
//
// A Role:"user" (or "assistant" / "tool") message is instruction text
// assembled at runtime, so its Content is bound by TEXT FLOW rather than
// resolved: the instruction walker follows the Content expression to
// every string literal that can reach it — literals and concatenations,
// every assignment of a local (single or repeated), the writes into a
// strings.Builder whose String() is the Content (including writes made
// by same-package callees the builder is passed to), the return values
// of same-package functions whose result flows in (transitively), the
// arguments of calls into other packages (fmt.Sprintf formats,
// strings.Join separators), and — for a parameter — the corresponding
// argument at every same-package call site. Field selectors, index
// expressions and struct literals are runtime data and contribute no
// literal; a parameter with no same-package caller is text supplied from
// outside the package. A function literal or an unresolvable identifier
// on the flow is an unrecognized shape and returns an error (§40.50 ④:
// census walkers fail loud on unrecognized shapes). An llm.Message
// literal whose Role is not a string literal, whose Role is outside the
// provider's four roles, or which has no Content field is likewise an
// error, and so is a Role:"system"/"user" literal of a type that is not
// llm.Message.
//
// Packages that build schemas from Tool methods (agent) are covered by
// the full static scan plus the tool roster instead, and do not use this
// lane. This lane exists for packages that mix operator-facing text with
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
	index := newPackageIndex(fset, parsed)

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
					r := &valueResolver{scope: pkgValues}
					text, err := r.resolve(vs.Values[i], false, 0)
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
			ctx := walkCtx{file: file}
			if fd, ok := decl.(*ast.FuncDecl); ok {
				if fd.Body == nil {
					continue
				}
				scope = withLocalValues(pkgValues, fd.Body)
				body = fd.Body
				ctx.fn = fd
			}
			messageLits := map[*ast.CompositeLit]bool{}
			ast.Inspect(body, func(n ast.Node) bool {
				cl, ok := n.(*ast.CompositeLit)
				if !ok {
					return true
				}
				pos := fset.Position(cl.Pos())
				if llmAlias != "" && isMessageSliceType(cl.Type, llmAlias) {
					for _, elt := range cl.Elts {
						if inner, ok := elt.(*ast.CompositeLit); ok && inner.Type == nil {
							messageLits[inner] = true
						}
					}
					return true
				}
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
						r := &valueResolver{scope: scope}
						text, err := r.resolve(kv.Value, false, 0)
						if err != nil {
							problems = append(problems, label+": "+err.Error())
							continue
						}
						surfaces = append(surfaces, PromptSurface{Label: label, Text: text})
					}
				case (llmAlias != "" && isSelectorType(cl.Type, llmAlias, "Message")) || messageLits[cl]:
					s, err := bindMessageLiteral(cl, pos, scope, ctx, index)
					if err != nil {
						problems = append(problems, err.Error())
						return true
					}
					surfaces = append(surfaces, s)
				case messageRoleLiteral(cl) != "":
					problems = append(problems, fmt.Sprintf("%s:%d Role:%q literal of a type other than llm.Message is an unrecognized message shape", pos.Filename, pos.Line, messageRoleLiteral(cl)))
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
		if len(s.parts) == 0 {
			hits = append(hits, scanWithTerms(s.Label, s.Text, terms)...)
			continue
		}
		owner := s.Label[strings.LastIndex(s.Label, " ")+1:]
		for _, p := range s.parts {
			hits = append(hits, scanWithTerms(p.pos+" "+owner, p.text, terms)...)
		}
	}
	return hits, surfaces, nil
}

// messageRoles is the provider's closed role set; system prompts resolve
// exactly, the other three are bound by text flow.
var messageRoles = map[string]bool{"system": true, "user": true, "assistant": true, "tool": true}

// bindMessageLiteral classifies one llm.Message literal by its Role and
// binds its Content through the matching lane.
func bindMessageLiteral(cl *ast.CompositeLit, pos token.Position, scope map[string]ast.Expr, ctx walkCtx, index *packageIndex) (PromptSurface, error) {
	role := keyValue(cl, "Role")
	if role == nil {
		return PromptSurface{}, fmt.Errorf("%s:%d llm.Message literal without a Role field is an unrecognized message shape", pos.Filename, pos.Line)
	}
	roleName := messageRoleLiteral(cl)
	if roleName == "" {
		return PromptSurface{}, fmt.Errorf("%s:%d llm.Message Role %s is not a string literal — unrecognized message shape (spell the role literally so the lane can classify the message)", pos.Filename, pos.Line, exprString(role))
	}
	if !messageRoles[roleName] {
		return PromptSurface{}, fmt.Errorf("%s:%d llm.Message Role %q is outside the provider role set — unrecognized message shape", pos.Filename, pos.Line, roleName)
	}
	owner := strings.ToUpper(roleName[:1]) + roleName[1:] + "Message.Content"
	label := fmt.Sprintf("%s:%d %s", pos.Filename, pos.Line, owner)
	content := keyValue(cl, "Content")
	if content == nil {
		return PromptSurface{}, fmt.Errorf("%s: Role:%q literal without a Content field is an unrecognized message shape", label, roleName)
	}
	if roleName == "system" {
		r := &valueResolver{scope: scope, walker: newInstructionWalker(index), ctx: ctx}
		text, err := r.resolve(content, false, 0)
		if err != nil {
			return PromptSurface{}, fmt.Errorf("%s: %v", label, err)
		}
		if len(r.walker.problems) > 0 {
			return PromptSurface{}, fmt.Errorf("%s: %s", label, strings.Join(r.walker.problems, "; "))
		}
		return PromptSurface{Label: label, Text: text, parts: r.walker.parts}, nil
	}
	w := newInstructionWalker(index)
	w.walk(content, ctx, 0)
	if len(w.problems) > 0 {
		return PromptSurface{}, fmt.Errorf("%s: %s", label, strings.Join(w.problems, "; "))
	}
	texts := make([]string, 0, len(w.parts))
	for _, p := range w.parts {
		texts = append(texts, p.text)
	}
	return PromptSurface{Label: label, Text: strings.Join(texts, "\n"), parts: w.parts}, nil
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
		t.Fatalf("glossarylint.RunPromptSurfaceScan: %s declares no `…SystemPrompt` const, no llm.ToolSchema literal and no llm.Message literal — remove the marker or bind the prompt through a recognized shape", dir)
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

// valueResolver flattens a string-literal-shaped expression to its text.
// Recognized shapes: string literal; "+" concatenation; parens; a
// const/var in scope of a recognized shape; json.RawMessage(x) where x is
// a recognized shape. With a walker (system-message lane), a call to a
// same-package function binds that function's text through the
// instruction walker (contributing parts, not text) and a call into
// another package is a pass-through — but only as a concatenation
// operand; as the whole value it is an unrecognized shape. Without a
// walker (const and ToolSchema lanes) every call is unrecognized.
type valueResolver struct {
	scope  map[string]ast.Expr
	walker *instructionWalker
	ctx    walkCtx
}

func (r *valueResolver) resolve(expr ast.Expr, inConcat bool, depth int) (string, error) {
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
		return r.resolve(v.X, inConcat, depth+1)
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return "", fmt.Errorf("binary %s is an unrecognized shape", v.Op)
		}
		left, err := r.resolve(v.X, true, depth+1)
		if err != nil {
			return "", err
		}
		right, err := r.resolve(v.Y, true, depth+1)
		if err != nil {
			return "", err
		}
		return left + right, nil
	case *ast.Ident:
		val, ok := r.scope[v.Name]
		if !ok {
			return "", fmt.Errorf("identifier %q is not a single-assignment const/var in scope (parameter, reassigned variable, or other package) — unrecognized shape", v.Name)
		}
		if val == nil {
			return "", fmt.Errorf("identifier %q is assigned more than once in this function — unrecognized shape", v.Name)
		}
		return r.resolve(val, inConcat, depth+1)
	case *ast.CallExpr:
		if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "json" && sel.Sel.Name == "RawMessage" && len(v.Args) == 1 {
				return r.resolve(v.Args[0], inConcat, depth+1)
			}
		}
		if r.walker == nil {
			return "", fmt.Errorf("call %s is an unrecognized shape", exprString(v.Fun))
		}
		if r.walker.samePackageCall(v, r.ctx) {
			// A same-package builder: its text is bound by flow.
			r.walker.walk(v, r.ctx, 0)
			return "", nil
		}
		if inConcat {
			return "", nil
		}
		return "", fmt.Errorf("call %s as the whole value is an unrecognized shape (a call into another package is a pass-through only as a concatenation operand)", exprString(v.Fun))
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

// isSystemMessageLiteral reports whether cl carries Role: "system"
// (the producer census shape, independent of the literal's type).
func isSystemMessageLiteral(cl *ast.CompositeLit) bool {
	return messageRoleLiteral(cl) == "system"
}

// isUserMessageLiteral reports whether cl carries Role: "user".
func isUserMessageLiteral(cl *ast.CompositeLit) bool {
	return messageRoleLiteral(cl) == "user"
}

// messageRoleLiteral returns the string literal bound to Role in cl, or
// "" when there is none or it is not a string literal.
func messageRoleLiteral(cl *ast.CompositeLit) string {
	role := keyValue(cl, "Role")
	lit, ok := role.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return ""
	}
	raw, err := strconv.Unquote(lit.Value)
	if err != nil {
		return ""
	}
	return raw
}

// isMessageSliceType reports whether t is `[]<alias>.Message`, whose
// elided-type elements are llm.Message literals.
func isMessageSliceType(t ast.Expr, alias string) bool {
	at, ok := t.(*ast.ArrayType)
	return ok && isSelectorType(at.Elt, alias, "Message")
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
