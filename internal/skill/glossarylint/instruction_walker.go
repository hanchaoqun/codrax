package glossarylint

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
)

// instruction_walker.go — the text-flow binder behind the prompt-surface
// lane's Role:"user" / "assistant" / "tool" messages and the same-package
// builder calls inside a system message (§40.52 fold-in, G6-jargon #1/#2).
//
// Runtime-assembled instruction text cannot be resolved to one string,
// so the walker binds it by TEXT FLOW: starting from the Content
// expression it collects every string literal that can become part of
// the text — through concatenation, every assignment of a local, the
// writes into a strings.Builder / io.Writer whose String() is the
// Content (following the builder into same-package callees it is
// passed to), the return values of same-package functions whose result
// flows in (only the result position that was bound, for a multi-value
// call, with the callee's parameters bound to THAT call's arguments),
// the arguments of the standard library's text-composition functions
// (fmt.Sprintf formats, strings.Join separators), and — for a parameter
// of the surface's own function — the matching argument at every
// same-package call site. Runtime data (field selectors, index
// expressions, struct literals, other packages' values and the results
// of other packages' calls) contributes no literal. Shapes the walker
// does not know are recorded as problems so the lane fails loud instead
// of narrowing its scope silently.
//
// Four rules keep the flow precise without type information:
//
//   - Locals resolve by lexical scope and position (instruction_scope.go),
//     never by name alone.
//   - A same-package callee is walked per call site: its parameters are
//     bound to the arguments of the call that reached it, so
//     `readInputPair(promptTag)` contributes the typed line, not the
//     prompt decoration passed in.
//   - The parameter hop — binding a parameter of the surface's own
//     function to the argument of EVERY same-package caller — is the one
//     context-insensitive step, so it is taken once per flow path: the
//     argument found at a call site is walked with backward hops
//     disabled, except that an argument which is itself a bare
//     parameter of the caller (pure forwarding) keeps hopping until the
//     authoring level is reached. A parameter met with hops disabled,
//     or one with no same-package caller, is text supplied by a caller
//     outside the bound flow.
//   - A method call resolves to same-package methods only when the
//     receiver's package-local type is recoverable by shape (or the bare
//     name is unique in the package); an ambiguous name on an unknown
//     receiver is another package's method.

// walkCtx is the function (nil at package level) and file an expression
// sits in; both are needed to resolve locals and import aliases.
// noBackward marks a flow that has already taken its parameter hop;
// site is the call through which the function was entered (nil for the
// surface's own function and for functions reached by a hop).
type walkCtx struct {
	fn         *ast.FuncDecl
	file       *ast.File
	noBackward bool
	site       *callSite
}

// callSite is one call expression and the context it is evaluated in.
type callSite struct {
	call   *ast.CallExpr
	caller walkCtx
}

type funcRef struct {
	fn   *ast.FuncDecl
	file *ast.File
}

type callRef struct {
	call *ast.CallExpr
	ctx  walkCtx
}

// paramInfo locates one parameter of a function.
type paramInfo struct {
	index    int
	variadic bool
}

// binding is one value bound to a local name: the expression and, for a
// multi-value call (`x, err := f()`), the result position the name
// receives (-1 for a single value).
type binding struct {
	expr ast.Expr
	idx  int
}

// textPackages are the standard-library packages whose functions
// compose their string result from their arguments, so an argument is
// on the text flow. Every other package's call (a store read, a
// renderer, a reader) returns its own data: its arguments are not text.
var textPackages = map[string]bool{
	"fmt": true, "strings": true, "strconv": true, "bytes": true, "errors": true,
	"path": true, "filepath": true, "unicode": true, "utf8": true,
}

// packageIndex is the per-package function / call-site / import index
// shared by every walker of one ScanPromptSurfaces run.
type packageIndex struct {
	fset      *token.FileSet
	funcs     map[string][]funcRef // functions (no receiver) by name
	methods   map[string][]funcRef // methods by bare name (any receiver)
	callers   map[string][]callRef // callee bare name → same-package call sites
	types     map[string]bool      // package-level type names (conversions)
	imports   map[*ast.File]map[string]bool
	pkgValues map[string]ast.Expr
	pkgFiles  map[string]*ast.File // package-level value name → declaring file
	pkgTypes  map[string]string    // package-level var name → declared type name
	scopes    map[*ast.FuncDecl]*fnScope
}

func newPackageIndex(fset *token.FileSet, files []*ast.File) *packageIndex {
	idx := &packageIndex{
		fset:      fset,
		funcs:     map[string][]funcRef{},
		methods:   map[string][]funcRef{},
		callers:   map[string][]callRef{},
		types:     map[string]bool{},
		imports:   map[*ast.File]map[string]bool{},
		pkgValues: packageLevelValues(files),
		pkgFiles:  map[string]*ast.File{},
		pkgTypes:  map[string]string{},
		scopes:    map[*ast.FuncDecl]*fnScope{},
	}
	for _, file := range files {
		names := map[string]bool{}
		for _, imp := range file.Imports {
			if imp.Path == nil {
				continue
			}
			p, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				continue
			}
			local := p[lastSlash(p)+1:]
			if imp.Name != nil {
				local = imp.Name.Name
			}
			names[local] = true
		}
		idx.imports[file] = names
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.FuncDecl:
				ref := funcRef{fn: d, file: file}
				if d.Recv == nil {
					idx.funcs[d.Name.Name] = append(idx.funcs[d.Name.Name], ref)
				} else {
					idx.methods[d.Name.Name] = append(idx.methods[d.Name.Name], ref)
				}
			case *ast.GenDecl:
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						idx.types[s.Name.Name] = true
					case *ast.ValueSpec:
						for i, name := range s.Names {
							idx.pkgFiles[name.Name] = file
							if s.Type != nil {
								idx.pkgTypes[name.Name] = typeNameOf(s.Type)
							} else if i < len(s.Values) {
								idx.pkgTypes[name.Name] = literalTypeName(s.Values[i])
							}
						}
					}
				}
			}
		}
		// Call sites, keyed by callee bare name, with their enclosing
		// function so an argument can be walked in the caller's scope.
		var current *ast.FuncDecl
		var visit func(n ast.Node) bool
		visit = func(n ast.Node) bool {
			switch v := n.(type) {
			case *ast.FuncDecl:
				saved := current
				current = v
				if v.Body != nil {
					ast.Inspect(v.Body, visit)
				}
				current = saved
				return false
			case *ast.CallExpr:
				name := ""
				switch fun := v.Fun.(type) {
				case *ast.Ident:
					name = fun.Name
				case *ast.SelectorExpr:
					name = fun.Sel.Name
				}
				if name != "" {
					idx.callers[name] = append(idx.callers[name], callRef{call: v, ctx: walkCtx{fn: current, file: file}})
				}
			}
			return true
		}
		ast.Inspect(file, visit)
	}
	return idx
}

func lastSlash(s string) int {
	for i := len(s) - 1; i >= 0; i-- {
		if s[i] == '/' {
			return i
		}
	}
	return -1
}

func (idx *packageIndex) isImport(file *ast.File, name string) bool {
	return file != nil && idx.imports[file][name]
}

// local resolves an identifier to the function-scope declaration visible
// at its position (nil at package level or for a package-level name).
func (idx *packageIndex) local(id *ast.Ident, ctx walkCtx) *varDecl {
	if ctx.fn == nil {
		return nil
	}
	return idx.scope(ctx.fn).lookup(id.Name, id.Pos())
}

// instructionWalker collects the literals bound to one surface. Its
// visited sets are per surface so every message reports its own parts;
// the package index is shared.
type instructionWalker struct {
	idx       *packageIndex
	seenExpr  map[exprKey]bool
	seenRet   map[retKey]bool
	seenLit   map[*ast.FuncLit]bool
	seenParam map[*varDecl]bool
	seenWrite map[writeKey]bool
	seenPos   map[token.Pos]bool
	parts     []surfacePart
	problems  []string
}

type exprKey struct {
	expr       ast.Expr
	noBackward bool
	site       *ast.CallExpr
}

type retKey struct {
	fn         *ast.FuncDecl
	idx        int
	noBackward bool
	site       *ast.CallExpr
}

type writeKey struct {
	decl       *varDecl
	noBackward bool
	site       *ast.CallExpr
}

func newInstructionWalker(idx *packageIndex) *instructionWalker {
	return &instructionWalker{
		idx:       idx,
		seenExpr:  map[exprKey]bool{},
		seenRet:   map[retKey]bool{},
		seenLit:   map[*ast.FuncLit]bool{},
		seenParam: map[*varDecl]bool{},
		seenWrite: map[writeKey]bool{},
		seenPos:   map[token.Pos]bool{},
	}
}

func siteCall(ctx walkCtx) *ast.CallExpr {
	if ctx.site == nil {
		return nil
	}
	return ctx.site.call
}

func (w *instructionWalker) problem(n ast.Node, format string, args ...any) {
	p := w.idx.fset.Position(n.Pos())
	w.problems = append(w.problems, fmt.Sprintf("%s:%d %s", p.Filename, p.Line, fmt.Sprintf(format, args...)))
}

func (w *instructionWalker) addLiteral(lit *ast.BasicLit) {
	if lit.Kind != token.STRING || w.seenPos[lit.Pos()] {
		return
	}
	w.seenPos[lit.Pos()] = true
	raw, err := strconv.Unquote(lit.Value)
	if err != nil {
		return
	}
	p := w.idx.fset.Position(lit.Pos())
	w.parts = append(w.parts, surfacePart{pos: p.Filename + ":" + strconv.Itoa(p.Line), text: raw, lit: lit.Pos()})
}

// universeIdents are the predeclared identifiers that carry no text.
var universeIdents = map[string]bool{"nil": true, "true": true, "false": true, "iota": true}

// builtinDataCalls are builtins whose result is not text.
var builtinDataCalls = map[string]bool{
	"len": true, "cap": true, "make": true, "new": true, "panic": true, "min": true, "max": true,
	"delete": true, "close": true, "complex": true, "real": true, "imag": true, "print": true,
	"println": true, "recover": true, "clear": true, "int": true, "int8": true, "int16": true,
	"int32": true, "int64": true, "uint": true, "uint8": true, "uint16": true, "uint32": true,
	"uint64": true, "uintptr": true, "float32": true, "float64": true, "bool": true, "byte": true,
	"rune": true, "error": true, "any": true,
}

// walk follows expr wherever text can flow from it.
func (w *instructionWalker) walk(expr ast.Expr, ctx walkCtx, depth int) {
	if expr == nil || depth > 256 {
		return
	}
	key := exprKey{expr: expr, noBackward: ctx.noBackward, site: siteCall(ctx)}
	if w.seenExpr[key] {
		return
	}
	w.seenExpr[key] = true
	switch v := expr.(type) {
	case *ast.BasicLit:
		w.addLiteral(v)
	case *ast.ParenExpr:
		w.walk(v.X, ctx, depth+1)
	case *ast.BinaryExpr:
		w.walk(v.X, ctx, depth+1)
		w.walk(v.Y, ctx, depth+1)
	case *ast.UnaryExpr:
		w.walk(v.X, ctx, depth+1)
	case *ast.StarExpr:
		w.walk(v.X, ctx, depth+1)
	case *ast.KeyValueExpr:
		w.walk(v.Value, ctx, depth+1)
	case *ast.CompositeLit:
		switch v.Type.(type) {
		case *ast.ArrayType, *ast.MapType, nil:
			for _, elt := range v.Elts {
				w.walk(elt, ctx, depth+1)
			}
		default:
			// A struct literal is a runtime record, not text.
		}
	case *ast.IndexExpr:
		w.walk(v.X, ctx, depth+1)
	case *ast.SliceExpr:
		w.walk(v.X, ctx, depth+1)
	case *ast.TypeAssertExpr:
		w.walk(v.X, ctx, depth+1)
	case *ast.IndexListExpr:
		// generic instantiation: no text
	case *ast.SelectorExpr:
		// pkg.Value from another package is rendered and linted there;
		// x.Field is runtime data.
	case *ast.Ident:
		w.walkIdent(v, ctx, depth)
	case *ast.CallExpr:
		w.walkCall(v, ctx, depth, -1)
	case *ast.FuncLit:
		// A closure on the flow (passed to a runner, bound to a local):
		// its text leaves through its returns.
		w.walkFuncLitReturns(v, ctx, depth)
	default:
		w.problem(expr, "%T on the prompt-text flow is an unrecognized shape", expr)
	}
}

// walkBinding walks one value bound to a local, narrowing a multi-value
// call to the bound result position.
func (w *instructionWalker) walkBinding(b binding, ctx walkCtx, depth int) {
	if b.expr == nil {
		return
	}
	if call, ok := b.expr.(*ast.CallExpr); ok && b.idx >= 0 {
		w.walkCall(call, ctx, depth+1, b.idx)
		return
	}
	w.walk(b.expr, ctx, depth+1)
}

// walkDecl walks everything bound to a function-scope declaration.
func (w *instructionWalker) walkDecl(d *varDecl, ctx walkCtx, depth int) {
	switch {
	case d.param != nil:
		if ctx.site != nil {
			w.walkSiteArg(*d.param, ctx.site, depth)
		} else if !ctx.noBackward {
			w.walkParam(d, ctx, depth)
		}
	case d.data:
		// receiver / closure parameter: runtime data
	default:
		for _, b := range d.values {
			w.walkBinding(b, ctx, depth)
		}
	}
}

func (w *instructionWalker) walkIdent(id *ast.Ident, ctx walkCtx, depth int) {
	name := id.Name
	if name == "_" || universeIdents[name] {
		return
	}
	if d := w.idx.local(id, ctx); d != nil {
		w.walkDecl(d, ctx, depth)
		return
	}
	if val, ok := w.idx.pkgValues[name]; ok {
		w.walk(val, walkCtx{file: w.idx.pkgFiles[name], noBackward: ctx.noBackward}, depth+1)
		return
	}
	if len(w.idx.funcs[name]) > 0 || w.idx.types[name] {
		return // a function or type used as a value is not text
	}
	if _, declared := w.idx.pkgTypes[name]; declared {
		return // a package-level var without a value expression
	}
	w.problem(id, "identifier %q is not resolvable in the package or the enclosing function — unrecognized shape", name)
}

// walkSiteArg walks the argument bound to a parameter by the call that
// entered the function (no hop is consumed: the argument sits at the
// caller's own level).
func (w *instructionWalker) walkSiteArg(info paramInfo, site *callSite, depth int) {
	args := site.call.Args
	if len(args) == 1 && info.index > 0 {
		if inner, ok := args[0].(*ast.CallExpr); ok {
			w.walkCall(inner, site.caller, depth+1, info.index) // f(g()) multi-value spread
			return
		}
	}
	if info.variadic {
		for i := info.index; i < len(args); i++ {
			w.walk(args[i], site.caller, depth+1)
		}
		return
	}
	if info.index < len(args) {
		w.walk(args[info.index], site.caller, depth+1)
	}
}

// walkParam binds a parameter of the surface's own function to the
// matching argument at every same-package call site (the one backward
// hop of a flow path; see the file comment). A parameter with no such
// caller is text supplied from outside the package.
func (w *instructionWalker) walkParam(d *varDecl, ctx walkCtx, depth int) {
	if w.seenParam[d] || ctx.fn == nil {
		return
	}
	w.seenParam[d] = true
	info := *d.param
	for _, site := range w.callSites(ctx.fn) {
		args := site.call.Args
		if len(args) == 1 && len(w.idx.scope(ctx.fn).params) > 1 {
			if inner, ok := args[0].(*ast.CallExpr); ok {
				w.walkArgument(inner, site.ctx, depth) // f(g()) multi-value spread
				continue
			}
		}
		if info.variadic {
			for i := info.index; i < len(args); i++ {
				w.walkArgument(args[i], site.ctx, depth)
			}
			continue
		}
		if info.index < len(args) {
			w.walkArgument(args[info.index], site.ctx, depth)
		}
	}
}

// walkArgument walks the argument found at a hop call site: a bare
// parameter of the caller is pure forwarding and keeps the hop open;
// anything else is the authoring level and is walked with backward hops
// disabled.
func (w *instructionWalker) walkArgument(arg ast.Expr, site walkCtx, depth int) {
	if id, ok := arg.(*ast.Ident); ok {
		if d := w.idx.local(id, site); d != nil && d.param != nil {
			w.walkParam(d, site, depth+1)
			return
		}
	}
	site.noBackward = true
	w.walk(arg, site, depth+1)
}

// callSites returns the same-package call sites compatible with fn: a
// plain identifier call for a function, a selector call for a method
// whose receiver resolves to fn's type (or is unresolvable).
func (w *instructionWalker) callSites(fn *ast.FuncDecl) []callRef {
	var out []callRef
	for _, site := range w.idx.callers[fn.Name.Name] {
		switch fun := site.call.Fun.(type) {
		case *ast.Ident:
			if fn.Recv == nil {
				out = append(out, site)
			}
		case *ast.SelectorExpr:
			if fn.Recv == nil {
				continue
			}
			if x, ok := fun.X.(*ast.Ident); ok && w.idx.isImport(site.ctx.file, x.Name) {
				continue
			}
			if typ := w.receiverTypeName(fun.X, site.ctx); typ != "" && typ != recvTypeName(fn) {
				continue
			}
			out = append(out, site)
		}
	}
	return out
}

// samePackageCall reports whether call targets a function or method
// declared in this package (the system lane binds such calls as
// instruction builders rather than passing them through).
func (w *instructionWalker) samePackageCall(call *ast.CallExpr, ctx walkCtx) bool {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return len(w.idx.funcs[fun.Name]) > 0
	case *ast.SelectorExpr:
		return len(w.methodRefs(fun, ctx)) > 0
	}
	return false
}

// methodRefs resolves a method call to the same-package methods it can
// target. Without type information the receiver type is recovered by
// shape: the enclosing function's receiver, a parameter's or local's
// declared type, a local bound to `T{…}` / `&T{…}` / `new(T)` / a
// same-package constructor's single result, or a package-level var.
// When the receiver type is known only methods of that type match; when
// it is not (a field, a call result) the bare name matches only if the
// package declares exactly one method of that name — an ambiguous name
// on an unknown receiver is treated as another package's method rather
// than unioning every same-named method's text into the surface.
func (w *instructionWalker) methodRefs(fun *ast.SelectorExpr, ctx walkCtx) []funcRef {
	if x, ok := fun.X.(*ast.Ident); ok && w.idx.isImport(ctx.file, x.Name) {
		return nil
	}
	refs := w.idx.methods[fun.Sel.Name]
	if len(refs) == 0 {
		return nil
	}
	if typ := w.receiverTypeName(fun.X, ctx); typ != "" {
		var out []funcRef
		for _, ref := range refs {
			if recvTypeName(ref.fn) == typ {
				out = append(out, ref)
			}
		}
		return out
	}
	if len(refs) == 1 {
		return refs
	}
	return nil
}

// receiverTypeName recovers the package-local type name of expr ("" when
// unknown or from another package).
func (w *instructionWalker) receiverTypeName(expr ast.Expr, ctx walkCtx) string {
	switch v := expr.(type) {
	case *ast.ParenExpr:
		return w.receiverTypeName(v.X, ctx)
	case *ast.UnaryExpr:
		return w.receiverTypeName(v.X, ctx)
	case *ast.CompositeLit:
		return typeNameOf(v.Type)
	case *ast.CallExpr:
		if id, ok := v.Fun.(*ast.Ident); ok {
			if id.Name == "new" && len(v.Args) == 1 {
				return typeNameOf(v.Args[0])
			}
			if refs := w.idx.funcs[id.Name]; len(refs) == 1 {
				if res := refs[0].fn.Type.Results; res != nil && len(res.List) == 1 && len(res.List[0].Names) == 0 {
					return typeNameOf(res.List[0].Type)
				}
			}
		}
	case *ast.Ident:
		if d := w.idx.local(v, ctx); d != nil {
			if d.typ != "" {
				return d.typ
			}
			if len(d.values) == 1 && d.values[0].expr != nil && d.values[0].idx < 0 {
				return w.receiverTypeName(d.values[0].expr, ctx)
			}
			return ""
		}
		return w.idx.pkgTypes[v.Name]
	}
	return ""
}

// typeNameOf returns the package-local name of a type expression (`T`,
// `*T`), or "" for another package's type or a literal type.
func typeNameOf(t ast.Expr) string {
	switch v := t.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.StarExpr:
		return typeNameOf(v.X)
	case *ast.ParenExpr:
		return typeNameOf(v.X)
	}
	return ""
}

// literalTypeName returns the type name of a `T{…}` / `&T{…}` /
// `new(T)` value expression, or "".
func literalTypeName(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.CompositeLit:
		return typeNameOf(v.Type)
	case *ast.UnaryExpr:
		return literalTypeName(v.X)
	case *ast.CallExpr:
		if id, ok := v.Fun.(*ast.Ident); ok && id.Name == "new" && len(v.Args) == 1 {
			return typeNameOf(v.Args[0])
		}
	}
	return ""
}

// recvTypeName returns the receiver type name of a method declaration.
func recvTypeName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) != 1 {
		return ""
	}
	t := fn.Recv.List[0].Type
	if star, ok := t.(*ast.StarExpr); ok {
		t = star.X
	}
	switch v := t.(type) {
	case *ast.IndexExpr:
		return typeNameOf(v.X)
	case *ast.IndexListExpr:
		return typeNameOf(v.X)
	}
	return typeNameOf(t)
}

func (w *instructionWalker) walkArgs(call *ast.CallExpr, ctx walkCtx, depth int) {
	for _, arg := range call.Args {
		w.walk(arg, ctx, depth+1)
	}
}

// walkCall follows a call. A same-package callee contributes the bound
// result position of its returns (all positions when resultIdx < 0),
// with its parameters bound to this call's arguments; a text-composing
// standard-library function contributes its arguments; any other call
// (another package's function or method, an unresolvable method)
// returns its own data.
func (w *instructionWalker) walkCall(call *ast.CallExpr, ctx walkCtx, depth int, resultIdx int) {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		switch {
		case fun.Name == "append" || fun.Name == "string" || fun.Name == "copy":
			w.walkArgs(call, ctx, depth)
		case builtinDataCalls[fun.Name]:
		case w.idx.local(fun, ctx) != nil:
			w.localClosure(w.idx.local(fun, ctx), ctx, depth)
			w.walkArgs(call, ctx, depth)
		case len(w.idx.funcs[fun.Name]) > 0:
			for _, ref := range w.idx.funcs[fun.Name] {
				w.walkReturns(ref, call, ctx, resultIdx)
			}
		case w.idx.types[fun.Name]:
			w.walkArgs(call, ctx, depth) // conversion to a package type
		default:
			w.problem(call, "call to %q is not resolvable in the package or the enclosing function — unrecognized shape", fun.Name)
		}
	case *ast.SelectorExpr:
		if x, ok := fun.X.(*ast.Ident); ok {
			if w.idx.isImport(ctx.file, x.Name) {
				if textPackages[x.Name] {
					w.walkArgs(call, ctx, depth) // fmt.Sprintf / strings.Join …: the arguments are the text
				}
				return
			}
			if d := w.idx.local(x, ctx); d != nil && (fun.Sel.Name == "String" || fun.Sel.Name == "Bytes") {
				w.builderWrites(d, ctx, depth)
				return
			}
		}
		for _, ref := range w.methodRefs(fun, ctx) {
			w.walkReturns(ref, call, ctx, resultIdx)
		}
	case *ast.FuncLit:
		w.walkFuncLitReturns(fun, ctx, depth)
	case *ast.ParenExpr:
		w.walk(fun.X, ctx, depth+1)
	case *ast.ArrayType, *ast.MapType, *ast.StarExpr, *ast.InterfaceType, *ast.FuncType, *ast.ChanType:
		w.walkArgs(call, ctx, depth) // type conversion: the operand is the text
	default:
		w.problem(call, "call through %T is an unrecognized shape", call.Fun)
	}
}

// localClosure walks the returns of the function literals bound to a
// local (`render := func(…) string {…}; render(x)`); a function-typed
// parameter's text comes with the caller's value.
func (w *instructionWalker) localClosure(d *varDecl, ctx walkCtx, depth int) {
	if d.param != nil || d.data {
		return
	}
	for _, b := range d.values {
		if fl, ok := b.expr.(*ast.FuncLit); ok {
			w.walkFuncLitReturns(fl, ctx, depth)
		} else {
			w.walkBinding(b, ctx, depth)
		}
	}
}

// walkReturns walks the return values of the callee entered through
// call — the result at resultIdx when it is bound, every result
// otherwise; a bare return walks the named results. The callee's
// parameters are bound to call's arguments in the caller's context.
func (w *instructionWalker) walkReturns(ref funcRef, call *ast.CallExpr, caller walkCtx, resultIdx int) {
	key := retKey{fn: ref.fn, idx: resultIdx, noBackward: caller.noBackward, site: call}
	if ref.fn.Body == nil || w.seenRet[key] {
		return
	}
	w.seenRet[key] = true
	ctx := walkCtx{fn: ref.fn, file: ref.file, noBackward: caller.noBackward, site: &callSite{call: call, caller: caller}}
	sc := w.idx.scope(ref.fn)
	w.eachReturn(ref.fn.Body, func(ret *ast.ReturnStmt) {
		if len(ret.Results) == 0 {
			for _, d := range sc.results {
				for _, b := range d.values {
					w.walkBinding(b, ctx, 0)
				}
			}
			return
		}
		if resultIdx >= 0 && len(ret.Results) > 1 {
			if resultIdx < len(ret.Results) {
				w.walk(ret.Results[resultIdx], ctx, 0)
			}
			return
		}
		for _, r := range ret.Results {
			if inner, ok := r.(*ast.CallExpr); ok && resultIdx >= 0 && len(ret.Results) == 1 {
				w.walkCall(inner, ctx, 0, resultIdx) // return g(): the bound position passes through
				continue
			}
			w.walk(r, ctx, 0)
		}
	})
}

func (w *instructionWalker) walkFuncLitReturns(fl *ast.FuncLit, ctx walkCtx, depth int) {
	if w.seenLit[fl] {
		return
	}
	w.seenLit[fl] = true
	w.eachReturn(fl.Body, func(ret *ast.ReturnStmt) {
		for _, r := range ret.Results {
			w.walk(r, ctx, depth+1)
		}
	})
}

// eachReturn visits the return statements of body that belong to it
// (nested function literals are not its returns).
func (w *instructionWalker) eachReturn(body *ast.BlockStmt, visit func(*ast.ReturnStmt)) {
	if body == nil {
		return
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ReturnStmt:
			visit(v)
		}
		return true
	})
}

// builderWrites binds the text written into the builder / writer
// declared by d inside ctx.fn: WriteString / WriteByte / WriteRune /
// Write calls on it, the other arguments of an other-package call that
// receives it (fmt.Fprintf formats), and the writes made by same-package
// callees it is passed to (`&b` or `b`), whose matching parameter is
// followed with the callee's parameters bound to that call. A builder
// that is itself a parameter is bound to the caller's builder: through
// the entering call when there is one, otherwise (the surface's own
// function) at every same-package call site.
func (w *instructionWalker) builderWrites(d *varDecl, ctx walkCtx, depth int) {
	key := writeKey{decl: d, noBackward: ctx.noBackward, site: siteCall(ctx)}
	if w.seenWrite[key] || ctx.fn == nil || ctx.fn.Body == nil {
		return
	}
	w.seenWrite[key] = true
	// Values assigned to the name (a builder obtained from a call, an
	// alias of another builder) flow in too.
	for _, b := range d.values {
		w.walkBinding(b, ctx, depth)
	}
	isBuilder := func(e ast.Expr) bool {
		id, ok := e.(*ast.Ident)
		return ok && w.idx.local(id, ctx) == d
	}
	ast.Inspect(ctx.fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && isBuilder(sel.X) {
			switch sel.Sel.Name {
			case "WriteString", "WriteByte", "WriteRune", "Write":
				w.walkArgs(call, ctx, depth)
			}
			return true
		}
		argIdx := -1
		for i, arg := range call.Args {
			if isBuilder(builderArgExpr(arg)) {
				argIdx = i
				break
			}
		}
		if argIdx < 0 {
			return true
		}
		refs := w.calleeRefs(call, ctx)
		if len(refs) == 0 {
			// Another package writes through the builder: every other
			// argument is the text it writes (fmt.Fprintf formats).
			for i, arg := range call.Args {
				if i != argIdx {
					w.walk(arg, ctx, depth+1)
				}
			}
			return true
		}
		for _, ref := range refs {
			if pd := w.idx.scope(ref.fn).paramAt(argIdx); pd != nil {
				w.builderWrites(pd, walkCtx{fn: ref.fn, file: ref.file, noBackward: ctx.noBackward, site: &callSite{call: call, caller: ctx}}, depth+1)
			}
		}
		return true
	})
	if d.param == nil {
		return
	}
	idx := d.param.index
	if ctx.site != nil {
		if idx < len(ctx.site.call.Args) {
			w.builderOrigin(ctx.site.call.Args[idx], ctx.site.caller, depth)
		}
		return
	}
	if ctx.noBackward {
		return
	}
	// The builder identity hop: the caller's builder is the same object,
	// so its writes belong to this surface; text authored in the caller
	// has then taken the flow's parameter hop.
	for _, site := range w.callSites(ctx.fn) {
		if idx >= len(site.call.Args) {
			continue
		}
		caller := site.ctx
		caller.noBackward = true
		w.builderOrigin(site.call.Args[idx], caller, depth)
	}
}

// builderOrigin binds a builder passed as an argument (`&b` / `b`) back
// to the caller's declaration, or walks the argument as text.
func (w *instructionWalker) builderOrigin(arg ast.Expr, caller walkCtx, depth int) {
	if id, ok := builderArgExpr(arg).(*ast.Ident); ok {
		if cd := w.idx.local(id, caller); cd != nil {
			w.builderWrites(cd, caller, depth+1)
			return
		}
	}
	w.walk(arg, caller, depth+1)
}

// paramAt returns the declaration of the i-th parameter (a variadic tail
// absorbs every later index), or nil.
func (sc *fnScope) paramAt(i int) *varDecl {
	if i < len(sc.params) {
		return sc.params[i]
	}
	if n := len(sc.params); n > 0 && sc.params[n-1] != nil && sc.params[n-1].param.variadic {
		return sc.params[n-1]
	}
	return nil
}

// calleeRefs resolves a call to the same-package functions or methods it
// may target.
func (w *instructionWalker) calleeRefs(call *ast.CallExpr, ctx walkCtx) []funcRef {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return w.idx.funcs[fun.Name]
	case *ast.SelectorExpr:
		return w.methodRefs(fun, ctx)
	}
	return nil
}

// builderArgExpr strips the `&` of a builder argument (`&b` → b).
func builderArgExpr(arg ast.Expr) ast.Expr {
	if u, ok := arg.(*ast.UnaryExpr); ok && u.Op == token.AND {
		return u.X
	}
	return arg
}
