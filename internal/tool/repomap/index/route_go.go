package index

// route_go.go — framework route → handler resolver for Go (gin + chi
// + gorilla/mux).
//
// Post-pass over an already-parsed Go AST (hooked at the end of
// extractGo, alongside the call/type-ref post-passes) that recognises
// LITERAL route registrations and emits:
//
//   - one Kind="route" types.Symbol per registration, named
//     "<VERB> <path>" at the registration line (for mux: one PER
//     string-literal verb in the chained .Methods(...) call); Parent
//     stays empty (file_map renders Parent.Name and would mangle the
//     display);
//   - one Kind="reference" route→handler types.Relation when the
//     handler argument is an identifier or selector expression.
//     Anonymous func literals (and any other expression shape) get
//     the Symbol but never a Relation.
//
// PRECISE GATE (architectural red line — precise signals for hard
// gates): the pass runs only when the file's imports contain the
// framework's root import path VERBATIM. Receiver names are noisy
// and never gate the pass; a gin-importing file calling
// client.GET("/x", cb) on a non-router receiver is an accepted
// residual, priced by types.ConfidenceRouteLiteral (0.70).
//
// Dynamic / non-literal path arguments emit NOTHING for that
// registration — paths are never fabricated.

import (
	"strconv"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"

	"github.com/hanchaoqun/codrax/internal/tool/repomap/types"
)

// Verbatim import-gate paths. chi additionally accepts any
// go-module major-version suffix ("github.com/go-chi/chi/v5");
// subpackages (".../middleware") do NOT open the gate — route
// registration requires the root package.
const (
	goGinImportPath     = "github.com/gin-gonic/gin"
	goChiImportPathBase = "github.com/go-chi/chi"
	goMuxImportPath     = "github.com/gorilla/mux"
)

// ginRouteVerbs maps gin registration method names to HTTP verbs.
// Source of truth: gin's IRoutes interface (routergroup.go) — the
// fixed-verb registration methods GET / POST / DELETE / PATCH / PUT /
// OPTIONS / HEAD plus Any (matches all verbs). Handle and Match take
// the method as a runtime argument (non-literal verb) and Static* do
// not bind a handler symbol — all deliberately out of scope.
var ginRouteVerbs = map[string]string{
	"GET":     "GET",
	"POST":    "POST",
	"PUT":     "PUT",
	"DELETE":  "DELETE",
	"PATCH":   "PATCH",
	"HEAD":    "HEAD",
	"OPTIONS": "OPTIONS",
	"Any":     "ANY",
}

// chiRouteVerbs maps chi registration method names to HTTP verbs.
// Source of truth: chi's Router interface (chi.go) — the fixed-verb
// methods Connect / Delete / Get / Head / Options / Patch / Post /
// Put / Trace, each (pattern string, h http.HandlerFunc). Method /
// MethodFunc take the verb as a runtime argument and Handle /
// HandleFunc / Mount bind no single verb — out of scope.
var chiRouteVerbs = map[string]string{
	"Connect": "CONNECT",
	"Delete":  "DELETE",
	"Get":     "GET",
	"Head":    "HEAD",
	"Options": "OPTIONS",
	"Patch":   "PATCH",
	"Post":    "POST",
	"Put":     "PUT",
	"Trace":   "TRACE",
}

// muxRouteVerbs: gorilla/mux has NO fixed-verb registration methods —
// Router.HandleFunc / Router.Handle (mux.go; same shapes on *Route,
// route.go) register a handler for ALL methods, and verbs arrive via
// the chained Route.Methods(methods ...string) matcher (route.go),
// which upper-cases every method string. Verbs are therefore read
// from the .Methods(...) string literals (upper-cased to mirror the
// framework), never from a method-name table; an unchained
// registration renders verb "ANY", mirroring gin's Any.

// goRouteState carries the per-file framework activation decided by
// the import gate, plus the local package names the frameworks are
// imported under (alias-aware, defaulting to "gin" / "chi" / "mux")
// so the root-constructor recognisers stay precise under aliased
// imports.
type goRouteState struct {
	gin    bool
	chi    bool
	mux    bool
	ginPkg string
	chiPkg string
	muxPkg string
}

// goRouteBinding is one tracked router-valued local variable: the
// route-path prefix composed so far, and whether composition is
// partial (some ancestor receiver was not resolvable in-function).
type goRouteBinding struct {
	prefix  string
	partial bool
}

// goExtractRoutes is the gin + chi route resolver entry point, hooked
// at the end of extractGo. Walks each top-level function/method body
// with its own binding map (goLocalScope binds only params/receivers
// to types, so prefix tracking needs its own value-binding map).
func goExtractRoutes(root *sitter.Node, src []byte, file string, imps []types.Import) ([]types.Symbol, []types.Relation) {
	st := goRouteImportState(imps)
	if !st.gin && !st.chi && !st.mux {
		return nil, nil
	}
	var syms []types.Symbol
	var rels []types.Relation
	for i := 0; i < int(root.NamedChildCount()); i++ {
		top := root.NamedChild(i)
		switch top.Type() {
		case "function_declaration", "method_declaration":
			body := top.ChildByFieldName("body")
			if body == nil {
				continue
			}
			goWalkRoutes(body, src, file, st, goRouteSeedBindings(top, src, st), &syms, &rels)
		}
	}
	return syms, rels
}

// goRouteImportState applies the PRECISE import gate: verbatim
// import-path match only. Dot- and blank-imports still open the gate
// (the path proves the framework is in play) but cannot rebind the
// package name used by the root-constructor recognisers.
func goRouteImportState(imps []types.Import) *goRouteState {
	st := &goRouteState{ginPkg: "gin", chiPkg: "chi", muxPkg: "mux"}
	for _, imp := range imps {
		aliasUsable := imp.Alias != "" && imp.Alias != "_" && imp.Alias != "."
		switch {
		case imp.Path == goGinImportPath:
			st.gin = true
			if aliasUsable {
				st.ginPkg = imp.Alias
			}
		case goIsChiRootImport(imp.Path):
			st.chi = true
			if aliasUsable {
				st.chiPkg = imp.Alias
			}
		case imp.Path == goMuxImportPath:
			st.mux = true
			if aliasUsable {
				st.muxPkg = imp.Alias
			}
		}
	}
	return st
}

// goIsChiRootImport reports whether path is chi's root package:
// "github.com/go-chi/chi" or any major-version form ".../chi/vN".
func goIsChiRootImport(path string) bool {
	if path == goChiImportPathBase {
		return true
	}
	rest, ok := strings.CutPrefix(path, goChiImportPathBase+"/")
	if !ok || len(rest) < 2 || rest[0] != 'v' {
		return false
	}
	for _, r := range rest[1:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// goRouteSeedBindings builds the initial per-function binding map.
// Parameters declared with the frameworks' concrete ROOT types are
// provably prefix-free: gin.Engine embeds a RouterGroup with basePath
// "/" (gin.New), and chi.Mux is the concrete root router returned by
// chi.NewRouter/NewMux. Group/interface-typed params
// (gin.RouterGroup, chi.Router) may carry an unknown caller-side
// prefix and are NOT seeded — their routes emit LOCAL paths flagged
// Metadata["partial_path"]="true". This is a declared-type signal,
// not a receiver-name heuristic. mux has NO seedable type at all:
// *mux.Router is also the SUBROUTER type (Route.Subrouter returns
// *Router), so a mux.Router parameter never proves rootness.
func goRouteSeedBindings(fnNode *sitter.Node, src []byte, st *goRouteState) map[string]goRouteBinding {
	bindings := make(map[string]goRouteBinding)
	params := fnNode.ChildByFieldName("parameters")
	if params == nil {
		return bindings
	}
	for i := 0; i < int(params.NamedChildCount()); i++ {
		pd := params.NamedChild(i)
		if pd.Type() != "parameter_declaration" {
			continue
		}
		typeNode := pd.ChildByFieldName("type")
		if typeNode == nil {
			continue
		}
		t := strings.TrimPrefix(strings.TrimSpace(nodeText(typeNode, src)), "*")
		isRoot := (st.gin && t == st.ginPkg+".Engine") || (st.chi && t == st.chiPkg+".Mux")
		if !isRoot {
			continue
		}
		for j := 0; j < int(pd.NamedChildCount()); j++ {
			if ch := pd.NamedChild(j); ch.Type() == "identifier" {
				bindings[nodeText(ch, src)] = goRouteBinding{prefix: "", partial: false}
			}
		}
	}
	return bindings
}

// goWalkRoutes recursively visits node in document order, so value
// bindings recorded by earlier statements are visible to later
// registrations in the same (or a nested) block.
func goWalkRoutes(node *sitter.Node, src []byte, file string, st *goRouteState, bindings map[string]goRouteBinding, syms *[]types.Symbol, rels *[]types.Relation) {
	switch node.Type() {
	case "short_var_declaration", "assignment_statement":
		goRouteHandleBinding(node, src, st, bindings)
	case "call_expression":
		if goRouteHandleChiSubrouter(node, src, file, st, bindings, syms, rels) {
			// Inline body already walked with the inner scope —
			// generic descent would re-walk it with outer bindings.
			return
		}
		goRouteEmit(node, src, file, st, bindings, syms, rels)
	}
	for i := 0; i < int(node.NamedChildCount()); i++ {
		goWalkRoutes(node.NamedChild(i), src, file, st, bindings, syms, rels)
	}
}

// goRouteHandleBinding tracks same-function router value bindings:
// g := <recv>.Group("<literal>"), r := gin.Default(), m := chi.NewRouter().
// Only single-assign statements are tracked; rebinding a tracked name
// to a non-router value invalidates the old binding.
func goRouteHandleBinding(stmt *sitter.Node, src []byte, st *goRouteState, bindings map[string]goRouteBinding) {
	left := stmt.ChildByFieldName("left")
	right := stmt.ChildByFieldName("right")
	if left == nil || right == nil ||
		int(left.NamedChildCount()) != 1 || int(right.NamedChildCount()) != 1 {
		return
	}
	lhs := left.NamedChild(0)
	rhs := right.NamedChild(0)
	if lhs.Type() != "identifier" || rhs.Type() != "call_expression" {
		return
	}
	name := nodeText(lhs, src)
	prefix, partial, isRouter := goResolveRouterExpr(rhs, src, st, bindings)
	if !isRouter {
		delete(bindings, name)
		return
	}
	bindings[name] = goRouteBinding{prefix: prefix, partial: partial}
}

// goResolveRouterExpr resolves a router-valued expression to the
// route-path prefix it carries. Returns (prefix, partial, isRouter):
//
//   - identifier present in the binding map → its tracked prefix;
//   - gin.New() / gin.Default(), chi.NewRouter() / chi.NewMux(),
//     mux.NewRouter() (alias-aware package match) → root, prefix "";
//   - <expr>.Group("<literal>") with gin active → resolve(expr) +
//     literal; an unresolvable base keeps the LOCAL prefix and turns
//     partial on; a non-literal Group path is still a router but its
//     prefix is unknowable (partial, empty);
//   - <expr>.PathPrefix("<literal>").Subrouter() with mux active →
//     resolve(expr) + literal (Route.Subrouter, route.go); Subrouter
//     over any other route shape is still a router, prefix unknowable;
//   - anything else → not provably a router: ("", true, false), so
//     registrations on it emit their LOCAL path flagged partial.
func goResolveRouterExpr(expr *sitter.Node, src []byte, st *goRouteState, bindings map[string]goRouteBinding) (string, bool, bool) {
	if expr == nil {
		return "", true, false
	}
	switch expr.Type() {
	case "identifier":
		if b, ok := bindings[nodeText(expr, src)]; ok {
			return b.prefix, b.partial, true
		}
		return "", true, false
	case "call_expression":
		fn := expr.ChildByFieldName("function")
		if fn == nil || fn.Type() != "selector_expression" {
			return "", true, false
		}
		field := fn.ChildByFieldName("field")
		op := fn.ChildByFieldName("operand")
		if field == nil || op == nil {
			return "", true, false
		}
		mname := nodeText(field, src)
		if op.Type() == "identifier" {
			pkg := nodeText(op, src)
			if st.gin && pkg == st.ginPkg && (mname == "New" || mname == "Default") {
				return "", false, true
			}
			if st.chi && pkg == st.chiPkg && (mname == "NewRouter" || mname == "NewMux") {
				return "", false, true
			}
			if st.mux && pkg == st.muxPkg && mname == "NewRouter" {
				// mux.NewRouter() (mux.go) — the only root constructor.
				return "", false, true
			}
		}
		if st.gin && mname == "Group" {
			// gin.IRouter.Group(relativePath string, ...) *RouterGroup.
			args := goRouteArgs(expr)
			if len(args) >= 1 {
				if lit, ok := goRouteStringLiteral(args[0], src); ok && (lit == "" || strings.HasPrefix(lit, "/")) {
					base, partial, _ := goResolveRouterExpr(op, src, st, bindings)
					return goJoinRoutePaths(base, lit), partial, true
				}
			}
			return "", true, true
		}
		if st.mux && mname == "Subrouter" {
			// mux Route.Subrouter() (route.go) returns a *Router that
			// inherits the route's matchers. Only the literal
			// Router.PathPrefix(tpl string) *Route base (mux.go /
			// route.go) is composable; any other route shape (Host,
			// Path, non-literal prefix, unresolvable value) is still a
			// router but its prefix is unknowable.
			if op.Type() == "call_expression" {
				if opFn := op.ChildByFieldName("function"); opFn != nil && opFn.Type() == "selector_expression" {
					opField := opFn.ChildByFieldName("field")
					opOperand := opFn.ChildByFieldName("operand")
					if opField != nil && opOperand != nil && nodeText(opField, src) == "PathPrefix" {
						args := goRouteArgs(op)
						if len(args) >= 1 {
							if lit, ok := goRouteStringLiteral(args[0], src); ok && strings.HasPrefix(lit, "/") {
								base, partial, _ := goResolveRouterExpr(opOperand, src, st, bindings)
								return goJoinRoutePaths(base, lit), partial, true
							}
						}
					}
				}
			}
			return "", true, true
		}
		return "", true, false
	}
	return "", true, false
}

// goRouteHandleChiSubrouter handles chi's structural composition
// forms (both from chi's Router interface, chi.go):
//
//	Route(pattern string, fn func(r Router)) Router
//	Group(fn func(r Router)) Router
//
// When the sub-tree argument is an inline func literal, the literal
// body is walked with the inner router param bound to the composed
// prefix (best-effort, same partial rule as gin groups). Returns true
// when the call was consumed (body already walked).
func goRouteHandleChiSubrouter(call *sitter.Node, src []byte, file string, st *goRouteState, bindings map[string]goRouteBinding, syms *[]types.Symbol, rels *[]types.Relation) bool {
	if !st.chi {
		return false
	}
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "selector_expression" {
		return false
	}
	field := fn.ChildByFieldName("field")
	op := fn.ChildByFieldName("operand")
	if field == nil || op == nil {
		return false
	}
	args := goRouteArgs(call)
	var fnLit *sitter.Node
	var prefix string
	var partial bool
	switch nodeText(field, src) {
	case "Route":
		if len(args) != 2 || args[1].Type() != "func_literal" {
			return false
		}
		fnLit = args[1]
		if lit, ok := goRouteStringLiteral(args[0], src); ok && strings.HasPrefix(lit, "/") {
			base, basePartial, _ := goResolveRouterExpr(op, src, st, bindings)
			prefix, partial = goJoinRoutePaths(base, lit), basePartial
		} else {
			// Dynamic mount pattern: inner registrations keep their
			// LOCAL paths, flagged partial.
			prefix, partial = "", true
		}
	case "Group":
		if len(args) != 1 || args[0].Type() != "func_literal" {
			return false
		}
		fnLit = args[0]
		prefix, partial, _ = goResolveRouterExpr(op, src, st, bindings)
	default:
		return false
	}
	inner := make(map[string]goRouteBinding, len(bindings)+1)
	for k, v := range bindings {
		inner[k] = v
	}
	if pname := goFuncLiteralFirstParam(fnLit, src); pname != "" {
		inner[pname] = goRouteBinding{prefix: prefix, partial: partial}
	}
	if body := fnLit.ChildByFieldName("body"); body != nil {
		goWalkRoutes(body, src, file, st, inner, syms, rels)
	}
	return true
}

// goFuncLiteralFirstParam returns the first bound parameter name of
// a func literal ("" when the literal declares no named params).
func goFuncLiteralFirstParam(fnLit *sitter.Node, src []byte) string {
	params := fnLit.ChildByFieldName("parameters")
	if params == nil {
		return ""
	}
	for i := 0; i < int(params.NamedChildCount()); i++ {
		pd := params.NamedChild(i)
		if pd.Type() != "parameter_declaration" {
			continue
		}
		for j := 0; j < int(pd.NamedChildCount()); j++ {
			if ch := pd.NamedChild(j); ch.Type() == "identifier" {
				return nodeText(ch, src)
			}
		}
		return ""
	}
	return ""
}

// goRouteEmit recognises a single literal route registration and
// emits the route Symbol (always) plus the route→handler Relation
// (identifier / selector handlers only).
func goRouteEmit(call *sitter.Node, src []byte, file string, st *goRouteState, bindings map[string]goRouteBinding, syms *[]types.Symbol, rels *[]types.Relation) {
	fn := call.ChildByFieldName("function")
	if fn == nil || fn.Type() != "selector_expression" {
		return
	}
	field := fn.ChildByFieldName("field")
	if field == nil {
		return
	}
	mname := nodeText(field, src)
	var verb, fw, resolvedBy string
	switch {
	case st.mux && (mname == "HandleFunc" || mname == "Handle"):
		// mux registrations carry their verbs in a chained
		// .Methods(...) call (or none → ANY) and may emit several
		// routes — dispatched to the dedicated emitter.
		goRouteEmitMux(call, src, file, st, bindings, syms, rels)
		return
	case st.gin && ginRouteVerbs[mname] != "":
		verb, fw, resolvedBy = ginRouteVerbs[mname], "gin", "gin_route_literal"
	case st.chi && chiRouteVerbs[mname] != "":
		verb, fw, resolvedBy = chiRouteVerbs[mname], "chi", "chi_route_literal"
	default:
		return
	}
	args := goRouteArgs(call)
	if len(args) < 2 {
		return
	}
	local, ok := goRouteStringLiteral(args[0], src)
	if !ok || !strings.HasPrefix(local, "/") {
		// Non-literal / dynamic path: emit NOTHING (never fabricate).
		return
	}
	var handler *sitter.Node
	if fw == "gin" {
		// gin handlers are variadic; the terminal handler is the
		// LAST argument (earlier ones are per-route middleware).
		handler = args[len(args)-1]
	} else {
		// chi registration methods are (pattern, http.HandlerFunc).
		handler = args[1]
	}

	prefix, partial, _ := goResolveRouterExpr(fn.ChildByFieldName("operand"), src, st, bindings)
	fullPath := goJoinRoutePaths(prefix, local)
	routeName := verb + " " + fullPath
	line := nodeLine(call)

	display := "func(...)"
	var handlerName, handlerRecv string
	hasTarget := false
	switch handler.Type() {
	case "identifier":
		handlerName = nodeText(handler, src)
		display = handlerName
		hasTarget = true
	case "selector_expression":
		if f := handler.ChildByFieldName("field"); f != nil {
			handlerName = nodeText(f, src)
			if hop := handler.ChildByFieldName("operand"); hop != nil {
				handlerRecv = nodeText(hop, src)
			}
			display = nodeText(handler, src)
			hasTarget = true
		}
	}

	*syms = append(*syms, types.Symbol{
		Name:      routeName,
		Kind:      "route",
		File:      file,
		Line:      line,
		EndLine:   line,
		Exported:  true,
		Signature: routeName + " -> " + display,
		// Parent MUST stay empty: file_map renders Parent.Name and
		// would mangle the route display.
	})
	if !hasTarget {
		// Anonymous func literal (or other non-name expression):
		// Symbol only, no Relation.
		return
	}
	md := map[string]string{"framework": fw, "method": verb, "path": fullPath}
	if partial {
		md["partial_path"] = "true"
	}
	*rels = append(*rels, types.Relation{
		Kind:       "reference",
		FromEP:     types.RelationEndpoint{Name: routeName, File: file, Line: line},
		ToEP:       types.RelationEndpoint{Name: handlerName, Receiver: handlerRecv, File: file, Line: line},
		File:       file,
		Line:       line,
		Confidence: types.ConfidenceRouteLiteral,
		Provenance: types.ProvenanceRouteResolver,
		ResolvedBy: resolvedBy,
		Metadata:   md,
	})
}

// goRouteEmitMux recognises gorilla/mux literal registrations:
//
//	Router.HandleFunc(path string, f func(http.ResponseWriter, *http.Request)) *Route
//	Router.Handle(path string, handler http.Handler) *Route
//
// (mux.go; identical shapes exist on *Route, route.go). The verbs
// come from the chained Route.Methods(methods ...string) call (see
// goRouteMuxMethodsCall): ONE route Symbol (+Relation for named
// handlers) is emitted PER string-literal verb, upper-cased the way
// Route.Methods upper-cases its arguments. Non-literal verbs are
// skipped; a Methods chain with NO literal verb emits nothing for the
// registration (verbs are never fabricated). An unchained
// registration matches every method — mux only restricts methods when
// a matcher says so — and renders verb "ANY", mirroring gin's Any.
//
// Handle's http.Handler argument follows the shared handler-shape
// rule: identifier / selector emits the Relation; any composite
// expression (http.StripPrefix(...), &h{}, func literals, ...) emits
// the Symbol only.
func goRouteEmitMux(call *sitter.Node, src []byte, file string, st *goRouteState, bindings map[string]goRouteBinding, syms *[]types.Symbol, rels *[]types.Relation) {
	args := goRouteArgs(call)
	if len(args) != 2 {
		return
	}
	local, ok := goRouteStringLiteral(args[0], src)
	if !ok || !strings.HasPrefix(local, "/") {
		// Non-literal / dynamic path: emit NOTHING (never fabricate).
		return
	}
	var verbs []string
	if mcall := goRouteMuxMethodsCall(call, src); mcall != nil {
		for _, va := range goRouteArgs(mcall) {
			if lit, ok := goRouteStringLiteral(va, src); ok && lit != "" {
				verbs = append(verbs, strings.ToUpper(lit))
			}
		}
		if len(verbs) == 0 {
			// Every chained verb is non-literal: emit nothing.
			return
		}
	} else {
		verbs = []string{"ANY"}
	}

	handler := args[1]
	display := "func(...)"
	var handlerName, handlerRecv string
	hasTarget := false
	switch handler.Type() {
	case "identifier":
		handlerName = nodeText(handler, src)
		display = handlerName
		hasTarget = true
	case "selector_expression":
		if f := handler.ChildByFieldName("field"); f != nil {
			handlerName = nodeText(f, src)
			if hop := handler.ChildByFieldName("operand"); hop != nil {
				handlerRecv = nodeText(hop, src)
			}
			display = nodeText(handler, src)
			hasTarget = true
		}
	}

	fn := call.ChildByFieldName("function")
	prefix, partial, _ := goResolveRouterExpr(fn.ChildByFieldName("operand"), src, st, bindings)
	fullPath := goJoinRoutePaths(prefix, local)
	line := nodeLine(call)
	for _, verb := range verbs {
		routeName := verb + " " + fullPath
		*syms = append(*syms, types.Symbol{
			Name:      routeName,
			Kind:      "route",
			File:      file,
			Line:      line,
			EndLine:   line,
			Exported:  true,
			Signature: routeName + " -> " + display,
			// Parent MUST stay empty: file_map renders Parent.Name and
			// would mangle the route display.
		})
		if !hasTarget {
			// Anonymous / composite handler expression: Symbol only.
			continue
		}
		md := map[string]string{"framework": "mux", "method": verb, "path": fullPath}
		if partial {
			md["partial_path"] = "true"
		}
		*rels = append(*rels, types.Relation{
			Kind:       "reference",
			FromEP:     types.RelationEndpoint{Name: routeName, File: file, Line: line},
			ToEP:       types.RelationEndpoint{Name: handlerName, Receiver: handlerRecv, File: file, Line: line},
			File:       file,
			Line:       line,
			Confidence: types.ConfidenceRouteLiteral,
			Provenance: types.ProvenanceRouteResolver,
			ResolvedBy: "mux_route_literal",
			Metadata:   md,
		})
	}
}

// goRouteMuxMethodsCall walks UP the mux method chain from a
// registration call. In `r.HandleFunc(...).Methods("GET")` the
// chained call sits ABOVE the registration in the AST — the
// registration is the operand of a selector_expression whose parent
// call_expression invokes the next chain link — and Route methods
// return *Route, so chains may interleave other matchers
// (`.Queries(...).Methods(...)`). Returns the first chained
// .Methods(...) call_expression, or nil when the chain has none
// (registration matches all methods → verb "ANY").
func goRouteMuxMethodsCall(reg *sitter.Node, src []byte) *sitter.Node {
	for cur := reg; ; {
		sel := cur.Parent()
		if sel == nil || sel.Type() != "selector_expression" {
			return nil
		}
		call := sel.Parent()
		if call == nil || call.Type() != "call_expression" {
			return nil
		}
		// The selector must be the call's FUNCTION (a chain link), not
		// some argument-position subexpression.
		fn := call.ChildByFieldName("function")
		if fn == nil || !fn.Equal(sel) {
			return nil
		}
		if f := sel.ChildByFieldName("field"); f != nil && nodeText(f, src) == "Methods" {
			return call
		}
		cur = call
	}
}

// goRouteArgs returns the named argument expressions of a call,
// skipping interleaved comments.
func goRouteArgs(call *sitter.Node) []*sitter.Node {
	argList := call.ChildByFieldName("arguments")
	if argList == nil {
		return nil
	}
	var out []*sitter.Node
	for i := 0; i < int(argList.NamedChildCount()); i++ {
		ch := argList.NamedChild(i)
		if ch.Type() == "comment" {
			continue
		}
		out = append(out, ch)
	}
	return out
}

// goRouteStringLiteral unquotes an interpreted_string_literal node.
// Raw strings and every other expression shape report !ok — the
// false-positive guard requires a literal path.
func goRouteStringLiteral(n *sitter.Node, src []byte) (string, bool) {
	if n == nil || n.Type() != "interpreted_string_literal" {
		return "", false
	}
	raw := nodeText(n, src)
	if v, err := strconv.Unquote(raw); err == nil {
		return v, true
	}
	return strings.Trim(raw, `"`), true
}

// goJoinRoutePaths concatenates a composed prefix and a local path,
// collapsing the doubled slash at the seam (mirrors the visible
// result of gin's joinPaths / chi's pattern nesting for the literal
// forms this pass accepts).
func goJoinRoutePaths(prefix, p string) string {
	if prefix == "" {
		return p
	}
	if strings.HasSuffix(prefix, "/") && strings.HasPrefix(p, "/") {
		return prefix + p[1:]
	}
	return prefix + p
}
