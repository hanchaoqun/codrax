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
// field of an llm.ToolSchema composite literal, the Content of an
// llm.Message literal, or the Text of one Type:"text" element of its
// ContentParts.
//
// The text a surface binds lives in two sets that are BOTH scanned:
// `exact` is the text resolved at the surface site itself (literals,
// concatenations, consts — scanned under Label) and `parts` are the
// literal positions bound through the instruction walker (Role:"user" /
// "assistant" / "tool" text and same-package builder calls inside a
// system message — each scanned under its own file:line so a hit names
// the literal, not the message site). The two sets are made disjoint by
// literal position in messageLane.bind — a literal the resolver folded
// into `exact` never stays in `parts`, even when the walker re-reaches
// it through a same-package builder — so no hit is reported twice. Text
// is the union of the two for callers that pin a roster. EVOLUTION
// RECORD (batch six fold-in, F6-prompt-surface #5): the hit loop used to
// scan exact text only when no parts were bound, so a system Content of
// the form `someConst + samePackageBuilder()` reported the surface as
// bound while never scanning the const — a latent hard-gate blind spot.
type PromptSurface struct {
	Label string // "<file>:<line> <owner>" — owner is the const name, "ToolSchema.<Field>", "<Role>Message.Content" or "<Role>Message.ContentParts.Text"
	Text  string
	exact string
	parts []surfacePart
}

// withText fills Text from exact and parts.
func (s PromptSurface) withText() PromptSurface {
	texts := make([]string, 0, len(s.parts)+1)
	if s.exact != "" {
		texts = append(texts, s.exact)
	}
	for _, p := range s.parts {
		texts = append(texts, p.text)
	}
	s.Text = strings.Join(texts, "\n")
	return s
}

// surfacePart is one string literal bound to a surface by text flow.
type surfacePart struct {
	pos  string
	text string
	lit  token.Pos // the literal's own position — the disjointness key against the exact text
}

// ScanPromptSurfaces finds every model-facing text that a package binds
// through one of three precise shapes and scans it against the glossary:
//
//  1. a package-level const or var whose name ends in "SystemPrompt"
//     (the repo's declaration convention for provider system prompts);
//  2. any composite literal of llm.ToolSchema, at package level or
//     inside a function — its Name / Description / Parameters fields;
//  3. any llm.Message composite literal (typed `llm.Message{…}` or an
//     element of a `[]llm.Message{…}` literal) — its Content field and,
//     when it carries ContentParts, the Text of every Type:"text" part
//     (the adapter serializes both to the provider as text; each text
//     part is its own surface, "<Role>Message.ContentParts.Text"). The
//     part list must be a `[]llm.ContentPart{…}` literal of keyed
//     elements whose Type is a string literal: "text" / "" parts bind
//     their Text through the message's role lane, "image_url" / "image"
//     parts carry data only (an image part with a Text field, a part of
//     any other Type, a positional or non-literal part, or a part list
//     that is not such a literal is an unrecognized shape and returns an
//     error).
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
					surfaces = append(surfaces, PromptSurface{Label: label, exact: text}.withText())
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
						surfaces = append(surfaces, PromptSurface{Label: label, exact: text}.withText())
					}
				case (llmAlias != "" && isSelectorType(cl.Type, llmAlias, "Message")) || messageLits[cl]:
					bound, err := bindMessageLiteral(cl, pos, scope, ctx, index, llmAlias)
					if err != nil {
						problems = append(problems, err.Error())
						return true
					}
					surfaces = append(surfaces, bound...)
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
		// Both sets are scanned: the exactly-resolved text under the
		// surface label and every flow-bound literal under its own line.
		// They are disjoint by literal position: messageLane.bind drops
		// from parts every literal the resolver resolved exactly, so a
		// package-level const the walker re-reaches through a
		// same-package builder is reported once, at the surface label.
		hits = append(hits, scanWithTerms(s.Label, s.exact, terms)...)
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
// binds its Content — and the Text of every Type:"text" ContentParts
// element — through the matching lane. It returns one surface for the
// Content and one per text part.
func bindMessageLiteral(cl *ast.CompositeLit, pos token.Position, scope map[string]ast.Expr, ctx walkCtx, index *packageIndex, llmAlias string) ([]PromptSurface, error) {
	role := keyValue(cl, "Role")
	if role == nil {
		return nil, fmt.Errorf("%s:%d llm.Message literal without a Role field is an unrecognized message shape", pos.Filename, pos.Line)
	}
	roleName := messageRoleLiteral(cl)
	if roleName == "" {
		return nil, fmt.Errorf("%s:%d llm.Message Role %s is not a string literal — unrecognized message shape (spell the role literally so the lane can classify the message)", pos.Filename, pos.Line, exprString(role))
	}
	if !messageRoles[roleName] {
		return nil, fmt.Errorf("%s:%d llm.Message Role %q is outside the provider role set — unrecognized message shape", pos.Filename, pos.Line, roleName)
	}
	ownerPrefix := strings.ToUpper(roleName[:1]) + roleName[1:] + "Message."
	label := fmt.Sprintf("%s:%d %sContent", pos.Filename, pos.Line, ownerPrefix)
	content := keyValue(cl, "Content")
	if content == nil {
		return nil, fmt.Errorf("%s: Role:%q literal without a Content field is an unrecognized message shape", label, roleName)
	}
	lane := messageLane{system: roleName == "system", scope: scope, ctx: ctx, index: index}
	s, err := lane.bind(label, content)
	if err != nil {
		return nil, err
	}
	surfaces := []PromptSurface{s}
	if parts := keyValue(cl, "ContentParts"); parts != nil {
		partSurfaces, err := lane.bindContentParts(fmt.Sprintf("%s:%d %sContentParts", pos.Filename, pos.Line, ownerPrefix), ownerPrefix+"ContentParts.Text", parts, index, llmAlias)
		if err != nil {
			return nil, err
		}
		surfaces = append(surfaces, partSurfaces...)
	}
	return surfaces, nil
}

// messageLane binds one text-valued expression of an llm.Message literal
// the way its Role prescribes: the system lane resolves the value
// exactly (same-package builder calls contribute flow-bound parts), the
// user / assistant / tool lane binds it by text flow.
type messageLane struct {
	system bool
	scope  map[string]ast.Expr
	ctx    walkCtx
	index  *packageIndex
}

func (l messageLane) bind(label string, expr ast.Expr) (PromptSurface, error) {
	if l.system {
		r := &valueResolver{scope: l.scope, walker: newInstructionWalker(l.index), ctx: l.ctx}
		text, err := r.resolve(expr, false, 0)
		if err != nil {
			return PromptSurface{}, fmt.Errorf("%s: %v", label, err)
		}
		if len(r.walker.problems) > 0 {
			return PromptSurface{}, fmt.Errorf("%s: %s", label, strings.Join(r.walker.problems, "; "))
		}
		// The exact text and the flow-bound parts are kept disjoint by
		// literal position: a package-level const the resolver folded into
		// the exact text is reached again by the walker whenever a
		// same-package builder in the same value names it (walkIdent →
		// pkgValues → addLiteral), in either concatenation order, and
		// would otherwise be reported twice under this owner. EVOLUTION
		// RECORD (batch six fold-in, review round three #6): the lane
		// appended every walker part, so `const + builderReturningConst()`
		// reported one glossary token at the surface line AND at the
		// const's line.
		parts := make([]surfacePart, 0, len(r.walker.parts))
		for _, p := range r.walker.parts {
			if !r.resolved[p.lit] {
				parts = append(parts, p)
			}
		}
		return PromptSurface{Label: label, exact: text, parts: parts}.withText(), nil
	}
	w := newInstructionWalker(l.index)
	w.walk(expr, l.ctx, 0)
	if len(w.problems) > 0 {
		return PromptSurface{}, fmt.Errorf("%s: %s", label, strings.Join(w.problems, "; "))
	}
	return PromptSurface{Label: label, parts: w.parts}.withText(), nil
}

// contentPartTypes mirrors the adapter's part switch
// (llm.openAIRequestContent): the Type is trimmed and lower-cased, "text"
// and "" serialize Text as a text part, "image_url" and "image" serialize
// ImageURL / Detail as an image part; any other Type is dropped by the
// adapter and is therefore an unrecognized shape here.
var contentPartTypes = map[string]bool{"text": true, "": true, "image_url": false, "image": false}

// bindContentParts binds the Text of every text part of a ContentParts
// value. The value must be a `[]llm.ContentPart{…}` literal whose elements
// are keyed ContentPart literals with a string-literal Type — every other
// part shape fails loud (§40.50 ④). EVOLUTION RECORD (batch six fold-in,
// F6-prompt-surface #6): the lane used to read only the Content key, so a
// Type:"text" part's Text — serialized to the provider as text — was
// neither bound nor failed loud on.
func (l messageLane) bindContentParts(label, owner string, value ast.Expr, index *packageIndex, llmAlias string) ([]PromptSurface, error) {
	list, ok := value.(*ast.CompositeLit)
	if !ok || llmAlias == "" || !isContentPartSliceType(list.Type, llmAlias) {
		return nil, fmt.Errorf("%s bound to %s is an unrecognized shape (spell the parts as a []llm.ContentPart literal so each text part can be bound)", label, exprString(value))
	}
	var surfaces []PromptSurface
	for i, elt := range list.Elts {
		part, ok := elt.(*ast.CompositeLit)
		if !ok || (part.Type != nil && !isSelectorType(part.Type, llmAlias, "ContentPart")) {
			return nil, fmt.Errorf("%s element %d is not a ContentPart literal — unrecognized part shape", label, i+1)
		}
		partPos := index.fset.Position(part.Pos())
		partLabel := fmt.Sprintf("%s:%d %s", partPos.Filename, partPos.Line, owner)
		fields := map[string]ast.Expr{}
		for _, f := range part.Elts {
			kv, ok := f.(*ast.KeyValueExpr)
			if !ok {
				return nil, fmt.Errorf("%s: positional ContentPart element is an unrecognized part shape", partLabel)
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok {
				return nil, fmt.Errorf("%s: non-identifier ContentPart key is an unrecognized part shape", partLabel)
			}
			fields[key.Name] = kv.Value
		}
		typeExpr, ok := fields["Type"]
		if !ok {
			return nil, fmt.Errorf("%s: ContentPart without a Type field is an unrecognized part shape (spell Type literally so the lane can tell text from image parts)", partLabel)
		}
		typeLit, ok := typeExpr.(*ast.BasicLit)
		if !ok || typeLit.Kind != token.STRING {
			return nil, fmt.Errorf("%s: ContentPart Type %s is not a string literal — unrecognized part shape", partLabel, exprString(typeExpr))
		}
		typeName, err := strconv.Unquote(typeLit.Value)
		if err != nil {
			return nil, fmt.Errorf("%s: unquote ContentPart Type: %v", partLabel, err)
		}
		typeName = strings.ToLower(strings.TrimSpace(typeName))
		isText, known := contentPartTypes[typeName]
		if !known {
			return nil, fmt.Errorf("%s: ContentPart Type %q is outside the adapter's part set (text / image_url) — unrecognized part shape", partLabel, typeName)
		}
		text, hasText := fields["Text"]
		if !isText {
			if hasText {
				return nil, fmt.Errorf("%s: image ContentPart carrying a Text field is an unrecognized part shape (the adapter drops it; move the text to a text part or to Content)", partLabel)
			}
			continue
		}
		if !hasText {
			return nil, fmt.Errorf("%s: text ContentPart without a Text field is an unrecognized part shape", partLabel)
		}
		s, err := l.bind(partLabel, text)
		if err != nil {
			return nil, err
		}
		surfaces = append(surfaces, s)
	}
	return surfaces, nil
}

// isContentPartSliceType reports whether t is `[]<alias>.ContentPart`.
func isContentPartSliceType(t ast.Expr, alias string) bool {
	at, ok := t.(*ast.ArrayType)
	return ok && isSelectorType(at.Elt, alias, "ContentPart")
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
	// resolved records the position of every string literal folded into
	// the exact text, so the system lane can keep the walker's parts
	// disjoint from it (see messageLane.bind).
	resolved map[token.Pos]bool
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
		if r.resolved == nil {
			r.resolved = map[token.Pos]bool{}
		}
		r.resolved[v.Pos()] = true
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
