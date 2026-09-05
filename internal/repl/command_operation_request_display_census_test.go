package repl

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// command_operation_request_display_census_test.go — request/display
// argument-order census (colleague_merge_audit §40.52, batch six fold-in
// review round three #7). The operation route carries two texts per turn:
// the REQUEST (what the planner plans from and the answerer's
// `## user_request` — an expanded prompt template, a replayed pasted
// follow-up, the clarification-combined request, a parked plan's
// RequestText) and the DISPLAY (what the history echoes — the typed line,
// "[pasted N line(s)] …", the clarification reply, "/approve"). Every
// dispatcher takes them as (request, display) in that order, and the
// direct pin (command_operation_request_display_test.go) only proves the
// swapped function; a swap at any producing call site upstream stayed
// green. This census binds every call site of the request/display
// dispatchers in package repl's non-test files to the lane of the
// expression it passes, by data flow from the producing variable:
//
//	request lane   — a `.RequestText` / `.OriginalLine` selector (a plan's
//	                 or a parked clarification's request), the first result
//	                 of r.expandTemplateCommand, a
//	                 commandOperationFollowupRequestText call, a parameter
//	                 named `request`;
//	display lane   — a `.Display` selector, a followUpDisplayText call, a
//	                 parameter named `display`, the second result of
//	                 r.readInputPair, a string literal ("/approve" is a
//	                 display form; the user's request is never a literal);
//	raw lane       — a line as typed, valid in either slot: a parameter
//	                 named `line`, a `.Text` selector (a queued follow-up
//	                 entry, a provider request), the first result of
//	                 r.readInputPair;
//	strings.TrimSpace/TrimPrefix/TrimSuffix(x, …) / (x) carry x's lane and
//	so does the first result of splitPastedLog(x); `a + b` merges operand
//	lanes (string literals contribute nothing; raw folds into request);
//	a local carries the merged lane of every assignment to it in the
//	enclosing function EXCEPT an empty-fallback fill (`if x == "" { x = … }`
//	or `if strings.TrimSpace(x) == "" { x = … }`), which never widens the
//	lane, and a self-referential re-assignment (`line = cleaned` where
//	cleaned derives from line), which carries the lane unchanged.
//
// The request slot accepts request or raw; the display slot accepts
// display or raw. A local with conflicting lanes, a parameter outside the
// three names, a selector outside the field set, a call outside the helper
// set, or any other expression is an unrecognized shape and is red (§40.50
// ruling — the census fails loud instead of skipping).
//
// Call shape (review round four #7): a dispatcher site is classified only
// when it is a direct selector call on the enclosing method's receiver
// (`r.<dispatcher>(…)` inside a *REPL method). Every other reference to a
// dispatcher name — a method value (`exec := r.dispatch`), a call through a
// receiver copy (`self := r; self.dispatch(…)`), a method expression
// (`(*REPL).dispatch(r, …)`), a call in a non-method function or on
// another receiver — is red as an unrecognized dispatch shape; the census
// never counts a site it did not classify. Site counts are exact
// (requestDisplayDispatchSiteRoster): an added or removed site is red until
// the roster is updated, so a new site cannot slip in unclassified.
//
// Receiver identity (review round five #5): the census keys "direct call on
// the enclosing method's receiver" on the receiver's identifier, so a
// rebinding of that name inside the method body — `r := other`, `var r`,
// `for r := range …`, a func-literal parameter or named result `r` —
// would let a call on the rebound name pass as a call on the receiver.
// Any such rebinding is red on a precise signal (the four binding shapes,
// nothing else), and the multi-value producer arm (`line, display :=
// r.readInputPair(…)`) resolves against the enclosing receiver ident, not
// the literal `r`.
//
// On a red census the failure output lists every classified site
// (file:line, enclosing method, dispatcher, request/display lanes) and the
// per-dispatcher counts, so a legitimately added site is red WITH its
// location (review round five #4).
//
// Self-red (TestCommandOperationRequestDisplayCensusSelfRed): each of those
// shapes, the round-three #5 literal-in-request-slot shape
// `executeCommandOperationPlan(plan, "/approve", "/approve")`, a swapped
// (display, request) pair, and roster drift in either direction.
func TestCommandOperationRequestDisplayArgumentOrderCensus(t *testing.T) {
	report := requestDisplayCensus(t, ".")
	problems := append(append([]string(nil), report.Problems...), requestDisplayRosterProblems(report.Counts, requestDisplayDispatchSiteRoster)...)
	if len(problems) > 0 {
		t.Fatal(requestDisplayCensusFailure(problems, report))
	}
	t.Logf("request/display census: classified call sites per dispatcher = %v", report.Counts)
}

// requestDisplayCensusFailure renders a red census: the problems, then
// every classified site and the per-dispatcher counts, so the roster
// message's "listed below" is true of what is printed.
func requestDisplayCensusFailure(problems []string, report requestDisplayCensusReport) string {
	sort.Strings(problems)
	sites := append([]string(nil), report.Sites...)
	sort.Strings(sites)
	if len(sites) == 0 {
		sites = []string{"(none)"}
	}
	return fmt.Sprintf("request/display argument-order census: %d problem(s):\n  %s\nclassified sites (%d), counts per dispatcher %v:\n  %s",
		len(problems), strings.Join(problems, "\n  "), len(report.Sites), report.Counts, strings.Join(sites, "\n  "))
}

// requestDisplayDispatchSiteRoster is the exact number of direct dispatcher
// call sites in package repl's non-test files. A site added anywhere is red
// here until it is registered (and therefore classified); a site removed is
// red until it is retired. EVOLUTION RECORD (review round four #7): the
// former floors were lower bounds and caught only a removed site; a site
// added through a method value or a receiver copy was neither classified
// nor counted and stayed green.
//
// EVOLUTION RECORD (review round five #3): the former Attempt roster of 7
// counted two sites inside maybeReplanCommandOperation and
// maybeContinueCommandOperation — methods with no caller outside a
// self-recursion (dead since before b6f7eeec3, deleted) — and labelled the
// executeCommandOperationPlan wrapper body "one-shot user mode".
var requestDisplayDispatchSiteRoster = map[string]int{
	"operationDispatch":                     3, // clarification resume, user-mode arm, RouteOperation arm
	"executeCommandOperationPlan":           2, // validate-and-run, initial auto-execute
	"executeCommandOperationPlanAttempt":    5, // follow-up lint / auto-execute, /approve (carry resume), executeCommandOperationPlan wrapper body, provider-to-command continuation
	"dispatch":                              6, // follow-up replay ×3, template expansion, typed line, one-shot user mode
	"dispatchWithUserMode":                  1,
	"resumeCommandOperationClarification":   1,
	"maybeDispatchCommandOperationFollowup": 3, // RouteLocal / RouteHybrid / RouteRepo arms of dispatch (review round five #5)
}

// requestDisplayRosterProblems compares classified site counts with the
// exact roster in both directions.
func requestDisplayRosterProblems(counts, roster map[string]int) []string {
	var problems []string
	for callee, want := range roster {
		if got := counts[callee]; got != want {
			problems = append(problems, fmt.Sprintf("roster: %d direct %s call site(s) classified, roster registers %d — register an added site (every classified site is listed below) or retire a removed one", got, callee, want))
		}
	}
	for callee := range counts {
		if _, ok := roster[callee]; !ok {
			problems = append(problems, fmt.Sprintf("roster: %s is classified but not registered in requestDisplayDispatchSiteRoster", callee))
		}
	}
	return problems
}

// requestDisplayDispatchers maps each dispatcher to the (request, display)
// argument indices of its call. maybeDispatchCommandOperationFollowup is on
// the operation route — it forwards display into two Attempt sites and line
// into the follow-up request text — so its call sites are classified too
// (review round five #5); the other (line, display) methods of package repl
// (chitchatDispatch, localDispatch, clarifyDispatch,
// operationUnavailableDispatch, dataTaskDispatch) are non-operation routes
// and stay outside the census.
var requestDisplayDispatchers = map[string][2]int{
	"operationDispatch":                     {0, 1},
	"executeCommandOperationPlan":           {1, 2},
	"executeCommandOperationPlanAttempt":    {1, 2},
	"dispatch":                              {0, 1},
	"dispatchWithUserMode":                  {0, 1},
	"resumeCommandOperationClarification":   {0, 1},
	"maybeDispatchCommandOperationFollowup": {0, 1},
}

// requestDisplayCensusReport is the census outcome: the problems, the
// per-dispatcher classified site counts, and every classified site.
type requestDisplayCensusReport struct {
	Problems []string
	Counts   map[string]int
	Sites    []string
}

type textLane int

const (
	laneUnknown textLane = iota
	laneRequest
	laneDisplay
	laneRaw
	// laneSelf marks an expression that resolves back to the identifier
	// being classified (`line = strings.TrimSpace(line)`): it carries the
	// lane unchanged and contributes nothing to a merge.
	laneSelf
)

func (l textLane) String() string {
	switch l {
	case laneRequest:
		return "request"
	case laneDisplay:
		return "display"
	case laneRaw:
		return "raw"
	case laneSelf:
		return "self"
	}
	return "unknown"
}

// requestDisplayCensus classifies every dispatcher call site in dir's
// non-test files.
func requestDisplayCensus(t *testing.T, dir string) requestDisplayCensusReport {
	t.Helper()
	fset := token.NewFileSet()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}
	var files []*ast.File
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		files = append(files, f)
	}
	if len(files) < 20 {
		t.Fatalf("census scanned %d non-test files in %s, want ≥ 20 (wrong directory?)", len(files), dir)
	}
	return requestDisplayCensusFiles(fset, files)
}

// requestDisplayCensusFiles is the census proper over parsed files: every
// direct `<receiver>.<dispatcher>(…)` call inside a *REPL method is
// classified and counted; any other reference to a dispatcher name is red,
// and so is any rebinding of a *REPL method's receiver name in its body.
func requestDisplayCensusFiles(fset *token.FileSet, files []*ast.File) requestDisplayCensusReport {
	var problems []string
	var sites []string
	counts := map[string]int{}
	for _, f := range files {
		name := filepath.Base(fset.Position(f.Pos()).Filename)
		for _, decl := range f.Decls {
			fd, ok := decl.(*ast.FuncDecl)
			if !ok || fd.Body == nil {
				continue
			}
			recv := replMethodReceiverName(fd)
			if recv != "" {
				for _, rb := range receiverRebindings(fset, fd, recv) {
					problems = append(problems, fmt.Sprintf("%s:%d %s: receiver %s rebound by %s — a dispatcher call on the rebound name is not a call on the method's receiver; unrecognized dispatch shape", name, rb.line, fd.Name.Name, recv, rb.shape))
				}
			}
			c := newLaneClassifier(fset, fd, recv)
			// dispatcher-named selectors that are the Fun of a call — every
			// other dispatcher-named selector is a method value or a
			// reference outside a call and is reported below.
			calleeSel := map[*ast.SelectorExpr]bool{}
			ast.Inspect(fd.Body, func(n ast.Node) bool {
				switch v := n.(type) {
				case *ast.CallExpr:
					sel, ok := v.Fun.(*ast.SelectorExpr)
					if !ok {
						return true
					}
					idx, ok := requestDisplayDispatchers[sel.Sel.Name]
					if !ok {
						return true
					}
					calleeSel[sel] = true
					site := fmt.Sprintf("%s:%d %s.%s", name, fset.Position(v.Pos()).Line, fd.Name.Name, sel.Sel.Name)
					if x, ok := sel.X.(*ast.Ident); !ok || recv == "" || x.Name != recv {
						problems = append(problems, fmt.Sprintf("%s: dispatcher called through %s instead of the enclosing *REPL method's receiver — receiver copy, method expression, non-method function or another receiver; unrecognized dispatch shape", site, exprText(sel.X)))
						return true
					}
					counts[sel.Sel.Name]++
					if len(v.Args) <= idx[1] {
						problems = append(problems, site+": fewer arguments than the (request, display) slots — unrecognized call shape")
						return true
					}
					reqLane, reqWhy := c.lane(v.Args[idx[0]])
					dispLane, dispWhy := c.lane(v.Args[idx[1]])
					sites = append(sites, fmt.Sprintf("%s: request=%s (%s lane: %s), display=%s (%s lane: %s)", site, exprText(v.Args[idx[0]]), reqLane, reqWhy, exprText(v.Args[idx[1]]), dispLane, dispWhy))
					if reqLane != laneRequest && reqLane != laneRaw {
						problems = append(problems, fmt.Sprintf("%s: request slot carries %s (%s lane: %s) — want the request text or the raw line", site, exprText(v.Args[idx[0]]), reqLane, reqWhy))
					}
					if dispLane != laneDisplay && dispLane != laneRaw {
						problems = append(problems, fmt.Sprintf("%s: display slot carries %s (%s lane: %s) — want the display form or the raw line", site, exprText(v.Args[idx[1]]), dispLane, dispWhy))
					}
				case *ast.SelectorExpr:
					if _, ok := requestDisplayDispatchers[v.Sel.Name]; !ok || calleeSel[v] {
						return true
					}
					problems = append(problems, fmt.Sprintf("%s:%d %s: reference to dispatcher %s outside a direct call — method value or other non-call reference; unrecognized dispatch shape", name, fset.Position(v.Pos()).Line, fd.Name.Name, exprText(v)))
				}
				return true
			})
		}
	}
	return requestDisplayCensusReport{Problems: problems, Counts: counts, Sites: sites}
}

// receiverRebinding is one rebinding of a *REPL method's receiver name
// inside its body.
type receiverRebinding struct {
	line  int
	shape string
}

// receiverRebindings finds every rebinding of recv inside fd's body on a
// precise signal: a `:=` definition (plain, type-switch or select
// receive), a `var` declaration, a `for … := range` key or value, and a
// func-literal parameter or named result. Nothing else is a rebinding.
func receiverRebindings(fset *token.FileSet, fd *ast.FuncDecl, recv string) []receiverRebinding {
	var found []receiverRebinding
	add := func(n ast.Node, shape string) {
		found = append(found, receiverRebinding{line: fset.Position(n.Pos()).Line, shape: shape})
	}
	names := func(ids []*ast.Ident) bool {
		for _, id := range ids {
			if id != nil && id.Name == recv {
				return true
			}
		}
		return false
	}
	ast.Inspect(fd.Body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			if v.Tok != token.DEFINE {
				return true
			}
			for _, lhs := range v.Lhs {
				if id, ok := lhs.(*ast.Ident); ok && id.Name == recv {
					add(v, "a := definition")
				}
			}
		case *ast.ValueSpec:
			if names(v.Names) {
				add(v, "a var declaration")
			}
		case *ast.RangeStmt:
			if v.Tok != token.DEFINE {
				return true
			}
			for _, e := range []ast.Expr{v.Key, v.Value} {
				if id, ok := e.(*ast.Ident); ok && id.Name == recv {
					add(v, "a range definition")
				}
			}
		case *ast.FuncLit:
			for _, field := range v.Type.Params.List {
				if names(field.Names) {
					add(field, "a func-literal parameter")
				}
			}
			if v.Type.Results != nil {
				for _, field := range v.Type.Results.List {
					if names(field.Names) {
						add(field, "a func-literal named result")
					}
				}
			}
		}
		return true
	})
	return found
}

// replMethodReceiverName returns the receiver identifier of a *REPL method
// ("" for a non-method function or a method of another type).
func replMethodReceiverName(fd *ast.FuncDecl) string {
	if fd.Recv == nil || len(fd.Recv.List) != 1 || len(fd.Recv.List[0].Names) != 1 {
		return ""
	}
	star, ok := fd.Recv.List[0].Type.(*ast.StarExpr)
	if !ok {
		return ""
	}
	if typ, ok := star.X.(*ast.Ident); !ok || typ.Name != "REPL" {
		return ""
	}
	return fd.Recv.List[0].Names[0].Name
}

// laneClassifier resolves the lane of an expression inside one function
// by data flow from the producing variable.
type laneClassifier struct {
	fset    *token.FileSet
	fn      *ast.FuncDecl
	recv    string // the enclosing *REPL method's receiver ident ("" outside one)
	params  map[string]textLane
	assigns map[string][]laneAssign // local name → every non-fallback assignment
	memo    map[string]textLane
	memoWhy map[string]string
	active  map[string]bool // names being resolved — a re-entry is a self-reference
	selfHit int             // bumped on every self-reference; a resolution that saw one is not memoized
}

type laneAssign struct {
	rhs   ast.Expr
	index int  // result index for multi-value call assignments, else -1
	add   bool // `x += rhs`
}

func newLaneClassifier(fset *token.FileSet, fn *ast.FuncDecl, recv string) *laneClassifier {
	c := &laneClassifier{
		fset:    fset,
		fn:      fn,
		recv:    recv,
		params:  map[string]textLane{},
		assigns: map[string][]laneAssign{},
		memo:    map[string]textLane{},
		memoWhy: map[string]string{},
		active:  map[string]bool{},
	}
	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			for _, id := range field.Names {
				switch id.Name {
				case "line":
					c.params[id.Name] = laneRaw
				case "display":
					c.params[id.Name] = laneDisplay
				case "request":
					c.params[id.Name] = laneRequest
				default:
					c.params[id.Name] = laneUnknown
				}
			}
		}
	}
	// parent links so an assignment can be recognized as an
	// empty-fallback fill (`if x == "" { x = … }`).
	parents := map[ast.Node]ast.Node{}
	var stack []ast.Node
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if n == nil {
			stack = stack[:len(stack)-1]
			return false
		}
		if len(stack) > 0 {
			parents[n] = stack[len(stack)-1]
		}
		stack = append(stack, n)
		as, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, lhs := range as.Lhs {
			id, ok := lhs.(*ast.Ident)
			if !ok || id.Name == "_" {
				continue
			}
			if isEmptyFallbackFill(id.Name, as, parents) {
				continue
			}
			switch {
			case as.Tok == token.ADD_ASSIGN:
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: as.Rhs[0], index: -1, add: true})
			case len(as.Lhs) == len(as.Rhs):
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: as.Rhs[i], index: -1})
			case len(as.Rhs) == 1:
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: as.Rhs[0], index: i})
			default:
				c.assigns[id.Name] = append(c.assigns[id.Name], laneAssign{rhs: nil, index: -1})
			}
		}
		return true
	})
	return c
}

// isEmptyFallbackFill reports whether as is `name = …` directly inside the
// body of `if name == "" { … }` or `if strings.TrimSpace(name) == "" { … }`
// — a fill of an empty value that never widens the variable's lane.
func isEmptyFallbackFill(name string, as *ast.AssignStmt, parents map[ast.Node]ast.Node) bool {
	if as.Tok != token.ASSIGN {
		return false
	}
	block, ok := parents[as].(*ast.BlockStmt)
	if !ok {
		return false
	}
	ifStmt, ok := parents[block].(*ast.IfStmt)
	if !ok || ifStmt.Body != block {
		return false
	}
	cond, ok := ifStmt.Cond.(*ast.BinaryExpr)
	if !ok || cond.Op != token.EQL {
		return false
	}
	lit, ok := cond.Y.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING || lit.Value != `""` {
		return false
	}
	x := cond.X
	if call, ok := x.(*ast.CallExpr); ok && len(call.Args) == 1 {
		if fun, ok := call.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "strings" && fun.Sel.Name == "TrimSpace" {
				x = call.Args[0]
			}
		}
	}
	id, ok := x.(*ast.Ident)
	return ok && id.Name == name
}

// lane classifies expr; the second result explains the classification
// for the census message.
func (c *laneClassifier) lane(expr ast.Expr) (textLane, string) {
	switch v := expr.(type) {
	case *ast.BasicLit:
		if v.Kind == token.STRING {
			return laneDisplay, "string literal is a display form"
		}
		return laneUnknown, "non-string literal"
	case *ast.ParenExpr:
		return c.lane(v.X)
	case *ast.Ident:
		return c.identLane(v.Name)
	case *ast.SelectorExpr:
		switch v.Sel.Name {
		case "RequestText", "OriginalLine":
			return laneRequest, "." + v.Sel.Name + " selector"
		case "Display":
			return laneDisplay, ".Display selector"
		case "Text":
			return laneRaw, ".Text selector (a line as typed)"
		}
		return laneUnknown, "selector ." + v.Sel.Name + " is outside the request/display field set"
	case *ast.CallExpr:
		switch fun := v.Fun.(type) {
		case *ast.Ident:
			switch fun.Name {
			case "commandOperationFollowupRequestText":
				return laneRequest, "commandOperationFollowupRequestText call"
			case "followUpDisplayText":
				return laneDisplay, "followUpDisplayText call"
			}
			return laneUnknown, "call " + fun.Name + " is outside the request/display helper set"
		case *ast.SelectorExpr:
			if pkg, ok := fun.X.(*ast.Ident); ok && pkg.Name == "strings" && len(v.Args) >= 1 {
				switch fun.Sel.Name {
				case "TrimSpace", "TrimPrefix", "TrimSuffix":
					// trims carry the lane of the text they trim
					return c.lane(v.Args[0])
				}
			}
			return laneUnknown, "call " + exprText(fun) + " is outside the request/display helper set"
		}
		return laneUnknown, "call of an unrecognized shape"
	case *ast.BinaryExpr:
		if v.Op != token.ADD {
			return laneUnknown, "binary " + v.Op.String()
		}
		return c.mergeLanes([]ast.Expr{v.X, v.Y})
	}
	return laneUnknown, fmt.Sprintf("%T is an unrecognized shape", expr)
}

// mergeLanes folds operand lanes: string literals contribute nothing, raw
// folds into request, and any other mix is unknown.
func (c *laneClassifier) mergeLanes(operands []ast.Expr) (textLane, string) {
	result := laneUnknown
	why := "concatenation of literals only"
	for _, op := range operands {
		if lit, ok := op.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			continue
		}
		l, w := c.lane(op)
		switch {
		case l == laneSelf:
			continue // the identifier under resolution itself: lane unchanged
		case l == laneUnknown:
			return laneUnknown, w
		case result == laneUnknown || result == l:
			result, why = l, w
		case result == laneRaw && l == laneRequest, result == laneRequest && l == laneRaw:
			result, why = laneRequest, "request text concatenated with a raw line"
		default:
			return laneUnknown, fmt.Sprintf("concatenation mixes %s and %s lanes", result, l)
		}
	}
	return result, why
}

func (c *laneClassifier) identLane(name string) (textLane, string) {
	if l, ok := c.memo[name]; ok {
		return l, c.memoWhy[name]
	}
	if c.active[name] {
		c.selfHit++
		return laneSelf, "self-reference to " + name
	}
	c.active[name] = true
	defer delete(c.active, name)
	before := c.selfHit
	lane, why := c.resolveIdent(name)
	// A resolution that ran into an outer name's self-reference (`cleaned`
	// resolved while `line` was active) is relative to that outer name and
	// must not be memoized as this name's own lane.
	if c.selfHit == before {
		c.memo[name], c.memoWhy[name] = lane, why
	}
	return lane, why
}

func (c *laneClassifier) resolveIdent(name string) (textLane, string) {
	assigns := c.assigns[name]
	// The parameter lane seeds the merge: a parameter that is only ever
	// re-assigned from itself or filled when empty keeps its lane.
	result := laneUnknown
	why := ""
	if l, ok := c.params[name]; ok {
		if l == laneUnknown {
			return laneUnknown, "parameter " + name + " is outside the line/display/request names"
		}
		result, why = l, "parameter "+name
	} else if len(assigns) == 0 {
		return laneUnknown, "identifier " + name + " has no assignment in " + c.fn.Name.Name + " and is not a parameter"
	}
	sawSelf := false
	for _, a := range assigns {
		var l textLane
		var w string
		switch {
		case a.rhs == nil:
			return laneUnknown, name + " is assigned through an unrecognized multi-value shape"
		case a.index >= 0:
			l, w = c.resultLane(a.rhs, a.index)
		default:
			// `x = rhs` and `x += rhs` alike: rhs's lane merges below with
			// the lane the earlier assignments established.
			l, w = c.lane(a.rhs)
		}
		switch {
		case l == laneSelf:
			sawSelf = true // `x = f(x)` re-assignment: lane unchanged
			continue
		case l == laneUnknown:
			return laneUnknown, name + " ← " + w
		case result == laneUnknown || result == l:
			result, why = l, name+" ← "+w
		case result == laneRaw && l == laneRequest, result == laneRequest && l == laneRaw:
			result, why = laneRequest, name+" ← request text and a raw line"
		default:
			return laneUnknown, fmt.Sprintf("%s is assigned %s and %s lanes in %s", name, result, l, c.fn.Name.Name)
		}
	}
	if result == laneUnknown {
		if sawSelf {
			// every producing assignment resolved back to the name under
			// resolution (`cleaned, _ := splitPastedLog(line)` while
			// resolving line): this local is that name's own text
			return laneSelf, name + " derives from the name under resolution"
		}
		return laneUnknown, name + " has only empty-fill assignments in " + c.fn.Name.Name + " and no producing one"
	}
	return result, why
}

// resultLane classifies result index i of a multi-value call assignment.
func (c *laneClassifier) resultLane(rhs ast.Expr, i int) (textLane, string) {
	call, ok := rhs.(*ast.CallExpr)
	if !ok {
		return laneUnknown, "multi-value assignment from a non-call"
	}
	if fun, ok := call.Fun.(*ast.Ident); ok {
		// splitPastedLog(x) → (cleaned, detected): the cleaned text is x
		// minus the auto-detected log and keeps x's lane.
		if fun.Name == "splitPastedLog" && i == 0 && len(call.Args) == 1 {
			return c.lane(call.Args[0])
		}
		return laneUnknown, "multi-value call " + fun.Name + " is outside the request/display producer set"
	}
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return laneUnknown, "multi-value call of an unrecognized shape"
	}
	// the producer must be called on the enclosing *REPL method's own
	// receiver ident (review round five #5), not on the literal `r`
	if recv, ok := sel.X.(*ast.Ident); !ok || c.recv == "" || recv.Name != c.recv {
		return laneUnknown, "multi-value call " + exprText(call.Fun) + " is outside the request/display producer set (not on the enclosing *REPL method's receiver)"
	}
	switch {
	case sel.Sel.Name == "readInputPair" && i == 0:
		return laneRaw, "readInputPair line result"
	case sel.Sel.Name == "readInputPair" && i == 1:
		return laneDisplay, "readInputPair display result"
	case sel.Sel.Name == "expandTemplateCommand" && i == 0:
		return laneRequest, "expandTemplateCommand expanded result"
	}
	return laneUnknown, fmt.Sprintf("result %d of r.%s is outside the request/display producer set", i, sel.Sel.Name)
}

func exprText(e ast.Expr) string {
	switch v := e.(type) {
	case *ast.Ident:
		return v.Name
	case *ast.BasicLit:
		return v.Value
	case *ast.SelectorExpr:
		return exprText(v.X) + "." + v.Sel.Name
	case *ast.CallExpr:
		return exprText(v.Fun) + "(…)"
	case *ast.ParenExpr:
		return "(" + exprText(v.X) + ")"
	case *ast.StarExpr:
		return "*" + exprText(v.X)
	case *ast.BinaryExpr:
		return exprText(v.X) + " " + v.Op.String() + " " + exprText(v.Y)
	}
	return fmt.Sprintf("%T", e)
}
